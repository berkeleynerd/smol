package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestAnnotatedMarkdownLiteralStylesAndParagraphNotes(t *testing.T) {
	input := "{anchor|Luke.4.31}{sup|31} {u-solid|**_Jesus_** {u-dotted|went}}{u-solid|   }P{sup|69}{br}Next.[^129]\n\n" +
		"[^129]: {anchor|fn-129}First *paragraph*, with [source](bibliography.html#bib-1).\n" +
		"    A soft continuation.\n\n" +
		"    Second paragraph with {u-dotted|literal wording}.\n"
	got, err := RenderAnnotatedMarkdownXHTML(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<span id="Luke.4.31"></span><sup>31</sup>`,
		`<span class="smol-u-solid"><strong><em>Jesus</em></strong> <span class="smol-u-dotted">went</span></span>`,
		`<span class="smol-u-solid smol-marked-space">   </span>`, `<sup>69</sup><br />`,
		`<span class="smol-note-paragraph"><span id="fn-129"></span>First <em>paragraph</em>`,
		"A soft continuation.</span>\n\n<span class=\"smol-note-paragraph\">Second paragraph",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Count(got, `class="smol-note-paragraph"`) != 2 {
		t.Fatalf("paragraph boundaries lost: %s", got)
	}
	if err := ValidateXHTMLDocument(xhtmlDocument("", got)); err != nil {
		t.Fatal(err)
	}
}

func TestAnnotatedMarkdownNoteBodyStyleIsolationLeavesLegacyUnchanged(t *testing.T) {
	input := "Text.[^26]\n\n[^26]: Note with *emphasis* and [source](https://example.org)."
	legacy, err := RenderMarkdownXHTML(input)
	if err != nil {
		t.Fatal(err)
	}
	annotated, err := RenderAnnotatedMarkdownXHTML(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(legacy, "smol-note-body") {
		t.Fatalf("legacy markup changed: %s", legacy)
	}
	if !strings.Contains(annotated, `<span class="smol-note-body">Note with <em>emphasis</em>`) {
		t.Fatalf("note isolation class or child emphasis missing: %s", annotated)
	}
	if strings.Replace(annotated, `<span class="smol-note-body">`, `<span>`, 1) != legacy {
		t.Fatalf("single-line wrapper changed beyond body isolation:\n%s\nwant:\n%s", annotated, legacy)
	}
}

func TestAnnotatedMarkdownDefinitionsInsideFencedCodeStayLiteral(t *testing.T) {
	input := "```text\n[^1]: Example, not a definition.\n{anchor|top}\n```\n\nReal.[^2]\n\n[^2]: Real note."
	got, err := RenderAnnotatedMarkdownXHTML(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "[^1]: Example, not a definition.") || strings.Contains(got, `id="cb1"`) {
		t.Fatalf("code was extracted: %s", got)
	}
	if !strings.Contains(got, `id="cb2"`) {
		t.Fatalf("actual note missing: %s", got)
	}
}

func TestAnnotatedMarkdownTOCUsesVisibleTextAndStableIDs(t *testing.T) {
	got, toc, err := RenderAnnotatedMarkdownXHTMLDocument("## {u-solid|**Greek**} {sup|2} {#chapter.2}\n\n{anchor|verse.2.1}Text.", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(toc) != 1 || toc[0].Href != "#chapter.2" || string(toc[0].Text) != "<strong>Greek</strong> 2" {
		t.Fatalf("bad TOC %#v", toc)
	}
	if !strings.Contains(got, `id="chapter.2"`) {
		t.Fatal(got)
	}
	_, toc, err = RenderAnnotatedMarkdownXHTMLDocument("## {u-solid|Greek} {sup|2}\n", nil, true)
	if err != nil || len(toc) != 1 || toc[0].Href != "#greek-2" {
		t.Fatalf("derived TOC %#v: %v", toc, err)
	}
}

func TestAnnotatedMarkdownEscapesAndControlledNoteWithinUnderline(t *testing.T) {
	input := "\\{u-solid|literal\\} {u-solid|A \\} \\| B[^1]}\n\n[^1]: Note."
	got, err := RenderAnnotatedMarkdownXHTML(input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "{u-solid|literal}") || !strings.Contains(got, `A } | B<span><input type="checkbox"`) {
		t.Fatal(got)
	}
}

func TestAnnotatedMarkdownRejectsMalformedAndAmbiguousInput(t *testing.T) {
	for _, input := range []string{
		"{u-solid}", "{u-dotted", "{sup class=x|one}", "{br extra}", "{unknown|one}",
		"Text[^1] again[^1]\n\n[^1]: Note.",
		"Text[^1]", "Text.\n\n[^1]: Unused.",
		"Text[^1]\n\n[^1]: One.\n[^1]: Two.",
		"Text[^1]\n\n[^1]: One.\n\n    > quote",
		"Text[^1]\n\n[^1]: One.\n\n    ```text\n    block\n    ```",
		"Text[^1]\n\n[^1]: One.\n\n    | A | B |",
	} {
		if got, err := RenderAnnotatedMarkdownXHTML(input); err == nil {
			t.Errorf("accepted %q: %s", input, got)
		}
	}
}

func TestAnnotatedMarkdownExtensionMetadataIsExplicitAndStrict(t *testing.T) {
	for _, header := range []string{"markdown_extensions: [annotated-text-v1]", "markdown_extensions:\n  - annotated-text-v1"} {
		meta, _, err := parseContentFrontMatter([]byte("---\n"+header+"\n---\nBody."), "page.md")
		if err != nil || !reflect.DeepEqual(meta.MarkdownExtensions, []string{annotatedTextExtension}) {
			t.Fatalf("metadata %#v: %v", meta, err)
		}
		if err := validateContentMeta(meta, "page", "sample", bodyFormatMarkdownXHTML, outputModeFlatGemini); err == nil || !strings.Contains(err.Error(), "XHTML-only") {
			t.Fatalf("Gemini accepted extension: %v", err)
		}
		if err := validateContentMeta(meta, "page", "sample", bodyFormatHTML, "xhtml5"); err == nil {
			t.Fatal("HTML body accepted markdown extension")
		}
	}
	for _, header := range []string{
		"markdown_extensions: [unknown-v1]", "markdown_extensions: [annotated-text-v1, annotated-text-v1]",
		"markdown_extensions: [annotated-text-v1]\nmarkdown_extensions: []",
		"foreign:\n  markdown_extensions: [annotated-text-v1]",
	} {
		if _, _, err := parseContentFrontMatter([]byte("---\n"+header+"\n---\nBody."), "page.md"); err == nil {
			t.Errorf("accepted metadata %s", header)
		}
	}
}

func TestAnnotatedSourceRolesStaySeparateFromTrueSuperscript(t *testing.T) {
	input := `{verse|{u-solid|11}b.} {chapter|**3**} {initial|P}aul {gap|{u-dotted|. .} .} P{sup|69}`
	got, err := RenderAnnotatedMarkdownXHTML(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<span class="smol-verse"><span class="smol-u-solid">11</span>b.</span>`,
		`<span class="smol-chapter"><strong>3</strong></span>`,
		`<span class="smol-initial">P</span>aul`,
		`<span class="smol-gap"><span class="smol-u-dotted">. .</span> .</span>`,
		`P<sup>69</sup>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Count(got, "<sup>") != 1 {
		t.Fatalf("baseline roles became superscripts: %s", got)
	}
	if err := ValidateXHTMLDocument(xhtmlDocument("", got)); err != nil {
		t.Fatal(err)
	}
	legacy, err := RenderMarkdownXHTML(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, class := range []string{"smol-verse", "smol-chapter", "smol-initial", "smol-gap"} {
		if strings.Contains(legacy, class) {
			t.Fatalf("legacy source acquired %s", class)
		}
	}
}

func TestAnnotatedSourceRoleSafetyAndLimits(t *testing.T) {
	for _, input := range []string{
		`{initial|Paul}`, `{initial| P}`, `{initial|1}`, `{initial|**P**}`, `{initial|[P](https://example.org)}`,
		`{verse|}`, `{verse|words}`, `{chapter|one}`, `{chapter|1a}`, `{gap|words}`, `{gap|   }`,
		`{verse|1{anchor|wrong}}`, `{chapter|1{br}}`, `{verse|{sup|1}}`, `{sup|{verse|1}}`,
		`{gap|{gap|. . .}}`, `{verse|1[link](https://example.org)}`, `{chapter|1![x](assets/a.png)}`,
		"{verse|1[^1]}\n\n[^1]: Note.", `{chapter class=x|1}`, `{verse}`, `{initial`, `{gap style=x|...}`,
		`{initial|<b>P</b>}`, `{gap|&#46;}`, `{verse|1" onclick="run()}`, `{chapter|1; color:red}`,
	} {
		if got, err := RenderAnnotatedMarkdownXHTML(input); err == nil {
			t.Errorf("accepted %q: %s", input, got)
		}
	}
	got, err := RenderAnnotatedMarkdownXHTML(`{initial|Ω} {verse|4:27} {gap|… . .}`)
	if err != nil || !strings.Contains(got, `class="smol-initial">Ω`) {
		t.Fatalf("valid literal roles: %s %v", got, err)
	}
}

func TestAnnotatedMarkedSpacesAreExplicitAndDoNotAffectTextRuns(t *testing.T) {
	got, err := RenderAnnotatedMarkdownXHTML("{u-solid|   }{u-dotted|\u00a0}{u-solid|word }{u-solid|{u-dotted| }}{u-solid|{anchor|empty}}")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got, "smol-marked-space") != 4 {
		t.Fatalf("wrong whitespace classification: %s", got)
	}
	if !strings.Contains(got, `<span class="smol-u-solid">word </span>`) {
		t.Fatalf("text run acquired whitespace-only class: %s", got)
	}
	if err := ValidateXHTMLDocument(xhtmlDocument("", got)); err != nil {
		t.Fatal(err)
	}
}

