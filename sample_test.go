package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSampleFixtureBuildsFlatFilesAndArticleGolden(t *testing.T) {
	dir := copyFixtureSite(t, filepath.Join("testdata", "sites", "sample"))
	buildSite(t, dir)

	public := filepath.Join(dir, "public")
	for _, name := range []string{"article-1.html", "article-2.html", "article-3.html", "reference.html"} {
		path := filepath.Join(public, name)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected flat output %s: %v", name, err)
		}
		if got := info.Mode().Perm(); got != 0o644 {
			t.Fatalf("%s mode = %o, want 644", name, got)
		}
	}
	if _, err := os.Stat(filepath.Join(public, "article-2", "index.html")); !os.IsNotExist(err) {
		t.Fatalf("nested article-2 output exists or unexpected stat error: %v", err)
	}

	got := readText(t, filepath.Join(public, "article-2.html"))
	want := strings.TrimSuffix(readText(t, filepath.Join("testdata", "golden", "sample", "article-2.html")), "\n")
	assertSameRegion(t, "style", between(got, "<style type=\"text/css\">\n", "</style>"), between(want, "<style type=\"text/css\">\n", "</style>"))
	assertSameRegion(t, "shell prefix", before(got, "<main>\n"), before(want, "<main>\n"))
	assertSameRegion(t, "body", between(got, "<main>\n", "\n</main>"), between(want, "<main>\n", "\n</main>"))
	assertSameRegion(t, "shell suffix", after(got, "\n</main>"), after(want, "\n</main>"))
	if got != want {
		t.Fatalf("generated article-2.html did not match golden")
	}
	if strings.HasSuffix(got, "\n") {
		t.Fatalf("generated article-2.html has trailing newline")
	}
}

func TestSampleFixtureRepeatedBuildsAreDeterministic(t *testing.T) {
	dir := copyFixtureSite(t, filepath.Join("testdata", "sites", "sample"))
	buildSite(t, dir)
	firstArticle := readText(t, filepath.Join(dir, "public", "article-1.html"))
	firstReference := readText(t, filepath.Join(dir, "public", "reference.html"))

	firstGzip := readText(t, filepath.Join(dir, "public", "article-1.html.gz"))

	buildSite(t, dir)
	secondArticle := readText(t, filepath.Join(dir, "public", "article-1.html"))
	secondReference := readText(t, filepath.Join(dir, "public", "reference.html"))
	secondGzip := readText(t, filepath.Join(dir, "public", "article-1.html.gz"))

	if firstArticle != secondArticle {
		t.Fatalf("repeated build changed article-1.html")
	}
	if firstReference != secondReference {
		t.Fatalf("repeated build changed reference.html")
	}
	if firstGzip != secondGzip {
		t.Fatalf("repeated build changed article-1.html.gz")
	}
}

func copyFixtureSite(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	if err := copyTree(src, dst); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return dst
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func assertSameRegion(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s region mismatch", name)
	}
}

func before(value, marker string) string {
	at := strings.Index(value, marker)
	if at < 0 {
		return value
	}
	return value[:at]
}

func after(value, marker string) string {
	at := strings.Index(value, marker)
	if at < 0 {
		return value
	}
	return value[at+len(marker):]
}

func between(value, start, end string) string {
	startAt := strings.Index(value, start)
	if startAt < 0 {
		return ""
	}
	startAt += len(start)
	endAt := strings.Index(value[startAt:], end)
	if endAt < 0 {
		return ""
	}
	return value[startAt : startAt+endAt]
}
