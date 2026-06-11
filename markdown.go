package main

import (
	"fmt"
	"html"
	"html/template"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

type MarkdownImageResolver func(path, alt, title string) (string, error)

type markdownRenderState struct {
	defs            map[string]string
	used            map[string]bool
	imageResolver   MarkdownImageResolver
	collectTOC      bool
	blockquoteDepth int
	tocEntries      []TOCEntry
	tocIDs          map[string]bool
}

type inlineOptions struct {
	links     bool
	images    bool
	footnotes bool
}

type listMarker struct {
	kind    string
	number  string
	text    string
	task    bool
	checked bool
}

type tableBlock struct {
	headers []string
	rows    [][]string
}

type markdownGeminiLink struct {
	url   string
	label string
}

var (
	footnoteDefRE        = regexp.MustCompile(`^\[\^([0-9]+)\]: (.*)$`)
	footnoteRefRE        = regexp.MustCompile(`^\[\^([0-9]+)\]`)
	rawFootnoteRefRE     = regexp.MustCompile(`\[\^([0-9]+)\]`)
	atxHeadingRE         = regexp.MustCompile(`^(#{1,6})[ \t]+(.+?)\s*$`)
	headingIDRE          = regexp.MustCompile(`^(.*?)[ \t]+\{#([A-Za-z0-9][A-Za-z0-9_-]*)\}$`)
	tocLinkLabelRE       = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	footnoteAnchorIDRE   = regexp.MustCompile(`^cb[0-9]+$`)
	setextHeadingRE      = regexp.MustCompile(`^[ \t]*(=+|-+)[ \t]*$`)
	tableSeparatorCellRE = regexp.MustCompile(`^:?-{3,}:?$`)
)

func RenderMarkdownXHTML(input string) (string, error) {
	return RenderMarkdownXHTMLWithImages(input, nil)
}

func RenderMarkdownXHTMLWithImages(input string, resolver MarkdownImageResolver) (string, error) {
	rendered, _, err := RenderMarkdownXHTMLDocument(input, resolver, false)
	return rendered, err
}

func RenderMarkdownXHTMLDocument(input string, resolver MarkdownImageResolver, collectTOC bool) (string, []TOCEntry, error) {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	defs := map[string]string{}
	contentLines := []string{}
	for _, line := range strings.Split(input, "\n") {
		if match := footnoteDefRE.FindStringSubmatch(line); match != nil {
			n := match[1]
			if _, ok := defs[n]; ok {
				return "", nil, fmt.Errorf("duplicate footnote definition: %s", n)
			}
			defs[n] = match[2]
			continue
		}
		contentLines = append(contentLines, line)
	}

	state := &markdownRenderState{
		defs:          defs,
		used:          map[string]bool{},
		imageResolver: resolver,
		collectTOC:    collectTOC,
		tocIDs:        map[string]bool{},
	}
	rendered, err := renderMarkdownBlocks(contentLines, state)
	if err != nil {
		return "", nil, err
	}
	for n := range defs {
		if !state.used[n] {
			return "", nil, fmt.Errorf("unused footnote definition: %s", n)
		}
	}
	if collectTOC && len(state.tocEntries) == 0 {
		return "", nil, fmt.Errorf("toc requested but no level-2 headings found")
	}
	return rendered, state.tocEntries, nil
}

func renderMarkdownBlocks(lines []string, state *markdownRenderState) (string, error) {
	out := []string{}
	for i := 0; i < len(lines); {
		if strings.TrimSpace(lines[i]) == "" {
			i++
			continue
		}

		if marker, ok := fencedCodeMarker(lines[i]); ok {
			rendered, next, err := renderFencedCode(lines, i, marker)
			if err != nil {
				return "", err
			}
			out = append(out, rendered)
			i = next
			continue
		}

		if isRawHTMLLine(lines[i]) {
			return "", fmt.Errorf("raw HTML is not supported in markdown bodies: <%s>", rawHTMLTagToken(lines[i]))
		}

		if rendered, ok, err := renderATXHeading(lines[i], state); ok || err != nil {
			if err != nil {
				return "", err
			}
			out = append(out, rendered)
			i++
			continue
		}

		if i+1 < len(lines) {
			if level, ok := setextHeadingLevel(lines[i+1]); ok && strings.TrimSpace(lines[i]) != "" {
				source := strings.TrimSpace(lines[i])
				body, err := renderInline(source, state)
				if err != nil {
					return "", err
				}
				id, err := collectTOCHeading(state, level, source, "")
				if err != nil {
					return "", err
				}
				if id != "" {
					out = append(out, fmt.Sprintf(`<h%d id="%s">%s</h%d>`, level, id, body, level))
				} else {
					out = append(out, fmt.Sprintf("<h%d>%s</h%d>", level, body, level))
				}
				i += 2
				continue
			}
		}

		if isHorizontalRule(lines[i]) {
			out = append(out, "<hr />")
			i++
			continue
		}

		if table, next, ok := parseTableBlock(lines, i); ok {
			rendered, err := renderTable(table, state)
			if err != nil {
				return "", err
			}
			out = append(out, rendered)
			i = next
			continue
		}

		if isBlockquoteLine(lines[i]) {
			rendered, next, err := renderBlockquote(lines, i, state)
			if err != nil {
				return "", err
			}
			out = append(out, rendered)
			i = next
			continue
		}

		if marker, ok := parseListMarker(lines[i]); ok {
			rendered, next, err := renderList(lines, i, marker.kind, state)
			if err != nil {
				return "", err
			}
			out = append(out, rendered)
			i = next
			continue
		}

		paragraph, next := collectParagraph(lines, i)
		body, err := renderInline(strings.Join(paragraph, "\n"), state)
		if err != nil {
			return "", err
		}
		out = append(out, "<p>"+body+"</p>")
		i = next
	}
	return strings.Join(out, "\n"), nil
}

func RenderMarkdownGemtext(input string) (string, error) {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
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

	state := &markdownRenderState{
		defs: defs,
		used: map[string]bool{},
	}
	rendered, err := renderMarkdownGemtextBlocks(contentLines, state)
	if err != nil {
		return "", err
	}
	for n := range defs {
		if !state.used[n] {
			return "", fmt.Errorf("unused footnote definition: %s", n)
		}
	}
	if err := ValidateGemtext(rendered); err != nil {
		return "", err
	}
	return rendered, nil
}

func renderMarkdownGemtextBlocks(lines []string, state *markdownRenderState) (string, error) {
	out := []string{}
	for i := 0; i < len(lines); {
		if strings.TrimSpace(lines[i]) == "" {
			i++
			continue
		}

		if marker, ok := fencedCodeMarker(lines[i]); ok {
			rendered, next, err := renderMarkdownGemtextFencedCode(lines, i, marker)
			if err != nil {
				return "", err
			}
			out = append(out, rendered)
			i = next
			continue
		}

		if isRawHTMLLine(lines[i]) {
			return "", fmt.Errorf("raw HTML is not supported in markdown bodies: <%s>", rawHTMLTagToken(lines[i]))
		}

		if rendered, ok, err := renderMarkdownGemtextATXHeading(lines[i], state); ok || err != nil {
			if err != nil {
				return "", err
			}
			out = append(out, rendered...)
			i++
			continue
		}

		if i+1 < len(lines) {
			if level, ok := setextHeadingLevel(lines[i+1]); ok && strings.TrimSpace(lines[i]) != "" {
				text, links, err := renderMarkdownGemtextInline(strings.TrimSpace(lines[i]), state)
				if err != nil {
					return "", err
				}
				out = append(out, gemtextHeading(level, text))
				out = appendGeminiLinks(out, links)
				i += 2
				continue
			}
		}

		if isHorizontalRule(lines[i]) {
			out = append(out, "---")
			i++
			continue
		}

		if table, next, ok := parseTableBlock(lines, i); ok {
			rendered, err := renderMarkdownGemtextTable(table, state)
			if err != nil {
				return "", err
			}
			out = append(out, rendered...)
			i = next
			continue
		}

		if isBlockquoteLine(lines[i]) {
			rendered, next, err := renderMarkdownGemtextBlockquote(lines, i, state)
			if err != nil {
				return "", err
			}
			out = append(out, rendered...)
			i = next
			continue
		}

		if marker, ok := parseListMarker(lines[i]); ok {
			rendered, next, err := renderMarkdownGemtextList(lines, i, marker.kind, state)
			if err != nil {
				return "", err
			}
			out = append(out, rendered...)
			i = next
			continue
		}

		paragraph, next := collectParagraph(lines, i)
		text, links, err := renderMarkdownGemtextInline(strings.Join(paragraph, " "), state)
		if err != nil {
			return "", err
		}
		if text != "" {
			out = append(out, text)
		}
		out = appendGeminiLinks(out, links)
		i = next
	}
	return strings.Join(out, "\n\n"), nil
}

func renderMarkdownGemtextATXHeading(line string, state *markdownRenderState) ([]string, bool, error) {
	match := atxHeadingRE.FindStringSubmatch(line)
	if match == nil {
		return nil, false, nil
	}
	level := len(match[1])
	body := strings.TrimSpace(match[2])
	if strings.HasSuffix(body, "#") {
		body = strings.TrimSpace(strings.TrimRight(body, "#"))
	}
	if idMatch := headingIDRE.FindStringSubmatch(body); idMatch != nil {
		body = strings.TrimSpace(idMatch[1])
	}
	text, links, err := renderMarkdownGemtextInline(body, state)
	if err != nil {
		return nil, true, err
	}
	out := []string{gemtextHeading(level, text)}
	out = appendGeminiLinks(out, links)
	return out, true, nil
}

func renderMarkdownGemtextFencedCode(lines []string, start int, marker string) (string, int, error) {
	body := []string{}
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimLeft(lines[i], " \t"), marker) {
			return "```\n" + strings.Join(body, "\n") + "\n```", i + 1, nil
		}
		if strings.HasPrefix(lines[i], "```") {
			return "", start, fmt.Errorf("markdown code block contains unsupported gemtext fence line")
		}
		body = append(body, lines[i])
	}
	return "", start, fmt.Errorf("unclosed fenced code block")
}

