package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPublishIPv6DestinationsAndPinnedHost(t *testing.T) {
	for _, host := range []string{"2001:db8::1", "[2001:db8::1]"} {
		for _, port := range []int{22, 2222} {
			t.Run(host+"/"+strconv.Itoa(port), func(t *testing.T) {
				dir, cfg := pinnedPublishFixture(t)
				cfg.Host, cfg.Port = host, port
				knownHost := "2001:db8::1"
				if port != 22 {
					knownHost = "[2001:db8::1]:" + strconv.Itoa(port)
				}
				writeText(t, filepath.Join(dir, cfg.KnownHostsFile), knownHost+" ssh-ed25519 "+testPublishHostKey+"\n")
				target, err := publishTargetFromConfig(cfg, PublishOptions{SiteDir: dir})
				if err != nil {
					t.Fatal(err)
				}
				if target.Host != "2001:db8::1" || sshDestination(target) != "deploy@2001:db8::1" {
					t.Fatalf("unnormalized SSH host: %#v", target)
				}
				args := scpArgs(target, "/tmp/output", "/srv/stage")
				if args[len(args)-1] != "deploy@[2001:db8::1]:/srv/stage" {
					t.Fatalf("SCP destination: %v", args)
				}
				if !strings.Contains(strings.Join(args, "\n"), "StrictHostKeyChecking=yes") || !strings.Contains(strings.Join(args, "\n"), "-P\n"+strconv.Itoa(port)) {
					t.Fatalf("missing SSH policy/port: %v", args)
				}
			})
		}
	}
	for _, host := range []string{"[example.org]", "[::1]:2222", "[::1", "::broken"} {
		if _, err := publishTargetFromConfig(PublishConfig{Host: host, Path: "/srv/site"}, PublishOptions{}); err == nil {
			t.Fatalf("invalid host accepted: %s", host)
		}
	}
}

func TestSCPParsesIPv6DestinationWithoutNetwork(t *testing.T) {
	scp, err := exec.LookPath("scp")
	if err != nil {
		t.Skip("requires scp")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "ssh-arguments")
	transport := filepath.Join(dir, "ssh-recorder")
	writeText(t, transport, "#!/bin/sh\nprintf '%s\\n' \"$@\" > "+posixSingleQuote(log)+"\nexit 1\n")
	if err := os.Chmod(transport, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "source")
	writeText(t, file, "never transmitted")
	for _, user := range []string{"", "deploy"} {
		destination := publishRemoteSpec(publishTarget{Host: "2001:db8::1", User: user}, "/srv/site")
		command := exec.Command(scp, "-S", transport, "-P", "2222", file, destination)
		if output, err := command.CombinedOutput(); err == nil {
			t.Fatalf("recording transport unexpectedly succeeded: %s", output)
		}
		args := strings.Split(readText(t, log), "\n")
		found := false
		for _, arg := range args {
			if arg == "2001:db8::1" {
				found = true
			}
		}
		if !found {
			t.Fatalf("SCP parsed the wrong host: %q", args)
		}
	}
}
