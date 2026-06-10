package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

type ContentMeta struct {
	Title        string     `json:"title"`
	Summary      string     `json:"summary"`
	PublishedUTC string     `json:"published_utc"`
	UpdatedUTC   string     `json:"updated_utc"`
	Tags         []string   `json:"tags"`
	Draft        bool       `json:"draft"`
	TOC          bool       `json:"toc"`
	Links        *PageLinks `json:"links,omitempty"`
}

type TOCEntry struct {
	Href string
	Text template.HTML
}

type PageLinks struct {
	Up       string   `json:"up,omitempty"`
	Previous string   `json:"previous,omitempty"`
	Next     string   `json:"next,omitempty"`
	Related  []string `json:"related,omitempty"`
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
	TOC          []TOCEntry
	Links        PageLinks
	Navigation   PageNavigation
	contentDir   string
	bodyPath     string
	body         []byte
	tocRequested bool
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
	seen := map[string]string{}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(base, name)
		if entry.IsDir() {
			slug := name
			if err := rememberSlug(seen, slug, path); err != nil {
				return nil, err
			}
			sourcePath, err := bundleIndexPath(path)
			if err != nil {
				return nil, err
			}
			page, err := loadContentSource(site, kind, slug, sourcePath, path)
			if err != nil {
				return nil, err
			}
			pages = append(pages, page)
			continue
		}
		ext := filepath.Ext(name)
		if bodyFormatForExtension(strings.ToLower(ext)) == "" {
			return nil, fmt.Errorf("unsupported content file: %s", path)
		}
		slug := strings.TrimSuffix(name, ext)
		if err := rememberSlug(seen, slug, path); err != nil {
			return nil, err
		}
		page, err := loadContentSource(site, kind, slug, path, "")
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}
	return pages, nil
}

func rememberSlug(seen map[string]string, slug, path string) error {
	if prior := seen[slug]; prior != "" {
		return fmt.Errorf("content slug collision: %s and %s", prior, path)
	}
	seen[slug] = path
	return nil
}

func bundleIndexPath(dir string) (string, error) {
	var found []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || entry.IsDir() {
			continue
		}
		ext := filepath.Ext(name)
		extLower := strings.ToLower(ext)
		if bodyFormatForExtension(extLower) == "" {
			continue
		}
		base := strings.TrimSuffix(name, ext)
		path := filepath.Join(dir, name)
		if !strings.EqualFold(base, "index") {
			return "", fmt.Errorf("content bundle contains non-index source file: %s", path)
		}
		if _, err := entry.Info(); err != nil {
			return "", err
		}
		found = append(found, path)
	}
	if len(found) == 0 {
		return "", fmt.Errorf("content bundle missing index source: %s", dir)
	}
	if len(found) > 1 {
		return "", fmt.Errorf("content bundle has multiple index sources: %s", dir)
	}
	return found[0], nil
}

func loadContentSource(site SiteConfig, kind, slug, sourcePath, contentDir string) (Page, error) {
	bodyFormat := bodyFormatForExtension(strings.ToLower(filepath.Ext(sourcePath)))
	if bodyFormat == "" {
		return Page{}, fmt.Errorf("unsupported content source: %s", sourcePath)
	}
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return Page{}, err
	}
	meta, body, err := parseContentFrontMatter(data, sourcePath)
	if err != nil {
		return Page{}, err
	}
	if err := validateContentMeta(meta, kind, slug, bodyFormat, site.OutputMode); err != nil {
		return Page{}, err
	}
	if meta.Tags == nil {
		meta.Tags = []string{}
	}
	links := PageLinks{}
	if meta.Links != nil {
		links = *meta.Links
	}
	route := routeFor(kind, slug, site.OutputMode)
	return Page{
		Kind:         kind,
		Title:        resolveContentTitle(meta.Title, slug),
		Slug:         slug,
		Summary:      meta.Summary,
		PublishedUTC: meta.PublishedUTC,
		UpdatedUTC:   meta.UpdatedUTC,
		Tags:         meta.Tags,
		URL:          route,
		CanonicalURL: canonicalURL(site.BaseURL, route),
		Draft:        meta.Draft,
		BodyFormat:   bodyFormat,
		Links:        links,
		contentDir:   contentDir,
		bodyPath:     sourcePath,
		body:         body,
		tocRequested: meta.TOC,
	}, nil
}

