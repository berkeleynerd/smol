package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// These tests exercise only the public source renderer and the unchanged
// publication validators. An annotation must not create a second HTML path.
func TestAnnotatedSecurityExistingPublicationBoundary(t *testing.T) {
	input := "## Witness {#witness}\n\n" +
		"{anchor|Luke.11.2}{u-solid|A & B {u-dotted|Greek κομᾶν}} P{sup|69}{br}Next.[^240]\n\n" +
		"[^240]: First **paragraph** with [source](https://example.org/?a=1&b=2).\n" +
		"    Continued wording.\n\n" +
		"    Second paragraph with {sup|2} and *italics*.\n"
	got, toc, err := RenderAnnotatedMarkdownXHTMLDocument(input, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`id="Luke.11.2"`, `class="smol-u-solid"`, `class="smol-u-dotted"`,
		`A &amp; B`, `κομᾶν`, `<sup>69</sup>`, `<br`,
		`class="smol-note-paragraph"`, `Second paragraph`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("annotated output lacks %q:\n%s", want, got)
		}
	}
	if len(toc) != 1 || toc[0].Href != "#witness" {
		t.Errorf("unexpected TOC: %#v", toc)
	}
	if !strings.Contains(got, "</span>\n\n<span class=\"smol-note-paragraph\">") {
		t.Errorf("note paragraphs lost their text separator:\n%s", got)
	}
	if err := ValidateHTML(got); err != nil {
		t.Fatalf("rendered fragment failed unchanged validator: %v\n%s", err, got)
	}
	if err := ValidateXHTMLDocument(xhtmlDocument("", got)); err != nil {
		t.Fatalf("rendered document failed unchanged validator: %v\n%s", err, got)
	}
}

func TestAnnotatedSecurityRawHTMLAndEntitiesRemainRejected(t *testing.T) {
	cases := map[string]string{
		"raw span":               `{u-solid|<span>word</span>}`,
		"closing tag":            `{u-dotted|word</p>}`,
		"HTML comment":           `{sup|<!-- hidden -->}`,
		"named entity":           `{u-solid|word&nbsp;word}`,
		"numeric entity":         `{sup|&#x3B1;}`,
		"note HTML":              "Text[^1]\n\n[^1]: <b>word</b>",
		"note continuation HTML": "Text[^1]\n\n[^1]: One.\n    <img src=\"https://example.org/a.png\" />",
		"note entity":            "Text[^1]\n\n[^1]: One.\n\n    Two &amp; three.",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if got, err := RenderAnnotatedMarkdownXHTML(input); err == nil {
				t.Fatalf("accepted prohibited source: %s", got)
			}
		})
	}
}

func TestAnnotatedSecurityClosedInlineGrammar(t *testing.T) {
	cases := map[string]string{
		"unclosed underline":      `{u-solid|word`,
		"empty underline":         `{u-dotted|}`,
		"duplicate style nesting": `{u-solid|one {u-solid|two}}`,
		"nested superscript":      `{sup|one {sup|two}}`,
		"link in superscript":     `{sup|[word](https://example.org)}`,
		"image in superscript":    `{sup|![word](assets/a.png)}`,
		"anchor in superscript":   `{sup|{anchor|inside}}`,
		"note in superscript":     "{sup|[^1]}\n\n[^1]: Note.",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if got, err := RenderAnnotatedMarkdownXHTML(input); err == nil {
				t.Fatalf("accepted invalid annotation: %s", got)
			}
		})
	}
}

func TestAnnotatedSecurityIDsCannotIntroduceAttributesOrCollide(t *testing.T) {
	cases := map[string]string{
		"empty":                  `{anchor|}`,
		"whitespace":             `{anchor|two words}`,
		"quote injection":        `{anchor|x\" onclick=\"run()}`,
		"attribute assignment":   `{anchor|x style=color:red}`,
		"non-ASCII":              `{anchor|λόγος}`,
		"reserved theme top":     `{anchor|top}`,
		"reserved theme content": `{anchor|content}`,
		"reserved TOC container": `{anchor|toc_container}`,
		"reserved TOC title":     `{anchor|toc_title}`,
		"reserved checkbox":      `{anchor|cb123}`,
		"duplicate inline":       `{anchor|same}First {anchor|same}Second`,
		"heading before inline":  "## One {#same}\n\n{anchor|same}Second",
		"inline before heading":  "{anchor|same}First\n\n## Two {#same}",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if got, err := RenderAnnotatedMarkdownXHTML(input); err == nil {
				t.Fatalf("accepted invalid ID: %s", got)
			}
		})
	}
}

