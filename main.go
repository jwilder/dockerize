package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
	"gopkg.in/ini.v1"
)

const defaultWaitRetryInterval = time.Second

type sliceVar []string
type hostFlagsVar []string

// Context is the type passed into the template renderer
type Context struct {
}

// HTTPHeader this is an optional header passed on http checks
type HTTPHeader struct {
	name  string
	value string
}

// Env is bound to the template rendering Context and returns the
// environment variables passed to the program
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

	envFlag           string
	envSection        string
	envHdrFlag        sliceVar
	validateCert      bool
	templatesFlag     sliceVar
	templateDirsFlag  sliceVar
	stdoutTailFlag    sliceVar
	stderrTailFlag    sliceVar
	headersFlag       sliceVar
	delimsFlag        string
	delims            []string
	headers           []HTTPHeader
	urls              []url.URL
	waitFlag          hostFlagsVar
	waitRetryInterval time.Duration
	waitTimeoutFlag   time.Duration
	dependencyChan    chan struct{}
	noOverwriteFlag   bool
	eUID              int
	eGID              int

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
					var tr = http.DefaultTransport
					if !validateCert {
						tr = &http.Transport{
							TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
						}
					}
					client := &http.Client{
						Timeout:   waitTimeoutFlag,
						Transport: tr,
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
						} else if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
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

func getINI(envFlag string, envHdrFlag []string) (iniFile []byte, err error) {

	// See if envFlag parses like an absolute URL, if so use http, otherwise treat as filename
	url, urlERR := url.ParseRequestURI(envFlag)
	if urlERR == nil && url.IsAbs() {
		var resp *http.Response
		var req *http.Request
		var hdr string
		var client *http.Client
		var tr = http.DefaultTransport
		// Define redirect handler to disallow redirects
		var redir = func(req *http.Request, via []*http.Request) error {
			return errors.New("Redirects disallowed")
		}

		if !validateCert {
			tr = &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
		}
		client = &http.Client{Transport: tr, CheckRedirect: redir}
		req, err = http.NewRequest("GET", envFlag, nil)
		if err != nil {
			// Weird problem with declaring client, bail
			return
		}
		// Handle headers for request - are they headers or filepaths?
		for _, h := range envHdrFlag {
			if strings.Contains(h, ":") {
				// This will break if path includes colon - don't use colons in path!
				hdr = h
			} else { // Treat this is a path to a secrets file containing header
				var hdrFile []byte
				hdrFile, err = ioutil.ReadFile(h)
				if err != nil { // Could not read file, error out
					return
				}
				hdr = string(hdrFile)
			}
			parts := strings.Split(hdr, ":")
			if len(parts) != 2 {
				log.Fatalf("Bad env-headers argument: %s. expected \"headerName: headerValue\"", hdr)
			}
			req.Header.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
		resp, err = client.Do(req)
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			iniFile, err = ioutil.ReadAll(resp.Body)
		} else if err == nil { // Request completed with unexpected HTTP status code, bail
			err = errors.New(resp.Status)
			return
		}
	} else {
		iniFile, err = ioutil.ReadFile(envFlag)
	}
	return
}

func main() {

	flag.BoolVar(&version, "version", false, "show version")
	flag.BoolVar(&poll, "poll", false, "enable polling")
	flag.StringVar(&envFlag, "env", "", "Optional path to INI file for injecting env vars. Does not overwrite existing env vars")
	flag.StringVar(&envSection, "env-section", "", "Optional section of INI file to use for loading env vars. Defaults to \"\"")
	flag.Var(&envHdrFlag, "env-header", "Optional string or path to secrets file for http headers passed if -env is a URL")
	flag.BoolVar(&validateCert, "validate-cert", true, "Verify SSL certs for https connections")
	flag.IntVar(&eGID, "egid", -1, "Set the numeric group ID for the running program") // Check for -1 later to skip
	flag.IntVar(&eUID, "euid", -1, "Set the numeric user id for the running program")
	flag.Var(&templatesFlag, "template", "Template (/template:/dest). Can be passed multiple times. Does also support directories")
	flag.BoolVar(&noOverwriteFlag, "no-overwrite", false, "Do not overwrite destination file if it already exists.")
	flag.Var(&stdoutTailFlag, "stdout", "Tails a file to stdout. Can be passed multiple times")
	flag.Var(&stderrTailFlag, "stderr", "Tails a file to stderr. Can be passed multiple times")
	flag.StringVar(&delimsFlag, "delims", "", `template tag delimiters. default "{{":"}}" `)
	flag.Var(&headersFlag, "wait-http-header", "HTTP headers, colon separated. e.g \"Accept-Encoding: gzip\". Can be passed multiple times")
	flag.Var(&waitFlag, "wait", "Host (tcp/tcp4/tcp6/http/https/unix/file) to wait for before this container starts. Can be passed multiple times. e.g. tcp://db:5432")
	flag.DurationVar(&waitTimeoutFlag, "timeout", 10*time.Second, "Host wait timeout")
	flag.DurationVar(&waitRetryInterval, "wait-retry-interval", defaultWaitRetryInterval, "Duration to wait before retrying")

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

	if envFlag != "" {
		iniFile, err := getINI(envFlag, envHdrFlag)
		if err != nil {
			log.Fatalf("unreadable INI file %s: %s", envFlag, err)
		}
		cfg, err := ini.LoadSources(ini.LoadOptions{}, iniFile)
		if err != nil {
			log.Fatalf("error parsing contents of %s as INI format: %s", envFlag, err)
		}
		envHash := cfg.Section(envSection).KeysHash()

		for k, v := range envHash {
			if _, ok := os.LookupEnv(k); !ok {
				// log.Printf("Setting %s to %s", k, v)
				os.Setenv(k, v)
			}
		}
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
			headers = append(headers, HTTPHeader{name: strings.TrimSpace(parts[0]), value: strings.TrimSpace(parts[1])})
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
		// Drop privs if passed the euid or egid params
		if eGID >= 0 {
			err := Setgid(eGID)
			if err != nil {
				log.Fatalf("Error while setting GID to %d: %s", eGID, err)
			}
		}

		if eUID >= 0 {
			err := Setuid(eUID)
			if err != nil {
				log.Fatalf("Error while setting UID to %d: %s", eUID, err)
			}
		}

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
