package tghtml

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitize_ReplacesTelegramUnsupportedHTML(t *testing.T) {
	t.Parallel()

	input := "<div>Hello<br/>world</div><ul><li>one</li><li>two</li></ul>"
	require.Equal(t, "Hello\nworld\n- one\n- two", Sanitize(input))
}

func TestSanitize_LeavesPlainTextUntouched(t *testing.T) {
	t.Parallel()

	require.Equal(t, "hello\nworld", Sanitize("hello\nworld"))
}

func TestSanitize_PreservesTelegramSupportedHTML(t *testing.T) {
	t.Parallel()

	input := `<strong>bold</strong> <em>italic</em> <ins>under</ins> <del>gone</del> <span class="tg-spoiler">spoiler</span> <a href="https://example.com?a=1&b=2" onclick="nope">link</a>`
	expected := `<b>bold</b> <i>italic</i> <u>under</u> <s>gone</s> <tg-spoiler>spoiler</tg-spoiler> <a href="https://example.com?a=1&amp;b=2">link</a>`
	require.Equal(t, expected, Sanitize(input))
}

func TestSanitize_EscapesRawSpecialCharacters(t *testing.T) {
	t.Parallel()

	input := `Fish & Chips <b>5 < 6 & 7</b>`
	expected := `Fish &amp; Chips <b>5 &lt; 6 &amp; 7</b>`
	require.Equal(t, expected, Sanitize(input))
}

func TestSanitize_PreservesSupportedCodeAndBlockquoteAttributes(t *testing.T) {
	t.Parallel()

	input := `<blockquote expandable="true">quote</blockquote><pre><code class="language-go" data-x="1">fmt.Println(x < y)</code></pre>`
	expected := `<blockquote expandable>quote</blockquote><pre><code class="language-go">fmt.Println(x &lt; y)</code></pre>`
	require.Equal(t, expected, Sanitize(input))
}

func TestSanitize_DropsUnsupportedTagsButKeepsText(t *testing.T) {
	t.Parallel()

	input := `<span style="color:red">hello</span><table><tr><td>world</td></tr></table><tg-emoji emoji-id="123" data-x="1"></tg-emoji>`
	expected := `helloworld<tg-emoji emoji-id="123"></tg-emoji>`
	require.Equal(t, expected, Sanitize(input))
}

func TestNormalize_RestoresEscapedSupportedTags(t *testing.T) {
	t.Parallel()

	input := `Done. 1) &lt;b&gt;a.txt&lt;/b&gt;`
	expected := `Done. 1) <b>a.txt</b>`
	require.Equal(t, expected, Normalize(input))
}

func TestNormalize_LeavesStandaloneEscapedSupportedTagsEscaped(t *testing.T) {
	t.Parallel()

	input := `Use &lt;b&gt; to start bold text.`
	require.Equal(t, input, Normalize(input))
}

func TestNormalize_LeavesUnsupportedEscapedTagsEscaped(t *testing.T) {
	t.Parallel()

	input := `&lt;table&gt;hello&lt;/table&gt;`
	require.Equal(t, input, Normalize(input))
}

func TestNormalize_RestoresEscapedSupportedAttributesOnly(t *testing.T) {
	t.Parallel()

	input := `&lt;a href=&quot;https://example.com?a=1&amp;amp;b=2&quot; onclick=&quot;nope&quot;&gt;link&lt;/a&gt;`
	expected := `<a href="https://example.com?a=1&amp;b=2">link</a>`
	require.Equal(t, expected, Normalize(input))
}

func TestNormalize_LeavesExistingHTMLUntouched(t *testing.T) {
	t.Parallel()

	input := `<b>ready</b>`
	require.Equal(t, input, Normalize(input))
}

func TestNormalize_LeavesEscapedTagsInsideCodeUntouched(t *testing.T) {
	t.Parallel()

	input := `<code>&lt;b&gt;literal&lt;/b&gt;</code>`
	require.Equal(t, input, Normalize(input))
}
