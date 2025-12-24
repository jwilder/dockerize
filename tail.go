package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hpcloud/tail"
	"golang.org/x/net/context"
)

func tailFile(ctx context.Context, file string, poll bool, dest *os.File) {
	defer wg.Done()

	var isPipe bool
	var errCount int

	s, err := os.Stat(file)
	if err != nil {
		log.Printf("Warning: unable to stat %s: %s", file, err)
		errCount++
		isPipe = false
	} else {
		isPipe = s.Mode()&os.ModeNamedPipe != 0
	}

	t, err := tail.TailFile(file, tail.Config{
		Follow: true,
		ReOpen: true,
		Poll:   poll,
		Logger: tail.DiscardingLogger,
		Pipe:   isPipe,
	})
	if err != nil {
		log.Fatalf("unable to tail %s: %s", file, err)
	}

	defer func() {
		t.Stop()
		t.Cleanup()
	}()

	// main loop
	for {
		select {
		// if the channel is done, then exit the loop
		case <-ctx.Done():
			return
		// get the next log line and echo it out
		case line := <-t.Lines:
			if line == nil {
				// Check if there's an actual error
				if err := t.Err(); err != nil {
					log.Printf("Warning: unable to tail %s: %s", file, err)
					errCount++
					if errCount > 30 {
						log.Fatalf("Logged %d consecutive errors while tailing. Exiting", errCount)
					}
					time.Sleep(2 * time.Second) // Sleep for 2 seconds before retrying
					continue
				}
				return
			} else {
				fmt.Fprintln(dest, line.Text)
				errCount = 0 // Zero the error count
			}
		}
	}
}
