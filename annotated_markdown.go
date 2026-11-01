package main

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const annotatedTextExtension = "annotated-text-v1"
const annotatedMaxDepth = 32

var annotatedIDRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var annotatedHeadingIDRE = regexp.MustCompile(`^(.*?)[ \t]+\{#([A-Za-z0-9][A-Za-z0-9_.-]*)\}$`)
var annotatedRenderedTagRE = regexp.MustCompile(`<[^>]*>`)
var annotatedVerseTextRE = regexp.MustCompile(`^[0-9][0-9A-Za-z.: -]*$`)
var annotatedChapterTextRE = regexp.MustCompile(`^[0-9]+$`)

type annotatedMarkdownState struct {
	ids   map[string]bool
	notes map[string][]string
}

type annotatedInlineContext struct {
	depth     int
	underline uint8
	sup       bool
	note      bool
	link      bool
	role      string
}

func annotatedSourceRole(kind string) bool {
	return kind == "verse" || kind == "chapter" || kind == "initial" || kind == "gap"
}

func annotatedRenderedText(rendered string) string {
	return html.UnescapeString(annotatedRenderedTagRE.ReplaceAllString(rendered, ""))
}

func validateAnnotatedRoleText(kind, rendered string) error {
	text := strings.TrimSpace(annotatedRenderedText(rendered))
	switch kind {
	case "verse":
		if !annotatedVerseTextRE.MatchString(text) {
			return fmt.Errorf("verse must contain a printed numeric label with only letters, digits, dot, colon, hyphen or spaces")
		}
	case "chapter":
		if !annotatedChapterTextRE.MatchString(text) {
			return fmt.Errorf("chapter must contain a printed decimal number")
		}
	case "gap":
		found := false
		for _, r := range text {
			if r == '.' || r == '…' {
				found = true
				continue
			}
			if !unicode.IsSpace(r) {
				return fmt.Errorf("gap must contain only printed periods, ellipsis characters or whitespace")
			}
		}
		if !found {
			return fmt.Errorf("gap must contain a printed ellipsis")
		}
	}
	return nil
}

func validateMarkdownExtensions(values []string) error {
	seen := map[string]bool{}
	for _, value := range values {
		if value != annotatedTextExtension {
			return fmt.Errorf("unknown markdown extension %q", value)
		}
		if seen[value] {
			return fmt.Errorf("duplicate markdown extension %q", value)
		}
		seen[value] = true
	}
	return nil
}

func hasAnnotatedTextExtension(values []string) bool {
	for _, value := range values {
		if value == annotatedTextExtension {
			return true
		}
	}
	return false
}

func (a *annotatedMarkdownState) registerID(id string, generated bool) error {
	if !annotatedIDRE.MatchString(id) {
		return fmt.Errorf("invalid annotated anchor ID %q", id)
	}
	if !generated && (footnoteAnchorIDRE.MatchString(id) || id == "top" || id == "content" || id == "toc_container" || id == "toc_title") {
		return fmt.Errorf("reserved annotated anchor ID %q", id)
	}
	if a.ids[id] {
		return fmt.Errorf("duplicate annotated anchor ID %q", id)
	}
	a.ids[id] = true
	return nil
}

func parseHeadingIDForState(body string, state *markdownRenderState) (string, string) {
	if state.annotated != nil {
		if match := annotatedHeadingIDRE.FindStringSubmatch(body); match != nil {
			return strings.TrimSpace(match[1]), match[2]
		}
	}
	return parseHeadingID(body)
}

// The opt-in entry point shares the existing block grammar and publication
// validators. Its definition collector, inline vocabulary, and table separator
// recognition account for annotations without changing the default dialect.
func RenderAnnotatedMarkdownXHTML(input string) (string, error) {
	rendered, _, err := RenderAnnotatedMarkdownXHTMLDocument(input, nil, false)
	return rendered, err
}

