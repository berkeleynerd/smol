package main

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

var globalAttributes = wordSet("id class title lang dir role hidden tabindex accesskey style xml:lang")
var htmlAttributes = map[string]map[string]bool{
	"a": wordSet("href rel target download hreflang type"), "blockquote": wordSet("cite"), "q": wordSet("cite"),
	"del": wordSet("cite datetime"), "ins": wordSet("cite datetime"), "data": wordSet("value"), "time": wordSet("datetime"),
	"meta": wordSet("charset name content http-equiv"), "link": wordSet("href rel hreflang type media"), "style": wordSet("type media"),
	"img": wordSet("src alt width height loading decoding"), "input": wordSet("type"), "label": wordSet("for"),
	"ol": wordSet("start reversed type"), "li": wordSet("value"), "col": wordSet("span"), "colgroup": wordSet("span"),
	"td": wordSet("colspan rowspan headers"), "th": wordSet("colspan rowspan headers scope abbr"), "details": wordSet("open"),
}
var orderedListTypes = wordSet("1 a A i I")

func hasAttribute(node *html.Node, name string) bool {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return true
		}
	}
	return false
}

func validateAttributes(node *html.Node) error {
	name := node.Data
	for _, attr := range node.Attr {
		key, value := attr.Key, attr.Val
		if key != "xmlns" && !globalAttributes[key] && !strings.HasPrefix(key, "aria-") && !strings.HasPrefix(key, "data-") && !htmlAttributes[name][key] {
			return fmt.Errorf("attribute %s is not supported on <%s>", key, name)
		}
		valid := true
		switch key {
		case "id":
			valid = value != "" && !strings.ContainsAny(value, " \t\r\n\f")
		case "open", "reversed":
			valid = value == "" || strings.EqualFold(value, key)
		case "hidden":
			valid = value == "" || strings.EqualFold(value, "hidden") || strings.EqualFold(value, "until-found")
		case "dir":
			valid = value == "ltr" || value == "rtl" || value == "auto"
		case "tabindex", "start", "value":
			if key != "value" || name == "li" {
				_, err := strconv.ParseInt(value, 10, 64)
				valid = err == nil
			}
		case "width", "height", "span", "colspan", "rowspan":
			n, err := strconv.ParseUint(value, 10, 32)
			valid = err == nil && (n > 0 || key == "rowspan")
			if key == "colspan" {
				valid = valid && n <= 1000
			}
			if key == "rowspan" {
				valid = valid && n <= 65534
			}
		case "loading":
			valid = value == "lazy" || value == "eager"
		case "decoding":
			valid = value == "async" || value == "sync" || value == "auto"
		case "scope":
			valid = value == "row" || value == "col" || value == "rowgroup" || value == "colgroup"
		}
		if !valid {
			return fmt.Errorf("invalid %s attribute on <%s>", key, name)
		}
	}
	if lang := attrValue(node, "xml:lang"); lang != "" && lang != attrValue(node, "lang") {
		return fmt.Errorf("xml:lang and lang must match")
	}
	if name == "bdo" && attrValue(node, "dir") != "ltr" && attrValue(node, "dir") != "rtl" {
		return fmt.Errorf("bdo requires dir=\"ltr\" or dir=\"rtl\"")
	}
	if name == "data" && !hasAttribute(node, "value") {
		return fmt.Errorf("data requires a value attribute")
	}
	if name == "time" && !hasAttribute(node, "datetime") {
		return fmt.Errorf("the static time subset requires datetime")
	}
	if name == "img" && !hasAttribute(node, "alt") {
		return fmt.Errorf("img requires alt text (empty alt is allowed for decoration)")
	}
	if name == "link" && attrValue(node, "href") == "" {
		return fmt.Errorf("canonical link requires href")
	}
	if name == "meta" {
		charset := attrValue(node, "charset")
		if charset != "" {
			if !strings.EqualFold(charset, "utf-8") || len(node.Attr) != 1 {
				return fmt.Errorf("meta charset must declare only UTF-8")
			}
		} else {
			equiv := attrValue(node, "http-equiv")
			if attrValue(node, "name") == "" && equiv == "" || !hasAttribute(node, "content") {
				return fmt.Errorf("meta requires charset, or name/http-equiv with content")
			}
			if equiv != "" && (attrValue(node, "name") != "" || !strings.EqualFold(strings.ReplaceAll(attrValue(node, "content"), " ", ""), "text/html;charset=utf-8")) {
				return fmt.Errorf("HTML-compatible encoding metadata must use text/html; charset=UTF-8")
			}
		}
	}
	if name == "style" && attrValue(node, "type") != "" && attrValue(node, "type") != "text/css" {
		return fmt.Errorf("style type must be text/css")
	}
	if name == "ol" && hasAttribute(node, "type") && !orderedListTypes[attrValue(node, "type")] {
		return fmt.Errorf("invalid ordered list type")
	}
	return nil
}
