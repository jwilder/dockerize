waiter ![version v0.1.0](https://img.shields.io/badge/version-v0.1.0-brightgreen.svg) ![License MIT](https://img.shields.io/badge/license-MIT-blue.svg)
=============

Utility to wait for services to be available from (TCP, HTTP(s), file, socket) - Most of the code is courtesy of [jwilder/dockerize](https://github.com/jwilder/dockerize)


```shell
Usage: waiter [options] [command]

Utility to wait until a condition is present.
Options:
  -header value
      HTTP headers, colon separated. e.g "Accept-Encoding: gzip". Can be passed multiple times
  -interval duration
      Duration to wait before retrying (default 1s)
  -skip-redirect
      Skip HTTP redirects
  -skip-verify
      Skip SSL certificate verification
  -status-code value
      HTTP code to wait for e.g. "-status-code 302  -status-code 200". Can be passed multiple times. (If not specified -wait returns on 200 >= x < 300)
  -timeout duration
      Host wait timeout (default 10s)
  -version
      show version
  -wait value
      Host (tcp/tcp4/tcp6/http/https/unix/file) to wait for before this container starts. Can be passed multiple times. e.g. tcp://db:5432
```

## Installation

Download the latest version in your container:

* [linux/amd64](https://github.com/moshloop/waiter/releases/download/v0.1.0/waiter-linux-amd64-v0.1.0.tar.gz)
* [alpine/amd64](https://github.com/moshloop/waiter/releases/download/v0.1.0/waiter-alpine-linux-amd64-v0.1.0.tar.gz)
* [darwin/amd64](https://github.com/moshloop/waiter/releases/download/v0.1.0/waiter-darwin-amd64-v0.1.0.tar.gz)


## Waiting for other dependencies

It is common when using tools like [Docker Compose](https://docs.docker.com/compose/) to depend on services in other linked containers, however oftentimes relying on [links](https://docs.docker.com/compose/compose-file/#links) is not enough - whilst the container itself may have _started_, the _service(s)_ within it may not yet be ready - resulting in shell script hacks to work around race conditions.

waiter gives you the ability to wait for services on a specified protocol (`file`, `tcp`, `tcp4`, `tcp6`, `http`, `https` and `unix`) before starting your application:

```
$ waiter -wait tcp://db:5432 -wait http://web:80 -wait file:///tmp/generated-file
```


Http headers can be specified for http/https protocols:

```
$ waiter -wait http://web:80 -wait-http-header "Authorization:Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ=="
```

### Timeout

You can optionally specify how long to wait for the services to become available by using the `-timeout #` argument (Default: 10 seconds).  If the timeout is reached and the service is still not available, the process exits with status code 1.

```
$ waiter -wait tcp://db:5432 -wait http://web:80 -timeout 10s
```

See [this issue](https://github.com/docker/compose/issues/374#issuecomment-126312313) for a deeper discussion, and why support isn't and won't be available in the Docker ecosystem itself.

## License

MIT


[go.string.Split]: https://golang.org/pkg/strings/#Split
[go.string.Replace]: https://golang.org/pkg/strings/#Replace
[go.url.Parse]: https://golang.org/pkg/net/url/#Parse
[go.url.URL]: https://golang.org/pkg/net/url/#URL
