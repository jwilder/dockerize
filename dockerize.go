package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"golang.org/x/net/context"
)

type sliceVar []string

type Context struct {
}

func (c *Context) Env() map[string]string {
	env := make(map[string]string)
	for _, i := range os.Environ() {
		sep := strings.Index(i, "=")
		env[i[0:sep]] = i[sep+1:]
	}
	return env
}

var (
	buildVersion string
	version      bool
	wg           sync.WaitGroup

	templatesFlag  sliceVar
	stdoutTailFlag sliceVar
	stderrTailFlag sliceVar
	delimsFlag     string
	delims         []string

	ctx context.Context
	cancel context.CancelFunc
)

func (s *sliceVar) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func (s *sliceVar) String() string {
	return strings.Join(*s, ",")
}

func main() {

	flag.BoolVar(&version, "version", false, "show version")
	flag.Var(&templatesFlag, "template", "Template (/template:/dest). Can be passed multiple times")
	flag.Var(&stdoutTailFlag, "stdout", "Tails a file to stdout. Can be passed multiple times")
	flag.Var(&stderrTailFlag, "stderr", "Tails a file to stderr. Can be passed multiple times")
	flag.StringVar(&delimsFlag, "delims", "", `template tag delimiters. default "{{":"}}" `)

	flag.Parse()

	if version {
		fmt.Println(buildVersion)
		return
	}

	if flag.NArg() == 0 {
		log.Fatalln("no command specified")
	}

	if delimsFlag != "" {
		delims = strings.Split(delimsFlag, ":")
		if len(delims) != 2 {
			log.Fatalf("bad delimiters argument: %s. expected \"left:right\"", delimsFlag)
		}
	}
	for _, t := range templatesFlag {
		parts := strings.Split(t, ":")
		if len(parts) != 2 {
			log.Fatalf("bad template argument: %s. expected \"/template:/dest\"", t)
		}
		generateFile(parts[0], parts[1])
	}

	// Setup context
	ctx, cancel = context.WithCancel(context.Background())

	wg.Add(1)
	go runCmd(ctx, cancel, flag.Arg(0), flag.Args()[1:]...)

	for _, out := range stdoutTailFlag {
		wg.Add(1)
		go tailFile(ctx, out, os.Stdout)
	}

	for _, err := range stderrTailFlag {
		wg.Add(1)
		go tailFile(ctx, err, os.Stderr)
	}

	wg.Wait()
}