func RenderAnnotatedMarkdownXHTMLDocument(input string, resolver MarkdownImageResolver, collectTOC bool) (string, []TOCEntry, error) {
	input = strings.ReplaceAll(strings.ReplaceAll(input, "\r\n", "\n"), "\r", "\n")
	if err := ValidateMarkdownSource(input, 1); err != nil {
		return "", nil, err
	}
	lines, defs, notes, err := collectAnnotatedFootnotes(input)
	if err != nil {
		return "", nil, err
	}
	state := &markdownRenderState{
		defs: defs, used: map[string]bool{}, imageResolver: resolver,
		collectTOC: collectTOC, tocIDs: map[string]bool{},
		annotated: &annotatedMarkdownState{ids: map[string]bool{}, notes: notes},
	}
	rendered, err := renderMarkdownBlocks(lines, state)
	if err != nil {
		return "", nil, fmt.Errorf("annotated-text-v1: %w", err)
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

// Definitions are top-level and may continue by four spaces. Blank lines only
// belong to a definition when followed by another four-space continuation.
// Fenced examples are never interpreted as definitions.
func collectAnnotatedFootnotes(input string) ([]string, map[string]string, map[string][]string, error) {
	lines := strings.Split(input, "\n")
	out := []string{}
	defs := map[string]string{}
	notes := map[string][]string{}
	fence := ""
	for i := 0; i < len(lines); {
		line := lines[i]
		if fence != "" {
			out = append(out, line)
			if strings.HasPrefix(strings.TrimLeft(line, " \t"), fence) {
				fence = ""
			}
			i++
			continue
		}
		if marker, ok := fencedCodeMarker(line); ok {
			fence = marker
			out = append(out, line)
			i++
			continue
		}
		match := footnoteDefRE.FindStringSubmatch(line)
		if match == nil {
			out = append(out, line)
			i++
			continue
		}
		n, first := match[1], match[2]
		if _, exists := defs[n]; exists {
			return nil, nil, nil, fmt.Errorf("line %d: duplicate footnote definition: %s", i+1, n)
		}
		if first == "" {
			return nil, nil, nil, fmt.Errorf("line %d: empty footnote definition: %s", i+1, n)
		}
		paragraphs := []string{}
		current := []string{first}
		if err := validateAnnotatedNoteLine(first); err != nil {
			return nil, nil, nil, fmt.Errorf("line %d: footnote %s: %w", i+1, n, err)
		}
		i++
		for i < len(lines) {
			if strings.TrimSpace(lines[i]) == "" {
				j := i
				for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
					j++
				}
				if j >= len(lines) || !strings.HasPrefix(lines[j], "    ") {
					break
				}
				paragraphs = append(paragraphs, strings.Join(current, "\n"))
				current = nil
				i = j
			}
			if !strings.HasPrefix(lines[i], "    ") {
				break
			}
			continuation := lines[i][4:]
			if err := validateAnnotatedNoteLine(continuation); err != nil {
				return nil, nil, nil, fmt.Errorf("line %d: footnote %s: %w", i+1, n, err)
			}
			current = append(current, continuation)
			i++
		}
		if len(current) > 0 {
			paragraphs = append(paragraphs, strings.Join(current, "\n"))
		}
		defs[n] = strings.Join(paragraphs, "\n\n")
		notes[n] = paragraphs
		// Keep a block boundary where a definition was removed.
		out = append(out, "")
	}
	return out, defs, notes, nil
}

func validateAnnotatedNoteLine(line string) error {
	trimmed := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ") {
		return fmt.Errorf("note continuation must contain inline prose, not an indented block")
	}
	if footnoteDefRE.MatchString(trimmed) || atxHeadingRE.MatchString(trimmed) || isBlockquoteLine(trimmed) || isHorizontalRule(trimmed) {
		return fmt.Errorf("note bodies support inline prose only")
	}
	if _, ok := fencedCodeMarker(trimmed); ok {
		return fmt.Errorf("note bodies do not support fenced blocks")
	}
	if _, ok := parseListMarker(trimmed); ok {
		return fmt.Errorf("note bodies do not support lists")
	}
	if strings.HasPrefix(trimmed, "|") {
		return fmt.Errorf("note bodies do not support tables")
	}
	return nil
}

func annotationError(text string, offset int, message string) error {
	prefix := text[:offset]
	line := 1 + strings.Count(prefix, "\n")
	column := offset + 1
	if end := strings.LastIndexByte(prefix, '\n'); end >= 0 {
		column = offset - end
	}
	return fmt.Errorf("inline line %d, column %d: %s", line, column, message)
}

