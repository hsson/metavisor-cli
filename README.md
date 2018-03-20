# metavisor-cli
The `metavisor-cli` is a command-line interface to easily deploy cloud instances with the Metavisor.

The latest release of **metavisor-cli** is [1.0.0](https://github.com/brkt/metavisor-cli/releases/latest).

## Requirements
This CLI is implemented using [Go](https://golang.org) (version 1.10 to be specific). Go must be installed in order to compile the CLI. If you don't have Go installed, every release of the CLI is also accompanied by pre-compiled binaries for Darwin (macOS), Linux, FreeBSD, OpenBSD, and Windows which don't have any additional dependenices. To get the correct dependency versions when compiling, make sure to use the depedency management tool `dep`.

## Installation
Start by either compiling the CLI from the source code, or grab a pre-compiled binary from the latest release of the CLI. To compile the CLI yourself, first make sure your Go environment is properly setup and that you cloned this project to `$GOPATH/src/github.com/brkt/metavisor-cli`, then run:
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

## Usage
Assuming you have the CLI installed and available in your `$PATH` as `metavisor`, you can find all available commands as well as their corresponding parameters by running:
```
$ metavisor help
```
In order to find more specific help for a command, e.g. for wrapping instances, run:
```
$ metavisor aws wrap-instance --help
```
The easiest way to try the Metavisor out is to wrap one of your existing instances with it. Here follows an example for wrapping an instance with the ID `i-foobar123456` which is running in the region `us-west-2`:
```
$ metavisor aws wrap-instance --region=us-west-2 --token=$YOUR_LAUNCH_TOKEN i-foobar123456
```
Notice the `--token` argument, where a so-called launch token must be specified (in this case saved in the `$YOUR_LAUNCH_TOKEN` environment variable). The launch token is required in order to allow the Metavisor to communicate with the [Metavisor Director Console](https://mgmt.brkt.com). You can get a launch token by logging into your account in the [Metavisor Director Console](https://mgmt.brkt.com) and navigating to the `Generate Userdata` section of the `Settings` tab, and then clicking: `Generate --> OK --> COPY TOKEN ONLY`.

### AWS Permissions
The CLI requires a set of IAM permissions in EC2 in order to work properly. This is required to get the Metavisor up and running in your AWS account. An example policy template with the minimum permission requirements can be found in the `policy_template.json` file. If you attach this policy to an IAM role, that role can then be used by specifing the `--iam` flag in the CLI. Here is an example of wrapping an instance with the role `mv-cli-role` (assuming your AWS account ID is `123456789012`):
```
$ export YOUR_LAUNCH_TOKEN=<your launch token from Metavisor Director Console>
$ export ROLE=arn:aws:iam::123456789012:role/mv-cli-role
$ metavisor aws wrap-instance --region=us-west-2 --token=$YOUR_LAUNCH_TOKEN --iam=$ROLE i-foobar123456
```

## Contributing
The metavisor-cli project uses `dep` to manage dependencies. For more information about `dep`, please take a look at [golang.github.io/dep/](https://golang.github.io/dep/).

The easiest way to install `dep` on macOS is through Homebrew:
```
$ brew install dep
$ brew upgrade dep
```

## Using Docker
If Go or `dep` is not installed, and you still want to compile from the source code, Docker can be used. The following `make` targets are available:
### `make docker-build`
Will compile a binary for your current system, e.g. if you're running Windows a binary called `metavisor.exe` will be created.

### `make docker-build-[darwin/linux/openbsd/freebsd/windows]`
This make target can be used to create a binary for the specified platform, regarless of which system you're currently using. E.g. running `make docker-build-darwin` on a Windows machine will create a binary called `metavisor`.

### `make docker-build-all`
Create binaries for Windows, Linux, OpenBSD, FreeBSD, and Darwin (macOS). The binaries will have a suffix indicating which platform they're built for. I.e. `make docker-build-all` outputs:

- metavisor-linux
- metavisor-darwin
- metavisor-openbsd
- metavisor-freebsd
- metavisor-windows.exe

## Licensing
This project is licensed under the Apache 2.0 license. Please see the `LICENSE` file for full licensing details.
