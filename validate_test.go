package main

import "testing"

func TestCSSImportIsRejected(t *testing.T) {
	if err := ValidateCSS("@import 'x.css';"); err == nil {
		t.Fatalf("ValidateCSS accepted @import")
	}
}

func TestCSSURLIsRejected(t *testing.T) {
	if err := ValidateCSS("body { background: url(x.png); }"); err == nil {
		t.Fatalf("ValidateCSS accepted url(")
	}
}

func TestCSSAllowsSafeInput(t *testing.T) {
	cases := map[string]string{
		"scroll-behavior":               `scroll-behavior:smooth`,
		"overscroll-behavior":           `overscroll-behavior:none`,
		"scroll-behavior in rule":       `html { scroll-behavior:smooth; }`,
		"overscroll behavior axis":      `body { overscroll-behavior-x:contain; }`,
		"url word in comment":           `/* no url() assets bundled */ body { color: #000; }`,
		"import word in comment":        `/* @import is intentionally avoided */ a { color: red; }`,
		"behavior word in comment":      `/* avoids the IE behavior: htc trick */ p { margin: 0; }`,
		"dangerous rule commented":      `/* body { background: url(x.png); } */ p { margin: 0; }`,
		"comment delimiters in strings": `p::before { content: "/* url(x) @import */"; }`,
		"quoted resource words":         `p::before { content: 'url(x) expression(x) behavior:'; }`,
		"escaped quote":                 `p::before { content: "a\\\"/*"; color: red; }`,
	}
	for name, css := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateCSS(css); err != nil {
				t.Fatalf("ValidateCSS rejected safe CSS %q: %v", css, err)
			}
		})
	}
}

func TestCSSRejectsDangerousInput(t *testing.T) {
	cases := map[string]string{
		"url function":             `body { background: url(x.png); }`,
		"import":                   `@import 'x.css';`,
		"expression":               `div { width: expression(alert(1)); }`,
		"ie behavior with url":     `body { behavior: url(evil.htc); }`,
		"ie behavior bare":         `body { behavior: default; }`,
		"ie behavior at start":     `behavior:url(evil.htc)`,
		"ie behavior spaced":       `body { behavior : url(evil.htc); }`,
		"ie star behavior":         `body { *behavior: default; }`,
		"ie underscore behavior":   `body { _behavior: default; }`,
		"moz binding":              `a { -moz-binding: thing; }`,
		"url outside comment":      `/* themed */ body { background: url(x.png); }`,
		"quoted comment bypass":    `p::before{content:"/*"} body{background-image:url("https://example.invalid/image")} p::after{content:"*/"}`,
		"attribute comment bypass": `--a:'/*'; background-image:url('https://example.invalid/image'); --b:'*/'`,
		"escaped import":           `@\69mport "https://example.invalid/style";`,
		"escaped URL":              `p{background:u\72l("https://example.invalid/image")}`,
		"six digit URL escape":     `p{background:\000075rl("https://example.invalid/image")}`,
		"CRLF escape terminator":   "p{background:\\75\r\nrl(https://example.invalid/image)}",
		"escaped image set":        `p{background:i\6d age-set("https://example.invalid/image" 1x)}`,
		"escaped simple URL":       `p{background:u\rl("https://example.invalid/image")}`,
		"escaped behavior":         `p{b\65havior:default}`,
		"bad URL":                  `p{background:url(bad"url)}`,
		"bad string":               "p{content:\"bad\nstring\"}",
	}
	for name, css := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateCSS(css); err == nil {
				t.Fatalf("ValidateCSS accepted dangerous CSS %q", css)
			}
		})
	}
}

func TestFinalHTMLContainingScriptIsRejected(t *testing.T) {
	if err := ValidateHTML("<p>ok</p><script>alert(1)</script>"); err == nil {
		t.Fatalf("ValidateHTML accepted script")
	}
}

func TestFinalHTMLContainingEventHandlerIsRejected(t *testing.T) {
	if err := ValidateHTML(`<p onclick="alert(1)">ok</p>`); err == nil {
		t.Fatalf("ValidateHTML accepted event handler")
	}
}

func TestFinalHTMLContainingJavascriptURLIsRejected(t *testing.T) {
	if err := ValidateHTML(`<a href="javascript:alert(1)">bad</a>`); err == nil {
		t.Fatalf("ValidateHTML accepted javascript URL")
	}
}

func TestFinalHTMLContainingExternalStylesheetIsRejected(t *testing.T) {
	if err := ValidateHTML(`<link rel="stylesheet" href="https://example.org/x.css">`); err == nil {
		t.Fatalf("ValidateHTML accepted external stylesheet")
	}
}

