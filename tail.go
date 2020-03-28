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
	const maxErr = 30
	const sleepDur = 2 * time.Second

	s, err := os.Stat(file)
	if err != nil {
		log.Printf("Warning: unable to stat %s: %s", file, err)
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
			if line.Err != nil || (line == nil && t.Err() != nil) {
				log.Printf("Warning: unable to tail %s: %s", file, t.Err())
				errCount++
				if errCount > maxErr {
					log.Fatalf("Logged %d consecutive errors while tailing. Exiting", errCount)
				}
				time.Sleep(sleepDur)
				continue
			} else if line == nil {
				return
			}
			fmt.Fprintln(dest, line.Text)
			errCount = 0 // Zero the error count
		}
	}
}
