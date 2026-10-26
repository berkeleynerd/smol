package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Use real attest/GPG with disposable keys; no transport or verification bypass.
// Standalone smol checkouts can provide the peer repo with SMOL_ATTEST_SOURCE.
func configurePublishSigning(t *testing.T, site string) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("gpg"); err != nil {
		t.Skip("real signing integration requires gpg")
	}
	source := os.Getenv("SMOL_ATTEST_SOURCE")
	if source == "" {
		for _, candidate := range []string{"../attest", "../web-ring/attest"} {
			if _, err := os.Stat(filepath.Join(candidate, "gpg.go")); err == nil {
				source = candidate
				break
			}
		}
	}
	if source == "" {
		t.Skip("real signing integration requires attest source (SMOL_ATTEST_SOURCE)")
	}
	tool := filepath.Join(t.TempDir(), "attest")
	build := exec.Command("go", "build", "-o", tool, ".")
	build.Dir = source
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build attest: %v %s", err, out)
	}
	home, err := os.MkdirTemp("/tmp", "smol-gpg-")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GNUPGHOME", home)
	t.Cleanup(func() { _ = exec.Command("gpgconf", "--kill", "all").Run(); os.RemoveAll(home) })
	command := exec.Command("gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase", "", "--quick-generate-key", "Smol Test <smol@example.invalid>", "ed25519", "sign", "1d")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generate disposable key: %v %s", err, out)
	}
	command = exec.Command("gpg", "--batch", "--with-colons", "--list-secret-keys", "smol@example.invalid")
	out, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := ""
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(line, ":")
		if len(fields) > 9 && fields[0] == "fpr" {
			fingerprint = fields[9]
			break
		}
	}
	if !fullFingerprintRE.MatchString(fingerprint) {
		t.Fatalf("missing fingerprint: %s", out)
	}
	replaceInFile(t, filepath.Join(site, "smol.json"), `"sign_key": ""`, `"sign_key": "`+fingerprint+`"`)
	return tool, fingerprint
}

func TestPublishRejectsUnverifiedPagesBeforeTransport(t *testing.T) {
	site := testSite(t)
	writePublishConfig(t, site, "example.org", "", 22, "/srv/site")
	tool, fingerprint := configurePublishSigning(t, site)
	if err := BuildSite(BuildOptions{SiteDir: site, AttestPath: tool}); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(site, "public")
	signedFixture := func(dir, document string) {
		t.Helper()
		input := filepath.Join(t.TempDir(), "input.html")
		writeText(t, input, document)
		command := exec.Command(tool, "sign", "-k", fingerprint, "--out", filepath.Join(dir, "index.html"), "--force", input)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("sign fixture: %v %s", err, out)
		}
	}
	cases := map[string]func(string){
		"unsigned": func(dir string) {
			writeText(t, filepath.Join(dir, "index.html"), xhtmlDocument("", "<p>Unsigned</p>"))
			os.Remove(filepath.Join(dir, "index.html.gz"))
		},
		"signed external resource": func(dir string) {
			signedFixture(dir, xhtmlDocument("", `<p style="background:url(https://example.invalid/image.png)">Signed but not self-contained</p>`))
		},
		"signed stylesheet comment bypass": func(dir string) {
			signedFixture(dir, xhtmlDocument(`<style>p::before{content:"/*"} body{background-image:url("https://example.invalid/image")} p::after{content:"*/"}</style>`, "<p>Signed</p>"))
		},
		"signed attribute comment bypass": func(dir string) {
			signedFixture(dir, xhtmlDocument("", `<p style="--a:'/*'; background-image:url('https://example.invalid/image'); --b:'*/'">Signed</p>`))
		},
		"signed missing encoding": func(dir string) {
			signedFixture(dir, strings.Replace(xhtmlDocument("", "<p>Café</p>"), `<meta charset="utf-8" />`, "", 1))
		},
		"modified content": func(dir string) {
			p := filepath.Join(dir, "index.html")
			writeText(t, p, strings.Replace(readText(t, p), "Write your page here.", "Attacker's page here.", 1))
		},
		"appended content": func(dir string) {
			p := filepath.Join(dir, "index.html")
			writeText(t, p, readText(t, p)+"<!-- Unsigned appended content -->")
		},
		"malformed XML": func(dir string) {
			p := filepath.Join(dir, "index.html")
			writeText(t, p, strings.Replace(readText(t, p), "<p>", "<p>&", 1))
		},
		"gzip mismatch": func(dir string) {
			var b bytes.Buffer
			w := gzip.NewWriter(&b)
			w.Write([]byte("different content"))
			w.Close()
			writeBytes(t, filepath.Join(dir, "index.html.gz"), b.Bytes())
		},
		"symlink": func(dir string) {
			if err := os.Symlink(filepath.Join(source, "index.html"), filepath.Join(dir, "other.html")); err != nil {
				t.Fatal(err)
			}
		},
		"wrong expected key": func(dir string) {
			replaceInFile(t, filepath.Join(site, "smol.json"), fingerprint, strings.Repeat("0", 40))
		},
	}
	config := readText(t, filepath.Join(site, "smol.json"))
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			defer writeText(t, filepath.Join(site, "smol.json"), config)
			dir := t.TempDir()
			if err := copyTree(source, dir); err != nil {
				t.Fatal(err)
			}
			mutate(dir)
			runner := &recordingPublishRunner{}
			err := PublishSite(PublishOptions{SiteDir: site, OutDir: dir, NoBuild: true, AttestPath: tool, commandRunner: runner})
			if err == nil {
				t.Fatal("unsafe publication accepted")
			}
			if len(runner.commands) != 0 {
				t.Fatalf("transport ran before verification failed: %#v", runner.commands)
			}
		})
	}
}

