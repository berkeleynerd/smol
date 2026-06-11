package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gunzipFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("gzip.NewReader(%s): %v", path, err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip(%s): %v", path, err)
	}
	if err := zr.Close(); err != nil {
		t.Fatalf("gzip close(%s): %v", path, err)
	}
	return string(out)
}

func TestBuildWritesGzipSiblingsOfFinalBytes(t *testing.T) {
	dir := testSite(t)
	buildSite(t, dir)
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	gz := gunzipFile(t, filepath.Join(dir, "public", "index.html.gz"))
	if gz != html {
		t.Fatalf("index.html.gz does not decompress to index.html bytes")
	}
}

func TestSignedBuildGzipsTheSignedBytes(t *testing.T) {
	dir := testSite(t)
	fake := fakeSigner(t, t.TempDir(), "fake-attest", 0)
	if err := BuildSite(BuildOptions{SiteDir: dir, SignKey: "KEY", SignKeySource: true, AttestPath: fake, Stdout: &strings.Builder{}}); err != nil {
		t.Fatalf("BuildSite signed: %v", err)
	}
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	if !strings.Contains(html, "FAKE-SIGNED") {
		t.Fatalf("signed output missing signature marker: %s", html)
	}
	gz := gunzipFile(t, filepath.Join(dir, "public", "index.html.gz"))
	if gz != html {
		t.Fatalf("index.html.gz does not decompress to the signed bytes")
	}
}

func TestGeminiBuildEmitsNoGzipSiblings(t *testing.T) {
	dir := geminiSite(t, "")
	writeGeminiPage(t, dir, "index", "Capsule", "Welcome.\n")
	if err := BuildSite(BuildOptions{SiteDir: dir, Stdout: &strings.Builder{}}); err != nil {
		t.Fatalf("BuildSite gemini: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "public", "*.gz"))
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("gemini build emitted gzip siblings: %v", matches)
	}
}
