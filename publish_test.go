package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type recordingPublishRunner struct {
	commands []publishCommand
	onRun    func(publishCommand) error
}

func (r *recordingPublishRunner) Run(command publishCommand) error {
	r.commands = append(r.commands, command)
	if r.onRun != nil {
		return r.onRun(command)
	}
	return nil
}

func TestPublishTargetCLIOverridesConfig(t *testing.T) {
	target, err := publishTargetFromConfig(PublishConfig{
		Method: "scp",
		Host:   "config.example",
		User:   "config-user",
		Port:   2200,
		Path:   "/srv/config",
	}, PublishOptions{
		Host:    "override.example",
		User:    "override-user",
		Port:    2022,
		Path:    "/srv/override",
		HostSet: true,
		UserSet: true,
		PortSet: true,
		PathSet: true,
	})
	if err != nil {
		t.Fatalf("publishTargetFromConfig: %v", err)
	}
	if target.Host != "override.example" || target.User != "override-user" || target.Port != 2022 || target.Path != "/srv/override" {
		t.Fatalf("target = %#v", target)
	}
}

func TestPublishTargetDefaultsMethodAndPort(t *testing.T) {
	target, err := publishTargetFromConfig(PublishConfig{
		Host: "example.org",
		Path: "/srv/site",
	}, PublishOptions{})
	if err != nil {
		t.Fatalf("publishTargetFromConfig: %v", err)
	}
	if target.Method != "scp" || target.Port != 22 {
		t.Fatalf("target defaults = %#v", target)
	}
}

func TestPublishTargetRejectsUnsafeSSHFields(t *testing.T) {
	tests := []PublishConfig{
		{Host: "-oProxyCommand=false", Path: "/srv/site"},
		{Host: "example.org", User: "-bad", Path: "/srv/site"},
		{Host: "example.org\nbad", Path: "/srv/site"},
	}
	for _, cfg := range tests {
		if _, err := publishTargetFromConfig(cfg, PublishOptions{}); err == nil {
			t.Fatalf("publishTargetFromConfig accepted %#v", cfg)
		}
	}
}

func TestPublishRejectsInvalidRemotePaths(t *testing.T) {
	tests := []string{
		"",
		"relative/path",
		"/",
		"/srv/../site",
		"/srv/site/",
		"/srv//site",
		"/srv/site\nx",
		"/srv/site\x00x",
	}
	for _, remotePath := range tests {
		t.Run(remotePath, func(t *testing.T) {
			if err := validateRemotePublishPath(remotePath); err == nil {
				t.Fatalf("validateRemotePublishPath accepted %q", remotePath)
			}
		})
	}
}

func TestPublishNoBuildRunsExpectedSSHAndSCPCommands(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "deploy", 2222, "/var/www/example")
	writeText(t, filepath.Join(dir, "public", "index.html"), "ready\n")
	runner := &recordingPublishRunner{}
	var out strings.Builder
	if err := PublishSite(PublishOptions{
		SiteDir:       dir,
		NoBuild:       true,
		Stdout:        &out,
		commandRunner: runner,
	}); err != nil {
		t.Fatalf("PublishSite: %v", err)
	}
	if len(runner.commands) != 3 {
		t.Fatalf("command count = %d, want 3: %#v", len(runner.commands), runner.commands)
	}
	if runner.commands[0].Name != "ssh" || strings.Join(runner.commands[0].Args, " ") != "-p 2222 deploy@example.org sh -s" {
		t.Fatalf("prepare command = %#v", runner.commands[0])
	}
	if !strings.Contains(runner.commands[0].Stdin, "mkdir -p \"$stage\"") || !strings.Contains(runner.commands[0].Stdin, "/var/www/.example.smol-upload") {
		t.Fatalf("prepare script = %q", runner.commands[0].Stdin)
	}
	scp := runner.commands[1]
	if scp.Name != "scp" || len(scp.Args) != 5 || scp.Args[0] != "-P" || scp.Args[1] != "2222" || scp.Args[2] != "-r" {
		t.Fatalf("scp command = %#v", scp)
	}
	if scp.Args[3] != filepath.Join(dir, "public")+string(filepath.Separator)+"." || scp.Args[4] != "deploy@example.org:/var/www/.example.smol-upload" {
		t.Fatalf("scp args = %#v", scp.Args)
	}
	if runner.commands[2].Name != "ssh" || !strings.Contains(runner.commands[2].Stdin, "mv \"$stage\" \"$target\"") || !strings.Contains(runner.commands[2].Stdin, "/var/www/.example.smol-backup") {
		t.Fatalf("swap command = %#v", runner.commands[2])
	}
	if !strings.Contains(out.String(), "published: public -> deploy@example.org:/var/www/example") {
		t.Fatalf("stdout = %s", out.String())
	}
}

