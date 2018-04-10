//    Copyright 2018 Immutable Systems, Inc.
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package scp

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/immutable/metavisor-cli/pkg/logging"
)

const (
	// DefaultPort is the default port used if not specified
	DefaultPort = 22
)

var (
	// ErrSCPNotInstalled is returned if the current machine doesn't have SCP
	ErrSCPNotInstalled = errors.New("scp not available on this machine")
	// ErrNoUsername is returned if trying to create a new client without username
	ErrNoUsername = errors.New("must specify a username")
	// ErrNoHost is returned if trying to create a new client without a host
	ErrNoHost = errors.New("must specify a host")
	// ErrNoProxyHost is returned if trying to specify a proxy without a host
	ErrNoProxyHost = errors.New("proxy must have a host")

	simpleClientDefaultFlags = []string{
		"-o ServerAliveInterval=10",
		"-o UserKnownHostsFile=/dev/null",
		"-o StrictHostKeyChecking=no",
		"-o ConnectTimeout=5",
		"-o LogLevel=quiet",
	}
)

// Config is used to specify connection details for the client
type Config struct {
	Username string
	Host     string
	Port     int
	Key      string
	Proxy    *Proxy
}

// Proxy can be passed to a Config to do SSH hopping
type Proxy struct {
	Username string
	Host     string
	Port     int
	Key      string
}

// SCPClient represents a client capable of downloading remote files using SCP
type SCPClient interface {
	// DownloadFile will download the specified remote file to the specified local destination
	DownloadFile(remoteSource, localDestination string) error
}

type simpleClient struct {
	conf  Config
	flags []string
}

func (c *simpleClient) DownloadFile(remoteSource, localDestination string) error {
	args := fmt.Sprintf("%s %s@%s:%s %s", strings.Join(c.flags, " "), c.conf.Username, c.conf.Host, remoteSource, localDestination)
	cmd, err := exec.LookPath("scp")
	if err != nil {
		return ErrSCPNotInstalled
	}
	command := exec.Command(cmd, strings.Split(args, " ")...)
	logging.Debugf("Running SCP with:\n%s %s", cmd, args)
	output, err := command.CombinedOutput()
	logging.Debugf("Got the following output from SCP:\n%s", string(output))
	return err
}

// New will initialize a new SCP Client based on the specified configuration
func New(conf Config) (SCPClient, error) {
	conf, err := parseConfig(conf)
	if err != nil {
		return nil, err
	}

	return initSimpleClient(conf)
}

func parseConfig(conf Config) (Config, error) {
	if conf.Username == "" {
		return conf, ErrNoUsername
	}
	if conf.Host == "" {
		return conf, ErrNoHost
	}
	if conf.Port <= 0 {
		conf.Port = DefaultPort
	}
	if conf.Proxy != nil {
		if conf.Proxy.Host == "" {
			return conf, ErrNoProxyHost
		}
		if conf.Proxy.Port <= 0 {
			conf.Proxy.Port = DefaultPort
		}
		if conf.Proxy.Username == "" {
			conf.Proxy.Username = conf.Username
		}
	}
	return conf, nil
}

func initSimpleClient(conf Config) (SCPClient, error) {
	client := &simpleClient{
		conf:  conf,
		flags: simpleClientDefaultFlags,
	}
	if conf.Proxy != nil {
		client.flags = append(client.flags, createProxyCommand(*conf.Proxy))
	}
	if conf.Port != DefaultPort {
		client.flags = append(client.flags, fmt.Sprintf("-P %d", conf.Port))
	}
	if conf.Key != "" {
		client.flags = append(client.flags, fmt.Sprintf("-i %s", conf.Key))
	}
	return client, nil
}

func createProxyCommand(proxy Proxy) string {
	flags := []string{}
	if proxy.Port != DefaultPort {
		flags = append(flags, fmt.Sprintf("-P %d", proxy.Port))
	}
	if proxy.Key != "" {
		flags = append(flags, fmt.Sprintf("-i %s", proxy.Key))
	}
	flags = append(flags, fmt.Sprintf("-W %%h:%%p %s@%s", proxy.Username, proxy.Host))
	return fmt.Sprintf("-o ProxyCommand='ssh %s'", strings.Join(flags, " "))
}
