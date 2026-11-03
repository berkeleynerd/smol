package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestForceBuildProtectsAliasedSources(t *testing.T) {
	for _, name := range []string{"content", "themes", ".git", "site", "site-parent", "missing-output", "site-alias", "redirected-content", "dangling-output"} {
		t.Run(name, func(t *testing.T) {
			site := testSite(t)
			writeContentSource(t, site, "page", "index", ".md", []string{"title: Home"}, "Retain this source.\n")
			writeText(t, filepath.Join(site, ".git", "sentinel"), "retain Git data")
			original := readText(t, filepath.Join(site, "content", "pages", "index.md"))
			link := filepath.Join(t.TempDir(), "alias")
			siteArg, destination := site, filepath.Join(link, "pages")
			source := filepath.Join(site, "content")
			switch name {
			case "themes", ".git":
				source = filepath.Join(site, name)
				destination = link
			case "site":
				source, destination = site, link
			case "site-parent":
				source, destination = filepath.Dir(site), link
			case "missing-output":
				destination = filepath.Join(link, "absent", "output")
			case "site-alias":
				source, siteArg, destination = site, link, filepath.Join(site, "content", "pages")
			case "redirected-content":
				moved := filepath.Join(t.TempDir(), "content")
				if err := os.Rename(source, moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(moved, source); err != nil {
					t.Fatal(err)
				}
				source, destination = moved, filepath.Join(moved, "pages")
			case "dangling-output":
				source = filepath.Join(t.TempDir(), "absent")
			}
			if err := os.Symlink(source, link); err != nil {
				t.Fatal(err)
			}
			if err := BuildSite(BuildOptions{SiteDir: siteArg, OutDir: destination, Force: true, Unsigned: true}); err == nil {
				t.Fatal("aliased source accepted as output")
			}
			if got := readText(t, filepath.Join(site, "content", "pages", "index.md")); got != original {
				t.Fatal("source changed")
			}
			if got := readText(t, filepath.Join(site, ".git", "sentinel")); got != "retain Git data" {
				t.Fatal("Git data changed")
			}
			if _, err := os.Stat(filepath.Join(site, "themes", "default", "theme.json")); err != nil {
				t.Fatal("theme changed:", err)
			}
		})
	}
}

func TestForceBuildAllowsSafeAliasWithMissingOutput(t *testing.T) {
	site := testSite(t)
	external := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(external, link); err != nil {
		t.Fatal(err)
	}
	if err := BuildSite(BuildOptions{SiteDir: site, OutDir: filepath.Join(link, "new", "public"), Force: true, Unsigned: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(external, "new", "public", "index.html")); err != nil {
		t.Fatal(err)
	}
}

func TestPublishDryRunRejectsAliasedSourceOutput(t *testing.T) {
	site := testSite(t)
	writePublishConfig(t, site, "example.org", "", 22, "/srv/site")
	link := filepath.Join(t.TempDir(), "content")
	if err := os.Symlink(filepath.Join(site, "content"), link); err != nil {
		t.Fatal(err)
	}
	if err := PublishSite(PublishOptions{SiteDir: site, OutDir: filepath.Join(link, "pages"), DryRun: true}); err == nil {
		t.Fatal("publish dry-run accepted aliased source output")
	}
}
