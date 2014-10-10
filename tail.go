package main

import (
	"fmt"
	"log"
	"os"

	"github.com/ActiveState/tail"
)

func tailFile(file string, dest *os.File) {
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
	for line := range t.Lines {
		fmt.Fprintln(dest, line.Text)
	}
}
