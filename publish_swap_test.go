package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func swapPlanFixture(t *testing.T, target, body string) publishPlan {
	t.Helper()
	plan, err := makePublishPlan(t.TempDir(), publishTarget{Host: "example.invalid", Path: target, Mode: publishModeSwap}, true)
	if err != nil {
		t.Fatal(err)
	}
	plan.commands[2].Stdin = remoteVerifyScript(plan.stage, publishInventory{hashes: map[string]string{"index.html": fmtHash([]byte(body))}}) + plan.commands[2].Stdin
	return plan
}

func runSwapScript(t *testing.T, script string, environment ...string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "sh", "-s")
	command.Stdin = strings.NewReader(script)
	command.Env = append(os.Environ(), environment...)
	return command.CombinedOutput()
}

func startPausedSwap(t *testing.T, script, executable, pauseBody string) (release func(), wait func() ([]byte, error)) {
	t.Helper()
	dir := t.TempDir()
	ready, proceed := filepath.Join(dir, "ready"), filepath.Join(dir, "proceed")
	shim := filepath.Join(dir, executable)
	writeText(t, shim, "#!/bin/sh\n"+pauseBody+"\n")
	if err := os.Chmod(shim, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	command := exec.CommandContext(ctx, "sh", "-s")
	command.Stdin = strings.NewReader(script)
	command.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "SWAP_READY="+ready, "SWAP_PROCEED="+proceed)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	release = func() { _ = os.WriteFile(proceed, nil, 0600) }
	t.Cleanup(func() { release(); cancel() })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("script ended before barrier: %v %s", err, output.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("script did not reach barrier")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return release, func() ([]byte, error) { err := <-done; cancel(); return output.Bytes(), err }
}

func TestSwapPreparationCannotReplaceAnotherVerifiedUpload(t *testing.T) {
	target := filepath.Join(t.TempDir(), "live")
	writeText(t, filepath.Join(target, "index.html"), "old live")
	first := swapPlanFixture(t, target, "first checked upload")
	second := swapPlanFixture(t, target, "second complete upload")
	if first.stage == second.stage {
		t.Fatal("publications share staging")
	}
	if output, err := runSwapScript(t, first.commands[0].Stdin); err != nil {
		t.Fatalf("prepare: %v %s", err, output)
	}
	writeText(t, filepath.Join(first.stage, "index.html"), "first checked upload")
	find, err := exec.LookPath("find")
	if err != nil {
		t.Fatal(err)
	}
	// Pause after the real inventory and hashes were computed, exactly where
	// another prepare previously replaced the shared verified directory.
	release, wait := startPausedSwap(t, first.commands[2].Stdin, "find",
		posixSingleQuote(find)+" \"$@\" || exit $?\n: > \"$SWAP_READY\"\nwhile [ ! -e \"$SWAP_PROCEED\" ]; do sleep 0.01; done")
	if output, err := runSwapScript(t, second.commands[0].Stdin); err != nil {
		t.Fatalf("second prepare: %v %s", err, output)
	}
	writeText(t, filepath.Join(second.stage, "index.html"), "second unfinished upload")
	if readText(t, filepath.Join(first.stage, "index.html")) != "first checked upload" {
		t.Fatal("second preparation changed verified bytes")
	}
	release()
	if output, err := wait(); err != nil {
		t.Fatalf("first commit: %v %s", err, output)
	}
	if readText(t, filepath.Join(target, "index.html")) != "first checked upload" {
		t.Fatal("first commit activated another upload")
	}
	if output, err := runSwapScript(t, second.commands[2].Stdin); err == nil {
		t.Fatalf("unfinished second upload accepted: %s", output)
	}
	if readText(t, filepath.Join(target, "index.html")) != "first checked upload" {
		t.Fatal("failed second verification changed live site")
	}
}

func TestSwapSerializesActivationAndRollsBackWhileLocked(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rollback"}[fail], func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "site's live")
			writeText(t, filepath.Join(target, "index.html"), "old live")
			first, second := swapPlanFixture(t, target, "first"), swapPlanFixture(t, target, "second")
			for _, row := range []struct {
				plan publishPlan
				body string
			}{{first, "first"}, {second, "second"}} {
				if output, err := runSwapScript(t, row.plan.commands[0].Stdin); err != nil {
					t.Fatalf("prepare: %v %s", err, output)
				}
				writeText(t, filepath.Join(row.plan.stage, "index.html"), row.body)
			}
			mv, err := exec.LookPath("mv")
			if err != nil {
				t.Fatal(err)
			}
			body := "if [ \"$1\" = " + posixSingleQuote(first.stage) + " ]; then\n: > \"$SWAP_READY\"\nwhile [ ! -e \"$SWAP_PROCEED\" ]; do sleep 0.01; done\n"
			if fail {
				body += "exit 19\n"
			}
			body += "fi\nexec " + posixSingleQuote(mv) + " \"$@\""
			// First publication holds the lock after moving live to its backup.
			release, wait := startPausedSwap(t, first.commands[2].Stdin, "mv", body)
			if output, err := runSwapScript(t, second.commands[2].Stdin); err == nil || !strings.Contains(string(output), "Another publication holds") {
				t.Fatalf("overlapping activation was not refused: %v %s", err, output)
			}
			if _, err := os.Stat(remoteLockPath(target)); err != nil {
				t.Fatal("second invocation removed first lock:", err)
			}
			release()
			output, err := wait()
			want := "first"
			if fail {
				want = "old live"
				if err == nil {
					t.Fatal("failed activation returned success")
				}
			} else if err != nil {
				t.Fatalf("activation failed: %v %s", err, output)
			}
			if readText(t, filepath.Join(target, "index.html")) != want {
				t.Fatal("wrong live bytes after activation/rollback")
			}
			if _, err := os.Stat(remoteLockPath(target)); !os.IsNotExist(err) {
				t.Fatalf("lock leaked: %v", err)
			}
			backups, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".*.smol-backup-*"))
			if len(backups) != 0 {
				t.Fatalf("completed operation leaked backups: %v", backups)
			}
			// Retrying after the first operation finishes can activate its own upload.
			if output, err := runSwapScript(t, second.commands[2].Stdin); err != nil {
				t.Fatalf("retry failed: %v %s", err, output)
			}
			if readText(t, filepath.Join(target, "index.html")) != "second" {
				t.Fatal("retry activated wrong upload")
			}
		})
	}
}

