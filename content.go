package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type ContentMeta struct {
	Format       string        `json:"format"`
	Kind         string        `json:"kind"`
	Title        string        `json:"title"`
	Slug         string        `json:"slug"`
	Summary      string        `json:"summary"`
	PublishedUTC string        `json:"published_utc"`
	UpdatedUTC   string        `json:"updated_utc"`
	Tags         []string      `json:"tags"`
	Draft        bool          `json:"draft"`
	BodyFormat   string        `json:"body_format"`
	Header       []HeaderEntry `json:"header"`
	TOCColumns   [][]TOCItem   `json:"toc_columns"`
	MainSpacer   bool          `json:"main_spacer"`
}

type HeaderEntry struct {
	Level int           `json:"level"`
	HTML  template.HTML `json:"html"`
}

type TOCItem struct {
	Href string        `json:"href"`
	HTML template.HTML `json:"html"`
}

type Page struct {
	Kind         string
	Title        string
	Slug         string
	Summary      string
	PublishedUTC string
	UpdatedUTC   string
	Tags         []string
	URL          string
	CanonicalURL string
	ContentHTML  template.HTML
	Draft        bool
	BodyFormat   string
	Header       []HeaderEntry
	TOCColumns   [][]TOCItem
	MainSpacer   bool
	contentDir   string
	bodyPath     string
}

const (
	bodyFormatHTML          = "html"
	bodyFormatMarkdownXHTML = "markdown-xhtml-v1"
	bodyFormatGemtext       = "gemtext-v1"
)

func LoadContent(siteDir string, site SiteConfig) ([]Page, []Page, error) {
	pages, err := loadKind(siteDir, site, "page")
	if err != nil {
		return nil, nil, err
	}
	posts, err := loadKind(siteDir, site, "post")
	if err != nil {
		return nil, nil, err
	}
	sortPages(pages)
	sortPosts(posts)
	return pages, posts, nil
}

func loadKind(siteDir string, site SiteConfig, kind string) ([]Page, error) {
	baseName := "pages"
	if kind == "post" {
		baseName = "posts"
	}
	base := filepath.Join(siteDir, "content", baseName)
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return []Page{}, nil
		}
		return nil, err
	}
	var pages []Page
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(base, entry.Name())
		meta, err := loadContentMeta(dir)
		if err != nil {
			return nil, err
		}
		if err := validateContentMeta(meta, kind, entry.Name(), site.OutputMode); err != nil {
			return nil, err
		}
		if meta.Tags == nil {
			meta.Tags = []string{}
		}
		bodyFormat := meta.BodyFormat
		if bodyFormat == "" {
			bodyFormat = bodyFormatHTML
		}
		bodyName := "body.html"
		if bodyFormat == bodyFormatMarkdownXHTML {
			bodyName = "body.md"
		} else if bodyFormat == bodyFormatGemtext {
			bodyName = "body.gmi"
		}
		route := routeFor(meta.Kind, meta.Slug, site.OutputMode)
		pages = append(pages, Page{
			Kind:         meta.Kind,
			Title:        meta.Title,
			Slug:         meta.Slug,
			Summary:      meta.Summary,
			PublishedUTC: meta.PublishedUTC,
			UpdatedUTC:   meta.UpdatedUTC,
			Tags:         meta.Tags,
			URL:          route,
			CanonicalURL: canonicalURL(site.BaseURL, route),
			Draft:        meta.Draft,
			BodyFormat:   bodyFormat,
			Header:       meta.Header,
			TOCColumns:   meta.TOCColumns,
			MainSpacer:   meta.MainSpacer,
			contentDir:   dir,
			bodyPath:     filepath.Join(dir, bodyName),
		})
	}
	return pages, nil
}

func loadContentMeta(dir string) (ContentMeta, error) {
	data, err := os.ReadFile(filepath.Join(dir, "page.json"))
	if err != nil {
		return ContentMeta{}, err
	}
	var meta ContentMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return ContentMeta{}, fmt.Errorf("invalid page.json: %w", err)
	}
	return meta, nil
}

func validateContentMeta(meta ContentMeta, expectedKind, dirSlug, outputMode string) error {
	if meta.Format != "smol-page-v1" {
		return fmt.Errorf("invalid page.json: format must be smol-page-v1")
	}
	if meta.Kind != "page" && meta.Kind != "post" {
		return fmt.Errorf("invalid page.json: kind must be page or post")
	}
	if (outputMode == outputModeFlatXHTML || outputMode == outputModeFlatGemini) && meta.Kind != "page" {
		return fmt.Errorf("%s supports only pages", outputMode)
	}
	if meta.Kind != expectedKind {
		return fmt.Errorf("content kind does not match directory: %s in %s", meta.Kind, expectedKind)
	}
	if meta.Title == "" {
		return fmt.Errorf("invalid page.json: title is required")
	}
	if meta.Slug == "" {
		return fmt.Errorf("invalid page.json: slug is required")
	}
	if !slugRE.MatchString(meta.Slug) {
		return fmt.Errorf("invalid slug: %s", meta.Slug)
	}
	if meta.Slug != dirSlug {
		return fmt.Errorf("page slug does not match directory name: %s != %s", meta.Slug, dirSlug)
	}
	if meta.BodyFormat != "" && meta.BodyFormat != bodyFormatHTML && meta.BodyFormat != bodyFormatMarkdownXHTML && meta.BodyFormat != bodyFormatGemtext {
		return fmt.Errorf("invalid page.json: body_format must be html, markdown-xhtml-v1, or gemtext-v1")
	}
	if outputMode == outputModeFlatGemini && meta.BodyFormat != bodyFormatGemtext {
		return fmt.Errorf("flat-gemini-v1 supports only gemtext-v1 bodies")
	}
	for _, entry := range meta.Header {
		if entry.Level < 1 || entry.Level > 6 {
			return fmt.Errorf("invalid page.json: header level must be 1 through 6")
		}
	}
	for _, column := range meta.TOCColumns {
		for _, item := range column {
			if item.Href == "" {
				return fmt.Errorf("invalid page.json: toc_columns href is required")
			}
		}
	}
	return nil
}

func sortPages(pages []Page) {
	sort.SliceStable(pages, func(i, j int) bool {
		return pages[i].URL < pages[j].URL
	})
}

func sortPosts(posts []Page) {
	sort.SliceStable(posts, func(i, j int) bool {
		leftTime, leftOK := parseOptionalTime(posts[i].PublishedUTC)
		rightTime, rightOK := parseOptionalTime(posts[j].PublishedUTC)
		if leftOK && rightOK && !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		if leftOK != rightOK {
			return leftOK
		}
		return posts[i].Slug < posts[j].Slug
	})
}

func parseOptionalTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, value)
	return t, err == nil
}

func nonDraft(in []Page) []Page {
	out := make([]Page, 0, len(in))
	for _, page := range in {
		if !page.Draft {
			out = append(out, page)
		}
	}
	return out
}
