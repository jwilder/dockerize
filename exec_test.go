package main

import (
	"bufio"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignalKillTimeoutDefault(t *testing.T) {
	assert.Equal(t, 10*time.Second, signalKillTimeout)
}

func TestSignalProcessWithTimeoutDeliversSignal(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("sleep", "30")
	require.NoError(t, cmd.Start())

	signalProcessWithTimeout(cmd, syscall.SIGTERM)

	require.NotNil(t, cmd.ProcessState)
	status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	assert.True(t, status.Signaled())
	assert.Equal(t, syscall.SIGTERM, status.Signal())
}

func TestSignalProcessWithTimeoutKillsAfterTimeout(t *testing.T) {
	// Not parallel: it overrides the package-level signalKillTimeout.
	oldTimeout := signalKillTimeout
	signalKillTimeout = 200 * time.Millisecond
	t.Cleanup(func() { signalKillTimeout = oldTimeout })

	// exec replaces the shell with sleep, so the ignored TERM disposition
	// is inherited and SIGKILL is the only thing that stops it.
	cmd := exec.Command("sh", "-c", `trap "" TERM; echo ready; exec sleep 30`)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())

	// Wait until the trap is installed before signalling.
	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "ready\n", line)

	// Reap the process in our own goroutine: the kill branch returns without
	// waiting, so reading ProcessState afterwards would race with cmd.Wait.
	waitDone := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(waitDone)
	}()

	start := time.Now()
	signalProcessWithTimeout(cmd, syscall.SIGTERM, waitDone)
	assert.Less(t, time.Since(start), 2*time.Second)

	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("process was not killed after the timeout")
	}

	status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
	require.True(t, ok)
	assert.True(t, status.Signaled())
	assert.Equal(t, syscall.SIGKILL, status.Signal())
}

func TestSignalProcessWithTimeoutReturnsWhenAlreadyDone(t *testing.T) {
	// Not parallel: it overrides the package-level signalKillTimeout.
	oldTimeout := signalKillTimeout
	signalKillTimeout = 5 * time.Second
	t.Cleanup(func() { signalKillTimeout = oldTimeout })

	cmd := exec.Command("sleep", "30")
	require.NoError(t, cmd.Start())

	waitDone := make(chan struct{})
	close(waitDone)

	start := time.Now()
	signalProcessWithTimeout(cmd, syscall.SIGTERM, waitDone)
	assert.Less(t, time.Since(start), time.Second)

	_ = cmd.Wait()
}
