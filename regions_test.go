package main

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegionHashesUseLiteralByteMarkers(t *testing.T) {
	inner := strings.Repeat("Ⱥ", 200) + " boil 373\u212a end"
	html := "İ prefix " + authoredContentOpen + inner + authoredContentClose
	hashes, err := regionHashes(html)
	if err != nil {
		t.Fatalf("regionHashes: %v", err)
	}
	if got, want := hashes[regionAuthoredContent], sha256Hex(inner); got != want {
		t.Fatalf("authored-content hash = %s, want %s", got, want)
	}
}

func TestRegionDocsContainLiteralMarkers(t *testing.T) {
	for _, path := range []string{"docs/smol-static-nojs-v1.md", "docs/smol-attested-manifest-v1.md"} {
		text := readText(t, path)
		for _, marker := range []string{authoredContentOpen, authoredContentClose, generatedNavigationOpen, generatedNavigationClose} {
			if !strings.Contains(text, marker) {
				t.Fatalf("%s missing literal region marker %q", path, marker)
			}
		}
	}
}

func TestRegionHashesUseFirstExactCloseMarker(t *testing.T) {
	inner := `Real body <!--/smol:authored-content-extra--> tail`
	html := authoredContentOpen + inner + authoredContentClose + " outside"
	hashes, err := regionHashes(html)
	if err != nil {
		t.Fatalf("regionHashes: %v", err)
	}
	if got, want := hashes[regionAuthoredContent], sha256Hex(inner); got != want {
		t.Fatalf("authored-content hash = %s, want %s", got, want)
	}
}

func TestRegionHashesRejectDuplicateLiteralMarkers(t *testing.T) {
	html := authoredContentOpen + "One." + authoredContentClose + authoredContentOpen + "Two." + authoredContentClose
	err := regionHashesErr(html)
	if err == nil || !strings.Contains(err.Error(), `region "authored-content" open marker appears 2 times`) {
		t.Fatalf("regionHashes error = %v, want duplicate authored-content marker", err)
	}
}

func TestCompareRegionHashesReportsDeterministically(t *testing.T) {
	err := compareRegionHashes(
		map[string]string{
			regionGeneratedNavigation: strings.Repeat("b", 64),
			regionAuthoredContent:     strings.Repeat("a", 64),
		},
		map[string]string{
			regionGeneratedNavigation: strings.Repeat("d", 64),
			regionAuthoredContent:     strings.Repeat("c", 64),
		},
	)
	if err == nil || !strings.Contains(err.Error(), `region "authored-content" hash mismatch`) {
		t.Fatalf("compareRegionHashes error = %v, want deterministic authored-content mismatch first", err)
	}
}

func TestStarterAuthoredRegionsAreBodyExact(t *testing.T) {
	dir := testSite(t)
	if err := NewContent(dir, "page", "about", "About"); err != nil {
		t.Fatalf("NewContent page: %v", err)
	}
	if err := NewContent(dir, "post", "hello-world", "Hello World"); err != nil {
		t.Fatalf("NewContent post: %v", err)
	}
	buildSite(t, dir)

	indexHTML := readText(t, filepath.Join(dir, "public", "index.html"))
	assertExactRegion(t, "default index", indexHTML, readText(t, filepath.Join(dir, "content", "pages", "index", "body.html")))
	assertRegionExcludes(t, "default index", indexHTML, "<h1>", "Posts", generatedNavigationOpen)
	if !strings.Contains(indexHTML, `<main id="content">`) {
		t.Fatalf("default index missing semantic main landmark:\n%s", indexHTML)
	}
	if !strings.Contains(indexHTML, `<section aria-label="Posts">`) {
		t.Fatalf("default index did not render generated posts list outside authored region:\n%s", indexHTML)
	}

	pageHTML := readText(t, filepath.Join(dir, "public", "about", "index.html"))
	assertExactRegion(t, "default page", pageHTML, readText(t, filepath.Join(dir, "content", "pages", "about", "body.html")))
	assertRegionExcludes(t, "default page", pageHTML, "<h1>", generatedNavigationOpen)

	postHTML := readText(t, filepath.Join(dir, "public", "posts", "hello-world", "index.html"))
	assertExactRegion(t, "default post", postHTML, readText(t, filepath.Join(dir, "content", "posts", "hello-world", "body.html")))
	assertRegionExcludes(t, "default post", postHTML, "<h1>", "<time", generatedNavigationOpen)
	if !strings.Contains(postHTML, "<main id=\"content\">\n    <article>") {
		t.Fatalf("default post did not restore main/article semantics:\n%s", postHTML)
	}
}

