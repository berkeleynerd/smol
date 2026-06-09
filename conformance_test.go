package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type conformanceCase struct {
	Name           string                 `json:"name"`
	OutputMode     string                 `json:"output_mode"`
	Action         string                 `json:"action"`
	Expect         string                 `json:"expect"`
	Starter        string                 `json:"starter"`
	Fixture        string                 `json:"fixture"`
	ErrorSubstring string                 `json:"error_substring"`
	Assertions     []conformanceAssertion `json:"assertions"`
}

type conformanceAssertion struct {
	File        string                      `json:"file"`
	Exact       string                      `json:"exact"`
	Contains    []string                    `json:"contains"`
	NotContains []string                    `json:"not_contains"`
	Region      *conformanceRegionAssertion `json:"region"`
}

type conformanceRegionAssertion struct {
	Start       string   `json:"start"`
	End         string   `json:"end"`
	Contains    []string `json:"contains"`
	NotContains []string `json:"not_contains"`
}

func TestStaticNoJSConformanceCorpus(t *testing.T) {
	root := filepath.Join("conformance", "smol-static-nojs-v1")
	data, err := os.ReadFile(filepath.Join(root, "expected-results.json"))
	if err != nil {
		t.Fatalf("read conformance expectations: %v", err)
	}
	var cases []conformanceCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("parse conformance expectations: %v", err)
	}
	if len(cases) == 0 {
		t.Fatalf("conformance corpus is empty")
	}
	if err := validateConformanceCorpus(root, cases); err != nil {
		t.Fatalf("invalid conformance corpus: %v", err)
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			dir := prepareConformanceSite(t, root, tc)
			err := runConformanceAction(dir, tc.Action)
			if tc.Expect == "fail" {
				if err == nil {
					t.Fatalf("%s unexpectedly passed", tc.Action)
				}
				if tc.ErrorSubstring != "" && !strings.Contains(err.Error(), tc.ErrorSubstring) {
					t.Fatalf("%s error = %v, want substring %q", tc.Action, err, tc.ErrorSubstring)
				}
				return
			}
			if tc.Expect != "pass" {
				t.Fatalf("unknown conformance expectation %q", tc.Expect)
			}
			if err != nil {
				t.Fatalf("%s failed: %v", tc.Action, err)
			}
			assertConformanceOutputMode(t, dir, tc.OutputMode)
			assertConformanceOutputs(t, dir, tc.Assertions)
		})
	}
}

func TestValidateConformanceCorpusReportsDrift(t *testing.T) {
	t.Run("orphan fixture", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "valid", "referenced"), 0o755); err != nil {
			t.Fatalf("MkdirAll referenced: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(root, "valid", "orphan"), 0o755); err != nil {
			t.Fatalf("MkdirAll orphan: %v", err)
		}
		err := validateConformanceCorpus(root, []conformanceCase{{Name: "referenced", Fixture: "valid/referenced"}})
		if err == nil || !strings.Contains(err.Error(), "orphaned conformance fixture") {
			t.Fatalf("validateConformanceCorpus error = %v, want orphaned fixture", err)
		}
	})
	t.Run("duplicate fixture", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "invalid", "duplicate"), 0o755); err != nil {
			t.Fatalf("MkdirAll duplicate: %v", err)
		}
		cases := []conformanceCase{
			{Name: "first", Fixture: "invalid/duplicate"},
			{Name: "second", Fixture: "invalid/duplicate"},
		}
		err := validateConformanceCorpus(root, cases)
		if err == nil || !strings.Contains(err.Error(), "duplicate fixture reference") {
			t.Fatalf("validateConformanceCorpus error = %v, want duplicate fixture", err)
		}
	})
}

func validateConformanceCorpus(root string, cases []conformanceCase) error {
	names := map[string]bool{}
	references := map[string]string{}
	for _, tc := range cases {
		if tc.Name == "" {
			return fmt.Errorf("conformance case has empty name")
		}
		if names[tc.Name] {
			return fmt.Errorf("duplicate conformance case name: %s", tc.Name)
		}
		names[tc.Name] = true
		if tc.Fixture == "" {
			continue
		}
		fixture := filepath.ToSlash(filepath.Clean(tc.Fixture))
		if fixture == "." || strings.HasPrefix(fixture, "../") || strings.HasPrefix(fixture, "/") {
			return fmt.Errorf("invalid fixture reference %q in %s", tc.Fixture, tc.Name)
		}
		if prior := references[fixture]; prior != "" {
			return fmt.Errorf("duplicate fixture reference %q in %s and %s", fixture, prior, tc.Name)
		}
		references[fixture] = tc.Name
	}
	actual, err := conformanceFixtureDirs(root)
	if err != nil {
		return err
	}
	for fixture := range references {
		if !actual[fixture] {
			return fmt.Errorf("referenced conformance fixture does not exist: %s", fixture)
		}
	}
	for fixture := range actual {
		if references[fixture] == "" {
			return fmt.Errorf("orphaned conformance fixture: %s", fixture)
		}
	}
	return nil
}