func renderMarkdownGemtextTable(table tableBlock, state *markdownRenderState) ([]string, error) {
	rows := [][]string{table.headers}
	rows = append(rows, table.rows...)
	widths := make([]int, len(table.headers))
	rendered := make([][]string, len(rows))
	var links []markdownGeminiLink
	for rowIndex, row := range rows {
		rendered[rowIndex] = make([]string, len(row))
		for cellIndex, cell := range row {
			text, cellLinks, err := renderMarkdownGemtextInline(cell, state)
			if err != nil {
				return nil, err
			}
			rendered[rowIndex][cellIndex] = text
			if len(text) > widths[cellIndex] {
				widths[cellIndex] = len(text)
			}
			links = append(links, cellLinks...)
		}
	}
	tableLines := []string{"```"}
	for rowIndex, row := range rendered {
		tableLines = append(tableLines, pipeTableRow(row, widths))
		if rowIndex == 0 {
			separators := make([]string, len(widths))
			for i, width := range widths {
				if width < 3 {
					width = 3
				}
				separators[i] = strings.Repeat("-", width)
			}
			tableLines = append(tableLines, pipeTableRow(separators, widths))
		}
	}
	tableLines = append(tableLines, "```")
	out := []string{strings.Join(tableLines, "\n")}
	out = appendGeminiLinks(out, links)
	return out, nil
}

func pipeTableRow(row []string, widths []int) string {
	cells := make([]string, len(row))
	for i, cell := range row {
		cells[i] = cell + strings.Repeat(" ", widths[i]-len(cell))
	}
	return "| " + strings.Join(cells, " | ") + " |"
}