func bodyFormatForExtension(ext string) string {
	switch ext {
	case ".html":
		return bodyFormatHTML
	case ".md":
		return bodyFormatMarkdownXHTML
	case ".gmi":
		return bodyFormatGemtext
	default:
		return ""
	}
}

func validateContentMeta(meta ContentMeta, expectedKind, slug, bodyFormat, outputMode string) error {
	if (outputMode == outputModeFlatXHTML || outputMode == outputModeFlatGemini) && expectedKind != "page" {
		return fmt.Errorf("%s supports only pages", outputMode)
	}
	if !slugRE.MatchString(slug) {
		return fmt.Errorf("invalid slug: %s", slug)
	}
	if outputMode == outputModeFlatGemini && bodyFormat != bodyFormatGemtext && bodyFormat != bodyFormatMarkdownXHTML {
		return fmt.Errorf("flat-gemini-v1 supports only markdown-xhtml-v1 or gemtext-v1 bodies")
	}
	if meta.TOC && outputMode == outputModeFlatGemini {
		return fmt.Errorf("toc is not supported for flat-gemini-v1")
	}
	if meta.TOC && bodyFormat != bodyFormatMarkdownXHTML {
		return fmt.Errorf("toc requires a markdown body")
	}
	if meta.Links != nil {
		if err := validatePageLinks(*meta.Links); err != nil {
			return err
		}
	}
	return nil
}

func validatePageLinks(links PageLinks) error {
	for _, link := range []struct {
		name string
		slug string
	}{
		{name: "up", slug: links.Up},
		{name: "previous", slug: links.Previous},
		{name: "next", slug: links.Next},
	} {
		name, slug := link.name, link.slug
		if slug != "" && !slugRE.MatchString(slug) {
			return fmt.Errorf("invalid content metadata: links.%s must be a page slug", name)
		}
	}
	for _, slug := range links.Related {
		if slug == "" || !slugRE.MatchString(slug) {
			return fmt.Errorf("invalid content metadata: links.related must contain page slugs")
		}
	}
	return nil
}

func parseContentFrontMatter(data []byte, path string) (ContentMeta, []byte, error) {
	data = stripLeadingBOM(data)
	hasOpening, err := hasFrontMatterOpening(data)
	if err != nil {
		return ContentMeta{}, nil, fmt.Errorf("%s: %w", path, err)
	}
	if !hasOpening {
		return ContentMeta{}, data, nil
	}
	_, pos, _ := readContentLine(data, 0)
	lines := []frontMatterLine{}
	lineNo := 2
	for pos <= len(data) {
		if pos == len(data) {
			return ContentMeta{}, nil, fmt.Errorf("%s: unclosed front matter; a leading --- starts front matter and must be closed with ---", path)
		}
		line, next, hadTerminator := readContentLine(data, pos)
		if isFrontMatterClosingFenceLine(line) {
			if !hadTerminator {
				next = len(data)
			}
			meta, err := parseFrontMatterLines(lines, path)
			if err != nil {
				return ContentMeta{}, nil, err
			}
			return meta, data[next:], nil
		}
		lines = append(lines, frontMatterLine{
			text:   string(line),
			no:     lineNo,
			indent: frontMatterIndent(string(line)),
		})
		pos = next
		lineNo++
	}
	return ContentMeta{}, nil, fmt.Errorf("%s: unclosed front matter; a leading --- starts front matter and must be closed with ---", path)
}

func stripLeadingBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xef && data[1] == 0xbb && data[2] == 0xbf {
		return data[3:]
	}
	return data
}

func hasFrontMatterOpening(data []byte) (bool, error) {
	if len(data) < 3 || string(data[:3]) != "---" {
		return false, nil
	}
	if len(data) == 3 {
		return true, nil
	}
	line, _, terminator := readContentLine(data, 0)
	if !isFrontMatterFenceLine(line) {
		return false, nil
	}
	if !terminator {
		return true, nil
	}
	if strings.Contains(string(data[:len(line)+1]), "\r") && (len(data) <= len(line)+1 || data[len(line)+1] != '\n') {
		return false, fmt.Errorf("front matter uses unsupported CR-only line endings; use LF or CRLF")
	}
	return true, nil
}

