FROM golang:1.19.1-alpine3.16 AS binary
RUN apk --no-cache --update add openssl git

WORKDIR /go/src/github.com/jwilder/dockerize
COPY *.go go.* /go/src/github.com/jwilder/dockerize/

ENV GO111MODULE=on
ENV CGO_ENABLED=0

RUN --mount=type=cache,mode=0777,target=/go/pkg/mod \
    --mount=type=cache,mode=0777,target=/root/.cache/build \
    go mod tidy && \
    go install

FROM alpine:3.16.2
LABEL MAINTAINER="Jason Wilder <mail@jasonwilder.com>"

COPY --from=binary /go/bin/dockerize /usr/local/bin

ENTRYPOINT ["dockerize"]
CMD ["--help"]
