package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The static profile uses the common HTML/XML syntax. XML well-formedness is
// necessary but insufficient: HTML must not repair or reinterpret its tree.
func validateHTMLStructure(document string, full bool) error {
	input := document
	if !full {
		input = `<div xmlns="` + xhtmlNamespace + `">` + input + `</div>`
	}
	xmlTree := &html.Node{Type: html.DocumentNode}
	current := xmlTree
	decoder := xml.NewDecoder(strings.NewReader(input))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err // The strict XML/static-policy pass normally catches this.
		}
		switch t := token.(type) {
		case xml.StartElement:
			node := &html.Node{Type: html.ElementNode, Data: t.Name.Local}
			if t.Name.Space == svgNamespace {
				node.Namespace = "svg"
			}
			for _, attr := range t.Attr {
				namespace := attr.Name.Space
				switch namespace {
				case "http://www.w3.org/XML/1998/namespace":
					namespace = "xml"
				case "http://www.w3.org/1999/xlink":
					namespace = "xlink"
				}
				// HTML keeps xml:lang as an unnamespaced qualified name on
				// HTML elements, but recognizes it in foreign SVG content.
				key := attr.Name.Local
				if namespace == "xml" && node.Namespace == "" {
					key, namespace = "xml:"+key, ""
				}
				node.Attr = append(node.Attr, html.Attribute{Namespace: namespace, Key: key, Val: attr.Value})
			}
			current.AppendChild(node)
			current = node
		case xml.CharData:
			if current.LastChild != nil && current.LastChild.Type == html.TextNode {
				current.LastChild.Data += string(t)
			} else {
				current.AppendChild(&html.Node{Type: html.TextNode, Data: string(t)})
			}
		case xml.Comment:
			current.AppendChild(&html.Node{Type: html.CommentNode, Data: string(t)})
		case xml.EndElement:
			current = current.Parent
		}
	}
	var htmlTree *html.Node
	if full {
		var err error
		htmlTree, err = html.Parse(strings.NewReader(document))
		if err != nil {
			return fmt.Errorf("HTML parse: %w", err)
		}
	} else {
		context := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
		children, err := html.ParseFragment(strings.NewReader(document), context)
		if err != nil {
			return fmt.Errorf("HTML fragment parse: %w", err)
		}
		htmlTree = &html.Node{Type: html.DocumentNode}
		root := &html.Node{Type: html.ElementNode, Data: "div", Attr: []html.Attribute{{Key: "xmlns", Val: xhtmlNamespace}}}
		htmlTree.AppendChild(root)
		for _, child := range children {
			root.AppendChild(child)
		}
	}
	if !reflect.DeepEqual(treeShape(xmlTree), treeShape(htmlTree)) {
		return fmt.Errorf("XHTML5 must have the same structure and text when parsed as XML and HTML; check nesting, explicit end tags, and entity/CDATA use")
	}
	return validateContentModel(xmlTree, full)
}

type nodeShape struct {
	Name, Text string
	Attrs      []string
	Children   []nodeShape
}

func treeShape(node *html.Node) nodeShape {
	shape := nodeShape{Name: node.Namespace + ":" + node.Data}
	if node.Type == html.TextNode {
		shape.Name, shape.Text = "#text", node.Data
		return shape
	}
	for _, attr := range node.Attr {
		shape.Attrs = append(shape.Attrs, attr.Namespace+":"+attr.Key+"="+attr.Val)
	}
	sort.Strings(shape.Attrs)
	if node.Namespace == "" && node.Data == "style" {
		var css strings.Builder
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			css.WriteString(child.Data)
		}
		// The XML CDATA boundaries are inside CSS comments for text/html.
		// Comments differ between parsers, but the effective CSS must match.
		shape.Text = strings.TrimSpace(cssCommentRE.ReplaceAllString(css.String(), " "))
		return shape
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode && child.Type != html.TextNode {
			continue
		}
		if child.Type == html.TextNode && strings.TrimSpace(child.Data) == "" {
			continue // HTML places trailing document whitespace in body.
		}
		sub := treeShape(child)
		if len(shape.Children) > 0 && sub.Name == "#text" && shape.Children[len(shape.Children)-1].Name == "#text" {
			shape.Children[len(shape.Children)-1].Text += sub.Text
		} else {
			shape.Children = append(shape.Children, sub)
		}
	}
	return shape
}

