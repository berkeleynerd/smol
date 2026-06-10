package main

import (
	"strings"
	"testing"
)

func TestRunRejectsUnknownCommand(t *testing.T) {
	var stdout strings.Builder
	var stderr strings.Builder
	code := Run([]string{"wat"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "unknown command: wat") {
		t.Fatalf("stderr missing unknown command: %s", stderr.String())
	}
}

func TestBuildOptionsMustPrecedePositionals(t *testing.T) {
	err := commandBuild([]string{".", "--out", "public"}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "options must appear before positional arguments") {
		t.Fatalf("expected option ordering error, got %v", err)
	}
	if _, ok := err.(usageError); !ok {
		t.Fatalf("option ordering error should be usageError, got %T", err)
	}
}

func TestFlagParseErrorsKeepDoubleDashForLongFlags(t *testing.T) {
	tests := map[string]struct {
		args []string
		want string
	}{
		"unknown flag": {
			args: []string{"publish", "--does-not-exist"},
			want: "flag provided but not defined: --does-not-exist",
		},
		"missing flag value": {
			args: []string{"publish", "--sign-key"},
			want: "flag needs an argument: --sign-key",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			if got := Run(tc.args, &stdout, &stderr); got != 2 {
				t.Fatalf("Run(%v) = %d, want 2; stdout=%q stderr=%q", tc.args, got, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tc.want)
			}
		})
	}
}

func TestSigningFlagUsageErrors(t *testing.T) {
	tests := map[string]struct {
		args []string
		want string
	}{
		"empty sign key": {
			args: []string{"build", "--sign-key="},
			want: "--sign-key requires a non-empty key",
		},
		"conflicting signing flags": {
			args: []string{"build", "--sign", "--sign-key", "KEY"},
			want: "--sign and --sign-key cannot be used together",
		},
		"publish no build signing": {
			args: []string{"publish", "--no-build", "--sign"},
			want: "--no-build skips the build phase",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			if got := Run(tc.args, &stdout, &stderr); got != 2 {
				t.Fatalf("Run(%v) = %d, want 2; stdout=%q stderr=%q", tc.args, got, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), tc.want)
			}
		})
	}
}

func TestUnsignedSuppressesSigningFlagConflicts(t *testing.T) {
	dir := testSite(t)
	var stdout, stderr strings.Builder
	code := Run([]string{"build", "--unsigned", "--sign", "--sign-key=", dir}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run unsigned conflicting signing flags = %d, want 0; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	html := readText(t, dir+"/public/index.html")
	if strings.Contains(html, "FAKE-SIGNED") {
		t.Fatalf("unsigned build was signed: %s", html)
	}
}
