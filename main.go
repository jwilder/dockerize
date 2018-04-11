package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/context"
	"crypto/tls"
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
	wg           sync.WaitGroup

	headersFlag       sliceVar
	statusCodesFlag	  sliceVar
	headers           []HttpHeader
	urls              []url.URL
	waitFlag          hostFlagsVar
	waitRetryInterval time.Duration
	waitTimeoutFlag   time.Duration
	dependencyChan    chan struct{}
	skipRedirectFlag  bool
	skipHttpVerifyFlag bool

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

					if skipHttpVerifyFlag {
						client.Transport =&http.Transport{
							TLSClientConfig: &tls.Config{InsecureSkipVerify : true},
						}
					}

					if skipRedirectFlag {
						client.CheckRedirect= func(req *http.Request, via []*http.Request) error {
							return http.ErrUseLastResponse
						}
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
						} else if (len(statusCodesFlag) > 0) {
							for _, code := range statusCodesFlag {
								if code == strconv.Itoa(resp.StatusCode) {
									log.Printf("Received %d from %s\n", resp.StatusCode, u.String())
									return
								}
							}
							log.Printf("Received %d from %s. Sleeping %s\n", resp.StatusCode, u.String(), waitRetryInterval)
							time.Sleep(waitRetryInterval)
						}	else if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
							log.Printf("Received %d from %s\n", resp.StatusCode, u.String())
							return
						} else {
							log.Printf("Received %d from %s. Sleeping %s\n", resp.StatusCode, u.String(), waitRetryInterval)
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
				return
			}
		}
	}()
}

func usage() {
	println(`Usage: waiter [options] [command]

Utility to wait until a condition is present.
Options:`)
	flag.PrintDefaults()

	println(`
Arguments:
  command - command to be executed
  `)

}

func main() {

	flag.BoolVar(&version, "version", false, "show version")
	flag.Var(&headersFlag, "header", "HTTP headers, colon separated. e.g \"Accept-Encoding: gzip\". Can be passed multiple times")
	flag.Var(&statusCodesFlag, "status-code", "HTTP code to wait for e.g. \"-status-code 302  -status-code 200\". Can be passed multiple times. (If not specified -wait returns on 200 >= x < 300) ")
	flag.BoolVar(&skipRedirectFlag, "skip-redirect", false, "Skip HTTP redirects")
	flag.BoolVar(&skipHttpVerifyFlag, "skip-verify", false, "Skip SSL certificate verification")
	flag.Var(&waitFlag, "wait", "Host (tcp/tcp4/tcp6/http/https/unix/file) to wait for before this container starts. Can be passed multiple times. e.g. tcp://db:5432")
	flag.DurationVar(&waitTimeoutFlag, "timeout", 10*time.Second, "Host wait timeout")
	flag.DurationVar(&waitRetryInterval, "interval", defaultWaitRetryInterval, "Duration to wait before retrying")

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
			log.Fatalf("-header \"%s\" provided with no -wait option", h)
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

	waitForDependencies()

	// Setup context
	ctx, cancel = context.WithCancel(context.Background())

	if flag.NArg() > 0 {
		wg.Add(1)
		go runCmd(ctx, cancel, flag.Arg(0), flag.Args()[1:]...)
	}



	wg.Wait()
}
