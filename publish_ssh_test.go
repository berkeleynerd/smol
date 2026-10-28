package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testPublishHostKey = "AAAAC3NzaC1lZDI1NTE5AAAAIFfMfnq19pHHlNygVv1oNn2DGC7np23IITFqNdW5hhX9"
const testPublishHostFingerprint = "SHA256:qeV8Atw/C/i4emFsy+aPutH8lKZ+OA2xBmhM0ZsvWWc"

func pinnedPublishFixture(t *testing.T) (string, PublishConfig) {
	t.Helper()
	dir := t.TempDir()
	writeText(t, filepath.Join(dir, "deploy", "identity"), "identity placeholder, never read")
	writeText(t, filepath.Join(dir, "deploy", "known hosts%file"), "example.org ssh-ed25519 "+testPublishHostKey+"\n")
	return dir, PublishConfig{Host: "example.org", User: "deploy", Path: "/srv/site",
		IdentityFile: "deploy/identity", KnownHostsFile: "deploy/known hosts%file",
		HostKeyFingerprint: testPublishHostFingerprint}
}

func TestPublishSSHPinAndIdentityApplyToSSHAndSCP(t *testing.T) {
	dir, cfg := pinnedPublishFixture(t)
	target, err := publishTargetFromConfig(cfg, PublishOptions{SiteDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{sshArgs(target), scpArgs(target, "/tmp/output", "/srv/stage")} {
		joined := strings.Join(args, "\n")
		for _, want := range []string{"-F\n/dev/null", "-i\n" + filepath.Join(dir, "deploy/identity"),
			"BatchMode=yes", "IdentitiesOnly=yes", "PasswordAuthentication=no", "KbdInteractiveAuthentication=no",
			"StrictHostKeyChecking=yes", "GlobalKnownHostsFile=/dev/null", "UpdateHostKeys=no",
			`UserKnownHostsFile="` + filepath.Join(dir, "deploy/known hosts%%file") + `"`, "HostKeyAlgorithms=ssh-ed25519"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("missing SSH policy %q in %v", want, args)
			}
		}
	}
}

func TestPublishSSHPinRejectsAmbiguousWrongOrMissingKeys(t *testing.T) {
	for _, name := range []string{"wrong-fingerprint", "wrong-host", "extra-key", "missing-pin", "missing-file", "wrong-key-type"} {
		t.Run(name, func(t *testing.T) {
			dir, cfg := pinnedPublishFixture(t)
			path := filepath.Join(dir, cfg.KnownHostsFile)
			switch name {
			case "wrong-fingerprint":
				cfg.HostKeyFingerprint = "SHA256:incorrect"
			case "wrong-host":
				cfg.Host = "other.example"
			case "extra-key":
				writeText(t, path, readText(t, path)+"other.example ssh-ed25519 "+testPublishHostKey+"\n")
			case "missing-pin":
				cfg.HostKeyFingerprint = ""
			case "missing-file":
				cfg.KnownHostsFile = ""
			case "wrong-key-type":
				writeText(t, path, "example.org ssh-rsa "+testPublishHostKey+"\n")
			}
			if _, err := publishTargetFromConfig(cfg, PublishOptions{SiteDir: dir}); err == nil {
				t.Fatal("unsafe pin configuration was accepted")
			}
		})
	}
}

func TestPublishSSHNondefaultPortHomeExpansionAndCLIOverrides(t *testing.T) {
	dir, cfg := pinnedPublishFixture(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeText(t, filepath.Join(home, ".ssh", "id"), "placeholder")
	writeText(t, filepath.Join(dir, cfg.KnownHostsFile), "[example.org]:2222 ssh-ed25519 "+testPublishHostKey+"\n")
	target, err := publishTargetFromConfig(cfg, PublishOptions{SiteDir: dir, Port: 2222, PortSet: true,
		Mode: publishModeInPlace, ModeSet: true, IdentityFile: "~/.ssh/id", IdentityFileSet: true})
	if err != nil || target.Mode != publishModeInPlace || target.IdentityFile != filepath.Join(home, ".ssh", "id") {
		t.Fatalf("target = %#v, %v", target, err)
	}
}

func TestPublishInvalidModeOrPinFailsBeforeBuildAndConnection(t *testing.T) {
	for _, mode := range []string{publishModeInPlace, "invalid"} {
		t.Run(mode, func(t *testing.T) {
			dir := testSite(t)
			_, pin := pinnedPublishFixture(t)
			pin.KnownHostsFile = filepath.Join(dir, "known_hosts")
			pin.IdentityFile = ""
			pin.HostKeyFingerprint = "SHA256:wrong"
			writeText(t, pin.KnownHostsFile, "example.org ssh-ed25519 "+testPublishHostKey+"\n")
			cfg, err := LoadSiteConfig(dir)
			if err != nil {
				t.Fatal(err)
			}
			cfg.Publish = pin
			data, _ := json.Marshal(cfg)
			writeText(t, filepath.Join(dir, "smol.json"), string(data))
			runner := &recordingPublishRunner{}
			err = PublishSite(PublishOptions{SiteDir: dir, Mode: mode, ModeSet: true, commandRunner: runner,
				buildSite: func(BuildOptions) error { t.Fatal("invalid target reached build"); return nil }})
			if err == nil || len(runner.commands) != 0 {
				t.Fatalf("invalid publishing options reached transport: %v", err)
			}
		})
	}
}

func TestPublishNewCLIFlagsOverrideConfiguration(t *testing.T) {
	dir := testSite(t)
	writePublishConfig(t, dir, "wrong.example", "", 22, "/srv/site")
	writeText(t, filepath.Join(dir, "public", "index.html"), "signed")
	writeText(t, filepath.Join(dir, "identity"), "placeholder")
	writeText(t, filepath.Join(dir, "known_hosts"), "example.org ssh-ed25519 "+testPublishHostKey+"\n")
	var out strings.Builder
	err := commandPublish([]string{"--dry-run", "--no-build", "--mode", "in-place", "--host", "example.org",
		"--identity-file", "identity", "--known-hosts-file", "known_hosts",
		"--host-key-fingerprint", testPublishHostFingerprint, dir}, &out)
	if err != nil || !strings.Contains(out.String(), "StrictHostKeyChecking=yes") ||
		!strings.Contains(out.String(), "example.org:/srv/site/.smol-upload-") {
		t.Fatalf("new CLI publish options not applied: %v\n%s", err, out.String())
	}
	for _, flag := range []string{"--mode", "--identity-file", "--known-hosts-file", "--host-key-fingerprint"} {
		if err := commandPublish([]string{dir, flag, "value"}, &out); err == nil || !strings.Contains(err.Error(), "options must appear before") {
			t.Fatalf("flag after positional was accepted: %s, %v", flag, err)
		}
	}
	if err := commandPublish([]string{"--check-ssh", "--dry-run", dir}, &out); err == nil {
		t.Fatal("conflicting check and dry-run flags were accepted")
	}
}

func TestPublishIdentityPathMustExist(t *testing.T) {
	dir, cfg := pinnedPublishFixture(t)
	if err := os.Remove(filepath.Join(dir, cfg.IdentityFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := publishTargetFromConfig(cfg, PublishOptions{SiteDir: dir}); err == nil {
		t.Fatal("missing SSH identity accepted")
	}
}

func TestPublishFreezesValidatedSSHPinForTransport(t *testing.T) {
	site := testSite(t)
	dir, pin := pinnedPublishFixture(t)
	pin.IdentityFile = filepath.Join(dir, pin.IdentityFile)
	pin.KnownHostsFile = filepath.Join(dir, pin.KnownHostsFile)
	pin.Mode = publishModeInPlace
	pin.Path = t.TempDir()
	cfg, err := LoadSiteConfig(site)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Publish = pin
	data, _ := json.Marshal(cfg)
	writeText(t, filepath.Join(site, "smol.json"), string(data))
	writeText(t, filepath.Join(site, "public", "index.html"), "transport check only")
	original := readText(t, pin.KnownHostsFile)
	frozen := ""
	runner := &recordingPublishRunner{onRun: func(command publishCommand) error {
		writeText(t, pin.KnownHostsFile, "replaced after fingerprint validation")
		for _, arg := range command.Args {
			if strings.HasPrefix(arg, `UserKnownHostsFile="`) {
				frozen = strings.TrimSuffix(strings.TrimPrefix(arg, `UserKnownHostsFile="`), `"`)
				if frozen == pin.KnownHostsFile || readText(t, frozen) != original {
					t.Fatal("SSH rereads mutable original pin file")
				}
			}
		}
		if frozen == "" {
			t.Fatal("transport has no frozen pin")
		}
		return nil
	}}
	if err := PublishSite(PublishOptions{SiteDir: site, CheckSSH: true, commandRunner: runner}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(frozen); !os.IsNotExist(err) {
		t.Fatalf("temporary pin leaked: %v", err)
	}
}