func TestPublishBuildsBeforePublishingByDefault(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	runner := &recordingPublishRunner{
		onRun: func(command publishCommand) error {
			if _, err := os.Stat(filepath.Join(dir, "public", "index.html")); err != nil {
				t.Fatalf("publish ran before build output existed: %v", err)
			}
			return nil
		},
	}
	if err := PublishSite(PublishOptions{
		SiteDir:       dir,
		commandRunner: runner,
	}); err != nil {
		t.Fatalf("PublishSite: %v", err)
	}
	if len(runner.commands) != 3 {
		t.Fatalf("command count = %d, want 3", len(runner.commands))
	}
}

func TestPublishPassesSignKeyOverrideToBuild(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	var got BuildOptions
	runner := &recordingPublishRunner{}
	if err := PublishSite(PublishOptions{
		SiteDir:       dir,
		SignKey:       "CLI-KEY",
		SignKeySource: true,
		commandRunner: runner,
		buildSite: func(opts BuildOptions) error {
			got = opts
			return BuildSite(BuildOptions{
				SiteDir:  opts.SiteDir,
				OutDir:   opts.OutDir,
				Unsigned: true,
				Force:    opts.Force,
				Stdout:   opts.Stdout,
			})
		},
	}); err != nil {
		t.Fatalf("PublishSite: %v", err)
	}
	if got.SignKey != "CLI-KEY" || !got.SignKeySource {
		t.Fatalf("build options SignKey=%q SignKeySource=%v, want CLI override", got.SignKey, got.SignKeySource)
	}
	if len(runner.commands) != 3 {
		t.Fatalf("command count = %d, want 3", len(runner.commands))
	}
}

func TestPublishBuildCleansOutputBeforePublishing(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	stalePath := filepath.Join(dir, "public", "stale.html")
	writeText(t, stalePath, "stale\n")
	runner := &recordingPublishRunner{
		onRun: func(command publishCommand) error {
			if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
				t.Fatalf("stale output remains before publish: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "public", "index.html")); err != nil {
				t.Fatalf("fresh output missing before publish: %v", err)
			}
			return nil
		},
	}
	if err := PublishSite(PublishOptions{
		SiteDir:       dir,
		commandRunner: runner,
	}); err != nil {
		t.Fatalf("PublishSite: %v", err)
	}
	if len(runner.commands) != 3 {
		t.Fatalf("command count = %d, want 3", len(runner.commands))
	}
}

func TestPublishNoBuildRequiresExistingOutputDirectory(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	if err := os.RemoveAll(filepath.Join(dir, "public")); err != nil {
		t.Fatal(err)
	}
	runner := &recordingPublishRunner{}
	err := PublishSite(PublishOptions{
		SiteDir:       dir,
		NoBuild:       true,
		commandRunner: runner,
	})
	if err == nil || !strings.Contains(err.Error(), "output directory not found") {
		t.Fatalf("expected missing output error, got %v", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands ran despite missing output: %#v", runner.commands)
	}
}

func TestPublishNoBuildRejectsExplicitSignKeyAtAPI(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	writeText(t, filepath.Join(dir, "public", "index.html"), "ready\n")
	err := PublishSite(PublishOptions{
		SiteDir:       dir,
		NoBuild:       true,
		SignKey:       "KEY",
		SignKeySource: true,
	})
	if err == nil || !strings.Contains(err.Error(), "--no-build skips the build phase") {
		t.Fatalf("expected no-build signing error, got %v", err)
	}
}

func TestPublishNoBuildRejectsOutputFile(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	if err := os.RemoveAll(filepath.Join(dir, "public")); err != nil {
		t.Fatal(err)
	}
	writeText(t, filepath.Join(dir, "public"), "not a directory\n")
	err := PublishSite(PublishOptions{SiteDir: dir, NoBuild: true})
	if err == nil || !strings.Contains(err.Error(), "output path is not a directory") {
		t.Fatalf("expected output file error, got %v", err)
	}
}

func TestPublishDryRunValidatesAndPrintsWithoutBuildOrCommands(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "config.example", "config-user", 2200, "/srv/config")
	if err := os.RemoveAll(filepath.Join(dir, "public")); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	if err := commandPublish([]string{
		"--dry-run",
		"--host", "override.example",
		"--user", "deploy",
		"--port", "2022",
		"--path", "/srv/override",
		dir,
	}, &out); err != nil {
		t.Fatalf("commandPublish: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "public")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created output directory or unexpected stat error: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"dry-run: would build output: public",
		"ssh -p 2022 deploy@override.example sh -s < remote-script",
		"scp -P 2022 -r",
		"deploy@override.example:/srv/.override.smol-upload",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, got)
		}
	}
}

