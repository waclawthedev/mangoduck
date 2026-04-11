package tghtml

import (
	"errors"
	"html"
	"io"
	"strings"

	xhtml "golang.org/x/net/html"
)

const (
	attrClass      = "class"
	attrEmojiID    = "emoji-id"
	attrExpandable = "expandable"
	attrFormat     = "format"
	attrHref       = "href"
	attrUnix       = "unix"
	tagA           = "a"
	tagB           = "b"
	tagBlockquote  = "blockquote"
	tagCode        = "code"
	tagI           = "i"
	tagPre         = "pre"
	tagS           = "s"
	tagSpoiler     = "tg-spoiler"
	tagTGEmoji     = "tg-emoji"
	tagTGTime      = "tg-time"
	tagU           = "u"
)

type tagSpec struct {
	name  string
	attrs map[string]string
}

func Sanitize(text string) string {
	var builder strings.Builder
	var stack []string

	tokenizer := xhtml.NewTokenizer(strings.NewReader(text))

	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case xhtml.ErrorToken:
			if errors.Is(tokenizer.Err(), io.EOF) {
				closeAllOpenTags(&builder, &stack)
				return strings.TrimSpace(builder.String())
			}

			closeAllOpenTags(&builder, &stack)
			builder.WriteString(html.EscapeString(text))
			return strings.TrimSpace(builder.String())
		case xhtml.TextToken:
			token := tokenizer.Token()
			builder.WriteString(html.EscapeString(token.Data))
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			tagName, hasAttrs := tokenizer.TagName()
			handleStartTag(&builder, &stack, strings.ToLower(string(tagName)), collectAttributes(tokenizer, hasAttrs), tokenType == xhtml.SelfClosingTagToken)
		case xhtml.EndTagToken:
			tagName, _ := tokenizer.TagName()
			handleEndTag(&builder, &stack, strings.ToLower(string(tagName)))
		case xhtml.CommentToken, xhtml.DoctypeToken:
			continue
		}
	}
}

func collectAttributes(tokenizer *xhtml.Tokenizer, hasAttrs bool) map[string]string {
	attrs := make(map[string]string)
	for hasAttrs {
		key, value, moreAttrs := tokenizer.TagAttr()
		attrName := strings.ToLower(string(key))
		if _, exists := attrs[attrName]; !exists {
			attrs[attrName] = string(value)
		}

		hasAttrs = moreAttrs
	}

	return attrs
}

func handleStartTag(builder *strings.Builder, stack *[]string, tagName string, attrs map[string]string, selfClosing bool) {
	switch tagName {
	case "br", "div", "ul", "ol":
		appendNewlines(builder, 1)
		return
	case "p":
		appendNewlines(builder, 2)
		return
	case "li":
		appendListItemPrefix(builder)
		if selfClosing {
			appendNewlines(builder, 1)
		}
		return
	}

	spec, allowed := telegramTagSpec(tagName, attrs, isTagOpen(*stack, tagPre))
	if !allowed {
		return
	}

	writeStartTag(builder, spec)
	if selfClosing {
		writeEndTag(builder, spec.name)
		return
	}

	*stack = append(*stack, spec.name)
}

func handleEndTag(builder *strings.Builder, stack *[]string, tagName string) {
	switch tagName {
	case "p":
		appendNewlines(builder, 2)
		return
	case "div", "ul", "ol", "li":
		appendNewlines(builder, 1)
		return
	}

	canonicalTagName, allowed := canonicalEndTagName(tagName)
	if !allowed {
		return
	}

	closeOpenTag(builder, stack, canonicalTagName)
}

