package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	Version = "0.1.0"
	Profile = "smol/static-nojs-v1"
)

type usageError struct {
	msg string
}

func (e usageError) Error() string {
	return e.msg
}

func main() {
	os.Exit(RunWithIO(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, isTerminal(os.Stdin)))
}

func Run(args []string, stdout, stderr io.Writer) int {
	return RunWithIO(args, strings.NewReader(""), stdout, stderr, false)
}

func RunWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer, interactive bool) int {
	if len(args) == 0 {
		printHelp(stdout)
		return 0
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printHelp(stdout)
		return 0
	}

	var err error
	switch args[0] {
	case "init":
		err = commandInit(args[1:])
	case "new":
		err = commandNew(args[1:])
	case "build":
		err = commandBuildWithIO(args[1:], stdin, stdout, stderr, interactive)
	case "check":
		err = commandCheck(args[1:], stdout)
	case "publish":
		err = commandPublishWithIO(args[1:], stdin, stdout, stderr, interactive)
	default:
		fmt.Fprintf(stderr, "error: unknown command: %s\n", args[0])
		return 2
	}

	if err == nil {
		return 0
	}
	fmt.Fprintf(stderr, "error: %s\n", err)
	if _, ok := err.(usageError); ok {
		return 2
	}
	return 1
}

func commandInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	starter := fs.String("starter", starterDefault, "starter name")
	if err := fs.Parse(args); err != nil {
		return usageError{formatFlagParseError(err, args)}
	}
	if !isValidStarter(*starter) {
		return usageError{"unknown starter: " + *starter + " (valid: " + validStarterList() + ")"}
	}
	if fs.NArg() > 1 {
		return usageError{"usage: smol init [--starter " + validStarterList() + "] [DIR]"}
	}
	dir := "."
	if fs.NArg() == 1 {
		dir = fs.Arg(0)
	}
	return InitSiteWithStarter(dir, *starter)
}

func commandNew(args []string) error {
	if len(args) < 1 {
		return usageError{"missing content kind"}
	}
	if len(args) < 2 {
		return usageError{"missing slug"}
	}
	if len(args) < 3 {
		return usageError{"missing title"}
	}
	kind := args[0]
	if kind != "page" && kind != "post" {
		return usageError{"unknown content kind: " + kind}
	}
	title := strings.Join(args[2:], " ")
	siteDir, err := os.Getwd()
	if err != nil {
		return err
	}
	return NewContent(siteDir, kind, args[1], title)
}

func commandBuild(args []string, stdout io.Writer) error {
	return commandBuildWithIO(args, strings.NewReader(""), stdout, io.Discard, false)
}

func commandBuildWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer, interactive bool) error {
	if flagsAppearAfterPositionals(args, "out", "sign-key", "attest", "attested-html") {
		return usageError{"options must appear before positional arguments"}
	}

	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	out := fs.String("out", "public", "output directory")
	sign := fs.Bool("sign", false, "interactively choose a signing key")
	signKey := fs.String("sign-key", "", "signing key fingerprint")
	attest := fs.String("attest", "", "attest signer binary")
	attestedHTML := fs.String("attested-html", "", "attested-html signer binary alias")
	unsigned := fs.Bool("unsigned", false, "build unsigned HTML even when sign_key is configured")
	force := fs.Bool("force", false, "remove output directory before build")
	if err := fs.Parse(args); err != nil {
		return usageError{formatFlagParseError(err, args)}
	}
	if fs.NArg() > 1 {
		return usageError{"usage: smol build [options] [SITE_DIR]"}
	}
	siteDir := "."
	if fs.NArg() == 1 {
		siteDir = fs.Arg(0)
	}
	attestPath := *attest
	if attestPath == "" {
		attestPath = *attestedHTML
	}
	signingIntent, err := classifyCLISigning(*unsigned, *sign, *signKey, flagWasSupplied(args, "sign-key"))
	if err != nil {
		return err
	}
	if err := preflightBuild(BuildOptions{
		SiteDir:       siteDir,
		OutDir:        *out,
		SignKey:       signingIntent.key,
		SignKeySource: signingIntent.source,
		Unsigned:      *unsigned,
		Force:         *force,
	}, signingIntent.intent == signingIntentInteractive); err != nil {
		return err
	}
	signing, err := resolveCLISigning(signingIntent, cliSigningOptions{
		Stdin:       stdin,
		Stderr:      stderr,
		Interactive: interactive,
	})
	if err != nil {
		return err
	}
	return BuildSite(BuildOptions{
		SiteDir:       siteDir,
		OutDir:        *out,
		SignKey:       signing.Key,
		AttestPath:    attestPath,
		Unsigned:      *unsigned,
		Force:         *force,
		Stdout:        stdout,
		SignKeySource: signing.Source,
	})
}

