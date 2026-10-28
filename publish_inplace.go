package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
)

type publishTree struct {
	dirs  []string
	files []string
}

func inspectPublishTree(outDir string) (publishTree, error) {
	var tree publishTree
	err := filepath.WalkDir(outDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("in-place publishing does not follow symlinks: %s", path)
		}
		rel, err := filepath.Rel(outDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if strings.ContainsAny(rel, "\x00\r\n") || strings.HasPrefix(strings.Split(rel, "/")[0], ".smol-upload-") {
			return fmt.Errorf("unsupported in-place output path: %s", rel)
		}
		if entry.IsDir() {
			tree.dirs = append(tree.dirs, rel)
		} else if entry.Type().IsRegular() {
			tree.files = append(tree.files, rel)
		} else {
			return fmt.Errorf("in-place publishing requires regular files: %s", path)
		}
		return nil
	})
	return tree, err
}

func makePublishPlan(outDir string, target publishTarget, inspect bool) (publishPlan, error) {
	if target.Mode != publishModeInPlace {
		return publishPlan{commands: publishCommands(outDir, target), stage: remoteStagePath(target.Path)}, nil
	}
	var tree publishTree
	var err error
	if inspect {
		tree, err = inspectPublishTree(outDir)
		if err != nil {
			return publishPlan{}, err
		}
		if len(tree.files) == 0 {
			return publishPlan{}, fmt.Errorf("in-place publishing requires at least one output file")
		}
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return publishPlan{}, err
	}
	stage := pathpkg.Join(target.Path, ".smol-upload-"+hex.EncodeToString(nonce[:]))
	prepare := inPlaceDestinationChecks(target.Path, tree) +
		"stage=" + posixSingleQuote(stage) + "\nmkdir -m 700 \"$stage\"\n"
	commit := inPlaceDestinationChecks(target.Path, tree) +
		"stage=" + posixSingleQuote(stage) + "\n" +
		"test -d \"$stage\" && test ! -L \"$stage\"\n" +
		"trap 'rm -rf \"$stage\"' EXIT\n"
	// SCP preserves source directory modes; make our staging directories
	// writable even when publishing a read-only build artifact.
	commit += "chmod u+rwx \"$stage\"\n"
	for _, dir := range tree.dirs {
		commit += "chmod u+rwx " + posixSingleQuote(pathpkg.Join(stage, dir)) + "\n"
	}
	for _, dir := range tree.dirs {
		commit += "mkdir -p " + posixSingleQuote(pathpkg.Join(target.Path, dir)) + "\n"
	}
	for _, file := range tree.files {
		commit += "mv -f " + posixSingleQuote(pathpkg.Join(stage, file)) + " " +
			posixSingleQuote(pathpkg.Join(target.Path, file)) + "\n"
	}
	cleanup := publishCommand{Name: "ssh", Args: sshArgs(target),
		Stdin: "set -eu\ntarget=" + posixSingleQuote(target.Path) + "\n[ -d \"$target\" ] && [ ! -L \"$target\" ] || exit 0\nrm -rf " + posixSingleQuote(stage) + "\n"}
	return publishPlan{commands: []publishCommand{
		{Name: "ssh", Args: sshArgs(target), Stdin: prepare},
		{Name: "scp", Args: scpArgs(target, outDir, stage)},
		{Name: "ssh", Args: sshArgs(target), Stdin: commit},
	}, cleanup: &cleanup, stage: stage}, nil
}

func inPlaceDestinationChecks(target string, tree publishTree) string {
	lines := []string{"set -eu", "target=" + posixSingleQuote(target),
		`if [ -L "$target" ] || [ ! -d "$target" ] || [ ! -w "$target" ] || [ ! -x "$target" ]; then`,
		`  echo 'publish target must be an existing writable directory, not a symlink' >&2; exit 1`, "fi"}
	for _, dir := range tree.dirs {
		lines = append(lines, "dest="+posixSingleQuote(pathpkg.Join(target, dir)),
			`if [ -L "$dest" ] || { [ -e "$dest" ] && [ ! -d "$dest" ]; }; then`,
			`  echo "unsafe publish directory: $dest" >&2; exit 1`, "fi",
			`if [ -d "$dest" ] && { [ ! -w "$dest" ] || [ ! -x "$dest" ]; }; then`,
			`  echo "publish directory is not writable: $dest" >&2; exit 1`, "fi")
	}
	for _, file := range tree.files {
		lines = append(lines, "dest="+posixSingleQuote(pathpkg.Join(target, file)),
			`if [ -L "$dest" ] || { [ -e "$dest" ] && [ ! -f "$dest" ]; }; then`,
			`  echo "unsafe publish file: $dest" >&2; exit 1`, "fi")
	}
	return strings.Join(lines, "\n") + "\n"
}

func publishPreflightCommand(outDir string, target publishTarget) (publishCommand, error) {
	var script string
	if target.Mode == publishModeInPlace {
		tree, err := inspectPublishTree(outDir)
		if err != nil {
			return publishCommand{}, err
		}
		script = inPlaceDestinationChecks(target.Path, tree)
	} else {
		parent := posixSingleQuote(pathpkg.Dir(target.Path))
		script = "set -eu\nparent=" + parent + "\ntest -d \"$parent\" && test -w \"$parent\" && test -x \"$parent\"\n"
	}
	return publishCommand{Name: "ssh", Args: sshArgs(target), Stdin: script}, nil
}