var phrasingElements = wordSet("a abbr b bdi bdo br cite code data del dfn em i img input ins kbd label mark q ruby rp rt s samp small span strong sub sup time u var wbr")
var phrasingParents = wordSet("p h1 h2 h3 h4 h5 h6 abbr b bdi bdo cite code data dfn em i kbd label mark pre q rt s samp small span strong sub sup time u var")
var headingElements = wordSet("h1 h2 h3 h4 h5 h6")
var sectioningElements = wordSet("article aside nav section")
var tableSections = wordSet("caption colgroup thead tbody tfoot")
var elementOnlyParents = wordSet("html head ul ol dl table colgroup thead tbody tfoot tr")
var mainExcludedAncestors = wordSet("main article aside nav header footer")
var requiredParents = map[string]map[string]bool{
	"head": wordSet("html"), "body": wordSet("html"), "title": wordSet("head"), "meta": wordSet("head"), "link": wordSet("head"), "style": wordSet("head"),
	"li": wordSet("ul ol"), "dt": wordSet("dl"), "dd": wordSet("dl"), "caption": wordSet("table"), "colgroup": wordSet("table"), "thead": wordSet("table"), "tbody": wordSet("table"), "tfoot": wordSet("table"),
	"tr": wordSet("thead tbody tfoot"), "td": wordSet("tr"), "th": wordSet("tr"), "col": wordSet("colgroup"), "figcaption": wordSet("figure"), "summary": wordSet("details"), "rt": wordSet("ruby"), "rp": wordSet("ruby"),
}
var tableSectionRank = map[string]int{"caption": 0, "colgroup": 1, "thead": 2, "tbody": 3, "tfoot": 4}

func attrValue(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func elementChildren(node *html.Node) []*html.Node {
	var out []*html.Node
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			out = append(out, child)
		}
	}
	return out
}