func TestIndexAuthoredRegionHashIgnoresGeneratedPostList(t *testing.T) {
	dir := testSite(t)
	buildSite(t, dir)
	beforeHTML := readText(t, filepath.Join(dir, "public", "index.html"))
	beforeHash := mustRegionHashes(t, beforeHTML)[regionAuthoredContent]

	if err := NewContent(dir, "post", "hello-world", "Hello World"); err != nil {
		t.Fatalf("NewContent post: %v", err)
	}
	buildSite(t, dir)
	afterHTML := readText(t, filepath.Join(dir, "public", "index.html"))
	afterHash := mustRegionHashes(t, afterHTML)[regionAuthoredContent]

	if beforeHash != afterHash {
		t.Fatalf("index authored-content hash changed after adding generated post list: before=%s after=%s", beforeHash, afterHash)
	}
	assertRegionExcludes(t, "default index after post", afterHTML, "Posts", "Hello World")
	if !strings.Contains(afterHTML, "Hello World") {
		t.Fatalf("generated post list did not render outside authored region:\n%s", afterHTML)
	}
}

func TestClassicAuthoredRegionIsRenderedBodyOnly(t *testing.T) {
	dir := t.TempDir()
	if err := InitSiteWithStarter(dir, starterClassicXHTML); err != nil {
		t.Fatalf("InitSiteWithStarter classic-xhtml: %v", err)
	}
	addMainSpacerToPageJSON(t, filepath.Join(dir, "content", "pages", "sample", "page.json"))
	buildSite(t, dir)

	html := readText(t, filepath.Join(dir, "public", "sample.html"))
	siteCfg, err := LoadSiteConfig(dir)
	if err != nil {
		t.Fatalf("LoadSiteConfig: %v", err)
	}
	site, err := siteView(siteCfg)
	if err != nil {
		t.Fatalf("siteView: %v", err)
	}
	pages, _, err := LoadContent(dir, siteCfg)
	if err != nil {
		t.Fatalf("LoadContent: %v", err)
	}
	var sample Page
	for _, page := range pages {
		if page.Slug == "sample" {
			sample = page
			break
		}
	}
	if sample.Slug == "" {
		t.Fatalf("sample page not found")
	}
	rendered, _, err := renderPageBody(sample, site, siteCfg.Nav)
	if err != nil {
		t.Fatalf("renderPageBody sample: %v", err)
	}
	assertExactRegion(t, "classic sample", html, rendered)
	assertRegionExcludes(t, "classic sample", html, "<p>&nbsp;</p>", generatedNavigationOpen)
	if !strings.Contains(html, "<p>&nbsp;</p>") {
		t.Fatalf("classic sample did not render main spacer outside authored region:\n%s", html)
	}
}

func TestNestedManifestIncludesRegionHashes(t *testing.T) {
	dir := testSite(t)
	buildSite(t, dir)
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	m, ok, err := ExtractManifest(html)
	if err != nil || !ok {
		t.Fatalf("ExtractManifest = %v, %v", ok, err)
	}
	if len(m.Regions) != 1 {
		t.Fatalf("manifest regions = %#v, want authored-content only", m.Regions)
	}
	if m.Regions[0].Name != regionAuthoredContent {
		t.Fatalf("manifest region name = %q, want authored-content", m.Regions[0].Name)
	}
	hashes := mustRegionHashes(t, html)
	if err := compareRegionHashes(manifestRegionHashes(m.Regions), hashes); err != nil {
		t.Fatalf("manifest region hashes did not match final HTML: %v", err)
	}
	if _, ok := hashes[regionGeneratedNavigation]; ok {
		t.Fatalf("link-less page recorded generated navigation region: %#v", hashes)
	}
}

