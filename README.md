dockerize
=============

Utility to simplify running applications in docker containers.

dockerize is a utility to simplify running applications in docker containers.  It allows you
to generate application configuration files at container startup time from templates and
container environment variables.  It also allows log files to be tailed to stdout and/or
stderr.

The typical use case for dockerize is when you have an application that has one or more
configuration files and you would like to control some of the values using environment variables.

For example, a Python application using Sqlalchemy may be able to use environment variables directly.
It may require that the database URL be read from a python settings file with a variable named
`SQLALCHEMY_DATABASE_URI`.  dockerize allows you to set an environment variable such as
`DATABASE_URL` and update the python file when the container starts.

Another use case is when the application logs to specific files on the filesystem and not stdout
or stderr.  This makes it difficult to troubleshoot the container using the `docker logs` command.
For example, nginx will log to `/var/log/nginx/access.log' and
'/var/log/nginx/error.log' by default.  While you can sometimes work around this, it's tedious to find
the a solution for every application.  dockerize allows you to specify which logs files should
be tailed and where they should be sent.

See [A Simple Way To Dockerize Applications](http://jasonwilder.com/blog/2014/10/13/a-simple-way-to-dockerize-applications/)

## Installation

Download the latest version in your container:

* [linux/amd64](https://github.com/jwilder/dockerize/releases/download/v0.0.2/dockerize-linux-amd64-v0.0.2.tar.gz)

For Ubuntu Images:

```
RUN apt-get update && apt-get install -y wget
RUN wget https://github.com/jwilder/dockerize/releases/download/v0.0.2/dockerize-linux-amd64-v0.0.2.tar.gz
RUN tar -C /usr/local/bin -xzvf dockerize-linux-amd64-v0.0.2.tar.gz
```

## Usage

dockerize works by wrapping the call to your application using the `ENTRYPOINT` or `CMD` directives.

This would generate `/etc/nginx/nginx.conf` from the template located at `/etc/nginx/nginx.tmpl` and
send `/var/log/nginx/access.log' to `STDOUT` and `/var/log/nginx/error.log` to `STDERR` after running
`nginx`.

```
CMD dockerize -template /etc/nginx/nginx.tmpl:/etc/nginx/nginx.conf -stdout /var/log/nginx/access.log -stderr /var/log/nginx/error.log nginx
```

### Command-line Options

You can specify multiple template by passing using `-template` multiple times:

```
$ dockerize -template template1.tmpl:file1.cfg -template template2.tmpl:file3

```

You can tail multiple files to `STDOUT` and `STDERR` by passing the options multiple times.

```
$ dockerize -stdout info.log -stdout perf.log

```

If your file uses `{{` and `}}` as part of it's syntax, you can change the template escape characters using the `-delims`.

```
$ dockerize -delims "<%:%>"
```

## Using Templates

Templates use Golang [text/template](http://golang.org/pkg/text/template/). You can access environment
variables within a template with `.Env`.

```
{{ .Env.PATH }} is my path
```

There are a few built in functions as well:

  * `default` - Returns a default value for one that does not exist
  * `contains` - Returns true if a string is within another string
  * `exists` - Determines if a file path exists or not
  * `split` - Splits a string into an array using a separator string
  * `replace` - Replaces all occurences of a string within another string
  * `parseUrl`- Parses a URL into it's protocol, scheme, host, etc. parts.

## License

MIT
