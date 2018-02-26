GOOS_LINUX        := linux
GOOS_WINDOWS      := windows
GOOS_DARWIN       := darwin
OUT_LINUX         := metavisor-linux
OUT_DARWIN        := metavisor-darwin
OUT_WINDOWS       := metavisor-windows.exe
ARCH              := amd64

ifeq ($(OS),Windows_NT)
	GO_OS := $(GOOS_WINDOWS)
else
	HOST_OS := $(shell uname)
	ifeq ($(HOST_OS), Darwin)
		GO_OS := $(GOOS_DARWIN)
	else
		GO_OS := $(GOOS_LINUX)
	endif
endif

%-linux : override GO_OS = $(GOOS_LINUX)
%-darwin : override GO_OS = $(GOOS_DARWIN)
%-windows : override GO_OS = $(GOOS_WINDOWS)

all: build

deps:
	dep ensure

build: deps
	GOOS=$(GO_OS) GOARCH=$(ARCH) go build cmd/metavisor.go

build-all:
	@$(MAKE) build-linux
	mv metavisor $(OUT_LINUX)
	@$(MAKE) build-windows
	mv metavisor.exe $(OUT_WINDOWS)
	@$(MAKE) build-darwin
	mv metavisor $(OUT_DARWIN)

build-linux: build
build-darwin: build
build-windows: build

docker-build-img:
	docker build --build-arg GOOS=$(GO_OS) -t metavisor-cli .

docker-build: docker-build-img
	docker rm metavisor-cli-build-$(GO_OS) ||:
	docker create --name metavisor-cli-build-$(GO_OS) metavisor-cli
	docker cp metavisor-cli-build-$(GO_OS):/app/metavisor ./metavisor
	docker rm metavisor-cli-build-$(GO_OS)

docker-build-all:
	@$(MAKE) docker-build-linux
	mv metavisor $(OUT_LINUX)
	@$(MAKE) docker-build-windows
	mv metavisor.exe $(OUT_WINDOWS)
	@$(MAKE) docker-build-darwin
	mv metavisor $(OUT_DARWIN)

docker-build-linux: docker-build-img docker-build
docker-build-darwin: docker-build-img docker-build
docker-build-windows: docker-build-img docker-build
	mv metavisor metavisor.exe