func renderMarkdownGemtextBlockquote(lines []string, start int, state *markdownRenderState) ([]string, int, error) {
	inner := []string{}
	i := start
	for i < len(lines) {
		trimmed := strings.TrimLeft(lines[i], " \t")
		if strings.TrimSpace(lines[i]) == "" {
			inner = append(inner, "")
			i++
			continue
		}
		if !strings.HasPrefix(trimmed, ">") {
			break
		}
		stripped := strings.TrimPrefix(trimmed, ">")
		stripped = strings.TrimPrefix(stripped, " ")
		inner = append(inner, stripped)
		i++
	}
	rendered, err := renderMarkdownGemtextBlocks(inner, state)
	if err != nil {
		return nil, start, err
	}
	out := []string{}
	for _, line := range strings.Split(rendered, "\n") {
		if strings.TrimSpace(line) == "" {
			out = append(out, ">")
		} else {
			out = append(out, "> "+line)
		}
	}
	return []string{strings.Join(out, "\n")}, i, nil
}

func renderMarkdownGemtextList(lines []string, start int, kind string, state *markdownRenderState) ([]string, int, error) {
	out := []string{}
	var links []markdownGeminiLink
	i := start
	for i < len(lines) {
		marker, ok := parseListMarker(lines[i])
		if !ok || marker.kind != kind {
			break
		}
		text, itemLinks, err := renderMarkdownGemtextInline(marker.text, state)
		if err != nil {
			return nil, start, err
		}
		prefix := "* "
		if marker.task {
			if marker.checked {
				prefix += "[x] "
			} else {
				prefix += "[ ] "
			}
		} else if kind == "ol" {
			prefix += marker.number + ". "
		}
		out = append(out, prefix+text)
		links = append(links, itemLinks...)
		i++
	}
	out = appendGeminiLinks(out, links)
	return out, i, nil
}

func renderMarkdownGemtextInline(text string, state *markdownRenderState) (string, []markdownGeminiLink, error) {
	var out strings.Builder
	var links []markdownGeminiLink
	for i := 0; i < len(text); {
		if text[i] == '\\' {
			if i+1 < len(text) && isEscapableMarkdownByte(text[i+1]) {
				out.WriteByte(text[i+1])
				i += 2
				continue
			}
			out.WriteByte(text[i])
			i++
			continue
		}
		if text[i] == '`' {
			end := strings.IndexByte(text[i+1:], '`')
			if end < 0 {
				return "", nil, fmt.Errorf("unclosed inline code span")
			}
			out.WriteString(text[i+1 : i+1+end])
			i += end + 2
			continue
		}
		if strings.HasPrefix(text[i:], "![") {
			return "", nil, fmt.Errorf("markdown images are not supported for flat-gemini-v1")
		}
		if text[i] == '[' && !strings.HasPrefix(text[i:], "[^") {
			label, link, next, ok, err := renderMarkdownGemtextInlineLink(text, i, state)
			if err != nil {
				return "", nil, err
			}
			if ok {
				out.WriteString(label)
				links = append(links, link)
				i = next
				continue
			}
		}
		if strings.HasPrefix(text[i:], "[^") {
			rendered, noteLinks, next, ok, err := renderMarkdownGemtextFootnote(text, i, state)
			if err != nil {
				return "", nil, err
			}
			if ok {
				out.WriteString(rendered)
				links = append(links, noteLinks...)
				i = next
				continue
			}
		}
		if strings.HasPrefix(text[i:], "**") || strings.HasPrefix(text[i:], "__") {
			rendered, nestedLinks, next, ok, err := renderMarkdownGemtextDelimited(text, i, text[i:i+2], state)
			if err != nil {
				return "", nil, err
			}
			if ok {
				out.WriteString(rendered)
				links = append(links, nestedLinks...)
				i = next
				continue
			}
		}
		if text[i] == '*' || text[i] == '_' {
			rendered, nestedLinks, next, ok, err := renderMarkdownGemtextDelimited(text, i, text[i:i+1], state)
			if err != nil {
				return "", nil, err
			}
			if ok {
				out.WriteString(rendered)
				links = append(links, nestedLinks...)
				i = next
				continue
			}
		}
		if text[i] == '<' && isInlineHTMLStart(text, i) {
			return "", nil, fmt.Errorf("inline HTML is not supported in markdown bodies; use a code span for literal tags")
		}
		next := nextSpecialInlineByte(text, i+1)
		out.WriteString(text[i:next])
		i = next
	}
	return normalizeGemtextText(out.String()), links, nil
}

func renderMarkdownGemtextDelimited(text string, start int, delimiter string, state *markdownRenderState) (string, []markdownGeminiLink, int, bool, error) {
	end := strings.Index(text[start+len(delimiter):], delimiter)
	if end < 0 {
		return "", nil, start, false, nil
	}
	innerStart := start + len(delimiter)
	innerEnd := innerStart + end
	if innerEnd == innerStart {
		return "", nil, start, false, nil
	}
	rendered, links, err := renderMarkdownGemtextInline(text[innerStart:innerEnd], state)
	if err != nil {
		return "", nil, start, false, err
	}
	return rendered, links, innerEnd + len(delimiter), true, nil
}

func renderMarkdownGemtextInlineLink(text string, start int, state *markdownRenderState) (string, markdownGeminiLink, int, bool, error) {
	closeBracket := findClosing(text, start+1, ']')
	if closeBracket < 0 || closeBracket+1 >= len(text) || text[closeBracket+1] != '(' {
		return "", markdownGeminiLink{}, start, false, nil
	}
	closeParen := findClosingParen(text, closeBracket+2)
	if closeParen < 0 {
		return "", markdownGeminiLink{}, start, false, fmt.Errorf("malformed markdown link")
	}
	target, title, err := parseDestinationAndTitle(text[closeBracket+2 : closeParen])
	if err != nil {
		return "", markdownGeminiLink{}, start, false, fmt.Errorf("malformed markdown link: %w", err)
	}
	if title != "" {
		return "", markdownGeminiLink{}, start, false, fmt.Errorf("markdown link titles are not supported")
	}
	if err := validateMarkdownLinkURL(target); err != nil {
		return "", markdownGeminiLink{}, start, false, err
	}
	label, nestedLinks, err := renderMarkdownGemtextInline(text[start+1:closeBracket], state)
	if err != nil {
		return "", markdownGeminiLink{}, start, false, err
	}
	if len(nestedLinks) > 0 {
		return "", markdownGeminiLink{}, start, false, fmt.Errorf("nested markdown links are not supported")
	}
	if label == "" {
		label = target
	}
	return label, markdownGeminiLink{url: target, label: label}, closeParen + 1, true, nil
}