func commandCheck(args []string, stdout io.Writer) error {
	if flagsAppearAfterPositionals(args, "mode") {
		return usageError{"options must appear before positional arguments"}
	}

	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	mode := fs.String("mode", checkModeDefault, "check mode")
	if err := fs.Parse(args); err != nil {
		return usageError{formatFlagParseError(err, args)}
	}
	if fs.NArg() != 1 {
		return usageError{"usage: smol check [--mode default|flat-xhtml-v1|flat-gemini-v1] PATH"}
	}
	if _, err := normalizeCheckMode(*mode); err != nil {
		return usageError{err.Error()}
	}
	return CheckPath(CheckOptions{
		Path:   fs.Arg(0),
		Mode:   *mode,
		Stdout: stdout,
	})
}

func commandPublish(args []string, stdout io.Writer) error {
	return commandPublishWithIO(args, strings.NewReader(""), stdout, io.Discard, false)
}

var runPublishSite = PublishSite

func commandPublishWithIO(args []string, stdin io.Reader, stdout, stderr io.Writer, interactive bool) error {
	if flagsAppearAfterPositionals(args, "out", "sign-key", "host", "user", "port", "path") {
		return usageError{"options must appear before positional arguments"}
	}

	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	out := fs.String("out", "public", "output directory")
	noBuild := fs.Bool("no-build", false, "publish existing output without building")
	sign := fs.Bool("sign", false, "interactively choose a signing key")
	signKey := fs.String("sign-key", "", "signing key fingerprint")
	unsigned := fs.Bool("unsigned", false, "build unsigned output even when sign_key is configured")
	host := fs.String("host", "", "SSH host override")
	user := fs.String("user", "", "SSH user override")
	port := fs.Int("port", 0, "SSH port override")
	path := fs.String("path", "", "remote publish path override")
	dryRun := fs.Bool("dry-run", false, "print publish steps without building or connecting")
	if err := fs.Parse(args); err != nil {
		return usageError{formatFlagParseError(err, args)}
	}
	if fs.NArg() > 1 {
		return usageError{"usage: smol publish [options] [SITE_DIR]"}
	}
	siteDir := "."
	if fs.NArg() == 1 {
		siteDir = fs.Arg(0)
	}
	signKeySupplied := flagWasSupplied(args, "sign-key")
	signingIntent, err := classifyCLISigning(*unsigned, *sign, *signKey, signKeySupplied)
	if err != nil {
		return err
	}
	if *noBuild && signingIntent.activeSigning() {
		return usageError{"--no-build skips the build phase, so signing cannot run; remove --no-build to build and sign, or drop --sign/--sign-key to publish existing output"}
	}
	preflightOpts := PublishOptions{
		SiteDir:       siteDir,
		OutDir:        *out,
		NoBuild:       *noBuild,
		SignKey:       signingIntent.key,
		Unsigned:      *unsigned,
		DryRun:        *dryRun,
		Host:          *host,
		User:          *user,
		Port:          *port,
		Path:          *path,
		HostSet:       flagWasSupplied(args, "host"),
		UserSet:       flagWasSupplied(args, "user"),
		PortSet:       flagWasSupplied(args, "port"),
		PathSet:       flagWasSupplied(args, "path"),
		SignKeySource: signingIntent.source,
	}
	if err := preflightPublish(preflightOpts, signingIntent.intent == signingIntentInteractive); err != nil {
		return err
	}
	signing, err := resolveCLISigning(signingIntent, cliSigningOptions{
		Stdin:       stdin,
		Stderr:      stderr,
		Interactive: interactive,
		DryRun:      *dryRun,
	})
	if err != nil {
		return err
	}
	err = runPublishSite(PublishOptions{
		SiteDir:       siteDir,
		OutDir:        *out,
		NoBuild:       *noBuild,
		SignKey:       signing.Key,
		Unsigned:      *unsigned,
		DryRun:        *dryRun,
		Host:          *host,
		User:          *user,
		Port:          *port,
		Path:          *path,
		HostSet:       flagWasSupplied(args, "host"),
		UserSet:       flagWasSupplied(args, "user"),
		PortSet:       flagWasSupplied(args, "port"),
		PathSet:       flagWasSupplied(args, "path"),
		Stdout:        stdout,
		SignKeySource: signing.Source,
	})
	if err == nil && *dryRun {
		switch signingIntent.intent {
		case signingIntentInteractive:
			fmt.Fprintln(stdout, "dry-run: signing key would be resolved on a real run")
		case signingIntentExplicit:
			fmt.Fprintf(stdout, "dry-run: would sign with key %s\n", signingIntent.key)
		}
	}
	return err
}

type cliSigningOptions struct {
	Stdin       io.Reader
	Stderr      io.Writer
	Interactive bool
	DryRun      bool
}

