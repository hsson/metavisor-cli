package list

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/brkt/metavisor-cli/pkg/logging"
)

// MetavisorVersions is a slice of MetavisorVersion
type MetavisorVersions struct {
	Latest   string   `json:"latest_mv_version"`
	Versions []string `json:"mv_versions"`
}

// FormatMetavisors will format the provided list of Metavisors for display.
// If withJSON is true, then the formatted string will be structured JSON,
// otherwise, it will be a simple list of the format:
// metavisor-2-1-32-abc (latest)
// metavisor-2-0-94-xyz
// etc...
func FormatMetavisors(mvs MetavisorVersions, withJSON bool) (string, error) {
	if withJSON {
		data, err := json.MarshalIndent(mvs, "", "\t")
		if err != nil {
			logging.Errorf("Failed to marshal metavisor versions to JSON: %s", err)
		}
		return string(data), err
	}
	var s bytes.Buffer
	for i := range mvs.Versions {
		if mvs.Versions[i] == mvs.Latest {
			s.WriteString(fmt.Sprintf("%s (latest)\n", mvs.Versions[i]))
		} else {
			if i == len(mvs.Versions)-1 {
				s.WriteString(mvs.Versions[i])
			} else {
				s.WriteString(fmt.Sprintf("%s\n", mvs.Versions[i]))
			}
		}
	}
	return s.String(), nil
}

// GetMetavisorVersions will retrieve a list of available Metavisors
func GetMetavisorVersions() (MetavisorVersions, error) {
	return awsGetMVVersions()
}
