package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestMarkdownXHTMLEscapesTextAndRendersFootnotes(t *testing.T) {
	got, err := RenderMarkdownXHTML("1. Keep <this> & that.[^26]\n\n[^26]: Note keeps <tag> & value.")
	if err != nil {
		t.Fatalf("RenderMarkdownXHTML: %v", err)
	}
	want := `<ol>
<li>Keep &lt;this&gt; &amp; that.<span><input type="checkbox" id="cb26" /><label for="cb26"><sup>26</sup></label><span>Note keeps &lt;tag&gt; &amp; value.</span></span></li>
</ol>`
	if got != want {
		t.Fatalf("rendered markdown =\n%s\nwant\n%s", got, want)
	}
}

func TestMarkdownXHTMLRawBlockFootnotes(t *testing.T) {
	got, err := RenderMarkdownXHTML("<p>Text.[^1]</p>\n\n[^1]: Raw note.")
	if err != nil {
		t.Fatalf("RenderMarkdownXHTML: %v", err)
	}
	want := `<p>Text.<span><input type="checkbox" id="cb1" /><label for="cb1"><sup>1</sup></label><span>Raw note.</span></span></p>`
	if got != want {
		t.Fatalf("rendered markdown =\n%s\nwant\n%s", got, want)
	}
}

func TestMarkdownXHTMLInlineSyntax(t *testing.T) {
	got, err := RenderMarkdownXHTML(`[Sample](https://example.org) and [Gemini](gemini://example.org) keep \_literal\_ plus _em_, *also em*, **strong**, __also strong__, and ` + "`code & <tag>`" + `.`)
	if err != nil {
		t.Fatalf("RenderMarkdownXHTML: %v", err)
	}
	want := `<p><a href="https://example.org">Sample</a> and <a href="gemini://example.org">Gemini</a> keep _literal_ plus <em>em</em>, <em>also em</em>, <strong>strong</strong>, <strong>also strong</strong>, and <code>code &amp; &lt;tag&gt;</code>.</p>`
	if got != want {
		t.Fatalf("rendered markdown =\n%s\nwant\n%s", got, want)
	}
}

func TestMarkdownXHTMLBlocks(t *testing.T) {
	input := `Setext Title
============

## ATX Section {#atx-section}

- Alpha
- Beta

- [x] Done
- [ ] Later

1. First
2. Second

> Quoted *voice*.
>
> Still quoted.

| Planet | Gift |
| --- | --- |
| Sun | Sight |
| Moon | Growth |

` + "```text" + `
if x < y {
  return x & y
}
` + "```" + `

---`
	got, err := RenderMarkdownXHTML(input)
	if err != nil {
		t.Fatalf("RenderMarkdownXHTML: %v", err)
	}
	for _, want := range []string{
		`<h1>Setext Title</h1>`,
		`<h2 id="atx-section">ATX Section</h2>`,
		"<ul>\n<li>Alpha</li>\n<li>Beta</li>\n</ul>",
		`<ul class="task-list">`,
		`<li class="task-list-item">&#9745; Done</li>`,
		`<li class="task-list-item">&#9744; Later</li>`,
		"<ol>\n<li>First</li>\n<li>Second</li>\n</ol>",
		"<blockquote>\n<p>Quoted <em>voice</em>.</p>\n<p>Still quoted.</p>\n</blockquote>",
		"<table>",
		"<th>Planet</th>",
		"<td>Growth</td>",
		"<pre><code>if x &lt; y {\n  return x &amp; y\n}</code></pre>",
		"<hr />",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered markdown missing %q:\n%s", want, got)
		}
	}
}

func TestMarkdownXHTMLImageResolver(t *testing.T) {
	got, err := RenderMarkdownXHTMLWithImages(`![Solar mark](assets/kore.png "Embedded local image")`, func(path, alt, title string) (string, error) {
		if path != "assets/kore.png" || alt != "Solar mark" || title != "Embedded local image" {
			t.Fatalf("image resolver args = %q %q %q", path, alt, title)
		}
		return `<img src="data:image/png;base64,AA==" alt="Solar mark" title="Embedded local image">`, nil
	})
	if err != nil {
		t.Fatalf("RenderMarkdownXHTMLWithImages: %v", err)
	}
	want := `<p><img src="data:image/png;base64,AA==" alt="Solar mark" title="Embedded local image"></p>`
	if got != want {
		t.Fatalf("rendered image = %q, want %q", got, want)
	}
}

func TestMarkdownXHTMLRawBlocksPassThrough(t *testing.T) {
	input := `<ol>
  <li><i>Lorem ipsum dolor sit amet.</i></li>
</ol>

<svg role="img">
  <title>Clean</title>
</svg>`
	got, err := RenderMarkdownXHTML(input)
	if err != nil {
		t.Fatalf("RenderMarkdownXHTML: %v", err)
	}
	want := `<ol>
  <li><i>Lorem ipsum dolor sit amet.</i></li>
</ol>
<svg role="img">
  <title>Clean</title>
</svg>`
	if got != want {
		t.Fatalf("raw blocks changed:\n%s", got)
	}
}

