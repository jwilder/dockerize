FROM golang:1.25.7 AS binary

WORKDIR /go/src/github.com/jwilder/dockerize
COPY *.go go.* /go/src/github.com/jwilder/dockerize/

ARG VERSION=dev
ENV GO111MODULE=on
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags "-X main.buildVersion=${VERSION}" -a -o /go/bin/dockerize .

FROM gcr.io/distroless/static:nonroot
LABEL maintainer="Jason Wilder <mail@jasonwilder.com>"

USER nonroot:nonroot
COPY --from=binary /go/bin/dockerize /bin/dockerize

ENTRYPOINT ["/bin/dockerize"]
CMD ["--help"]