func TestFinalHTMLContainingExternalImageReferencesIsRejected(t *testing.T) {
	tests := map[string]string{
		"double quoted src":        `<img src="https://example.org/x.png" alt="x">`,
		"single quoted src":        `<img src='http://example.org/x.png' alt="x">`,
		"unquoted src":             `<img src=https://example.org/x.png alt="x">`,
		"protocol relative src":    `<img src="//example.org/x.png" alt="x">`,
		"external srcset":          `<img src="data:image/png;base64,AA==" srcset="/x.png 1x, https://example.org/x.png 2x" alt="x">`,
		"protocol relative srcset": `<img src="data:image/png;base64,AA==" srcset="//example.org/x.png 2x" alt="x">`,
	}
	for name, html := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateHTML(html); err == nil {
				t.Fatalf("ValidateHTML accepted external image reference")
			}
		})
	}
}

func TestFinalHTMLAllowsEmbeddedImageDataURL(t *testing.T) {
	if err := ValidateHTML(`<img src="data:image/png;base64,AA==" alt="x" />`); err != nil {
		t.Fatalf("ValidateHTML rejected embedded image: %v", err)
	}
}

func TestFinalHTMLRejectsLocalImageReferences(t *testing.T) {
	tests := map[string]string{
		"relative": `<img src="assets/x.png" alt="x">`,
		"absolute": `<img src="/assets/x.png" alt="x">`,
		"empty":    `<img src="" alt="x">`,
		"srcset":   `<img src="data:image/png;base64,AA==" srcset="data:image/png;base64,AA== 1x" alt="x">`,
	}
	for name, html := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateHTML(html); err == nil {
				t.Fatalf("ValidateHTML accepted local or alternate image reference")
			}
		})
	}
}

func TestExternalAnchorLinksAreAllowed(t *testing.T) {
	if err := ValidateHTML(`<a href="https://example.org/">Example</a>`); err != nil {
		t.Fatalf("ValidateHTML rejected external anchor: %v", err)
	}
}

func TestXHTMLAllowsSVG(t *testing.T) {
	if err := ValidateHTML(`<svg xmlns="http://www.w3.org/2000/svg" role="img"><title>Clean</title></svg>`); err != nil {
		t.Fatalf("ValidateHTML rejected clean namespaced SVG")
	}
}

func TestXHTMLAllowsFootnoteCheckbox(t *testing.T) {
	html := `<span><input type="checkbox" id="cb12" /><label for="cb12"><sup>12</sup></label><span>Note.</span></span>`
	if err := ValidateHTML(html); err != nil {
		t.Fatalf("ValidateHTML rejected canonical checkbox: %v", err)
	}
}

func TestXHTMLAllowsReorderedCheckboxAttributes(t *testing.T) {
	html := `<input id="cb12" type="checkbox" />`
	if err := ValidateHTML(html); err != nil {
		t.Fatalf("ValidateHTML rejected reordered checkbox attributes: %v", err)
	}
}

func TestXHTMLAllowsCanonicalCheckbox(t *testing.T) {
	html := `<input type="checkbox" id="cb12" />`
	if err := ValidateHTML(html); err != nil {
		t.Fatalf("ValidateHTML rejected checkbox: %v", err)
	}
}

func TestXHTMLRejectsNoncanonicalInputs(t *testing.T) {
	cases := []string{
		`<input type="text" id="cb12" />`,
		`<input type="checkbox" />`,
		`<input type="checkbox" id="other12" />`,
		`<input type="checkbox" id="cb12" name="choice" />`,
		`<input type="checkbox" id="cb12" value="yes" />`,
		`<input type="checkbox" id="cb12" form="other" />`,
		`<input type="checkbox" id="cb12" checked="checked" />`,
		`<input type="checkbox" id="cb12" style="display:none" />`,
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			if err := ValidateXHTMLDocument(xhtmlDocument("", input)); err == nil {
				t.Fatalf("accepted noncanonical input: %s", input)
			}
		})
	}
}

func TestXHTMLAllowsCleanSVG(t *testing.T) {
	html := `<svg xmlns="http://www.w3.org/2000/svg" role="img" viewBox="0 0 1 1"><title>Clean</title><path d="M0 0h1v1z"/></svg>`
	if err := ValidateHTML(html); err != nil {
		t.Fatalf("ValidateHTML rejected clean SVG: %v", err)
	}
}

func TestXHTMLRejectsUnsafeSVG(t *testing.T) {
	tests := map[string]string{
		"event":        `<svg onload="alert(1)"></svg>`,
		"script":       `<svg><script>alert(1)</script></svg>`,
		"external use": `<svg><use href="https://example.org/x.svg#id"></use></svg>`,
		"external img": `<svg><image xlink:href="//example.org/x.png"></image></svg>`,
	}
	for name, html := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateHTML(html); err == nil {
				t.Fatalf("ValidateHTML accepted unsafe SVG")
			}
		})
	}
}
