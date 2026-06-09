package main

import (
	"path/filepath"
	"testing"
)

func TestMinimalSmolJSONLoadsWithDefaults(t *testing.T) {
	dir := t.TempDir()
	writeText(t, filepath.Join(dir, "smol.json"), `{
  "format": "smol-site-v1",
  "title": "Example",
  "base_url": "https://example.org"
}
`)
	cfg, err := LoadSiteConfig(dir)
	if err != nil {
		t.Fatalf("LoadSiteConfig: %v", err)
	}
	if cfg.Language != "en" {
		t.Fatalf("Language = %q, want en", cfg.Language)
	}
	if cfg.Publisher != "Example" {
		t.Fatalf("Publisher = %q, want title default", cfg.Publisher)
	}
	if cfg.Theme != "default" {
		t.Fatalf("Theme = %q, want default", cfg.Theme)
	}
	if len(cfg.Nav) != 0 {
		t.Fatalf("Nav length = %d, want 0", len(cfg.Nav))
	}
}

func TestInvalidBaseURLIsRejected(t *testing.T) {
	dir := t.TempDir()
	writeText(t, filepath.Join(dir, "smol.json"), `{
  "format": "smol-site-v1",
  "title": "Example",
  "base_url": "example.org"
}
`)
	if _, err := LoadSiteConfig(dir); err == nil {
		t.Fatalf("LoadSiteConfig succeeded for invalid base_url")
	}
}

func TestFlatGeminiRequiresGeminiBaseURL(t *testing.T) {
	dir := t.TempDir()
	writeText(t, filepath.Join(dir, "smol.json"), `{
  "format": "smol-site-v1",
  "title": "Example",
  "base_url": "https://example.org",
  "output_mode": "flat-gemini-v1"
}
`)
	if _, err := LoadSiteConfig(dir); err == nil {
		t.Fatalf("LoadSiteConfig accepted https base_url for flat-gemini-v1")
	}
}

func TestPublishConfigLoadsWithoutValidation(t *testing.T) {
	dir := t.TempDir()
	writeText(t, filepath.Join(dir, "smol.json"), `{
  "format": "smol-site-v1",
  "title": "Example",
  "base_url": "https://example.org",
  "publish": {
    "method": "scp",
    "host": "example.org",
    "user": "deploy",
    "port": 2222,
    "path": "/var/www/example"
  }
}
`)
	cfg, err := LoadSiteConfig(dir)
	if err != nil {
		t.Fatalf("LoadSiteConfig: %v", err)
	}
	if cfg.Publish.Method != "scp" || cfg.Publish.Host != "example.org" || cfg.Publish.User != "deploy" || cfg.Publish.Port != 2222 || cfg.Publish.Path != "/var/www/example" {
		t.Fatalf("publish config = %#v", cfg.Publish)
	}
}

func TestThemeConfigLoads(t *testing.T) {
	dir := testSite(t)
	theme, err := LoadThemeConfig(filepath.Join(dir, "themes", "default"))
	if err != nil {
		t.Fatalf("LoadThemeConfig: %v", err)
	}
	if theme.ID != "default" {
		t.Fatalf("theme.ID = %q, want default", theme.ID)
	}
	if theme.Templates.Index == "" || theme.Templates.Page == "" || theme.Templates.Post == "" {
		t.Fatalf("theme templates were not loaded: %#v", theme.Templates)
	}
}
