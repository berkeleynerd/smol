package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func localPublishRunner(t *testing.T, output string) *recordingPublishRunner {
	t.Helper()
	return &recordingPublishRunner{onRun: func(command publishCommand) error {
		if command.Name == "ssh" {
			cmd := exec.Command("sh", "-s")
			cmd.Stdin = strings.NewReader(command.Stdin)
			data, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("remote script: %w: %s", err, data)
			}
			return nil
		}
		_, stage, ok := strings.Cut(command.Args[len(command.Args)-1], ":")
		if !ok {
			return fmt.Errorf("missing SCP destination")
		}
		output = filepath.Clean(command.Args[len(command.Args)-2])
		return filepath.WalkDir(output, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(output, path)
			if err != nil {
				return err
			}
			if rel == "." {
				return nil
			}
			dest := filepath.Join(stage, rel)
			if entry.IsDir() {
				return os.MkdirAll(dest, 0755)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(dest, data, 0644)
		})
	}}
}

func TestPublishInPlacePreservesRootAndObsoleteFilesWithReadOnlyParent(t *testing.T) {
	dir := testSite(t)
	parent := filepath.Join(t.TempDir(), "parent")
	target := filepath.Join(parent, "site's files")
	writeText(t, filepath.Join(target, "index.html"), "old")
	writeText(t, filepath.Join(target, "obsolete.html"), "keep this")
	writeText(t, filepath.Join(dir, "public", "index.html"), "new signed bytes")
	writeText(t, filepath.Join(dir, "public", "nested", "page's name.html"), "nested signed bytes")
	if err := os.Chmod(filepath.Join(target, "index.html"), 0444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0755) })
	before, _ := os.Stat(target)
	writePublishConfig(t, dir, "example.org", "deploy", 22, target)
	tool := signInPlaceOutput(t, dir)
	wantSigned := readText(t, filepath.Join(dir, "public", "index.html"))
	runner := localPublishRunner(t, filepath.Join(dir, "public"))
	copyFiles := runner.onRun
	runner.onRun = func(command publishCommand) error {
		if err := copyFiles(command); err != nil {
			return err
		}
		if command.Name == "scp" {
			_, stage, _ := strings.Cut(command.Args[len(command.Args)-1], ":")
			if err := os.Chmod(filepath.Join(stage, "nested"), 0555); err != nil {
				return err
			}
			return os.Chmod(stage, 0555)
		}
		return nil
	}
	if err := PublishSite(PublishOptions{SiteDir: dir, NoBuild: true, AttestPath: tool,
		Mode: publishModeInPlace, ModeSet: true, commandRunner: runner}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(target)
	if !os.SameFile(before, after) || after.Mode().Perm() != 0750 {
		t.Fatal("in-place publishing replaced or chmodded the destination directory")
	}
	for name, want := range map[string]string{"index.html": wantSigned, "obsolete.html": "keep this",
		"nested/page's name.html": wantSigned} {
		if got := readText(t, filepath.Join(target, name)); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
	entries, err := os.ReadDir(parent)
	if err != nil || len(entries) != 1 {
		t.Fatalf("publisher touched destination parent: %v, %v", entries, err)
	}
	stages, _ := filepath.Glob(filepath.Join(target, ".smol-upload-*"))
	if len(stages) != 0 {
		t.Fatalf("staging directories not cleaned up: %v", stages)
	}
}

func TestPublishInPlaceRejectsRemoteSymlinkBeforeTransfer(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(fmt.Sprint(directory), func(t *testing.T) {
			dir := testSite(t)
			target := t.TempDir()
			outside := t.TempDir()
			name := "index.html"
			linkTarget := filepath.Join(outside, "index.html")
			writeText(t, linkTarget, "outside")
			if directory {
				name = "nested"
				linkTarget = outside
				writeText(t, filepath.Join(dir, "public", name, "page.html"), "new")
			} else {
				writeText(t, filepath.Join(dir, "public", name), "new")
			}
			if err := os.Symlink(linkTarget, filepath.Join(target, name)); err != nil {
				t.Fatal(err)
			}
			writePublishConfig(t, dir, "example.org", "", 22, target)
			tool := signInPlaceOutput(t, dir)
			runner := localPublishRunner(t, filepath.Join(dir, "public"))
			err := PublishSite(PublishOptions{SiteDir: dir, NoBuild: true, AttestPath: tool,
				Mode: publishModeInPlace, ModeSet: true, commandRunner: runner})
			if err == nil || len(runner.commands) != 2 || readText(t, filepath.Join(outside, "index.html")) != "outside" {
				t.Fatalf("unsafe destination was not rejected before transfer: %v, %v", err, runner.commands)
			}
		})
	}
}

