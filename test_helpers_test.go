package main

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testSite(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := InitSite(dir); err != nil {
		t.Fatalf("InitSite: %v", err)
	}
	return dir
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(data)
}

func writeText(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func writeBytes(t *testing.T, path string, value []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, value, 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func buildSite(t *testing.T, dir string) string {
	t.Helper()
	var out strings.Builder
	if err := BuildSite(BuildOptions{SiteDir: dir, Stdout: &out}); err != nil {
		t.Fatalf("BuildSite: %v", err)
	}
	return out.String()
}

func decodeManifestMap(t *testing.T, html string) map[string]any {
	t.Helper()
	const start = "<!--SMOL ATTESTED MANIFEST V1\n"
	const end = "\nSMOL ATTESTED MANIFEST END-->"
	startAt := strings.Index(html, start)
	if startAt < 0 {
		t.Fatalf("manifest start not found")
	}
	startAt += len(start)
	endAt := strings.Index(html[startAt:], end)
	if endAt < 0 {
		t.Fatalf("manifest end not found")
	}
	encoded := strings.ReplaceAll(html[startAt:startAt+endAt], "\n", "")
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("DecodeString: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal manifest: %v", err)
	}
	return decoded
}

func resourceWithKind(manifest map[string]any, kind string) map[string]any {
	resources, ok := manifest["resources"].([]any)
	if !ok {
		return nil
	}
	for _, value := range resources {
		resource, ok := value.(map[string]any)
		if ok && resource["kind"] == kind {
			return resource
		}
	}
	return nil
}