func renderMarkdownGemtextFootnote(text string, start int, state *markdownRenderState) (string, []markdownGeminiLink, int, bool, error) {
	match := footnoteRefRE.FindStringSubmatch(text[start:])
	if match == nil {
		return "", nil, start, false, nil
	}
	n := match[1]
	body, ok := state.defs[n]
	if !ok {
		return "", nil, start, false, fmt.Errorf("missing footnote definition: %s", n)
	}
	state.used[n] = true
	rendered, links, err := renderMarkdownGemtextInline(body, state)
	if err != nil {
		return "", nil, start, false, err
	}
	if rendered == "" {
		return "", links, start + len(match[0]), true, nil
	}
	return " (" + rendered + ")", links, start + len(match[0]), true, nil
}

func appendGeminiLinks(out []string, links []markdownGeminiLink) []string {
	for _, link := range links {
		label := singleLineGemtext(link.label)
		if label == "" || label == link.url {
			out = append(out, "=> "+link.url)
		} else {
			out = append(out, "=> "+link.url+" "+label)
		}
	}
	return out
}

func gemtextHeading(level int, text string) string {
	if level > 3 {
		level = 3
	}
	if level < 1 {
		level = 1
	}
	return strings.Repeat("#", level) + " " + singleLineGemtext(text)
}

func normalizeGemtextText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func renderATXHeading(line string, state *markdownRenderState) (string, bool, error) {
	match := atxHeadingRE.FindStringSubmatch(line)
	if match == nil {
		return "", false, nil
	}
	level := len(match[1])
	body := strings.TrimSpace(match[2])
	if strings.HasSuffix(body, "#") {
		body = strings.TrimRight(body, "#")
		body = strings.TrimSpace(body)
	}
	id := ""
	if idMatch := headingIDRE.FindStringSubmatch(body); idMatch != nil {
		body = strings.TrimSpace(idMatch[1])
		id = idMatch[2]
	}
	rendered, err := renderInline(body, state)
	if err != nil {
		return "", true, err
	}
	id, err = collectTOCHeading(state, level, body, id)
	if err != nil {
		return "", true, err
	}
	if id != "" {
		return fmt.Sprintf(`<h%d id="%s">%s</h%d>`, level, id, rendered, level), true, nil
	}
	return fmt.Sprintf("<h%d>%s</h%d>", level, rendered, level), true, nil
}

// collectTOCHeading registers heading anchors and collects TOC entries while
// TOC collection is active. Level-2 headings outside blockquotes receive
// auto-derived anchors and TOC entries; explicit anchors on any heading join
// duplicate detection so the page's ids stay unique. It returns the id the
// heading must emit.
func collectTOCHeading(state *markdownRenderState, level int, body, id string) (string, error) {
	if !state.collectTOC {
		return id, nil
	}
	eligible := level == 2 && state.blockquoteDepth == 0
	tocSource := strings.TrimSpace(stripTOCMarkup(body))
	if id == "" && eligible {
		id = slugifyHeadingID(tocSource)
		if id == "" {
			return "", fmt.Errorf("cannot derive an anchor for heading %q; add an explicit {#id}", body)
		}
	}
	if id == "" {
		return "", nil
	}
	if state.tocIDs[id] {
		return "", fmt.Errorf("duplicate heading anchor %q; add an explicit {#id} to disambiguate", id)
	}
	if len(state.defs) > 0 && footnoteAnchorIDRE.MatchString(id) {
		return "", fmt.Errorf("heading anchor %q collides with footnote checkbox ids; choose a different {#id}", id)
	}
	state.tocIDs[id] = true
	if !eligible {
		return id, nil
	}
	tocText, err := renderInlineWithOptions(tocSource, state, inlineOptions{})
	if err != nil {
		return "", err
	}
	state.tocEntries = append(state.tocEntries, TOCEntry{Href: "#" + id, Text: template.HTML(tocText)})
	return id, nil
}

// stripTOCMarkup reduces a heading's source text to the words a TOC entry
// shows: footnote refs become spaces and link/image markup contributes only
// its label.
func stripTOCMarkup(body string) string {
	body = rawFootnoteRefRE.ReplaceAllString(body, " ")
	return tocLinkLabelRE.ReplaceAllString(body, "$1")
}

func slugifyHeadingID(text string) string {
	var out strings.Builder
	pendingHyphen := false
	for i := 0; i < len(text); i++ {
		ch := text[i]
		switch {
		case ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9':
		case ch >= 'A' && ch <= 'Z':
			ch += 'a' - 'A'
		default:
			pendingHyphen = true
			continue
		}
		if pendingHyphen && out.Len() > 0 {
			out.WriteByte('-')
		}
		pendingHyphen = false
		out.WriteByte(ch)
	}
	return out.String()
}

func setextHeadingLevel(line string) (int, bool) {
	match := setextHeadingRE.FindStringSubmatch(line)
	if match == nil {
		return 0, false
	}
	if strings.HasPrefix(match[1], "=") {
		return 1, true
	}
	return 2, true
}

func isRawHTMLLine(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	return len(trimmed) > 0 && trimmed[0] == '<' && isInlineHTMLStart(trimmed, 0)
}

func rawHTMLTagToken(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	end := 1
	for end < len(trimmed) && end < 24 {
		ch := trimmed[end]
		if ch == ' ' || ch == '\t' || ch == '>' {
			break
		}
		end++
	}
	return trimmed[1:end]
}

