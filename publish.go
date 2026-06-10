package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
)

type PublishOptions struct {
	SiteDir       string
	OutDir        string
	NoBuild       bool
	SignKey       string
	SignKeySource bool
	Unsigned      bool
	DryRun        bool
	Host          string
	User          string
	Port          int
	Path          string
	HostSet       bool
	UserSet       bool
	PortSet       bool
	PathSet       bool
	Stdout        io.Writer

	commandRunner publishCommandRunner
	buildSite     func(BuildOptions) error
}

type publishTarget struct {
	Method string
	Host   string
	User   string
	Port   int
	Path   string
}

type publishCommand struct {
	Name  string
	Args  []string
	Stdin string
}

type publishCommandRunner interface {
	Run(publishCommand) error
}

type execPublishCommandRunner struct{}

func PublishSite(opts PublishOptions) error {
	if opts.SiteDir == "" {
		opts.SiteDir = "."
	}
	if opts.OutDir == "" {
		opts.OutDir = "public"
	}
	if opts.Stdout == nil {
		opts.Stdout = io.Discard
	}
	if opts.NoBuild && opts.SignKey != "" && !opts.Unsigned {
		return fmt.Errorf("--no-build skips the build phase, so signing cannot run")
	}
	siteDir, err := filepath.Abs(opts.SiteDir)
	if err != nil {
		return err
	}
	siteCfg, err := LoadSiteConfig(siteDir)
	if err != nil {
		return err
	}
	target, err := publishTargetFromConfig(siteCfg.Publish, opts)
	if err != nil {
		return err
	}
	if !opts.Unsigned && opts.SignKey != "" && siteCfg.OutputMode == outputModeFlatGemini {
		return fmt.Errorf("signing is not supported for flat-gemini-v1")
	}
	outDir := resolveOutputDir(siteDir, opts.OutDir)
	commands := publishCommands(outDir, target)
	if opts.DryRun {
		if opts.NoBuild {
			if err := validatePublishOutputDir(outDir); err != nil {
				return err
			}
		} else if err := validateForceOutputDir(siteDir, outDir); err != nil {
			return err
		}
		printPublishDryRun(opts.Stdout, siteDir, outDir, opts.NoBuild, commands)
		return nil
	}
	if !opts.NoBuild {
		build := opts.buildSite
		if build == nil {
			build = BuildSite
		}
		if err := build(BuildOptions{
			SiteDir:       siteDir,
			OutDir:        opts.OutDir,
			SignKey:       opts.SignKey,
			SignKeySource: opts.SignKeySource,
			Unsigned:      opts.Unsigned,
			Force:         true,
			Stdout:        opts.Stdout,
		}); err != nil {
			return err
		}
	}
	if err := validatePublishOutputDir(outDir); err != nil {
		return err
	}
	runner := opts.commandRunner
	if runner == nil {
		runner = execPublishCommandRunner{}
	}
	for _, command := range commands {
		if err := runner.Run(command); err != nil {
			return err
		}
	}
	fmt.Fprintf(opts.Stdout, "published: %s -> %s\n", cleanSlash(relativeDisplay(siteDir, outDir)), publishRemoteSpec(target, target.Path))
	return nil
}

func publishTargetFromConfig(cfg PublishConfig, opts PublishOptions) (publishTarget, error) {
	method := strings.TrimSpace(cfg.Method)
	if method == "" {
		method = "scp"
	}
	target := publishTarget{
		Method: method,
		Host:   cfg.Host,
		User:   cfg.User,
		Port:   cfg.Port,
		Path:   cfg.Path,
	}
	if opts.HostSet {
		target.Host = opts.Host
	}
	if opts.UserSet {
		target.User = opts.User
	}
	if opts.PortSet {
		target.Port = opts.Port
	}
	if opts.PathSet {
		target.Path = opts.Path
	}
	if target.Port == 0 {
		target.Port = 22
	}
	if target.Method != "scp" {
		return publishTarget{}, fmt.Errorf("publish method must be scp")
	}
	if err := validateSSHField("publish host", target.Host, true); err != nil {
		return publishTarget{}, err
	}
	if err := validateSSHField("publish user", target.User, false); err != nil {
		return publishTarget{}, err
	}
	if target.Port < 1 || target.Port > 65535 {
		return publishTarget{}, fmt.Errorf("publish port must be between 1 and 65535")
	}
	if err := validateRemotePublishPath(target.Path); err != nil {
		return publishTarget{}, err
	}
	return target, nil
}

func validateSSHField(label, value string, required bool) error {
	if required && value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if value == "" {
		return nil
	}
	if strings.ContainsAny(value, "\x00\n\r") {
		return fmt.Errorf("%s must not contain NUL or newline", label)
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("%s must not start with -", label)
	}
	return nil
}

