FROM golang:1.21.1 AS binary

WORKDIR /go/src/github.com/jwilder/dockerize
COPY *.go go.* /go/src/github.com/jwilder/dockerize/

ENV GO111MODULE=on
RUN go mod tidy

RUN CGO_ENABLED=0 GOOS=linux GO111MODULE=on go build -a -o /go/bin/dockerize .

FROM gcr.io/distroless/static:nonroot
LABEL MAINTAINER="Jason Wilder <mail@jasonwilder.com>"

USER nonroot:nonroot
COPY --from=binary /go/bin/dockerize /bin/dockerize

ENTRYPOINT ["/bin/dockerize"]
CMD ["--help"]