func telegramTagSpec(tagName string, attrs map[string]string, insidePre bool) (*tagSpec, bool) {
	switch tagName {
	case tagB, "strong":
		return &tagSpec{name: tagB}, true
	case tagI, "em":
		return &tagSpec{name: tagI}, true
	case tagU, "ins":
		return &tagSpec{name: tagU}, true
	case tagS, "strike", "del":
		return &tagSpec{name: tagS}, true
	case tagSpoiler:
		return &tagSpec{name: tagSpoiler}, true
	case "span":
		if strings.EqualFold(attrs[attrClass], tagSpoiler) {
			return &tagSpec{name: tagSpoiler}, true
		}
	case tagA:
		href := strings.TrimSpace(attrs[attrHref])
		if href == "" {
			return nil, false
		}

		var tag tagSpec
		tag.name = tagA
		tag.attrs = map[string]string{attrHref: href}
		return &tag, true
	case tagCode:
		var tag tagSpec
		tag.name = tagCode
		if insidePre {
			className := strings.TrimSpace(attrs[attrClass])
			if strings.HasPrefix(strings.ToLower(className), "language-") {
				tag.attrs = map[string]string{attrClass: className}
			}
		}

		return &tag, true
	case tagPre:
		return &tagSpec{name: tagPre}, true
	case tagBlockquote:
		var tag tagSpec
		tag.name = tagBlockquote
		if _, expandable := attrs[attrExpandable]; expandable {
			tag.attrs = map[string]string{attrExpandable: ""}
		}

		return &tag, true
	case tagTGEmoji:
		emojiID := strings.TrimSpace(attrs[attrEmojiID])
		if emojiID == "" {
			return nil, false
		}

		var tag tagSpec
		tag.name = tagTGEmoji
		tag.attrs = map[string]string{attrEmojiID: emojiID}
		return &tag, true
	case tagTGTime:
		var tag tagSpec
		tag.name = tagTGTime
		tag.attrs = make(map[string]string)

		unixValue := strings.TrimSpace(attrs[attrUnix])
		if unixValue != "" {
			tag.attrs[attrUnix] = unixValue
		}

		formatValue := strings.TrimSpace(attrs[attrFormat])
		if formatValue != "" {
			tag.attrs[attrFormat] = formatValue
		}

		if len(tag.attrs) == 0 {
			return nil, false
		}

		return &tag, true
	}

	return nil, false
}

func canonicalEndTagName(tagName string) (string, bool) {
	switch tagName {
	case tagB, "strong":
		return tagB, true
	case tagI, "em":
		return tagI, true
	case tagU, "ins":
		return tagU, true
	case tagS, "strike", "del":
		return tagS, true
	case "span", tagSpoiler:
		return tagSpoiler, true
	case tagA, tagCode, tagPre, tagBlockquote, tagTGEmoji, tagTGTime:
		return tagName, true
	}

	return "", false
}

func writeStartTag(builder *strings.Builder, spec *tagSpec) {
	builder.WriteByte('<')
	builder.WriteString(spec.name)

	switch spec.name {
	case tagA:
		appendEscapedAttribute(builder, attrHref, spec.attrs[attrHref])
	case tagCode:
		if className, ok := spec.attrs[attrClass]; ok {
			appendEscapedAttribute(builder, attrClass, className)
		}
	case tagBlockquote:
		if _, ok := spec.attrs[attrExpandable]; ok {
			builder.WriteByte(' ')
			builder.WriteString(attrExpandable)
		}
	case tagTGEmoji:
		appendEscapedAttribute(builder, attrEmojiID, spec.attrs[attrEmojiID])
	case tagTGTime:
		if unixValue, ok := spec.attrs[attrUnix]; ok {
			appendEscapedAttribute(builder, attrUnix, unixValue)
		}
		if formatValue, ok := spec.attrs[attrFormat]; ok {
			appendEscapedAttribute(builder, attrFormat, formatValue)
		}
	}

	builder.WriteByte('>')
}

func appendEscapedAttribute(builder *strings.Builder, name string, value string) {
	builder.WriteByte(' ')
	builder.WriteString(name)
	builder.WriteString(`="`)
	builder.WriteString(html.EscapeString(value))
	builder.WriteByte('"')
}

func writeEndTag(builder *strings.Builder, tagName string) {
	builder.WriteString("</")
	builder.WriteString(tagName)
	builder.WriteByte('>')
}

func closeOpenTag(builder *strings.Builder, stack *[]string, tagName string) {
	for idx := len(*stack) - 1; idx >= 0; idx-- {
		if (*stack)[idx] != tagName {
			continue
		}

		for len(*stack) > idx {
			lastIdx := len(*stack) - 1
			writeEndTag(builder, (*stack)[lastIdx])
			*stack = (*stack)[:lastIdx]
		}

		return
	}
}

func closeAllOpenTags(builder *strings.Builder, stack *[]string) {
	for len(*stack) > 0 {
		lastIdx := len(*stack) - 1
		writeEndTag(builder, (*stack)[lastIdx])
		*stack = (*stack)[:lastIdx]
	}
}

func appendListItemPrefix(builder *strings.Builder) {
	appendNewlines(builder, 1)
	builder.WriteString("- ")
}

func appendNewlines(builder *strings.Builder, count int) {
	if count <= 0 {
		return
	}

	current := builder.String()
	trailingNewlines := 0
	for idx := len(current) - 1; idx >= 0 && current[idx] == '\n'; idx-- {
		trailingNewlines++
	}

	if trailingNewlines >= count {
		return
	}

	for idx := trailingNewlines; idx < count; idx++ {
		builder.WriteByte('\n')
	}
}

func isTagOpen(stack []string, tagName string) bool {
	for idx := len(stack) - 1; idx >= 0; idx-- {
		if stack[idx] == tagName {
			return true
		}
	}

	return false
}