// annotationAt returns only the closed, finite vocabulary. Unknown typed forms
// are errors in this opt-in dialect; ordinary braces remain ordinary text.
func annotationAt(text string, start int) (kind, body string, next int, ok bool, err error) {
	if strings.HasPrefix(text[start:], "{br}") {
		return "br", "", start + 4, true, nil
	}
	if text[start] != '{' {
		return "", "", start, false, nil
	}
	for _, name := range []string{"u-solid", "u-dotted", "sup", "anchor", "br", "verse", "chapter", "initial", "gap"} {
		prefix := "{" + name
		if strings.HasPrefix(text[start:], prefix) {
			after := start + len(prefix)
			if after == len(text) || strings.ContainsRune("|} \t\n\r", rune(text[after])) {
				if name == "br" || after == len(text) || text[after] != '|' {
					return "", "", start, false, annotationError(text, start, "malformed annotated form "+name)
				}
			}
		}
	}
	pipe := start + 1
	for pipe < len(text) && text[pipe] != '|' {
		ch := text[pipe]
		if !(isASCIILetter(ch) || ch >= '0' && ch <= '9' || ch == '-' || ch == '_') {
			return "", "", start, false, nil
		}
		pipe++
	}
	if pipe == len(text) {
		return "", "", start, false, nil
	}
	kind = text[start+1 : pipe]
	if strings.ContainsAny(kind, " \t\n\r{}[]\\") {
		return "", "", start, false, nil
	}
	if kind != "u-solid" && kind != "u-dotted" && kind != "sup" && kind != "anchor" && !annotatedSourceRole(kind) {
		return "", "", start, false, annotationError(text, start, "unknown annotated form "+kind)
	}
	depth := 1
	for i := pipe + 1; i < len(text); i++ {
		if text[i] == '\\' && i+1 < len(text) && isEscapableMarkdownByte(text[i+1]) {
			i++
			continue
		}
		if text[i] == '`' {
			end := strings.IndexByte(text[i+1:], '`')
			if end < 0 {
				return "", "", start, false, annotationError(text, i, "unclosed inline code span")
			}
			i += end + 1
			continue
		}
		if text[i] == '{' {
			depth++
			if depth > annotatedMaxDepth {
				return "", "", start, false, annotationError(text, i, "annotation nesting exceeds 32")
			}
		}
		if text[i] == '}' {
			depth--
			if depth == 0 {
				return kind, text[pipe+1 : i], i + 1, true, nil
			}
		}
	}
	return "", "", start, false, annotationError(text, start, "unclosed annotated form "+kind)
}

// Split only table delimiters, preserving annotation bodies, code spans, and
// Markdown escapes for the inline renderer. A malformed inline construct leaves
// the line outside the table so the ordinary renderer reports its source error.
func splitAnnotatedTableRow(line string) []string {
	line = strings.TrimSpace(line)
	var cells []string
	start := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '\\':
			if i+1 < len(line) && isEscapableMarkdownByte(line[i+1]) {
				i++
			}
		case '`':
			end := strings.IndexByte(line[i+1:], '`')
			if end < 0 {
				return nil
			}
			i += end + 1
		case '{':
			_, _, next, ok, err := annotationAt(line, i)
			if err != nil {
				return nil
			}
			if ok {
				i = next - 1
			}
		case '|':
			cells = append(cells, strings.TrimSpace(line[start:i]))
			start = i + 1
		}
	}
	if len(cells) == 0 {
		return nil
	}
	cells = append(cells, strings.TrimSpace(line[start:]))
	if line[0] == '|' {
		cells = cells[1:]
	}
	if start == len(line) {
		cells = cells[:len(cells)-1]
	}
	return cells
}

