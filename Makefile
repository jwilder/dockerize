.SILENT :
.PHONY : waiter clean fmt

TAG:=`git describe --abbrev=0 --tags`
LDFLAGS:=-X main.buildVersion=$(TAG)

all: waiter

deps:
	go get github.com/robfig/glock
	glock sync -n < GLOCKFILE

waiter:
	echo "Building waiter"
	go install -ldflags "$(LDFLAGS)"

dist-clean:
	rm -rf dist
	rm -f waiter-*.tar.gz

dist: deps dist-clean
	mkdir -p dist/alpine-linux/amd64 && GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -a -tags netgo -installsuffix netgo -o dist/alpine-linux/amd64/waiter
	mkdir -p dist/linux/amd64 && GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/linux/amd64/waiter
	mkdir -p dist/linux/386 && GOOS=linux GOARCH=386 go build -ldflags "$(LDFLAGS)" -o dist/linux/386/waiter
	mkdir -p dist/linux/armel && GOOS=linux GOARCH=arm GOARM=5 go build -ldflags "$(LDFLAGS)" -o dist/linux/armel/waiter
	mkdir -p dist/linux/armhf && GOOS=linux GOARCH=arm GOARM=6 go build -ldflags "$(LDFLAGS)" -o dist/linux/armhf/waiter
	mkdir -p dist/darwin/amd64 && GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/darwin/amd64/waiter

release: dist
	tar -cvzf waiter-alpine-linux-amd64-$(TAG).tar.gz -C dist/alpine-linux/amd64 waiter
	tar -cvzf waiter-linux-amd64-$(TAG).tar.gz -C dist/linux/amd64 waiter
	tar -cvzf waiter-linux-386-$(TAG).tar.gz -C dist/linux/386 waiter
	tar -cvzf waiter-linux-armel-$(TAG).tar.gz -C dist/linux/armel waiter
	tar -cvzf waiter-linux-armhf-$(TAG).tar.gz -C dist/linux/armhf waiter
	tar -cvzf waiter-darwin-amd64-$(TAG).tar.gz -C dist/darwin/amd64 waiter
