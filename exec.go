package main

import (
	"log"
	"os"
	"os/exec"
	"syscall"
)

func runCmd(cmd string, args ...string) {
	defer wg.Done()

	//FIXME: forward signals
	process := exec.Command(cmd, args...)
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr

	// start the process
	err := process.Start()
	if err != nil {
		log.Fatalf("error starting command: %s, %s\n", cmd, err)
	}

	// wait for process to finish
	err = process.Wait()
	if err == nil {
		// command completed with exit status 0
		os.Exit(0)
	} else {
		// command failed, exit with the same code
		log.Printf("error running command: %s, %s\n", cmd, err)
		os.Exit(err.(*exec.ExitError).Sys().(syscall.WaitStatus).ExitStatus())
	}
}
