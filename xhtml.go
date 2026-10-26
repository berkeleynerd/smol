package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

const xhtmlNamespace = "http://www.w3.org/1999/xhtml"
const svgNamespace = "http://www.w3.org/2000/svg"

var staticElements = wordSet("html head title meta link style body header footer main nav article section aside address h1 h2 h3 h4 h5 h6 hgroup p div span a abbr b bdi bdo blockquote br cite code data dd del dfn dl dt em figcaption figure hr i img ins kbd li mark ol pre q rp rt ruby s samp small strong sub sup table caption colgroup col tbody td tfoot th thead tr time u ul var wbr label input details summary")
var svgElements = wordSet("svg g path rect circle ellipse line polyline polygon text tspan title desc defs symbol use clipPath mask linearGradient radialGradient stop pattern image")
var ordinaryAttributes = wordSet("id class title lang dir role hidden tabindex accesskey href rel target download hreflang type media charset name content http-equiv alt width height span abbr loading decoding for start reversed value colspan rowspan scope headers datetime cite open viewBox preserveAspectRatio d x y x1 x2 y1 y2 cx cy r rx ry points fill fill-rule fill-opacity stroke stroke-width stroke-linecap stroke-linejoin stroke-opacity opacity transform offset stop-color stop-opacity font-size font-family text-anchor version")
var literalAttributeRE = regexp.MustCompile(`=\s*(?:"([^"]*)"|'([^']*)')`)
var voidElements = wordSet("img meta link br hr input col wbr")
var headElements = wordSet("title meta link style")

func wordSet(words string) map[string]bool {
	set := map[string]bool{}
	for _, word := range strings.Fields(words) {
		set[word] = true
	}
	return set
}

// ValidateXHTMLDocument is the publication boundary. Unlike fragment validation,
// it requires exactly one XHTML html root, containing head followed by body.
// No external DTD, entity declarations, or processing instructions are accepted.
func ValidateXHTMLDocument(document string) error { return validateXML(document, true) }

