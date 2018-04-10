#    Copyright 2018 Immutable Systems, Inc.
# 
#    Licensed under the Apache License, Version 2.0 (the "License");
#    you may not use this file except in compliance with the License.
#    You may obtain a copy of the License at
# 
#        http://www.apache.org/licenses/LICENSE-2.0
# 
#    Unless required by applicable law or agreed to in writing, software
#    distributed under the License is distributed on an "AS IS" BASIS,
#    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#    See the License for the specific language governing permissions and
#    limitations under the License.

GOOS_LINUX        := linux
GOOS_WINDOWS      := windows
GOOS_FREEBSD      := freebsd
GOOS_OPENBSD      := openbsd
GOOS_DARWIN       := darwin
OUT_LINUX         := metavisor-linux
OUT_DARWIN        := metavisor-darwin
OUT_FREEBSD       := metavisor-freebsd
OUT_OPENBSD       := metavisor-openbsd
OUT_WINDOWS       := metavisor-windows.exe
ARCH              := amd64

ifeq ($(OS),Windows_NT)
	GO_OS := $(GOOS_WINDOWS)
else
	HOST_OS := $(shell uname)
	ifeq ($(HOST_OS), Darwin)
		GO_OS := $(GOOS_DARWIN)
	else ifeq ($(HOST_OS), FreeBSD)
		GO_OS := $(GOOS_FREEBSD)
	else ifeq ($(HOST_OS), OpenBSD)
		GO_OS := $(GOOS_OPENBSD)
	else
		GO_OS := $(GOOS_LINUX)
	endif
endif

%-linux : override GO_OS = $(GOOS_LINUX)
%-darwin : override GO_OS = $(GOOS_DARWIN)
%-windows : override GO_OS = $(GOOS_WINDOWS)
%-freebsd : override GO_OS = $(GOOS_FREEBSD)
%-openbsd : override GO_OS = $(GOOS_OPENBSD)

all: build

test: deps
	go test -race ./...

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
	@$(MAKE) build-freebsd
	mv metavisor $(OUT_FREEBSD)
	@$(MAKE) build-openbsd
	mv metavisor $(OUT_OPENBSD)

build-linux: build
build-darwin: build
build-windows: build
build-freebsd: build
build-openbsd: build

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
	@$(MAKE) docker-build-freebsd
	mv metavisor $(OUT_FREEBSD)
	@$(MAKE) docker-build-openbsd
	mv metavisor $(OUT_OPENBSD)

docker-build-linux: docker-build-img docker-build
docker-build-darwin: docker-build-img docker-build
docker-build-freebsd: docker-build-img docker-build
docker-build-openbsd: docker-build-img docker-build
docker-build-windows: docker-build-img docker-build
	mv metavisor metavisor.exe
