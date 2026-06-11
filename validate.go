package main

import (
	"fmt"
	"regexp"
	"strings"
)

var eventHandlerRE = regexp.MustCompile(`(?i)(^|[^a-zA-Z0-9_-])on[a-zA-Z0-9_-]*\s*=`)
var externalLinkRE = regexp.MustCompile(`(?is)<link\s+[^>]*rel\s*=\s*['"]?(stylesheet|preload|modulepreload|prefetch|preconnect)\b`)
var imgTagRE = regexp.MustCompile(`(?is)<img\b[^>]*>`)
var imgSrcAttrRE = regexp.MustCompile(`(?is)\bsrc\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
var imgSrcsetAttrRE = regexp.MustCompile(`(?is)\bsrcset\s*=`)
var canonicalCheckboxRE = regexp.MustCompile(`<input type="checkbox" id="cb[0-9]+" />`)
var externalSVGRefRE = regexp.MustCompile(`(?is)<(use|image)\b[^>]*(?:href|xlink:href)\s*=\s*['"]?(https?:|//)`)
var cssCommentRE = regexp.MustCompile(`(?s)/\*.*?(?:\*/|\z)`)
var cssBehaviorPropertyRE = regexp.MustCompile(`(?i)(?:^|[{;\s])behavior\s*:`)

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
	if err := validateEmbeddedImages(html); err != nil {
		return err
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

func validateEmbeddedImages(html string) error {
	for _, tag := range imgTagRE.FindAllString(html, -1) {
		if imgSrcsetAttrRE.MatchString(tag) {
			return fmt.Errorf("external resources are not supported: found <img srcset>")
		}
		src := imageSrc(tag)
		if src == "" || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(src)), "data:") {
			return fmt.Errorf("external resources are not supported: found non-embedded <img src>")
		}
	}
	return nil
}

func imageSrc(tag string) string {
	match := imgSrcAttrRE.FindStringSubmatch(tag)
	if match == nil {
		return ""
	}
	for _, value := range match[1:] {
		if value != "" {
			return value
		}
	}
	return ""
}

func ValidateCSS(css string) error {
	// CSS comments are inert in the browser, so strip them before scanning;
	// otherwise a provenance note such as "no url() assets" would trip the
	// substring checks below even though it declares nothing. Comments are
	// replaced with a space rather than removed so they cannot splice two
	// fragments into a token that the browser would never have parsed.
	scan := cssCommentRE.ReplaceAllString(css, " ")
	lower := strings.ToLower(scan)
	for _, token := range []string{"@import", "url(", "expression(", "-moz-binding"} {
		if strings.Contains(lower, token) {
			return fmt.Errorf("CSS is not supported: found %s", token)
		}
	}
	// Block the legacy IE `behavior:` HTC property, but only where it appears
	// as a property name at a declaration boundary, so the standard
	// scroll-behavior / overscroll-behavior properties remain allowed.
	if cssBehaviorPropertyRE.MatchString(scan) {
		return fmt.Errorf("CSS is not supported: found behavior:")
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
