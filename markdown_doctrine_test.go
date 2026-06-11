package main

import (
	"strings"
	"testing"
)

func TestValidateMarkdownSourceRejectsHTMLEntities(t *testing.T) {
	cases := map[string]struct {
		body string
		line string
	}{
		"named nbsp":    {"a&nbsp;b", "line 1"},
		"named amp":     {"Tom &amp; Jerry", "line 1"},
		"decimal":       {"x&#160;y", "line 1"},
		"hex":           {"x&#xA0;y", "line 1"},
		"on third line": {"one\ntwo\nthree&copy;", "line 3"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateMarkdownSource(tc.body, 1)
			if err == nil || !strings.Contains(err.Error(), "HTML character entities are not supported") {
				t.Fatalf("expected entity rejection for %q, got %v", tc.body, err)
			}
			if !strings.Contains(err.Error(), tc.line) {
				t.Fatalf("expected %q in error for %q, got %v", tc.line, tc.body, err)
			}
		})
	}
}

func TestValidateMarkdownSourceRejectsHTMLElements(t *testing.T) {
	cases := map[string]string{
		"open tag":    "<section>",
		"close tag":   "text </p> more",
		"comment":     "<!-- hi -->",
		"void tag":    "<br/>",
		"on line two": "ok\n<div>",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			err := ValidateMarkdownSource(body, 1)
			if err == nil || !strings.Contains(err.Error(), "embedded HTML is not supported") {
				t.Fatalf("expected HTML rejection for %q, got %v", body, err)
			}
		})
	}
}

func TestValidateMarkdownSourceAllowsPureMarkdown(t *testing.T) {
	cases := map[string]string{
		"bare ampersand":        "Tom & Jerry and AT&T",
		"less-than with space":  "if a < b then",
		"less-than digit":       "I <3 markdown",
		"trailing less-than":    "ends with <",
		"backslash escape dot":  `1\. not a list`,
		"backslash escape star": `\*literal\*`,
		"entity in code span":   "use `&nbsp;` for a space",
		"element in code span":  "the `<div>` element",
		"html in fenced code":   "```\n<div>&nbsp;</div>\n```",
		"ampersand in url":      "[x](http://example.org/?a=1&b=2)",
		"heading and list":      "## Title\n\n- one\n- two\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateMarkdownSource(body, 1); err != nil {
				t.Fatalf("rejected pure markdown %q: %v", body, err)
			}
		})
	}
}

func TestValidateMarkdownSourceReportsAbsoluteLine(t *testing.T) {
	// Body's first line maps to file line 7; the entity is on the body's third
	// line, so the report must read file line 9 (7 + 2).
	err := ValidateMarkdownSource("first\nsecond\nthird &nbsp; here", 7)
	if err == nil || !strings.Contains(err.Error(), "line 9") {
		t.Fatalf("expected absolute file line 9, got %v", err)
	}
}

func TestBuildRejectsHTMLEntityInMarkdownBody(t *testing.T) {
	dir := testSite(t)
	writeMarkdownIndex(t, dir, "Plain opening line.\n\nA spaced&nbsp;word breaks the doctrine.\n")
	err := BuildSite(BuildOptions{SiteDir: dir})
	if err == nil || !strings.Contains(err.Error(), "HTML character entities are not supported") {
		t.Fatalf("build accepted an HTML entity: %v", err)
	}
}
