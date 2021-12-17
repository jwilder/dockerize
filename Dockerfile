FROM golang:1.17-alpine3.15 AS binary
RUN apk --no-cache --update add openssl git

WORKDIR /go/src/github.com/jwilder/dockerize
COPY *.go go.* /go/src/github.com/jwilder/dockerize/

ENV GO111MODULE=on
RUN go mod tidy
RUN go install

FROM alpine:3.15
LABEL MAINTAINER="Jason Wilder <mail@jasonwilder.com>"

COPY --from=binary /go/bin/dockerize /usr/local/bin

ENTRYPOINT ["dockerize"]
CMD ["--help"]
