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
		links []string
		want  string
	}{
		"unknown target": {
			links: []string{"links:", "  next: missing"},
			want:  `unknown target slug: missing`,
		},
		"external target": {
			links: []string{"links:", "  next: https://example.org"},
			want:  `links.next must be a page slug`,
		},
		"post slug target": {
			setup: func(dir string) {
				writePost(t, dir, "hello-world", "Hello World", "", false)
			},
			links: []string{"links:", "  next: hello-world"},
			want:  `unknown target slug: hello-world`,
		},
		"draft target": {
			setup: func(dir string) {
				writeHTMLPageWithLinks(t, dir, "draft-page", "Draft Page", true, nil)
			},
			links: []string{"links:", "  next: draft-page"},
			want:  `targets draft page: draft-page`,
		},
		"self target": {
			links: []string{"links:", "  next: index"},
			want:  `links.next cannot target itself`,
		},
		"duplicate related target": {
			setup: func(dir string) {
				writeHTMLPageWithLinks(t, dir, "about", "About", false, nil)
			},
			links: []string{"links:", "  related: [about, about]"},
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
	writeHTMLPageWithLinks(t, dir, "about", "About", false, []string{"links:", "  up: index", "  next: sample", "  related: [reference]"})
	writeHTMLPageWithLinks(t, dir, "sample", "Sample", false, nil)
	writeHTMLPageWithLinks(t, dir, "reference", "Reference", false, nil)
	buildSite(t, dir)

	html := readText(t, filepath.Join(dir, "public", "about", "index.html"))
	for _, want := range []string{
		`<body id="top">`,
		authoredContentOpen,
		generatedNavigationOpen,
		`<a href="#top" rel="top">Top</a>`,
		`<a href="../index.html" rel="home">Home</a>`,
		`<a href="../sample/" rel="next">Next: Sample</a>`,
		`<a href="../reference/" rel="related">Related: Reference</a>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("about output missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, `rel="up"`) {
		t.Fatalf("up link to home was not suppressed as redundant:\n%s", html)
	}
	mainClose := strings.Index(html, "</main>")
	navAt := strings.Index(html, generatedNavigationOpen)
	if mainClose < 0 || navAt < 0 || navAt < mainClose {
		t.Fatalf("generated navigation was not rendered after main:\n%s", html)
	}
	body := between(html, authoredContentOpen, authoredContentClose)
	if strings.Contains(body, generatedNavigationOpen) || strings.Contains(body, `Next: Sample`) {
		t.Fatalf("generated navigation leaked into authored content region:\n%s", body)
	}
}

func TestExplicitPageNavigationRendersFlatXHTMLHrefs(t *testing.T) {
	dir := t.TempDir()
	if err := InitSiteWithStarter(dir, starterClassicXHTML); err != nil {
		t.Fatalf("InitSiteWithStarter classic-xhtml: %v", err)
	}
	addFieldsToContentSource(t, filepath.Join(dir, "content", "pages", "sample", "index.md"), []string{"links:", "  up: about", "  previous: index", "  related: [about]"})
	buildSite(t, dir)

	html := readText(t, filepath.Join(dir, "public", "sample.html"))
	for _, want := range []string{
		`<body id="top">`,
		authoredContentOpen,
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

func TestNavigationUsesNavLabelOverTitle(t *testing.T) {
	dir := t.TempDir()
	if err := InitSiteWithStarter(dir, starterClassicXHTML); err != nil {
		t.Fatalf("InitSiteWithStarter classic-xhtml: %v", err)
	}
	addFieldsToContentSource(t, filepath.Join(dir, "content", "pages", "about.md"), []string{`nav_label: "Shortcut"`})
	addFieldsToContentSource(t, filepath.Join(dir, "content", "pages", "sample", "index.md"), []string{"links:", "  up: about", "  previous: index"})
	buildSite(t, dir)

	html := readText(t, filepath.Join(dir, "public", "sample.html"))
	if !strings.Contains(html, `<a href="about.html" rel="up">Up: Shortcut</a>`) {
		t.Fatalf("navigation did not use nav_label:\n%s", html)
	}
	if !strings.Contains(html, `<a href="index.html" rel="previous">Previous: Home</a>`) {
		t.Fatalf("navigation without nav_label did not fall back to title:\n%s", html)
	}
	about := readText(t, filepath.Join(dir, "public", "about.html"))
	if !strings.Contains(about, "<h1>About</h1>") || !strings.Contains(about, "<title>About</title>") {
		t.Fatalf("nav_label leaked into the page's own title chrome:\n%s", about)
	}
}

func writeHTMLPageWithLinks(t *testing.T, dir, slug, title string, draft bool, links []string) {
	t.Helper()
	fields := []string{"title: " + quoteFrontMatterString(title)}
	if draft {
		fields = append(fields, "draft: true")
	}
	fields = append(fields, links...)
	writeContentSource(t, dir, "page", slug, ".html", fields, "<p>Page.</p>\n")
}

func addFieldsToContentSource(t *testing.T, path string, fields []string) {
	t.Helper()
	text := readText(t, path)
	marker := "---\n"
	if !strings.HasPrefix(text, marker) {
		t.Fatalf("%s did not start with front matter", path)
	}
	afterOpen := strings.Index(text[len(marker):], marker)
	if afterOpen < 0 {
		t.Fatalf("%s did not contain closing front matter marker", path)
	}
	insertAt := len(marker) + afterOpen
	writeText(t, path, text[:insertAt]+strings.Join(fields, "\n")+"\n"+text[insertAt:])
}
