package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fatalCaseEnv names the environment variable that selects a log.Fatalf case in a re-executed test binary.
const fatalCaseEnv = "DOCKERIZE_FATAL_CASE"

// resetGlobals snapshots the package globals tests mutate, resets wg, and restores the snapshot in t.Cleanup.
// Tests that call it must not run in parallel.
func resetGlobals(t *testing.T) {
	t.Helper()

	oldVersion, oldPoll := version, poll
	oldDelims, oldHeaders, oldURLs := delims, headers, urls
	oldWaitFlag, oldWaitTimeout, oldWaitRetry := waitFlag, waitTimeoutFlag, waitRetryInterval
	oldDialTimeout, oldNoOverwrite := dialTimeout, noOverwriteFlag
	oldCtx, oldCancel := ctx, cancel

	t.Cleanup(func() {
		version, poll = oldVersion, oldPoll
		delims, headers, urls = oldDelims, oldHeaders, oldURLs
		waitFlag, waitTimeoutFlag, waitRetryInterval = oldWaitFlag, oldWaitTimeout, oldWaitRetry
		dialTimeout, noOverwriteFlag = oldDialTimeout, oldNoOverwrite
		ctx, cancel = oldCtx, oldCancel
		wg = sync.WaitGroup{}
	})

	wg = sync.WaitGroup{}
}

// runFatalCase re-runs testName in a child test binary with caseName set, returning its exit code and stderr.
// The child runs the case body guarded by fatalCase(caseName).
func runFatalCase(t *testing.T, testName, caseName string) (int, string) {
	t.Helper()

	runCtx, cancelRun := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelRun()

	cmd := exec.CommandContext(runCtx, os.Args[0], "-test.run=^"+testName+"$")
	cmd.Env = append(os.Environ(), fatalCaseEnv+"="+caseName)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if runCtx.Err() != nil {
		t.Fatalf("fatal case %q timed out", caseName)
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), stderr.String()
	}
	if err != nil {
		t.Fatalf("run fatal case %q: %v", caseName, err)
	}
	return 0, stderr.String()
}

// fatalCase reports whether this process is the child re-run selected for name.
func fatalCase(name string) bool {
	return os.Getenv(fatalCaseEnv) == name
}

func TestResetGlobalsRestoresPackageState(t *testing.T) {
	before := waitTimeoutFlag
	beforeNoOverwrite := noOverwriteFlag

	t.Run("mutates", func(t *testing.T) {
		resetGlobals(t)
		waitTimeoutFlag = before + time.Hour
		noOverwriteFlag = !beforeNoOverwrite
	})

	assert.Equal(t, before, waitTimeoutFlag)
	assert.Equal(t, beforeNoOverwrite, noOverwriteFlag)
}

func TestRunFatalCaseCapturesLogFatalf(t *testing.T) {
	if fatalCase("boom") {
		log.Fatalf("boom")
	}

	exitCode, stderr := runFatalCase(t, "TestRunFatalCaseCapturesLogFatalf", "boom")
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "boom")
}
