package main

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

type ManifestResource struct {
	Kind       string `json:"kind"`
	Source     string `json:"source"`
	EmbeddedAs string `json:"embedded_as"`
	SHA256     string `json:"sha256"`
}

type manifest struct {
	Format         string             `json:"format"`
	Profile        string             `json:"profile"`
	Generator      manifestGenerator  `json:"generator"`
	Publisher      manifestPublisher  `json:"publisher"`
	Page           manifestPage       `json:"page"`
	Policy         manifestPolicy     `json:"policy"`
	AllowedOrigins []string           `json:"allowed_origins"`
	Resources      []ManifestResource `json:"resources"`
}

type manifestGenerator struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type manifestPublisher struct {
	Name   string `json:"name"`
	Origin string `json:"origin"`
	URL    string `json:"url"`
}

type manifestPage struct {
	Kind         string `json:"kind"`
	Title        string `json:"title"`
	CanonicalURL string `json:"canonical_url"`
	PublishedUTC string `json:"published_utc"`
	UpdatedUTC   string `json:"updated_utc"`
}

type manifestPolicy struct {
	JavaScript        bool `json:"javascript"`
	ExternalResources bool `json:"external_resources"`
	SelfContained     bool `json:"self_contained"`
}

func ManifestComment(site SiteView, page Page, resources []ManifestResource) (string, error) {
	m := manifest{
		Format:  "smol-attested-manifest-v1",
		Profile: Profile,
		Generator: manifestGenerator{
			Name:    "smol",
			Version: Version,
		},
		Publisher: manifestPublisher{
			Name:   site.Publisher,
			Origin: site.CanonicalOrigin,
			URL:    site.BaseURL,
		},
		Page: manifestPage{
			Kind:         page.Kind,
			Title:        page.Title,
			CanonicalURL: page.CanonicalURL,
			PublishedUTC: page.PublishedUTC,
			UpdatedUTC:   page.UpdatedUTC,
		},
		Policy: manifestPolicy{
			JavaScript:        false,
			ExternalResources: false,
			SelfContained:     true,
		},
		AllowedOrigins: []string{site.CanonicalOrigin},
		Resources:      resources,
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return "<!--SMOL ATTESTED MANIFEST V1\n" + wrapBase64(base64.StdEncoding.EncodeToString(data), 76) + "\nSMOL ATTESTED MANIFEST END-->", nil
}

func wrapBase64(value string, width int) string {
	if width <= 0 || len(value) <= width {
		return value
	}
	var b strings.Builder
	for i := 0; i < len(value); i += width {
		if i > 0 {
			b.WriteByte('\n')
		}
		end := i + width
		if end > len(value) {
			end = len(value)
		}
		b.WriteString(value[i:end])
	}
	return b.String()
}
