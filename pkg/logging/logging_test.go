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
