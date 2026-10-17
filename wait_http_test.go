package main

import (
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitGroupDone returns a channel that is closed once wg is done.
func waitGroupDone() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()
	return done
}

// within reports whether done is closed within d.
func within(done <-chan struct{}, d time.Duration) bool {
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

func TestWaitForHTTPInvalidURLDoesNotPanic(t *testing.T) {
	// The retry goroutine never ends on its own, so the case runs in a child process.
	// Without the fix, the nil request panics in the child and the exit code is not 0.
	if fatalCase("invalid-url") {
		// The child process exits when the test returns, so it does not restore globals or the log output.
		waitTimeoutFlag = 10 * time.Second
		waitRetryInterval = 10 * time.Millisecond
		var logs syncBuffer
		log.SetOutput(&logs)

		// A control character in the host makes url.Parse fail inside http.NewRequest.
		waitForHTTP(url.URL{Scheme: "http", Host: "bad\x7fhost"})

		deadline := time.Now().Add(5 * time.Second)
		for strings.Count(logs.String(), "Problem creating request") < 2 {
			if time.Now().After(deadline) {
				t.Fatalf("expected at least two retries, got logs: %q", logs.String())
			}
			time.Sleep(5 * time.Millisecond)
		}
		return
	}

	exitCode, stderr := runFatalCase(t, "TestWaitForHTTPInvalidURLDoesNotPanic", "invalid-url")
	assert.Equal(t, 0, exitCode)
	assert.NotContains(t, stderr, "panic:")
}

func TestWaitForHTTPStatusCodes(t *testing.T) {
	tests := []struct {
		name      string
		code      int
		wantRetry bool
	}{
		{name: "200 succeeds", code: http.StatusOK},
		{name: "204 succeeds", code: http.StatusNoContent},
		{name: "301 without Location retries", code: http.StatusMovedPermanently, wantRetry: true},
		{name: "404 retries", code: http.StatusNotFound, wantRetry: true},
		{name: "500 retries", code: http.StatusInternalServerError, wantRetry: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetGlobals(t)
			waitTimeoutFlag = 5 * time.Second
			waitRetryInterval = 10 * time.Millisecond
			logs := captureLog(t)

			var code atomic.Int32
			var hits atomic.Int32
			code.Store(int32(tt.code))
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				w.WriteHeader(int(code.Load()))
			}))
			defer srv.Close()

			u, err := url.Parse(srv.URL)
			require.NoError(t, err)

			waitForHTTP(*u)
			done := waitGroupDone()

			finalCode := tt.code
			if tt.wantRetry {
				require.Eventually(t, func() bool { return hits.Load() > 1 }, 5*time.Second, 5*time.Millisecond)
				assert.Contains(t, logs.String(), "Received "+strconv.Itoa(tt.code)+" from")
				assert.False(t, within(done, 50*time.Millisecond), "waitForHTTP returned on status %d", tt.code)

				finalCode = http.StatusOK
				code.Store(int32(finalCode))
			}

			require.True(t, within(done, 5*time.Second), "waitForHTTP did not return after status %d", tt.code)
			assert.Contains(t, logs.String(), "Received "+strconv.Itoa(finalCode)+" from")
		})
	}
}

func TestWaitForHTTPSendsHeaders(t *testing.T) {
	resetGlobals(t)
	waitTimeoutFlag = 5 * time.Second
	waitRetryInterval = 10 * time.Millisecond
	captureLog(t)

	headers = []HttpHeader{
		{name: "X-Test-One", value: "first"},
		{name: "X-Test-Two", value: "second"},
	}

	got := make(chan http.Header, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case got <- r.Header.Clone():
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	waitForHTTP(*u)
	require.True(t, within(waitGroupDone(), 5*time.Second), "waitForHTTP did not return")

	h := <-got
	assert.Equal(t, "first", h.Get("X-Test-One"))
	assert.Equal(t, "second", h.Get("X-Test-Two"))
}
