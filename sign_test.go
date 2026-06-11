package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestResolveAttestExplicitPathWins(t *testing.T) {
	dir := t.TempDir()
	fake := fakeSigner(t, dir, "explicit-attest", 0)
	got, err := ResolveAttestTool(AttestResolverOptions{
		ExplicitPath:  fake,
		ExecutableDir: t.TempDir(),
		TempDir:       t.TempDir(),
		LookPath: func(name string) (string, error) {
			return "", exec.ErrNotFound
		},
	})
	if err != nil {
		t.Fatalf("ResolveAttestTool: %v", err)
	}
	if got != fake {
		t.Fatalf("resolved signer = %s, want %s", got, fake)
	}
}

func TestResolveAttestPathPrefersAttestOverAttestedHTML(t *testing.T) {
	dir := t.TempDir()
	attest := fakeSigner(t, dir, "attest", 0)
	attestedHTML := fakeSigner(t, dir, "attested-html", 0)
	got, err := ResolveAttestTool(AttestResolverOptions{
		ExecutableDir: t.TempDir(),
		TempDir:       t.TempDir(),
		LookPath: func(name string) (string, error) {
			switch name {
			case "attest":
				return attest, nil
			case "attested-html":
				return attestedHTML, nil
			default:
				return "", exec.ErrNotFound
			}
		},
	})
	if err != nil {
		t.Fatalf("ResolveAttestTool: %v", err)
	}
	if got != attest {
		t.Fatalf("resolved signer = %s, want %s", got, attest)
	}
}

func TestResolveAttestFindsSiblingExecutable(t *testing.T) {
	root := t.TempDir()
	exeDir := filepath.Join(root, "bin")
	sibling := filepath.Join(root, "attest")
	if err := os.MkdirAll(exeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll exeDir: %v", err)
	}
	fake := fakeSigner(t, sibling, "attested-html", 0)
	got, err := ResolveAttestTool(AttestResolverOptions{
		ExecutableDir: exeDir,
		TempDir:       t.TempDir(),
		LookPath: func(name string) (string, error) {
			return "", exec.ErrNotFound
		},
	})
	if err != nil {
		t.Fatalf("ResolveAttestTool: %v", err)
	}
	if got != fake {
		t.Fatalf("resolved signer = %s, want %s", got, fake)
	}
}

func TestResolveAttestBuildsSiblingStdlibGoProject(t *testing.T) {
	root := t.TempDir()
	exeDir := filepath.Join(root, "bin")
	project := filepath.Join(root, "attest")
	if err := os.MkdirAll(exeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll exeDir: %v", err)
	}
	writeTinySignerProject(t, project, "")
	var out strings.Builder
	got, err := ResolveAttestTool(AttestResolverOptions{
		ExecutableDir: exeDir,
		TempDir:       t.TempDir(),
		Stdout:        &out,
		LookPath: func(name string) (string, error) {
			return "", exec.ErrNotFound
		},
	})
	if err != nil {
		t.Fatalf("ResolveAttestTool: %v", err)
	}
	if !strings.Contains(out.String(), "building signer from "+project) {
		t.Fatalf("build log missing project path: %s", out.String())
	}
	if !isExecutableFile(got) {
		t.Fatalf("built signer is not executable: %s", got)
	}
	if _, err := os.Stat(filepath.Join(project, "attest")); !os.IsNotExist(err) {
		t.Fatalf("sibling project was mutated or unexpected stat error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "attested-html")); !os.IsNotExist(err) {
		t.Fatalf("sibling project was mutated or unexpected stat error: %v", err)
	}
}

func TestResolveAttestRejectsSiblingGoProjectWithRequire(t *testing.T) {
	root := t.TempDir()
	exeDir := filepath.Join(root, "bin")
	project := filepath.Join(root, "attest")
	if err := os.MkdirAll(exeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll exeDir: %v", err)
	}
	writeTinySignerProject(t, project, "require example.org/dep v1.0.0\n")
	_, err := ResolveAttestTool(AttestResolverOptions{
		ExecutableDir: exeDir,
		TempDir:       t.TempDir(),
		LookPath: func(name string) (string, error) {
			return "", exec.ErrNotFound
		},
	})
	if err == nil || !strings.Contains(err.Error(), "go.mod has require dependency") {
		t.Fatalf("expected require rejection, got %v", err)
	}
}

func TestResolveAttestMissingReportsSearchedLocations(t *testing.T) {
	root := t.TempDir()
	exeDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(exeDir, 0o755); err != nil {
		t.Fatalf("MkdirAll exeDir: %v", err)
	}
	_, err := ResolveAttestTool(AttestResolverOptions{
		ExecutableDir: exeDir,
		TempDir:       t.TempDir(),
		LookPath: func(name string) (string, error) {
			return "", exec.ErrNotFound
		},
	})
	if err == nil {
		t.Fatalf("ResolveAttestTool succeeded without signer")
	}
	for _, want := range []string{"PATH:attest", "PATH:attested-html", filepath.Join(root, "attest", "attest"), filepath.Join(root, "attested-html", "attested-html")} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing searched location %q in %v", want, err)
		}
	}
}

