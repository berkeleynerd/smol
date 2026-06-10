package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesExpectedStructure(t *testing.T) {
	dir := testSite(t)
	want := []string{
		"smol.json",
		filepath.Join("content", "pages", "index.html"),
		filepath.Join("content", "posts"),
		filepath.Join("themes", "default", "theme.json"),
		filepath.Join("themes", "default", "templates", "index.html.tmpl"),
		filepath.Join("themes", "default", "templates", "page.html.tmpl"),
		filepath.Join("themes", "default", "templates", "post.html.tmpl"),
		filepath.Join("themes", "default", "partials", "head.html.tmpl"),
		filepath.Join("themes", "default", "partials", "footer.html.tmpl"),
		filepath.Join("themes", "default", "assets", "style.css"),
		"public",
		".gitattributes",
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("expected %s: %v", rel, err)
		}
	}
	attrs := readText(t, filepath.Join(dir, ".gitattributes"))
	if !strings.Contains(attrs, "public/**/*.html -text") {
		t.Fatalf(".gitattributes missing public rule: %q", attrs)
	}
	if !strings.Contains(attrs, "public/**/*.gmi -text") {
		t.Fatalf(".gitattributes missing public Gemini rule: %q", attrs)
	}
}

func TestInitClassicXHTMLCreatesExpectedStructure(t *testing.T) {
	dir := t.TempDir()
	if err := InitSiteWithStarter(dir, starterClassicXHTML); err != nil {
		t.Fatalf("InitSiteWithStarter classic-xhtml: %v", err)
	}
	want := []string{
		"smol.json",
		filepath.Join("content", "pages", "index.md"),
		filepath.Join("content", "pages", "about.md"),
		filepath.Join("content", "pages", "sample", "index.md"),
		filepath.Join("content", "pages", "sample", "assets", "sample.png"),
		filepath.Join("themes", "classic-xhtml", "theme.json"),
		filepath.Join("themes", "classic-xhtml", "templates", "index.html.tmpl"),
		filepath.Join("themes", "classic-xhtml", "templates", "page.html.tmpl"),
		filepath.Join("themes", "classic-xhtml", "assets", "style.css"),
		"public",
		".gitattributes",
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("expected %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "content", "pages", "index", "body.html")); !os.IsNotExist(err) {
		t.Fatalf("classic-xhtml should not create body.html for index: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "content", "pages", "kore")); !os.IsNotExist(err) {
		t.Fatalf("classic-xhtml should not create a separate kore page: %v", err)
	}
	site := readText(t, filepath.Join(dir, "smol.json"))
	if !strings.Contains(site, `"output_mode": "flat-xhtml-v1"`) || !strings.Contains(site, `"theme": "classic-xhtml"`) {
		t.Fatalf("classic smol.json missing flat mode/theme:\n%s", site)
	}
	indexMeta, _ := readContentSource(t, filepath.Join(dir, "content", "pages", "index.md"))
	if indexMeta.Title != "Home" {
		t.Fatalf("index title = %q", indexMeta.Title)
	}
	css := readText(t, filepath.Join(dir, "themes", "classic-xhtml", "assets", "style.css"))
	for _, want := range []string{
		"max-width: 650px;",
		"text-indent: 3.281%;",
		"pre code {",
		"table {",
		"ul.task-list {",
		"img {",
		"hr {",
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("classic CSS missing %q:\n%s", want, css)
		}
	}
	if strings.Contains(css, "input[type=checkbox]") || strings.Contains(css, "\nsup {") {
		t.Fatalf("classic CSS retained footnote controls:\n%s", css)
	}
	_, sampleBody := readContentSource(t, filepath.Join(dir, "content", "pages", "sample", "index.md"))
	for _, want := range []string{"Markdown Capability Sample", "![Embedded sample image]", "| Source | HTML output | Gemini output |", "- [x] Static task item", "```text", "<section>"} {
		if !strings.Contains(sampleBody, want) {
			t.Fatalf("sample demo missing %q:\n%s", want, sampleBody)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "content", "pages", "sample", "assets", "sample.png")); err != nil {
		t.Fatalf("sample image missing: %v", err)
	}
}

func TestInitRejectsUnknownStarter(t *testing.T) {
	err := commandInit([]string{"--starter", "unknown", t.TempDir()})
	if err == nil {
		t.Fatalf("commandInit accepted unknown starter")
	}
	if _, ok := err.(usageError); !ok {
		t.Fatalf("unknown starter should be usageError, got %T", err)
	}
	if !strings.Contains(err.Error(), "valid: default|classic-xhtml") {
		t.Fatalf("unknown starter error missing valid list: %v", err)
	}
}

func TestNewPageCreatesExpectedMetadataAndBody(t *testing.T) {
	dir := testSite(t)
	if err := NewContent(dir, "page", "about", "About"); err != nil {
		t.Fatalf("NewContent page: %v", err)
	}
	meta, body := readContentSource(t, filepath.Join(dir, "content", "pages", "about.md"))
	if meta.Title != "About" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	if meta.PublishedUTC == "" || meta.UpdatedUTC == "" {
		t.Fatalf("expected scaffolded timestamps: %#v", meta)
	}
	if body != "Write your page here.\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestNewPageInFlatXHTMLCreatesMarkdownBody(t *testing.T) {
	dir := t.TempDir()
	if err := InitSiteWithStarter(dir, starterClassicXHTML); err != nil {
		t.Fatalf("InitSiteWithStarter classic-xhtml: %v", err)
	}
	if err := NewContent(dir, "page", "notes", "Notes"); err != nil {
		t.Fatalf("NewContent page: %v", err)
	}
	meta, body := readContentSource(t, filepath.Join(dir, "content", "pages", "notes.md"))
	if meta.Title != "Notes" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	if body != "Write your page here.\n" {
		t.Fatalf("body = %q", body)
	}
	if _, err := os.Stat(filepath.Join(dir, "content", "pages", "notes.html")); !os.IsNotExist(err) {
		t.Fatalf("flat-xhtml page should not create notes.html: %v", err)
	}
}

func TestNewPageInFlatGeminiCreatesGemtextBody(t *testing.T) {
	dir := geminiSite(t, "")
	if err := NewContent(dir, "page", "notes", "Notes"); err != nil {
		t.Fatalf("NewContent page: %v", err)
	}
	meta, body := readContentSource(t, filepath.Join(dir, "content", "pages", "notes.gmi"))
	if meta.Title != "Notes" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	if body != "Write your page here.\n" {
		t.Fatalf("body = %q", body)
	}
	if _, err := os.Stat(filepath.Join(dir, "content", "pages", "notes.html")); !os.IsNotExist(err) {
		t.Fatalf("flat-gemini page should not create notes.html: %v", err)
	}
}

func TestNewPostInFlatGeminiIsRejected(t *testing.T) {
	dir := geminiSite(t, "")
	err := NewContent(dir, "post", "hello-world", "Hello World")
	if err == nil {
		t.Fatalf("NewContent post accepted flat-gemini site")
	}
	if !strings.Contains(err.Error(), "flat-gemini-v1 supports pages only") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewPostInFlatXHTMLIsRejected(t *testing.T) {
	dir := t.TempDir()
	if err := InitSiteWithStarter(dir, starterClassicXHTML); err != nil {
		t.Fatalf("InitSiteWithStarter classic-xhtml: %v", err)
	}
	err := NewContent(dir, "post", "hello-world", "Hello World")
	if err == nil {
		t.Fatalf("NewContent post accepted flat-xhtml site")
	}
	if !strings.Contains(err.Error(), "flat-xhtml-v1 supports pages only") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewPostCreatesExpectedMetadataAndBody(t *testing.T) {
	dir := testSite(t)
	if err := NewContent(dir, "post", "hello-world", "Hello World"); err != nil {
		t.Fatalf("NewContent post: %v", err)
	}
	meta, body := readContentSource(t, filepath.Join(dir, "content", "posts", "hello-world.md"))
	if meta.Title != "Hello World" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	if body != "Write your page here.\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestInvalidSlugsAreRejected(t *testing.T) {
	dir := testSite(t)
	for _, slug := range []string{"About", "-about", "about_", "about/world"} {
		if err := NewContent(dir, "page", slug, "About"); err == nil {
			t.Fatalf("NewContent accepted invalid slug %q", slug)
		}
	}
}

func readContentSource(t *testing.T, path string) (ContentMeta, string) {
	t.Helper()
	meta, body, err := parseContentFrontMatter([]byte(readText(t, path)), path)
	if err != nil {
		t.Fatalf("parseContentFrontMatter(%s): %v", path, err)
	}
	return meta, string(body)
}
