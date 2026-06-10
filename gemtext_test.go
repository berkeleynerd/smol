package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestGemtextRendersConstrainedXHTML(t *testing.T) {
	input := `# Title & <One>
## Section
### Subsection
Text & <tag>.

=> gemini://example.org Capsule & docs
=> /about

* one
* two & three

> quoted & line
> second

` + "```" + `
pre & <raw>
* still pre
` + "```" + `
`
	got, err := RenderGemtextXHTML(input)
	if err != nil {
		t.Fatalf("RenderGemtextXHTML: %v", err)
	}
	want := `<h1>Title &amp; &lt;One&gt;</h1>
<h2>Section</h2>
<h3>Subsection</h3>
<p>Text &amp; &lt;tag&gt;.</p>
<p><a href="gemini://example.org">Capsule &amp; docs</a></p>
<p><a href="/about">/about</a></p>
<ul>
  <li>one</li>
  <li>two &amp; three</li>
</ul>
<blockquote>
  <p>quoted &amp; line</p>
  <p>second</p>
</blockquote>
<pre>pre &amp; &lt;raw&gt;
* still pre</pre>`
	if got != want {
		t.Fatalf("rendered gemtext =\n%s\nwant\n%s", got, want)
	}
}

func TestGemtextRejectsMissingLinkURL(t *testing.T) {
	if _, err := RenderGemtextXHTML("=>   \n"); err == nil {
		t.Fatalf("RenderGemtextXHTML accepted a link without a URL")
	}
}

func TestGemtextBodyBuildsFromBodyGMI(t *testing.T) {
	dir := testSite(t)
	writeContentSource(t, dir, "page", "capsule", ".gmi", []string{"title: Capsule"}, `# Gemtext Source

=> / About
* item
`)
	buildSite(t, dir)
	html := readText(t, filepath.Join(dir, "public", "capsule", "index.html"))
	for _, needle := range []string{
		"<h1>Gemtext Source</h1>",
		`<p><a href="/">About</a></p>`,
		"<li>item</li>",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("generated HTML missing %q:\n%s", needle, html)
		}
	}
}