func TestBuildWithFakeSignerAutoSignsWhenKeyIsProvided(t *testing.T) {
	dir := testSite(t)
	fake := fakeSigner(t, t.TempDir(), "fake-attest", 0)
	var out strings.Builder
	err := BuildSite(BuildOptions{
		SiteDir:    dir,
		SignKey:    "KEY",
		AttestPath: fake,
		Stdout:     &out,
	})
	if err != nil {
		t.Fatalf("BuildSite signed: %v", err)
	}
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	if !strings.Contains(html, "FAKE-SIGNED key=KEY") {
		t.Fatalf("signed output missing fake trailer")
	}
	if !strings.Contains(out.String(), "signed: public/index.html") {
		t.Fatalf("stdout did not report signing: %s", out.String())
	}
}

func TestBuildCommandAcceptsAttestFlags(t *testing.T) {
	for _, flagName := range []string{"--attest", "--attested-html"} {
		t.Run(flagName, func(t *testing.T) {
			dir := testSite(t)
			fake := fakeSigner(t, t.TempDir(), "fake-attest", 0)
			var out strings.Builder
			err := commandBuild([]string{"--sign-key", "KEY", flagName, fake, dir}, &out)
			if err != nil {
				t.Fatalf("commandBuild: %v", err)
			}
			html := readText(t, filepath.Join(dir, "public", "index.html"))
			if !strings.Contains(html, "FAKE-SIGNED key=KEY") {
				t.Fatalf("signed output missing fake trailer")
			}
		})
	}
}

func TestConfigSignKeyAutoSigns(t *testing.T) {
	dir := testSite(t)
	replaceInFile(t, filepath.Join(dir, "smol.json"), `"sign_key": ""`, `"sign_key": "CONFIG"`)
	fake := fakeSigner(t, t.TempDir(), "fake-attest", 0)
	err := BuildSite(BuildOptions{SiteDir: dir, AttestPath: fake})
	if err != nil {
		t.Fatalf("BuildSite signed with config key: %v", err)
	}
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	if !strings.Contains(html, "FAKE-SIGNED key=CONFIG") {
		t.Fatalf("signed output did not use config key: %s", html)
	}
}

func TestBuildCommandDoesNotAutoSignSoleSecretKey(t *testing.T) {
	dir := testSite(t)
	fake := fakeSigner(t, t.TempDir(), "fake-attest", 0)
	called := false
	withSecretKeys(t, func() ([]gpgSecretKey, error) {
		called = true
		return []gpgSecretKey{{Fingerprint: "AUTO-FPR", UID: "Curator <curator@example.org>"}}, nil
	})
	var out, stderr strings.Builder
	err := commandBuildWithIO([]string{"--attest", fake, dir}, strings.NewReader("y\n"), &out, &stderr, true)
	if err != nil {
		t.Fatalf("commandBuildWithIO unsigned: %v", err)
	}
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	if strings.Contains(html, "FAKE-SIGNED") {
		t.Fatalf("build auto-signed without --sign: %s", html)
	}
	if called {
		t.Fatalf("listed secret keys without --sign")
	}
}

func TestBuildCommandInteractiveSignSingleKeyAccepts(t *testing.T) {
	dir := testSite(t)
	fake := fakeSigner(t, t.TempDir(), "fake-attest", 0)
	withSecretKeys(t, func() ([]gpgSecretKey, error) {
		return []gpgSecretKey{{Fingerprint: "AUTO-FPR", UID: "Curator <curator@example.org>"}}, nil
	})
	var out, stderr strings.Builder
	err := commandBuildWithIO([]string{"--sign", "--attest", fake, dir}, strings.NewReader(""), &out, &stderr, true)
	if err != nil {
		t.Fatalf("commandBuildWithIO interactive sign: %v", err)
	}
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	if !strings.Contains(html, "FAKE-SIGNED key=AUTO-FPR") {
		t.Fatalf("signed output did not use selected key: %s", html)
	}
	if !strings.Contains(stderr.String(), "Signing with AUTO-FPR Curator <curator@example.org>") || strings.Contains(stderr.String(), "[y/N]") {
		t.Fatalf("stderr did not report auto-selected single key cleanly: %s", stderr.String())
	}
}