func renderAnnotatedInline(text string, state *markdownRenderState, opts inlineOptions, ctx annotatedInlineContext) (string, error) {
	if ctx.depth > annotatedMaxDepth {
		return "", fmt.Errorf("annotation nesting exceeds 32")
	}
	var out strings.Builder
	for i := 0; i < len(text); {
		if text[i] == '\\' {
			if i+1 < len(text) && isEscapableMarkdownByte(text[i+1]) {
				out.WriteString(html.EscapeString(text[i+1 : i+2]))
				i += 2
				continue
			}
			out.WriteByte('\\')
			i++
			continue
		}
		if text[i] == '`' {
			end := strings.IndexByte(text[i+1:], '`')
			if end < 0 {
				return "", annotationError(text, i, "unclosed inline code span")
			}
			out.WriteString("<code>" + html.EscapeString(text[i+1:i+1+end]) + "</code>")
			i += end + 2
			continue
		}
		if text[i] == '{' {
			kind, body, next, ok, err := annotationAt(text, i)
			if err != nil {
				return "", err
			}
			if ok {
				if kind == "br" {
					if ctx.sup || ctx.role != "" {
						return "", annotationError(text, i, "superscript and source roles cannot contain a line break")
					}
					out.WriteString("<br />")
					i = next
					continue
				}
				if kind == "anchor" {
					if ctx.sup || ctx.link || ctx.role != "" {
						return "", annotationError(text, i, "anchor is not permitted in superscript, source roles or link text")
					}
					if err := state.annotated.registerID(body, false); err != nil {
						return "", annotationError(text, i, err.Error())
					}
					out.WriteString(`<span id="` + body + `"></span>`)
					i = next
					continue
				}
				if body == "" {
					return "", annotationError(text, i, "empty annotated form "+kind)
				}
				if kind == "initial" {
					if ctx.sup || ctx.role != "" || !utf8.ValidString(body) || utf8.RuneCountInString(body) != 1 {
						return "", annotationError(text, i, "initial must be one Unicode letter and cannot be nested in another source role or superscript")
					}
					r, _ := utf8.DecodeRuneInString(body)
					if !unicode.IsLetter(r) {
						return "", annotationError(text, i, "initial must be one Unicode letter")
					}
					out.WriteString(`<span class="smol-initial">` + html.EscapeString(body) + `</span>`)
					i = next
					continue
				}
				child := ctx
				child.depth++
				tagStart, tagEnd := "", "</span>"
				if kind == "sup" {
					if ctx.sup || ctx.role != "" {
						return "", annotationError(text, i, "superscript cannot be nested in superscript or source roles")
					}
					child.sup = true
					tagStart, tagEnd = "<sup>", "</sup>"
				} else if annotatedSourceRole(kind) {
					if ctx.sup || ctx.role != "" {
						return "", annotationError(text, i, "source roles cannot be nested in another source role or superscript")
					}
					child.role = kind
					tagStart = `<span class="smol-` + kind + `">`
				} else {
					bit := uint8(1)
					if kind == "u-dotted" {
						bit = 2
					}
					if ctx.underline&bit != 0 {
						return "", annotationError(text, i, "duplicate nested underline kind "+kind)
					}
					child.underline |= bit
					tagStart = `<span class="smol-` + kind + `">`
				}
				rendered, err := renderAnnotatedInline(body, state, opts, child)
				if err != nil {
					return "", err
				}
				if annotatedSourceRole(kind) {
					if err := validateAnnotatedRoleText(kind, rendered); err != nil {
						return "", annotationError(text, i, err.Error())
					}
				}
				visible := annotatedRenderedText(rendered)
				if (kind == "u-solid" || kind == "u-dotted") && visible != "" && strings.TrimSpace(visible) == "" {
					tagStart = `<span class="smol-` + kind + ` smol-marked-space">`
				}
				out.WriteString(tagStart + rendered + tagEnd)
				i = next
				continue
			}
		}
		if strings.HasPrefix(text[i:], "![") {
			if ctx.note || ctx.sup || ctx.link || ctx.role != "" {
				return "", annotationError(text, i, "images are not permitted in notes, superscript, source roles or link text")
			}
			if opts.images {
				rendered, next, err := renderInlineImage(text, i, state)
				if err != nil {
					return "", err
				}
				out.WriteString(rendered)
				i = next
				continue
			}
		}
		if strings.HasPrefix(text[i:], "[^") {
			if match := footnoteRefRE.FindStringSubmatch(text[i:]); match != nil {
				if ctx.note || ctx.sup || ctx.link || ctx.role != "" {
					return "", annotationError(text, i, "note controls are not permitted in notes, superscript, source roles or link text")
				}
				if opts.footnotes {
					rendered, err := renderAnnotatedFootnote(match[1], state)
					if err != nil {
						return "", err
					}
					out.WriteString(rendered)
					i += len(match[0])
					continue
				}
			}
		}
		if text[i] == '[' && !strings.HasPrefix(text[i:], "[^") {
			close := findClosing(text, i+1, ']')
			if close >= 0 && close+1 < len(text) && text[close+1] == '(' {
				if ctx.sup || ctx.link || ctx.role != "" {
					return "", annotationError(text, i, "links are not permitted in superscript, source roles or nested link text")
				}
				if opts.links {
					end := findClosingParen(text, close+2)
					if end < 0 {
						return "", annotationError(text, i, "malformed markdown link")
					}
					target, title, err := parseDestinationAndTitle(text[close+2 : end])
					if err != nil {
						return "", err
					}
					if title != "" {
						return "", fmt.Errorf("markdown link titles are not supported")
					}
					if err := validateMarkdownLinkURL(target); err != nil {
						return "", err
					}
					child := ctx
					child.depth++
					child.link = true
					label, err := renderAnnotatedInline(text[i+1:close], state, inlineOptions{}, child)
					if err != nil {
						return "", err
					}
					out.WriteString(`<a href="` + html.EscapeString(target) + `">` + label + `</a>`)
					i = end + 1
					continue
				}
			}
		}
		if text[i] == '*' || text[i] == '_' {
			delimiter, tag := text[i:i+1], "em"
			if i+1 < len(text) && text[i+1] == text[i] {
				delimiter, tag = text[i:i+2], "strong"
			}
			if end := findAnnotatedDelimiter(text, i+len(delimiter), delimiter); end >= 0 && end > i+len(delimiter) {
				child := ctx
				child.depth++
				inner, err := renderAnnotatedInline(text[i+len(delimiter):end], state, opts, child)
				if err != nil {
					return "", err
				}
				out.WriteString("<" + tag + ">" + inner + "</" + tag + ">")
				i = end + len(delimiter)
				continue
			}
		}
		if text[i] == '<' && isInlineHTMLStart(text, i) {
			return "", annotationError(text, i, "inline HTML is not supported in markdown bodies")
		}
		out.WriteString(html.EscapeString(text[i : i+1]))
		i++
	}
	return out.String(), nil
}