func TestNestedManifestIncludesGeneratedNavigationRegionWhenPresent(t *testing.T) {
	dir := testSite(t)
	writeHTMLPageWithLinks(t, dir, "about", "About", false, `"links": {"next": "sample"}`)
	writeHTMLPageWithLinks(t, dir, "sample", "Sample", false, "")
	buildSite(t, dir)
	html := readText(t, filepath.Join(dir, "public", "about", "index.html"))
	m, ok, err := ExtractManifest(html)
	if err != nil || !ok {
		t.Fatalf("ExtractManifest = %v, %v", ok, err)
	}
	hashes := manifestRegionHashes(m.Regions)
	for _, name := range []string{regionAuthoredContent, regionGeneratedNavigation} {
		if hashes[name] == "" {
			t.Fatalf("manifest regions missing %s: %#v", name, m.Regions)
		}
	}
	if err := compareRegionHashes(hashes, mustRegionHashes(t, html)); err != nil {
		t.Fatalf("manifest region hashes did not match final HTML: %v", err)
	}
	navRegion := regionInner(t, html, generatedNavigationOpen, generatedNavigationClose)
	for _, notWant := range []string{`aria-label="Page navigation"`, `aria-label="Main navigation"`, `<nav`} {
		if strings.Contains(navRegion, notWant) {
			t.Fatalf("generated-navigation region included nav chrome %q:\n%s", notWant, navRegion)
		}
	}
	if !strings.Contains(navRegion, `rel="next"`) {
		t.Fatalf("generated-navigation region did not include generated links:\n%s", navRegion)
	}
}

func TestBuildRejectsReservedRegionMarkersInRenderedBody(t *testing.T) {
	for _, marker := range reservedRegionMarkers() {
		t.Run(marker, func(t *testing.T) {
			dir := testSite(t)
			writeText(t, filepath.Join(dir, "content", "pages", "index", "body.html"), "<p>Before.</p>"+markerTemplateAction(marker)+"<p>After.</p>\n")
			err := BuildSite(BuildOptions{SiteDir: dir})
			if err == nil || !strings.Contains(err.Error(), "rendered body contains reserved region marker") || !strings.Contains(err.Error(), marker) || !strings.Contains(err.Error(), `page "index"`) {
				t.Fatalf("BuildSite error = %v, want reserved marker failure with page context", err)
			}
		})
	}
}

func TestFlatXHTMLBuildRejectsReservedRegionMarkersInRenderedBody(t *testing.T) {
	dir := t.TempDir()
	if err := InitSiteWithStarter(dir, starterClassicXHTML); err != nil {
		t.Fatalf("InitSiteWithStarter classic-xhtml: %v", err)
	}
	pageJSON := filepath.Join(dir, "content", "pages", "sample", "page.json")
	meta := readText(t, pageJSON)
	meta = strings.Replace(meta, `"body_format": "markdown-xhtml-v1"`, `"body_format": "html"`, 1)
	writeText(t, pageJSON, meta)
	bodyPath := filepath.Join(dir, "content", "pages", "sample", "body.html")
	writeText(t, bodyPath, "<p>Before.</p>{{.Smol.GeneratedNavigationOpen}}<p>After.</p>\n")
	err := BuildSite(BuildOptions{SiteDir: dir})
	if err == nil || !strings.Contains(err.Error(), "rendered body contains reserved region marker") || !strings.Contains(err.Error(), generatedNavigationOpen) || !strings.Contains(err.Error(), `page "sample"`) {
		t.Fatalf("BuildSite flat XHTML error = %v, want reserved marker failure with page context", err)
	}
}

func TestBuildFailsWhenManifestOutputIsInsideRegion(t *testing.T) {
	dir := testSite(t)
	path := filepath.Join(dir, "themes", "default", "templates", "index.html.tmpl")
	text := readText(t, path)
	text = strings.Replace(text, "{{.Page.ContentHTML}}", "{{.Smol.ManifestComment}}{{.Page.ContentHTML}}", 1)
	writeText(t, path, text)
	err := BuildSite(BuildOptions{SiteDir: dir})
	if err == nil || !strings.Contains(err.Error(), "region content changed between manifest passes") || !strings.Contains(err.Error(), `page "index"`) {
		t.Fatalf("BuildSite error = %v, want region self-verification failure with page context", err)
	}
}