func TestBuildCommandInteractiveDeclineBypassesConfigSignKey(t *testing.T) {
	dir := testSite(t)
	replaceInFile(t, filepath.Join(dir, "smol.json"), `"sign_key": ""`, `"sign_key": "CONFIG"`)
	fake := fakeSigner(t, t.TempDir(), "fake-attest", 0)
	withSecretKeys(t, func() ([]gpgSecretKey, error) {
		return []gpgSecretKey{{Fingerprint: "FPR1"}, {Fingerprint: "FPR2"}}, nil
	})
	var out, stderr strings.Builder
	err := commandBuildWithIO([]string{"--sign", "--attest", fake, dir}, strings.NewReader("\n"), &out, &stderr, true)
	if err != nil {
		t.Fatalf("commandBuildWithIO interactive decline: %v", err)
	}
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	if strings.Contains(html, "FAKE-SIGNED") {
		t.Fatalf("declined interactive signing fell back to config key: %s", html)
	}
}

func TestBuildCommandInteractiveSignMultipleKeysSelectsNumber(t *testing.T) {
	dir := testSite(t)
	fake := fakeSigner(t, t.TempDir(), "fake-attest", 0)
	withSecretKeys(t, func() ([]gpgSecretKey, error) {
		return []gpgSecretKey{
			{Fingerprint: "FPR1", UID: "One <one@example.org>"},
			{Fingerprint: "FPR2", UID: "Two <two@example.org>"},
		}, nil
	})
	var out, stderr strings.Builder
	err := commandBuildWithIO([]string{"--sign", "--attest", fake, dir}, strings.NewReader("2\n"), &out, &stderr, true)
	if err != nil {
		t.Fatalf("commandBuildWithIO interactive multi-key sign: %v", err)
	}
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	if !strings.Contains(html, "FAKE-SIGNED key=FPR2") {
		t.Fatalf("signed output did not use selected key: %s", html)
	}
	if !strings.Contains(stderr.String(), "1) FPR1") || !strings.Contains(stderr.String(), "2) FPR2") {
		t.Fatalf("stderr missing key list: %s", stderr.String())
	}
}

func TestBuildCommandSignPreflightsBeforeKeyLookup(t *testing.T) {
	dir := testSite(t)
	writeText(t, filepath.Join(dir, "smol.json"), "{")
	withSecretKeys(t, func() ([]gpgSecretKey, error) {
		t.Fatalf("--sign listed keys before config validation")
		return nil, nil
	})
	var out, stderr strings.Builder
	err := commandBuildWithIO([]string{"--sign", dir}, strings.NewReader("y\n"), &out, &stderr, true)
	if err == nil || !strings.Contains(err.Error(), "invalid smol.json") {
		t.Fatalf("commandBuildWithIO error = %v, want config validation error", err)
	}
}

func TestCLIKeyOverridesConfigSignKey(t *testing.T) {
	dir := testSite(t)
	replaceInFile(t, filepath.Join(dir, "smol.json"), `"sign_key": ""`, `"sign_key": "CONFIG"`)
	fake := fakeSigner(t, t.TempDir(), "fake-attest", 0)
	err := BuildSite(BuildOptions{
		SiteDir:       dir,
		SignKey:       "CLI",
		SignKeySource: true,
		AttestPath:    fake,
	})
	if err != nil {
		t.Fatalf("BuildSite signed with CLI key: %v", err)
	}
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	if !strings.Contains(html, "FAKE-SIGNED key=CLI") {
		t.Fatalf("signed output did not use CLI key: %s", html)
	}
}

