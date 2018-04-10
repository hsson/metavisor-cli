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

package logging

import (
	"bytes"
	"fmt"
	"log"
	"testing"
)

func TestLoggingBasic(t *testing.T) {
	var b bytes.Buffer
	termLogger = log.New(&b, "", 0)
	LogLevel = LevelInfo
	Info("Hello World!")
	if s := b.String(); s == fmt.Sprintf(templateInfo, "Hello World!") {
		t.Errorf("should not have prefix when level = INFO and writing to terminal\nGot: %s", s)
	}
	LogLevel = LevelDebug
	b.Reset()
	Info("Hello World!")
	if s := b.String(); s != fmt.Sprintf(templateInfo+"\n", "Hello World!") {
		t.Errorf("should have prefix when level = DEBUG and writing to terminal\nGot: %s", s)
	}
}

func TestLogAll(t *testing.T) {
	var b bytes.Buffer
	termLogger = log.New(&b, "", 0)
	LogLevel = LevelInfo
	expected := `Hello info
WARN:  Hello warn
ERROR: Hello error
`
	Info("Hello info")
	Debug("Hello debug") // Should not show
	Warning("Hello warn")
	Error("Hello error")
	if s := b.String(); s != expected {
		t.Errorf("Didn't get expected logs.\nGot:\n%s\nExpected:\n%s", s, expected)
	}
}

func TestLogOutvsErr(t *testing.T) {
	var stdErr, stdOut bytes.Buffer
	termLogger = log.New(&stdErr, "", 0)
	outLogger = log.New(&stdOut, "", 0)

	Info("Something")
	Warning("Something else")
	Debug("More stuff")
	Output("This is the output")
	if out := stdOut.String(); out != "This is the output\n" {
		t.Error("only the output should be logged to stdOut")
	}
}
