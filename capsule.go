package main

import (
	"fmt"
	"os"
	"strings"
)

func buildGeminiCapsule(siteDir string, siteCfg SiteConfig, opts BuildOptions) error {
	if opts.SignKey != "" {
		return fmt.Errorf("signing is not supported for flat-gemini-v1")
	}
	pages, posts, err := LoadContent(siteDir, siteCfg)
	if err != nil {
		return err
	}
	pages = nonDraft(pages)
	posts = nonDraft(posts)
	if len(posts) > 0 {
		return fmt.Errorf("flat-gemini-v1 supports only pages")
	}

	outDir := resolveOutputDir(siteDir, opts.OutDir)
	if opts.Force {
		if err := validateForceOutputDir(siteDir, outDir); err != nil {
			return err
		}
	}

	tempDir, err := os.MkdirTemp(siteDir, ".smol-gemini-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	results := []renderResult{}
	for _, page := range pages {
		rendered, err := renderGeminiPage(page)
		if err != nil {
			return err
		}
		tempPath := geminiOutputPathFor(tempDir, page.URL)
		if err := atomicWriteFile(tempPath, []byte(rendered), 0o644); err != nil {
			return err
		}
		results = append(results, renderResult{
			page:      page,
			route:     page.URL,
			tempPath:  tempPath,
			finalPath: geminiOutputPathFor(outDir, page.URL),
		})
	}

	if opts.Force {
		if err := os.RemoveAll(outDir); err != nil {
			return err
		}
	}
	for _, result := range results {
		data, err := os.ReadFile(result.tempPath)
		if err != nil {
			return err
		}
		if err := atomicWriteFile(result.finalPath, data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(opts.Stdout, "built: %s\n", cleanSlash(relativeDisplay(siteDir, result.finalPath)))
	}
	fmt.Fprintln(opts.Stdout, "note: generated unsigned Gemini capsule; signing for flat-gemini-v1 is not supported yet")
	return nil
}

func renderGeminiPage(page Page) (string, error) {
	bodyBytes, err := os.ReadFile(page.bodyPath)
	if err != nil {
		return "", err
	}
	body := string(bodyBytes)
	switch page.BodyFormat {
	case bodyFormatGemtext:
		if err := ValidateGemtext(body); err != nil {
			return "", err
		}
	case bodyFormatMarkdownXHTML:
		body, err = RenderMarkdownGemtext(body)
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("flat-gemini-v1 supports only markdown-xhtml-v1 or gemtext-v1 bodies")
	}
	var out strings.Builder
	out.WriteString("# ")
	out.WriteString(singleLineGemtext(page.Title))
	out.WriteByte('\n')
	if body == "" {
		return out.String(), nil
	}
	out.WriteByte('\n')
	out.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		out.WriteByte('\n')
	}
	return out.String(), nil
}

func singleLineGemtext(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.Join(strings.Fields(value), " ")
}
