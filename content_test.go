package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseContentFrontMatter(t *testing.T) {
	tests := map[string]struct {
		input     string
		wantMeta  ContentMeta
		wantBody  string
		wantLinks *PageLinks
	}{
		"no front matter": {
			input:    "Body only.\n",
			wantBody: "Body only.\n",
		},
		"empty front matter": {
			input:    "---\n---\nBody.\n",
			wantBody: "Body.\n",
		},
		"bom without front matter": {
			input:    "\xef\xbb\xbfBody only.\n",
			wantBody: "Body only.\n",
		},
		"bom with front matter": {
			input:    "\xef\xbb\xbf---\ntitle: BOM\n---\nBody.\n",
			wantMeta: ContentMeta{Title: "BOM"},
			wantBody: "Body.\n",
		},
		"trailing whitespace fences": {
			input:    "--- \t\ntitle: Fence\n--- \t\nBody.\n",
			wantMeta: ContentMeta{Title: "Fence"},
			wantBody: "Body.\n",
		},
		"ellipsis closing fence": {
			input:    "---\ntitle: Ellipsis\n...\nBody.\n",
			wantMeta: ContentMeta{Title: "Ellipsis"},
			wantBody: "Body.\n",
		},
		"ellipsis is not opening fence": {
			input:    "...\ntitle: Body\n...\n",
			wantBody: "...\ntitle: Body\n...\n",
		},
		"crlf preserves body bytes": {
			input:    "---\r\ntitle: CRLF\r\n---\r\nBody\r\n",
			wantMeta: ContentMeta{Title: "CRLF"},
			wantBody: "Body\r\n",
		},
		"quoted keys": {
			input:    "---\n\"draft\": true\n'title': Quoted Key\n---\nBody.\n",
			wantMeta: ContentMeta{Title: "Quoted Key", Draft: true},
			wantBody: "Body.\n",
		},
		"quoted unknown key ignored": {
			input:    "---\n\"aliases\": [old]\ntitle: Known\n---\nBody.\n",
			wantMeta: ContentMeta{Title: "Known"},
			wantBody: "Body.\n",
		},
		"quoted scalar containing colon": {
			input:    "---\ntitle: \"Time: A History\"\n---\nBody.\n",
			wantMeta: ContentMeta{Title: "Time: A History"},
			wantBody: "Body.\n",
		},
		"date only normalizes": {
			input:    "---\npublished: 2026-06-07\nupdated_utc: 2026-06-08\n---\nBody.\n",
			wantMeta: ContentMeta{PublishedUTC: "2026-06-07T00:00:00Z", UpdatedUTC: "2026-06-08T00:00:00Z"},
			wantBody: "Body.\n",
		},
		"rfc3339 dates pass unchanged": {
			input:    "---\npublished: 2026-06-07T12:34:56Z\nupdated: 2026-06-08T01:02:03Z\n---\nBody.\n",
			wantMeta: ContentMeta{PublishedUTC: "2026-06-07T12:34:56Z", UpdatedUTC: "2026-06-08T01:02:03Z"},
			wantBody: "Body.\n",
		},
		"empty dates are omitted": {
			input:    "---\npublished:\nupdated_utc:\n---\nBody.\n",
			wantBody: "Body.\n",
		},
		"single quoted scalar": {
			input:    "---\ntitle: 'Don''t Panic'\n---\nBody.\n",
			wantMeta: ContentMeta{Title: "Don't Panic"},
			wantBody: "Body.\n",
		},
		"hash is literal": {
			input:    "---\ntitle: C# Programming\n---\nBody.\n",
			wantMeta: ContentMeta{Title: "C# Programming"},
			wantBody: "Body.\n",
		},
		"inline list with quoted comma": {
			input:    "---\ntags: [meta, \"b, c\"]\n---\nBody.\n",
			wantMeta: ContentMeta{Tags: []string{"meta", "b, c"}},
			wantBody: "Body.\n",
		},
		"inline list with bare apostrophes": {
			input:    "---\ntags: [don't, a's, stop]\n---\nBody.\n",
			wantMeta: ContentMeta{Tags: []string{"don't", "a's", "stop"}},
			wantBody: "Body.\n",
		},
		"inline list with single quote escape and comma": {
			input:    "---\ntags: ['a'', b', \"c, d\"]\n---\nBody.\n",
			wantMeta: ContentMeta{Tags: []string{"a', b", "c, d"}},
			wantBody: "Body.\n",
		},
		"block list": {
			input:    "---\ntags:\n  - alpha\n  - \"beta, gamma\"\n---\nBody.\n",
			wantMeta: ContentMeta{Tags: []string{"alpha", "beta, gamma"}},
			wantBody: "Body.\n",
		},
		"block list at key indent ends before next key": {
			input:    "---\ntags:\n- alpha\n- beta\nsummary: Done\n---\nBody.\n",
			wantMeta: ContentMeta{Tags: []string{"alpha", "beta"}, Summary: "Done"},
			wantBody: "Body.\n",
		},
		"case insensitive booleans": {
			input:    "---\ndraft: True\nmain_spacer: FALSE\n---\nBody.\n",
			wantMeta: ContentMeta{Draft: true},
			wantBody: "Body.\n",
		},
		"unknown keys ignored": {
			input: "---\ntitle: Known\naliases:\n  - old\ncssclass: wide\n---\nBody.\n",
			wantMeta: ContentMeta{
				Title: "Known",
			},
			wantBody: "Body.\n",
		},
		"unknown key indent list ignored": {
			input:    "---\ncategories:\n- news\n- updates\nsummary: Done\n---\nBody.\n",
			wantMeta: ContentMeta{Summary: "Done"},
			wantBody: "Body.\n",
		},
		"unknown nested map ignored": {
			input:    "---\nparams:\n  seo:\n    description: Search text\nsummary: Done\n---\nBody.\n",
			wantMeta: ContentMeta{Summary: "Done"},
			wantBody: "Body.\n",
		},
		"unknown list of maps ignored": {
			input: "---\nresources:\n- name: hero\n  src: hero.jpg\nsummary: Done\n---\nBody.\n",
			wantMeta: ContentMeta{
				Summary: "Done",
			},
			wantBody: "Body.\n",
		},
		"unknown quoted multi word child ignored": {
			input: "---\nforeign:\n  \"a b\": 1\nsummary: Done\n---\nBody.\n",
			wantMeta: ContentMeta{
				Summary: "Done",
			},
			wantBody: "Body.\n",
		},
		"top-level links subkey ignored": {
			input:    "---\ntitle: Known\nrelated: [ignored]\n---\nBody.\n",
			wantMeta: ContentMeta{Title: "Known"},
			wantBody: "Body.\n",
		},
		"json metadata": {
			input:    "----\n",
			wantBody: "----\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			meta, body, err := parseContentFrontMatter([]byte(tc.input), name)
			if err != nil {
				t.Fatalf("parseContentFrontMatter: %v", err)
			}
			if string(body) != tc.wantBody {
				t.Fatalf("body = %q, want %q", string(body), tc.wantBody)
			}
			if !reflect.DeepEqual(meta, tc.wantMeta) {
				t.Fatalf("meta = %#v, want %#v", meta, tc.wantMeta)
			}
		})
	}
}

