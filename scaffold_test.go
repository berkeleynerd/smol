package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitCreatesExpectedStructure(t *testing.T) {
	dir := testSite(t)
	want := []string{
		"smol.json",
		filepath.Join("content", "pages", "index", "page.json"),
		filepath.Join("content", "pages", "index", "body.html"),
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
		filepath.Join("content", "pages", "index", "page.json"),
		filepath.Join("content", "pages", "index", "body.md"),
		filepath.Join("content", "pages", "about", "page.json"),
		filepath.Join("content", "pages", "about", "body.md"),
		filepath.Join("content", "pages", "sample", "page.json"),
		filepath.Join("content", "pages", "sample", "body.md"),
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
	site := readText(t, filepath.Join(dir, "smol.json"))
	if !strings.Contains(site, `"output_mode": "flat-xhtml-v1"`) || !strings.Contains(site, `"theme": "classic-xhtml"`) {
		t.Fatalf("classic smol.json missing flat mode/theme:\n%s", site)
	}
	indexMeta := readContentMeta(t, filepath.Join(dir, "content", "pages", "index", "page.json"))
	if indexMeta.BodyFormat != bodyFormatMarkdownXHTML {
		t.Fatalf("index body_format = %q", indexMeta.BodyFormat)
	}
	css := readText(t, filepath.Join(dir, "themes", "classic-xhtml", "assets", "style.css"))
	if !strings.Contains(css, "max-width: 650px;") || !strings.Contains(css, "text-indent: 3.281%;") {
		t.Fatalf("classic CSS missing expected typography:\n%s", css)
	}
	if strings.Contains(css, "input[type=checkbox]") || strings.Contains(css, "\nsup {") {
		t.Fatalf("classic CSS retained footnote controls:\n%s", css)
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
	meta := readContentMeta(t, filepath.Join(dir, "content", "pages", "about", "page.json"))
	if meta.Kind != "page" || meta.Slug != "about" || meta.Title != "About" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	if meta.PublishedUTC == "" || meta.UpdatedUTC == "" {
		t.Fatalf("expected scaffolded timestamps: %#v", meta)
	}
	body := readText(t, filepath.Join(dir, "content", "pages", "about", "body.html"))
	if body != "<p>Write your page here.</p>\n" {
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
	meta := readContentMeta(t, filepath.Join(dir, "content", "pages", "notes", "page.json"))
	if meta.Kind != "page" || meta.Slug != "notes" || meta.BodyFormat != bodyFormatMarkdownXHTML {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	body := readText(t, filepath.Join(dir, "content", "pages", "notes", "body.md"))
	if body != "Write your page here.\n" {
		t.Fatalf("body = %q", body)
	}
	if _, err := os.Stat(filepath.Join(dir, "content", "pages", "notes", "body.html")); !os.IsNotExist(err) {
		t.Fatalf("flat-xhtml page should not create body.html: %v", err)
	}
}

func TestNewPageInFlatGeminiCreatesGemtextBody(t *testing.T) {
	dir := geminiSite(t, "")
	if err := NewContent(dir, "page", "notes", "Notes"); err != nil {
		t.Fatalf("NewContent page: %v", err)
	}
	meta := readContentMeta(t, filepath.Join(dir, "content", "pages", "notes", "page.json"))
	if meta.Kind != "page" || meta.Slug != "notes" || meta.BodyFormat != bodyFormatGemtext {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	body := readText(t, filepath.Join(dir, "content", "pages", "notes", "body.gmi"))
	if body != "Write your page here.\n" {
		t.Fatalf("body = %q", body)
	}
	if _, err := os.Stat(filepath.Join(dir, "content", "pages", "notes", "body.html")); !os.IsNotExist(err) {
		t.Fatalf("flat-gemini page should not create body.html: %v", err)
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
	meta := readContentMeta(t, filepath.Join(dir, "content", "posts", "hello-world", "page.json"))
	if meta.Kind != "post" || meta.Slug != "hello-world" || meta.Title != "Hello World" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	body := readText(t, filepath.Join(dir, "content", "posts", "hello-world", "body.html"))
	if body != "<p>Write your page here.</p>\n" {
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

func readContentMeta(t *testing.T, path string) ContentMeta {
	t.Helper()
	var meta ContentMeta
	if err := json.Unmarshal([]byte(readText(t, path)), &meta); err != nil {
		t.Fatalf("Unmarshal(%s): %v", path, err)
	}
	return meta
}