func TestPublishInPlaceRejectsLocalSymlinkBeforeConnecting(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "example.org", "", 22, "/srv/site")
	writeText(t, filepath.Join(dir, "public", "index.html"), "page")
	if err := os.Symlink(filepath.Join(dir, "smol.json"), filepath.Join(dir, "public", "secret.html")); err != nil {
		t.Fatal(err)
	}
	tool := signInPlaceOutput(t, dir)
	runner := &recordingPublishRunner{}
	err := PublishSite(PublishOptions{SiteDir: dir, NoBuild: true, AttestPath: tool,
		Mode: publishModeInPlace, ModeSet: true, commandRunner: runner})
	if err == nil || !strings.Contains(err.Error(), "symlink") || len(runner.commands) != 0 {
		t.Fatalf("symlink output was not rejected locally: %v", err)
	}
}

func TestPublishInPlaceFailedUploadKeepsOldFilesAndCleansStaging(t *testing.T) {
	dir := testSite(t)
	target := t.TempDir()
	writeText(t, filepath.Join(dir, "public", "index.html"), "new")
	writeText(t, filepath.Join(target, "index.html"), "old")
	writePublishConfig(t, dir, "example.org", "", 22, target)
	tool := signInPlaceOutput(t, dir)
	runner := localPublishRunner(t, filepath.Join(dir, "public"))
	localRun := runner.onRun
	runner.onRun = func(command publishCommand) error {
		if command.Name == "scp" {
			return errors.New("transfer interrupted")
		}
		return localRun(command)
	}
	err := PublishSite(PublishOptions{SiteDir: dir, NoBuild: true, AttestPath: tool,
		Mode: publishModeInPlace, ModeSet: true, commandRunner: runner})
	stages, _ := filepath.Glob(filepath.Join(target, ".smol-upload-*"))
	if err == nil || readText(t, filepath.Join(target, "index.html")) != "old" || len(stages) != 0 {
		t.Fatalf("failed transfer changed live files or leaked staging: %v, %v", err, stages)
	}
}

func TestPublishCheckSSHDoesNotBuildUploadOrCreateStaging(t *testing.T) {
	dir := testSite(t)
	target := t.TempDir()
	writeText(t, filepath.Join(dir, "public", "index.html"), "page")
	writePublishConfig(t, dir, "example.org", "", 22, target)
	runner := localPublishRunner(t, filepath.Join(dir, "public"))
	err := PublishSite(PublishOptions{SiteDir: dir, CheckSSH: true,
		Mode: publishModeInPlace, ModeSet: true, commandRunner: runner,
		buildSite: func(BuildOptions) error { t.Fatal("SSH check tried to build"); return nil }})
	entries, _ := os.ReadDir(target)
	if err != nil || len(runner.commands) != 1 || runner.commands[0].Name != "ssh" || len(entries) != 0 {
		t.Fatalf("SSH check had side effects: %v, %v, %v", err, runner.commands, entries)
	}
}

