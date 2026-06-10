package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeminiCapsuleBuildsFlatGMIWithoutTheme(t *testing.T) {
	dir := geminiSite(t, "")
	writeGeminiPage(t, dir, "index", "Capsule", "Welcome.\n\n=> about.gmi About\n")
	writeGeminiPage(t, dir, "about", "About", "About this capsule.\n")

	var out strings.Builder
	if err := BuildSite(BuildOptions{SiteDir: dir, Stdout: &out}); err != nil {
		t.Fatalf("BuildSite: %v", err)
	}
	index := readText(t, filepath.Join(dir, "public", "index.gmi"))
	if index != "# Capsule\n\nWelcome.\n\n=> about.gmi About\n" {
		t.Fatalf("index.gmi = %q", index)
	}
	about := readText(t, filepath.Join(dir, "public", "about.gmi"))
	if about != "# About\n\nAbout this capsule.\n" {
		t.Fatalf("about.gmi = %q", about)
	}
	if _, err := os.Stat(filepath.Join(dir, "public", "index.html")); !os.IsNotExist(err) {
		t.Fatalf("Gemini build should not emit index.html: %v", err)
	}
	for _, want := range []string{"built: public/index.gmi", "built: public/about.gmi", "generated unsigned Gemini capsule"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("stdout missing %q: %s", want, out.String())
		}
	}
}

