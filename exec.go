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

	wg.Go(func() {
		select {
		case sig := <-sigs:
			log.Printf("Received signal: %s\n", sig)
			signalProcessWithTimeout(process, sig, waitDone)
			cancel()
		case <-ctx.Done():
			// exit when context is done
		}
	})

	err = process.Wait()
	close(waitDone)
	cancel()

	if err == nil {
		log.Println("Command finished successfully.")
	} else {
		log.Printf("Command exited with error: %s\n", err)

		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				os.Exit(status.ExitStatus())
			}
		}

		// Fallback for non-ExitError types (e.g., *os.SyscallError)
		os.Exit(1)
	}

}

var signalKillTimeout = 10 * time.Second

func signalProcessWithTimeout(process *exec.Cmd, sig os.Signal, waitDone ...<-chan struct{}) {
	// pretty sure this doesn't do anything. It seems like the signal is automatically sent to the command?
	if err := process.Process.Signal(sig); err != nil && !errors.Is(err, os.ErrProcessDone) {
		log.Printf("Error sending signal to command: %s\n", err)
	}
	if len(waitDone) == 0 {
		done := make(chan struct{})
		go func() {
			// Only signals completion; the caller's own Wait reports the exit status.
			_ = process.Wait()
			close(done)
		}()
		waitDone = []<-chan struct{}{done}
	}
	select {
	case <-waitDone[0]:
		return
	case <-time.After(signalKillTimeout):
		log.Println("Killing command due to timeout.")
		if err := process.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			log.Printf("Error killing command: %s\n", err)
		}
	}
}
