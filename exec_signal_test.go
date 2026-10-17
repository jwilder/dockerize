package main

import (
	"bytes"
	"context"
	"log"
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

	waitDone := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("timed out waiting for helper to exit")
	}

	if strings.Contains(stderr.String(), "DATA RACE") {
		t.Fatalf("unexpected data race reported: %s", stderr.String())
	}
}

func TestSignalProcessWithTimeoutAlreadyExited(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("run command: %v", err)
	}

	var logBuf bytes.Buffer
	prevOutput := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(prevOutput)

	done := make(chan struct{})
	go func() {
		signalProcessWithTimeout(cmd, syscall.SIGTERM)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("signalProcessWithTimeout did not return for an exited command")
	}

	if logBuf.Len() != 0 {
		t.Errorf("expected no log output, got: %q", logBuf.String())
	}
}

func TestSignalProcessWithTimeoutStopsLiveProcess(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}

	waitDone := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(waitDone)
	}()

	start := time.Now()
	signalProcessWithTimeout(cmd, syscall.SIGTERM, waitDone)
	if elapsed := time.Since(start); elapsed >= 10*time.Second {
		t.Fatalf("signalProcessWithTimeout took %s, expected the signal to stop sleep before the timeout", elapsed)
	}

	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("sleep was not stopped")
	}
}