func isFrontMatterFenceLine(line []byte) bool {
	return string(bytes.TrimRight(line, " \t")) == "---"
}

func isFrontMatterClosingFenceLine(line []byte) bool {
	trimmed := string(bytes.TrimRight(line, " \t"))
	return trimmed == "---" || trimmed == "..."
}

func readContentLine(data []byte, pos int) ([]byte, int, bool) {
	start := pos
	for pos < len(data) && data[pos] != '\n' && data[pos] != '\r' {
		pos++
	}
	line := data[start:pos]
	if pos >= len(data) {
		return line, pos, false
	}
	if data[pos] == '\r' && pos+1 < len(data) && data[pos+1] == '\n' {
		return line, pos + 2, true
	}
	return line, pos + 1, true
}

type frontMatterLine struct {
	text   string
	no     int
	indent int
}

type frontMatterParseState struct {
	meta         ContentMeta
	publishedSet bool
	updatedSet   bool
}

type frontMatterParser func(*frontMatterParseState, []frontMatterLine, int, string) (int, error)

var frontMatterParsers = map[string]frontMatterParser{
	"title": func(state *frontMatterParseState, _ []frontMatterLine, index int, value string) (int, error) {
		title, err := parseScalarString(value)
		if err != nil {
			return index, err
		}
		if title == "" {
			return index, fmt.Errorf("title must not be empty")
		}
		state.meta.Title = title
		return index, nil
	},
	"summary": func(state *frontMatterParseState, _ []frontMatterLine, index int, value string) (int, error) {
		v, err := parseScalarString(value)
		if err != nil {
			return index, err
		}
		state.meta.Summary = v
		return index, nil
	},
	"published_utc": parsePublishedFrontMatterDate("published_utc"),
	"published":     parsePublishedFrontMatterDate("published"),
	"updated_utc":   parseUpdatedFrontMatterDate("updated_utc"),
	"updated":       parseUpdatedFrontMatterDate("updated"),
	"draft": func(state *frontMatterParseState, _ []frontMatterLine, index int, value string) (int, error) {
		v, err := parseStrictBool("draft", value)
		if err != nil {
			return index, err
		}
		state.meta.Draft = v
		return index, nil
	},
	"toc": func(state *frontMatterParseState, _ []frontMatterLine, index int, value string) (int, error) {
		v, err := parseStrictBool("toc", value)
		if err != nil {
			return index, err
		}
		state.meta.TOC = v
		return index, nil
	},
	"tags": func(state *frontMatterParseState, lines []frontMatterLine, index int, value string) (int, error) {
		values, next, err := parseStringListValue(value, lines, index, lines[index].indent)
		if err != nil {
			return index, err
		}
		state.meta.Tags = values
		return next, nil
	},
	"links": func(state *frontMatterParseState, lines []frontMatterLine, index int, value string) (int, error) {
		links, next, err := parseLinksBlock(value, lines, index, lines[index].indent)
		if err != nil {
			return index, err
		}
		state.meta.Links = &links
		return next, nil
	},
}

func parseFrontMatterLines(lines []frontMatterLine, path string) (ContentMeta, error) {
	state := &frontMatterParseState{}
	seen := map[string]int{}
	for i := 0; i < len(lines); i++ {
		raw := lines[i].text
		if strings.TrimSpace(raw) == "" {
			continue
		}
		key, value, ok, err := splitFrontMatterKeyValue(raw)
		if err != nil {
			return ContentMeta{}, frontMatterError(path, lines[i].no, err.Error())
		}
		if !ok {
			return ContentMeta{}, frontMatterError(path, lines[i].no, "expected key: value; a leading --- starts front matter")
		}
		parser, known := frontMatterParsers[key]
		if !known {
			next, errLine, err := skipUnknownFrontMatterBlock(lines, i, lines[i].indent, key)
			if err != nil {
				return ContentMeta{}, frontMatterError(path, errLine, err.Error())
			}
			i = next
			continue
		}
		if lines[i].indent != 0 {
			return ContentMeta{}, frontMatterError(path, lines[i].no, fmt.Sprintf("recognized key %q must start at column 0", key))
		}
		if prior := seen[key]; prior != 0 {
			return ContentMeta{}, frontMatterError(path, lines[i].no, fmt.Sprintf("duplicate key %q previously defined on line %d", key, prior))
		}
		seen[key] = lines[i].no
		next, err := parser(state, lines, i, value)
		if err != nil {
			return ContentMeta{}, frontMatterError(path, lines[i].no, err.Error())
		}
		i = next
	}
	return state.meta, nil
}

