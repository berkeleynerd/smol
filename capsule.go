package main

import (
	"fmt"
	"os"
	"path/filepath"
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

	outDir := opts.OutDir
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(siteDir, outDir)
	}
	if opts.Force {
		if err := os.RemoveAll(outDir); err != nil {
			return err
		}
	}

	for _, page := range pages {
		rendered, err := renderGeminiPage(page)
		if err != nil {
			return err
		}
		finalPath := geminiOutputPathFor(outDir, page.URL)
		if err := atomicWriteFile(finalPath, []byte(rendered), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(opts.Stdout, "built: %s\n", cleanSlash(relativeDisplay(siteDir, finalPath)))
	}
	fmt.Fprintln(opts.Stdout, "note: generated unsigned Gemini capsule; signing for flat-gemini-v1 is not supported yet")
	return nil
}

func renderGeminiPage(page Page) (string, error) {
	if page.BodyFormat != bodyFormatGemtext {
		return "", fmt.Errorf("flat-gemini-v1 supports only gemtext-v1 bodies")
	}
	bodyBytes, err := os.ReadFile(page.bodyPath)
	if err != nil {
		return "", err
	}
	body := string(bodyBytes)
	if err := ValidateGemtext(body); err != nil {
		return "", err
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
