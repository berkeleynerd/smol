package main

import (
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

func canonicalURL(baseURL, route string) string {
	return strings.TrimRight(baseURL, "/") + route
}

func cleanSlash(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}
