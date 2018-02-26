package mv

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/brkt/metavisor-cli/pkg/logging"
)

const (
	// CLIVersion is the current version of the CLI
	CLIVersion = "1.0.0"

	outputTemplate = "CLI Version:\t%s\nMV Version:\t%s"
	fetchTimeout   = 2 * time.Second
)

// Info is what will be displayed as a result of the version command
type Info struct {
	// CLIVersion is the version of the CLI itself
	CLIVersion string `json:"cli_version"`
	// MVVersion is the latest available MV Version
	MVVersion string `json:"mv_version"`
	// Success indicates whether fetching latest MV version succeeded or not
	Success bool `json:"-"`
}

// FormatInfo will format the provided version information for display in
// e.g. a CLI. The withJSON parameter determines if the formatted string
// should be structured as JSON or not. An error will be returned if the
// provided version information can not be marshalled to JSON.
func FormatInfo(info *Info, withJSON bool) (string, error) {
	if withJSON {
		data, err := json.MarshalIndent(info, "", "\t")
		if err != nil {
			logging.Errorf("Failed to marshal version information to JSON: %s", err)
		}
		return string(data), err
	}
	mvVersion := info.MVVersion
	if !info.Success || info.MVVersion == "" {
		mvVersion = "<couldn't fetch>"
	}
	return fmt.Sprintf(outputTemplate, info.CLIVersion, mvVersion), nil
}

// GetInfo will retrieve the CLI version and the Latest MV version. If the latest MV version
// cannot be retrieved, the CLI version will still be returned — but along with an error.
func GetInfo(ctx context.Context) (*Info, error) {
	// Cancel latest MV fetch if it takes too long
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	out := &Info{
		CLIVersion: CLIVersion,
	}
	res := make(chan MaybeString, 1)
	go getLatestMVVersion(ctx, res)
	var err error
	select {
	case <-ctx.Done():
		out.Success = false
		err = ctx.Err()
	case r := <-res:
		out.MVVersion = r.Result
		out.Success = r.Error == nil
		err = r.Error
	}
	return out, err
}

func getLatestMVVersion(ctx context.Context, c chan MaybeString) {
	versions, err := GetMetavisorVersions(ctx)
	c <- MaybeString{versions.Latest, err}
}
