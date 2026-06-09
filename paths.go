package main

import (
	"path"
	"path/filepath"
	"strings"
)

func routeFor(kind, slug, outputMode string) string {
	if outputMode == outputModeFlatXHTML {
		if slug == "index" {
			return "/"
		}
		return "/" + slug + ".html"
	}
	if outputMode == outputModeFlatGemini {
		if slug == "index" {
			return "/"
		}
		return "/" + slug + ".gmi"
	}
	if kind == "page" {
		if slug == "index" {
			return "/"
		}
		return "/" + slug + "/"
	}
	return "/posts/" + slug + "/"
}

func outputPathFor(outDir, route string) string {
	if route == "/" {
		return filepath.Join(outDir, "index.html")
	}
	trimmed := strings.Trim(route, "/")
	if strings.HasSuffix(trimmed, ".html") {
		return filepath.Join(outDir, filepath.FromSlash(trimmed))
	}
	parts := strings.Split(trimmed, "/")
	parts = append(parts, "index.html")
	return filepath.Join(append([]string{outDir}, parts...)...)
}

func geminiOutputPathFor(outDir, route string) string {
	if route == "/" {
		return filepath.Join(outDir, "index.gmi")
	}
	trimmed := strings.Trim(route, "/")
	if strings.HasSuffix(trimmed, ".gmi") {
		return filepath.Join(outDir, filepath.FromSlash(trimmed))
	}
	parts := strings.Split(trimmed, "/")
	parts = append(parts, "index.gmi")
	return filepath.Join(append([]string{outDir}, parts...)...)
}

func canonicalURL(baseURL, route string) string {
	return strings.TrimRight(baseURL, "/") + route
}

func relativeHref(fromRoute, targetRoute, outputMode string) string {
	target := hrefPathForRoute(targetRoute, outputMode, false)
	base := path.Dir(hrefPathForRoute(fromRoute, outputMode, true))
	if base == "." {
		base = ""
	}
	rel, err := filepath.Rel(filepath.FromSlash(base), filepath.FromSlash(target))
	if err != nil {
		return target
	}
	rel = filepath.ToSlash(rel)
	if strings.HasSuffix(target, "/") && !strings.HasSuffix(rel, "/") {
		rel += "/"
	}
	return rel
}

func hrefPathForRoute(route, outputMode string, asFile bool) string {
	if route == "/" {
		if outputMode == outputModeFlatGemini {
			return "index.gmi"
		}
		return "index.html"
	}
	trimmed := strings.Trim(route, "/")
	if outputMode == outputModeFlatXHTML || strings.HasSuffix(trimmed, ".html") {
		return trimmed
	}
	if outputMode == outputModeFlatGemini || strings.HasSuffix(trimmed, ".gmi") {
		return trimmed
	}
	if asFile {
		return path.Join(trimmed, "index.html")
	}
	return trimmed + "/"
}

func cleanSlash(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}