// A deliberately constrained static HTML content model. The HTML parser alone
// accepts nonconforming combinations such as a div inside a span or a p in a ul.
func validateContentModel(root *html.Node, full bool) error {
	ids := map[string]bool{}
	var walk func(*html.Node) error
	walk = func(node *html.Node) error {
		if node.Type != html.ElementNode {
			for _, child := range elementChildren(node) {
				if err := walk(child); err != nil {
					return err
				}
			}
			return nil
		}
		name := node.Data
		children := elementChildren(node)
		if id := attrValue(node, "id"); id != "" {
			if ids[id] || strings.ContainsAny(id, " \t\n\r\f") {
				return fmt.Errorf("HTML id must be unique and contain no whitespace: %q", id)
			}
			ids[id] = true
		}
		if node.Namespace == "" {
			if err := validateAttributes(node); err != nil {
				return err
			}
			for _, child := range children {
				if phrasingParents[name] && !(phrasingElements[child.Data] && child.Namespace == "" || child.Namespace == "svg" && child.Data == "svg") {
					return fmt.Errorf("<%s> permits only phrasing content, found <%s>", name, child.Data)
				}
				allowed := true
				switch name {
				case "html":
					allowed = child.Data == "head" || child.Data == "body"
				case "head":
					allowed = headElements[child.Data]
				case "ul", "ol":
					allowed = child.Data == "li"
				case "dl":
					allowed = child.Data == "dt" || child.Data == "dd"
				case "table":
					allowed = tableSections[child.Data]
				case "thead", "tbody", "tfoot":
					allowed = child.Data == "tr"
				case "tr":
					allowed = child.Data == "td" || child.Data == "th"
				case "colgroup":
					allowed = child.Data == "col"
				case "hgroup":
					allowed = child.Data == "p" || headingElements[child.Data]
				case "ruby":
					allowed = phrasingElements[child.Data]
				case "rp", "title", "style":
					allowed = false
				}
				if !allowed {
					return fmt.Errorf("unsupported HTML nesting: <%s> inside <%s>", child.Data, name)
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				if child.Type == html.TextNode && strings.TrimSpace(child.Data) != "" && elementOnlyParents[name] {
					return fmt.Errorf("text is not allowed directly inside <%s>", name)
				}
			}
			if full && node.Parent != nil {
				parent := node.Parent.Data
				if parents, ok := requiredParents[name]; ok && !parents[parent] {
					return fmt.Errorf("unsupported parent <%s> for <%s>", parent, name)
				}
			}
			for ancestor := node.Parent; ancestor != nil; ancestor = ancestor.Parent {
				if ancestor.Namespace != "" {
					continue
				}
				an := ancestor.Data
				if (an == "a" && (name == "a" || name == "input" || name == "label" || name == "details" || attrValue(node, "tabindex") != "")) ||
					((an == "label" || an == "dfn") && name == an) ||
					((an == "header" || an == "footer") && (name == "header" || name == "footer")) ||
					((an == "address" || an == "dt" || an == "th") && (headingElements[name] || sectioningElements[name] || name == "header" || name == "footer" || name == "address")) ||
					(name == "main" && mainExcludedAncestors[an]) ||
					(an == "caption" && name == "table") ||
					((an == "a" || an == "del" || an == "ins") && phrasingContext(ancestor) && !phrasingElements[name] && !(node.Namespace == "svg" && name == "svg")) {
					return fmt.Errorf("unsupported HTML descendant <%s> inside <%s>", name, an)
				}
			}
			if name == "head" && full {
				titles := 0
				for _, child := range children {
					if child.Data == "title" {
						titles++
					}
				}
				if titles != 1 {
					return fmt.Errorf("head requires exactly one title")
				}
			}
			if name == "title" {
				var title strings.Builder
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Type == html.TextNode {
						title.WriteString(child.Data)
					}
				}
				if strings.TrimSpace(title.String()) == "" {
					return fmt.Errorf("title must contain text")
				}
			}
			if name == "hgroup" {
				headings := 0
				for _, child := range children {
					if headingElements[child.Data] {
						headings++
					}
				}
				if headings != 1 {
					return fmt.Errorf("hgroup requires exactly one heading")
				}
			}
			if name == "dl" {
				terms, descriptions := 0, 0
				for _, child := range children {
					if child.Data == "dt" {
						if descriptions > 0 {
							terms, descriptions = 0, 0
						}
						terms++
					} else {
						if terms == 0 {
							return fmt.Errorf("dl descriptions must follow terms")
						}
						descriptions++
					}
				}
				if terms > 0 && descriptions == 0 {
					return fmt.Errorf("dl term requires a description")
				}
			}
			if name == "details" && (len(children) == 0 || children[0].Data != "summary") {
				return fmt.Errorf("details requires summary as its first child element")
			}
			if name == "summary" {
				if node.Parent != nil && len(elementChildren(node.Parent)) > 0 && elementChildren(node.Parent)[0] != node {
					return fmt.Errorf("summary must be the first child element")
				}
				for _, child := range children {
					if !(phrasingElements[child.Data] || len(children) == 1 && headingElements[child.Data]) {
						return fmt.Errorf("summary requires phrasing content or one heading")
					}
				}
			}
			if name == "figure" {
				captions := 0
				for i, child := range children {
					if child.Data == "figcaption" {
						captions++
						if i != 0 && i != len(children)-1 {
							return fmt.Errorf("figcaption must be first or last")
						}
					}
				}
				if captions > 1 {
					return fmt.Errorf("figure may contain only one figcaption")
				}
			}
			if name == "table" {
				last := -1
				seen := map[string]bool{}
				for _, child := range children {
					rank := tableSectionRank[child.Data]
					if rank < last || seen[child.Data] && child.Data != "colgroup" && child.Data != "tbody" {
						return fmt.Errorf("invalid HTML table section order")
					}
					last, seen[child.Data] = rank, true
				}
			}
		}
		for _, child := range children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root)
}

func phrasingContext(node *html.Node) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Data != "a" && parent.Data != "del" && parent.Data != "ins" {
			return phrasingParents[parent.Data]
		}
	}
	return false
}
