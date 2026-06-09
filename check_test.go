package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCheckPathAcceptsGeneratedOutputs(t *testing.T) {
	t.Run("nested HTML", func(t *testing.T) {
		dir := testSite(t)
		buildSite(t, dir)
		var out strings.Builder
		if err := CheckPath(CheckOptions{Path: dir + "/public", Stdout: &out}); err != nil {
			t.Fatalf("CheckPath nested HTML: %v", err)
		}
		if !strings.Contains(out.String(), "ok: ") {
			t.Fatalf("CheckPath did not report checked files: %q", out.String())
		}
	})
	t.Run("flat XHTML", func(t *testing.T) {
		dir := t.TempDir()
		if err := InitSiteWithStarter(dir, starterClassicXHTML); err != nil {
			t.Fatalf("InitSiteWithStarter classic-xhtml: %v", err)
		}
		buildSite(t, dir)
		if err := CheckPath(CheckOptions{Path: dir + "/public", Mode: outputModeFlatXHTML}); err != nil {
			t.Fatalf("CheckPath flat XHTML: %v", err)
		}
	})
	t.Run("Gemini", func(t *testing.T) {
		dir := geminiSite(t, "")
		writeGeminiPage(t, dir, "index", "Home", "Welcome.\n")
		if err := BuildSite(BuildOptions{SiteDir: dir}); err != nil {
			t.Fatalf("BuildSite Gemini: %v", err)
		}
		if err := CheckPath(CheckOptions{Path: dir + "/public", Mode: outputModeFlatGemini}); err != nil {
			t.Fatalf("CheckPath Gemini: %v", err)
		}
	})
}

func TestCheckHTMLModePolicies(t *testing.T) {
	checkbox := `<input type="checkbox" id="cb12" />`
	if err := CheckHTML(checkbox, checkModeDefault); err == nil || !strings.Contains(err.Error(), "interactive HTML") || !strings.Contains(err.Error(), "--mode flat-xhtml-v1") {
		t.Fatalf("CheckHTML default checkbox error = %v, want interactive HTML with flat XHTML hint", err)
	}
	if err := CheckHTML(checkbox, outputModeFlatXHTML); err != nil {
		t.Fatalf("CheckHTML flat XHTML rejected canonical checkbox: %v", err)
	}
	svg := `<svg role="img" viewBox="0 0 1 1"><title>Clean</title><path d="M0 0h1v1z"/></svg>`
	if err := CheckHTML(svg, checkModeDefault); err == nil || !strings.Contains(err.Error(), "inline SVG") || !strings.Contains(err.Error(), "--mode flat-xhtml-v1") {
		t.Fatalf("CheckHTML default SVG error = %v, want inline SVG with flat XHTML hint", err)
	}
	if err := CheckHTML(svg, outputModeFlatXHTML); err != nil {
		t.Fatalf("CheckHTML flat XHTML rejected clean SVG: %v", err)
	}
}

