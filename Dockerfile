FROM golang:1.13.7-alpine3.11 AS binary
RUN apk -U add openssl git

ADD . /go/src/github.com/jwilder/dockerize
WORKDIR /go/src/github.com/jwilder/dockerize

RUN go get github.com/robfig/glock
RUN glock sync -n < GLOCKFILE
RUN go install

FROM alpine:3.11
MAINTAINER Jason Wilder <mail@jasonwilder.com>

COPY --from=binary /go/bin/dockerize /usr/local/bin

ENTRYPOINT ["dockerize"]
CMD ["--help"]