func TestParseContentFrontMatterJSONFields(t *testing.T) {
	input := "---\n" +
		`header: [{"level":1,"html":"Title"}]` + "\n" +
		`toc_columns: [[{"href":"#x","html":"X"}]]` + "\n" +
		"---\nBody.\n"
	meta, body, err := parseContentFrontMatter([]byte(input), "page.md")
	if err != nil {
		t.Fatalf("parseContentFrontMatter: %v", err)
	}
	if string(body) != "Body.\n" {
		t.Fatalf("body = %q", string(body))
	}
	if len(meta.Header) != 1 || meta.Header[0].Level != 1 || string(meta.Header[0].HTML) != "Title" {
		t.Fatalf("header = %#v", meta.Header)
	}
	if len(meta.TOCColumns) != 1 || len(meta.TOCColumns[0]) != 1 || meta.TOCColumns[0][0].Href != "#x" || string(meta.TOCColumns[0][0].HTML) != "X" {
		t.Fatalf("toc_columns = %#v", meta.TOCColumns)
	}
}

func TestParseContentFrontMatterLinksIndentation(t *testing.T) {
	tests := map[string]string{
		"two-space":       "links:\n  up: essays\n  related:\n    - kore\n    - lament\n",
		"four-space":      "links:\n    up: essays\n    related: [kore, lament]\n",
		"tab":             "links:\n\tup: essays\n\trelated:\n\t\t- kore\n\t\t- lament\n",
		"mixed-space-tab": "links:\n  up: essays\n  related:\n\t- kore\n\t- lament\n",
		"key-indent-list": "links:\n  up: essays\n  related:\n  - kore\n  - lament\n",
	}
	for name, fields := range tests {
		t.Run(name, func(t *testing.T) {
			input := "---\n" + fields + "---\nBody.\n"
			meta, body, err := parseContentFrontMatter([]byte(input), "page.md")
			if err != nil {
				t.Fatalf("parseContentFrontMatter: %v", err)
			}
			if string(body) != "Body.\n" {
				t.Fatalf("body = %q", string(body))
			}
			if meta.Links == nil {
				t.Fatalf("links not parsed")
			}
			if meta.Links.Up != "essays" || !reflect.DeepEqual(meta.Links.Related, []string{"kore", "lament"}) {
				t.Fatalf("links = %#v", *meta.Links)
			}
		})
	}
}