func TestMarkdownXHTMLFootnoteErrors(t *testing.T) {
	tests := map[string]string{
		"missing":   "Text.[^1]",
		"duplicate": "Text.[^1]\n\n[^1]: one\n[^1]: two",
		"unused":    "Text.\n\n[^1]: one",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := RenderMarkdownXHTML(input); err == nil {
				t.Fatalf("RenderMarkdownXHTML accepted %s footnote", name)
			}
		})
	}
}

func TestMarkdownXHTMLFootnoteBodiesAreNotRecursivelyScanned(t *testing.T) {
	got, err := RenderMarkdownXHTML("Text.[^1]\n\n[^1]: literal [^2]\n[^2]: unused")
	if err == nil {
		t.Fatalf("RenderMarkdownXHTML accepted unused recursive footnote, got %s", got)
	}
	if !strings.Contains(err.Error(), "unused footnote definition: 2") {
		t.Fatalf("error = %v, want unused footnote definition", err)
	}
}

func TestMarkdownXHTMLRejectsMalformedSyntax(t *testing.T) {
	tests := map[string]string{
		"unclosed fence": "```text\nmissing close",
		"unclosed code":  "`missing close",
		"bad link":       "[bad](https://example.org",
		"bad image":      "![bad](assets/x.png",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := RenderMarkdownXHTML(input); err == nil {
				t.Fatalf("RenderMarkdownXHTML accepted %s", name)
			}
		})
	}
}

func TestMarkdownXHTMLRejectsUnsafeLinks(t *testing.T) {
	tests := []string{
		"[bad](javascript:alert(1))",
		"[bad](data:text/html,evil)",
		"[bad](ftp://example.org/file)",
		"[bad](//example.org/file)",
	}
	for _, input := range tests {
		if _, err := RenderMarkdownXHTML(input); err == nil {
			t.Fatalf("RenderMarkdownXHTML accepted unsafe link %q", input)
		}
	}
}

func TestMarkdownXHTMLRejectsRemoteImages(t *testing.T) {
	tests := []string{
		"![bad](https://example.org/x.png)",
		"![bad](http://example.org/x.png)",
		"![bad](//example.org/x.png)",
		"![bad](data:image/png;base64,AA==)",
	}
	for _, input := range tests {
		if _, err := RenderMarkdownXHTMLWithImages(input, func(path, alt, title string) (string, error) {
			return "", nil
		}); err == nil {
			t.Fatalf("RenderMarkdownXHTMLWithImages accepted remote image %q", input)
		}
	}
}

func TestMarkdownGemtextRendersLossySubset(t *testing.T) {
	input := `Setext Title
============

## ATX Section {#atx-section}

Paragraph with [docs](https://example.org), *emphasis*, **strong**, ` + "`inline code`" + `, and a note.[^1]

- Alpha
- Beta

- [x] Done
- [ ] Later

1. First
2. Second

> Quoted *voice*.
>
> Still quoted.

| Planet | Gift |
| --- | --- |
| Sun | Sight |
| Moon | Growth |

` + "```text" + `
if x < y {
  return x & y
}
` + "```" + `

---

<section>
  <p>Raw <em>XHTML</em> text.</p>
</section>

[^1]: Footnote *body*.`
	got, err := RenderMarkdownGemtext(input)
	if err != nil {
		t.Fatalf("RenderMarkdownGemtext: %v", err)
	}
	for _, want := range []string{
		"# Setext Title",
		"## ATX Section",
		"Paragraph with docs, emphasis, strong, inline code, and a note. (Footnote body.)",
		"=> https://example.org docs",
		"* Alpha",
		"* Beta",
		"* [x] Done",
		"* [ ] Later",
		"* 1. First",
		"* 2. Second",
		"> Quoted voice.",
		"> Still quoted.",
		"```\n| Planet | Gift",
		"| Moon   | Growth |",
		"```\n\n```\nif x < y {\n  return x & y\n}\n```",
		"---",
		"Raw XHTML text.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered gemtext missing %q:\n%s", want, got)
		}
	}
}

func TestMarkdownGemtextCollapsesDeepHeadings(t *testing.T) {
	got, err := RenderMarkdownGemtext("##### Deep Heading")
	if err != nil {
		t.Fatalf("RenderMarkdownGemtext: %v", err)
	}
	if got != "### Deep Heading" {
		t.Fatalf("deep heading = %q", got)
	}
}

func TestMarkdownGemtextRejectsImagesAndMalformedRawXHTML(t *testing.T) {
	tests := map[string]string{
		"image":             "![bad](assets/x.png)",
		"malformed raw":     "<section><p>Missing close</section>",
		"unsupported raw":   "<section><script>alert(1)</script></section>",
		"unsupported fence": "```\n``` nested\n```",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := RenderMarkdownGemtext(input); err == nil {
				t.Fatalf("RenderMarkdownGemtext accepted %s", name)
			}
		})
	}
}