func TestPublishDryRunHonorsNoBuild(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	var out strings.Builder
	if err := commandPublish([]string{"--dry-run", "--no-build", dir}, &out); err != nil {
		t.Fatalf("commandPublish: %v", err)
	}
	if !strings.Contains(out.String(), "dry-run: would publish existing output: public") {
		t.Fatalf("dry-run output = %s", out.String())
	}
}

func TestPublishNoBuildRejectsExplicitSigningFlags(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	tests := map[string][]string{
		"interactive sign": {"--no-build", "--sign", dir},
		"explicit key":     {"--no-build", "--sign-key", "KEY", dir},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			err := commandPublish(args, &strings.Builder{})
			if err == nil || !strings.Contains(err.Error(), "--no-build skips the build phase") {
				t.Fatalf("commandPublish error = %v, want no-build signing usage error", err)
			}
		})
	}
}

func TestPublishNoBuildAllowsConfigSignKeyAndUnsignedSigningFlags(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	replaceInFile(t, filepath.Join(dir, "smol.json"), `"sign_key": ""`, `"sign_key": "CONFIG"`)
	writeText(t, filepath.Join(dir, "public", "index.html"), "ready\n")
	var out strings.Builder
	if err := commandPublish([]string{"--dry-run", "--no-build", "--unsigned", "--sign", dir}, &out); err != nil {
		t.Fatalf("commandPublish no-build unsigned sign: %v", err)
	}
	if !strings.Contains(out.String(), "dry-run: would publish existing output: public") {
		t.Fatalf("stdout = %s", out.String())
	}
}

func TestPublishDryRunNoBuildRequiresExistingOutputDirectory(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	if err := os.RemoveAll(filepath.Join(dir, "public")); err != nil {
		t.Fatal(err)
	}
	err := commandPublish([]string{"--dry-run", "--no-build", dir}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "output directory not found") {
		t.Fatalf("expected missing output error, got %v", err)
	}
}

func TestPublishDryRunRejectsUnsafeBuildOutputDirectory(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	err := commandPublish([]string{"--dry-run", "--out", ".", dir}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "refusing to remove output directory that contains site directory") {
		t.Fatalf("expected unsafe output error, got %v", err)
	}
}

func TestPublishDryRunSignDoesNotPrompt(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	called := false
	withSecretKeys(t, func() ([]gpgSecretKey, error) {
		called = true
		return []gpgSecretKey{{Fingerprint: "FPR", UID: "Curator <curator@example.org>"}}, nil
	})
	var out, stderr strings.Builder
	if err := commandPublishWithIO([]string{"--dry-run", "--sign", dir}, strings.NewReader("y\n"), &out, &stderr, true); err != nil {
		t.Fatalf("commandPublishWithIO dry-run sign: %v", err)
	}
	if !called {
		t.Fatalf("dry-run --sign did not probe secret keys")
	}
	if stderr.String() != "" {
		t.Fatalf("dry-run --sign prompted on stderr: %s", stderr.String())
	}
	got := out.String()
	if !strings.Contains(got, "dry-run: would build output: public") || !strings.Contains(got, "dry-run: signing key would be resolved on a real run") {
		t.Fatalf("dry-run output = %s", got)
	}
}

