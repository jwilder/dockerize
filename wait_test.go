package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWaitForSocket(t *testing.T) {
	// Start a TCP listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer ln.Close()

	// Accept connections in background
	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	// Save and restore globals
	origWaitRetryInterval := waitRetryInterval
	origWaitTimeoutFlag := waitTimeoutFlag
	defer func() {
		waitRetryInterval = origWaitRetryInterval
		waitTimeoutFlag = origWaitTimeoutFlag
	}()

	waitRetryInterval = 100 * time.Millisecond
	waitTimeoutFlag = 5 * time.Second

	// Reset the WaitGroup for this test
	wg = sync.WaitGroup{}

	waitForSocket("tcp", ln.Addr().String(), waitTimeoutFlag)
	wg.Wait() // Should complete quickly since the listener is ready
}

func TestWaitForDependencies_TCP(t *testing.T) {
	// Start a TCP listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)
	defer ln.Close()

	// Accept connections in background
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	// Save and restore globals
	origURLs := urls
	origWaitRetryInterval := waitRetryInterval
	origWaitTimeoutFlag := waitTimeoutFlag
	origWaitFlag := waitFlag
	defer func() {
		urls = origURLs
		waitRetryInterval = origWaitRetryInterval
		waitTimeoutFlag = origWaitTimeoutFlag
		waitFlag = origWaitFlag
	}()

	waitRetryInterval = 100 * time.Millisecond
	waitTimeoutFlag = 5 * time.Second
	wg = sync.WaitGroup{}

	u, err := url.Parse("tcp://" + ln.Addr().String())
	assert.NoError(t, err)
	urls = []url.URL{*u}
	waitFlag = hostFlagsVar{"tcp://" + ln.Addr().String()}

	waitForDependencies()
}

func TestWaitForDependencies_HTTP(t *testing.T) {
	// Start a test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	// Save and restore globals
	origURLs := urls
	origHeaders := headers
	origWaitRetryInterval := waitRetryInterval
	origWaitTimeoutFlag := waitTimeoutFlag
	origWaitFlag := waitFlag
	defer func() {
		urls = origURLs
		headers = origHeaders
		waitRetryInterval = origWaitRetryInterval
		waitTimeoutFlag = origWaitTimeoutFlag
		waitFlag = origWaitFlag
	}()

	waitRetryInterval = 100 * time.Millisecond
	waitTimeoutFlag = 5 * time.Second
	headers = nil
	wg = sync.WaitGroup{}

	u, err := url.Parse(ts.URL)
	assert.NoError(t, err)
	urls = []url.URL{*u}
	waitFlag = hostFlagsVar{ts.URL}

	waitForDependencies()
}
