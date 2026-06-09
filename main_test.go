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
