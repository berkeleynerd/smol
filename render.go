package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type BuildOptions struct {
	SiteDir             string
	OutDir              string
	SignKey             string
	SignKeySource       bool
	AttestPath          string
	Unsigned            bool
	Force               bool
	Stdout              io.Writer
	attestExecutableDir string
	attestLookPath      func(string) (string, error)
}

type SmolView struct {
	Version                  string
	Profile                  string
	InlineCSS                template.CSS
	ManifestComment          template.HTML
	AuthoredContentOpen      template.HTML
	AuthoredContentClose     template.HTML
	GeneratedNavigationOpen  template.HTML
	GeneratedNavigationClose template.HTML
}

type TemplateRoot struct {
	Site    SiteView
	Page    Page
	Pages   []Page
	Posts   []Page
	Nav     []NavItem
	PageNav PageNavigation
	Smol    SmolView
}

type renderResult struct {
	page      Page
	route     string
	tempPath  string
	finalPath string
}

func BuildSite(opts BuildOptions) error {
	if opts.SiteDir == "" {
		opts.SiteDir = "."
	}
	if opts.OutDir == "" {
		opts.OutDir = "public"
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	siteDir, err := filepath.Abs(opts.SiteDir)
	if err != nil {
		return err
	}
	siteCfg, err := LoadSiteConfig(siteDir)
	if err != nil {
		return err
	}
	if opts.Unsigned {
		opts.SignKey = ""
	} else if opts.SignKey == "" && !opts.SignKeySource {
		opts.SignKey = siteCfg.SignKey
	}
	outDir := resolveOutputDir(siteDir, opts.OutDir)
	if opts.Force {
		if err := validateForceOutputDir(siteDir, outDir); err != nil {
			return err
		}
	}
	site, err := siteView(siteCfg)
	if err != nil {
		return err
	}
	if siteCfg.OutputMode == outputModeFlatGemini {
		return buildGeminiCapsule(siteDir, siteCfg, opts)
	}
	themeDir := filepath.Join(siteDir, "themes", siteCfg.Theme)
	themeCfg, err := LoadThemeConfigForMode(themeDir, siteCfg.OutputMode)
	if err != nil {
		return err
	}
	pages, posts, err := LoadContent(siteDir, siteCfg)
	if err != nil {
		return err
	}
	pages, posts, err = resolvePageNavigations(pages, posts, siteCfg.OutputMode)
	if err != nil {
		return err
	}
	pages = nonDraft(pages)
	posts = nonDraft(posts)
	css, cssResources, err := loadThemeCSS(themeDir, themeCfg)
	if err != nil {
		return err
	}
	if err := ValidateCSS(css); err != nil {
		return err
	}

	tempDir, err := os.MkdirTemp(siteDir, ".smol-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	attestTool := ""
	if opts.SignKey != "" {
		attestTool, err = ResolveAttestTool(AttestResolverOptions{
			ExplicitPath:  opts.AttestPath,
			ExecutableDir: opts.attestExecutableDir,
			TempDir:       tempDir,
			Stdout:        opts.Stdout,
			LookPath:      opts.attestLookPath,
		})
		if err != nil {
			return err
		}
	}

	results := []renderResult{}
	for _, page := range append(append([]Page{}, pages...), posts...) {
		rendered, imageResources, tocEntries, err := renderPageBody(page, site, siteCfg.Nav)
		if err != nil {
			return fmt.Errorf("%s %q: %w", page.Kind, page.Slug, err)
		}
		if err := validateNoReservedRegionMarkers(page, rendered); err != nil {
			return err
		}
		if err := ValidateHTMLForMode(rendered, siteCfg.OutputMode); err != nil {
			return err
		}
		page.ContentHTML = template.HTML(rendered)
		page.TOC = tocEntries
		resources := append([]ManifestResource{}, cssResources...)
		resources = append(resources, imageResources...)
		root := TemplateRoot{
			Site:    site,
			Page:    page,
			Pages:   pages,
			Posts:   posts,
			Nav:     siteCfg.Nav,
			PageNav: page.Navigation,
			Smol: SmolView{
				Version:                  Version,
				Profile:                  Profile,
				InlineCSS:                template.CSS(css),
				AuthoredContentOpen:      template.HTML(authoredContentOpen),
				AuthoredContentClose:     template.HTML(authoredContentClose),
				GeneratedNavigationOpen:  template.HTML(generatedNavigationOpen),
				GeneratedNavigationClose: template.HTML(generatedNavigationClose),
			},
		}
		var html string
		if siteCfg.OutputMode == outputModeFlatXHTML {
			html, err = renderFullPage(themeDir, themeCfg, page.Kind, page.Slug, root)
		} else {
			html, err = renderNestedPage(themeDir, themeCfg, site, page, resources, root)
		}
		if err != nil {
			return err
		}
		if err := ValidateHTMLForMode(html, siteCfg.OutputMode); err != nil {
			return err
		}
		tempPath := outputPathFor(tempDir, page.URL)
		if err := atomicWriteFile(tempPath, []byte(html), 0o644); err != nil {
			return err
		}
		results = append(results, renderResult{
			page:      page,
			route:     page.URL,
			tempPath:  tempPath,
			finalPath: outputPathFor(outDir, page.URL),
		})
	}

	if opts.Force {
		if err := os.RemoveAll(outDir); err != nil {
			return err
		}
	}
	for _, result := range results {
		if opts.SignKey == "" {
			data, err := os.ReadFile(result.tempPath)
			if err != nil {
				return err
			}
			if err := atomicWriteFile(result.finalPath, data, 0o644); err != nil {
				return err
			}
			if err := writeGzipSibling(result.finalPath); err != nil {
				return err
			}
			fmt.Fprintf(opts.Stdout, "built: %s\n", cleanSlash(relativeDisplay(siteDir, result.finalPath)))
			continue
		}
		if err := os.MkdirAll(filepath.Dir(result.finalPath), 0o755); err != nil {
			return err
		}
		if err := SignHTML(attestTool, opts.SignKey, result.tempPath, result.finalPath); err != nil {
			return err
		}
		if err := writeGzipSibling(result.finalPath); err != nil {
			return err
		}
		fmt.Fprintf(opts.Stdout, "signed: %s\n", cleanSlash(relativeDisplay(siteDir, result.finalPath)))
	}
	if opts.SignKey == "" {
		fmt.Fprintln(opts.Stdout, "note: generated unsigned HTML; run with --sign or --sign-key to create attested pages")
	}
	return nil
}

// writeGzipSibling writes a deterministic best-compression gzip of the final
// page bytes next to it, so static servers can serve precompressed content
// that decompresses to exactly the attested file.
func writeGzipSibling(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return err
	}
	if _, err := zw.Write(data); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return atomicWriteFile(path+".gz", buf.Bytes(), 0o644)
}

func renderNestedPage(themeDir string, themeCfg ThemeConfig, site SiteView, page Page, resources []ManifestResource, root TemplateRoot) (string, error) {
	comment, err := ManifestComment(site, page, resources, nil)
	if err != nil {
		return "", err
	}
	root.Smol.ManifestComment = template.HTML(comment)
	passOneHTML, err := renderFullPage(themeDir, themeCfg, page.Kind, page.Slug, root)
	if err != nil {
		return "", err
	}
	hashes, err := regionHashes(passOneHTML)
	if err != nil {
		return "", fmt.Errorf("%s %q region hashing: %w", page.Kind, page.Slug, err)
	}
	comment, err = ManifestComment(site, page, resources, manifestRegionsFromHashes(hashes))
	if err != nil {
		return "", err
	}
	root.Smol.ManifestComment = template.HTML(comment)
	html, err := renderFullPage(themeDir, themeCfg, page.Kind, page.Slug, root)
	if err != nil {
		return "", err
	}
	finalHashes, err := regionHashes(html)
	if err != nil {
		return "", fmt.Errorf("%s %q final region hashing: %w", page.Kind, page.Slug, err)
	}
	if err := compareRegionHashes(hashes, finalHashes); err != nil {
		return "", fmt.Errorf("%s %q region content changed between manifest passes: %w; ensure manifest output is outside marked regions", page.Kind, page.Slug, err)
	}
	return html, nil
}

func resolveOutputDir(siteDir, outDir string) string {
	if filepath.IsAbs(outDir) {
		return filepath.Clean(outDir)
	}
	return filepath.Clean(filepath.Join(siteDir, outDir))
}

func validateForceOutputDir(siteDir, outDir string) error {
	siteDir = filepath.Clean(siteDir)
	outDir = filepath.Clean(outDir)
	if pathWithinOrSame(outDir, siteDir) {
		return fmt.Errorf("refusing to remove output directory that contains site directory: %s", outDir)
	}
	for _, sourceDir := range []string{
		filepath.Join(siteDir, "content"),
		filepath.Join(siteDir, "themes"),
		filepath.Join(siteDir, ".git"),
	} {
		if pathWithinOrSame(sourceDir, outDir) {
			return fmt.Errorf("refusing to remove source directory as output: %s", outDir)
		}
	}
	return nil
}

func pathWithinOrSame(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func renderPageBody(page Page, site SiteView, nav []NavItem) (string, []ManifestResource, []TOCEntry, error) {
	body := page.body
	if body == nil {
		return "", nil, nil, fmt.Errorf("content body was not loaded for %s %q", page.Kind, page.Slug)
	}
	if page.BodyFormat == bodyFormatMarkdownXHTML {
		var resources []ManifestResource
		html, toc, err := RenderMarkdownXHTMLDocument(string(body), func(path, alt, title string) (string, error) {
			html, resource, err := embedImageWithTitle(page, path, alt, title)
			if err != nil {
				return "", err
			}
			resources = append(resources, resource)
			return string(html), nil
		}, page.tocRequested)
		if err != nil {
			return "", nil, nil, err
		}
		return html, resources, toc, nil
	}
	if page.BodyFormat == bodyFormatGemtext {
		html, err := RenderGemtextXHTML(string(body))
		if err != nil {
			return "", nil, nil, err
		}
		return html, nil, nil, nil
	}
	var resources []ManifestResource
	funcs := template.FuncMap{
		"image": func(path, alt string) (template.HTML, error) {
			html, resource, err := embedImage(page, path, alt)
			if err != nil {
				return "", err
			}
			resources = append(resources, resource)
			return html, nil
		},
		"date": formatDate,
	}
	tmpl, err := template.New("body").Funcs(funcs).Parse(string(body))
	if err != nil {
		return "", nil, nil, err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, TemplateRoot{
		Site: site,
		Page: page,
		Nav:  nav,
		Smol: SmolView{
			Version:                  Version,
			Profile:                  Profile,
			AuthoredContentOpen:      template.HTML(authoredContentOpen),
			AuthoredContentClose:     template.HTML(authoredContentClose),
			GeneratedNavigationOpen:  template.HTML(generatedNavigationOpen),
			GeneratedNavigationClose: template.HTML(generatedNavigationClose),
		},
	})
	if err != nil {
		return "", nil, nil, err
	}
	return buf.String(), resources, nil, nil
}

func embedImage(page Page, path, alt string) (template.HTML, ManifestResource, error) {
	return embedImageWithTitle(page, path, alt, "")
}

func embedImageWithTitle(page Page, path, alt, title string) (template.HTML, ManifestResource, error) {
	if alt == "" {
		return "", ManifestResource{}, fmt.Errorf("image alt text is required")
	}
	if page.contentDir == "" {
		return "", ManifestResource{}, fmt.Errorf("local images require bundle form")
	}
	if filepath.IsAbs(path) || strings.Contains(path, "..") {
		return "", ManifestResource{}, fmt.Errorf("image path escapes content directory: %s", path)
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	mimeType := imageMIME(ext)
	if mimeType == "" {
		return "", ManifestResource{}, fmt.Errorf("unsupported image extension: %s", ext)
	}
	full := filepath.Join(page.contentDir, filepath.FromSlash(path))
	data, err := os.ReadFile(full)
	if err != nil {
		return "", ManifestResource{}, err
	}
	hash := sha256.Sum256(data)
	src := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	titleAttr := ""
	if title != "" {
		titleAttr = ` title="` + template.HTMLEscapeString(title) + `"`
	}
	tag := `<img src="` + src + `" alt="` + template.HTMLEscapeString(alt) + `"` + titleAttr + ` loading="lazy" decoding="async">`
	plural := "pages"
	if page.Kind == "post" {
		plural = "posts"
	}
	return template.HTML(tag), ManifestResource{
		Kind:       "image",
		Source:     "content:" + plural + "/" + page.Slug + "/" + filepath.ToSlash(path),
		EmbeddedAs: "data-url",
		SHA256:     hex.EncodeToString(hash[:]),
	}, nil
}

func imageMIME(ext string) string {
	switch ext {
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "avif":
		return "image/avif"
	default:
		return ""
	}
}

func loadThemeCSS(themeDir string, theme ThemeConfig) (string, []ManifestResource, error) {
	var css strings.Builder
	var resources []ManifestResource
	for i, rel := range theme.CSS {
		if filepath.IsAbs(rel) || strings.Contains(rel, "..") {
			return "", nil, fmt.Errorf("theme CSS path escapes theme directory: %s", rel)
		}
		data, err := os.ReadFile(filepath.Join(themeDir, filepath.FromSlash(rel)))
		if err != nil {
			return "", nil, err
		}
		if i > 0 {
			css.WriteByte('\n')
		}
		css.Write(data)
		hash := sha256.Sum256(data)
		resources = append(resources, ManifestResource{
			Kind:       "css",
			Source:     "theme:" + theme.ID + "/" + filepath.ToSlash(rel),
			EmbeddedAs: "inline-style",
			SHA256:     hex.EncodeToString(hash[:]),
		})
	}
	return css.String(), resources, nil
}

func renderFullPage(themeDir string, theme ThemeConfig, kind, slug string, root TemplateRoot) (string, error) {
	templatePath := theme.Templates.Page
	if kind == "post" {
		templatePath = theme.Templates.Post
	}
	if kind == "page" && slug == "index" {
		templatePath = theme.Templates.Index
	}
	if filepath.IsAbs(templatePath) || strings.Contains(templatePath, "..") {
		return "", fmt.Errorf("template path escapes theme directory: %s", templatePath)
	}
	tmpl := template.New("page").Funcs(template.FuncMap{
		"date":   formatDate,
		"indent": indentHTML,
	})
	partialsDir := filepath.Join(themeDir, "partials")
	partials, err := os.ReadDir(partialsDir)
	if err == nil {
		for _, entry := range partials {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmpl") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(partialsDir, entry.Name()))
			if err != nil {
				return "", err
			}
			name := strings.TrimSuffix(entry.Name(), ".html.tmpl")
			if name == entry.Name() {
				name = strings.TrimSuffix(entry.Name(), ".tmpl")
			}
			if _, err := tmpl.Parse(`{{define "` + name + `"}}` + string(data) + `{{end}}`); err != nil {
				return "", err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(themeDir, filepath.FromSlash(templatePath)))
	if err != nil {
		return "", err
	}
	if _, err := tmpl.Parse(`{{define "__main__"}}` + string(data) + `{{end}}`); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "__main__", root); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func indentHTML(value template.HTML, prefix string) template.HTML {
	text := string(value)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return template.HTML(strings.Join(lines, "\n"))
}

func formatDate(value, layout string) (string, error) {
	if value == "" {
		return "", nil
	}
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", fmt.Errorf("invalid RFC3339 timestamp: %s", value)
	}
	return t.Format(layout), nil
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func relativeDisplay(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