func TestAnnotatedTablesPreserveCellBoundaries(t *testing.T) {
	cases := []struct {
		name, input, header, row string
	}{
		{
			name:   "body annotation",
			input:  "| Reading | Witness |\n| --- | --- |\n| {u-solid|word} | A |",
			header: "<tr><th>Reading</th><th>Witness</th></tr>",
			row:    `<tr><td><span class="smol-u-solid">word</span></td><td>A</td></tr>`,
		},
		{
			name:   "annotated header after paragraph",
			input:  "Introduction.\n| {u-solid|Reading} | Witness |\n| --- | --- |\n| word | A |",
			header: `<tr><th><span class="smol-u-solid">Reading</span></th><th>Witness</th></tr>`,
			row:    "<tr><td>word</td><td>A</td></tr>",
		},
		{
			name:   "nested annotation and code without outer delimiters",
			input:  "Reading | Witness\n--- | ---\n{u-solid|one {u-dotted|two `x|y`}} | `A|B`",
			header: "<tr><th>Reading</th><th>Witness</th></tr>",
			row:    `<tr><td><span class="smol-u-solid">one <span class="smol-u-dotted">two <code>x|y</code></span></span></td><td><code>A|B</code></td></tr>`,
		},
		{
			name:   "escaped cell and annotation pipes",
			input:  "Reading | Witness\n--- | ---\n{u-solid|one \\| two \\} three} | A\\|",
			header: "<tr><th>Reading</th><th>Witness</th></tr>",
			row:    `<tr><td><span class="smol-u-solid">one | two } three</span></td><td>A|</td></tr>`,
		},
		{
			name:   "blockquote table",
			input:  "> | {verse|31} | Witness |\n> | --- | --- |\n> | {anchor|reading.31}word | A |",
			header: `<tr><th><span class="smol-verse">31</span></th><th>Witness</th></tr>`,
			row:    `<tr><td><span id="reading.31"></span>word</td><td>A</td></tr>`,
		},
		{
			name:   "loose list table",
			input:  "- Reading:\n\n  | {u-solid|Reading} | Witness |\n  | --- | --- |\n  | word | A |",
			header: `<tr><th><span class="smol-u-solid">Reading</span></th><th>Witness</th></tr>`,
			row:    "<tr><td>word</td><td>A</td></tr>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RenderAnnotatedMarkdownXHTML(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(got, "<table>") != 1 || !strings.Contains(got, tc.header) || !strings.Contains(got, tc.row) {
				t.Fatalf("table structure or cell content changed:\n%s", got)
			}
			if err := CheckHTML(xhtmlDocument("", got)); err != nil {
				t.Fatalf("rendered table failed document validation: %v\n%s", err, got)
			}
		})
	}
}