type cliSigningResolution struct {
	Key string
	// Source is true when CLI input made an explicit signing decision. It also
	// covers declined interactive signing, where Key is empty but smol.json
	// sign_key fallback must be bypassed.
	Source bool
}

type signingIntentKind int

const (
	signingIntentNone signingIntentKind = iota
	signingIntentUnsigned
	signingIntentExplicit
	signingIntentInteractive
)

type cliSigningIntent struct {
	intent signingIntentKind
	key    string
	source bool
}

func (i cliSigningIntent) activeSigning() bool {
	return i.intent == signingIntentExplicit || i.intent == signingIntentInteractive
}

func classifyCLISigning(unsigned, sign bool, signKey string, signKeySupplied bool) (cliSigningIntent, error) {
	if unsigned {
		return cliSigningIntent{intent: signingIntentUnsigned}, nil
	}
	if sign && signKeySupplied {
		return cliSigningIntent{}, usageError{"--sign and --sign-key cannot be used together"}
	}
	if signKeySupplied {
		key := strings.TrimSpace(signKey)
		if key == "" {
			return cliSigningIntent{}, usageError{"--sign-key requires a non-empty key"}
		}
		return cliSigningIntent{intent: signingIntentExplicit, key: key, source: true}, nil
	}
	if sign {
		return cliSigningIntent{intent: signingIntentInteractive, source: true}, nil
	}
	return cliSigningIntent{}, nil
}

func resolveCLISigning(intent cliSigningIntent, opts cliSigningOptions) (cliSigningResolution, error) {
	switch intent.intent {
	case signingIntentUnsigned, signingIntentNone:
		return cliSigningResolution{}, nil
	case signingIntentExplicit:
		return cliSigningResolution{Key: intent.key, Source: true}, nil
	case signingIntentInteractive:
		if opts.DryRun {
			if err := probeInteractiveSigning(opts.Interactive); err != nil {
				return cliSigningResolution{}, err
			}
			return cliSigningResolution{Source: true}, nil
		}
		key, err := promptSigningKey(opts.Stdin, opts.Stderr, opts.Interactive)
		return cliSigningResolution{Key: key, Source: true}, err
	default:
		return cliSigningResolution{Source: true}, nil
	}
}

func probeInteractiveSigning(interactive bool) error {
	if !interactive {
		return fmt.Errorf("interactive signing requires a terminal; use --sign-key KEY or run from a terminal")
	}
	keys, err := listSecretKeys()
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return fmt.Errorf("no GPG secret keys found")
	}
	return nil
}

func preflightBuild(opts BuildOptions, requireSigningCompatible bool) error {
	if opts.SiteDir == "" {
		opts.SiteDir = "."
	}
	if opts.OutDir == "" {
		opts.OutDir = "public"
	}
	siteDir, err := filepath.Abs(opts.SiteDir)
	if err != nil {
		return err
	}
	siteCfg, err := LoadSiteConfig(siteDir)
	if err != nil {
		return err
	}
	signKey := opts.SignKey
	if opts.Unsigned {
		signKey = ""
		requireSigningCompatible = false
	} else if signKey == "" && !opts.SignKeySource {
		signKey = siteCfg.SignKey
	}
	if (requireSigningCompatible || signKey != "") && siteCfg.OutputMode == outputModeFlatGemini {
		return fmt.Errorf("signing is not supported for flat-gemini-v1")
	}
	if opts.Force {
		if err := validateForceOutputDir(siteDir, resolveOutputDir(siteDir, opts.OutDir)); err != nil {
			return err
		}
	}
	return nil
}

func preflightPublish(opts PublishOptions, requireSigningCompatible bool) error {
	if opts.SiteDir == "" {
		opts.SiteDir = "."
	}
	if opts.OutDir == "" {
		opts.OutDir = "public"
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
	if _, err := publishTargetFromConfig(siteCfg.Publish, opts); err != nil {
		return err
	}
	outDir := resolveOutputDir(siteDir, opts.OutDir)
	if opts.NoBuild {
		if opts.DryRun {
			if err := validatePublishOutputDir(outDir); err != nil {
				return err
			}
		}
		return nil
	}
	return preflightBuild(BuildOptions{
		SiteDir:       siteDir,
		OutDir:        opts.OutDir,
		SignKey:       opts.SignKey,
		SignKeySource: opts.SignKeySource,
		Unsigned:      opts.Unsigned,
		Force:         true,
	}, requireSigningCompatible)
}

func promptSigningKey(stdin io.Reader, stderr io.Writer, interactive bool) (string, error) {
	if !interactive {
		return "", fmt.Errorf("interactive signing requires a terminal; use --sign-key KEY or run from a terminal")
	}
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	if stderr == nil {
		stderr = io.Discard
	}
	keys, err := listSecretKeys()
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", fmt.Errorf("no GPG secret keys found")
	}
	if len(keys) == 1 {
		fmt.Fprintf(stderr, "Signing with %s\n", formatGPGSecretKey(keys[0]))
		return keys[0].Fingerprint, nil
	}
	reader := bufio.NewReader(stdin)
	fmt.Fprintln(stderr, "Select a signing key:")
	for i, key := range keys {
		fmt.Fprintf(stderr, "  %d) %s\n", i+1, formatGPGSecretKey(key))
	}
	fmt.Fprint(stderr, "Signing key number [blank/q for unsigned]: ")
	answer, err := readPromptLine(reader)
	if err != nil {
		return "", err
	}
	if isDeclineAnswer(answer) {
		return "", nil
	}
	index, err := strconv.Atoi(answer)
	if err != nil || strconv.Itoa(index) != answer || index < 1 || index > len(keys) {
		return "", fmt.Errorf("invalid signing key selection: %q", answer)
	}
	return keys[index-1].Fingerprint, nil
}

func readPromptLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func isDeclineAnswer(answer string) bool {
	switch strings.ToLower(answer) {
	case "", "n", "no", "q", "quit":
		return true
	default:
		return false
	}
}

func formatGPGSecretKey(key gpgSecretKey) string {
	if key.UID == "" {
		return key.Fingerprint
	}
	return key.Fingerprint + " " + key.UID
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func formatFlagParseError(err error, args []string) string {
	msg := err.Error()
	for _, prefix := range []string{
		"flag provided but not defined: -",
		"flag needs an argument: -",
	} {
		if !strings.HasPrefix(msg, prefix) {
			continue
		}
		name := strings.TrimPrefix(msg, prefix)
		if flagUsedWithDoubleDash(args, name) {
			return prefix + "-" + name
		}
	}
	return msg
}

func flagUsedWithDoubleDash(args []string, name string) bool {
	long := "--" + name
	for _, arg := range args {
		if eq := strings.IndexByte(arg, '='); eq >= 0 {
			arg = arg[:eq]
		}
		if arg == long {
			return true
		}
	}
	return false
}

func flagsAppearAfterPositionals(args []string, valueFlagNames ...string) bool {
	valueFlags := map[string]bool{}
	for _, name := range valueFlagNames {
		valueFlags["--"+name] = true
		valueFlags["-"+name] = true
	}
	seenPositional := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			seenPositional = true
			continue
		}
		if !seenPositional && strings.HasPrefix(arg, "-") {
			name := arg
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				name = name[:eq]
			}
			if valueFlags[name] && !strings.Contains(arg, "=") {
				i++
			}
			continue
		}
		if seenPositional && strings.HasPrefix(arg, "-") {
			return true
		}
		seenPositional = true
	}
	return false
}

func flagWasSupplied(args []string, name string) bool {
	long := "--" + name
	short := "-" + name
	prefixLong := long + "="
	prefixShort := short + "="
	for _, arg := range args {
		if arg == long || arg == short || strings.HasPrefix(arg, prefixLong) || strings.HasPrefix(arg, prefixShort) {
			return true
		}
	}
	return false
}

func printHelp(w io.Writer) {
	fmt.Fprint(w, `smol 0.1.0

Usage:
  smol init [--starter default|classic-xhtml] [DIR]
  smol new page SLUG TITLE
  smol new post SLUG TITLE
  smol build [options] [SITE_DIR]
  smol check [options] PATH
  smol publish [options] [SITE_DIR]
  smol help

Init options:
  --starter NAME            Starter: default or classic-xhtml. Default: default.

Build options:
  --out DIR                 Output directory. Default: public.
  --sign                    Interactively choose a signing key.
  --sign-key FINGERPRINT    Sign generated pages with attest/attested-html.
  --attest PATH             Path to attest signer binary. Default: auto-discover.
  --attested-html PATH      Backward-compatible signer binary alias.
  --unsigned                Build unsigned HTML even when sign_key is configured.
  --force                   Remove existing output directory before build.

Check options:
  --mode MODE               Check mode: default, flat-xhtml-v1, or flat-gemini-v1.
                            Default: default.

Publish options:
  --out DIR                 Output directory. Default: public.
  --no-build                Publish existing output without building.
  --sign                    Interactively choose a signing key during the build phase.
  --sign-key FINGERPRINT    Sign generated pages during the build phase.
  --unsigned                Build unsigned output even when sign_key is configured.
  --host HOST               SSH host override.
  --user USER               SSH user override.
  --port PORT               SSH port override.
  --path PATH               Remote publish path override.
  --dry-run                 Print publish steps without building or connecting.

Options must appear before positional arguments.
`)
}
