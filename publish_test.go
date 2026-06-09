package main

import (
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