func conformanceFixtureDirs(root string) (map[string]bool, error) {
	fixtures := map[string]bool{}
	for _, tier := range []string{"valid", "invalid"} {
		dir := filepath.Join(root, tier)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				fixtures[filepath.ToSlash(filepath.Join(tier, entry.Name()))] = true
			}
		}
	}
	return fixtures, nil
}

func prepareConformanceSite(t *testing.T, root string, tc conformanceCase) string {
	t.Helper()
	dir := t.TempDir()
	if tc.Starter != "" {
		if err := InitSiteWithStarter(dir, tc.Starter); err != nil {
			t.Fatalf("init starter %q: %v", tc.Starter, err)
		}
	}
	if tc.Fixture != "" {
		fixtureDir := filepath.Join(root, filepath.FromSlash(tc.Fixture))
		if err := copyTree(fixtureDir, dir); err != nil {
			t.Fatalf("copy conformance fixture %q: %v", tc.Fixture, err)
		}
	}
	return dir
}

func runConformanceAction(siteDir, action string) error {
	switch action {
	case "load":
		return loadConformanceSite(siteDir)
	case "build":
		return BuildSite(BuildOptions{SiteDir: siteDir})
	default:
		return fmt.Errorf("unknown conformance action: %s", action)
	}
}

func loadConformanceSite(siteDir string) error {
	cfg, err := LoadSiteConfig(siteDir)
	if err != nil {
		return err
	}
	pages, posts, err := LoadContent(siteDir, cfg)
	if err != nil {
		return err
	}
	_, _, err = resolvePageNavigations(pages, posts, cfg.OutputMode)
	return err
}

func assertConformanceOutputMode(t *testing.T, siteDir, want string) {
	t.Helper()
	if want == "" {
		return
	}
	cfg, err := LoadSiteConfig(siteDir)
	if err != nil {
		t.Fatalf("load config for output_mode assertion: %v", err)
	}
	if cfg.OutputMode != want {
		t.Fatalf("output_mode = %q, want %q", cfg.OutputMode, want)
	}
}

func assertConformanceOutputs(t *testing.T, siteDir string, assertions []conformanceAssertion) {
	t.Helper()
	for _, assertion := range assertions {
		path := filepath.Join(siteDir, filepath.FromSlash(assertion.File))
		content := readText(t, path)
		if assertion.Exact != "" && content != assertion.Exact {
			t.Fatalf("%s exact output mismatch:\n--- got ---\n%s\n--- want ---\n%s", assertion.File, content, assertion.Exact)
		}
		assertContainsAll(t, assertion.File, content, assertion.Contains)
		assertContainsNone(t, assertion.File, content, assertion.NotContains)
		if assertion.Region != nil {
			if assertion.Region.Start == "" || assertion.Region.End == "" {
				t.Fatalf("%s region assertion must have non-empty start and end markers", assertion.File)
			}
			region := between(content, assertion.Region.Start, assertion.Region.End)
			if region == "" {
				t.Fatalf("%s did not contain expected region between %q and %q", assertion.File, assertion.Region.Start, assertion.Region.End)
			}
			assertContainsAll(t, assertion.File+" region", region, assertion.Region.Contains)
			assertContainsNone(t, assertion.File+" region", region, assertion.Region.NotContains)
		}
	}
}

func assertContainsAll(t *testing.T, name, content string, values []string) {
	t.Helper()
	for _, want := range values {
		if !strings.Contains(content, want) {
			t.Fatalf("%s missing %q", name, want)
		}
	}
}

func assertContainsNone(t *testing.T, name, content string, values []string) {
	t.Helper()
	for _, unwanted := range values {
		if strings.Contains(content, unwanted) {
			t.Fatalf("%s unexpectedly contained %q", name, unwanted)
		}
	}
}