func TestParseContentFrontMatterLinksBlockListBoundary(t *testing.T) {
	input := "---\nlinks:\n  related:\n  - kore\n  next: lament\n---\nBody.\n"
	meta, body, err := parseContentFrontMatter([]byte(input), "page.md")
	if err != nil {
		t.Fatalf("parseContentFrontMatter: %v", err)
	}
	if string(body) != "Body.\n" {
		t.Fatalf("body = %q", string(body))
	}
	if meta.Links == nil {
		t.Fatalf("links not parsed")
	}
	if !reflect.DeepEqual(meta.Links.Related, []string{"kore"}) || meta.Links.Next != "lament" {
		t.Fatalf("links = %#v", *meta.Links)
	}
}

func TestParseContentFrontMatterErrors(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"unclosed": {
			input: "---\ntitle: Missing Close\nBody.\n",
			want:  "a leading --- starts front matter",
		},
		"duplicate recognized": {
			input: "---\ntitle: One\ntitle: Two\n---\nBody.\n",
			want:  "duplicate key",
		},
		"malformed bool": {
			input: "---\ndraft: yes\n---\nBody.\n",
			want:  `draft: expected true or false, got "yes"`,
		},
		"invalid date": {
			input: "---\npublished: June 7, 2026\n---\nBody.\n",
			want:  `published: expected RFC3339 timestamp or YYYY-MM-DD date, got "June 7, 2026"`,
		},
		"alias collision": {
			input: "---\npublished: 2026-01-01T00:00:00Z\npublished_utc: 2026-01-02T00:00:00Z\n---\nBody.\n",
			want:  "published and published_utc cannot both be set",
		},
		"quoted key with whitespace": {
			input: "---\n\"bad key\": value\n---\nBody.\n",
			want:  `front matter key "bad key" must not contain whitespace`,
		},
		"malformed quoted key": {
			input: "---\n\"draft: true\n---\nBody.\n",
			want:  "invalid quoted key",
		},
		"empty title": {
			input: "---\ntitle:\n---\nBody.\n",
			want:  "title must not be empty",
		},
		"malformed list": {
			input: "---\ntags: alpha\n---\nBody.\n",
			want:  "list must use",
		},
		"bare tags no items": {
			input: "---\ntags:\n---\nBody.\n",
			want:  "list must contain at least one item",
		},
		"nested recognized under unknown": {
			input: "---\nparams:\n  draft: true\n---\nBody.\n",
			want:  `nested recognized under unknown:3: invalid front matter: recognized key "draft" found nested under unknown key "params"; move it to column 0 or remove the tool-specific block`,
		},
		"over-indented recognized key": {
			input: "---\ntags:\n  - a\n    title: Hidden\n---\nBody.\n",
			want:  `recognized key "title" must start at column 0`,
		},
		"mis-indented draft": {
			input: "---\ntags:\n  - a\n    draft: true\n---\nBody.\n",
			want:  `recognized key "draft" must start at column 0`,
		},
		"empty inline list tail": {
			input: "---\ntags: [a, ]\n---\nBody.\n",
			want:  "list items must not be empty",
		},
		"empty inline list middle": {
			input: "---\ntags: [a, , b]\n---\nBody.\n",
			want:  "list items must not be empty",
		},
		"empty block list item": {
			input: "---\ntags:\n  - a\n  - \n---\nBody.\n",
			want:  "list items must not be empty",
		},
		"header block json": {
			input: "---\nheader:\n  - level: 1\n---\nBody.\n",
			want:  "header must be a compact single-line JSON array",
		},
		"toc columns empty json": {
			input: "---\ntoc_columns:\n---\nBody.\n",
			want:  "toc_columns must be a compact single-line JSON array",
		},
		"cr-only front matter": {
			input: "---\rtitle: CR\r---\rBody.\r",
			want:  "unsupported CR-only line endings",
		},
		"leading thematic break prose": {
			input: "---\nA thematic break.\n---\nBody.\n",
			want:  "a leading --- starts front matter",
		},
		"single quoted unterminated": {
			input: "---\ntitle: 'Tis the Season\n---\nBody.\n",
			want:  "value starts with ' so it is parsed as single-quoted",
		},
		"single quoted trailing junk": {
			input: "---\ntitle: 'Tis' the Season\n---\nBody.\n",
			want:  "trailing text after closing quote",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := parseContentFrontMatter([]byte(tc.input), name)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("parseContentFrontMatter error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidatePageLinksReportsDeterministically(t *testing.T) {
	err := validatePageLinks(PageLinks{Up: "Bad", Previous: "Still-Bad", Next: "Also-Bad"})
	if err == nil || !strings.Contains(err.Error(), "links.up") {
		t.Fatalf("validatePageLinks error = %v, want links.up first", err)
	}
}

func TestFrontMatterParserRegistryIsRecognizedKeySource(t *testing.T) {
	for _, key := range []string{"title", "summary", "published", "published_utc", "updated", "updated_utc", "draft", "main_spacer", "tags", "links", "header", "toc_columns"} {
		if _, ok := frontMatterParsers[key]; !ok {
			t.Fatalf("frontMatterParsers missing %q", key)
		}
		if !knownFrontMatterKey(key) {
			t.Fatalf("knownFrontMatterKey(%q) = false", key)
		}
	}
	for _, key := range []string{"up", "previous", "next", "related"} {
		if knownFrontMatterKey(key) {
			t.Fatalf("links subkey %q must not be a top-level front matter key", key)
		}
	}
}

func TestFrontMatterDateOnlyWorksWithDateHelpers(t *testing.T) {
	meta, _, err := parseContentFrontMatter([]byte("---\npublished: 2026-06-07\n---\nBody.\n"), "page.md")
	if err != nil {
		t.Fatalf("parseContentFrontMatter: %v", err)
	}
	if _, ok := parseOptionalTime(meta.PublishedUTC); !ok {
		t.Fatalf("parseOptionalTime(%q) = false", meta.PublishedUTC)
	}
	formatted, err := formatDate(meta.PublishedUTC, "2006-01-02")
	if err != nil {
		t.Fatalf("formatDate: %v", err)
	}
	if formatted != "2026-06-07" {
		t.Fatalf("formatted = %q", formatted)
	}
}

func TestLoadContentSourceLayout(t *testing.T) {
	dir := t.TempDir()
	writeText(t, filepath.Join(dir, "content", "pages", "about.md"), "About body.\n")
	writeText(t, filepath.Join(dir, "content", "pages", "upper.MD"), "Upper body.\n")
	writeText(t, filepath.Join(dir, "content", "pages", "plain.html"), "---\ntitle: \"Plain HTML\"\n---\n<p>Plain.</p>\n")
	writeText(t, filepath.Join(dir, "content", "pages", "capsule.gmi"), "---\ntitle: \"Καψούλα\"\n---\nGemtext body.\n")
	writeText(t, filepath.Join(dir, "content", "pages", "bundle", "index.MD"), "Bundle body.\n")
	writeText(t, filepath.Join(dir, "content", "pages", "bundle", "assets", "hero.png"), "not a real png")
	writeText(t, filepath.Join(dir, "content", "pages", "bundle", "child", "index.md"), "Nested source is an asset.\n")

	pages, posts, err := LoadContent(dir, SiteConfig{BaseURL: "https://example.org"})
	if err != nil {
		t.Fatalf("LoadContent: %v", err)
	}
	if len(posts) != 0 {
		t.Fatalf("posts = %#v", posts)
	}
	bySlug := map[string]Page{}
	for _, page := range pages {
		bySlug[page.Slug] = page
	}
	if bySlug["about"].Title != "About" || bySlug["about"].BodyFormat != bodyFormatMarkdownXHTML || string(bySlug["about"].body) != "About body.\n" {
		t.Fatalf("about page = %#v body %q", bySlug["about"], string(bySlug["about"].body))
	}
	if bySlug["upper"].Title != "Upper" || bySlug["upper"].BodyFormat != bodyFormatMarkdownXHTML || string(bySlug["upper"].body) != "Upper body.\n" {
		t.Fatalf("upper page = %#v body %q", bySlug["upper"], string(bySlug["upper"].body))
	}
	if bySlug["plain"].Title != "Plain HTML" || bySlug["plain"].BodyFormat != bodyFormatHTML {
		t.Fatalf("plain page = %#v", bySlug["plain"])
	}
	if bySlug["capsule"].Title != "Καψούλα" || bySlug["capsule"].BodyFormat != bodyFormatGemtext {
		t.Fatalf("capsule page = %#v", bySlug["capsule"])
	}
	if bySlug["bundle"].Title != "Bundle" || bySlug["bundle"].BodyFormat != bodyFormatMarkdownXHTML || string(bySlug["bundle"].body) != "Bundle body.\n" {
		t.Fatalf("bundle page = %#v body %q", bySlug["bundle"], string(bySlug["bundle"].body))
	}
}

func TestLoadContentSourceLayoutErrors(t *testing.T) {
	tests := map[string]struct {
		setup func(string)
		want  string
	}{
		"invalid slug": {
			setup: func(dir string) {
				writeText(t, filepath.Join(dir, "content", "pages", "Bad.md"), "Body.\n")
			},
			want: "invalid slug",
		},
		"unsupported visible root file": {
			setup: func(dir string) {
				writeText(t, filepath.Join(dir, "content", "pages", "notes.txt"), "Body.\n")
			},
			want: "unsupported content file",
		},
		"flat bundle collision": {
			setup: func(dir string) {
				writeText(t, filepath.Join(dir, "content", "pages", "about.md"), "Flat.\n")
				writeText(t, filepath.Join(dir, "content", "pages", "about", "index.md"), "Bundle.\n")
			},
			want: "content slug collision",
		},
		"missing bundle index": {
			setup: func(dir string) {
				writeText(t, filepath.Join(dir, "content", "pages", "about", "assets", "hero.png"), "asset\n")
			},
			want: "content bundle missing index source",
		},
		"multiple bundle indexes": {
			setup: func(dir string) {
				writeText(t, filepath.Join(dir, "content", "pages", "about", "index.md"), "Markdown.\n")
				writeText(t, filepath.Join(dir, "content", "pages", "about", "index.html"), "<p>HTML.</p>\n")
			},
			want: "content bundle has multiple index sources",
		},
		"extra bundle source": {
			setup: func(dir string) {
				writeText(t, filepath.Join(dir, "content", "pages", "about", "index.md"), "Index.\n")
				writeText(t, filepath.Join(dir, "content", "pages", "about", "notes.md"), "Notes.\n")
			},
			want: "content bundle contains non-index source file",
		},
		"uppercase slug remains invalid": {
			setup: func(dir string) {
				writeText(t, filepath.Join(dir, "content", "pages", "About.MD"), "About.\n")
			},
			want: "invalid slug: About",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(dir)
			_, _, err := LoadContent(dir, SiteConfig{BaseURL: "https://example.org"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("LoadContent error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestBundleIndexPathRejectsCaseVariantIndexesWhenFilesystemAllows(t *testing.T) {
	dir := t.TempDir()
	bundleDir := filepath.Join(dir, "content", "pages", "about")
	writeText(t, filepath.Join(bundleDir, "index.md"), "Markdown.\n")
	writeText(t, filepath.Join(bundleDir, "index.MD"), "Markdown uppercase.\n")
	entries, err := os.ReadDir(bundleDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	sourceCount := 0
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), "index.md") {
			sourceCount++
		}
	}
	if sourceCount < 2 {
		t.Skip("filesystem does not allow case-variant filenames in one directory")
	}
	_, err = bundleIndexPath(bundleDir)
	if err == nil || !strings.Contains(err.Error(), "content bundle has multiple index sources") {
		t.Fatalf("bundleIndexPath error = %v, want multiple index sources", err)
	}
}
