package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	manifestCommentStart = "<!--SMOL ATTESTED MANIFEST V1\n"
	manifestCommentEnd   = "\nSMOL ATTESTED MANIFEST END-->"
)

type ManifestResource struct {
	Kind       string `json:"kind"`
	Source     string `json:"source"`
	EmbeddedAs string `json:"embedded_as"`
	SHA256     string `json:"sha256"`
}

type ManifestRegion struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
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
	Regions        []ManifestRegion   `json:"regions,omitempty"`
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

func ManifestComment(site SiteView, page Page, resources []ManifestResource, regions []ManifestRegion) (string, error) {
	if resources == nil {
		resources = []ManifestResource{}
	}
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
		Regions:        regions,
	}
	data, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return manifestCommentStart + wrapBase64(base64.StdEncoding.EncodeToString(data), 76) + manifestCommentEnd, nil
}

func ExtractManifest(html string) (manifest, bool, error) {
	data, ok, err := extractManifestPayload(html)
	if err != nil {
		return manifest{}, false, err
	}
	if !ok {
		return manifest{}, false, nil
	}
	m, err := parseManifestJSON(data)
	if err != nil {
		return manifest{}, false, err
	}
	return m, true, nil
}

func extractManifestPayload(html string) ([]byte, bool, error) {
	html = normalizeManifestLineEndings(html)
	startAt := strings.Index(html, manifestCommentStart)
	endAtOnly := strings.Index(html, manifestCommentEnd)
	if startAt < 0 {
		if endAtOnly >= 0 {
			return nil, false, fmt.Errorf("malformed manifest comment: end marker without start marker")
		}
		return nil, false, nil
	}
	startAt += len(manifestCommentStart)
	endAt := strings.Index(html[startAt:], manifestCommentEnd)
	if endAt < 0 {
		return nil, false, fmt.Errorf("malformed manifest comment: missing end marker")
	}
	afterEnd := html[startAt+endAt+len(manifestCommentEnd):]
	if strings.Contains(afterEnd, manifestCommentStart) {
		return nil, false, fmt.Errorf("multiple manifest comments found")
	}
	encoded := strings.ReplaceAll(html[startAt:startAt+endAt], "\n", "")
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, false, fmt.Errorf("invalid manifest base64: %w", err)
	}
	return data, true, nil
}

func normalizeManifestLineEndings(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return value
}

func parseManifestJSON(data []byte) (manifest, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return manifest{}, fmt.Errorf("invalid manifest JSON: %w", err)
	}
	if err := requireManifestFields(raw); err != nil {
		return manifest{}, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, fmt.Errorf("invalid manifest JSON: %w", err)
	}
	if m.Format != "smol-attested-manifest-v1" {
		return manifest{}, fmt.Errorf("invalid manifest: format must be smol-attested-manifest-v1")
	}
	if m.Profile != Profile {
		return manifest{}, fmt.Errorf("invalid manifest: profile must be %s", Profile)
	}
	if m.Policy.JavaScript {
		return manifest{}, fmt.Errorf("invalid manifest: policy.javascript must be false")
	}
	if m.Policy.ExternalResources {
		return manifest{}, fmt.Errorf("invalid manifest: policy.external_resources must be false")
	}
	if !m.Policy.SelfContained {
		return manifest{}, fmt.Errorf("invalid manifest: policy.self_contained must be true")
	}
	return m, nil
}

func requireManifestFields(raw map[string]json.RawMessage) error {
	requiredTop := []string{"format", "profile", "generator", "publisher", "page", "policy", "allowed_origins", "resources"}
	for _, name := range requiredTop {
		if _, ok := raw[name]; !ok {
			return fmt.Errorf("invalid manifest: missing %s", name)
		}
	}
	if err := requireManifestObjectFields(raw, "generator", []string{"name", "version"}); err != nil {
		return err
	}
	if err := requireManifestObjectFields(raw, "publisher", []string{"name", "origin", "url"}); err != nil {
		return err
	}
	if err := requireManifestObjectFields(raw, "page", []string{"kind", "title", "canonical_url", "published_utc", "updated_utc"}); err != nil {
		return err
	}
	if err := requireManifestObjectFields(raw, "policy", []string{"javascript", "external_resources", "self_contained"}); err != nil {
		return err
	}
	if err := requireManifestObjectBoolFields(raw, "policy", []string{"javascript", "external_resources", "self_contained"}); err != nil {
		return err
	}
	if err := requireManifestArray(raw, "allowed_origins"); err != nil {
		return err
	}
	if err := requireManifestResources(raw["resources"]); err != nil {
		return err
	}
	if value, ok := raw["regions"]; ok {
		if err := requireManifestRegions(value); err != nil {
			return err
		}
	}
	return nil
}