func knownFrontMatterKey(key string) bool {
	_, ok := frontMatterParsers[key]
	return ok
}

func skipUnknownFrontMatterBlock(lines []frontMatterLine, index, parentIndent int, parentKey string) (int, int, error) {
	i := index + 1
	for {
		childIndex, trimmed, ok := nextFrontMatterChildLine(lines, i, parentIndent, true)
		if !ok {
			break
		}
		key, _, keyOK, err := splitFrontMatterKeyValue(trimmed)
		if err == nil && keyOK && knownFrontMatterKey(key) {
			return index, lines[childIndex].no, fmt.Errorf("recognized key %q found nested under unknown key %q; move it to column 0 or remove the tool-specific block", key, parentKey)
		}
		i = childIndex + 1
	}
	return i - 1, 0, nil
}

func nextFrontMatterChildLine(lines []frontMatterLine, index, parentIndent int, allowKeyIndentList bool) (int, string, bool) {
	for index < len(lines) {
		line := lines[index].text
		if strings.TrimSpace(line) == "" {
			index++
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		if lines[index].indent > parentIndent || (allowKeyIndentList && lines[index].indent == parentIndent && isBlockListItem(trimmed)) {
			return index, trimmed, true
		}
		return index, "", false
	}
	return index, "", false
}

func isBlockListItem(trimmed string) bool {
	return trimmed == "-" || strings.HasPrefix(trimmed, "- ")
}

func splitFrontMatterKeyValue(line string) (string, string, bool, error) {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" {
		return "", "", false, nil
	}
	var key, rest string
	if trimmed[0] == '"' || trimmed[0] == '\'' {
		var err error
		key, rest, err = parseQuotedFrontMatterKey(trimmed)
		if err != nil {
			return "", "", false, err
		}
		rest = strings.TrimLeft(rest, " \t")
		if !strings.HasPrefix(rest, ":") {
			return "", "", false, fmt.Errorf("invalid quoted key: expected ':' after quoted key")
		}
		rest = rest[1:]
	} else {
		colon := strings.IndexByte(trimmed, ':')
		if colon <= 0 {
			return "", "", false, nil
		}
		key = strings.TrimSpace(trimmed[:colon])
		rest = trimmed[colon+1:]
	}
	if key == "" {
		return "", "", false, nil
	}
	if strings.ContainsAny(key, " \t") {
		return "", "", false, fmt.Errorf("front matter key %q must not contain whitespace", key)
	}
	return key, strings.TrimSpace(rest), true, nil
}

func parseQuotedFrontMatterKey(value string) (string, string, error) {
	switch value[0] {
	case '"':
		escape := false
		for i := 1; i < len(value); i++ {
			if escape {
				escape = false
				continue
			}
			if value[i] == '\\' {
				escape = true
				continue
			}
			if value[i] == '"' {
				var key string
				if err := json.Unmarshal([]byte(value[:i+1]), &key); err != nil {
					return "", "", fmt.Errorf("invalid quoted key: %w", err)
				}
				return key, value[i+1:], nil
			}
		}
		return "", "", fmt.Errorf("invalid quoted key: unterminated double-quoted key")
	case '\'':
		key, rest, err := parseSingleQuotedPrefix(value)
		if err != nil {
			return "", "", fmt.Errorf("invalid quoted key: %w", err)
		}
		return key, rest, nil
	default:
		return "", "", fmt.Errorf("invalid quoted key")
	}
}