func TestCheckFailsAfterBenignRegionEdit(t *testing.T) {
	dir := testSite(t)
	buildSite(t, dir)
	path := filepath.Join(dir, "public", "index.html")
	html := readText(t, path)
	if !strings.Contains(html, "Write your page here.") {
		t.Fatalf("test fixture missing expected starter body:\n%s", html)
	}
	writeText(t, path, strings.Replace(html, "Write your page here.", "Write your page there.", 1))
	err := CheckPath(CheckOptions{Path: path})
	if err == nil || !strings.Contains(err.Error(), `region "authored-content" hash mismatch`) {
		t.Fatalf("CheckPath error = %v, want authored-content hash mismatch", err)
	}
}

func TestFlatOutputsDoNotCarryManifestRegions(t *testing.T) {
	classicDir := t.TempDir()
	if err := InitSiteWithStarter(classicDir, starterClassicXHTML); err != nil {
		t.Fatalf("InitSiteWithStarter classic-xhtml: %v", err)
	}
	buildSite(t, classicDir)
	classicHTML := readText(t, filepath.Join(classicDir, "public", "sample.html"))
	if containsManifest(classicHTML) || strings.Contains(classicHTML, `"regions"`) {
		t.Fatalf("flat XHTML output carried manifest regions:\n%s", classicHTML)
	}

	geminiDir := geminiSite(t, "")
	writeGeminiPage(t, geminiDir, "index", "Home", "Welcome.\n")
	if err := BuildSite(BuildOptions{SiteDir: geminiDir}); err != nil {
		t.Fatalf("BuildSite Gemini: %v", err)
	}
	gemini := readText(t, filepath.Join(geminiDir, "public", "index.gmi"))
	if strings.Contains(gemini, "SMOL ATTESTED MANIFEST V1") || strings.Contains(gemini, "regions") {
		t.Fatalf("flat Gemini output carried manifest regions:\n%s", gemini)
	}
}

func assertExactRegion(t *testing.T, label, html, want string) {
	t.Helper()
	got := regionInner(t, html, authoredContentOpen, authoredContentClose)
	if got != want {
		t.Fatalf("%s authored region mismatch:\n--- got ---\n%q\n--- want ---\n%q", label, got, want)
	}
}

func assertRegionExcludes(t *testing.T, label, html string, values ...string) {
	t.Helper()
	region := regionInner(t, html, authoredContentOpen, authoredContentClose)
	for _, value := range values {
		if strings.Contains(region, value) {
			t.Fatalf("%s authored region unexpectedly contained %q:\n%s", label, value, region)
		}
	}
}

func regionInner(t *testing.T, html, open, close string) string {
	t.Helper()
	start := strings.Index(html, open)
	if start < 0 {
		t.Fatalf("missing region open marker %q:\n%s", open, html)
	}
	start += len(open)
	end := strings.Index(html[start:], close)
	if end < 0 {
		t.Fatalf("missing region close marker %q:\n%s", close, html)
	}
	return html[start : start+end]
}

func mustRegionHashes(t *testing.T, html string) map[string]string {
	t.Helper()
	hashes, err := regionHashes(html)
	if err != nil {
		t.Fatalf("regionHashes: %v", err)
	}
	return hashes
}

func regionHashesErr(html string) error {
	_, err := regionHashes(html)
	return err
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func addMainSpacerToPageJSON(t *testing.T, path string) {
	t.Helper()
	text := readText(t, path)
	marker := "\n}"
	if !strings.Contains(text, marker) {
		t.Fatalf("page JSON missing closing marker:\n%s", text)
	}
	writeText(t, path, strings.Replace(text, marker, ",\n  \"main_spacer\": true\n}", 1))
}

func markerTemplateAction(marker string) string {
	switch marker {
	case authoredContentOpen:
		return "{{.Smol.AuthoredContentOpen}}"
	case authoredContentClose:
		return "{{.Smol.AuthoredContentClose}}"
	case generatedNavigationOpen:
		return "{{.Smol.GeneratedNavigationOpen}}"
	case generatedNavigationClose:
		return "{{.Smol.GeneratedNavigationClose}}"
	default:
		return marker
	}
}