func requireManifestObjectBoolFields(raw map[string]json.RawMessage, name string, fields []string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw[name], &object); err != nil {
		return fmt.Errorf("invalid manifest: %s must be an object", name)
	}
	for _, field := range fields {
		var value bool
		if err := json.Unmarshal(object[field], &value); err != nil {
			return fmt.Errorf("invalid manifest: %s.%s must be a boolean", name, field)
		}
		if strings.TrimSpace(string(object[field])) == "null" {
			return fmt.Errorf("invalid manifest: %s.%s must be a boolean", name, field)
		}
	}
	return nil
}

func requireManifestObjectFields(raw map[string]json.RawMessage, name string, fields []string) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw[name], &object); err != nil {
		return fmt.Errorf("invalid manifest: %s must be an object", name)
	}
	for _, field := range fields {
		if _, ok := object[field]; !ok {
			return fmt.Errorf("invalid manifest: missing %s.%s", name, field)
		}
	}
	return nil
}

func requireManifestArray(raw map[string]json.RawMessage, name string) error {
	if strings.TrimSpace(string(raw[name])) == "null" {
		return fmt.Errorf("invalid manifest: %s must be an array", name)
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw[name], &values); err != nil {
		return fmt.Errorf("invalid manifest: %s must be an array", name)
	}
	return nil
}

func requireManifestResources(raw json.RawMessage) error {
	if strings.TrimSpace(string(raw)) == "null" {
		return fmt.Errorf("invalid manifest: resources must be an array")
	}
	var resources []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &resources); err != nil {
		return fmt.Errorf("invalid manifest: resources must be an array")
	}
	for i, resource := range resources {
		for _, field := range []string{"kind", "source", "embedded_as", "sha256"} {
			if _, ok := resource[field]; !ok {
				return fmt.Errorf("invalid manifest: missing resources[%d].%s", i, field)
			}
		}
		hash, err := manifestStringField(resource, "sha256")
		if err != nil {
			return fmt.Errorf("invalid manifest: resources[%d].sha256 must be a string", i)
		}
		if !isLowercaseSHA256Hex(hash) {
			return fmt.Errorf("invalid manifest: resources[%d].sha256 must be lowercase hex SHA-256", i)
		}
	}
	return nil
}

func requireManifestRegions(raw json.RawMessage) error {
	if strings.TrimSpace(string(raw)) == "null" {
		return fmt.Errorf("invalid manifest: regions must be an array")
	}
	var regions []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &regions); err != nil {
		return fmt.Errorf("invalid manifest: regions must be an array")
	}
	seen := map[string]bool{}
	for i, region := range regions {
		for _, field := range []string{"name", "sha256"} {
			if _, ok := region[field]; !ok {
				return fmt.Errorf("invalid manifest: missing regions[%d].%s", i, field)
			}
		}
		name, err := manifestStringField(region, "name")
		if err != nil {
			return fmt.Errorf("invalid manifest: regions[%d].name must be a string", i)
		}
		if seen[name] {
			return fmt.Errorf("invalid manifest: duplicate region %q", name)
		}
		seen[name] = true
		hash, err := manifestStringField(region, "sha256")
		if err != nil {
			return fmt.Errorf("invalid manifest: regions[%d].sha256 must be a string", i)
		}
		if isKnownRegionName(name) && !isLowercaseSHA256Hex(hash) {
			return fmt.Errorf("invalid manifest: regions[%d].sha256 must be lowercase hex SHA-256", i)
		}
	}
	return nil
}

func manifestStringField(object map[string]json.RawMessage, name string) (string, error) {
	if strings.TrimSpace(string(object[name])) == "null" {
		return "", fmt.Errorf("null")
	}
	var value string
	if err := json.Unmarshal(object[name], &value); err != nil {
		return "", err
	}
	return value, nil
}

func isLowercaseSHA256Hex(value string) bool {
	if len(value) != 64 {
		return false
	}
	if strings.ToLower(value) != value {
		return false
	}
	data, err := hex.DecodeString(value)
	return err == nil && len(data) == 32
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
