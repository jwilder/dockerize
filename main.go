// Dockerize is a utility for rendering templates from environment variables,
// waiting for dependencies to become available, tailing log files, and then
// executing a command.
package main

import (
	"context"
	"errors"
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
)

const defaultWaitRetryInterval = time.Second

type sliceVar []string
type hostFlagsVar []string

// Context is the data passed to templates when they are rendered. Its methods
// are callable from templates, for example {{ .Env.HOME }}.
type Context struct {
}

// HttpHeader is a name and value pair sent with the HTTP requests made when
// waiting on a URL, set with -wait-http-header.
type HttpHeader struct {
	name  string
	value string
}

// Config holds the options parsed from the command-line flags.
type Config struct {
	version           bool
	poll              bool
	templates         []string
	stdoutTails       []string
	stderrTails       []string
	headersFlag       []string
	delims            []string
	headers           []HttpHeader
	urls              []url.URL
	waits             []string
	waitTimeout       time.Duration
	waitRetryInterval time.Duration
	noOverwrite       bool
	args              []string
}

// Env returns the current process environment as a map of variable names to
// values. It is used as .Env in templates.
func (c *Context) Env() map[string]string {
	return envMap(os.Environ())
}

// envMap parses environment entries of the form NAME=value into a map. Entries
// without an '=' are skipped; the value keeps everything after the first '='.
func envMap(environ []string) map[string]string {
	env := make(map[string]string, len(environ))
	for _, i := range environ {
		key, value, found := strings.Cut(i, "=")
		if !found {
			continue
		}
		env[key] = value
	}
	return env
}