// isInlineHTMLStart reports whether the '<' at text[i] begins an HTML-shaped
// construct: a tag (`<a`), a closing tag (`</a`), a comment (`<!--`), or a
// declaration (`<!D`). A '<' followed by anything else — space, digit,
// punctuation, or end of text — is ordinary prose.
func isInlineHTMLStart(text string, i int) bool {
	if i+1 >= len(text) {
		return false
	}
	switch next := text[i+1]; {
	case isASCIILetter(next):
		return true
	case next == '/':
		return i+2 < len(text) && isASCIILetter(text[i+2])
	case next == '!':
		if i+3 < len(text) && text[i+2] == '-' && text[i+3] == '-' {
			return true
		}
		return i+2 < len(text) && isASCIILetter(text[i+2])
	default:
		return false
	}
}

func isASCIILetter(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z')
}

var markdownHTMLEntityRE = regexp.MustCompile(`^&(?:#[0-9]+|#[xX][0-9a-fA-F]+|[A-Za-z][A-Za-z0-9]*);`)

// ValidateMarkdownSource enforces the pure-Markdown doctrine: a markdown body
// may not embed HTML in any form — neither HTML elements (<div>, </p>,
// <!-- … -->) nor HTML character references (&nbsp;, &amp;, &#160;, &#xA0;).
// Standard CommonMark backslash escapes are permitted, since they are Markdown
// rather than HTML. Angle brackets and ampersands inside fenced code blocks and
// inline code spans are literal content and are therefore allowed. baseLine is
// the 1-based source-file line of the body's first line, so the reported line
// number is absolute within the .md file.
func ValidateMarkdownSource(body string, baseLine int) error {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	if baseLine < 1 {
		baseLine = 1
	}
	inFence := false
	fence := ""
	inCodeSpan := false
	for idx, line := range strings.Split(body, "\n") {
		lineNo := baseLine + idx
		if inFence {
			if strings.HasPrefix(strings.TrimLeft(line, " \t"), fence) {
				inFence = false
			}
			continue
		}
		if marker, ok := fencedCodeMarker(line); ok {
			inFence = true
			fence = marker
			inCodeSpan = false
			continue
		}
		if strings.TrimSpace(line) == "" {
			inCodeSpan = false
			continue
		}
		for i := 0; i < len(line); {
			if inCodeSpan {
				if line[i] == '`' {
					inCodeSpan = false
				}
				i++
				continue
			}
			switch line[i] {
			case '\\':
				// CommonMark backslash escape: the escaped punctuation is
				// literal text, so step over it. Escapes are Markdown, not HTML.
				if i+1 < len(line) && isEscapableMarkdownByte(line[i+1]) {
					i += 2
				} else {
					i++
				}
			case '`':
				inCodeSpan = true
				i++
			case '&':
				if entity := markdownHTMLEntityRE.FindString(line[i:]); entity != "" {
					return fmt.Errorf("line %d: HTML character entities are not supported in markdown bodies; type the character directly (found %q)", lineNo, entity)
				}
				i++
			case '<':
				if isInlineHTMLStart(line, i) {
					return fmt.Errorf("line %d: embedded HTML is not supported in markdown bodies; use pure Markdown (found %q)", lineNo, "<"+rawHTMLTagToken(line[i:])+">")
				}
				i++
			default:
				i++
			}
		}
	}
	return nil
}

func fencedCodeMarker(line string) (string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmed, "```") {
		return "```", true
	}
	if strings.HasPrefix(trimmed, "~~~") {
		return "~~~", true
	}
	return "", false
}

func renderFencedCode(lines []string, start int, marker string) (string, int, error) {
	body := []string{}
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimLeft(lines[i], " \t"), marker) {
			return "<pre><code>" + html.EscapeString(strings.Join(body, "\n")) + "</code></pre>", i + 1, nil
		}
		body = append(body, lines[i])
	}
	return "", start, fmt.Errorf("unclosed fenced code block")
}

func isHorizontalRule(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < 3 {
		return false
	}
	marker := byte(0)
	count := 0
	for i := 0; i < len(trimmed); i++ {
		if trimmed[i] == ' ' || trimmed[i] == '\t' {
			continue
		}
		if trimmed[i] != '-' && trimmed[i] != '*' && trimmed[i] != '_' {
			return false
		}
		if marker == 0 {
			marker = trimmed[i]
		}
		if trimmed[i] != marker {
			return false
		}
		count++
	}
	return count >= 3
}

func parseTableBlock(lines []string, start int) (tableBlock, int, bool) {
	if start+1 >= len(lines) || !strings.Contains(lines[start], "|") {
		return tableBlock{}, start, false
	}
	headers := splitTableRow(lines[start])
	separator := splitTableRow(lines[start+1])
	if len(headers) == 0 || len(headers) != len(separator) {
		return tableBlock{}, start, false
	}
	for _, cell := range separator {
		if !tableSeparatorCellRE.MatchString(strings.TrimSpace(cell)) {
			return tableBlock{}, start, false
		}
	}
	rows := [][]string{}
	i := start + 2
	for i < len(lines) {
		if strings.TrimSpace(lines[i]) == "" || !strings.Contains(lines[i], "|") {
			break
		}
		row := splitTableRow(lines[i])
		if len(row) != len(headers) {
			break
		}
		rows = append(rows, row)
		i++
	}
	return tableBlock{headers: headers, rows: rows}, i, true
}

func splitTableRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func renderTable(table tableBlock, state *markdownRenderState) (string, error) {
	var out strings.Builder
	out.WriteString("<table>\n<thead>\n<tr>")
	for _, cell := range table.headers {
		rendered, err := renderInline(cell, state)
		if err != nil {
			return "", err
		}
		out.WriteString("<th>")
		out.WriteString(rendered)
		out.WriteString("</th>")
	}
	out.WriteString("</tr>\n</thead>\n<tbody>")
	for _, row := range table.rows {
		out.WriteString("\n<tr>")
		for _, cell := range row {
			rendered, err := renderInline(cell, state)
			if err != nil {
				return "", err
			}
			out.WriteString("<td>")
			out.WriteString(rendered)
			out.WriteString("</td>")
		}
		out.WriteString("</tr>")
	}
	out.WriteString("\n</tbody>\n</table>")
	return out.String(), nil
}

