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
make build
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

## Licensing
This project is licensed under the Apache 2.0 license. Please see the `LICENSE` file for full licensing details.
