.SILENT :
.PHONY : shuttle fmt test dist-clean

TAG :=`git describe --tags`

VERSION = `git describe --tags 2>/dev/null || git rev-parse --short HEAD 2>/dev/null`
LDFLAGS = -X main.buildVersion=$(VERSION)

all: shuttle

deps:
	glock sync github.com/litl/shuttle

shuttle:
	echo "Building shuttle"
	go install -x -ldflags "$(LDFLAGS)" github.com/litl/shuttle

fmt:
	go fmt github.com/litl/shuttle/...

test:
	go test -v github.com/litl/shuttle

dist-clean:
	rm -rf dist
	rm -f shuttle-*.tar.gz

dist-init:
	mkdir -p dist/$$GOOS/$$GOARCH

dist-build: dist-init
	echo "Compiling $$GOOS/$$GOARCH"
	go build -a -ldflags "$(LDFLAGS)" -o dist/$$GOOS/$$GOARCH/shuttle github.com/litl/shuttle

dist-linux-amd64:
	export GOOS="linux"; \
	export GOARCH="amd64"; \
	$(MAKE) dist-build

dist-linux-386:
	export GOOS="linux"; \
	export GOARCH="386"; \
	$(MAKE) dist-build

dist-darwin-amd64:
	export GOOS="darwin"; \
	export GOARCH="amd64"; \
	$(MAKE) dist-build

dist: dist-clean dist-init dist-linux-amd64 dist-linux-386 dist-darwin-amd64

release-tarball:
	echo "Building $$GOOS-$$GOARCH-$(TAG).tar.gz"
	GZIP=-9 tar -cvzf shuttle-$$GOOS-$$GOARCH-$(TAG).tar.gz -C dist/$$GOOS/$$GOARCH shuttle >/dev/null 2>&1

release-linux-amd64:
	export GOOS="linux"; \
	export GOARCH="amd64"; \
	$(MAKE) release-tarball

release-linux-386:
	export GOOS="linux"; \
	export GOARCH="386"; \
	$(MAKE) release-tarball

release-darwin-amd64:
	export GOOS="darwin"; \
	export GOARCH="amd64"; \
	$(MAKE) release-tarball

release: deps dist release-linux-amd64 release-linux-386 release-darwin-amd64
