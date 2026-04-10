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

	var builder strings.Builder
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
		if normalizedTag, ok := normalizeEscapedTag(candidate); ok {
			builder.WriteString(normalizedTag)
		} else {
			builder.WriteString(candidate)
		}

		idx = end
	}

	return builder.String()
}

func normalizeEscapedTag(candidate string) (string, bool) {
	decoded := html.UnescapeString(candidate)
	if decoded == candidate || decoded == "" {
		return "", false
	}

	tokenizer := xhtml.NewTokenizer(strings.NewReader(decoded))
	tokenType := tokenizer.Next()

	switch tokenType {
	case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
		tagName, hasAttrs := tokenizer.TagName()
		spec, allowed := telegramTagSpec(strings.ToLower(string(tagName)), collectAttributes(tokenizer, hasAttrs), false)
		if !allowed {
			return "", false
		}

		if !tokenizerOnlyEOF(tokenizer) {
			return "", false
		}

		var builder strings.Builder
		writeStartTag(&builder, spec)
		if tokenType == xhtml.SelfClosingTagToken {
			writeEndTag(&builder, spec.name)
		}

		return builder.String(), true
	case xhtml.EndTagToken:
		tagName, _ := tokenizer.TagName()
		canonicalTagName, allowed := canonicalEndTagName(strings.ToLower(string(tagName)))
		if !allowed {
			return "", false
		}

		if !tokenizerOnlyEOF(tokenizer) {
			return "", false
		}

		var builder strings.Builder
		writeEndTag(&builder, canonicalTagName)
		return builder.String(), true
	default:
		return "", false
	}
}

func tokenizerOnlyEOF(tokenizer *xhtml.Tokenizer) bool {
	if tokenizer == nil {
		return false
	}

	tokenType := tokenizer.Next()
	return tokenType == xhtml.ErrorToken && errors.Is(tokenizer.Err(), io.EOF)
}
