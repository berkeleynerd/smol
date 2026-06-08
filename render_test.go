package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBodyHTMLRendersWithImage(t *testing.T) {
	dir := testSite(t)
	if err := NewContent(dir, "post", "hello-world", "Hello World"); err != nil {
		t.Fatalf("NewContent: %v", err)
	}
	postDir := filepath.Join(dir, "content", "posts", "hello-world")
	imageBytes := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	writeBytes(t, filepath.Join(postDir, "assets", "hero.png"), imageBytes)
	writeText(t, filepath.Join(postDir, "body.html"), `{{image "assets/hero.png" "Hero image"}}`)

	siteCfg, err := LoadSiteConfig(dir)
	if err != nil {
		t.Fatalf("LoadSiteConfig: %v", err)
	}
	site, err := siteView(siteCfg)
	if err != nil {
		t.Fatalf("siteView: %v", err)
	}
	_, posts, err := LoadContent(dir, siteCfg)
	if err != nil {
		t.Fatalf("LoadContent: %v", err)
	}
	body, resources, err := renderPageBody(posts[0], site, siteCfg.Nav)
	if err != nil {
		t.Fatalf("renderPageBody: %v", err)
	}
	if !strings.Contains(body, `src="data:image/png;base64,`) {
		t.Fatalf("body did not contain embedded png: %s", body)
	}
	if !strings.Contains(body, `alt="Hero image"`) {
		t.Fatalf("body did not contain alt text: %s", body)
	}
	if len(resources) != 1 || resources[0].Kind != "image" {
		t.Fatalf("image resource not recorded: %#v", resources)
	}
}

func TestImagePathWithDotDotIsRejected(t *testing.T) {
	page := Page{Kind: "post", Slug: "x", contentDir: t.TempDir()}
	if _, _, err := embedImage(page, "../x.png", "Alt"); err == nil {
		t.Fatalf("embedImage accepted path with ..")
	}
}

func TestUnsupportedImageExtensionIsRejected(t *testing.T) {
	dir := t.TempDir()
	writeText(t, filepath.Join(dir, "assets", "hero.svg"), "<svg></svg>")
	page := Page{Kind: "post", Slug: "x", contentDir: dir}
	if _, _, err := embedImage(page, "assets/hero.svg", "Alt"); err == nil {
		t.Fatalf("embedImage accepted unsupported SVG")
	}
}

func TestCSSIsInlined(t *testing.T) {
	dir := testSite(t)
	buildSite(t, dir)
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	if !strings.Contains(html, "<style>\nbody {") {
		t.Fatalf("generated HTML did not inline CSS:\n%s", html)
	}
}

func TestBuildRejectsCSSImport(t *testing.T) {
	dir := testSite(t)
	writeText(t, filepath.Join(dir, "themes", "default", "assets", "style.css"), "@import 'x.css';")
	err := BuildSite(BuildOptions{SiteDir: dir})
	if err == nil {
		t.Fatalf("BuildSite accepted CSS @import")
	}
}

func TestBuildRejectsCSSURL(t *testing.T) {
	dir := testSite(t)
	writeText(t, filepath.Join(dir, "themes", "default", "assets", "style.css"), "body { background: url(x.png); }")
	err := BuildSite(BuildOptions{SiteDir: dir})
	if err == nil {
		t.Fatalf("BuildSite accepted CSS url(")
	}
}

func TestBuildRejectsScriptInBody(t *testing.T) {
	dir := testSite(t)
	writeText(t, filepath.Join(dir, "content", "pages", "index", "body.html"), "<script>alert(1)</script>")
	if err := BuildSite(BuildOptions{SiteDir: dir}); err == nil {
		t.Fatalf("BuildSite accepted script body")
	}
}

func TestBuildRejectsKindMismatch(t *testing.T) {
	dir := testSite(t)
	metaPath := filepath.Join(dir, "content", "pages", "index", "page.json")
	meta := readText(t, metaPath)
	writeText(t, metaPath, strings.Replace(meta, `"kind": "page"`, `"kind": "post"`, 1))
	if err := BuildSite(BuildOptions{SiteDir: dir}); err == nil {
		t.Fatalf("BuildSite accepted kind mismatch")
	}
}

func TestBuildRejectsSlugMismatch(t *testing.T) {
	dir := testSite(t)
	metaPath := filepath.Join(dir, "content", "pages", "index", "page.json")
	meta := readText(t, metaPath)
	writeText(t, metaPath, strings.Replace(meta, `"slug": "index"`, `"slug": "home"`, 1))
	if err := BuildSite(BuildOptions{SiteDir: dir}); err == nil {
		t.Fatalf("BuildSite accepted slug mismatch")
	}
}

func TestGeneratedRoutesMatchSpec(t *testing.T) {
	dir := testSite(t)
	if err := NewContent(dir, "page", "about", "About"); err != nil {
		t.Fatalf("NewContent page: %v", err)
	}
	if err := NewContent(dir, "post", "hello-world", "Hello World"); err != nil {
		t.Fatalf("NewContent post: %v", err)
	}
	buildSite(t, dir)
	for _, rel := range []string{
		filepath.Join("public", "index.html"),
		filepath.Join("public", "about", "index.html"),
		filepath.Join("public", "posts", "hello-world", "index.html"),
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("expected route output %s: %v", rel, err)
		}
	}
}

