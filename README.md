# metavisor-cli
The `metavisor-cli` is a command-line interface to easily deploy cloud instances with the Metavisor.

## Requirements
This CLI is implemented using [Go](https://golang.org) (version 1.10 to be specific). Go must be installed in order to compile the CLI. If you don't have Go installed, every release of the CLI is also accompanied by pre-compiled binaries for Darwin, Linux, and Windows which don't have any additional dependenices. To get the correct dependency versions when compiling, make sure to use the depedency management tool `dep`.

## Installation
Start by either compiling the CLI from the source code, or grab a pre-compiled binary from the latest release of the CLI. To compile the CLI yourself, first make sure your Go environment is properly setup, then run:
```
$ dep ensure
$ go build cmd/metavisor.go
```
OR run
```
$ make build
```
OR you can also build the CLI using Docker, to avoid having to install Go and `dep`, simply run:
```
$ make docker-build
```
If you instead decide to grab a binary from the releases page, make sure to get the one built for your system (e.g. get metavisor-darwin if you're on macOS).


After you have a compiled binary of the CLI, simply put it somewhere in your `$PATH` and you're ready to start using it. Try it out by running `metavisor version`.

## Contributing
The metavisor-cli project uses `dep` to manage dependencies. For more information about `dep`, please take a look at [golang.github.io/dep/](https://golang.github.io/dep/).

The easiest way to install `dep` on macOS is through Homebrew:
```
$ brew install dep
$ brew upgrade dep
```

## Using Docker
If `go` or `dep` is not installed, and you still want to compile from the soruce code, Docker can be user. The following `make` targets are available:
### `make docker-build`
Will compile a binary for your current system, e.g. if you're running Windows a binary called `metavisor.exe` will be created.

### `make docker-build-[darwin/linux/windows]`
This make target can be used to create a binary for the specified platform, regarless of which system you're currently using. E.g. running `make docker-build-darwin` on a Windows machine will create a binary called `metavisor`.

### `make docker-build-all`
Create binaries for Windows, Linux, and Darwin. The binaries will have a suffix indicating which platform they're built for. I.e. `make docker-build-all` outputs:

- metavisor-linux
- metavisor-darwin
- metavisor-windows.exe

## Licensing
This project is licensed under the Apache 2.0 license. Please see the `LICENSE` file for full licensing details.