func parsePublishedFrontMatterDate(field string) frontMatterParser {
	return func(state *frontMatterParseState, _ []frontMatterLine, index int, value string) (int, error) {
		if state.publishedSet {
			return index, fmt.Errorf("published and published_utc cannot both be set")
		}
		parsed, err := parseFrontMatterDate(field, value)
		if err != nil {
			return index, err
		}
		state.meta.PublishedUTC = parsed
		state.publishedSet = true
		return index, nil
	}
}

func parseUpdatedFrontMatterDate(field string) frontMatterParser {
	return func(state *frontMatterParseState, _ []frontMatterLine, index int, value string) (int, error) {
		if state.updatedSet {
			return index, fmt.Errorf("updated and updated_utc cannot both be set")
		}
		parsed, err := parseFrontMatterDate(field, value)
		if err != nil {
			return index, err
		}
		state.meta.UpdatedUTC = parsed
		state.updatedSet = true
		return index, nil
	}
}

func parseFrontMatterDate(field, value string) (string, error) {
	parsed, err := parseScalarString(value)
	if err != nil {
		return "", err
	}
	if parsed == "" {
		return "", nil
	}
	if _, err := time.Parse(time.RFC3339, parsed); err == nil {
		return parsed, nil
	}
	if t, err := time.Parse("2006-01-02", parsed); err == nil {
		return t.UTC().Format(time.RFC3339), nil
	}
	return "", fmt.Errorf("%s: expected RFC3339 timestamp or YYYY-MM-DD date, got %q", field, parsed)
}

func frontMatterIndent(line string) int {
	indent := 0
	for _, r := range line {
		if r == ' ' {
			indent++
			continue
		}
		if r == '\t' {
			indent += 4 - indent%4
			continue
		}
		break
	}
	return indent
}

func parseSingleQuotedPrefix(value string) (string, string, error) {
	if value == "" || value[0] != '\'' {
		return "", "", fmt.Errorf("expected single-quoted string")
	}
	var out strings.Builder
	for i := 1; i < len(value); i++ {
		if value[i] != '\'' {
			out.WriteByte(value[i])
			continue
		}
		if i+1 < len(value) && value[i+1] == '\'' {
			out.WriteByte('\'')
			i++
			continue
		}
		return out.String(), value[i+1:], nil
	}
	return "", "", fmt.Errorf("unterminated single-quoted string")
}

func parseScalarString(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "\"") {
		var out string
		if err := json.Unmarshal([]byte(value), &out); err != nil {
			return "", fmt.Errorf("invalid quoted string: %w", err)
		}
		return out, nil
	}
	if strings.HasPrefix(value, "'") {
		return parseSingleQuotedString(value)
	}
	return value, nil
}

func parseSingleQuotedString(value string) (string, error) {
	out, rest, err := parseSingleQuotedPrefix(value)
	if err != nil {
		return "", fmt.Errorf("invalid single-quoted string: value starts with ' so it is parsed as single-quoted; close it with ' or use double quotes")
	}
	if strings.TrimSpace(rest) != "" {
		return "", fmt.Errorf("invalid single-quoted string: trailing text after closing quote")
	}
	return out, nil
}

func parseStrictBool(name, value string) (bool, error) {
	token := strings.TrimSpace(value)
	switch strings.ToLower(token) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s: expected true or false, got %q", name, token)
	}
}

func parseStringListValue(value string, lines []frontMatterLine, index, parentIndent int) ([]string, int, error) {
	value = strings.TrimSpace(value)
	if value != "" {
		values, err := parseInlineStringList(value)
		return values, index, err
	}
	var values []string
	i := index + 1
	for {
		childIndex, trimmed, ok := nextFrontMatterChildLine(lines, i, parentIndent, true)
		if !ok {
			break
		}
		if !isBlockListItem(trimmed) {
			break
		}
		itemText := ""
		if trimmed != "-" {
			itemText = strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		}
		item, err := parseScalarString(itemText)
		if err != nil {
			return nil, index, err
		}
		if item == "" {
			return nil, index, fmt.Errorf("list items must not be empty")
		}
		values = append(values, item)
		i = childIndex + 1
	}
	if len(values) == 0 {
		return nil, index, fmt.Errorf("list must contain at least one item")
	}
	return values, i - 1, nil
}

