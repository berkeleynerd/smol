package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var eventHandlerRE = regexp.MustCompile(`(?i)(^|[^a-zA-Z0-9_-])on[a-zA-Z0-9_-]*\s*=`)
var externalLinkRE = regexp.MustCompile(`(?is)<link\s+[^>]*rel\s*=\s*['"]?(stylesheet|preload|modulepreload|prefetch|preconnect)\b`)
var imgTagRE = regexp.MustCompile(`(?is)<img\b[^>]*>`)
var imgSrcAttrRE = regexp.MustCompile(`(?is)\bsrc\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
var imgSrcsetAttrRE = regexp.MustCompile(`(?is)\bsrcset\s*=`)
var checkboxIDRE = regexp.MustCompile(`^cb[0-9]+$`)
var cssEscapeRE = regexp.MustCompile(`\\(?:[0-9a-fA-F]{1,6}[\t\n\r\f ]?|\r\n|[\s\S])`)
var cssResourceFunctionRE = regexp.MustCompile(`(?i)(?:url|image|image-set|-webkit-image-set|src|paint|element)\s*\(`)
var externalSVGRefRE = regexp.MustCompile(`(?is)<(use|image)\b[^>]*(?:href|xlink:href)\s*=\s*['"]?(https?:|//)`)
var cssCommentRE = regexp.MustCompile(`(?s)/\*.*?(?:\*/|\z)`)
var cssBehaviorPropertyRE = regexp.MustCompile(`(?i)(?:^|[{;\s])[*_]?behavior\s*:`)

func ValidateHTML(html string) error {
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
	return validateXML(html, strings.Contains(lower, "<html"))
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
	if strings.Contains(strings.ToLower(css), "</style") {
		return fmt.Errorf("CSS cannot close the HTML style boundary")
	}
	if strings.Contains(css, "]]>") {
		return fmt.Errorf("CSS cannot close the XML CDATA boundary")
	}
	scan := cssCommentRE.ReplaceAllString(css, " ")
	scan = cssEscapeRE.ReplaceAllStringFunc(scan, func(escape string) string {
		value := strings.TrimSpace(escape[1:])
		if n, err := strconv.ParseInt(value, 16, 32); err == nil && len(value) <= 6 {
			return string(rune(n))
		}
		return strings.TrimLeft(escape[1:], "\r\n\f")
	})
	lower := strings.ToLower(scan)
	if cssResourceFunctionRE.MatchString(lower) {
		return fmt.Errorf("CSS is not supported: found resource-loading function")
	}
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