var (
	buildVersion string
	version      bool
	poll         bool
	wg           sync.WaitGroup

	templatesFlag     sliceVar
	stdoutTailFlag    sliceVar
	stderrTailFlag    sliceVar
	headersFlag       sliceVar
	delimsFlag        string
	delims            []string
	headers           []HttpHeader
	urls              []url.URL
	waitFlag          hostFlagsVar
	waitRetryInterval time.Duration
	waitTimeoutFlag   time.Duration
	noOverwriteFlag   bool
	dialTimeout       = net.DialTimeout

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

// drainBody reads any remaining data from a response body and closes it.
// Errors are deliberately ignored since this is cleanup-only.
func drainBody(body io.ReadCloser) {
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}

func waitForDependencies() {
	dependencyChan := make(chan struct{})

	go func() {
		for _, u := range urls {
			log.Println("Waiting for:", u.String())

			switch u.Scheme {
			case "file":
				waitForFile(u)
			case "tcp", "tcp4", "tcp6":
				waitForSocket(u.Scheme, u.Host, waitTimeoutFlag)
			case "unix":
				waitForSocket(u.Scheme, u.Path, waitTimeoutFlag)
			case "http", "https":
				waitForHTTP(u)
			default:
				log.Fatalf("invalid host protocol provided: %s. supported protocols are: %s", u.Scheme, supportedWaitSchemes)
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

func waitForFile(u url.URL) {
	wg.Go(func() {
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
			} else if errors.Is(err, os.ErrNotExist) {
				continue
			} else {
				log.Printf("Problem with check file %s exist: %v. Sleeping %s\n", u.String(), err.Error(), waitRetryInterval)
			}
		}
	})
}

func waitForHTTP(u url.URL) {
	wg.Go(func() {
		client := &http.Client{
			Timeout: waitTimeoutFlag,
		}

		for {
			req, err := http.NewRequest("GET", u.String(), nil)
			if err != nil {
				log.Printf("Problem creating request for %s: %v. Sleeping %s\n", u.String(), err.Error(), waitRetryInterval)
				time.Sleep(waitRetryInterval)
				continue
			}
			if len(headers) > 0 {
				for _, header := range headers {
					req.Header.Add(header.name, header.value)
				}
			}

			resp, err := client.Do(req)
			if err != nil {
				log.Printf("Problem with request to %s: %s. Sleeping %s\n", u.String(), err.Error(), waitRetryInterval)
				time.Sleep(waitRetryInterval)
			} else if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
				log.Printf("Received %d from %s\n", resp.StatusCode, u.String())
				drainBody(resp.Body)
				return
			} else {
				log.Printf("Received %d from %s. Sleeping %s\n", resp.StatusCode, u.String(), waitRetryInterval)
				drainBody(resp.Body)
				time.Sleep(waitRetryInterval)
			}
		}
	})
}

func waitForSocket(scheme, addr string, timeout time.Duration) {
	wg.Go(func() {
		for {
			conn, err := dialTimeout(scheme, addr, timeout)
			if err != nil {
				log.Printf("Problem with dial %s://%s: %v. Sleeping %s\n", scheme, addr, err.Error(), waitRetryInterval)
				time.Sleep(waitRetryInterval)
			}
			if conn != nil {
				log.Printf("Connected to %s://%s\n", scheme, addr)
				_ = conn.Close()
				return
			}
		}
	})
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

func registerFlags() {
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
	flag.Usage = usage
}

func parseDelimiters(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}

	delims := strings.Split(value, ":")
	if len(delims) != 2 {
		return nil, fmt.Errorf("bad delimiters argument: %s. expected \"left:right\"", value)
	}
	if delims[0] == "" || delims[1] == "" {
		return nil, fmt.Errorf("invalid -delims \"%s\": both delimiters must be non-empty, expected \"left:right\"", value)
	}
	return delims, nil
}

// validateTemplateSpec rejects a "template:" spec with an empty destination.
// processTemplates would otherwise write it to stdout without saying so.
func validateTemplateSpec(spec string) error {
	_, dest, found := strings.Cut(spec, ":")
	if found && dest == "" {
		return fmt.Errorf("invalid -template \"%s\": missing destination after ':', drop the ':' to write to stdout", spec)
	}
	return nil
}

// supportedWaitSchemes lists the -wait URL schemes waitForDependencies handles.
const supportedWaitSchemes = "file, tcp, tcp4, tcp6, unix, http, https"

func parseWaitURLs(hosts hostFlagsVar) ([]url.URL, error) {
	urls := make([]url.URL, 0, len(hosts))
	for _, host := range hosts {
		u, err := url.Parse(host)
		if err != nil {
			return nil, fmt.Errorf("bad hostname provided: %s. %s", host, err.Error())
		}
		if err := validateWaitURL(u); err != nil {
			return nil, fmt.Errorf("invalid -wait \"%s\": %s", host, err.Error())
		}
		urls = append(urls, *u)
	}
	return urls, nil
}

// validateWaitURL checks that a -wait URL has the parts its scheme needs, so a
// malformed URL fails at startup instead of waiting on the wrong target.
func validateWaitURL(u *url.URL) error {
	switch u.Scheme {
	case "file", "unix":
		if u.Opaque != "" || u.Host != "" {
			return fmt.Errorf("path must be absolute, use %s:///abs/path", u.Scheme)
		}
		if !strings.HasPrefix(u.Path, "/") {
			return fmt.Errorf("missing absolute path, use %s:///abs/path", u.Scheme)
		}
	case "tcp", "tcp4", "tcp6":
		if u.Hostname() == "" {
			return fmt.Errorf("missing host, expected %s://host:port", u.Scheme)
		}
		if u.Port() == "" {
			return fmt.Errorf("missing port, expected %s://host:port", u.Scheme)
		}
	case "http", "https":
		if u.Hostname() == "" {
			return fmt.Errorf("missing host, expected %s://host[:port][/path]", u.Scheme)
		}
	default:
		return fmt.Errorf("unsupported scheme %q, supported schemes are: %s", u.Scheme, supportedWaitSchemes)
	}
	return nil
}

func parseHeaders(values []string, waits hostFlagsVar) ([]HttpHeader, error) {
	headers := make([]HttpHeader, 0, len(values))
	for _, h := range values {
		if len(waits) == 0 {
			return nil, fmt.Errorf("-wait-http-header \"%s\" provided with no -wait option", h)
		}

		const errMsg = "bad HTTP Headers argument: %s. expected \"headerName: headerValue\""
		if strings.Contains(h, ":") {
			parts := strings.SplitN(h, ":", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf(errMsg, h)
			}
			name := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if !validHeaderName(name) {
				return nil, fmt.Errorf("invalid -wait-http-header %q: header name must be a non-empty HTTP token", h)
			}
			if !validHeaderValue(value) {
				return nil, fmt.Errorf("invalid -wait-http-header %q: header value contains a control character", h)
			}
			headers = append(headers, HttpHeader{name: name, value: value})
			continue
		}
		return nil, fmt.Errorf(errMsg, h)
	}
	return headers, nil
}

// validHeaderName reports whether name is a non-empty RFC 7230 token.
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		isAlnum := ('0' <= c && c <= '9') || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
		if !isAlnum && strings.IndexByte(headerNameTokenChars, c) < 0 {
			return false
		}
	}
	return true
}

