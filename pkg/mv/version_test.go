package mv

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestGetCLIVersion(t *testing.T) {
	i, _ := GetInfo(context.Background())
	if i.CLIVersion != CLIVersion {
		t.Errorf("CLI version should always be returned. Got: %s, Expected: %s", i.CLIVersion, CLIVersion)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		foo, err := GetInfo(ctx)
		if fmt.Sprint(err) != "context canceled" {
			t.Errorf("Expected context to be cancelled, got: %s", err)
		}
		if foo.CLIVersion != CLIVersion {
			t.Errorf("Didn't get correct CLI version after cancel. Got: %s, Expected: %s", foo.CLIVersion, CLIVersion)
		}
		wg.Done()
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	wg.Wait()
}

func TestVersionFormat(t *testing.T) {
	info := Info{
		CLIVersion: "the-cli-version",
		MVVersion:  "metavisor-1-2-3-abc",
		Success:    true,
	}

	simpleFormat, err := FormatInfo(&info, false)
	if err != nil {
		t.Errorf("Got error when formatting without JSON: %s", err)
	}
	expected := `CLI Version:	the-cli-version
MV Version:	metavisor-1-2-3-abc`
	if simpleFormat != expected {
		t.Errorf("bad format, Got:\n%s\nExpected:\n%s", simpleFormat, expected)
	}

	info.MVVersion = ""

	simpleFormat, err = FormatInfo(&info, false)
	if err != nil {
		t.Errorf("Got error when formatting without JSON: %s", err)
	}
	expected = `CLI Version:	the-cli-version
MV Version:	<couldn't fetch>`
	if simpleFormat != expected {
		t.Errorf("bad format, Got:\n%s\nExpected:\n%s", simpleFormat, expected)
	}
}

func TestVersionFormatJSON(t *testing.T) {
	info := Info{
		CLIVersion: "the-cli-version",
		MVVersion:  "metavisor-1-2-3-abc",
		Success:    true,
	}

	jsonF, err := FormatInfo(&info, true)
	if err != nil {
		if err != nil {
			t.Errorf("Got error when formatting with JSON: %s", err)
		}
	}

	res := Info{}
	err = json.Unmarshal([]byte(jsonF), &res)
	if err != nil {
		t.Fatalf("Could not unmarshal the JSON produced")
	}
	if res.CLIVersion != "the-cli-version" {
		t.Errorf("CLI version wrong after unmarshal: %s", res.CLIVersion)
	}
}