func TestSlugifyHeadingID(t *testing.T) {
	tests := map[string]string{
		"Sample Section":     "sample-section",
		"FAQ":                "faq",
		"**Bold** & Friends": "bold-friends",
		"  --x--  ":          "x",
		"Café":               "caf",
		"日本語":                "",
		"A  B":               "a-b",
		"123":                "123",
	}
	for input, want := range tests {
		if got := slugifyHeadingID(input); got != want {
			t.Fatalf("slugifyHeadingID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMarkdownTOCGeneration(t *testing.T) {
	input := strings.Join([]string{
		"## First Section",
		"",
		"Text.",
		"",
		"## Custom Title {#custom-anchor}",
		"",
		"More.",
		"",
		"## **Bold** & Friends",
		"",
		"End.",
	}, "\n")
	html, toc, err := RenderMarkdownXHTMLDocument(input, nil, true)
	if err != nil {
		t.Fatalf("RenderMarkdownXHTMLDocument: %v", err)
	}
	want := []TOCEntry{
		{Href: "#first-section", Text: "First Section"},
		{Href: "#custom-anchor", Text: "Custom Title"},
		{Href: "#bold-friends", Text: "<strong>Bold</strong> &amp; Friends"},
	}
	if !reflect.DeepEqual(toc, want) {
		t.Fatalf("toc = %#v, want %#v", toc, want)
	}
	for _, marker := range []string{
		`<h2 id="first-section">First Section</h2>`,
		`<h2 id="custom-anchor">Custom Title</h2>`,
		`<h2 id="bold-friends"><strong>Bold</strong> &amp; Friends</h2>`,
	} {
		if !strings.Contains(html, marker) {
			t.Fatalf("output missing %q in:\n%s", marker, html)
		}
	}
}

func TestMarkdownTOCDisabledRendersByteIdentical(t *testing.T) {
	input := "## First Section\n\nText.\n\n## Second Section\n\nMore.\n"
	plain, err := RenderMarkdownXHTML(input)
	if err != nil {
		t.Fatalf("RenderMarkdownXHTML: %v", err)
	}
	if strings.Contains(plain, "id=") {
		t.Fatalf("toc-off render injected ids: %s", plain)
	}
	doc, toc, err := RenderMarkdownXHTMLDocument(input, nil, false)
	if err != nil {
		t.Fatalf("RenderMarkdownXHTMLDocument: %v", err)
	}
	if toc != nil {
		t.Fatalf("toc-off collected entries: %#v", toc)
	}
	if doc != plain {
		t.Fatalf("toc-off output differs:\n%s\nvs\n%s", doc, plain)
	}
}

func TestMarkdownTOCErrors(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"zero level-2 headings": {
			input: "# Title\n\nText.\n",
			want:  "toc requested but no level-2 headings found",
		},
		"underivable anchor": {
			input: "## 日本語\n\nText.\n",
			want:  "cannot derive an anchor",
		},
		"duplicate auto anchors": {
			input: "## Same\n\nText.\n\n## Same\n\nMore.\n",
			want:  `duplicate heading anchor "same"`,
		},
		"auto collides with explicit": {
			input: "## Alpha {#beta}\n\nText.\n\n## Beta\n\nMore.\n",
			want:  `duplicate heading anchor "beta"`,
		},
		"explicit collides with explicit": {
			input: "## A {#x}\n\nText.\n\n## B {#x}\n\nMore.\n",
			want:  `duplicate heading anchor "x"`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := RenderMarkdownXHTMLDocument(tc.input, nil, true)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("RenderMarkdownXHTMLDocument error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestMarkdownTOCSkipsFencesAndBlockquotes(t *testing.T) {
	input := strings.Join([]string{
		"```",
		"## Fenced Fake",
		"```",
		"",
		"> ## Quoted Heading",
		"",
		"## Real Section",
		"",
		"Text.",
	}, "\n")
	html, toc, err := RenderMarkdownXHTMLDocument(input, nil, true)
	if err != nil {
		t.Fatalf("RenderMarkdownXHTMLDocument: %v", err)
	}
	want := []TOCEntry{{Href: "#real-section", Text: "Real Section"}}
	if !reflect.DeepEqual(toc, want) {
		t.Fatalf("toc = %#v, want %#v", toc, want)
	}
	if !strings.Contains(html, "<h2>Quoted Heading</h2>") {
		t.Fatalf("blockquote heading gained an id: %s", html)
	}
}

func TestMarkdownTOCStripsFootnoteRefsFromEntryText(t *testing.T) {
	input := "## Notes[^1]\n\nText.\n\n[^1]: A note.\n"
	html, toc, err := RenderMarkdownXHTMLDocument(input, nil, true)
	if err != nil {
		t.Fatalf("RenderMarkdownXHTMLDocument: %v", err)
	}
	want := []TOCEntry{{Href: "#notes", Text: "Notes"}}
	if !reflect.DeepEqual(toc, want) {
		t.Fatalf("toc = %#v, want %#v", toc, want)
	}
	if !strings.Contains(html, `<h2 id="notes">`) {
		t.Fatalf("heading missing id: %s", html)
	}
}
