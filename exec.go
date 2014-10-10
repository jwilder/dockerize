package main

import (
	"log"
	"os"
	"os/exec"
)

func runCmd(cmd string, args ...string) {

	//FIXME: forward signals
	process := exec.Command(cmd, args...)
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	err := process.Run()
	if err != nil {
		log.Fatalf("error running command: %s, %s\n", cmd, err)
	}
}