func parseInlineStringList(value string) ([]string, error) {
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil, fmt.Errorf("list must use [a, b] or block list syntax")
	}
	inner := strings.TrimSpace(value[1 : len(value)-1])
	if inner == "" {
		return []string{}, nil
	}
	parts, err := splitInlineList(inner)
	if err != nil {
		return nil, err
	}
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			return nil, fmt.Errorf("list items must not be empty")
		}
		item, err := parseScalarString(trimmed)
		if err != nil {
			return nil, err
		}
		if item == "" {
			return nil, fmt.Errorf("list items must not be empty")
		}
		values = append(values, item)
	}
	return values, nil
}

func splitInlineList(value string) ([]string, error) {
	var parts []string
	var current strings.Builder
	quote := byte(0)
	escape := false
	atItemStart := true
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if escape {
			current.WriteByte(ch)
			escape = false
			continue
		}
		if quote != 0 {
			if quote == '"' {
				if ch == '\\' {
					current.WriteByte(ch)
					escape = true
					continue
				}
				if ch == '"' {
					quote = 0
					current.WriteByte(ch)
					continue
				}
			}
			if quote == '\'' && ch == '\'' {
				if i+1 < len(value) && value[i+1] == '\'' {
					current.WriteByte(ch)
					current.WriteByte(value[i+1])
					i++
					continue
				}
				quote = 0
				current.WriteByte(ch)
				continue
			}
			current.WriteByte(ch)
			continue
		}
		if ch == ',' {
			parts = append(parts, current.String())
			current.Reset()
			atItemStart = true
			continue
		}
		if (ch == ' ' || ch == '\t') && atItemStart {
			current.WriteByte(ch)
			continue
		}
		if (ch == '"' || ch == '\'') && atItemStart {
			quote = ch
			current.WriteByte(ch)
			atItemStart = false
			continue
		}
		current.WriteByte(ch)
		atItemStart = false
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quoted list item")
	}
	parts = append(parts, current.String())
	return parts, nil
}

func parseLinksBlock(value string, lines []frontMatterLine, index, parentIndent int) (PageLinks, int, error) {
	if strings.TrimSpace(value) != "" {
		return PageLinks{}, index, fmt.Errorf("links must be a block map")
	}
	var links PageLinks
	seen := map[string]bool{}
	i := index + 1
	for {
		childIndex, trimmed, ok := nextFrontMatterChildLine(lines, i, parentIndent, false)
		if !ok {
			break
		}
		key, value, splitOK, err := splitFrontMatterKeyValue(trimmed)
		if err != nil {
			return PageLinks{}, index, err
		}
		if !splitOK {
			return PageLinks{}, index, fmt.Errorf("invalid links entry")
		}
		if seen[key] {
			return PageLinks{}, index, fmt.Errorf("duplicate links.%s", key)
		}
		seen[key] = true
		switch key {
		case "up":
			v, err := parseScalarString(value)
			if err != nil {
				return PageLinks{}, index, err
			}
			links.Up = v
		case "previous":
			v, err := parseScalarString(value)
			if err != nil {
				return PageLinks{}, index, err
			}
			links.Previous = v
		case "next":
			v, err := parseScalarString(value)
			if err != nil {
				return PageLinks{}, index, err
			}
			links.Next = v
		case "related":
			values, next, err := parseStringListValue(value, lines, childIndex, lines[childIndex].indent)
			if err != nil {
				return PageLinks{}, index, err
			}
			links.Related = values
			childIndex = next
		default:
			return PageLinks{}, index, fmt.Errorf("unknown links key: %s", key)
		}
		i = childIndex + 1
	}
	return links, i - 1, nil
}

func frontMatterError(path string, line int, msg string) error {
	return fmt.Errorf("%s:%d: invalid front matter: %s", path, line, msg)
}

func resolveContentTitle(title, slug string) string {
	if title != "" {
		return title
	}
	if slug == "index" {
		return "Home"
	}
	parts := strings.Split(slug, "-")
	for i, part := range parts {
		parts[i] = titleWord(part)
	}
	return strings.Join(parts, " ")
}

func titleWord(value string) string {
	if value == "" {
		return ""
	}
	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
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