func TestAnnotatedTablesDoNotHideInvalidForms(t *testing.T) {
	for _, input := range []string{
		"| {unknown|Reading} | Witness |\n| --- | --- |\n| word | A |",
		"| Reading | Witness |\n| --- | --- |\n| {unknown|word} | A |",
		"| Reading | Witness |\n| --- | --- |\n| {u-solid|word | A |",
		"| Reading | Witness |\n| --- | --- |\n| {u-solid|one {u-solid|two}} | A |",
	} {
		if got, err := RenderAnnotatedMarkdownXHTML(input); err == nil {
			t.Fatalf("table splitting hid invalid annotation %q:\n%s", input, got)
		}
	}
}

func TestAnnotatedTableSupportDoesNotChangeLegacyRendering(t *testing.T) {
	input := "| Reading | Witness |\n| --- | --- |\n| {u-solid|word} | A |"
	got, err := RenderMarkdownXHTML(input)
	if err != nil {
		t.Fatal(err)
	}
	want := "<table>\n<thead>\n<tr><th>Reading</th><th>Witness</th></tr>\n</thead>\n<tbody>\n</tbody>\n</table>\n<p>| {u-solid|word} | A |</p>"
	if got != want {
		t.Fatalf("legacy rendering changed without opt-in:\n%s\nwant:\n%s", got, want)
	}
}
