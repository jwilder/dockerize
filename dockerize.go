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

	ctx    context.Context
	cancel context.CancelFunc
)

func (s *sliceVar) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func (s *sliceVar) String() string {
	return strings.Join(*s, ",")
}

func usage() {
	println(`Usage: dockerize [options] [command]

Utility to simplify running applications in docker containers

Options:`)
	flag.PrintDefaults()

	println(`
Arguments:
  command - command to executed
  `)

	println(`Examples:
`)
	println(`   Generate /etc/nginx/nginx.conf using nginx.tmpl as a template, tail /var/log/nginx/access.log
   and /var/log/nginx/error.log and start nginx.`)
	println(`
   dockerize -template nginx.tmpl:/etc/nginx/nginx.conf \
             -stdout /var/log/nginx/access.log \
             -stderr /var/log/nginx/error.log nginx
	`)

	println(`For more information, see https://github.com/jwilder/dockerize`)
}
func main() {

	flag.BoolVar(&version, "version", false, "show version")
	flag.Var(&templatesFlag, "template", "Template (/template:/dest). Can be passed multiple times")
	flag.Var(&stdoutTailFlag, "stdout", "Tails a file to stdout. Can be passed multiple times")
	flag.Var(&stderrTailFlag, "stderr", "Tails a file to stderr. Can be passed multiple times")
	flag.StringVar(&delimsFlag, "delims", "", `template tag delimiters. default "{{":"}}" `)

	flag.Usage = usage
	flag.Parse()

	if version {
		fmt.Println(buildVersion)
		return
	}

	if flag.NArg() == 0 {
		usage()
		os.Exit(1)
	}

	if delimsFlag != "" {
		delims = strings.Split(delimsFlag, ":")
		if len(delims) != 2 {
			log.Fatalf("bad delimiters argument: %s. expected \"left:right\"", delimsFlag)
		}
	}
	for _, t := range templatesFlag {
		template, dest := t, ""
		if strings.Contains(t, ":") {
			parts := strings.Split(t, ":")
			if len(parts) != 2 {
				log.Fatalf("bad template argument: %s. expected \"/template:/dest\"", t)
			}
			template, dest = parts[0], parts[1]
		}
		generateFile(template, dest)
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
