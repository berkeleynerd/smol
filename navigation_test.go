package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRelativeHrefForOutputModes(t *testing.T) {
	tests := []struct {
		name        string
		fromRoute   string
		targetRoute string
		outputMode  string
		want        string
	}{
		{
			name:        "nested from index to page",
			fromRoute:   "/",
			targetRoute: "/sample/",
			want:        "sample/",
		},
		{
			name:        "nested from page to index",
			fromRoute:   "/sample/",
			targetRoute: "/",
			want:        "../index.html",
		},
		{
			name:        "flat xhtml page to index",
			fromRoute:   "/sample.html",
			targetRoute: "/",
			outputMode:  outputModeFlatXHTML,
			want:        "index.html",
		},
		{
			name:        "flat xhtml page to page",
			fromRoute:   "/sample.html",
			targetRoute: "/about.html",
			outputMode:  outputModeFlatXHTML,
			want:        "about.html",
		},
		{
			name:        "flat gemini page to index",
			fromRoute:   "/sample.gmi",
			targetRoute: "/",
			outputMode:  outputModeFlatGemini,
			want:        "index.gmi",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := relativeHref(tc.fromRoute, tc.targetRoute, tc.outputMode); got != tc.want {
				t.Fatalf("relativeHref() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPageLinksDefaultEmpty(t *testing.T) {
	dir := testSite(t)
	siteCfg, err := LoadSiteConfig(dir)
	if err != nil {
		t.Fatalf("LoadSiteConfig: %v", err)
	}
	pages, _, err := LoadContent(dir, siteCfg)
	if err != nil {
		t.Fatalf("LoadContent: %v", err)
	}
	if hasPageLinks(pages[0].Links) {
		t.Fatalf("default page links were not empty: %#v", pages[0].Links)
	}
}

func TestLinklessStarterNavigationWhitespaceIsStable(t *testing.T) {
	defaultDir := testSite(t)
	buildSite(t, defaultDir)
	defaultHTML := readText(t, filepath.Join(defaultDir, "public", "index.html"))
	if !strings.Contains(defaultHTML, "</main>\n\n  <footer>") {
		t.Fatalf("default output did not preserve one blank line before footer:\n%s", defaultHTML)
	}
	if strings.Contains(defaultHTML, "</main>\n\n\n  <footer>") {
		t.Fatalf("default output gained extra blank line before footer:\n%s", defaultHTML)
	}

	classicDir := t.TempDir()
	if err := InitSiteWithStarter(classicDir, starterClassicXHTML); err != nil {
		t.Fatalf("InitSiteWithStarter classic-xhtml: %v", err)
	}
	buildSite(t, classicDir)
	classicHTML := readText(t, filepath.Join(classicDir, "public", "sample.html"))
	if !strings.Contains(classicHTML, "</main>\n</body>") {
		t.Fatalf("classic output did not preserve tight main/body boundary:\n%s", classicHTML)
	}
	if strings.Contains(classicHTML, "</main>\n\n</body>") {
		t.Fatalf("classic output gained blank line before body close:\n%s", classicHTML)
	}
}

func TestPageNavigationValidationErrors(t *testing.T) {
	tests := map[string]struct {
		setup func(string)
		links string
		want  string
	}{
		"unknown target": {
			links: `"links": {"next": "missing"}`,
			want:  `unknown target slug: missing`,
		},
		"external target": {
			links: `"links": {"next": "https://example.org"}`,
			want:  `links.next must be a page slug`,
		},
		"post slug target": {
			setup: func(dir string) {
				writePost(t, dir, "hello-world", "Hello World", "", false)
			},
			links: `"links": {"next": "hello-world"}`,
			want:  `unknown target slug: hello-world`,
		},
		"draft target": {
			setup: func(dir string) {
				writeHTMLPageWithLinks(t, dir, "draft-page", "Draft Page", true, "")
			},
			links: `"links": {"next": "draft-page"}`,
			want:  `targets draft page: draft-page`,
		},
		"self target": {
			links: `"links": {"next": "index"}`,
			want:  `links.next cannot target itself`,
		},
		"duplicate related target": {
			setup: func(dir string) {
				writeHTMLPageWithLinks(t, dir, "about", "About", false, "")
			},
			links: `"links": {"related": ["about", "about"]}`,
			want:  `duplicate related link target: about`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := testSite(t)
			if tc.setup != nil {
				tc.setup(dir)
			}
			writeHTMLPageWithLinks(t, dir, "index", "Home", false, tc.links)
			err := BuildSite(BuildOptions{SiteDir: dir})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("BuildSite error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestExplicitPageNavigationRendersOutsideAuthoredContent(t *testing.T) {
	dir := testSite(t)
	writeHTMLPageWithLinks(t, dir, "about", "About", false, `"links": {"up": "index", "next": "sample", "related": ["reference"]}`)
	writeHTMLPageWithLinks(t, dir, "sample", "Sample", false, "")
	writeHTMLPageWithLinks(t, dir, "reference", "Reference", false, "")
	buildSite(t, dir)

	html := readText(t, filepath.Join(dir, "public", "about", "index.html"))
	for _, want := range []string{
		`<body id="top">`,
		`<main id="content" data-smol-authored-content="true">`,
		`<nav data-smol-generated-chrome="navigation" aria-label="Page navigation">`,
		`<a href="#top" rel="top">Top</a>`,
		`<a href="../index.html" rel="home">Home</a>`,
		`<a href="../index.html" rel="up">Up: Home</a>`,
		`<a href="../sample/" rel="next">Next: Sample</a>`,
		`<a href="../reference/" rel="related">Related: Reference</a>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("about output missing %q:\n%s", want, html)
		}
	}
	mainClose := strings.Index(html, "</main>")
	navAt := strings.Index(html, `data-smol-generated-chrome="navigation"`)
	if mainClose < 0 || navAt < 0 || navAt < mainClose {
		t.Fatalf("generated navigation was not rendered after main:\n%s", html)
	}
	body := between(html, `<main id="content" data-smol-authored-content="true">`, "\n  </main>")
	if strings.Contains(body, `data-smol-generated-chrome`) || strings.Contains(body, `Next: Sample`) {
		t.Fatalf("generated navigation leaked into authored content region:\n%s", body)
	}
}

func TestExplicitPageNavigationRendersFlatXHTMLHrefs(t *testing.T) {
	dir := t.TempDir()
	if err := InitSiteWithStarter(dir, starterClassicXHTML); err != nil {
		t.Fatalf("InitSiteWithStarter classic-xhtml: %v", err)
	}
	addLinksToPageJSON(t, filepath.Join(dir, "content", "pages", "sample", "page.json"), `"links": {"up": "about", "previous": "index", "related": ["about"]}`)
	buildSite(t, dir)

	html := readText(t, filepath.Join(dir, "public", "sample.html"))
	for _, want := range []string{
		`<body id="top">`,
		`<main id="content" data-smol-authored-content="true">`,
		`<a href="index.html" rel="home">Home</a>`,
		`<a href="about.html" rel="up">Up: About</a>`,
		`<a href="index.html" rel="previous">Previous: Home</a>`,
		`<a href="about.html" rel="related">Related: About</a>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("sample flat XHTML output missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, `href="/"`) {
		t.Fatalf("flat XHTML navigation used root href:\n%s", html)
	}
}

func writeHTMLPageWithLinks(t *testing.T, dir, slug, title string, draft bool, links string) {
	t.Helper()
	draftValue := "false"
	if draft {
		draftValue = "true"
	}
	linkBlock := ""
	if links != "" {
		linkBlock = ",\n  " + links
	}
	pageDir := filepath.Join(dir, "content", "pages", slug)
	writeText(t, filepath.Join(pageDir, "page.json"), `{
  "format": "smol-page-v1",
  "kind": "page",
  "title": "`+title+`",
  "slug": "`+slug+`",
  "summary": "",
  "published_utc": "",
  "updated_utc": "",
  "tags": [],
  "draft": `+draftValue+linkBlock+`
}
`)
	writeText(t, filepath.Join(pageDir, "body.html"), "<p>Page.</p>\n")
}

func addLinksToPageJSON(t *testing.T, path, links string) {
	t.Helper()
	text := readText(t, path)
	marker := "\n}\n"
	if !strings.HasSuffix(text, marker) {
		t.Fatalf("%s did not end with page.json object marker", path)
	}
	writeText(t, path, strings.TrimSuffix(text, marker)+",\n  "+links+marker)
}
