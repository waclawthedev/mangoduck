package tghtml

import (
	"errors"
	"html"
	"io"
	"strings"

	xhtml "golang.org/x/net/html"
)

const (
	escapedTagOpen  = "&lt;"
	escapedTagClose = "&gt;"
)

// Normalize restores only Telegram-supported HTML tags from escaped text while
// leaving all other content unchanged.
func Normalize(text string) string {
	if text == "" {
		return ""
	}

	var rawTagStack []string
	var builder strings.Builder

	tokenizer := xhtml.NewTokenizer(strings.NewReader(text))
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case xhtml.ErrorToken:
			if errors.Is(tokenizer.Err(), io.EOF) {
				return builder.String()
			}

			return text
		case xhtml.TextToken:
			rawText := string(tokenizer.Raw())
			if isTagOpen(rawTagStack, tagPre) || isTagOpen(rawTagStack, tagCode) {
				builder.WriteString(rawText)
				continue
			}

			builder.WriteString(normalizeEscapedTextSegment(rawText))
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			tagName, _ := tokenizer.TagName()
			tagNameLower := strings.ToLower(string(tagName))
			builder.Write(tokenizer.Raw())
			if tokenType != xhtml.SelfClosingTagToken && (tagNameLower == tagPre || tagNameLower == tagCode) {
				rawTagStack = append(rawTagStack, tagNameLower)
			}
		case xhtml.EndTagToken:
			tagName, _ := tokenizer.TagName()
			tagNameLower := strings.ToLower(string(tagName))
			builder.Write(tokenizer.Raw())
			if tagNameLower == tagPre || tagNameLower == tagCode {
				popOpenTag(&rawTagStack, tagNameLower)
			}
		case xhtml.CommentToken, xhtml.DoctypeToken:
			builder.Write(tokenizer.Raw())
		}
	}
}

func normalizeEscapedTextSegment(text string) string {
	var builder strings.Builder
	var escapedTagStack []string

	for idx := 0; idx < len(text); {
		start := strings.Index(text[idx:], escapedTagOpen)
		if start < 0 {
			builder.WriteString(text[idx:])
			break
		}

		start += idx
		builder.WriteString(text[idx:start])

		end := strings.Index(text[start:], escapedTagClose)
		if end < 0 {
			builder.WriteString(text[start:])
			break
		}

		end += start + len(escapedTagClose)
		candidate := text[start:end]
		tag, ok := normalizeEscapedTag(candidate)
		if !ok {
			builder.WriteString(candidate)
			idx = end
			continue
		}

		switch {
		case tag.selfClosing:
			builder.WriteString(tag.rendered)
		case tag.closing:
			if len(escapedTagStack) > 0 && escapedTagStack[len(escapedTagStack)-1] == tag.name {
				escapedTagStack = escapedTagStack[:len(escapedTagStack)-1]
				builder.WriteString(tag.rendered)
			} else {
				builder.WriteString(candidate)
			}
		default:
			if hasMatchingEscapedEndTag(text[end:], tag.name) {
				escapedTagStack = append(escapedTagStack, tag.name)
				builder.WriteString(tag.rendered)
			} else {
				builder.WriteString(candidate)
			}
		}

		idx = end
	}

	return builder.String()
}

type normalizedEscapedTag struct {
	name        string
	rendered    string
	closing     bool
	selfClosing bool
}

func normalizeEscapedTag(candidate string) (*normalizedEscapedTag, bool) {
	decoded := html.UnescapeString(candidate)
	if decoded == candidate || decoded == "" {
		return nil, false
	}

	tokenizer := xhtml.NewTokenizer(strings.NewReader(decoded))
	tokenType := tokenizer.Next()

	switch tokenType {
	case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
		tagName, hasAttrs := tokenizer.TagName()
		spec, allowed := telegramTagSpec(strings.ToLower(string(tagName)), collectAttributes(tokenizer, hasAttrs), false)
		if !allowed {
			return nil, false
		}

		if !tokenizerOnlyEOF(tokenizer) {
			return nil, false
		}

		var builder strings.Builder
		writeStartTag(&builder, spec)
		if tokenType == xhtml.SelfClosingTagToken {
			writeEndTag(&builder, spec.name)
		}

		return &normalizedEscapedTag{
			name:        spec.name,
			rendered:    builder.String(),
			selfClosing: tokenType == xhtml.SelfClosingTagToken,
		}, true
	case xhtml.EndTagToken:
		tagName, _ := tokenizer.TagName()
		canonicalTagName, allowed := canonicalEndTagName(strings.ToLower(string(tagName)))
		if !allowed {
			return nil, false
		}

		if !tokenizerOnlyEOF(tokenizer) {
			return nil, false
		}

		var builder strings.Builder
		writeEndTag(&builder, canonicalTagName)
		return &normalizedEscapedTag{
			name:     canonicalTagName,
			rendered: builder.String(),
			closing:  true,
		}, true
	case xhtml.ErrorToken, xhtml.TextToken, xhtml.CommentToken, xhtml.DoctypeToken:
		return nil, false
	default:
		return nil, false
	}
}

func hasMatchingEscapedEndTag(text string, tagName string) bool {
	for idx := 0; idx < len(text); {
		start := strings.Index(text[idx:], escapedTagOpen)
		if start < 0 {
			return false
		}

		start += idx
		end := strings.Index(text[start:], escapedTagClose)
		if end < 0 {
			return false
		}

		end += start + len(escapedTagClose)
		tag, ok := normalizeEscapedTag(text[start:end])
		if ok && tag.closing && tag.name == tagName {
			return true
		}

		idx = end
	}

	return false
}

func popOpenTag(stack *[]string, tagName string) {
	if stack == nil || len(*stack) == 0 {
		return
	}

	for idx := len(*stack) - 1; idx >= 0; idx-- {
		if (*stack)[idx] != tagName {
			continue
		}

		*stack = (*stack)[:idx]
		return
	}
}

func tokenizerOnlyEOF(tokenizer *xhtml.Tokenizer) bool {
	if tokenizer == nil {
		return false
	}

	tokenType := tokenizer.Next()
	return tokenType == xhtml.ErrorToken && errors.Is(tokenizer.Err(), io.EOF)
}