func TestClassicXHTMLStarterBuildsFlatPages(t *testing.T) {
	dir := t.TempDir()
	if err := InitSiteWithStarter(dir, starterClassicXHTML); err != nil {
		t.Fatalf("InitSiteWithStarter classic-xhtml: %v", err)
	}
	buildSite(t, dir)
	for _, rel := range []string{
		filepath.Join("public", "index.html"),
		filepath.Join("public", "about.html"),
		filepath.Join("public", "sample.html"),
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("expected flat output %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "public", "about", "index.html")); !os.IsNotExist(err) {
		t.Fatalf("classic-xhtml should not generate nested about/index.html: %v", err)
	}
	html := readText(t, filepath.Join(dir, "public", "sample.html"))
	if !strings.Contains(html, `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN"`) {
		t.Fatalf("sample output missing XHTML doctype:\n%s", html)
	}
	if !strings.Contains(html, `<h2 id="sample-section">A sample section</h2>`) {
		t.Fatalf("sample output missing markdown heading id:\n%s", html)
	}
	if !strings.Contains(html, `<p>1. This is a plain paragraph that begins with a number, not an ordered list.</p>`) {
		t.Fatalf("sample output converted numbered prose unexpectedly:\n%s", html)
	}
}

func TestPostSortingIsDeterministic(t *testing.T) {
	dir := testSite(t)
	writePost(t, dir, "b", "B", "2026-06-01T00:00:00Z", false)
	writePost(t, dir, "a", "A", "2026-06-01T00:00:00Z", false)
	writePost(t, dir, "z", "Z", "", false)
	writePost(t, dir, "c", "C", "2026-06-02T00:00:00Z", false)
	siteCfg, err := LoadSiteConfig(dir)
	if err != nil {
		t.Fatalf("LoadSiteConfig: %v", err)
	}
	_, posts, err := LoadContent(dir, siteCfg)
	if err != nil {
		t.Fatalf("LoadContent: %v", err)
	}
	got := []string{posts[0].Slug, posts[1].Slug, posts[2].Slug, posts[3].Slug}
	want := []string{"c", "a", "b", "z"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("post order = %v, want %v", got, want)
		}
	}
}

func TestRepeatedUnsignedBuildsAreByteIdentical(t *testing.T) {
	dir := testSite(t)
	buildSite(t, dir)
	first := readText(t, filepath.Join(dir, "public", "index.html"))
	buildSite(t, dir)
	second := readText(t, filepath.Join(dir, "public", "index.html"))
	if first != second {
		t.Fatalf("repeated unsigned build changed output")
	}
}

func TestBuildSkipsDrafts(t *testing.T) {
	dir := testSite(t)
	writePost(t, dir, "draft-post", "Draft Post", "2026-06-01T00:00:00Z", true)
	buildSite(t, dir)
	if _, err := os.Stat(filepath.Join(dir, "public", "posts", "draft-post", "index.html")); !os.IsNotExist(err) {
		t.Fatalf("draft output exists or unexpected stat error: %v", err)
	}
}

func TestManifestRecordsCSSAndImageHashes(t *testing.T) {
	dir := testSite(t)
	if err := NewContent(dir, "post", "hello-world", "Hello World"); err != nil {
		t.Fatalf("NewContent: %v", err)
	}
	postDir := filepath.Join(dir, "content", "posts", "hello-world")
	imageBytes := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	writeBytes(t, filepath.Join(postDir, "assets", "hero.png"), imageBytes)
	writeText(t, filepath.Join(postDir, "body.html"), `{{image "assets/hero.png" "Hero image"}}`)
	buildSite(t, dir)
	html := readText(t, filepath.Join(dir, "public", "posts", "hello-world", "index.html"))
	manifest := decodeManifestMap(t, html)

	cssResource := resourceWithKind(manifest, "css")
	if cssResource == nil {
		t.Fatalf("manifest missing CSS resource: %#v", manifest["resources"])
	}
	cssBytes, err := os.ReadFile(filepath.Join(dir, "themes", "default", "assets", "style.css"))
	if err != nil {
		t.Fatalf("Read CSS: %v", err)
	}
	cssHash := sha256.Sum256(cssBytes)
	if cssResource["sha256"] != hex.EncodeToString(cssHash[:]) {
		t.Fatalf("CSS sha256 = %v, want %s", cssResource["sha256"], hex.EncodeToString(cssHash[:]))
	}

	imageResource := resourceWithKind(manifest, "image")
	if imageResource == nil {
		t.Fatalf("manifest missing image resource: %#v", manifest["resources"])
	}
	imageHash := sha256.Sum256(imageBytes)
	if imageResource["sha256"] != hex.EncodeToString(imageHash[:]) {
		t.Fatalf("image sha256 = %v, want %s", imageResource["sha256"], hex.EncodeToString(imageHash[:]))
	}
}

func writePost(t *testing.T, dir, slug, title, published string, draft bool) {
	t.Helper()
	postDir := filepath.Join(dir, "content", "posts", slug)
	draftValue := "false"
	if draft {
		draftValue = "true"
	}
	writeText(t, filepath.Join(postDir, "page.json"), `{
  "format": "smol-page-v1",
  "kind": "post",
  "title": "`+title+`",
  "slug": "`+slug+`",
  "summary": "",
  "published_utc": "`+published+`",
  "updated_utc": "`+published+`",
  "tags": [],
  "draft": `+draftValue+`
}
`)
	writeText(t, filepath.Join(postDir, "body.html"), "<p>Post.</p>\n")
}