func TestAnnotatedSecurityPayloadIsTextNotAttributesOrTemplates(t *testing.T) {
	input := `{u-solid|a " data-test="value & \{\{.Site.Title\}\}}`
	got, err := RenderAnnotatedMarkdownXHTML(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `&#34;`) && !strings.Contains(got, `&quot;`) {
		t.Fatalf("quotes were not escaped as text: %s", got)
	}
	if !strings.Contains(got, `{{.Site.Title}}`) {
		t.Fatalf("template-looking text was executed or lost: %s", got)
	}
	if err := ValidateXHTMLDocument(xhtmlDocument("", got)); err != nil {
		t.Fatalf("text payload altered the document structure: %v", err)
	}
}

func TestAnnotatedSecurityNotesRemainInlineAndNonrecursive(t *testing.T) {
	cases := map[string]string{
		"recursive note":            "Text[^1]\n\n[^1]: Note[^2].\n[^2]: Nested.",
		"note image":                "Text[^1]\n\n[^1]: ![alt](assets/a.png)",
		"note continuation heading": "Text[^1]\n\n[^1]: Note.\n\n    ## Heading",
		"note continuation list":    "Text[^1]\n\n[^1]: Note.\n\n    - list",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if got, err := RenderAnnotatedMarkdownXHTML(input); err == nil {
				t.Fatalf("accepted unsupported note structure: %s", got)
			}
		})
	}
}

func TestAnnotatedSecurityCodeAndLegacyOptInRemainDistinct(t *testing.T) {
	input := "Literal `{anchor|top}` and `{u-solid|<b>&nbsp;</b>}`."
	got, err := RenderAnnotatedMarkdownXHTML(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, `id="top"`) || strings.Contains(got, `class="smol-u-solid"`) {
		t.Fatalf("annotation syntax was interpreted inside code: %s", got)
	}
	legacy, err := RenderMarkdownXHTML("{anchor|Luke.11.2}{u-solid|word} P{sup|69}{br}")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(legacy, `id="Luke.11.2"`) || strings.Contains(legacy, `<sup>69</sup>`) || strings.Contains(legacy, `smol-u-solid`) {
		t.Fatalf("legacy source acquired opt-in annotation rendering: %s", legacy)
	}
}

func TestAnnotatedSecurityOptionalManifestDoesNotChangeValidation(t *testing.T) {
	got, err := RenderAnnotatedMarkdownXHTML(`{anchor|Luke.11.2}{u-dotted|One paragraph.}`)
	if err != nil {
		t.Fatal(err)
	}
	withoutManifest := xhtmlDocument("", got)
	if err := CheckHTML(withoutManifest); err != nil {
		t.Fatalf("optional manifest became mandatory: %v", err)
	}
	if err := ValidateXHTMLDocument(withoutManifest); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(got))
	comment, err := ManifestComment(SiteView{}, Page{}, nil, []ManifestRegion{{
		Name: "authored-content", SHA256: hex.EncodeToString(digest[:]),
	}})
	if err != nil {
		t.Fatal(err)
	}
	withManifest := xhtmlDocument("", "<!--smol:authored-content-->"+got+"<!--/smol:authored-content-->"+comment)
	if err := CheckHTML(withManifest); err != nil {
		t.Fatalf("valid optional manifest failed: %v", err)
	}
	if err := ValidateXHTMLDocument(withManifest); err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(withManifest, "One paragraph.", "Two paragraph.", 1)
	if err := CheckHTML(tampered); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("present manifest failed to detect changed annotation text: %v", err)
	}
}