func TestPublishUploadsVerifiedSnapshotWithoutManifest(t *testing.T) {
	site := testSite(t)
	writePublishConfig(t, site, "example.org", "", 22, "/srv/site")
	tool, _ := configurePublishSigning(t, site)
	replaceInFile(t, filepath.Join(site, "themes", "default", "partials", "head.html.tmpl"), "{{.Smol.ManifestComment}}", "")
	if err := BuildSite(BuildOptions{SiteDir: site, AttestPath: tool}); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(site, "public", "index.html")
	original := readText(t, page)
	if strings.Contains(original, manifestCommentStart) {
		t.Fatal("fixture unexpectedly contains a manifest")
	}
	snapshot := ""
	runner := &recordingPublishRunner{onRun: func(command publishCommand) error {
		if command.Name == "ssh" {
			writeText(t, page, "edited after verification")
		}
		if command.Name == "scp" {
			snapshot = filepath.Clean(command.Args[3])
			if readText(t, filepath.Join(snapshot, "index.html")) != original {
				t.Fatal("upload uses changed source bytes")
			}
			entries, err := os.ReadDir(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.Name() != "index.html" && entry.Name() != "index.html.gz" {
					t.Fatalf("unexpected snapshot file: %s", entry.Name())
				}
			}
		}
		return nil
	}}
	if err := PublishSite(PublishOptions{SiteDir: site, NoBuild: true, AttestPath: tool, commandRunner: runner}); err != nil {
		t.Fatal(err)
	}
	if snapshot == "" {
		t.Fatal("no snapshot uploaded")
	}
	if _, err := os.Stat(snapshot); !os.IsNotExist(err) {
		t.Fatalf("snapshot was not removed: %v", err)
	}
	last := runner.commands[len(runner.commands)-1].Stdin
	if strings.Index(last, "Uploaded bytes differ") > strings.Index(last, `mv "$stage" "$target"`) {
		t.Fatal("remote validation occurs after activation")
	}
}

