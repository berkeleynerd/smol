package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type AttestResolverOptions struct {
	ExplicitPath  string
	ExecutableDir string
	TempDir       string
	Stdout        io.Writer
	LookPath      func(string) (string, error)
}

type AttestResolutionError struct {
	Searched []string
	Notes    []string
}

func (e AttestResolutionError) Error() string {
	msg := "attest signer not found"
	if len(e.Searched) > 0 {
		msg += "; searched: " + strings.Join(e.Searched, ", ")
	}
	if len(e.Notes) > 0 {
		msg += "; notes: " + strings.Join(e.Notes, "; ")
	}
	return msg
}

func ResolveAttestTool(opts AttestResolverOptions) (string, error) {
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.LookPath == nil {
		opts.LookPath = exec.LookPath
	}
	var searched []string
	var notes []string

	if opts.ExplicitPath != "" {
		searched = append(searched, "explicit:"+opts.ExplicitPath)
		if path, err := executablePath(opts.ExplicitPath, opts.LookPath); err == nil {
			return path, nil
		} else {
			return "", AttestResolutionError{Searched: searched, Notes: []string{err.Error()}}
		}
	}

	for _, name := range []string{"attest", "attested-html"} {
		searched = append(searched, "PATH:"+name)
		if path, err := opts.LookPath(name); err == nil {
			return path, nil
		}
	}

	executableDir := opts.ExecutableDir
	if executableDir == "" {
		dir, err := defaultExecutableDir()
		if err != nil {
			notes = append(notes, err.Error())
		}
		executableDir = dir
	}
	if executableDir != "" {
		for _, siblingName := range []string{"attest", "attested-html"} {
			siblingDir := filepath.Clean(filepath.Join(executableDir, "..", siblingName))
			for _, binaryName := range []string{"attest", "attested-html"} {
				candidate := filepath.Join(siblingDir, binaryName)
				searched = append(searched, candidate)
				if isExecutableFile(candidate) {
					return candidate, nil
				}
			}
			if isGoMainProject(siblingDir) {
				searched = append(searched, siblingDir+" (go build)")
				if err := ensureStdlibOnlyGoMod(filepath.Join(siblingDir, "go.mod")); err != nil {
					notes = append(notes, err.Error())
					continue
				}
				if opts.TempDir == "" {
					notes = append(notes, "cannot build sibling signer without a temp directory: "+siblingDir)
					continue
				}
				out := filepath.Join(opts.TempDir, "smol-attest-signer-"+siblingName)
				fmt.Fprintf(opts.Stdout, "building signer from %s\n", siblingDir)
				if err := buildGoSigner(siblingDir, out); err != nil {
					notes = append(notes, err.Error())
					continue
				}
				return out, nil
			}
		}
	}

	return "", AttestResolutionError{Searched: searched, Notes: notes}
}

func executablePath(path string, lookPath func(string) (string, error)) (string, error) {
	if strings.ContainsRune(path, filepath.Separator) {
		if isExecutableFile(path) {
			return path, nil
		}
		return "", fmt.Errorf("explicit signer is not executable: %s", path)
	}
	resolved, err := lookPath(path)
	if err != nil {
		return "", fmt.Errorf("explicit signer not found on PATH: %s", path)
	}
	return resolved, nil
}

func defaultExecutableDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve smol executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("resolve smol executable symlink: %w", err)
	}
	return filepath.Dir(resolved), nil
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Mode().Perm()&0o111 != 0
}

func isGoMainProject(dir string) bool {
	for _, name := range []string{"go.mod", "main.go"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil || info.IsDir() {
			return false
		}
	}
	return true
}

func ensureStdlibOnlyGoMod(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	inRequireBlock := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if inRequireBlock {
			if trimmed == ")" {
				inRequireBlock = false
				continue
			}
			return fmt.Errorf("sibling signer go.mod has require dependency: %s", path)
		}
		if trimmed == "require (" {
			inRequireBlock = true
			continue
		}
		if strings.HasPrefix(trimmed, "require ") {
			return fmt.Errorf("sibling signer go.mod has require dependency: %s", path)
		}
	}
	return nil
}

func buildGoSigner(projectDir, output string) error {
	cmd := exec.Command("go", "build", "-o", output, ".")
	cmd.Dir = projectDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = stdout.String()
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("build sibling signer failed: %s", trimForError(msg))
	}
	return nil
}

func SignHTML(attestTool, key, input, output string) error {
	cmd := exec.Command(attestTool, "sign", "-k", key, "-o", output, "--force", input)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return nil
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return fmt.Errorf("attest signer could not be started: %w", err)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		msg := stderr.String()
		if msg == "" {
			msg = stdout.String()
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("attest signer failed: %s", trimForError(msg))
	}
	return fmt.Errorf("attest signer could not be started: %w", err)
}

type gpgSecretKey struct {
	Fingerprint string
	UID         string
}

var listSecretKeys = listGPGSecretKeys

func listGPGSecretKeys() ([]gpgSecretKey, error) {
	gpg, err := exec.LookPath("gpg")
	if err != nil {
		return nil, fmt.Errorf("gpg not found on PATH")
	}
	cmd := exec.Command(gpg, "--list-secret-keys", "--with-colons")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = stdout.String()
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("list GPG secret keys: %s", trimForError(msg))
	}
	return parseGPGSecretKeys(stdout.String()), nil
}

func parseGPGSecretKeys(output string) []gpgSecretKey {
	keys := []gpgSecretKey{}
	wantPrimaryFingerprint := false
	current := -1
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "sec":
			wantPrimaryFingerprint = true
			current = -1
		case "fpr":
			if !wantPrimaryFingerprint || len(fields) <= 9 || fields[9] == "" {
				continue
			}
			fingerprint := fields[9]
			keys = append(keys, gpgSecretKey{Fingerprint: fingerprint})
			current = len(keys) - 1
			wantPrimaryFingerprint = false
		case "uid":
			if current >= 0 && keys[current].UID == "" && len(fields) > 9 {
				keys[current].UID = fields[9]
			}
		case "ssb", "sub", "pub":
			wantPrimaryFingerprint = false
			current = -1
		}
	}
	return keys
}

func trimForError(value string) string {
	value = bytes.NewBufferString(value).String()
	for len(value) > 0 {
		last := value[len(value)-1]
		if last != '\n' && last != '\r' && last != '\t' && last != ' ' {
			break
		}
		value = value[:len(value)-1]
	}
	return value
}