func validateRemotePublishPath(value string) error {
	if value == "" {
		return fmt.Errorf("publish path is required")
	}
	if strings.ContainsAny(value, "\x00\n\r") {
		return fmt.Errorf("publish path must not contain NUL or newline")
	}
	if !strings.HasPrefix(value, "/") {
		return fmt.Errorf("publish path must be absolute")
	}
	if hasDotDotSegment(value) {
		return fmt.Errorf("publish path must not contain .. segments")
	}
	cleaned := pathpkg.Clean(value)
	if cleaned != value {
		return fmt.Errorf("publish path must be clean")
	}
	if cleaned == "/" {
		return fmt.Errorf("publish path must not be /")
	}
	return nil
}

func hasDotDotSegment(value string) bool {
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

func validatePublishOutputDir(outDir string) error {
	info, err := os.Stat(outDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("output directory not found: %s", outDir)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("output path is not a directory: %s", outDir)
	}
	return nil
}

func publishCommands(outDir string, target publishTarget) []publishCommand {
	stage := remoteStagePath(target.Path)
	backup := remoteBackupPath(target.Path)
	return []publishCommand{
		{
			Name:  "ssh",
			Args:  sshArgs(target),
			Stdin: remotePrepareScript(target.Path, stage),
		},
		{
			Name: "scp",
			Args: scpArgs(target, outDir, stage),
		},
		{
			Name:  "ssh",
			Args:  sshArgs(target),
			Stdin: remoteSwapScript(target.Path, stage, backup),
		},
	}
}

func sshArgs(target publishTarget) []string {
	return []string{"-p", strconv.Itoa(target.Port), sshDestination(target), "sh", "-s"}
}

func scpArgs(target publishTarget, outDir, remotePath string) []string {
	return []string{"-P", strconv.Itoa(target.Port), "-r", scpSourceContents(outDir), publishRemoteSpec(target, remotePath)}
}

func scpSourceContents(outDir string) string {
	return filepath.Clean(outDir) + string(filepath.Separator) + "."
}

func sshDestination(target publishTarget) string {
	if target.User == "" {
		return target.Host
	}
	return target.User + "@" + target.Host
}

func publishRemoteSpec(target publishTarget, remotePath string) string {
	return sshDestination(target) + ":" + remotePath
}

func remoteStagePath(targetPath string) string {
	return pathpkg.Join(pathpkg.Dir(targetPath), "."+pathpkg.Base(targetPath)+".smol-upload")
}

func remoteBackupPath(targetPath string) string {
	return pathpkg.Join(pathpkg.Dir(targetPath), "."+pathpkg.Base(targetPath)+".smol-backup")
}

func remotePrepareScript(targetPath, stagePath string) string {
	return strings.Join([]string{
		"set -eu",
		"parent=" + posixSingleQuote(pathpkg.Dir(targetPath)),
		"stage=" + posixSingleQuote(stagePath),
		"mkdir -p \"$parent\"",
		"rm -rf \"$stage\"",
		"mkdir -p \"$stage\"",
		"",
	}, "\n")
}

func remoteSwapScript(targetPath, stagePath, backupPath string) string {
	return strings.Join([]string{
		"set -eu",
		"target=" + posixSingleQuote(targetPath),
		"stage=" + posixSingleQuote(stagePath),
		"backup=" + posixSingleQuote(backupPath),
		"rm -rf \"$backup\"",
		"if [ -e \"$target\" ] || [ -L \"$target\" ]; then",
		"  mv \"$target\" \"$backup\"",
		"fi",
		"if mv \"$stage\" \"$target\"; then",
		"  rm -rf \"$backup\"",
		"else",
		"  status=$?",
		"  if [ -e \"$backup\" ] || [ -L \"$backup\" ]; then",
		"    mv \"$backup\" \"$target\"",
		"  fi",
		"  exit \"$status\"",
		"fi",
		"",
	}, "\n")
}

func posixSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func printPublishDryRun(stdout io.Writer, siteDir, outDir string, noBuild bool, commands []publishCommand) {
	if noBuild {
		fmt.Fprintf(stdout, "dry-run: would publish existing output: %s\n", cleanSlash(relativeDisplay(siteDir, outDir)))
	} else {
		fmt.Fprintf(stdout, "dry-run: would build output: %s\n", cleanSlash(relativeDisplay(siteDir, outDir)))
	}
	for _, command := range commands {
		fmt.Fprintf(stdout, "dry-run: would run: %s\n", formatPublishCommand(command))
	}
}

func formatPublishCommand(command publishCommand) string {
	parts := []string{command.Name}
	parts = append(parts, command.Args...)
	for i, part := range parts {
		parts[i] = shellDisplayQuote(part)
	}
	if command.Stdin != "" {
		return strings.Join(parts, " ") + " < remote-script"
	}
	return strings.Join(parts, " ")
}

func shellDisplayQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z') &&
			!(r >= 'a' && r <= 'z') &&
			!(r >= '0' && r <= '9') &&
			!strings.ContainsRune("@%_+=:,./-", r)
	}) >= 0 {
		return posixSingleQuote(value)
	}
	return value
}

func (execPublishCommandRunner) Run(command publishCommand) error {
	cmd := exec.Command(command.Name, command.Args...)
	if command.Stdin != "" {
		cmd.Stdin = strings.NewReader(command.Stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s failed: %s", command.Name, msg)
	}
	return nil
}
