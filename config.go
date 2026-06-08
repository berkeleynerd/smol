package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type NavItem struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type SiteConfig struct {
	Format      string    `json:"format"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Language    string    `json:"language"`
	BaseURL     string    `json:"base_url"`
	Publisher   string    `json:"publisher"`
	Theme       string    `json:"theme"`
	OutputMode  string    `json:"output_mode"`
	SignKey     string    `json:"sign_key"`
	Nav         []NavItem `json:"nav"`
}

type ThemeConfig struct {
	Format    string         `json:"format"`
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Version   string         `json:"version"`
	Templates ThemeTemplates `json:"templates"`
	CSS       []string       `json:"css"`
}

type ThemeTemplates struct {
	Index string `json:"index"`
	Page  string `json:"page"`
	Post  string `json:"post"`
}

const outputModeFlatXHTML = "flat-xhtml-v1"

type SiteView struct {
	Title           string
	Description     string
	Language        string
	BaseURL         string
	CanonicalOrigin string
	Publisher       string
}

func LoadSiteConfig(siteDir string) (SiteConfig, error) {
	path := filepath.Join(siteDir, "smol.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SiteConfig{}, fmt.Errorf("smol.json not found")
		}
		return SiteConfig{}, err
	}
	var cfg SiteConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return SiteConfig{}, fmt.Errorf("invalid smol.json: %w", err)
	}
	if cfg.Format != "smol-site-v1" {
		return SiteConfig{}, fmt.Errorf("invalid smol.json: format must be smol-site-v1")
	}
	if cfg.Title == "" {
		return SiteConfig{}, fmt.Errorf("invalid smol.json: title is required")
	}
	if cfg.BaseURL == "" {
		return SiteConfig{}, fmt.Errorf("invalid smol.json: base_url is required")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return SiteConfig{}, fmt.Errorf("invalid smol.json: base_url must be absolute")
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	if cfg.Language == "" {
		cfg.Language = "en"
	}
	if cfg.Publisher == "" {
		cfg.Publisher = cfg.Title
	}
	if cfg.Theme == "" {
		cfg.Theme = "default"
	}
	if cfg.OutputMode != "" && cfg.OutputMode != outputModeFlatXHTML {
		return SiteConfig{}, fmt.Errorf("invalid smol.json: output_mode must be %s", outputModeFlatXHTML)
	}
	if cfg.Nav == nil {
		cfg.Nav = []NavItem{}
	}
	return cfg, nil
}

func LoadThemeConfig(themeDir string) (ThemeConfig, error) {
	return LoadThemeConfigForMode(themeDir, "")
}

func LoadThemeConfigForMode(themeDir, outputMode string) (ThemeConfig, error) {
	data, err := os.ReadFile(filepath.Join(themeDir, "theme.json"))
	if err != nil {
		return ThemeConfig{}, err
	}
	var cfg ThemeConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ThemeConfig{}, fmt.Errorf("invalid theme.json: %w", err)
	}
	if cfg.Format != "smol-theme-v1" {
		return ThemeConfig{}, fmt.Errorf("invalid theme.json: format must be smol-theme-v1")
	}
	if cfg.ID == "" {
		return ThemeConfig{}, fmt.Errorf("invalid theme.json: id is required")
	}
	if cfg.Templates.Index == "" {
		return ThemeConfig{}, fmt.Errorf("invalid theme.json: templates.index is required")
	}
	if cfg.Templates.Page == "" {
		return ThemeConfig{}, fmt.Errorf("invalid theme.json: templates.page is required")
	}
	if outputMode != outputModeFlatXHTML && cfg.Templates.Post == "" {
		return ThemeConfig{}, fmt.Errorf("invalid theme.json: templates.post is required")
	}
	if cfg.CSS == nil {
		cfg.CSS = []string{}
	}
	return cfg, nil
}

func siteView(cfg SiteConfig) (SiteView, error) {
	u, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return SiteView{}, err
	}
	origin := u.Scheme + "://" + u.Host
	return SiteView{
		Title:           cfg.Title,
		Description:     cfg.Description,
		Language:        cfg.Language,
		BaseURL:         cfg.BaseURL,
		CanonicalOrigin: origin,
		Publisher:       cfg.Publisher,
	}, nil
}
