package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func xhtmlDocument(head, body string) string {
	return `<!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml" lang="en"><head><meta charset="utf-8" /><title>Test</title>` + head + `</head><body>` + body + `</body></html>`
}

func TestXHTMLRejectsMalformedOrHTMLIncompatibleDocuments(t *testing.T) {
	cases := map[string]string{
		"empty title":                     strings.Replace(xhtmlDocument("", ""), "<title>Test</title>", "<title></title>", 1),
		"empty ID":                        xhtmlDocument("", `<p id="">Text</p>`),
		"invalid boolean value":           xhtmlDocument("", `<details open="false"><summary>More</summary></details>`),
		"missing meta content":            xhtmlDocument(`<meta name="description" />`, ""),
		"heading group without heading":   xhtmlDocument("", `<hgroup><p>Text</p></hgroup>`),
		"description before term":         xhtmlDocument("", `<dl><dd>Text</dd></dl>`),
		"term without description":        xhtmlDocument("", `<dl><dt>Text</dt></dl>`),
		"literal attribute whitespace":    xhtmlDocument("", "<p title=\"A\tB\">Text</p>"),
		"unclosed image":                  xhtmlDocument("", `<img alt="x" src="data:image/png;base64,AA==">`),
		"bare ampersand":                  xhtmlDocument("", `<p>one & two</p>`),
		"HTML-only entity":                xhtmlDocument("", `<p>&nbsp;</p>`),
		"unquoted attribute":              xhtmlDocument("", `<p id=x>one</p>`),
		"boolean shorthand":               xhtmlDocument("", `<details open><summary>More</summary><p>Text</p></details>`),
		"duplicate attributes":            xhtmlDocument("", `<p id="x" id="y">Text</p>`),
		"duplicate namespace":             strings.Replace(xhtmlDocument("", ""), `<html xmlns=`, `<html xmlns="http://www.w3.org/1999/xhtml" xmlns=`, 1),
		"undeclared prefix":               xhtmlDocument("", `<x:p>Text</x:p>`),
		"wrong namespace":                 strings.Replace(xhtmlDocument("", ""), xhtmlNamespace, "urn:wrong", 1),
		"extra root":                      xhtmlDocument("", "") + `<p>Unsigned</p>`,
		"trailing text":                   xhtmlDocument("", "") + "trailing",
		"bad comment":                     xhtmlDocument("", `<!-- invalid -- comment -->`),
		"CDATA end in text":               xhtmlDocument("", `<p>bad ]]></p>`),
		"external DTD":                    strings.Replace(xhtmlDocument("", ""), "<!DOCTYPE html>", `<!DOCTYPE html SYSTEM "https://example.invalid/a.dtd">`, 1),
		"entity declaration":              strings.Replace(xhtmlDocument("", ""), "<!DOCTYPE html>", `<!DOCTYPE html [<!ENTITY x "text">]>`, 1),
		"duplicate doctype":               "<!DOCTYPE html>" + xhtmlDocument("", ""),
		"no doctype":                      strings.TrimPrefix(xhtmlDocument("", ""), "<!DOCTYPE html>"),
		"processing instruction":          `<?xml-stylesheet href="remote.css"?>` + xhtmlDocument("", ""),
		"XML declaration":                 `<?xml version="1.0"?>` + xhtmlDocument("", ""),
		"self-closing div":                xhtmlDocument("", `<div /><p>Text</p>`),
		"paragraph repair":                xhtmlDocument("", `<p><div>Text</div></p>`),
		"invalid phrasing":                xhtmlDocument("", `<span><div>Text</div></span>`),
		"implicit tbody":                  xhtmlDocument("", `<table><tr><td>Text</td></tr></table>`),
		"table foster parenting":          xhtmlDocument("", `<table><p>Text</p></table>`),
		"bad list child":                  xhtmlDocument("", `<ul><p>Text</p></ul>`),
		"uncontained li":                  xhtmlDocument("", `<li>Text</li>`),
		"nested link":                     xhtmlDocument("", `<a href="/a"><a href="/b">Text</a></a>`),
		"duplicate ID":                    xhtmlDocument("", `<p id="x">One</p><p id="x">Two</p>`),
		"misplaced metadata":              xhtmlDocument("", `<meta name="description" content="wrong place" />`),
		"misplaced attribute":             xhtmlDocument("", `<p charset="utf-8">Text</p>`),
		"CDATA in HTML content":           xhtmlDocument("", `<p><![CDATA[Text]]></p>`),
		"different entity interpretation": xhtmlDocument("", `<p>&#128;</p>`),
		"different style interpretation":  xhtmlDocument(`<style>p::before{content:"&amp;"}</style>`, ""),
		"XHTML MIME in HTML meta":         xhtmlDocument(`<meta http-equiv="content-type" content="application/xhtml+xml; charset=UTF-8" />`, ""),
	}
	for name, document := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateXHTMLDocument(document); err == nil {
				t.Fatalf("accepted invalid XHTML5: %s", document)
			}
		})
	}
}