func findAnnotatedDelimiter(text string, start int, delimiter string) int {
	for i := start; i < len(text); i++ {
		if text[i] == '\\' && i+1 < len(text) && isEscapableMarkdownByte(text[i+1]) {
			i++
			continue
		}
		if text[i] == '`' {
			if end := strings.IndexByte(text[i+1:], '`'); end >= 0 {
				i += end + 1
				continue
			}
		}
		if text[i] == '{' {
			_, _, next, ok, err := annotationAt(text, i)
			if err == nil && ok {
				i = next - 1
				continue
			}
		}
		if strings.HasPrefix(text[i:], delimiter) {
			return i
		}
	}
	return -1
}

func renderAnnotatedFootnote(n string, state *markdownRenderState) (string, error) {
	paragraphs, ok := state.annotated.notes[n]
	if !ok {
		return "", fmt.Errorf("missing footnote definition: %s", n)
	}
	if state.used[n] {
		return "", fmt.Errorf("repeated footnote reference: %s; use an ordinary link instead", n)
	}
	if err := state.annotated.registerID("cb"+n, true); err != nil {
		return "", err
	}
	state.used[n] = true
	var body strings.Builder
	for index, paragraph := range paragraphs {
		rendered, err := renderAnnotatedInline(paragraph, state, inlineOptions{links: true}, annotatedInlineContext{note: true})
		if err != nil {
			return "", fmt.Errorf("footnote %s: %w", n, err)
		}
		if len(paragraphs) > 1 {
			if index > 0 {
				body.WriteString("\n\n")
			}
			body.WriteString(`<span class="smol-note-paragraph">` + rendered + `</span>`)
		} else {
			body.WriteString(rendered)
		}
	}
	return `<span><input type="checkbox" id="cb` + n + `" /><label for="cb` + n + `"><sup>` + n + `</sup></label><span class="smol-note-body">` + body.String() + `</span></span>`, nil
}

// TOC labels contain only visible source text. In particular they do not repeat
// anchor registration or inline note controls while being rendered a second time.
func annotatedTOCSource(text string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(text); {
		if text[i] == '\\' && i+1 < len(text) {
			out.WriteString(text[i : i+2])
			i += 2
			continue
		}
		if text[i] == '`' {
			if end := strings.IndexByte(text[i+1:], '`'); end >= 0 {
				out.WriteString(text[i : i+end+2])
				i += end + 2
				continue
			}
		}
		if text[i] == '{' {
			kind, body, next, ok, err := annotationAt(text, i)
			if err != nil {
				return "", err
			}
			if ok {
				if kind == "br" {
					out.WriteByte(' ')
				} else if kind != "anchor" {
					inner, err := annotatedTOCSource(body)
					if err != nil {
						return "", err
					}
					out.WriteString(inner)
				}
				i = next
				continue
			}
		}
		out.WriteByte(text[i])
		i++
	}
	return out.String(), nil
}
