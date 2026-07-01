package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func runCmd(ctx context.Context, cancel context.CancelFunc, cmd string, args ...string) {
	defer wg.Done()

	process := exec.Command(cmd, args...)
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr

	// start the process
	err := process.Start()
	if err != nil {
		log.Fatalf("Error starting command: `%s` - %s\n", cmd, err)
	}

	// Setup signaling
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)
	waitDone := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()

		select {
		case sig := <-sigs:
			log.Printf("Received signal: %s\n", sig)
			signalProcessWithTimeout(process, sig, waitDone)
			cancel()
		case <-ctx.Done():
			// exit when context is done
		}
	}()

	err = process.Wait()
	close(waitDone)
	cancel()

	if err == nil {
		log.Println("Command finished successfully.")
	} else {
		log.Printf("Command exited with error: %s\n", err)

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				os.Exit(status.ExitStatus())
			}
		}

		// Fallback for non-ExitError types (e.g., *os.SyscallError)
		os.Exit(1)
	}

}

func signalProcessWithTimeout(process *exec.Cmd, sig os.Signal, waitDone ...<-chan struct{}) {
	if err := process.Process.Signal(sig); err != nil {
		log.Printf("Failed to send signal %s: %s\n", sig, err)
	}
	if len(waitDone) == 0 {
		done := make(chan struct{})
		go func() {
			_ = process.Wait() // error handled by the caller's Wait call
			close(done)
		}()
		waitDone = []<-chan struct{}{done}
	}
	select {
	case <-waitDone[0]:
		return
	case <-time.After(10 * time.Second):
		log.Println("Killing command due to timeout.")
		if err := process.Process.Kill(); err != nil {
			log.Printf("Failed to kill process: %s\n", err)
		}
	}
}
