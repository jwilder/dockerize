package main

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
)

// helper function mirroring the exit code extraction logic
func extractExitCode(err error) int {
	if err == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus()
		}
	}

	return 1
}

func TestExtractExitCode_FromExitError(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 42")
	err := cmd.Run()

	if err == nil {
		t.Fatalf("expected non-nil error")
	}

	code := extractExitCode(err)
	if code != 42 {
		t.Fatalf("expected exit code 42, got %d", code)
	}
}

func TestExtractExitCode_FromGenericError(t *testing.T) {
	err := errors.New("some generic error")

	code := extractExitCode(err)
	if code != 1 {
		t.Fatalf("expected fallback exit code 1, got %d", code)
	}
}
