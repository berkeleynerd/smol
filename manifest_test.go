package main

import (
	"path/filepath"
	"testing"
)

func TestManifestCommentIsGenerated(t *testing.T) {
	dir := testSite(t)
	buildSite(t, dir)
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	if !containsManifest(html) {
		t.Fatalf("generated HTML missing manifest comment")
	}
}

func TestManifestBase64DecodesToValidJSON(t *testing.T) {
	dir := testSite(t)
	buildSite(t, dir)
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	manifest := decodeManifestMap(t, html)
	if manifest["format"] != "smol-attested-manifest-v1" {
		t.Fatalf("manifest format = %v", manifest["format"])
	}
	if manifest["profile"] != Profile {
		t.Fatalf("manifest profile = %v", manifest["profile"])
	}
}

func containsManifest(html string) bool {
	return len(html) > 0 &&
		contains(html, manifestCommentStart) &&
		contains(html, manifestCommentEnd)
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
