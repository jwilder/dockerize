dockerize ![version v0.6.1](https://img.shields.io/badge/version-v0.6.1-brightgreen.svg) ![License MIT](https://img.shields.io/badge/license-MIT-blue.svg)
=============

Utility to simplify running applications in docker containers.

dockerize is a utility to simplify running applications in docker containers.  It allows you to:
* generate application configuration files at container startup time from templates and container environment variables
* Tail multiple log files to stdout and/or stderr
* Wait for other services to be available using TCP, HTTP(S), unix before starting the main process.

The typical use case for dockerize is when you have an application that has one or more configuration files and you would like to control some of the values using environment variables.

For example, a Python application using Sqlalchemy might not be able to use environment variables directly.
It may require that the database URL be read from a python settings file with a variable named
`SQLALCHEMY_DATABASE_URI`.  dockerize allows you to set an environment variable such as
`DATABASE_URL` and update the python file when the container starts.
In addition, it can also delay the starting of the python application until the database container is running and listening on the TCP port.

Another use case is when the application logs to specific files on the filesystem and not stdout
or stderr. This makes it difficult to troubleshoot the container using the `docker logs` command.
For example, nginx will log to `/var/log/nginx/access.log` and
`/var/log/nginx/error.log` by default. While you can sometimes work around this, it's tedious to find a solution for every application. dockerize allows you to specify which logs files should be tailed and where they should be sent.

See [A Simple Way To Dockerize Applications](http://jasonwilder.com/blog/2014/10/13/a-simple-way-to-dockerize-applications/)


## Installation

Download the latest version in your container:

* [linux/amd64](https://github.com/jwilder/dockerize/releases/download/v0.6.1/dockerize-linux-amd64-v0.6.1.tar.gz)
* [alpine/amd64](https://github.com/jwilder/dockerize/releases/download/v0.6.1/dockerize-alpine-linux-amd64-v0.6.1.tar.gz)
* [darwin/amd64](https://github.com/jwilder/dockerize/releases/download/v0.6.1/dockerize-darwin-amd64-v0.6.1.tar.gz)


### Docker Base Image

The `jwilder/dockerize` image is a base image based on `alpine linux`.  `dockerize` is installed in the `$PATH` and can be used directly.

```
FROM jwilder/dockerize
...
ENTRYPOINT dockerize ...
```

### Ubuntu Images

``` Dockerfile
RUN apt-get update && apt-get install -y wget

ENV DOCKERIZE_VERSION v0.6.1
RUN wget https://github.com/jwilder/dockerize/releases/download/$DOCKERIZE_VERSION/dockerize-linux-amd64-$DOCKERIZE_VERSION.tar.gz \
    && tar -C /usr/local/bin -xzvf dockerize-linux-amd64-$DOCKERIZE_VERSION.tar.gz \
    && rm dockerize-linux-amd64-$DOCKERIZE_VERSION.tar.gz
```


### For Alpine Images:

``` Dockerfile
RUN apk add --no-cache openssl

ENV DOCKERIZE_VERSION v0.6.1
RUN wget https://github.com/jwilder/dockerize/releases/download/$DOCKERIZE_VERSION/dockerize-alpine-linux-amd64-$DOCKERIZE_VERSION.tar.gz \
    && tar -C /usr/local/bin -xzvf dockerize-alpine-linux-amd64-$DOCKERIZE_VERSION.tar.gz \
    && rm dockerize-alpine-linux-amd64-$DOCKERIZE_VERSION.tar.gz
```

## Usage

dockerize works by wrapping the call to your application using the `ENTRYPOINT` or `CMD` directives.

This would generate `/etc/nginx/nginx.conf` from the template located at `/etc/nginx/nginx.tmpl` and
send `/var/log/nginx/access.log` to `STDOUT` and `/var/log/nginx/error.log` to `STDERR` after running
`nginx`, only after waiting for the `web` host to respond on `tcp 8000`:

``` Dockerfile
CMD dockerize -template /etc/nginx/nginx.tmpl:/etc/nginx/nginx.conf -stdout /var/log/nginx/access.log -stderr /var/log/nginx/error.log -wait tcp://web:8000 nginx
```

### Command-line Options

You can specify multiple templates by passing using `-template` multiple times:

```
$ dockerize -template template1.tmpl:file1.cfg -template template2.tmpl:file3

```

Templates can be generated to `STDOUT` by not specifying a dest:

```
$ dockerize -template template1.tmpl

```

Template may also be a directory. In this case all files within this directory are processed as template and stored with the same name in the destination directory.
If the destination directory is omitted, the output is sent to `STDOUT`. The files in the source directory are processed in sorted order (as returned by `ioutil.ReadDir`).

```
$ dockerize -template src_dir:dest_dir

```

If the destination file already exists, dockerize will overwrite it. The -no-overwrite flag overrides this behaviour.

```
$ dockerize -no-overwrite -template template1.tmpl:file
```

You can tail multiple files to `STDOUT` and `STDERR` by passing the options multiple times.

```
$ dockerize -stdout info.log -stdout perf.log

```

If `inotify` does not work in you container, you use `-poll` to poll for file changes instead.

```
$ dockerize -stdout info.log -stdout perf.log -poll

```

If your file uses `{{` and `}}` as part of it's syntax, you can change the template escape characters using the `-delims`.

```
$ dockerize -delims "<%:%>"
```

Http headers can be specified for http/https protocols.

```
$ dockerize -wait http://web:80 -wait-http-header "Authorization:Basic QWxhZGRpbjpvcGVuIHNlc2FtZQ=="
```

## Waiting for other dependencies

It is common when using tools like [Docker Compose](https://docs.docker.com/compose/) to depend on services in other linked containers, however oftentimes relying on [links](https://docs.docker.com/compose/compose-file/#links) is not enough - whilst the container itself may have _started_, the _service(s)_ within it may not yet be ready - resulting in shell script hacks to work around race conditions.

Dockerize gives you the ability to wait for services on a specified protocol (`file`, `tcp`, `tcp4`, `tcp6`, `http`, `https` and `unix`) before starting your application:

```
$ dockerize -wait tcp://db:5432 -wait http://web:80 -wait file:///tmp/generated-file
```

### Timeout

You can optionally specify how long to wait for the services to become available by using the `-timeout #` argument (Default: 10 seconds).  If the timeout is reached and the service is still not available, the process exits with status code 1.

```
$ dockerize -wait tcp://db:5432 -wait http://web:80 -timeout 10s
```

See [this issue](https://github.com/docker/compose/issues/374#issuecomment-126312313) for a deeper discussion, and why support isn't and won't be available in the Docker ecosystem itself.

## Using Templates

Templates use Golang [text/template](http://golang.org/pkg/text/template/). You can access environment
variables within a template with `.Env`.

```
{{ .Env.PATH }} is my path
```

There are a few built in functions as well:

  * `default $var $default` - Returns a default value for one that does not exist. `{{ default .Env.VERSION "0.1.2" }}`
  * `contains $map $key` - Returns true if a string is within another string
  * `exists $path` - Determines if a file path exists or not. `{{ exists "/etc/default/myapp" }}`
  * `split $string $sep` - Splits a string into an array using a separator string. Alias for [`strings.Split`][go.string.Split]. `{{ split .Env.PATH ":" }}`
  * `replace $string $old $new $count` - Replaces all occurrences of a string within another string. Alias for [`strings.Replace`][go.string.Replace]. `{{ replace .Env.PATH ":" }}`
  * `parseUrl $url` - Parses a URL into it's [protocol, scheme, host, etc. parts][go.url.URL]. Alias for [`url.Parse`][go.url.Parse]
  * `atoi $value` - Parses a string $value into an int. `{{ if (gt (atoi .Env.NUM_THREADS) 1) }}`
  * `add $arg1 $arg` - Performs integer addition. `{{ add (atoi .Env.SHARD_NUM) -1 }}`
  * `isTrue $value` - Parses a string $value to a boolean value. `{{ if isTrue .Env.ENABLED }}`
  * `lower $value` - Lowercase a string.
  * `upper $value` - Uppercase a string.
  * `jsonQuery $json $query` - Returns the result of a selection query against a json document.
  * `loop` - Create for loops.

### jsonQuery

Objects and fields are accessed by name. Array elements are accessed by index in square brackets (e.g. `[1]`). Nested elements are separated by dots (`.`).

**Examples:**

With the following JSON in `.Env.SERVICES`

```
{
  "services": [
    {
      "name": "service1",
      "port": 8000,
    },{
      "name": "service2",
      "port": 9000,
    }
  ]
}
```

the template expression `jsonQuery .Env.SERVICES "services.[1].port"` returns `9000`.

### loop

`loop` allows for creating for loop within a template.  It takes 1 to 3 arguments.

```
# Loop from 0...10
{{ range loop 10 }}
i = {{ . }}
{{ end }}

# Loop from 5...10
{{ range $i := loop 5 10 }}
i = {{ $i }}
{{ end }}

# Loop from 5...10 by 2
{{ range $i := loop 5 10 2 }}
i = {{ $i }}
{{ end }}
```

## License

MIT


[go.string.Split]: https://golang.org/pkg/strings/#Split
[go.string.Replace]: https://golang.org/pkg/strings/#Replace
[go.url.Parse]: https://golang.org/pkg/net/url/#Parse
[go.url.URL]: https://golang.org/pkg/net/url/#URL
