package main

import (
	"fmt"
	"log"
	"os"

	"github.com/hpcloud/tail"
	"golang.org/x/net/context"
)

func tailFile(ctx context.Context, file string, poll bool, dest *os.File, end bool) {
	defer wg.Done()
	seek := os.SEEK_SET
	if end {
		seek = os.SEEK_END
	}
	t, err := tail.TailFile(file, tail.Config{
		Follow: true,
		ReOpen: true,
		Poll:   poll,
		Logger: tail.DiscardingLogger,
		Location: &tail.SeekInfo{-0, seek},
	})
	if err != nil {
		log.Fatalf("unable to tail %s: %s", "foo", err)
	}

	// main loop
	for {
		select {
		// if the channel is done, then exit the loop
		case <-ctx.Done():
			t.Stop()
			t.Cleanup()
			return
		// get the next log line and echo it out
		case line := <-t.Lines:
			if line != nil {
				fmt.Fprintln(dest, line.Text)
			}
		}
	}
}
