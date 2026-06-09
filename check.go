package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const checkModeDefault = "default"

// Unclosed style blocks are scanned to EOF so edited or foreign files fail safe.
var inlineStyleRE = regexp.MustCompile(`(?is)<style\b[^>]*>(.*?)(?:</style\s*>|\z)`)

type CheckOptions struct {
	Path   string
	Mode   string
	Stdout io.Writer
}

func CheckPath(opts CheckOptions) error {
	mode, err := normalizeCheckMode(opts.Mode)
	if err != nil {
		return err
	}
	if opts.Path == "" {
		return fmt.Errorf("check path is required")
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	info, err := os.Stat(opts.Path)
	if err != nil {
		return err
	}

	checked := 0
	failed := 0
	var firstFailure error
	checkFile := func(path string) error {
		err := checkOneFile(path, mode)
		checked++
		if err != nil {
			failed++
			if firstFailure == nil {
				firstFailure = err
			}
			fmt.Fprintf(opts.Stdout, "fail: %s: %s\n", cleanSlash(path), err)
			return nil
		}
		fmt.Fprintf(opts.Stdout, "ok: %s\n", cleanSlash(path))
		return nil
	}

	if !info.IsDir() {
		if !isCheckableFile(opts.Path) {
			return fmt.Errorf("unsupported file extension: %s", opts.Path)
		}
		if err := checkFile(opts.Path); err != nil {
			return err
		}
	} else {
		err = filepath.WalkDir(opts.Path, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				checked++
				failed++
				if firstFailure == nil {
					firstFailure = err
				}
				fmt.Fprintf(opts.Stdout, "fail: %s: %s\n", cleanSlash(path), err)
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			if !isCheckableFile(path) {
				return nil
			}
			return checkFile(path)
		})
		if err != nil {
			return err
		}
	}

	if checked == 0 {
		return fmt.Errorf("no checkable files found: %s", opts.Path)
	}
	if failed > 0 {
		return fmt.Errorf("%d file(s) failed check; first failure: %w", failed, firstFailure)
	}
	return nil
}

func normalizeCheckMode(mode string) (string, error) {
	if mode == "" || mode == checkModeDefault {
		return checkModeDefault, nil
	}
	if mode == outputModeFlatXHTML || mode == outputModeFlatGemini {
		return mode, nil
	}
	return "", fmt.Errorf("invalid check mode: %s", mode)
}

func isCheckableFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".html", ".xhtml", ".gmi":
		return true
	default:
		return false
	}
}

func checkOneFile(path, mode string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".gmi" {
		return ValidateGemtext(string(data))
	}
	if mode == outputModeFlatGemini {
		return fmt.Errorf("flat-gemini-v1 checks only .gmi files")
	}
	return CheckHTML(string(data), mode)
}

func CheckHTML(html, mode string) error {
	if err := ValidateHTMLForMode(html, mode); err != nil {
		if mode == checkModeDefault && ValidateHTMLForMode(html, outputModeFlatXHTML) == nil {
			return fmt.Errorf("%w; use --mode flat-xhtml-v1 for flat XHTML output", err)
		}
		return err
	}
	for i, match := range inlineStyleRE.FindAllStringSubmatch(html, -1) {
		if len(match) < 2 {
			continue
		}
		if err := ValidateCSS(match[1]); err != nil {
			return fmt.Errorf("inline style %d: %w", i+1, err)
		}
	}
	if _, _, err := ExtractManifest(html); err != nil {
		return err
	}
	return nil
}
