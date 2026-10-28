package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func resolvePublishSSH(target *publishTarget, siteDir string) error {
	var err error
	if target.IdentityFile != "" {
		target.IdentityFile, err = publishLocalPath(siteDir, target.IdentityFile)
		if err != nil {
			return fmt.Errorf("publish identity_file: %w", err)
		}
		info, err := os.Stat(target.IdentityFile)
		if err != nil {
			return fmt.Errorf("publish identity_file: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("publish identity_file must be a regular file")
		}
	}
	if (target.KnownHostsFile == "") != (target.HostKeyFingerprint == "") {
		return fmt.Errorf("publish known_hosts_file and host_key_fingerprint must be supplied together")
	}
	if target.KnownHostsFile == "" {
		return nil
	}
	target.KnownHostsFile, err = publishLocalPath(siteDir, target.KnownHostsFile)
	if err != nil {
		return fmt.Errorf("publish known_hosts_file: %w", err)
	}
	data, err := os.ReadFile(target.KnownHostsFile)
	if err != nil {
		return fmt.Errorf("publish known_hosts_file: %w", err)
	}
	var entries [][]string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			entries = append(entries, strings.Fields(line))
		}
	}
	host := target.Host
	if target.Port != 22 {
		host = "[" + host + "]:" + strconv.Itoa(target.Port)
	}
	if len(entries) != 1 || len(entries[0]) < 3 || entries[0][0] != host {
		return fmt.Errorf("publish known_hosts_file must contain exactly one plain host key for %s", host)
	}
	entry := entries[0]
	key, err := base64.StdEncoding.DecodeString(entry[2])
	if err != nil || len(key) < 4 {
		return fmt.Errorf("publish known_hosts_file contains an invalid public key")
	}
	algorithmLength := uint64(binary.BigEndian.Uint32(key[:4]))
	if algorithmLength > uint64(len(key)-4) || string(key[4:4+algorithmLength]) != entry[1] {
		return fmt.Errorf("publish known_hosts_file key type does not match its public key")
	}
	switch entry[1] {
	case "ssh-ed25519", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521":
		target.hostKeyAlgorithms = entry[1]
	case "ssh-rsa":
		target.hostKeyAlgorithms = "rsa-sha2-512,rsa-sha2-256"
	default:
		return fmt.Errorf("unsupported pinned SSH host key type: %s", entry[1])
	}
	sum := sha256.Sum256(key)
	fingerprint := "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
	if fingerprint != target.HostKeyFingerprint {
		return fmt.Errorf("publish SSH host-key fingerprint mismatch: got %s", fingerprint)
	}
	target.knownHostsData = append([]byte(nil), data...)
	return nil
}

func publishLocalPath(siteDir, value string) (string, error) {
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("path must not contain NUL or newline")
	}
	if strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		value = filepath.Join(home, value[2:])
	} else if strings.HasPrefix(value, "~") {
		return "", fmt.Errorf("only ~/ home expansion is supported")
	}
	if !filepath.IsAbs(value) {
		value = filepath.Join(siteDir, value)
	}
	return filepath.Abs(value)
}

func publishSSHOptions(target publishTarget) []string {
	var args []string
	if target.KnownHostsFile != "" {
		// Ignore ambient settings that could change which host key is accepted.
		args = append(args, "-F", "/dev/null")
	}
	if target.IdentityFile != "" {
		args = append(args, "-i", target.IdentityFile)
	}
	if target.IdentityFile != "" || target.KnownHostsFile != "" {
		args = append(args, "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes",
			"-o", "PasswordAuthentication=no", "-o", "KbdInteractiveAuthentication=no",
			"-o", "ConnectTimeout=15")
	}
	if target.KnownHostsFile != "" {
		// OpenSSH token-expands percent signs in UserKnownHostsFile paths.
		knownHosts := strings.ReplaceAll(target.KnownHostsFile, "%", "%%")
		knownHosts = strings.ReplaceAll(strings.ReplaceAll(knownHosts, `\`, `\\`), `"`, `\"`)
		args = append(args, "-o", "StrictHostKeyChecking=yes",
			"-o", `UserKnownHostsFile="`+knownHosts+`"`, "-o", "GlobalKnownHostsFile=/dev/null",
			"-o", "UpdateHostKeys=no", "-o", "HostKeyAlgorithms="+target.hostKeyAlgorithms)
	}
	return args
}

// Freeze the already-validated host key so mutable site files cannot change the
// accepted server identity while building or transferring the output snapshot.
func freezePublishSSH(target *publishTarget) (func(), error) {
	dir, err := os.MkdirTemp("", "smol-ssh-pin-*")
	if err != nil {
		return nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "known_hosts")
	if err := os.WriteFile(path, target.knownHostsData, 0o600); err != nil {
		cleanup()
		return nil, err
	}
	target.KnownHostsFile = path
	return cleanup, nil
}