func isBlockquoteLine(line string) bool {
	return strings.HasPrefix(strings.TrimLeft(line, " \t"), ">")
}

func renderBlockquote(lines []string, start int, state *markdownRenderState) (string, int, error) {
	inner := []string{}
	i := start
	for i < len(lines) {
		trimmed := strings.TrimLeft(lines[i], " \t")
		if strings.TrimSpace(lines[i]) == "" {
			inner = append(inner, "")
			i++
			continue
		}
		if !strings.HasPrefix(trimmed, ">") {
			break
		}
		stripped := strings.TrimPrefix(trimmed, ">")
		stripped = strings.TrimPrefix(stripped, " ")
		inner = append(inner, stripped)
		i++
	}
	state.blockquoteDepth++
	rendered, err := renderMarkdownBlocks(inner, state)
	state.blockquoteDepth--
	if err != nil {
		return "", start, err
	}
	return "<blockquote>\n" + rendered + "\n</blockquote>", i, nil
}

func parseListMarker(line string) (listMarker, bool) {
	indent := len(line) - len(strings.TrimLeft(line, " \t"))
	if indent > 3 {
		return listMarker{}, false
	}
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) >= 2 && strings.ContainsRune("-+*", rune(trimmed[0])) && isSpace(trimmed[1]) {
		text := strings.TrimLeft(trimmed[2:], " \t")
		marker := listMarker{kind: "ul", text: text}
		applyTaskMarker(&marker)
		return marker, true
	}
	digits := 0
	for digits < len(trimmed) && trimmed[digits] >= '0' && trimmed[digits] <= '9' {
		digits++
	}
	if digits > 0 && digits+1 < len(trimmed) && (trimmed[digits] == '.' || trimmed[digits] == ')') && isSpace(trimmed[digits+1]) {
		return listMarker{kind: "ol", number: trimmed[:digits], text: strings.TrimLeft(trimmed[digits+2:], " \t")}, true
	}
	return listMarker{}, false
}

func applyTaskMarker(marker *listMarker) {
	if len(marker.text) < 3 || marker.text[0] != '[' || marker.text[2] != ']' {
		return
	}
	if marker.text[1] != ' ' && marker.text[1] != 'x' && marker.text[1] != 'X' {
		return
	}
	if len(marker.text) > 3 && !isSpace(marker.text[3]) {
		return
	}
	marker.task = true
	marker.checked = marker.text[1] == 'x' || marker.text[1] == 'X'
	marker.text = strings.TrimLeft(marker.text[3:], " \t")
}

// listItemBlock is one collected list item: its marker plus the item's content
// lines, already dedented to the marker's content column. An empty string in
// content marks a paragraph break (a blank line within the item).
type listItemBlock struct {
	marker  listMarker
	content []string
}

func renderList(lines []string, start int, kind string, state *markdownRenderState) (string, int, error) {
	var items []listItemBlock
	// A list is "loose" if any items are separated by a blank line, or any item
	// contains a blank line between paragraphs; loose items render with <p>.
	loose := false
	i := start
	for i < len(lines) {
		marker, ok := parseListMarker(lines[i])
		if !ok || marker.kind != kind {
			break
		}
		contentIndent := listContentIndent(lines[i], marker)
		content := []string{marker.text}
		i++
		blanks := 0
		endList := false
		for i < len(lines) {
			line := lines[i]
			if strings.TrimSpace(line) == "" {
				blanks++
				i++
				continue
			}
			// A sibling marker at the list's own indentation starts the next item.
			if m, ok := parseListMarker(line); ok && m.kind == kind && indentWidth(line) < contentIndent {
				if blanks > 0 {
					// A blank line before a same-kind marker keeps it in this
					// (loose) list — unless its task-ness differs, in which case
					// it begins a separate list (e.g. plain bullets, then a
					// checkbox task list).
					if m.task != marker.task {
						endList = true
					} else {
						loose = true
					}
				}
				break
			}
			// A line indented to the content column continues this item.
			if indentWidth(line) >= contentIndent {
				if blanks > 0 {
					content = append(content, "")
					loose = true
				}
				content = append(content, stripIndent(line, contentIndent))
				blanks = 0
				i++
				continue
			}
			// Anything else (a non-indented, non-marker line) ends the list.
			break
		}
		items = append(items, listItemBlock{marker: marker, content: content})
		if endList {
			break
		}
	}

	taskList := false
	for _, item := range items {
		if item.marker.task {
			taskList = true
			break
		}
	}

	var out strings.Builder
	switch {
	case kind == "ol":
		// Preserve the first item's number so a list that does not begin at 1
		// (e.g. a numbered section resuming after a heading) continues correctly.
		start := ""
		if len(items) > 0 {
			if n := items[0].marker.number; n != "" && n != "1" {
				start = ` start="` + n + `"`
			}
		}
		out.WriteString("<ol" + start + ">")
	case taskList:
		out.WriteString(`<ul class="task-list">`)
	default:
		out.WriteString("<ul>")
	}
	for _, item := range items {
		out.WriteByte('\n')
		if item.marker.task {
			out.WriteString(`<li class="task-list-item">`)
			if item.marker.checked {
				out.WriteString("&#9745; ")
			} else {
				out.WriteString("&#9744; ")
			}
			rendered, err := renderInline(strings.TrimSpace(strings.Join(item.content, "\n")), state)
			if err != nil {
				return "", start, err
			}
			out.WriteString(rendered)
			out.WriteString("</li>")
			continue
		}
		if loose {
			inner, err := renderMarkdownBlocks(item.content, state)
			if err != nil {
				return "", start, err
			}
			out.WriteString("<li>\n")
			out.WriteString(inner)
			out.WriteString("\n</li>")
		} else {
			rendered, err := renderInline(strings.Join(item.content, "\n"), state)
			if err != nil {
				return "", start, err
			}
			out.WriteString("<li>")
			out.WriteString(rendered)
			out.WriteString("</li>")
		}
	}
	out.WriteByte('\n')
	if kind == "ol" {
		out.WriteString("</ol>")
	} else {
		out.WriteString("</ul>")
	}
	return out.String(), i, nil
}

