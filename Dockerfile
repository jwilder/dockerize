FROM golang:1.27.3 AS binary

WORKDIR /go/src/github.com/jwilder/dockerize
COPY *.go go.* /go/src/github.com/jwilder/dockerize/

ARG VERSION=dev
ENV GO111MODULE=on
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -mod=readonly -ldflags "-X main.buildVersion=${VERSION}" -a -o /go/bin/dockerize .

FROM gcr.io/distroless/static:nonroot
LABEL maintainer="Jason Wilder <mail@jasonwilder.com>"

USER nonroot:nonroot
COPY --from=binary /go/bin/dockerize /bin/dockerize

ENTRYPOINT ["/bin/dockerize"]
CMD ["--help"]