func TestGeminiCapsuleRejectsSigning(t *testing.T) {
	dir := geminiSite(t, "KEY")
	writeGeminiPage(t, dir, "index", "Capsule", "Welcome.\n")
	err := BuildSite(BuildOptions{SiteDir: dir})
	if err == nil || !strings.Contains(err.Error(), "signing is not supported for flat-gemini-v1") {
		t.Fatalf("expected signing rejection, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "public", "index.gmi")); !os.IsNotExist(statErr) {
		t.Fatalf("output was rendered despite signing rejection or unexpected stat error: %v", statErr)
	}
}

func TestGeminiCapsuleUnsignedSuppressesConfiguredSignKey(t *testing.T) {
	dir := geminiSite(t, "KEY")
	writeGeminiPage(t, dir, "index", "Capsule", "Welcome.\n")
	if err := BuildSite(BuildOptions{SiteDir: dir, Unsigned: true}); err != nil {
		t.Fatalf("BuildSite unsigned: %v", err)
	}
	index := readText(t, filepath.Join(dir, "public", "index.gmi"))
	if index != "# Capsule\n\nWelcome.\n" {
		t.Fatalf("index.gmi = %q", index)
	}
}

func TestGeminiCapsuleBuildsMarkdownBody(t *testing.T) {
	dir := geminiSite(t, "")
	writeMarkdownGeminiPage(t, dir, "index", "Capsule", `## Markdown Section

Paragraph with [docs](https://example.org), *emphasis*, and `+"`code`"+`.

1. First
2. Second
`)
	if err := BuildSite(BuildOptions{SiteDir: dir}); err != nil {
		t.Fatalf("BuildSite: %v", err)
	}
	index := readText(t, filepath.Join(dir, "public", "index.gmi"))
	for _, want := range []string{
		"# Capsule",
		"## Markdown Section",
		"Paragraph with docs, emphasis, and code.",
		"=> https://example.org docs",
		"* 1. First",
	} {
		if !strings.Contains(index, want) {
			t.Fatalf("index.gmi missing %q:\n%s", want, index)
		}
	}
}

func TestRenderGeminiPageFailsClosedWhenBodyIsMissing(t *testing.T) {
	page := Page{Kind: "page", Slug: "index", BodyFormat: bodyFormatGemtext, bodyPath: "index.gmi"}
	_, err := renderGeminiPage(page)
	if err == nil || !strings.Contains(err.Error(), "content body was not loaded") {
		t.Fatalf("renderGeminiPage error = %v, want body-not-loaded error", err)
	}
}

func TestGeminiCapsuleRendersExplicitPageNavigation(t *testing.T) {
	dir := geminiSite(t, "")
	writeGeminiPage(t, dir, "index", "Home", "Welcome.\n")
	writeGeminiPage(t, dir, "essays", "Essays", "Essays.\n")
	writeGeminiPage(t, dir, "kore", "Kore", "Kore.\n")
	writeGeminiPageWithLinks(t, dir, "lament", "Lament", "Lament.\n", []string{
		"links:",
		"  up: essays",
		"  previous: kore",
		"  next: parable-of-old-stone",
		"  related: [index]",
	})
	writeGeminiPage(t, dir, "parable-of-old-stone", "Parable of Old Stone", "Stone.\n")
	if err := BuildSite(BuildOptions{SiteDir: dir}); err != nil {
		t.Fatalf("BuildSite: %v", err)
	}
	lament := readText(t, filepath.Join(dir, "public", "lament.gmi"))
	for _, want := range []string{
		"=> index.gmi Home",
		"=> essays.gmi Up: Essays",
		"=> kore.gmi Previous: Kore",
		"=> parable-of-old-stone.gmi Next: Parable of Old Stone",
		"=> index.gmi Related: Home",
	} {
		if !strings.Contains(lament, want) {
			t.Fatalf("lament.gmi missing %q:\n%s", want, lament)
		}
	}
	if strings.Contains(lament, "#top") || strings.Contains(lament, "=> /") {
		t.Fatalf("Gemini navigation contained HTML top link or root href:\n%s", lament)
	}
}

func TestGeminiCapsuleRejectsHTMLBody(t *testing.T) {
	dir := geminiSite(t, "")
	writeContentSource(t, dir, "page", "index", ".html", []string{"title: Capsule"}, "<p>Welcome.</p>\n")
	err := BuildSite(BuildOptions{SiteDir: dir})
	if err == nil || !strings.Contains(err.Error(), "flat-gemini-v1 supports only markdown-xhtml-v1 or gemtext-v1 bodies") {
		t.Fatalf("expected body format rejection, got %v", err)
	}
}

func TestGeminiCapsuleRejectsBadGemtext(t *testing.T) {
	dir := geminiSite(t, "")
	writeGeminiPage(t, dir, "index", "Capsule", "=>   \n")
	err := BuildSite(BuildOptions{SiteDir: dir})
	if err == nil || !strings.Contains(err.Error(), "invalid gemtext link") {
		t.Fatalf("expected gemtext validation error, got %v", err)
	}
}

func TestGeminiCapsuleBuildFailureDoesNotWritePartialOutput(t *testing.T) {
	dir := geminiSite(t, "")
	writeGeminiPage(t, dir, "aaa", "Good", "This page renders.\n")
	writeGeminiPage(t, dir, "zzz", "Bad", "=>   \n")
	err := BuildSite(BuildOptions{SiteDir: dir})
	if err == nil || !strings.Contains(err.Error(), "invalid gemtext link") {
		t.Fatalf("expected gemtext validation error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "public", "aaa.gmi")); !os.IsNotExist(statErr) {
		t.Fatalf("partial output was written despite later render failure: %v", statErr)
	}
}

func geminiSite(t *testing.T, signKey string) string {
	t.Helper()
	dir := t.TempDir()
	writeText(t, filepath.Join(dir, "smol.json"), `{
  "format": "smol-site-v1",
  "title": "Capsule",
  "description": "Minimal capsule.",
  "language": "en",
  "base_url": "gemini://example.org",
  "publisher": "Example Publisher",
  "output_mode": "flat-gemini-v1",
  "sign_key": "`+signKey+`",
  "nav": []
}
`)
	return dir
}

func writeGeminiPage(t *testing.T, dir, slug, title, body string) {
	t.Helper()
	writeGeminiPageWithLinks(t, dir, slug, title, body, nil)
}

func writeGeminiPageWithLinks(t *testing.T, dir, slug, title, body string, links []string) {
	t.Helper()
	fields := append([]string{"title: " + quoteFrontMatterString(title)}, links...)
	writeContentSource(t, dir, "page", slug, ".gmi", fields, body)
}

func writeMarkdownGeminiPage(t *testing.T, dir, slug, title, body string) {
	t.Helper()
	writeContentSource(t, dir, "page", slug, ".md", []string{"title: " + quoteFrontMatterString(title)}, body)
}
