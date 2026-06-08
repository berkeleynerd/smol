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

func TestExternalAnchorLinksAreAllowed(t *testing.T) {
	if err := ValidateHTML(`<a href="https://example.org/">Example</a>`); err != nil {
		t.Fatalf("ValidateHTML rejected external anchor: %v", err)
	}
}

func TestDefaultHTMLRejectsSVG(t *testing.T) {
	if err := ValidateHTML(`<svg role="img"><title>Clean</title></svg>`); err == nil {
		t.Fatalf("ValidateHTML accepted SVG in default mode")
	}
}

func TestFlatXHTMLAllowsCanonicalCheckbox(t *testing.T) {
	html := `<span><input type="checkbox" id="cb12" /><label for="cb12"><sup>12</sup></label><span>Note.</span></span>`
	if err := ValidateHTMLForMode(html, outputModeFlatXHTML); err != nil {
		t.Fatalf("ValidateHTMLForMode rejected canonical checkbox: %v", err)
	}
}

func TestFlatXHTMLRejectsMalformedCheckbox(t *testing.T) {
	html := `<input id="cb12" type="checkbox" />`
	if err := ValidateHTMLForMode(html, outputModeFlatXHTML); err == nil {
		t.Fatalf("ValidateHTMLForMode accepted malformed checkbox")
	}
}

func TestDefaultHTMLRejectsCanonicalCheckbox(t *testing.T) {
	html := `<input type="checkbox" id="cb12" />`
	if err := ValidateHTML(html); err == nil {
		t.Fatalf("ValidateHTML accepted checkbox in default mode")
	}
}

func TestFlatXHTMLAllowsCleanSVG(t *testing.T) {
	html := `<svg role="img" viewBox="0 0 1 1"><title>Clean</title><path d="M0 0h1v1z"/></svg>`
	if err := ValidateHTMLForMode(html, outputModeFlatXHTML); err != nil {
		t.Fatalf("ValidateHTMLForMode rejected clean SVG: %v", err)
	}
}

func TestFlatXHTMLRejectsUnsafeSVG(t *testing.T) {
	tests := map[string]string{
		"event":        `<svg onload="alert(1)"></svg>`,
		"script":       `<svg><script>alert(1)</script></svg>`,
		"external use": `<svg><use href="https://example.org/x.svg#id"></use></svg>`,
		"external img": `<svg><image xlink:href="//example.org/x.png"></image></svg>`,
	}
	for name, html := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateHTMLForMode(html, outputModeFlatXHTML); err == nil {
				t.Fatalf("ValidateHTMLForMode accepted unsafe SVG")
			}
		})
	}
}