func TestXHTMLAllowsSharedStaticFeatures(t *testing.T) {
	document := xhtmlDocument(`<style>/*<![CDATA[*/p::before{content:"A & B < C"}/*]]>*/</style>`, `
<header><nav><a href="/?a=1&amp;b=2">Home</a></nav></header>
<main><article><h1>Title</h1><p>Text &amp; &#160; &apos; <br />
<img alt="" src="data:image/png;base64,AA==" />
<span><input id="cb1" type="checkbox" /><label for="cb1">Note</label><span>Text</span></span></p>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><title>Icon</title><path d="M0 0h1v1z" /></svg>
<table><thead><tr><th>Heading</th></tr></thead><tbody><tr><td>Value</td></tr></tbody></table>
<details open="open"><summary>More</summary><p>Text</p></details></article></main>`)
	if err := ValidateXHTMLDocument(document + "\n<!--ATTEST EVIDENCE V1\nbase64\nATTEST END-->\n"); err != nil {
		t.Fatal(err)
	}
}

func TestXHTMLRejectsExternalResourceBypasses(t *testing.T) {
	cases := map[string]string{
		"quoted comment style":        `<p style="--a:'/*'; background-image:url('https://example.invalid/image'); --b:'*/'">Text</p>`,
		"entity quoted comment style": `<p style="--a:&quot;/*&quot;; background-image:u&#114;l('https://example.invalid/image'); --b:&quot;*/&quot;">Text</p>`,
		"SVG quoted comment style":    `<svg xmlns="http://www.w3.org/2000/svg"><path style="--a:'/*'; fill:url('https://example.invalid/image'); --b:'*/'" /></svg>`,
		"SVG quoted comment paint":    `<svg xmlns="http://www.w3.org/2000/svg"><path fill="'/*' url('https://example.invalid/image') '*/'" /></svg>`,
		"inline style":                `<p style="background:url(https://example.invalid/a.png)">Text</p>`,
		"entity-obfuscated style":     `<p style="background:u&#114;l(https://example.invalid/a.png)">Text</p>`,
		"CSS escape":                  `<p style="background:\75rl(https://example.invalid/a.png)">Text</p>`,
		"CSS simple escape":           `<p style="background:u\rl(https://example.invalid/a.png)">Text</p>`,
		"image set string":            `<p style="background:image-set('https://example.invalid/a.png' 1x)">Text</p>`,
		"SVG escaped paint":           `<svg xmlns="http://www.w3.org/2000/svg"><path fill="u\72l(https://example.invalid/a.svg)" /></svg>`,
		"SVG image":                   `<svg xmlns="http://www.w3.org/2000/svg"><image href="https://example.invalid/a.png" /></svg>`,
		"SVG image fragment fetch":    `<svg xmlns="http://www.w3.org/2000/svg"><image href="#other-document" /></svg>`,
		"SVG xlink":                   `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><use xlink:href="https://example.invalid/a.svg#x" /></svg>`,
		"SVG data document":           `<img alt="x" src="data:image/svg+xml;base64,AA==" />`,
		"ping":                        `<a href="/" ping="https://example.invalid/track">Home</a>`,
		"entity-obfuscated script":    `<a href="java&#115;cript:alert(1)">Bad</a>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateXHTMLDocument(xhtmlDocument("", body)); err == nil {
				t.Fatalf("accepted external/active content: %s", body)
			}
		})
	}
	for _, css := range []string{`@\69mport 'https://example.invalid/a.css';`, `p{background:i\6d age-set('https://example.invalid/a.png' 1x)}`, `p::before{content:"]]>"}`, `p::before{content:"</style>"}`} {
		if err := ValidateCSS(css); err == nil {
			t.Fatalf("accepted unsafe CSS: %s", css)
		}
	}
}

func TestXHTMLRequiresEarlyUTF8Declaration(t *testing.T) {
	const meta = `<meta charset="utf-8" />`
	base := xhtmlDocument("", "<p>Café 日本語</p>")
	for name, document := range map[string]string{
		"missing":                   strings.Replace(base, meta, "", 1),
		"duplicate":                 strings.Replace(base, meta, meta+meta, 1),
		"duplicate equivalent":      strings.Replace(base, meta, meta+`<meta http-equiv="content-type" content="text/html; charset=UTF-8" />`, 1),
		"wrong charset":             strings.Replace(base, meta, `<meta charset="windows-1252" />`, 1),
		"empty charset":             strings.Replace(base, meta, `<meta charset="" />`, 1),
		"entity charset":            strings.Replace(base, meta, `<meta charset="utf&#45;8" />`, 1),
		"entity equivalent charset": strings.Replace(base, meta, `<meta http-equiv="content-type" content="text/html; charset=utf&#45;8" />`, 1),
		"late":                      strings.Replace(base, meta, "<!--"+strings.Repeat("x", 1024)+"-->"+meta, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateXHTMLDocument(document); err == nil {
				t.Fatal("accepted absent, ambiguous, or unsupported encoding declaration")
			}
		})
	}
	for _, declaration := range []string{meta, `<meta http-equiv="content-type" content="text/html; charset=UTF-8" />`} {
		if err := ValidateXHTMLDocument(strings.Replace(base, meta, declaration, 1)); err != nil {
			t.Fatalf("rejected supported declaration: %v", err)
		}
	}
	// Count bytes, including multibyte text, and require the entire tag to fit.
	for _, end := range []int{1024, 1025} {
		prefix, suffix, _ := strings.Cut(base, meta)
		padding := "<!--é" + strings.Repeat("x", end-len(prefix)-len(meta)-9) + "-->"
		document := prefix + padding + meta + suffix
		if got := strings.Index(document, meta) + len(meta); got != end {
			t.Fatalf("fixture declaration ends at %d, want %d", got, end)
		}
		if err := ValidateXHTMLDocument(document); (err == nil) != (end == 1024) {
			t.Fatalf("declaration ending at %d: %v", end, err)
		}
	}
}

func TestBuildRejectsCSSCommentBypassBeforeSigning(t *testing.T) {
	for _, source := range []string{"theme", "body stylesheet", "body attribute"} {
		t.Run(source, func(t *testing.T) {
			dir := testSite(t)
			buildSite(t, dir)
			output := filepath.Join(dir, "public", "index.html")
			before := readText(t, output)
			marker := filepath.Join(t.TempDir(), "signer-ran")
			tool := filepath.Join(t.TempDir(), "attest")
			writeText(t, tool, "#!/bin/sh\ntouch "+posixSingleQuote(marker)+"\nexit 1\n")
			if err := os.Chmod(tool, 0o755); err != nil {
				t.Fatal(err)
			}
			css := `p::before{content:"/*"} body{background-image:url("https://example.invalid/image")} p::after{content:"*/"}`
			switch source {
			case "theme":
				writeText(t, filepath.Join(dir, "themes", "default", "assets", "style.css"), css)
			case "body stylesheet":
				writeContentSource(t, dir, "page", "index", ".html", []string{"title: Home"}, "<style>"+css+"</style>")
			case "body attribute":
				writeContentSource(t, dir, "page", "index", ".html", []string{"title: Home"}, `<p style="--a:'/*'; background-image:url('https://example.invalid/image'); --b:'*/'">Text</p>`)
			}
			if err := BuildSite(BuildOptions{SiteDir: dir, SignKey: "KEY", AttestPath: tool, Force: true}); err == nil || !strings.Contains(err.Error(), "resource-loading") {
				t.Fatalf("error = %v", err)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("signer ran before validation: %v", err)
			}
			if readText(t, output) != before {
				t.Fatal("failed build replaced output")
			}
		})
	}
}

func TestBuildRejectsMalformedXMLBeforeSigningAndPreservesOutput(t *testing.T) {
	dir := testSite(t)
	buildSite(t, dir)
	output := filepath.Join(dir, "public", "index.html")
	before := readText(t, output)
	marker := filepath.Join(t.TempDir(), "signer-ran")
	tool := filepath.Join(t.TempDir(), "attest")
	writeText(t, tool, "#!/bin/sh\ntouch "+posixSingleQuote(marker)+"\nexit 1\n")
	if err := os.Chmod(tool, 0o755); err != nil {
		t.Fatal(err)
	}
	writeContentSource(t, dir, "page", "index", ".html", []string{"title: Home"}, `<p>bad & text</p>`)
	if err := BuildSite(BuildOptions{SiteDir: dir, SignKey: "KEY", AttestPath: tool, Force: true}); err == nil || !strings.Contains(err.Error(), "well-formed XML") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("signer ran before XML validation: %v", err)
	}
	if readText(t, output) != before {
		t.Fatal("failed build replaced existing output")
	}
}

func TestBuildValidatesFinalSignerOutput(t *testing.T) {
	dir := testSite(t)
	buildSite(t, dir)
	output := filepath.Join(dir, "public", "index.html")
	before := readText(t, output)
	tool := filepath.Join(t.TempDir(), "attest")
	writeText(t, tool, "#!/bin/sh\ncp \"$7\" \"$5\"\nprintf 'unsigned text' >> \"$5\"\n")
	if err := os.Chmod(tool, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := BuildSite(BuildOptions{SiteDir: dir, SignKey: "KEY", AttestPath: tool, Force: true}); err == nil {
		t.Fatal("accepted non-XML signer output")
	}
	if readText(t, output) != before {
		t.Fatal("failed signer replaced existing output")
	}
}

func TestBuildCSSIsXMLSafe(t *testing.T) {
	for _, starter := range starterNames {
		t.Run(starter, func(t *testing.T) {
			dir := t.TempDir()
			if err := InitSiteWithStarter(dir, starter); err != nil {
				t.Fatal(err)
			}
			writeText(t, filepath.Join(dir, "themes", starter, "assets", "style.css"), `p::before{content:"A & B < C"}`)
			buildSite(t, dir)
			if err := CheckPath(CheckOptions{Path: filepath.Join(dir, "public")}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