// indentWidth counts leading space/tab characters.
func indentWidth(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

// stripIndent removes up to n leading space/tab characters.
func stripIndent(line string, n int) string {
	if w := indentWidth(line); w < n {
		n = w
	}
	return line[n:]
}

// listContentIndent is the column at which a list item's content begins —
// the marker's own indentation plus the marker width and its trailing space.
func listContentIndent(line string, marker listMarker) int {
	base := indentWidth(line)
	if marker.kind == "ol" {
		return base + len(marker.number) + 2
	}
	return base + 2
}

func collectParagraph(lines []string, start int) ([]string, int) {
	block := []string{}
	i := start
	for i < len(lines) {
		if strings.TrimSpace(lines[i]) == "" {
			break
		}
		if len(block) > 0 && isMarkdownBlockStart(lines, i) {
			break
		}
		block = append(block, lines[i])
		i++
	}
	return block, i
}

func isMarkdownBlockStart(lines []string, i int) bool {
	if _, ok := fencedCodeMarker(lines[i]); ok {
		return true
	}
	if isRawHTMLLine(lines[i]) {
		return true
	}
	if atxHeadingRE.MatchString(lines[i]) || isHorizontalRule(lines[i]) || isBlockquoteLine(lines[i]) {
		return true
	}
	if _, ok := parseListMarker(lines[i]); ok {
		return true
	}
	if _, _, ok := parseTableBlock(lines, i); ok {
		return true
	}
	return false
}

func renderInline(text string, state *markdownRenderState) (string, error) {
	return renderInlineWithOptions(text, state, inlineOptions{links: true, images: true, footnotes: true})
}

func renderInlineWithOptions(text string, state *markdownRenderState, opts inlineOptions) (string, error) {
	var out strings.Builder
	for i := 0; i < len(text); {
		if text[i] == '\\' {
			if i+1 < len(text) && isEscapableMarkdownByte(text[i+1]) {
				out.WriteString(html.EscapeString(text[i+1 : i+2]))
				i += 2
				continue
			}
			out.WriteString(`\`)
			i++
			continue
		}
		if text[i] == '`' {
			end := strings.IndexByte(text[i+1:], '`')
			if end < 0 {
				return "", fmt.Errorf("unclosed inline code span")
			}
			code := text[i+1 : i+1+end]
			out.WriteString("<code>")
			out.WriteString(html.EscapeString(code))
			out.WriteString("</code>")
			i += end + 2
			continue
		}
		if opts.images && strings.HasPrefix(text[i:], "![") {
			rendered, next, err := renderInlineImage(text, i, state)
			if err != nil {
				return "", err
			}
			out.WriteString(rendered)
			i = next
			continue
		}
		if opts.links && text[i] == '[' && !strings.HasPrefix(text[i:], "[^") {
			rendered, next, ok, err := renderInlineLink(text, i, state)
			if err != nil {
				return "", err
			}
			if ok {
				out.WriteString(rendered)
				i = next
				continue
			}
		}
		if opts.footnotes && strings.HasPrefix(text[i:], "[^") {
			rendered, next, ok, err := renderFootnoteRef(text, i, state)
			if err != nil {
				return "", err
			}
			if ok {
				out.WriteString(rendered)
				i = next
				continue
			}
		}
		if strings.HasPrefix(text[i:], "**") || strings.HasPrefix(text[i:], "__") {
			rendered, next, ok, err := renderDelimitedInline(text, i, text[i:i+2], "strong", state, opts)
			if err != nil {
				return "", err
			}
			if ok {
				out.WriteString(rendered)
				i = next
				continue
			}
		}
		if text[i] == '*' || text[i] == '_' {
			rendered, next, ok, err := renderDelimitedInline(text, i, text[i:i+1], "em", state, opts)
			if err != nil {
				return "", err
			}
			if ok {
				out.WriteString(rendered)
				i = next
				continue
			}
		}
		if text[i] == '<' && isInlineHTMLStart(text, i) {
			return "", fmt.Errorf("inline HTML is not supported in markdown bodies; use a code span for literal tags")
		}
		next := nextSpecialInlineByte(text, i+1)
		out.WriteString(html.EscapeString(text[i:next]))
		i = next
	}
	return out.String(), nil
}

func renderDelimitedInline(text string, start int, delimiter, tag string, state *markdownRenderState, opts inlineOptions) (string, int, bool, error) {
	end := strings.Index(text[start+len(delimiter):], delimiter)
	if end < 0 {
		return "", start, false, nil
	}
	innerStart := start + len(delimiter)
	innerEnd := innerStart + end
	if innerEnd == innerStart {
		return "", start, false, nil
	}
	inner, err := renderInlineWithOptions(text[innerStart:innerEnd], state, opts)
	if err != nil {
		return "", start, false, err
	}
	return "<" + tag + ">" + inner + "</" + tag + ">", innerEnd + len(delimiter), true, nil
}

func renderInlineImage(text string, start int, state *markdownRenderState) (string, int, error) {
	closeBracket := findClosing(text, start+2, ']')
	if closeBracket < 0 || closeBracket+1 >= len(text) || text[closeBracket+1] != '(' {
		return "", start, fmt.Errorf("malformed markdown image")
	}
	closeParen := findClosingParen(text, closeBracket+2)
	if closeParen < 0 {
		return "", start, fmt.Errorf("malformed markdown image")
	}
	alt := unescapeMarkdownText(text[start+2 : closeBracket])
	target, title, err := parseDestinationAndTitle(text[closeBracket+2 : closeParen])
	if err != nil {
		return "", start, fmt.Errorf("malformed markdown image: %w", err)
	}
	if err := validateMarkdownImagePath(target); err != nil {
		return "", start, err
	}
	if state.imageResolver == nil {
		return "", start, fmt.Errorf("markdown image resolver is not configured")
	}
	rendered, err := state.imageResolver(target, alt, title)
	if err != nil {
		return "", start, err
	}
	return rendered, closeParen + 1, nil
}

func renderInlineLink(text string, start int, state *markdownRenderState) (string, int, bool, error) {
	closeBracket := findClosing(text, start+1, ']')
	if closeBracket < 0 || closeBracket+1 >= len(text) || text[closeBracket+1] != '(' {
		return "", start, false, nil
	}
	closeParen := findClosingParen(text, closeBracket+2)
	if closeParen < 0 {
		return "", start, false, fmt.Errorf("malformed markdown link")
	}
	target, title, err := parseDestinationAndTitle(text[closeBracket+2 : closeParen])
	if err != nil {
		return "", start, false, fmt.Errorf("malformed markdown link: %w", err)
	}
	if title != "" {
		return "", start, false, fmt.Errorf("markdown link titles are not supported")
	}
	if err := validateMarkdownLinkURL(target); err != nil {
		return "", start, false, err
	}
	label, err := renderInlineWithOptions(text[start+1:closeBracket], state, inlineOptions{links: false, images: false, footnotes: true})
	if err != nil {
		return "", start, false, err
	}
	return `<a href="` + html.EscapeString(target) + `">` + label + `</a>`, closeParen + 1, true, nil
}

func renderFootnoteRef(text string, start int, state *markdownRenderState) (string, int, bool, error) {
	match := footnoteRefRE.FindStringSubmatch(text[start:])
	if match == nil {
		return "", start, false, nil
	}
	n := match[1]
	body, ok := state.defs[n]
	if !ok {
		return "", start, false, fmt.Errorf("missing footnote definition: %s", n)
	}
	state.used[n] = true
	rendered, err := renderInlineWithOptions(body, state, inlineOptions{links: true, images: false, footnotes: false})
	if err != nil {
		return "", start, false, err
	}
	return `<span><input type="checkbox" id="cb` + n + `" /><label for="cb` + n + `"><sup>` + n + `</sup></label><span>` + rendered + `</span></span>`, start + len(match[0]), true, nil
}

func parseDestinationAndTitle(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", fmt.Errorf("empty destination")
	}
	pathEnd := 0
	for pathEnd < len(value) && !unicode.IsSpace(rune(value[pathEnd])) {
		pathEnd++
	}
	target := value[:pathEnd]
	rest := strings.TrimSpace(value[pathEnd:])
	if target == "" {
		return "", "", fmt.Errorf("empty destination")
	}
	if rest == "" {
		return target, "", nil
	}
	if len(rest) < 2 || (rest[0] != '"' && rest[0] != '\'') || rest[len(rest)-1] != rest[0] {
		return "", "", fmt.Errorf("invalid title")
	}
	return target, unescapeMarkdownText(rest[1 : len(rest)-1]), nil
}

func validateMarkdownLinkURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("markdown link URL is required")
	}
	lower := strings.ToLower(strings.TrimSpace(raw))
	if strings.HasPrefix(lower, "//") || strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "data:") {
		return fmt.Errorf("unsupported markdown link URL: %s", raw)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("malformed markdown link URL: %s", raw)
	}
	switch parsed.Scheme {
	case "":
		return nil
	case "http", "https", "gemini":
		if parsed.Host == "" {
			return fmt.Errorf("malformed markdown link URL: %s", raw)
		}
		return nil
	case "mailto":
		if parsed.Opaque == "" && parsed.Path == "" {
			return fmt.Errorf("malformed markdown link URL: %s", raw)
		}
		return nil
	default:
		return fmt.Errorf("unsupported markdown link URL scheme: %s", parsed.Scheme)
	}
}

