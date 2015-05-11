package main

import (
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/context"
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

	wg.Add(1)
	go func(){
		defer wg.Done()

		// Setup signaling
		sigs := make(chan os.Signal)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT)

		select {
			case sig := <-sigs:
				log.Printf("Received signal: %s\n", sig)
				ctx, _ = context.WithTimeout(context.Background(), 10 * time.Second)
				signalProcessWithTimeout(ctx, process, sig)
				cancel()
			case <-ctx.Done():
				// exit when context is done
		}
	}()

	err = process.Wait()
	cancel()
	
	if err == nil {
		log.Println("Command finished successfully.")
	} else {
		log.Printf("Command exited with error: %s\n", err)
		// OPTIMIZE: This could be cleaner
		os.Exit(err.(*exec.ExitError).Sys().(syscall.WaitStatus).ExitStatus())
	}

}

func signalProcessWithTimeout(ctx context.Context, process *exec.Cmd, sig os.Signal) {
	process.Process.Signal(sig) // pretty sure this doesn't do anything. It seems like the signal is automatically sent to the command?
	process.Wait()
	select {
	case <-ctx.Done():
    log.Println("Killing command due to timeout.")
    process.Process.Kill()
	}
}
