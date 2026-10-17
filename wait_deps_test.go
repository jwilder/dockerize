package main

import (
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// closedTCPPort returns a local TCP port with nothing listening on it.
func closedTCPPort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

// waitForDependenciesReturns runs waitForDependencies and fails the test if it does not return within d.
func waitForDependenciesReturns(t *testing.T, d time.Duration) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		defer close(done)
		waitForDependencies()
	}()

	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("waitForDependencies did not return within %s", d)
	}
}

func TestWaitForDependenciesUnsupportedSchemeFatal(t *testing.T) {
	if fatalCase("unsupported-scheme") {
		waitTimeoutFlag = 10 * time.Second
		waitRetryInterval = 10 * time.Millisecond
		urls = []url.URL{{Scheme: "ftp", Host: "x"}}
		waitForDependencies()
		return
	}

	exitCode, stderr := runFatalCase(t, "TestWaitForDependenciesUnsupportedSchemeFatal", "unsupported-scheme")
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "invalid host protocol provided: ftp")
}

func TestWaitForDependenciesTimeoutFatal(t *testing.T) {
	if fatalCase("timeout") {
		port := closedTCPPort(t)
		waitTimeoutFlag = 300 * time.Millisecond
		waitRetryInterval = 10 * time.Millisecond
		urls = []url.URL{{Scheme: "tcp", Host: "127.0.0.1:" + strconv.Itoa(port)}}
		waitForDependencies()
		return
	}

	exitCode, stderr := runFatalCase(t, "TestWaitForDependenciesTimeoutFatal", "timeout")
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "Timeout after")
}

func TestWaitForDependenciesUnixSocket(t *testing.T) {
	resetGlobals(t)

	sockPath := filepath.Join(t.TempDir(), "s.sock")
	l, err := net.Listen("unix", sockPath)
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	u, err := url.Parse("unix://" + sockPath)
	require.NoError(t, err)

	urls = []url.URL{*u}
	waitTimeoutFlag = 5 * time.Second
	waitRetryInterval = 10 * time.Millisecond

	waitForDependenciesReturns(t, 10*time.Second)
}

func TestWaitForDependenciesTCP4(t *testing.T) {
	resetGlobals(t)

	l, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()

	u, err := url.Parse("tcp4://" + l.Addr().String())
	require.NoError(t, err)

	urls = []url.URL{*u}
	waitTimeoutFlag = 5 * time.Second
	waitRetryInterval = 10 * time.Millisecond

	waitForDependenciesReturns(t, 10*time.Second)
}

func TestWaitForDependenciesTCP6(t *testing.T) {
	l, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback not available: %v", err)
	}
	defer func() { _ = l.Close() }()

	resetGlobals(t)

	u, err := url.Parse("tcp6://" + l.Addr().String())
	require.NoError(t, err)

	urls = []url.URL{*u}
	waitTimeoutFlag = 5 * time.Second
	waitRetryInterval = 10 * time.Millisecond

	waitForDependenciesReturns(t, 10*time.Second)
}
