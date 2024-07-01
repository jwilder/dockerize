package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
)

const defaultWaitRetryInterval = time.Second

type sliceVar []string
type hostFlagsVar []string

type Context struct {
}

type HttpHeader struct {
	name  string
	value string
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
	poll         bool
	wg           sync.WaitGroup

	templatesFlag      sliceVar
	templateDirsFlag   sliceVar
	stdoutTailFlag     sliceVar
	stderrTailFlag     sliceVar
	headersFlag        sliceVar
	delimsFlag         string
	delims             []string
	headers            []HttpHeader
	urls               []url.URL
	waitFlag           hostFlagsVar
	waitRetryInterval  time.Duration
	waitTimeoutFlag    time.Duration
	expectResponseCode int
	dependencyChan     chan struct{}
	noOverwriteFlag    bool

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

	go func() {
		for _, u := range urls {
			log.Println("Waiting for:", u.String())

			switch u.Scheme {
			case "file":
				wg.Add(1)
				go func(u url.URL) {
					defer wg.Done()
					ticker := time.NewTicker(waitRetryInterval)
					defer ticker.Stop()
					var err error
					if _, err = os.Stat(u.Path); err == nil {
						log.Printf("File %s had been generated\n", u.String())
						return
					}
					for range ticker.C {
						if _, err = os.Stat(u.Path); err == nil {
							log.Printf("File %s had been generated\n", u.String())
							return
						} else if os.IsNotExist(err) {
							continue
						} else {
							log.Printf("Problem with check file %s exist: %v. Sleeping %s\n", u.String(), err.Error(), waitRetryInterval)

						}
					}
				}(u)
			case "tcp", "tcp4", "tcp6":
				waitForSocket(u.Scheme, u.Host, waitTimeoutFlag)
			case "unix":
				waitForSocket(u.Scheme, u.Path, waitTimeoutFlag)
			case "http", "https":
				wg.Add(1)
				go func(u url.URL) {
					client := &http.Client{
						Timeout: waitTimeoutFlag,
					}

					defer wg.Done()
					for {
						req, err := http.NewRequest("GET", u.String(), nil)
						if err != nil {
							log.Printf("Problem with dial: %v. Sleeping %s\n", err.Error(), waitRetryInterval)
							time.Sleep(waitRetryInterval)
						}
						if len(headers) > 0 {
							for _, header := range headers {
								req.Header.Add(header.name, header.value)
							}
						}

						resp, err := client.Do(req)
						if err != nil {
							log.Printf("Problem with request: %s. Sleeping %s\n", err.Error(), waitRetryInterval)
							time.Sleep(waitRetryInterval)
						} else if resp.StatusCode == expectResponseCode {
							log.Printf("Received user expected response code %d from %s\n", resp.StatusCode, u.String())
							return
						} else if err == nil && expectResponseCode == 0 && resp.StatusCode >= 200 && resp.StatusCode < 300 {
							log.Printf("Received %d from %s\n", resp.StatusCode, u.String())
							// dispose the response body and close it.
							io.Copy(io.Discard, resp.Body)
							resp.Body.Close()
							return
						} else {
							log.Printf("Received %d from %s. Sleeping %s\n", resp.StatusCode, u.String(), waitRetryInterval)
							io.Copy(io.Discard, resp.Body)
							resp.Body.Close()
							time.Sleep(waitRetryInterval)
						}
					}
				}(u)
			default:
				log.Fatalf("invalid host protocol provided: %s. supported protocols are: tcp, tcp4, tcp6 and http", u.Scheme)
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

func waitForSocket(scheme, addr string, timeout time.Duration) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := net.DialTimeout(scheme, addr, waitTimeoutFlag)
			if err != nil {
				log.Printf("Problem with dial: %v. Sleeping %s\n", err.Error(), waitRetryInterval)
				time.Sleep(waitRetryInterval)
			}
			if conn != nil {
				log.Printf("Connected to %s://%s\n", scheme, addr)
				conn.Close()
				return
			}
		}
	}()
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
             -stdout /var/log/nginx/access.log \
             -stderr /var/log/nginx/error.log \
             -wait tcp://web:8000 nginx
	`)

	println(`For more information, see https://github.com/jwilder/dockerize`)
}

func main() {

	flag.BoolVar(&version, "version", false, "show version")
	flag.BoolVar(&poll, "poll", false, "enable polling")

	flag.Var(&templatesFlag, "template", "Template (/template:/dest). Can be passed multiple times. Does also support directories")
	flag.BoolVar(&noOverwriteFlag, "no-overwrite", false, "Do not overwrite destination file if it already exists.")
	flag.Var(&stdoutTailFlag, "stdout", "Tails a file to stdout. Can be passed multiple times")
	flag.Var(&stderrTailFlag, "stderr", "Tails a file to stderr. Can be passed multiple times")
	flag.StringVar(&delimsFlag, "delims", "", `template tag delimiters. default "{{":"}}" `)
	flag.Var(&headersFlag, "wait-http-header", "HTTP headers, colon separated. e.g \"Accept-Encoding: gzip\". Can be passed multiple times")
	flag.Var(&waitFlag, "wait", "Host (tcp/tcp4/tcp6/http/https/unix/file) to wait for before this container starts. Can be passed multiple times. e.g. tcp://db:5432")
	flag.DurationVar(&waitTimeoutFlag, "timeout", 10*time.Second, "Host wait timeout")
	flag.DurationVar(&waitRetryInterval, "wait-retry-interval", defaultWaitRetryInterval, "Duration to wait before retrying")
	flag.IntVar(&expectResponseCode, "expect-response-code", 0, "Http response code expected")

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

	for _, host := range waitFlag {
		u, err := url.Parse(host)
		if err != nil {
			log.Fatalf("bad hostname provided: %s. %s", host, err.Error())
		}
		urls = append(urls, *u)
	}

	for _, h := range headersFlag {
		//validate headers need -wait options
		if len(waitFlag) == 0 {
			log.Fatalf("-wait-http-header \"%s\" provided with no -wait option", h)
		}

		const errMsg = "bad HTTP Headers argument: %s. expected \"headerName: headerValue\""
		if strings.Contains(h, ":") {
			parts := strings.Split(h, ":")
			if len(parts) != 2 {
				log.Fatalf(errMsg, headersFlag)
			}
			headers = append(headers, HttpHeader{name: strings.TrimSpace(parts[0]), value: strings.TrimSpace(parts[1])})
		} else {
			log.Fatalf(errMsg, headersFlag)
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

		fi, err := os.Stat(template)
		if err != nil {
			log.Fatalf("unable to stat %s, error: %s", template, err)
		}
		if fi.IsDir() {
			generateDir(template, dest)
		} else {
			generateFile(template, dest)
		}
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
