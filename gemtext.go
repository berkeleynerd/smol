package main

import (
	"fmt"
	"html"
	"strings"
)

type gemtextLineKind int

const (
	gemtextLineText gemtextLineKind = iota
	gemtextLineBlank
	gemtextLineHeading
	gemtextLineLink
	gemtextLineList
	gemtextLineQuote
	gemtextLinePre
)

type gemtextLine struct {
	kind  gemtextLineKind
	level int
	url   string
	text  string
}

func RenderGemtextXHTML(input string) (string, error) {
	lines, err := parseGemtext(input)
	if err != nil {
		return "", err
	}
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch line.kind {
		case gemtextLineBlank:
			continue
		case gemtextLineText:
			out = append(out, "<p>"+escapeGemtextText(line.text)+"</p>")
		case gemtextLineHeading:
			out = append(out, fmt.Sprintf("<h%d>%s</h%d>", line.level, escapeGemtextText(line.text), line.level))
		case gemtextLineLink:
			label := line.text
			if label == "" {
				label = line.url
			}
			out = append(out, `<p><a href="`+escapeGemtextAttr(line.url)+`">`+escapeGemtextText(label)+`</a></p>`)
		case gemtextLineList:
			var b strings.Builder
			b.WriteString("<ul>")
			for i < len(lines) && lines[i].kind == gemtextLineList {
				b.WriteString("\n  <li>")
				b.WriteString(escapeGemtextText(lines[i].text))
				b.WriteString("</li>")
				i++
			}
			i--
			b.WriteString("\n</ul>")
			out = append(out, b.String())
		case gemtextLineQuote:
			var b strings.Builder
			b.WriteString("<blockquote>")
			for i < len(lines) && lines[i].kind == gemtextLineQuote {
				b.WriteString("\n  <p>")
				b.WriteString(escapeGemtextText(lines[i].text))
				b.WriteString("</p>")
				i++
			}
			i--
			b.WriteString("\n</blockquote>")
			out = append(out, b.String())
		case gemtextLinePre:
			var b strings.Builder
			b.WriteString("<pre>")
			for i < len(lines) && lines[i].kind == gemtextLinePre {
				if i > 0 && lines[i-1].kind == gemtextLinePre {
					b.WriteByte('\n')
				}
				b.WriteString(escapeGemtextText(lines[i].text))
				i++
			}
			i--
			b.WriteString("</pre>")
			out = append(out, b.String())
		}
	}
	return strings.Join(out, "\n"), nil
}

func parseGemtext(input string) ([]gemtextLine, error) {
	var lines []gemtextLine
	inPre := false
	for _, raw := range strings.Split(input, "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if strings.HasPrefix(line, "```") {
			inPre = !inPre
			continue
		}
		if inPre {
			lines = append(lines, gemtextLine{kind: gemtextLinePre, text: line})
			continue
		}
		switch {
		case line == "":
			lines = append(lines, gemtextLine{kind: gemtextLineBlank})
		case strings.HasPrefix(line, "### "):
			lines = append(lines, gemtextLine{kind: gemtextLineHeading, level: 3, text: strings.TrimSpace(line[4:])})
		case strings.HasPrefix(line, "## "):
			lines = append(lines, gemtextLine{kind: gemtextLineHeading, level: 2, text: strings.TrimSpace(line[3:])})
		case strings.HasPrefix(line, "# "):
			lines = append(lines, gemtextLine{kind: gemtextLineHeading, level: 1, text: strings.TrimSpace(line[2:])})
		case strings.HasPrefix(line, "=>"):
			link, err := parseGemtextLink(line)
			if err != nil {
				return nil, err
			}
			lines = append(lines, link)
		case strings.HasPrefix(line, "* "):
			lines = append(lines, gemtextLine{kind: gemtextLineList, text: line[2:]})
		case strings.HasPrefix(line, ">"):
			lines = append(lines, gemtextLine{kind: gemtextLineQuote, text: strings.TrimSpace(line[1:])})
		default:
			lines = append(lines, gemtextLine{kind: gemtextLineText, text: line})
		}
	}
	return lines, nil
}

func parseGemtextLink(line string) (gemtextLine, error) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "=>"))
	if rest == "" {
		return gemtextLine{}, fmt.Errorf("invalid gemtext link: missing URL")
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return gemtextLine{}, fmt.Errorf("invalid gemtext link: missing URL")
	}
	url := fields[0]
	label := strings.TrimSpace(strings.TrimPrefix(rest, url))
	return gemtextLine{kind: gemtextLineLink, url: url, text: label}, nil
}

func escapeGemtextText(text string) string {
	return html.EscapeString(text)
}

func escapeGemtextAttr(text string) string {
	return html.EscapeString(text)
}
