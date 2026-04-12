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
		done, fallback := normalizeToken(&builder, tokenizer, tokenType, &rawTagStack)
		if fallback {
			return text
		}
		if done {
			return builder.String()
		}
	}
}

func normalizeToken(builder *strings.Builder, tokenizer *xhtml.Tokenizer, tokenType xhtml.TokenType, rawTagStack *[]string) (bool, bool) {
	switch tokenType {
	case xhtml.ErrorToken:
		return true, !errors.Is(tokenizer.Err(), io.EOF)
	case xhtml.TextToken:
		builder.WriteString(normalizeRawTextToken(string(tokenizer.Raw()), *rawTagStack))
	case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
		handleNormalizeStartTag(builder, tokenizer, tokenType, rawTagStack)
	case xhtml.EndTagToken:
		handleNormalizeEndTag(builder, tokenizer, rawTagStack)
	case xhtml.CommentToken, xhtml.DoctypeToken:
		builder.Write(tokenizer.Raw())
	}

	return false, false
}

func normalizeRawTextToken(rawText string, rawTagStack []string) string {
	if isTagOpen(rawTagStack, tagPre) || isTagOpen(rawTagStack, tagCode) {
		return rawText
	}

	return normalizeEscapedTextSegment(rawText)
}

func handleNormalizeStartTag(builder *strings.Builder, tokenizer *xhtml.Tokenizer, tokenType xhtml.TokenType, rawTagStack *[]string) {
	tagName, _ := tokenizer.TagName()
	tagNameLower := strings.ToLower(string(tagName))

	builder.Write(tokenizer.Raw())
	if tokenType == xhtml.SelfClosingTagToken || !isRawTagName(tagNameLower) {
		return
	}

	*rawTagStack = append(*rawTagStack, tagNameLower)
}

func handleNormalizeEndTag(builder *strings.Builder, tokenizer *xhtml.Tokenizer, rawTagStack *[]string) {
	tagName, _ := tokenizer.TagName()
	tagNameLower := strings.ToLower(string(tagName))

	builder.Write(tokenizer.Raw())
	if !isRawTagName(tagNameLower) {
		return
	}

	popOpenTag(rawTagStack, tagNameLower)
}

func isRawTagName(tagName string) bool {
	return tagName == tagPre || tagName == tagCode
}

func normalizeEscapedTextSegment(text string) string {
	var builder strings.Builder
	var escapedTagStack []string

	for idx := 0; idx < len(text); {
		start, end, ok := findEscapedTagBounds(text, idx)
		if !ok {
			builder.WriteString(text[idx:])
			break
		}

		builder.WriteString(text[idx:start])
		candidate := text[start:end]
		writeNormalizedEscapedTag(&builder, &escapedTagStack, text[end:], candidate)
		idx = end
	}

	return builder.String()
}

func findEscapedTagBounds(text string, idx int) (int, int, bool) {
	start := strings.Index(text[idx:], escapedTagOpen)
	if start < 0 {
		return 0, 0, false
	}

	start += idx
	end := strings.Index(text[start:], escapedTagClose)
	if end < 0 {
		return start, 0, false
	}

	end += start + len(escapedTagClose)
	return start, end, true
}

func writeNormalizedEscapedTag(builder *strings.Builder, escapedTagStack *[]string, remainingText string, candidate string) {
	tag, ok := normalizeEscapedTag(candidate)
	if !ok {
		builder.WriteString(candidate)
		return
	}

	if shouldRenderEscapedTag(tag, remainingText, escapedTagStack) {
		builder.WriteString(tag.rendered)
		return
	}

	builder.WriteString(candidate)
}

func shouldRenderEscapedTag(tag *normalizedEscapedTag, remainingText string, escapedTagStack *[]string) bool {
	switch {
	case tag.selfClosing:
		return true
	case tag.closing:
		return popMatchingEscapedTag(escapedTagStack, tag.name)
	default:
		if !hasMatchingEscapedEndTag(remainingText, tag.name) {
			return false
		}

		*escapedTagStack = append(*escapedTagStack, tag.name)
		return true
	}
}

func popMatchingEscapedTag(escapedTagStack *[]string, tagName string) bool {
	if escapedTagStack == nil || len(*escapedTagStack) == 0 {
		return false
	}

	lastIndex := len(*escapedTagStack) - 1
	if (*escapedTagStack)[lastIndex] != tagName {
		return false
	}

	*escapedTagStack = (*escapedTagStack)[:lastIndex]
	return true
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

func hasMatchingEscapedEndTag(text, tagName string) bool {
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
