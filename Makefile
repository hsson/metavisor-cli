GOOS_LINUX        := linux
GOOS_WINDOWS      := windows
GOOS_DARWIN       := darwin
ARCH              := amd64

deps:
	dep ensure

build: deps
	go build cmd/metavisor.go

build-all: deps build-linux build-darwin build-windows

build-linux: deps
	GOOS=$(GOOS_LINUX) GOARCH=$(ARCH) go build -o metavisor-linux cmd/metavisor.go

build-darwin: deps
	GOOS=$(GOOS_DARWIN) GOARCH=$(ARCH) go build -o metavisor-darwin cmd/metavisor.go

build-windows: deps
	GOOS=$(GOOS_WINDOWS) GOARCH=$(ARCH) go build -o metavisor-windows.exe cmd/metavisor.go