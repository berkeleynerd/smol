package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var fullFingerprintRE = regexp.MustCompile(`^(?:[A-Fa-f0-9]{40}|[A-Fa-f0-9]{64})$`)

type publishInventory struct {
	hashes      map[string]string
	directories []string
}

// Upload a private snapshot, not a directory an editor/build can change after
// verification. Symlinks and special files cannot cross this boundary.
func snapshotOutput(source string) (string, error) {
	dir, err := os.MkdirTemp("", "smol-publish-*")
	if err != nil {
		return "", err
	}
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if strings.ContainsAny(rel, "\r\n") {
			return fmt.Errorf("newlines in publication paths are forbidden")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("publication cannot contain symlinks: %s", rel)
		}
		target := filepath.Join(dir, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("publication cannot contain special files: %s", rel)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func verifyPublishOutput(dir, attestTool, fingerprint string, gemini bool) (publishInventory, error) {
	inventory := publishInventory{hashes: map[string]string{}}
	pages := 0
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if rel != "." {
				inventory.directories = append(inventory.directories, filepath.ToSlash(rel))
			}
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".html" || ext == ".xhtml" || ext == ".htm" {
			if gemini {
				return fmt.Errorf("Gemini publication contains HTML: %s", rel)
			}
			if err := CheckHTML(string(data)); err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
			if err := ValidateXHTMLDocument(string(data)); err != nil {
				return fmt.Errorf("%s: %w", rel, err)
			}
			command := exec.Command(attestTool, "verify", "--trusted", fingerprint, path)
			if output, err := command.CombinedOutput(); err != nil {
				return fmt.Errorf("refusing to publish %s: signature verification failed: %w: %s", rel, err, strings.TrimSpace(string(output)))
			}
			pages++
		} else if ext == ".gz" {
			original := strings.TrimSuffix(path, filepath.Ext(path))
			if !strings.HasSuffix(strings.ToLower(original), ".html") && !strings.HasSuffix(strings.ToLower(original), ".xhtml") && !strings.HasSuffix(strings.ToLower(original), ".htm") {
				return fmt.Errorf("unsupported compressed publication file: %s", rel)
			}
			plain, err := os.ReadFile(original)
			if err != nil {
				return fmt.Errorf("gzip has no matching page: %s", rel)
			}
			reader, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				return err
			}
			decompressed, readErr := io.ReadAll(io.LimitReader(reader, int64(len(plain))+1))
			reader.Close()
			if readErr != nil || !bytes.Equal(decompressed, plain) {
				return fmt.Errorf("gzip differs from signed page: %s", rel)
			}
		} else if gemini && ext == ".gmi" {
			if err := ValidateGemtext(string(data)); err != nil {
				return err
			}
			pages++
		}
		inventory.hashes[filepath.ToSlash(rel)] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	})
	if err != nil {
		return publishInventory{}, err
	}
	if pages == 0 {
		return publishInventory{}, fmt.Errorf("publication contains no pages")
	}
	return inventory, nil
}

// Verify the uploaded snapshot before replacing the live directory. This needs
// only a SHA-256 utility on the static host, not GPG or any private/public key.
func remoteVerifyScript(stage string, inventory publishInventory) string {
	lines := []string{"set -eu", "stage=" + posixSingleQuote(stage),
		`[ -d "$stage" ] && [ ! -L "$stage" ] || { echo 'Invalid upload directory' >&2; exit 1; }`,
		`hash_file() {`,
		`  if command -v sha256sum >/dev/null 2>&1; then sha256sum < "$1" | awk '{print $1}'`,
		`  elif command -v sha256 >/dev/null 2>&1; then sha256 -q "$1"`,
		`  elif command -v shasum >/dev/null 2>&1; then shasum -a 256 < "$1" | awk '{print $1}'`,
		`  else echo 'SHA-256 utility required' >&2; return 1; fi`, `}`}
	paths := make([]string, 0, len(inventory.hashes))
	for rel := range inventory.hashes {
		paths = append(paths, rel)
	}
	sort.Strings(inventory.directories)
	for _, dir := range inventory.directories {
		lines = append(lines, `dir="$stage"/`+posixSingleQuote(dir),
			`[ -d "$dir" ] && [ ! -L "$dir" ] || { echo 'Uploaded directory differs from verified snapshot' >&2; exit 1; }`)
	}
	sort.Strings(paths)
	for _, path := range paths {
		lines = append(lines, `file="$stage"/`+posixSingleQuote(path),
			`[ -f "$file" ] && [ ! -L "$file" ] || { echo 'Uploaded file differs from verified snapshot' >&2; exit 1; }`,
			`[ "$(hash_file "$file")" = `+posixSingleQuote(inventory.hashes[path])+` ] || { echo 'Uploaded bytes differ from verified snapshot' >&2; exit 1; }`)
	}
	// Every expected file and directory (including empty ones) exists with the right type.
	// Count all entries without serializing filenames: extras containing spaces,
	// newlines, or shell metacharacters must not escape the equality check.
	lines = append(lines, `entries=$(find "$stage" -exec sh -c 'for entry; do printf x; done' sh {} +)`,
		fmt.Sprintf(`[ "${#entries}" -eq %d ] || { echo 'Upload inventory differs from verified snapshot' >&2; exit 1; }`, 1+len(inventory.directories)+len(paths)))
	return strings.Join(lines, "\n") + "\n"
}