func TestSwapPrepareRefusesExistingStage(t *testing.T) {
	target := filepath.Join(t.TempDir(), "live")
	plan := swapPlanFixture(t, target, "new")
	writeText(t, filepath.Join(plan.stage, "index.html"), "existing upload")
	if output, err := runSwapScript(t, plan.commands[0].Stdin); err == nil {
		t.Fatalf("existing stage reused: %s", output)
	}
	if readText(t, filepath.Join(plan.stage, "index.html")) != "existing upload" {
		t.Fatal("existing stage removed")
	}
}

func TestSwapFailedTransferCleansOnlyItsStage(t *testing.T) {
	site := testSite(t)
	target := filepath.Join(t.TempDir(), "live")
	writeText(t, filepath.Join(target, "index.html"), "old live")
	writePublishConfig(t, site, "example.org", "", 22, target)
	tool := signInPlaceOutput(t, site)
	other := swapPlanFixture(t, target, "other")
	writeText(t, filepath.Join(other.stage, "index.html"), "other publication")
	runner := localPublishRunner(t, "")
	run := runner.onRun
	runner.onRun = func(command publishCommand) error {
		if command.Name == "scp" {
			return context.DeadlineExceeded
		}
		return run(command)
	}
	if err := PublishSite(PublishOptions{SiteDir: site, NoBuild: true, AttestPath: tool, commandRunner: runner}); err == nil {
		t.Fatal("transfer failure accepted")
	}
	stages, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".live.smol-upload-*"))
	if len(stages) != 1 || stages[0] != other.stage {
		t.Fatalf("cleanup removed another upload or leaked own stage: %v", stages)
	}
	if readText(t, filepath.Join(target, "index.html")) != "old live" {
		t.Fatal("failed upload changed live site")
	}
}

func TestSwapPrepareCollisionDoesNotCleanUnownedStage(t *testing.T) {
	site := testSite(t)
	target := filepath.Join(t.TempDir(), "live")
	writeText(t, filepath.Join(target, "index.html"), "old live")
	writePublishConfig(t, site, "example.org", "", 22, target)
	tool := signInPlaceOutput(t, site)
	runner := localPublishRunner(t, "")
	run := runner.onRun
	runner.onRun = func(command publishCommand) error {
		if len(runner.commands) == 1 {
			// Create the collision immediately before the real prepare mkdir.
			command.Stdin = strings.Replace(command.Stdin, "mkdir -m 700 \"$stage\"",
				"mkdir -m 700 \"$stage\"\nprintf existing > \"$stage/index.html\"\nmkdir -m 700 \"$stage\"", 1)
		}
		return run(command)
	}
	if err := PublishSite(PublishOptions{SiteDir: site, NoBuild: true, AttestPath: tool, commandRunner: runner}); err == nil {
		t.Fatal("stage collision accepted")
	}
	stages, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".live.smol-upload-*"))
	if len(runner.commands) != 1 || len(stages) != 1 || readText(t, filepath.Join(stages[0], "index.html")) != "existing" {
		t.Fatalf("failed preparation cleaned a stage it did not create: commands=%d stages=%v", len(runner.commands), stages)
	}
	if readText(t, filepath.Join(target, "index.html")) != "old live" {
		t.Fatal("preparation failure changed live site")
	}
}

func TestSwapRollbackFailureRetainsBackup(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "live")
	writeText(t, filepath.Join(target, "index.html"), "old live")
	plan := swapPlanFixture(t, target, "new")
	if output, err := runSwapScript(t, plan.commands[0].Stdin); err != nil {
		t.Fatalf("prepare: %v %s", err, output)
	}
	writeText(t, filepath.Join(plan.stage, "index.html"), "new")
	realMV, err := exec.LookPath("mv")
	if err != nil {
		t.Fatal(err)
	}
	shimDir := t.TempDir()
	shim := filepath.Join(shimDir, "mv")
	// Permit taking the backup, fail both activation and restoration.
	writeText(t, shim, "#!/bin/sh\nif [ \"$2\" = "+posixSingleQuote(target)+" ]; then exit 21; fi\nexec "+posixSingleQuote(realMV)+" \"$@\"\n")
	if err := os.Chmod(shim, 0700); err != nil {
		t.Fatal(err)
	}
	output, err := runSwapScript(t, plan.commands[2].Stdin, "PATH="+shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if err == nil || !strings.Contains(string(output), "Rollback failed; previous publication retained") {
		t.Fatalf("rollback failure hidden: %v %s", err, output)
	}
	backups, _ := filepath.Glob(filepath.Join(dir, ".live.smol-backup-*"))
	if len(backups) != 1 || readText(t, filepath.Join(backups[0], "index.html")) != "old live" {
		t.Fatalf("original backup lost: %v", backups)
	}
	if _, err := os.Stat(remoteLockPath(target)); !os.IsNotExist(err) {
		t.Fatalf("lock leaked: %v", err)
	}
}