func TestPublishDryRunSignKeyNotesExplicitKey(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	withSecretKeys(t, func() ([]gpgSecretKey, error) {
		t.Fatalf("dry-run --sign-key should not list secret keys")
		return nil, nil
	})
	var out strings.Builder
	if err := commandPublishWithIO([]string{"--dry-run", "--sign-key", "KEY", dir}, strings.NewReader(""), &out, &strings.Builder{}, false); err != nil {
		t.Fatalf("commandPublishWithIO dry-run sign-key: %v", err)
	}
	if !strings.Contains(out.String(), "dry-run: would sign with key KEY") {
		t.Fatalf("dry-run output missing explicit signing note: %s", out.String())
	}
}

func TestPublishDryRunSignPreflightsBeforeKeyProbe(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "", "", 22, "/srv/site")
	withSecretKeys(t, func() ([]gpgSecretKey, error) {
		t.Fatalf("dry-run --sign listed keys before publish target validation")
		return nil, nil
	})
	err := commandPublishWithIO([]string{"--dry-run", "--sign", dir}, strings.NewReader("y\n"), &strings.Builder{}, &strings.Builder{}, true)
	if err == nil || !strings.Contains(err.Error(), "publish host is required") {
		t.Fatalf("expected publish target validation error, got %v", err)
	}
}

func TestPublishDryRunSignAvailabilityErrors(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	tests := map[string]struct {
		interactive bool
		keys        func() ([]gpgSecretKey, error)
		want        string
	}{
		"non interactive": {
			interactive: false,
			keys: func() ([]gpgSecretKey, error) {
				return []gpgSecretKey{{Fingerprint: "FPR"}}, nil
			},
			want: "interactive signing requires a terminal",
		},
		"zero keys": {
			interactive: true,
			keys: func() ([]gpgSecretKey, error) {
				return nil, nil
			},
			want: "no GPG secret keys found",
		},
		"missing gpg": {
			interactive: true,
			keys: func() ([]gpgSecretKey, error) {
				return nil, errors.New("gpg not found on PATH")
			},
			want: "gpg not found on PATH",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			withSecretKeys(t, tc.keys)
			err := commandPublishWithIO([]string{"--dry-run", "--sign", dir}, strings.NewReader(""), &strings.Builder{}, &strings.Builder{}, tc.interactive)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("commandPublishWithIO error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestPublishCommandInteractiveSignAccepts(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	withSecretKeys(t, func() ([]gpgSecretKey, error) {
		return []gpgSecretKey{{Fingerprint: "FPR", UID: "Curator <curator@example.org>"}}, nil
	})
	original := runPublishSite
	var got PublishOptions
	runPublishSite = func(opts PublishOptions) error {
		got = opts
		return nil
	}
	t.Cleanup(func() {
		runPublishSite = original
	})
	var out, stderr strings.Builder
	if err := commandPublishWithIO([]string{"--sign", dir}, strings.NewReader("y\n"), &out, &stderr, true); err != nil {
		t.Fatalf("commandPublishWithIO: %v", err)
	}
	if got.SignKey != "FPR" || !got.SignKeySource {
		t.Fatalf("publish options SignKey=%q SignKeySource=%v, want selected key", got.SignKey, got.SignKeySource)
	}
	if !strings.Contains(stderr.String(), "Curator <curator@example.org>") {
		t.Fatalf("stderr missing signing prompt: %s", stderr.String())
	}
}

func writePublishConfig(t *testing.T, dir, host, user string, port int, remotePath string) {
	t.Helper()
	writeText(t, filepath.Join(dir, "smol.json"), `{
  "format": "smol-site-v1",
  "title": "Example Site",
  "description": "Minimal signed pages.",
  "language": "en",
  "base_url": "https://example.org",
  "publisher": "Example Publisher",
  "theme": "default",
  "sign_key": "",
  "nav": [
    {
      "label": "Home",
      "url": "/"
    }
  ],
  "publish": {
    "method": "scp",
    "host": "`+host+`",
    "user": "`+user+`",
    "port": `+strconv.Itoa(port)+`,
    "path": "`+remotePath+`"
  }
}
`)
}
