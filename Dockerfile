FROM golang:1.11-alpine3.7 AS binary
RUN apk -U add openssl git

ADD . /src
WORKDIR /src

RUN CGO_ENABLED=0 go install -ldflags "-X 'main.buildVersion=$(git describe --abbrev=0 --tags)'"

FROM alpine:3.7
MAINTAINER Jason Wilder <mail@jasonwilder.com>

COPY --from=binary /go/bin/dockerize /usr/local/bin

ENTRYPOINT ["dockerize"]
CMD ["--help"]