// validHeaderValue reports whether value has no control characters other than tab.
func validHeaderValue(value string) bool {
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < 0x20 && c != '\t') || c == 0x7f {
			return false
		}
	}
	return true
}

// headerNameTokenChars are the non-alphanumeric characters allowed in an HTTP token.
const headerNameTokenChars = "!#$%&'*+-.^_`|~"

func parseConfigFromFlags() (Config, error) {
	parsedDelims, err := parseDelimiters(delimsFlag)
	if err != nil {
		return Config{}, err
	}

	for _, spec := range templatesFlag {
		if err := validateTemplateSpec(spec); err != nil {
			return Config{}, err
		}
	}

	parsedURLs, err := parseWaitURLs(waitFlag)
	if err != nil {
		return Config{}, err
	}

	parsedHeaders, err := parseHeaders(headersFlag, waitFlag)
	if err != nil {
		return Config{}, err
	}

	if waitRetryInterval <= 0 {
		return Config{}, fmt.Errorf("-wait-retry-interval must be positive, got %s", waitRetryInterval)
	}

	if len(waitFlag) > 0 && waitTimeoutFlag <= 0 {
		return Config{}, fmt.Errorf("-timeout must be positive when -wait is used, got %s", waitTimeoutFlag)
	}

	// Flags with no effect without a companion option are errors, not silently
	// ignored. The duration flags have defaults, so they are checked with
	// flag.Visit rather than by comparing against the default value.
	explicit := make(map[string]bool)
	flag.Visit(func(f *flag.Flag) { explicit[f.Name] = true })

	if explicit["timeout"] && len(waitFlag) == 0 {
		return Config{}, fmt.Errorf("-timeout %s provided with no -wait option", waitTimeoutFlag)
	}
	if explicit["wait-retry-interval"] && len(waitFlag) == 0 {
		return Config{}, fmt.Errorf("-wait-retry-interval %s provided with no -wait option", waitRetryInterval)
	}
	if poll && len(stdoutTailFlag)+len(stderrTailFlag) == 0 {
		return Config{}, fmt.Errorf("-poll provided with no -stdout or -stderr option")
	}
	if noOverwriteFlag && len(templatesFlag) == 0 {
		return Config{}, fmt.Errorf("-no-overwrite provided with no -template option")
	}
	if delimsFlag != "" && len(templatesFlag) == 0 {
		return Config{}, fmt.Errorf("-delims \"%s\" provided with no -template option", delimsFlag)
	}

	return Config{
		version:           version,
		poll:              poll,
		templates:         templatesFlag,
		stdoutTails:       stdoutTailFlag,
		stderrTails:       stderrTailFlag,
		headersFlag:       headersFlag,
		delims:            parsedDelims,
		headers:           parsedHeaders,
		urls:              parsedURLs,
		waits:             waitFlag,
		waitTimeout:       waitTimeoutFlag,
		waitRetryInterval: waitRetryInterval,
		noOverwrite:       noOverwriteFlag,
		args:              flag.Args(),
	}, nil
}

func processTemplates(templates []string) {
	for _, t := range templates {
		template, dest := t, ""
		if strings.Contains(t, ":") {
			parts := strings.SplitN(t, ":", 2)
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
}

func startCommand(ctx context.Context, cancel context.CancelFunc) {
	if flag.NArg() > 0 {
		wg.Add(1)
		go runCmd(ctx, cancel, flag.Arg(0), flag.Args()[1:]...)
	}
}

func startTailers(ctx context.Context) {
	for _, out := range stdoutTailFlag {
		wg.Add(1)
		go tailFile(ctx, out, poll, os.Stdout)
	}

	for _, err := range stderrTailFlag {
		wg.Add(1)
		go tailFile(ctx, err, poll, os.Stderr)
	}
}

func main() {
	registerFlags()
	flag.Parse()

	config, err := parseConfigFromFlags()
	if err != nil {
		log.Fatal(err)
	}

	delims = config.delims
	headers = config.headers
	urls = config.urls
	waitFlag = config.waits
	waitTimeoutFlag = config.waitTimeout
	waitRetryInterval = config.waitRetryInterval
	noOverwriteFlag = config.noOverwrite
	poll = config.poll

	if config.version {
		fmt.Println(buildVersion)
		return
	}

	if flag.NArg() == 0 && flag.NFlag() == 0 {
		usage()
		os.Exit(1)
	}

	processTemplates(config.templates)
	waitForDependencies()

	ctx, cancel = context.WithCancel(context.Background())
	startCommand(ctx, cancel)
	startTailers(ctx)

	wg.Wait()
}
