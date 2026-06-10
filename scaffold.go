package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var slugRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

const (
	starterDefault      = "default"
	starterClassicXHTML = "classic-xhtml"
)

var starterNames = []string{starterDefault, starterClassicXHTML}

func validStarterList() string {
	return strings.Join(starterNames, "|")
}

func isValidStarter(name string) bool {
	for _, starter := range starterNames {
		if name == starter {
			return true
		}
	}
	return false
}

func InitSite(dir string) error {
	return InitSiteWithStarter(dir, starterDefault)
}

func InitSiteWithStarter(dir, starter string) error {
	if starter == "" {
		starter = starterDefault
	}
	switch starter {
	case starterDefault:
		return initDefaultSite(dir)
	case starterClassicXHTML:
		return initClassicXHTMLSite(dir)
	default:
		return fmt.Errorf("unknown starter: %s (valid: %s)", starter, validStarterList())
	}
}

func initDefaultSite(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "smol.json")); err == nil {
		return fmt.Errorf("target directory already contains smol.json")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	dirs := []string{
		filepath.Join(dir, "content", "pages"),
		filepath.Join(dir, "content", "posts"),
		filepath.Join(dir, "themes", "default", "templates"),
		filepath.Join(dir, "themes", "default", "partials"),
		filepath.Join(dir, "themes", "default", "assets"),
		filepath.Join(dir, "public"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	files := map[string]string{
		"smol.json": siteConfigJSON(),
		filepath.Join("content", "pages", "index.html"): contentSource([]string{
			"title: Home",
			"published_utc: 2026-06-07T00:00:00Z",
			"updated_utc: 2026-06-07T00:00:00Z",
		}, "<p>Write your page here.</p>\n"),
		filepath.Join("themes", "default", "theme.json"):                   defaultThemeJSON(),
		filepath.Join("themes", "default", "templates", "index.html.tmpl"): defaultIndexTemplate(),
		filepath.Join("themes", "default", "templates", "page.html.tmpl"):  defaultPageTemplate(),
		filepath.Join("themes", "default", "templates", "post.html.tmpl"):  defaultPostTemplate(),
		filepath.Join("themes", "default", "partials", "head.html.tmpl"):   defaultHeadPartial(),
		filepath.Join("themes", "default", "partials", "footer.html.tmpl"): defaultFooterPartial(),
		filepath.Join("themes", "default", "assets", "style.css"):          defaultCSS(),
		".gitattributes": `public/**/*.html -text
public/**/*.gmi -text
build/**/*.html -text
build/**/*.gmi -text
*.attested.html -text
`,
	}
	for rel, content := range files {
		if err := writeFileExclusive(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func initClassicXHTMLSite(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "smol.json")); err == nil {
		return fmt.Errorf("target directory already contains smol.json")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	dirs := []string{
		filepath.Join(dir, "content", "pages"),
		filepath.Join(dir, "content", "pages", "sample", "assets"),
		filepath.Join(dir, "themes", "classic-xhtml", "templates"),
		filepath.Join(dir, "themes", "classic-xhtml", "assets"),
		filepath.Join(dir, "public"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	files := map[string]string{
		"smol.json": classicXHTMLSiteConfigJSON(),
		filepath.Join("content", "pages", "index.md"):                            classicPageSource("Home", "A small flat XHTML site.", classicIndexMarkdown()),
		filepath.Join("content", "pages", "about.md"):                            classicPageSource("About", "About this starter.", classicAboutMarkdown()),
		filepath.Join("content", "pages", "sample", "index.md"):                  classicPageSource("Sample Page", "A sample Markdown/XHTML page.", classicSampleMarkdown()),
		filepath.Join("content", "pages", "sample", "assets", "sample.png"):      classicSamplePNG(),
		filepath.Join("themes", "classic-xhtml", "theme.json"):                   classicXHTMLThemeJSON(),
		filepath.Join("themes", "classic-xhtml", "templates", "index.html.tmpl"): classicXHTMLPageTemplate(),
		filepath.Join("themes", "classic-xhtml", "templates", "page.html.tmpl"):  classicXHTMLPageTemplate(),
		filepath.Join("themes", "classic-xhtml", "assets", "style.css"):          classicXHTMLCSS(),
		".gitattributes": `public/**/*.html -text
public/**/*.gmi -text
build/**/*.html -text
build/**/*.gmi -text
*.attested.html -text
`,
	}
	for rel, content := range files {
		if err := writeFileExclusive(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func NewContent(siteDir, kind, slug, title string) error {
	site, err := LoadSiteConfig(siteDir)
	if err != nil {
		return err
	}
	if (site.OutputMode == outputModeFlatXHTML || site.OutputMode == outputModeFlatGemini) && kind == "post" {
		return fmt.Errorf("%s supports pages only", site.OutputMode)
	}
	if !slugRE.MatchString(slug) {
		return fmt.Errorf("invalid slug: %s", slug)
	}
	base := "pages"
	if kind == "post" {
		base = "posts"
	}
	ext := ".md"
	bodyContent := "Write your page here.\n"
	if site.OutputMode == outputModeFlatGemini {
		ext = ".gmi"
	}
	path := filepath.Join(siteDir, "content", base, slug+ext)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists: %s", kind, slug)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(filepath.Join(siteDir, "content", base, slug)); err == nil {
		return fmt.Errorf("%s already exists: %s", kind, slug)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	return writeFileExclusive(path, []byte(contentSource([]string{
		"title: " + quoteFrontMatterString(title),
		"published_utc: " + now,
		"updated_utc: " + now,
	}, bodyContent)), 0o644)
}

func writeFileExclusive(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func contentSource(fields []string, body string) string {
	return "---\n" + strings.Join(fields, "\n") + "\n---\n" + body
}

func quoteFrontMatterString(value string) string {
	quoted := fmt.Sprintf("%q", value)
	return quoted
}

func siteConfigJSON() string {
	return `{
  "format": "smol-site-v1",
  "title": "Example Site",
  "description": "Minimal signed pages.",
  "language": "en",
  "base_url": "https://example.org",
  "publisher": "Example Publisher",
  "theme": "default",
  "sign_key": "",
  "nav": [
    {
      "label": "Home",
      "url": "/"
    }
  ]
}
`
}

func defaultThemeJSON() string {
	return `{
  "format": "smol-theme-v1",
  "id": "default",
  "name": "Smol Default",
  "version": "0.1.0",
  "templates": {
    "index": "templates/index.html.tmpl",
    "page": "templates/page.html.tmpl",
    "post": "templates/post.html.tmpl"
  },
  "css": [
    "assets/style.css"
  ]
}
`
}

func defaultCSS() string {
	return `body {
  max-width: 70ch;
  margin: 3rem auto;
  padding: 0 1rem;
  font-family: system-ui, sans-serif;
  line-height: 1.5;
}

img {
  max-width: 100%;
  height: auto;
}

nav a {
  margin-right: 1rem;
}
`
}

func defaultHeadPartial() string {
	return `<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Page.Title}} - {{.Site.Title}}</title>
<meta name="description" content="{{.Page.Summary}}">
<link rel="canonical" href="{{.Page.CanonicalURL}}">
<style>
{{.Smol.InlineCSS}}
</style>
{{.Smol.ManifestComment}}
`
}

func defaultFooterPartial() string {
	return `<footer>
  <p>{{.Site.Publisher}}</p>
</footer>
`
}

func defaultPageTemplate() string {
	return `<!doctype html>
<html lang="{{.Site.Language}}">
<head>
  {{template "head" .}}
</head>
<body{{if .PageNav.HasLinks}} id="top"{{end}}>
  <header>
    <a href="/">{{.Site.Title}}</a>
    <nav aria-label="Main navigation">
      {{range .Nav}}
        <a href="{{.URL}}">{{.Label}}</a>
      {{end}}
    </nav>
  </header>

  <main id="content">
    <h1>{{.Page.Title}}</h1>
    {{.Smol.AuthoredContentOpen}}{{.Page.ContentHTML}}{{.Smol.AuthoredContentClose}}
  </main>
{{if .PageNav.HasLinks}}{{with .PageNav}}
  <nav aria-label="Page navigation">
    {{$.Smol.GeneratedNavigationOpen}}<a href="{{.Top.Href}}" rel="{{.Top.Rel}}">{{.Top.Label}}</a>
{{with .Home}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{with .Up}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{with .Previous}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{with .Next}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{range .Related}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{$.Smol.GeneratedNavigationClose}}
  </nav>
{{end}}{{end}}
  {{template "footer" .}}
</body>
</html>
`
}

func defaultPostTemplate() string {
	return `<!doctype html>
<html lang="{{.Site.Language}}">
<head>
  {{template "head" .}}
</head>
<body{{if .PageNav.HasLinks}} id="top"{{end}}>
  <header>
    <a href="/">{{.Site.Title}}</a>
    <nav aria-label="Main navigation">
      {{range .Nav}}
        <a href="{{.URL}}">{{.Label}}</a>
      {{end}}
    </nav>
  </header>

  <main id="content">
    <article>
      <h1>{{.Page.Title}}</h1>
{{if .Page.PublishedUTC}}      <p><time datetime="{{.Page.PublishedUTC}}">{{date .Page.PublishedUTC "2006-01-02"}}</time></p>
{{end}}      {{.Smol.AuthoredContentOpen}}{{.Page.ContentHTML}}{{.Smol.AuthoredContentClose}}
    </article>
  </main>
{{if .PageNav.HasLinks}}{{with .PageNav}}
  <nav aria-label="Page navigation">
    {{$.Smol.GeneratedNavigationOpen}}<a href="{{.Top.Href}}" rel="{{.Top.Rel}}">{{.Top.Label}}</a>
{{with .Home}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{with .Up}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{with .Previous}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{with .Next}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{range .Related}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{$.Smol.GeneratedNavigationClose}}
  </nav>
{{end}}{{end}}
  {{template "footer" .}}
</body>
</html>
`
}

func defaultIndexTemplate() string {
	return `<!doctype html>
<html lang="{{.Site.Language}}">
<head>
  {{template "head" .}}
</head>
<body{{if .PageNav.HasLinks}} id="top"{{end}}>
  <header>
    <a href="/">{{.Site.Title}}</a>
    <nav aria-label="Main navigation">
      {{range .Nav}}
        <a href="{{.URL}}">{{.Label}}</a>
      {{end}}
    </nav>
  </header>

  <main id="content">
    <h1>{{.Site.Title}}</h1>
    {{.Smol.AuthoredContentOpen}}{{.Page.ContentHTML}}{{.Smol.AuthoredContentClose}}
{{if .Posts}}    <section aria-label="Posts">
    <h2>Posts</h2>
    <ul>
{{range .Posts}}      <li><a href="{{.URL}}">{{.Title}}</a>{{if .PublishedUTC}} <time datetime="{{.PublishedUTC}}">{{date .PublishedUTC "2006-01-02"}}</time>{{end}}</li>
{{end}}    </ul>
  </section>
{{end}}  </main>
{{if .PageNav.HasLinks}}{{with .PageNav}}
  <nav aria-label="Page navigation">
    {{$.Smol.GeneratedNavigationOpen}}<a href="{{.Top.Href}}" rel="{{.Top.Rel}}">{{.Top.Label}}</a>
{{with .Home}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{with .Up}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{with .Previous}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{with .Next}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{range .Related}}    <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{$.Smol.GeneratedNavigationClose}}
  </nav>
{{end}}{{end}}
  {{template "footer" .}}
</body>
</html>
`
}

func classicXHTMLSiteConfigJSON() string {
	return `{
  "format": "smol-site-v1",
  "title": "Classic XHTML",
  "description": "A flat XHTML starter site.",
  "language": "en-US",
  "base_url": "https://example.org",
  "publisher": "Example Publisher",
  "theme": "classic-xhtml",
  "output_mode": "flat-xhtml-v1",
  "sign_key": "",
  "nav": []
}
`
}

func classicPageSource(title, summary, body string) string {
	fields := []string{"title: " + quoteFrontMatterString(title)}
	if summary != "" {
		fields = append(fields, "summary: "+quoteFrontMatterString(summary))
	}
	return contentSource(fields, body)
}

func classicXHTMLThemeJSON() string {
	return `{
  "format": "smol-theme-v1",
  "id": "classic-xhtml",
  "name": "Classic XHTML",
  "version": "0.1.0",
  "templates": {
    "index": "templates/index.html.tmpl",
    "page": "templates/page.html.tmpl"
  },
  "css": [
    "assets/style.css"
  ]
}
`
}

func classicXHTMLPageTemplate() string {
	return `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">
<html
  xmlns:epub="http://www.idpf.org/2007/ops" xmlns="http://www.w3.org/1999/xhtml"
  xml:lang="{{.Site.Language}}" epub="http://www.idpf.org/2007/ops" lang="{{.Site.Language}}">
<head>
<meta charset="utf-8" http-equiv="content-type" content="application/xhtml+xml; charset=UTF-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>{{.Page.Title}}</title>
<style type="text/css">
{{.Smol.InlineCSS}}</style>
</head>
<body{{if .PageNav.HasLinks}} id="top"{{end}}>
<header>
  <h1>{{.Page.Title}}</h1>
{{if .Page.TOC}}  <div id="toc_container">
    <p id="toc_title">Table of Contents</p>
    <ul class="toc_list">
{{range .Page.TOC}}      <li><a href="{{.Href}}">{{.Text}}</a></li>
{{end}}    </ul>
  </div>
{{end}}</header>
<main id="content">{{.Smol.AuthoredContentOpen}}{{.Page.ContentHTML}}{{.Smol.AuthoredContentClose}}</main>
{{if .PageNav.HasLinks}}{{with .PageNav}}<nav aria-label="Page navigation">
  {{$.Smol.GeneratedNavigationOpen}}<a href="{{.Top.Href}}" rel="{{.Top.Rel}}">{{.Top.Label}}</a>
{{with .Home}}  <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{with .Up}}  <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{with .Previous}}  <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{with .Next}}  <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{range .Related}}  <a href="{{.Href}}" rel="{{.Rel}}">{{.Label}}</a>
{{end}}{{$.Smol.GeneratedNavigationClose}}
</nav>
{{end}}{{end -}}
</body>
</html>`
}

func classicIndexMarkdown() string {
	return `This starter demonstrates the flat XHTML publishing path in smol. The pages are written as constrained Markdown and rendered as old-school single-file XHTML.

<ul>
  <li><a href="about.html">About this starter</a></li>
  <li><a href="sample.html">Sample page</a></li>
</ul>

1. Numbered Markdown now renders as an ordered list.
2. Use raw XHTML only when you need exact markup.
`
}

func classicAboutMarkdown() string {
	return `## About this starter

This site uses the classic-xhtml theme, flat filenames, inline CSS, and Markdown bodies. It is meant to show the ordinary publish flow without requiring signing setup.

<blockquote>
  <p>Raw XHTML blocks pass through unchanged, so the author controls exact markup when the narrow Markdown dialect is not enough.</p>
</blockquote>

Footnote rendering exists in the markdown-xhtml-v1 dialect, but this starter stylesheet intentionally leaves out the checkbox note controls.
`
}

func classicSampleMarkdown() string {
	return "Markdown Capability Sample\n" +
		"==========================\n\n" +
		"This page exercises the supported `markdown-xhtml-v1` source dialect. It includes [relative links](about.html), [HTTPS links](https://example.org), *emphasis*, __strong text__, `inline code`, escaped punctuation like \\*literal stars\\*, and a numeric note.[^1]\n\n" +
		"![Embedded sample image](assets/sample.png \"Local embedded image\")\n\n" +
		"## ATX heading with an ID {#sample-section}\n\n" +
		"1. Ordered items preserve sequence in HTML.\n" +
		"2. Ordered items render lossily as numbered text in Gemini.\n\n" +
		"- Unordered item\n" +
		"- Another unordered item\n\n" +
		"- [x] Static task item\n" +
		"- [ ] Another static task item\n\n" +
		"> Blockquotes stay readable in both HTML and Gemini output.\n> They can span more than one line.\n\n" +
		"| Source | HTML output | Gemini output |\n" +
		"| --- | --- | --- |\n" +
		"| Markdown | rich static XHTML | lossy Gemtext |\n" +
		"| Gemtext | constrained XHTML | native capsule text |\n\n" +
		"```text\n" +
		"No scripts.\n" +
		"No remote image references.\n" +
		"Signing remains the final build step.\n" +
		"```\n\n" +
		"---\n\n" +
		"<section>\n" +
		"  <p>Raw XHTML block passthrough remains available for explicit publisher-controlled markup.</p>\n" +
		"</section>\n\n" +
		"[^1]: Footnotes are rendered inline in this constrained dialect.\n"
}

func classicSamplePNG() string {
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=")
	if err != nil {
		panic(err)
	}
	return string(data)
}

func classicXHTMLCSS() string {
	return `a {
  border-bottom: 1px solid #444444;
  color: #444444;
  text-decoration: none;
}
a:hover {
  border-bottom: 0;
}
blockquote {
  font-size: 0.95em;
  font-style: italic;
  margin-left: 4.602%;
  margin-right: 4.602%;
  margin-top: 0.3em;
  text-align: justify
}
body {
  line-height: 1.6;
  font-size: 18px;
  color: #444;
  margin: 0;
  padding: 0;
  max-width: 650px;
  margin: 0px auto;
  padding: 0 10px;
}
html {
  margin: 0;
  padding: 0;
}
code {
  background: #f6f6f6;
  font-family: monospace;
  font-size: 0.95em;
  padding: 0.05em 0.2em;
}
h1 {
  font-size: 1.4em;
  font-weight: normal;
  line-height: 1.2;
  margin-top: 0.571428em;
  margin-left: 0.625%;
  margin-right: 0.625%;
  text-align: center
}
h2 {
  font-size: 1.2em;
  font-weight: normal;
  line-height: 1.2;
  font-style: italic;
  margin-left: 0.156%;
  margin-right: 0.156%;
  margin-top: 1em;
  text-align: center
}
h3 {
  font-size: 1.61803398875em;
  font-weight: normal;
  margin-bottom: 0;
  text-align: center;
  color: #cc9933;
}
h4 {
  font-style: normal;
  font-weight: normal;
  line-height: 1.2;
  font-size: 1.1em;
  margin-top: 1em;
  margin-bottom: 0.1em;
  text-align: center
}
h5 {
  font-style: normal;
  font-weight: normal;
  line-height: 1.2;
  font-size: 0.9em;
  margin-top: 0em;
  margin-bottom: 1.5em;
  text-align: center
}
h6 {
  font-size: 1em;
  font-weight: normal;
  line-height: 0em;
  margin-top: 0;
  margin-bottom: 1em;
  text-align: center;
}
hr {
  border: 0;
  border-top: 1px solid #cccccc;
  margin: 1.5em 0;
}
img {
  display: block;
  height: auto;
  margin: 1em auto;
  max-width: 100%;
}
li {
  margin-top: 0.3em
}
ol {
  list-style-type: decimal;
  margin-top: 1em
}
p {
  margin-left: 0.625%;
  margin-right: 0.625%;
  margin-top: 0.0em;
  text-indent: 3.281%;
  text-align: normal;
}
pre {
  background: #f6f6f6;
  overflow-x: auto;
  padding: 0.75em;
  white-space: pre-wrap;
}
pre code {
  background: transparent;
  padding: 0;
}
table {
  border-collapse: collapse;
  margin: 1em 0;
  width: 100%;
}
td, th {
  border: 1px solid #cccccc;
  padding: 0.25em 0.4em;
  text-align: left;
}
ul {
  margin-top: 1em;
}
ul.task-list {
  list-style-type: none;
  padding-left: 1em;
}
li.task-list-item {
  margin-left: 0;
}
#toc_container {
  font-size: 95%;
  margin-bottom: 1em;
  padding: 20px;
  width: auto;
}
#toc_title {
    font-weight: 700;
    text-align: center;
}
#toc_container li, #toc_container ul, #toc_container ul li {
  list-style: outside none none !important;
}
@media screen and (min-width: 650px) {
  #toc_container ul.toc_list {
    column-count: 2;
    column-gap: 2em;
  }
}
`
}
