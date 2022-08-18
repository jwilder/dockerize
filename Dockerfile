FROM golang:1.18-alpine3.16 AS binary
RUN apk --no-cache --update add openssl git

WORKDIR /go/src/github.com/jwilder/dockerize
COPY *.go go.* /go/src/github.com/jwilder/dockerize/

ENV GO111MODULE=on
RUN go mod tidy
RUN go install

FROM alpine:3.16
LABEL MAINTAINER="Jason Wilder <mail@jasonwilder.com>"

COPY --from=binary /go/bin/dockerize /usr/local/bin

ENTRYPOINT ["dockerize"]
CMD ["--help"]