func validateXML(document string, full bool) error {
	input := document
	if !full {
		input = `<smol-fragment xmlns="` + xhtmlNamespace + `">` + document + `</smol-fragment>`
	}
	d := xml.NewDecoder(strings.NewReader(input))
	var stack []xml.Name
	var style strings.Builder
	roots, heads, bodies, doctypes := 0, 0, 0, 0
	for {
		start := d.InputOffset()
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("XHTML must be well-formed XML: %w", err)
		}
		switch t := token.(type) {
		case xml.Directive:
			doctypes++
			if !full || strings.TrimSpace(string(t)) != "DOCTYPE html" || roots != 0 || doctypes != 1 {
				return fmt.Errorf("only <!DOCTYPE html> is allowed; external DTDs and entities are forbidden")
			}
		case xml.ProcInst:
			return fmt.Errorf("XML processing instructions and declarations are not supported in HTML-compatible XHTML5")
		case xml.StartElement:
			for _, match := range literalAttributeRE.FindAllStringSubmatch(input[start:d.InputOffset()], -1) {
				if strings.ContainsAny(match[1]+match[2], "\t\n\r") {
					return fmt.Errorf("literal attribute whitespace differs under XML and HTML; use character references")
				}
			}
			if len(stack) == 0 {
				roots++
				if full && (roots != 1 || t.Name.Local != "html" || t.Name.Space != xhtmlNamespace) {
					return fmt.Errorf("document must have one html root in the XHTML namespace")
				}
			}
			if !full && len(stack) == 0 {
				stack = append(stack, t.Name)
				continue
			}
			inSVG := t.Name.Space == svgNamespace
			if t.Name.Space != xhtmlNamespace && !inSVG {
				return fmt.Errorf("unsupported element namespace %q", t.Name.Space)
			}
			if (!inSVG && !staticElements[t.Name.Local]) || (inSVG && !svgElements[t.Name.Local]) {
				return fmt.Errorf("unsupported static element <%s>", t.Name.Local)
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1].Local
				if voidElements[parent] || parent == "title" || parent == "style" {
					return fmt.Errorf("element <%s> cannot contain child elements", parent)
				}
				if full && parent == "html" {
					switch t.Name.Local {
					case "head":
						heads++
						if bodies != 0 {
							return fmt.Errorf("head must precede body")
						}
					case "body":
						bodies++
					default:
						return fmt.Errorf("html may contain only head and body")
					}
				}
				if full && parent == "head" && !headElements[t.Name.Local] {
					return fmt.Errorf("unsupported element in head")
				}
			}
			attrs := map[xml.Name]bool{}
			values := map[string]string{}
			for _, attr := range t.Attr {
				if attrs[attr.Name] {
					return fmt.Errorf("duplicate XML attribute %s", attr.Name.Local)
				}
				attrs[attr.Name] = true
				if attr.Name.Space == "xmlns" || (attr.Name.Space == "" && attr.Name.Local == "xmlns") {
					if attr.Name.Space == "xmlns" && (attr.Name.Local != "xlink" || attr.Value != "http://www.w3.org/1999/xlink" || !inSVG) {
						return fmt.Errorf("unsupported namespace declaration")
					}
					if attr.Name.Space == "" && attr.Value != t.Name.Space {
						return fmt.Errorf("namespace declaration must match the element namespace")
					}
					continue
				}
				if attr.Name.Space == "http://www.w3.org/XML/1998/namespace" && attr.Name.Local == "lang" {
					continue
				}
				if attr.Name.Space != "" && !(inSVG && attr.Name.Space == "http://www.w3.org/1999/xlink" && attr.Name.Local == "href") {
					return fmt.Errorf("unsupported attribute namespace %s", attr.Name.Space)
				}
				name := attr.Name.Local
				values[name] = attr.Value
				if name == "style" {
					if err := ValidateCSS(attr.Value); err != nil {
						return fmt.Errorf("inline style attribute: %w", err)
					}
					continue
				}
				if name == "src" {
					if t.Name.Local != "img" || !embeddedRaster(attr.Value) {
						return fmt.Errorf("external resources are not supported: found non-embedded <img src>")
					}
					continue
				}
				if !ordinaryAttributes[name] && !strings.HasPrefix(name, "aria-") && !strings.HasPrefix(name, "data-") {
					return fmt.Errorf("unsupported static attribute %s", name)
				}
				if name == "href" {
					if inSVG {
						if t.Name.Local == "image" && !embeddedRaster(attr.Value) || t.Name.Local != "image" && !strings.HasPrefix(attr.Value, "#") {
							return fmt.Errorf("external SVG references are not supported")
						}
					} else if t.Name.Local != "a" && t.Name.Local != "link" {
						return fmt.Errorf("href is only allowed on navigation links")
					} else if err := staticLink(attr.Value); err != nil {
						return err
					}
				}
				if inSVG {
					if err := ValidateCSS(attr.Value); err != nil {
						return err
					}
				}
			}
			if t.Name.Local == "link" && values["rel"] != "canonical" {
				return fmt.Errorf("only canonical metadata links are allowed")
			}
			if t.Name.Local == "meta" && values["http-equiv"] != "" && strings.ToLower(values["http-equiv"]) != "content-type" {
				return fmt.Errorf("meta http-equiv is not supported")
			}
			if t.Name.Local == "img" && !embeddedRaster(values["src"]) {
				return fmt.Errorf("img requires an embedded raster src")
			}
			if inSVG && t.Name.Local == "image" && !embeddedRaster(values["href"]) {
				return fmt.Errorf("SVG image requires an embedded raster href")
			}
			if t.Name.Local == "input" && (values["type"] != "checkbox" || !checkboxIDRE.MatchString(values["id"]) || len(values) != 2) {
				return fmt.Errorf("only inert checkbox inputs are supported")
			}
			if t.Name.Local == "style" {
				style.Reset()
			}
			stack = append(stack, t.Name)
		case xml.CharData:
			if len(stack) == 0 && strings.TrimSpace(string(t)) != "" {
				return fmt.Errorf("text outside XHTML root")
			}
			if len(stack) > 0 && stack[len(stack)-1].Local == "style" {
				style.Write(t)
			}
			if len(stack) > 0 && voidElements[stack[len(stack)-1].Local] && strings.TrimSpace(string(t)) != "" {
				return fmt.Errorf("void elements cannot contain text")
			}
		case xml.EndElement:
			if t.Name.Local == "style" {
				if err := ValidateCSS(style.String()); err != nil {
					return fmt.Errorf("inline style: %w", err)
				}
			}
			stack = stack[:len(stack)-1]
		}
	}
	if full && (roots != 1 || heads != 1 || bodies != 1) {
		return fmt.Errorf("XHTML document requires one html root, one head, and one body")
	}
	if full && doctypes != 1 {
		return fmt.Errorf("XHTML5 requires <!DOCTYPE html>")
	}
	return validateHTMLStructure(document, full)
}

func embeddedRaster(value string) bool {
	lower := strings.ToLower(value)
	for _, mime := range []string{"png", "jpeg", "gif", "webp", "avif"} {
		if strings.HasPrefix(lower, "data:image/"+mime+";base64,") {
			return true
		}
	}
	return false
}

func staticLink(value string) error {
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("control character in link")
	}
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("invalid link: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "", "https", "http", "gemini", "mailto", "tel":
		return nil
	}
	return fmt.Errorf("unsupported link scheme %q", u.Scheme)
}