func TestRemoteByteVerificationBeforeActivation(t *testing.T) {
	path := "nested/page\n ' \\ $literal.html"
	data := []byte("signed bytes")
	inventory := publishInventory{hashes: map[string]string{path: fmtHash(data)}, directories: []string{"nested", "empty", "empty/sub"}}
	symlink := func(target, link string) {
		t.Helper()
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	cases := map[string]func(string){
		"valid":         func(stage string) {},
		"changed bytes": func(stage string) { writeText(t, filepath.Join(stage, path), "changed in transit") },
		"extra file":    func(stage string) { writeText(t, filepath.Join(stage, "injected.html"), "extra") },
		"hidden file":   func(stage string) { writeText(t, filepath.Join(stage, ".hidden"), "extra") },
		"newline file":  func(stage string) { writeText(t, filepath.Join(stage, "extra\nfile"), "extra") },
		"extra directory": func(stage string) {
			if err := os.Mkdir(filepath.Join(stage, "unlisted"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"extra symlink": func(stage string) { symlink("nested", filepath.Join(stage, "link")) },
		"extra special file": func(stage string) {
			if _, err := exec.LookPath("mkfifo"); err != nil {
				t.Skip("requires POSIX mkfifo")
			}
			if out, err := exec.Command("mkfifo", filepath.Join(stage, "pipe")).CombinedOutput(); err != nil {
				t.Fatalf("mkfifo: %v %s", err, out)
			}
		},
		"missing file": func(stage string) {
			if err := os.Remove(filepath.Join(stage, path)); err != nil {
				t.Fatal(err)
			}
		},
		"missing empty directory": func(stage string) {
			if err := os.Remove(filepath.Join(stage, "empty", "sub")); err != nil {
				t.Fatal(err)
			}
		},
		"file symlink": func(stage string) {
			file := filepath.Join(stage, path)
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.Rename(file, outside); err != nil {
				t.Fatal(err)
			}
			symlink(outside, file)
		},
		"parent symlink": func(stage string) {
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.Rename(filepath.Join(stage, "nested"), outside); err != nil {
				t.Fatal(err)
			}
			symlink(outside, filepath.Join(stage, "nested"))
		},
		"stage symlink": func(stage string) {
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.Rename(stage, outside); err != nil {
				t.Fatal(err)
			}
			symlink(outside, stage)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			stage := filepath.Join(dir, "stage ' with spaces")
			target := filepath.Join(dir, "live")
			backup := filepath.Join(dir, "backup")
			writeText(t, filepath.Join(target, "old.html"), "old live content")
			writeBytes(t, filepath.Join(stage, path), data)
			if err := os.MkdirAll(filepath.Join(stage, "empty", "sub"), 0o755); err != nil {
				t.Fatal(err)
			}
			mutate(stage)
			script := remoteVerifyScript(stage, inventory) + remoteSwapScript(target, stage, backup)
			command := exec.Command("sh", "-s")
			command.Stdin = strings.NewReader(script)
			out, err := command.CombinedOutput()
			if name != "valid" {
				if err == nil {
					t.Fatal("changed upload activated")
				}
				if readText(t, filepath.Join(target, "old.html")) != "old live content" {
					t.Fatal("corrupt upload replaced live site")
				}
			} else {
				if err != nil {
					t.Fatalf("valid upload failed: %v %s", err, out)
				}
				if readText(t, filepath.Join(target, path)) != string(data) {
					t.Fatal("activated bytes differ")
				}
			}
		})
	}
}

func TestPublishInventoryRetainsEmptyDirectories(t *testing.T) {
	dir := t.TempDir()
	writeText(t, filepath.Join(dir, "index.gmi"), "# Home\n\nText.\n")
	if err := os.MkdirAll(filepath.Join(dir, "empty", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	snapshot, err := snapshotOutput(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(snapshot)
	inventory, err := verifyPublishOutput(snapshot, "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.directories) != 2 || len(inventory.hashes) != 1 {
		t.Fatalf("inventory: %#v", inventory)
	}
	command := exec.Command("sh", "-s")
	command.Stdin = strings.NewReader(remoteVerifyScript(snapshot, inventory))
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("exact snapshot failed: %v %s", err, out)
	}
}

func fmtHash(data []byte) string { sum := sha256.Sum256(data); return fmt.Sprintf("%x", sum) }
