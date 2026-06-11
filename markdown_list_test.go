package main

import (
	"strings"
	"testing"
)

func renderMarkdownDoc(t *testing.T, input string) string {
	t.Helper()
	html, _, err := RenderMarkdownXHTMLDocument(input, nil, false)
	if err != nil {
		t.Fatalf("RenderMarkdownXHTMLDocument(%q): %v", input, err)
	}
	return html
}

func TestLooseOrderedListIsOneListNumberedSequentially(t *testing.T) {
	html := renderMarkdownDoc(t, "1. First section.\n\n2. Second section.\n\n3. Third section.\n")
	if n := strings.Count(html, "<ol>"); n != 1 {
		t.Fatalf("expected a single <ol> (loose items stay one list), got %d:\n%s", n, html)
	}
	if n := strings.Count(html, "<li>"); n != 3 {
		t.Fatalf("expected 3 <li>, got %d:\n%s", n, html)
	}
	for _, want := range []string{"<p>First section.</p>", "<p>Second section.</p>", "<p>Third section.</p>"} {
		if !strings.Contains(html, want) {
			t.Fatalf("loose item missing %q:\n%s", want, html)
		}
	}
}

func TestLooseListMultiParagraphItem(t *testing.T) {
	// Item 2 has an indented continuation paragraph; it must remain inside the
	// item as a second <p>, not break the list.
	html := renderMarkdownDoc(t, "1. One.\n\n2. Two.\n\n   Still two.\n\n3. Three.\n")
	if n := strings.Count(html, "<ol>"); n != 1 {
		t.Fatalf("expected one <ol>, got %d:\n%s", n, html)
	}
	if n := strings.Count(html, "<li>"); n != 3 {
		t.Fatalf("expected 3 items, got %d:\n%s", n, html)
	}
	if !strings.Contains(html, "<p>Two.</p>") || !strings.Contains(html, "<p>Still two.</p>") {
		t.Fatalf("continuation paragraph not folded into the item:\n%s", html)
	}
}

func TestTightListRenderingUnchanged(t *testing.T) {
	if html := renderMarkdownDoc(t, "1. a\n2. b\n3. c\n"); !strings.Contains(html, "<ol>\n<li>a</li>\n<li>b</li>\n<li>c</li>\n</ol>") {
		t.Fatalf("tight ordered list rendering changed:\n%s", html)
	}
	if html := renderMarkdownDoc(t, "- Alpha\n- Beta\n"); !strings.Contains(html, "<ul>\n<li>Alpha</li>\n<li>Beta</li>\n</ul>") {
		t.Fatalf("tight unordered list rendering changed:\n%s", html)
	}
}

func TestOrderedListPreservesStartNumber(t *testing.T) {
	// A list whose first item is not 1 (a numbered section resuming after a
	// heading) must emit <ol start="N"> so the numbering continues.
	if html := renderMarkdownDoc(t, "7. Seven.\n8. Eight.\n"); !strings.Contains(html, `<ol start="7">`) {
		t.Fatalf("expected <ol start=\"7\">:\n%s", html)
	}
	if html := renderMarkdownDoc(t, "3. Three.\n\n4. Four.\n"); !strings.Contains(html, `<ol start="3">`) {
		t.Fatalf("expected loose <ol start=\"3\">:\n%s", html)
	}
	if html := renderMarkdownDoc(t, "1. One.\n2. Two.\n"); strings.Contains(html, "start=") {
		t.Fatalf("a 1-based list must stay bare <ol>:\n%s", html)
	}
}

func TestLooseListItemFootnote(t *testing.T) {
	html := renderMarkdownDoc(t, "1. Alpha[^1]\n\n2. Beta\n\n[^1]: a footnote body\n")
	if n := strings.Count(html, "<ol>"); n != 1 {
		t.Fatalf("expected one <ol>, got %d:\n%s", n, html)
	}
	if !strings.Contains(html, "Alpha") || !strings.Contains(html, "a footnote body") {
		t.Fatalf("footnote inside a loose item not rendered:\n%s", html)
	}
}
