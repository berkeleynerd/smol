package main

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	footnoteDefRE = regexp.MustCompile(`^\[\^([0-9]+)\]: (.*)$`)
	footnoteRefRE = regexp.MustCompile(`\[\^([0-9]+)\]`)
	atxHeadingRE  = regexp.MustCompile(`^(#{1,6})[ \t]+(.+)$`)
	headingIDRE   = regexp.MustCompile(`^(.*?)[ \t]+\{#([A-Za-z0-9][A-Za-z0-9_-]*)\}$`)
	rawBlockRE    = regexp.MustCompile(`(?is)^<(p|ol|ul|blockquote|svg|div|table|pre|hr|br|section|article|header|main|footer|h[1-6])(\s|>|/)`)
)

func RenderMarkdownXHTML(input string) (string, error) {
	defs := map[string]string{}
	contentLines := []string{}
	for _, line := range strings.Split(input, "\n") {
		if match := footnoteDefRE.FindStringSubmatch(line); match != nil {
			n := match[1]
			if _, ok := defs[n]; ok {
				return "", fmt.Errorf("duplicate footnote definition: %s", n)
			}
			defs[n] = match[2]
			continue
		}
		contentLines = append(contentLines, line)
	}

	used := map[string]bool{}
	blocks := markdownBlocks(contentLines)
	out := make([]string, 0, len(blocks))
	for _, block := range blocks {
		rendered, err := renderMarkdownBlock(block, defs, used)
		if err != nil {
			return "", err
		}
		if rendered != "" {
			out = append(out, rendered)
		}
	}
	for n := range defs {
		if !used[n] {
			return "", fmt.Errorf("unused footnote definition: %s", n)
		}
	}
	return strings.Join(out, "\n"), nil
}

func markdownBlocks(lines []string) [][]string {
	var blocks [][]string
	var block []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			if len(block) > 0 {
				blocks = append(blocks, block)
				block = nil
			}
			continue
		}
		block = append(block, line)
	}
	if len(block) > 0 {
		blocks = append(blocks, block)
	}
	return blocks
}

func renderMarkdownBlock(lines []string, defs map[string]string, used map[string]bool) (string, error) {
	text := strings.Join(lines, "\n")
	if match := atxHeadingRE.FindStringSubmatch(text); match != nil && !strings.Contains(text, "\n") {
		level := len(match[1])
		body := match[2]
		id := ""
		if idMatch := headingIDRE.FindStringSubmatch(body); idMatch != nil {
			body = idMatch[1]
			id = idMatch[2]
		}
		body, err := spliceFootnotes(body, defs, used)
		if err != nil {
			return "", err
		}
		if id != "" {
			return fmt.Sprintf(`<h%d id="%s">%s</h%d>`, level, id, body, level), nil
		}
		return fmt.Sprintf(`<h%d>%s</h%d>`, level, body, level), nil
	}
	if rawBlockRE.MatchString(strings.TrimLeft(text, " \t")) {
		return spliceFootnotes(text, defs, used)
	}
	body, err := spliceFootnotes(text, defs, used)
	if err != nil {
		return "", err
	}
	return "<p>" + body + "</p>", nil
}

func spliceFootnotes(text string, defs map[string]string, used map[string]bool) (string, error) {
	var err error
	rendered := footnoteRefRE.ReplaceAllStringFunc(text, func(match string) string {
		if err != nil {
			return match
		}
		n := footnoteRefRE.FindStringSubmatch(match)[1]
		body, ok := defs[n]
		if !ok {
			err = fmt.Errorf("missing footnote definition: %s", n)
			return match
		}
		used[n] = true
		return `<span><input type="checkbox" id="cb` + n + `" /><label for="cb` + n + `"><sup>` + n + `</sup></label><span>` + body + `</span></span>`
	})
	return rendered, err
}
