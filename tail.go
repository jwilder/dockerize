package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ActiveState/tail"
	"golang.org/x/net/context"
)

func tailFile(ctx context.Context, file string, dest *os.File) {
	defer wg.Done()
	t, err := tail.TailFile(file, tail.Config{
		Follow: true,
		ReOpen: true,
		//Poll:   true,
		Logger: tail.DiscardingLogger,
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
				tail.Cleanup()
				return
			// get the next log line and echo it out
			case line := <-t.Lines:
				fmt.Fprintln(dest, line.Text)
		}
	}
}