func TestBuildCommandInteractiveSignErrors(t *testing.T) {
	tests := map[string]struct {
		keys        func() ([]gpgSecretKey, error)
		stdin       string
		interactive bool
		want        string
	}{
		"non interactive": {
			keys: func() ([]gpgSecretKey, error) {
				return []gpgSecretKey{{Fingerprint: "FPR"}}, nil
			},
			interactive: false,
			want:        "interactive signing requires a terminal",
		},
		"missing gpg": {
			keys: func() ([]gpgSecretKey, error) {
				return nil, errors.New("gpg not found on PATH")
			},
			interactive: true,
			want:        "gpg not found on PATH",
		},
		"zero keys": {
			keys: func() ([]gpgSecretKey, error) {
				return nil, nil
			},
			interactive: true,
			want:        "no GPG secret keys found",
		},
		"invalid selection": {
			keys: func() ([]gpgSecretKey, error) {
				return []gpgSecretKey{{Fingerprint: "FPR1"}, {Fingerprint: "FPR2"}}, nil
			},
			stdin:       "wat\n",
			interactive: true,
			want:        "invalid signing key selection",
		},
		"plus selection": {
			keys: func() ([]gpgSecretKey, error) {
				return []gpgSecretKey{{Fingerprint: "FPR1"}, {Fingerprint: "FPR2"}}, nil
			},
			stdin:       "+2\n",
			interactive: true,
			want:        "invalid signing key selection",
		},
		"leading zero selection": {
			keys: func() ([]gpgSecretKey, error) {
				return []gpgSecretKey{{Fingerprint: "FPR1"}, {Fingerprint: "FPR2"}}, nil
			},
			stdin:       "02\n",
			interactive: true,
			want:        "invalid signing key selection",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dir := testSite(t)
			fake := fakeSigner(t, t.TempDir(), "fake-attest", 0)
			withSecretKeys(t, tc.keys)
			var out, stderr strings.Builder
			err := commandBuildWithIO([]string{"--sign", "--attest", fake, dir}, strings.NewReader(tc.stdin), &out, &stderr, tc.interactive)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("commandBuildWithIO error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseGPGSecretKeys(t *testing.T) {
	output := strings.Join([]string{
		"sec:u:255:22:AE2EC3CBB111EB87:1780994685:1844066685::u:::scSC:::+::ed25519:::0:",
		"fpr:::::::::C6174EFE4839EBCFE724670FAE2EC3CBB111EB87:",
		"grp:::::::::6A9107DA918321A0B13C4C31E255063F713AE874:",
		"uid:u::::1780994685::1961FA92FF2E461EEFFD15B41640765B10BB682B::Curator <curator@example.org>::::::::::0:",
		"ssb:u:255:22:1111111111111111:1780994685::::::e:::+::ed25519:::0:",
		"fpr:::::::::1111111111111111111111111111111111111111:",
		"sec:u:255:22:2222222222222222:1780994685:1844066685::u:::scSC:::+::ed25519:::0:",
		"fpr:::::::::2222222222222222222222222222222222222222:",
		"uid:u::::1780994685::1961FA92FF2E461EEFFD15B41640765B10BB682B::Second <second@example.org>::::::::::0:",
	}, "\n")
	keys := parseGPGSecretKeys(output)
	if len(keys) != 2 {
		t.Fatalf("keys = %#v, want 2 primary keys", keys)
	}
	if keys[0].Fingerprint != "C6174EFE4839EBCFE724670FAE2EC3CBB111EB87" || keys[0].UID != "Curator <curator@example.org>" {
		t.Fatalf("first key = %#v", keys[0])
	}
	if keys[1].Fingerprint != "2222222222222222222222222222222222222222" || keys[1].UID != "Second <second@example.org>" {
		t.Fatalf("second key = %#v", keys[1])
	}
}

func withSecretKeys(t *testing.T, fn func() ([]gpgSecretKey, error)) {
	t.Helper()
	original := listSecretKeys
	listSecretKeys = fn
	t.Cleanup(func() {
		listSecretKeys = original
	})
}

func TestUnsignedSuppressesConfigAndCLIKeys(t *testing.T) {
	dir := testSite(t)
	replaceInFile(t, filepath.Join(dir, "smol.json"), `"sign_key": ""`, `"sign_key": "CONFIG"`)
	fake := fakeSigner(t, t.TempDir(), "fake-attest", 0)
	err := BuildSite(BuildOptions{
		SiteDir:       dir,
		SignKey:       "CLI",
		SignKeySource: true,
		AttestPath:    fake,
		Unsigned:      true,
	})
	if err != nil {
		t.Fatalf("BuildSite unsigned: %v", err)
	}
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	if strings.Contains(html, "FAKE-SIGNED") {
		t.Fatalf("unsigned build was signed: %s", html)
	}
}

func TestNoKeyBuildsUnsignedEvenWhenSignerExists(t *testing.T) {
	dir := testSite(t)
	fake := fakeSigner(t, t.TempDir(), "fake-attest", 0)
	err := BuildSite(BuildOptions{SiteDir: dir, AttestPath: fake})
	if err != nil {
		t.Fatalf("BuildSite unsigned: %v", err)
	}
	html := readText(t, filepath.Join(dir, "public", "index.html"))
	if strings.Contains(html, "FAKE-SIGNED") {
		t.Fatalf("unsigned build was signed: %s", html)
	}
}

func TestKeyWithNoSignerFailsBeforeRendering(t *testing.T) {
	dir := testSite(t)
	outFile := filepath.Join(dir, "public", "index.html")
	err := BuildSite(BuildOptions{
		SiteDir:             dir,
		SignKey:             "KEY",
		attestExecutableDir: t.TempDir(),
		attestLookPath: func(name string) (string, error) {
			return "", exec.ErrNotFound
		},
	})
	if err == nil || !strings.Contains(err.Error(), "attest signer not found") {
		t.Fatalf("expected signer lookup failure, got %v", err)
	}
	if _, statErr := os.Stat(outFile); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("output was rendered despite signer failure or unexpected stat error: %v", statErr)
	}
}

func TestRepeatedSignedBuildsSucceed(t *testing.T) {
	dir := testSite(t)
	fake := fakeSigner(t, t.TempDir(), "fake-attest", 0)
	opts := BuildOptions{SiteDir: dir, SignKey: "KEY", AttestPath: fake}
	if err := BuildSite(opts); err != nil {
		t.Fatalf("first signed build: %v", err)
	}
	if err := BuildSite(opts); err != nil {
		t.Fatalf("second signed build: %v", err)
	}
}

func TestSigningStartFailureIsClear(t *testing.T) {
	input := filepath.Join(t.TempDir(), "input.html")
	writeText(t, input, "<!doctype html>")
	err := SignHTML(filepath.Join(t.TempDir(), "missing-attest"), "KEY", input, filepath.Join(t.TempDir(), "out.html"))
	if err == nil || !strings.Contains(err.Error(), "attest signer could not be started") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSignerNonzeroExitIsClear(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.html")
	output := filepath.Join(dir, "output.html")
	writeText(t, input, "<!doctype html>")
	fake := fakeSigner(t, dir, "fake-attest", 7)
	err := SignHTML(fake, "KEY", input, output)
	if err == nil || !strings.Contains(err.Error(), "attest signer failed") || !strings.Contains(err.Error(), "fake signer failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func fakeSigner(t *testing.T, dir, name string, exitCode int) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(fake signer dir): %v", err)
	}
	path := filepath.Join(dir, name)
	exit := strconv.Itoa(exitCode)
	body := `#!/bin/sh
if [ "$1" != "sign" ] || [ "$2" != "-k" ] || [ "$4" != "-o" ] || [ "$6" != "--force" ]; then
  echo "bad args: $@" >&2
  exit 9
fi
if [ ` + exit + ` -ne 0 ]; then
  echo "fake signer failed" >&2
  exit ` + exit + `
fi
cp "$7" "$5"
printf '\nFAKE-SIGNED key=%s\n' "$3" >> "$5"
`
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(fake signer): %v", err)
	}
	return path
}

func writeTinySignerProject(t *testing.T, dir, goModExtra string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(project): %v", err)
	}
	writeText(t, filepath.Join(dir, "go.mod"), "module tiny-signer\n\ngo 1.26\n"+goModExtra)
	writeText(t, filepath.Join(dir, "main.go"), `package main

import (
	"os"
)

func main() {
	if len(os.Args) != 8 || os.Args[1] != "sign" || os.Args[2] != "-k" || os.Args[4] != "-o" || os.Args[6] != "--force" {
		os.Exit(9)
	}
	input, err := os.ReadFile(os.Args[7])
	if err != nil {
		os.Exit(8)
	}
	if err := os.WriteFile(os.Args[5], append(input, []byte("\nTINY-SIGNED\n")...), 0644); err != nil {
		os.Exit(7)
	}
}
`)
}

func replaceInFile(t *testing.T, path, old, new string) {
	t.Helper()
	text := readText(t, path)
	if !strings.Contains(text, old) {
		t.Fatalf("%s did not contain %q", path, old)
	}
	writeText(t, path, strings.Replace(text, old, new, 1))
}
