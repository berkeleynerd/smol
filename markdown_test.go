package main

import (
	"strings"
	"testing"
)

func TestMarkdownXHTMLEntitiesRemainVerbatim(t *testing.T) {
	got, err := RenderMarkdownXHTML("1. Keep &lt;this&gt; &amp; that.[^26]\n\n[^26]: Note keeps &lt;tag&gt; &amp; value.")
	if err != nil {
		t.Fatalf("RenderMarkdownXHTML: %v", err)
	}
	want := `<p>1. Keep &lt;this&gt; &amp; that.<span><input type="checkbox" id="cb26" /><label for="cb26"><sup>26</sup></label><span>Note keeps &lt;tag&gt; &amp; value.</span></span></p>`
	if got != want {
		t.Fatalf("rendered markdown =\n%s\nwant\n%s", got, want)
	}
}

func TestMarkdownXHTMLLiteralTextIsNotInlineMarkdown(t *testing.T) {
	got, err := RenderMarkdownXHTML("[Sample Label] and [sample text] keep _underscores_ *stars* `ticks`.")
	if err != nil {
		t.Fatalf("RenderMarkdownXHTML: %v", err)
	}
	want := "<p>[Sample Label] and [sample text] keep _underscores_ *stars* `ticks`.</p>"
	if got != want {
		t.Fatalf("rendered markdown = %q, want %q", got, want)
	}
}

func TestMarkdownXHTMLNumberedProseDoesNotBecomeList(t *testing.T) {
	got, err := RenderMarkdownXHTML("1. Numbered prose.\n\n2. More prose.")
	if err != nil {
		t.Fatalf("RenderMarkdownXHTML: %v", err)
	}
	if strings.Contains(got, "<ol>") || strings.Contains(got, "<li>") {
		t.Fatalf("numbered prose became a list: %s", got)
	}
	want := "<p>1. Numbered prose.</p>\n<p>2. More prose.</p>"
	if got != want {
		t.Fatalf("rendered markdown = %q, want %q", got, want)
	}
}

func TestMarkdownXHTMLHeadingID(t *testing.T) {
	got, err := RenderMarkdownXHTML("## Sample Section {#sample-section}")
	if err != nil {
		t.Fatalf("RenderMarkdownXHTML: %v", err)
	}
	want := `<h2 id="sample-section">Sample Section</h2>`
	if got != want {
		t.Fatalf("rendered heading = %q, want %q", got, want)
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
