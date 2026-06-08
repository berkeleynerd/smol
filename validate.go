package main

import (
	"fmt"
	"regexp"
	"strings"
)

var eventHandlerRE = regexp.MustCompile(`(?i)(^|[^a-zA-Z0-9_-])on[a-zA-Z0-9_-]*\s*=`)
var externalLinkRE = regexp.MustCompile(`(?is)<link\s+[^>]*rel\s*=\s*['"]?(stylesheet|preload|modulepreload|prefetch|preconnect)\b`)
var externalImgRE = regexp.MustCompile(`(?is)<img\s+[^>]*src\s*=\s*['"]https?:`)
var canonicalCheckboxRE = regexp.MustCompile(`<input type="checkbox" id="cb[0-9]+" />`)
var externalSVGRefRE = regexp.MustCompile(`(?is)<(use|image)\b[^>]*(?:href|xlink:href)\s*=\s*['"]?(https?:|//)`)

func ValidateHTML(html string) error {
	return ValidateHTMLForMode(html, "")
}

func ValidateHTMLForMode(html, outputMode string) error {
	jsTokens := []string{"<script", "</script", "javascript:"}
	lower := strings.ToLower(html)
	for _, token := range jsTokens {
		if strings.Contains(lower, token) {
			return fmt.Errorf("JavaScript is not supported: found %s", token)
		}
	}
	blocked := []string{"<iframe", "<object", "<embed", "<form", "<button", "<textarea", "<select"}
	for _, token := range blocked {
		if strings.Contains(lower, token) {
			return fmt.Errorf("interactive HTML is not supported: found %s", token)
		}
	}
	if strings.Contains(lower, "<input") {
		if outputMode != outputModeFlatXHTML {
			return fmt.Errorf("interactive HTML is not supported: found <input")
		}
		withoutCheckboxes := canonicalCheckboxRE.ReplaceAllString(html, "")
		if strings.Contains(strings.ToLower(withoutCheckboxes), "<input") {
			return fmt.Errorf("interactive HTML is not supported: found non-canonical <input")
		}
	}
	if strings.Contains(lower, "<svg") && outputMode != outputModeFlatXHTML {
		return fmt.Errorf("inline SVG is not supported: found <svg")
	}
	if strings.Contains(lower, `<meta http-equiv="refresh"`) {
		return fmt.Errorf(`automatic navigation is not supported: found <meta http-equiv="refresh"`)
	}
	if eventHandlerRE.MatchString(html) {
		return fmt.Errorf("JavaScript is not supported: found event handler attribute")
	}
	if m := externalLinkRE.FindString(html); m != "" {
		return fmt.Errorf("external resources are not supported: found %s", compactTag(m))
	}
	if externalImgRE.MatchString(html) {
		return fmt.Errorf(`external resources are not supported: found <img src="http:">`)
	}
	if externalSVGRefRE.MatchString(html) {
		return fmt.Errorf("external resources are not supported: found external SVG reference")
	}
	for _, token := range []string{"<audio", "<video", "<source"} {
		if strings.Contains(lower, token) {
			return fmt.Errorf("external resources are not supported: found %s", token)
		}
	}
	return nil
}

func ValidateCSS(css string) error {
	lower := strings.ToLower(css)
	for _, token := range []string{"@import", "url(", "expression(", "behavior:", "-moz-binding"} {
		if strings.Contains(lower, token) {
			return fmt.Errorf("CSS is not supported: found %s", token)
		}
	}
	return nil
}

func compactTag(tag string) string {
	tag = strings.Join(strings.Fields(tag), " ")
	lower := strings.ToLower(tag)
	if strings.Contains(lower, "stylesheet") {
		return `<link rel="stylesheet">`
	}
	if strings.Contains(lower, "modulepreload") {
		return `<link rel="modulepreload">`
	}
	if strings.Contains(lower, "preload") {
		return `<link rel="preload">`
	}
	if strings.Contains(lower, "prefetch") {
		return `<link rel="prefetch">`
	}
	if strings.Contains(lower, "preconnect") {
		return `<link rel="preconnect">`
	}
	return tag
}
