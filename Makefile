.SILENT :
.PHONY : dockerize clean fmt

TAG:=`git describe --abbrev=0 --tags`
LDFLAGS:=-X main.buildVersion=$(TAG)
GO111MODULE:=on

all: dockerize

deps:
	go mod tidy

dockerize:
	echo "Building dockerize"
	go install -ldflags "$(LDFLAGS)"

dist-clean:
	rm -rf dist
	rm -f dockerize-*.tar.gz

dist: deps dist-clean
	mkdir -p dist/alpine-linux/amd64 && GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -a -tags netgo -installsuffix netgo -o dist/alpine-linux/amd64/dockerize
	mkdir -p dist/alpine-linux/ppc64le && GOOS=linux GOARCH=ppc64le go build -ldflags "$(LDFLAGS)" -a -tags netgo -installsuffix netgo -o dist/alpine-linux/ppc64le/dockerize
	mkdir -p dist/linux/amd64 && GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/linux/amd64/dockerize
	mkdir -p dist/linux/386 && GOOS=linux GOARCH=386 go build -ldflags "$(LDFLAGS)" -o dist/linux/386/dockerize
	mkdir -p dist/linux/armel && GOOS=linux GOARCH=arm GOARM=5 go build -ldflags "$(LDFLAGS)" -o dist/linux/armel/dockerize
	mkdir -p dist/linux/armhf && GOOS=linux GOARCH=arm GOARM=6 go build -ldflags "$(LDFLAGS)" -o dist/linux/armhf/dockerize
	mkdir -p dist/linux/arm64 && GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/linux/arm64/dockerize
	mkdir -p dist/linux/ppc64le && GOOS=linux GOARCH=ppc64le go build -ldflags "$(LDFLAGS)" -o dist/linux/ppc64le/dockerize
	mkdir -p dist/darwin/amd64 && GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/darwin/amd64/dockerize
	mkdir -p dist/darwin/amd64 && GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/darwin/arm64/dockerize
	mkdir -p dist/linux/s390x && GOOS=linux GOARCH=s390x go build -ldflags "$(LDFLAGS)" -o dist/linux/s390x/dockerize

release: dist
	tar -cvzf dockerize-alpine-linux-amd64-$(TAG).tar.gz -C dist/alpine-linux/amd64 dockerize
	tar -cvzf dockerize-alpine-linux-ppc64le-$(TAG).tar.gz -C dist/alpine-linux/ppc64le dockerize
	tar -cvzf dockerize-linux-amd64-$(TAG).tar.gz -C dist/linux/amd64 dockerize
	tar -cvzf dockerize-linux-386-$(TAG).tar.gz -C dist/linux/386 dockerize
	tar -cvzf dockerize-linux-armel-$(TAG).tar.gz -C dist/linux/armel dockerize
	tar -cvzf dockerize-linux-armhf-$(TAG).tar.gz -C dist/linux/armhf dockerize
	tar -cvzf dockerize-linux-arm64-$(TAG).tar.gz -C dist/linux/arm64 dockerize
	tar -cvzf dockerize-linux-ppc64le-$(TAG).tar.gz -C dist/linux/ppc64le dockerize
	tar -cvzf dockerize-darwin-amd64-$(TAG).tar.gz -C dist/darwin/amd64 dockerize
	tar -cvzf dockerize-darwin-arm64-$(TAG).tar.gz -C dist/darwin/arm64 dockerize
	tar -cvzf dockerize-linux-s390x-$(TAG).tar.gz -C dist/linux/s390x dockerize