func validateMarkdownImagePath(raw string) error {
	if raw == "" {
		return fmt.Errorf("markdown image path is required")
	}
	lower := strings.ToLower(strings.TrimSpace(raw))
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "//") {
		return fmt.Errorf("remote markdown images are not supported: %s", raw)
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Scheme != "" {
		return fmt.Errorf("markdown image path must be local: %s", raw)
	}
	return nil
}

func findClosing(text string, start int, close byte) int {
	for i := start; i < len(text); i++ {
		if text[i] == '\\' {
			i++
			continue
		}
		if text[i] == close {
			return i
		}
	}
	return -1
}

func findClosingParen(text string, start int) int {
	var quote byte
	for i := start; i < len(text); i++ {
		if text[i] == '\\' {
			i++
			continue
		}
		if quote != 0 {
			if text[i] == quote {
				quote = 0
			}
			continue
		}
		if text[i] == '"' || text[i] == '\'' {
			quote = text[i]
			continue
		}
		if text[i] == ')' {
			return i
		}
	}
	return -1
}

func unescapeMarkdownText(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+1 < len(value) && isEscapableMarkdownByte(value[i+1]) {
			out.WriteByte(value[i+1])
			i++
			continue
		}
		out.WriteByte(value[i])
	}
	return out.String()
}

func nextSpecialInlineByte(text string, start int) int {
	for i := start; i < len(text); i++ {
		if isSpecialInlineByte(text[i]) {
			return i
		}
	}
	return len(text)
}

func isSpecialInlineByte(value byte) bool {
	switch value {
	case '\\', '`', '*', '_', '[', '!', '<':
		return true
	default:
		return false
	}
}

func isEscapableMarkdownByte(value byte) bool {
	return strings.ContainsRune(`\`+"`"+`*_{}[]()#+-.!|`, rune(value))
}

func isSpace(value byte) bool {
	return value == ' ' || value == '\t'
}
