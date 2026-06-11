package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunCmdSignalShutdownHelper(t *testing.T) {
	if os.Getenv("RUNCMD_SIGNAL_SHUTDOWN_HELPER") != "1" {
		t.Skip("helper test")
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg.Add(1)
	runCmd(ctx, cancel, "sleep", "30")
}

func TestRunCmdSignalShutdown(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunCmdSignalShutdownHelper$")
	cmd.Env = append(os.Environ(), "RUNCMD_SIGNAL_SHUTDOWN_HELPER=1", "GORACE=halt_on_error=1")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}

	time.Sleep(time.Second)

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("signal helper: %v", err)
	}

	waitErrCh := make(chan error, 1)
	go func() {
		waitErrCh <- cmd.Wait()
	}()

	select {
	case <-waitErrCh:
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("timed out waiting for helper to exit")
	}

	if strings.Contains(stderr.String(), "DATA RACE") {
		t.Fatalf("unexpected data race reported: %s", stderr.String())
	}
}