// Keep the previous transport fixtures but use actual signed XHTML5 and matching
// gzip bytes so transport tests cannot bypass the mandatory publication gate.
func signInPlaceOutput(t *testing.T, site string) string {
	t.Helper()
	tool, _ := configurePublishSigning(t, site)
	if err := BuildSite(BuildOptions{SiteDir: site, AttestPath: tool}); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(site, "public")
	signed := readText(t, filepath.Join(output, "index.html"))
	if err := filepath.WalkDir(output, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() && strings.HasSuffix(path, ".html") {
			writeText(t, path, signed)
			return writeGzipSibling(path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return tool
}

func TestPublishInPlaceVerifiesExactSnapshotBeforeActivation(t *testing.T) {
	site := testSite(t)
	writePublishConfig(t, site, "example.org", "", 22, "/srv/site")
	tool := signInPlaceOutput(t, site)
	output := filepath.Join(site, "public")
	original := readText(t, filepath.Join(output, "index.html"))
	if err := os.Mkdir(filepath.Join(output, "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(string){
		"valid":         func(stage string) {},
		"changed bytes": func(stage string) { writeText(t, filepath.Join(stage, "index.html"), "changed upload") },
		"extra file":    func(stage string) { writeText(t, filepath.Join(stage, "injected.html"), "extra") },
		"extra directory": func(stage string) {
			if err := os.Mkdir(filepath.Join(stage, "extra"), 0755); err != nil {
				t.Fatal(err)
			}
		},
		"missing file": func(stage string) {
			if err := os.Remove(filepath.Join(stage, "index.html")); err != nil {
				t.Fatal(err)
			}
		},
		"symlink": func(stage string) {
			p := filepath.Join(stage, "index.html")
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(output, "index.html"), p); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			target := t.TempDir()
			writeText(t, filepath.Join(target, "index.html"), "old live")
			writeText(t, filepath.Join(target, "obsolete.html"), "keep")
			defer writeText(t, filepath.Join(output, "index.html"), original)
			runner := localPublishRunner(t, output)
			localRun := runner.onRun
			runner.onRun = func(command publishCommand) error {
				if command.Name == "ssh" {
					writeText(t, filepath.Join(output, "index.html"), "edited after verification")
				}
				if command.Name == "scp" {
					source := filepath.Clean(command.Args[len(command.Args)-2])
					if source == output || readText(t, filepath.Join(source, "index.html")) != original {
						t.Fatal("not uploading verified private snapshot")
					}
				}
				if err := localRun(command); err != nil {
					return err
				}
				if command.Name == "scp" {
					_, stage, _ := strings.Cut(command.Args[len(command.Args)-1], ":")
					mutate(stage)
				}
				return nil
			}
			err := PublishSite(PublishOptions{SiteDir: site, NoBuild: true, AttestPath: tool, Path: target, PathSet: true, Mode: publishModeInPlace, ModeSet: true, commandRunner: runner})
			if name == "valid" {
				if err != nil || readText(t, filepath.Join(target, "index.html")) != original {
					t.Fatalf("verified in-place install failed: %v", err)
				}
				if info, err := os.Stat(filepath.Join(target, "empty")); err != nil || !info.IsDir() {
					t.Fatalf("empty directory missing: %v", err)
				}
			} else if err == nil || readText(t, filepath.Join(target, "index.html")) != "old live" {
				t.Fatalf("invalid upload activated: %v", err)
			}
			if readText(t, filepath.Join(target, "obsolete.html")) != "keep" {
				t.Fatal("obsolete remote file changed")
			}
			stages, _ := filepath.Glob(filepath.Join(target, ".smol-upload-*"))
			if len(stages) != 0 {
				t.Fatalf("staging not cleaned: %v", stages)
			}
		})
	}
}

func TestPublishInPlacePrepareFailureCleansStaging(t *testing.T) {
	site := testSite(t)
	target := t.TempDir()
	writePublishConfig(t, site, "example.org", "", 22, target)
	tool := signInPlaceOutput(t, site)
	writeText(t, filepath.Join(target, "index.html"), "old live")
	runner := localPublishRunner(t, filepath.Join(site, "public"))
	localRun := runner.onRun
	prepared := false
	runner.onRun = func(command publishCommand) error {
		if err := localRun(command); err != nil {
			return err
		}
		if !prepared {
			prepared = true
			return errors.New("connection lost after prepare")
		}
		return nil
	}
	err := PublishSite(PublishOptions{SiteDir: site, NoBuild: true, AttestPath: tool, Mode: publishModeInPlace, ModeSet: true, commandRunner: runner})
	stages, _ := filepath.Glob(filepath.Join(target, ".smol-upload-*"))
	if err == nil || len(stages) != 0 || readText(t, filepath.Join(target, "index.html")) != "old live" {
		t.Fatalf("prepare failure leaked stage or changed output: %v %v", err, stages)
	}
}
