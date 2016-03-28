package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
)

type sliceVar []string
type hostFlagsVar []string

var (
	buildVersion string
	version      bool
	poll         bool
	wg           sync.WaitGroup

	templatesFlag   sliceVar
	stdoutTailFlag  sliceVar
	stderrTailFlag  sliceVar
	overlaysFlag    sliceVar
	delimsFlag      string
	delims          []string
	waitFlag        hostFlagsVar
	waitTimeoutFlag time.Duration
	dependencyChan  chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
)

func (i *hostFlagsVar) String() string {
	return fmt.Sprint(*i)
}

func (i *hostFlagsVar) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func (s *sliceVar) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func (s *sliceVar) String() string {
	return strings.Join(*s, ",")
}

func waitForDependencies() {
	dependencyChan := make(chan struct{})

	if waitFlag == nil {
		return
	}

	go func() {
		for _, host := range waitFlag {
			log.Println("Waiting for host:", host)
			u, err := url.Parse(host)
			if err != nil {
				log.Fatalf("bad hostname provided: %s. %s", host, err.Error())
			}

			switch u.Scheme {
			case "tcp", "tcp4", "tcp6":
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						conn, _ := net.DialTimeout(u.Scheme, u.Host, waitTimeoutFlag)
						if conn != nil {
							log.Println("Connected to", u.String())
							return
						}
					}
				}()
			case "http", "https":
				wg.Add(1)
				go func() {
					defer wg.Done()
					for {
						resp, err := http.Get(u.String())
						if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
							log.Printf("Received %d from %s\n", resp.StatusCode, u.String())
							return
						}
					}
				}()
			default:
				log.Fatalf("invalid host protocol provided: %s. supported protocols are: tcp, tcp4, tcp6, http and https", u.Scheme)
			}
		}
		wg.Wait()
		close(dependencyChan)
	}()

	select {
	case <-dependencyChan:
		break
	case <-time.After(waitTimeoutFlag):
		log.Fatalf("Timeout after %s waiting on dependencies to become available: %v", waitTimeoutFlag, waitFlag)
	}

}

func usage() {
	println(`Usage: dockerize [options] [command]

Utility to simplify running applications in docker containers

Options:`)
	flag.PrintDefaults()

	println(`
Arguments:
  command - command to be executed
  `)

	println(`Examples:
`)
	println(`   Generate /etc/nginx/nginx.conf using nginx.tmpl as a template, tail /var/log/nginx/access.log
   and /var/log/nginx/error.log, waiting for a website to become available on port 8000 and start nginx.`)
	println(`
   dockerize -template nginx.tmpl:/etc/nginx/nginx.conf \
   	     -overlay overlays/_common/html:/usr/share/nginx/ \
   	     -overlay overlays/{{ .Env.DEPLOYMENT_ENV }}/html:/usr/share/nginx/ \`)
	println(`   	     -stdout /var/log/nginx/access.log \
             -stderr /var/log/nginx/error.log \
             -wait tcp://web:8000 nginx
	`)

	println(`For more information, see https://github.com/jwilder/dockerize`)
}

func main() {

	flag.BoolVar(&version, "version", false, "show version")
	flag.BoolVar(&poll, "poll", false, "enable polling")
	flag.Var(&templatesFlag, "template", "Template (/template:/dest). Can be passed multiple times")
	flag.Var(&overlaysFlag, "overlay", "overlay (/src:/dest). Can be passed multiple times")
	flag.Var(&stdoutTailFlag, "stdout", "Tails a file to stdout. Can be passed multiple times")
	flag.Var(&stderrTailFlag, "stderr", "Tails a file to stderr. Can be passed multiple times")
	flag.StringVar(&delimsFlag, "delims", "", `template tag delimiters. default "{{":"}}" `)
	flag.Var(&waitFlag, "wait", "Host (tcp/tcp4/tcp6/http/https) to wait for before this container starts. Can be passed multiple times. e.g. tcp://db:5432")
	flag.DurationVar(&waitTimeoutFlag, "timeout", 10*time.Second, "Host wait timeout")

	flag.Usage = usage
	flag.Parse()

	if version {
		fmt.Println(buildVersion)
		return
	}

	if flag.NArg() == 0 && flag.NFlag() == 0 {
		usage()
		os.Exit(1)
	}

	if delimsFlag != "" {
		delims = strings.Split(delimsFlag, ":")
		if len(delims) != 2 {
			log.Fatalf("bad delimiters argument: %s. expected \"left:right\"", delimsFlag)
		}
	}

	// Overlay files from src --> dst
	for _, o := range overlaysFlag {
		if strings.Contains(o, ":") {
			parts := strings.Split(o, ":")
			if len(parts) != 2 {
				log.Fatalf("bad overlay argument: '%s'. expected \"/src:/dest\"", o)
			}
			src, dest := string_template_eval(parts[0]), string_template_eval(parts[1])

			log.Printf("overlaying %s --> %s", src, dest)

			cmd := exec.Command("cp", "-rv", src, dest)
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			if err := cmd.Run(); err != nil {
				log.Fatal(err)
			}
		}
	}

	for _, t := range templatesFlag {
		template, dest := t, ""
		if strings.Contains(t, ":") {
			parts := strings.Split(t, ":")
			if len(parts) != 2 {
				log.Fatalf("bad template argument: %s. expected \"/template:/dest\"", t)
			}
			template, dest = string_template_eval(parts[0]), string_template_eval(parts[1])
		}
		generateFile(template, dest)
	}

	waitForDependencies()

	// Setup context
	ctx, cancel = context.WithCancel(context.Background())

	if flag.NArg() > 0 {
		wg.Add(1)
		go runCmd(ctx, cancel, flag.Arg(0), flag.Args()[1:]...)
	}

	for _, out := range stdoutTailFlag {
		wg.Add(1)
		go tailFile(ctx, out, poll, os.Stdout)
	}

	for _, err := range stderrTailFlag {
		wg.Add(1)
		go tailFile(ctx, err, poll, os.Stderr)
	}

	wg.Wait()
}