func TestCheckHTMLRejectsUnsafeContent(t *testing.T) {
	tests := map[string]struct {
		html string
		want string
	}{
		"script":              {html: `<script>alert(1)</script>`, want: "JavaScript"},
		"event handler":       {html: `<p onclick="alert(1)">Bad</p>`, want: "event handler"},
		"external resource":   {html: `<link rel="stylesheet" href="https://example.org/x.css">`, want: "external resources"},
		"non embedded image":  {html: `<img src="x.png" alt="x">`, want: "non-embedded"},
		"unsafe inline style": {html: `<style>@import "x.css";</style><p>Bad</p>`, want: "inline style 1"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := CheckHTML(tc.html, checkModeDefault)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CheckHTML error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCheckHTMLRejectsUnsafeStyleVariants(t *testing.T) {
	tests := map[string]struct {
		html string
		want string
	}{
		"closed":            {html: `<style>@import "x.css";</style>`, want: "inline style 1"},
		"whitespace closed": {html: "<style>@import \"x.css\";</style \t>", want: "inline style 1"},
		"newline closed":    {html: "<style>@import \"x.css\";</style\n>", want: "inline style 1"},
		"uppercase":         {html: `<STYLE>@import "x.css";</STYLE>`, want: "inline style 1"},
		"attributes":        {html: `<style type="text/css">@import "x.css";</style>`, want: "inline style 1"},
		"unclosed":          {html: `<style>@import "x.css";`, want: "inline style 1"},
		"multi block":       {html: `<style>p { color: black; }</style><p>Ok.</p><style>body { background: url(x.png); }</style>`, want: "inline style 2"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := CheckHTML(tc.html, checkModeDefault)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CheckHTML error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCheckHTMLValidatesManifestWhenPresent(t *testing.T) {
	validManifest := manifestFixture(t, `"smol-attested-manifest-v1"`, `"`+Profile+`"`)
	if _, ok, err := ExtractManifest("<p>ok</p>" + validManifest); err != nil || !ok {
		t.Fatalf("ExtractManifest valid = %v, %v", ok, err)
	}
	tests := map[string]struct {
		html string
		want string
	}{
		"malformed base64": {
			html: "<p>ok</p>" + manifestCommentStart + "not-base64!" + manifestCommentEnd,
			want: "invalid manifest base64",
		},
		"invalid JSON": {
			html: "<p>ok</p>" + manifestCommentStart + base64.StdEncoding.EncodeToString([]byte("{")) + manifestCommentEnd,
			want: "invalid manifest JSON",
		},
		"wrong format": {
			html: "<p>ok</p>" + manifestFixture(t, `"wrong-format"`, `"`+Profile+`"`),
			want: "format",
		},
		"wrong profile": {
			html: "<p>ok</p>" + manifestFixture(t, `"smol-attested-manifest-v1"`, `"wrong/profile"`),
			want: "profile",
		},
		"crlf wrong profile": {
			html: strings.ReplaceAll("<p>ok</p>"+manifestFixture(t, `"smol-attested-manifest-v1"`, `"wrong/profile"`), "\n", "\r\n"),
			want: "profile",
		},
		"javascript policy true": {
			html: "<p>ok</p>" + manifestFixtureWithPolicy(t, false, `"smol-attested-manifest-v1"`, `"`+Profile+`"`, true, false, true),
			want: "policy.javascript",
		},
		"external resources policy true": {
			html: "<p>ok</p>" + manifestFixtureWithPolicy(t, false, `"smol-attested-manifest-v1"`, `"`+Profile+`"`, false, true, true),
			want: "policy.external_resources",
		},
		"self contained policy false": {
			html: "<p>ok</p>" + manifestFixtureWithPolicy(t, false, `"smol-attested-manifest-v1"`, `"`+Profile+`"`, false, false, false),
			want: "policy.self_contained",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := CheckHTML(tc.html, checkModeDefault)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CheckHTML error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestManifestCommentNilResourcesEmitsEmptyArray(t *testing.T) {
	comment, err := ManifestComment(SiteView{
		Title:           "Site",
		BaseURL:         "https://example.org",
		CanonicalOrigin: "https://example.org",
		Publisher:       "Publisher",
	}, Page{
		Kind:         "page",
		Title:        "Page",
		CanonicalURL: "https://example.org/page",
	}, nil)
	if err != nil {
		t.Fatalf("ManifestComment nil resources: %v", err)
	}
	manifest := decodeManifestMap(t, comment)
	resources, ok := manifest["resources"].([]any)
	if !ok || len(resources) != 0 {
		t.Fatalf("resources = %#v, want empty array", manifest["resources"])
	}
}

func TestDecodeManifestMapPreservesExtraFields(t *testing.T) {
	comment := manifestFixtureWithPolicy(t, true, `"smol-attested-manifest-v1"`, `"`+Profile+`"`, false, false, true)
	manifest := decodeManifestMap(t, comment)
	if manifest["extra_field"] != "preserved" {
		t.Fatalf("extra_field = %#v, want preserved", manifest["extra_field"])
	}
}

func TestCheckPathWalksDirectoriesAndAggregatesFailures(t *testing.T) {
	dir := t.TempDir()
	writeText(t, dir+"/ok.html", "<p>ok</p>")
	writeText(t, dir+"/notes.txt", "<script>ignored</script>")
	writeText(t, dir+"/nested/bad.html", "<script>alert(1)</script>")
	var out strings.Builder
	err := CheckPath(CheckOptions{Path: dir, Stdout: &out})
	if err == nil || !strings.Contains(err.Error(), "1 file(s) failed") {
		t.Fatalf("CheckPath error = %v, want aggregate failure", err)
	}
	got := out.String()
	if !strings.Contains(got, "ok: ") || !strings.Contains(got, "fail: ") || strings.Contains(got, "notes.txt") {
		t.Fatalf("CheckPath output = %q", got)
	}
}

func TestCheckPathRejectsUnsupportedFileAndModeMix(t *testing.T) {
	dir := t.TempDir()
	writeText(t, dir+"/notes.txt", "ignored")
	if err := CheckPath(CheckOptions{Path: dir + "/notes.txt"}); err == nil || !strings.Contains(err.Error(), "unsupported file extension") {
		t.Fatalf("CheckPath unsupported file error = %v", err)
	}
	writeText(t, dir+"/index.html", "<p>ok</p>")
	err := CheckPath(CheckOptions{Path: dir, Mode: outputModeFlatGemini})
	if err == nil || !strings.Contains(err.Error(), "failed check") {
		t.Fatalf("CheckPath flat Gemini mixed directory error = %v, want failed check", err)
	}
}

func TestRunCheckCLIUsageAndExitCodes(t *testing.T) {
	dir := t.TempDir()
	writeText(t, dir+"/ok.html", "<p>ok</p>")
	writeText(t, dir+"/bad.html", "<script>alert(1)</script>")
	writeText(t, dir+"/notes.txt", "not checked")
	tests := map[string]struct {
		args []string
		want int
	}{
		"missing path":          {args: []string{"check"}, want: 2},
		"too many paths":        {args: []string{"check", dir + "/ok.html", dir + "/bad.html"}, want: 2},
		"unknown mode":          {args: []string{"check", "--mode", "loose", dir + "/ok.html"}, want: 2},
		"unsupported extension": {args: []string{"check", dir + "/notes.txt"}, want: 1},
		"valid file":            {args: []string{"check", dir + "/ok.html"}, want: 0},
		"invalid file":          {args: []string{"check", dir + "/bad.html"}, want: 1},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			if got := Run(tc.args, &stdout, &stderr); got != tc.want {
				t.Fatalf("Run(%v) = %d, want %d; stdout=%q stderr=%q", tc.args, got, tc.want, stdout.String(), stderr.String())
			}
		})
	}
}

func manifestFixture(t *testing.T, format, profile string) string {
	return manifestFixtureWithPolicy(t, false, format, profile, false, false, true)
}

func manifestFixtureWithPolicy(t *testing.T, extra bool, format, profile string, javascript, externalResources, selfContained bool) string {
	t.Helper()
	extraField := ""
	if extra {
		extraField = `,"extra_field":"preserved"`
	}
	json := `{"format":` + format + `,"profile":` + profile + `,"generator":{"name":"tool","version":"0"},"publisher":{"name":"Publisher","origin":"https://example.org","url":"https://example.org"},"page":{"kind":"page","title":"Page","canonical_url":"https://example.org/page","published_utc":"","updated_utc":""},"policy":{"javascript":` + boolJSON(javascript) + `,"external_resources":` + boolJSON(externalResources) + `,"self_contained":` + boolJSON(selfContained) + `},"allowed_origins":["https://example.org"],"resources":[]` + extraField + `}`
	return manifestCommentStart + base64.StdEncoding.EncodeToString([]byte(json)) + manifestCommentEnd
}

func boolJSON(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
