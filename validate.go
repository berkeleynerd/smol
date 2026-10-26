package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
)

var eventHandlerRE = regexp.MustCompile(`(?i)(^|[^a-zA-Z0-9_-])on[a-zA-Z0-9_-]*\s*=`)
var externalLinkRE = regexp.MustCompile(`(?is)<link\s+[^>]*rel\s*=\s*['"]?(stylesheet|preload|modulepreload|prefetch|preconnect)\b`)
var imgTagRE = regexp.MustCompile(`(?is)<img\b[^>]*>`)
var imgSrcAttrRE = regexp.MustCompile(`(?is)\bsrc\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
var imgSrcsetAttrRE = regexp.MustCompile(`(?is)\bsrcset\s*=`)
var checkboxIDRE = regexp.MustCompile(`^cb[0-9]+$`)
var externalSVGRefRE = regexp.MustCompile(`(?is)<(use|image)\b[^>]*(?:href|xlink:href)\s*=\s*['"]?(https?:|//)`)
var cssInputNormalization = strings.NewReplacer("\r\n", "\n", "\r", "\n", "\f", "\n", "\x00", "\ufffd")

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

func ValidateCSS(text string) error {
	// These bytes can terminate the containing HTML/XML style boundary even
	// inside a CSS string. Tokenization must never rewrite the signed content.
	if strings.Contains(strings.ToLower(text), "</style") {
		return fmt.Errorf("CSS cannot close the HTML style boundary")
	}
	if strings.Contains(text, "]]>") {
		return fmt.Errorf("CSS cannot close the XML CDATA boundary")
	}
	// CSS Syntax preprocesses line endings before tokenization. In particular,
	// CRLF is one whitespace character terminating a hexadecimal escape. This
	// is a scanning view only; the original source is rendered and signed.
	lexer := css.NewLexer(parse.NewInputString(cssInputNormalization.Replace(text)))
	previousIdent := ""
	for {
		token, data := lexer.Next()
		switch token {
		case css.ErrorToken:
			if lexer.Err() == io.EOF {
				return nil
			}
			return fmt.Errorf("invalid CSS: %w", lexer.Err())
		case css.WhitespaceToken, css.CommentToken:
			continue
		case css.BadStringToken, css.BadURLToken:
			return fmt.Errorf("invalid CSS: %s", token)
		case css.URLToken:
			return fmt.Errorf("CSS is not supported: found resource-loading function url(")
		case css.FunctionToken:
			name := decodeCSSIdentifier(data[:len(data)-1])
			switch name {
			case "url", "image", "image-set", "-webkit-image-set", "src", "paint", "element", "expression":
				return fmt.Errorf("CSS is not supported: found resource-loading function %s(", name)
			}
		case css.AtKeywordToken:
			if decodeCSSIdentifier(data[1:]) == "import" {
				return fmt.Errorf("CSS is not supported: found @import")
			}
		case css.IdentToken:
			previousIdent = decodeCSSIdentifier(data)
			if previousIdent == "-moz-binding" {
				return fmt.Errorf("CSS is not supported: found -moz-binding")
			}
			continue
		case css.ColonToken:
			if previousIdent == "behavior" || previousIdent == "_behavior" {
				return fmt.Errorf("CSS is not supported: found behavior:")
			}
		}
		previousIdent = ""
	}
}

// Decode escapes only inside identifier tokens. Decoding before tokenization
// would change the meaning of quoted strings, comments, and escaped delimiters.
func decodeCSSIdentifier(data []byte) string {
	var value strings.Builder
	for i := 0; i < len(data); i++ {
		if data[i] != '\\' {
			value.WriteByte(data[i])
			continue
		}
		i++
		var codepoint rune
		digits := 0
		for i < len(data) && digits < 6 {
			var digit byte
			switch c := data[i]; {
			case c >= '0' && c <= '9':
				digit = c - '0'
			case c >= 'a' && c <= 'f':
				digit = c - 'a' + 10
			case c >= 'A' && c <= 'F':
				digit = c - 'A' + 10
			default:
				goto decoded
			}
			codepoint = codepoint*16 + rune(digit)
			digits++
			i++
		}
	decoded:
		if digits == 0 {
			if i < len(data) {
				value.WriteByte(data[i])
			}
			continue
		}
		if codepoint == 0 || !utf8.ValidRune(codepoint) {
			codepoint = utf8.RuneError
		}
		value.WriteRune(codepoint)
		if i < len(data) && strings.ContainsRune(" \t\r\n\f", rune(data[i])) {
			if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
				i++
			}
		} else {
			i--
		}
	}
	return strings.ToLower(value.String())
}

// Compare the same token stream under HTML and XML parsing, ignoring only
// actual CSS comments (not comment delimiters inside quoted strings).
func cssStructure(text string) string {
	lexer := css.NewLexer(parse.NewInputString(cssInputNormalization.Replace(strings.TrimSpace(text))))
	var shape strings.Builder
	for {
		token, data := lexer.Next()
		if token == css.ErrorToken {
			return shape.String()
		}
		if token != css.CommentToken {
			fmt.Fprintf(&shape, "%d:%q;", token, data)
		}
	}
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
