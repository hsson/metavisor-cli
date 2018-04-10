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

package mv

import (
	"encoding/json"
	"testing"
)

func TestListFormatSimple(t *testing.T) {
	latest := "latest-version"
	versions := MetavisorVersions{
		Latest: latest,
		Versions: []string{
			latest,
			"other-version",
			"some-other-version",
		},
	}
	simpleFormat, err := FormatMetavisors(versions, false)
	if err != nil {
		t.Fatalf("Got error when formatting without JSON:\n%s", err)
	}

	expected := `latest-version (latest)
other-version
some-other-version`
	if simpleFormat != expected {
		t.Fatalf("Got unexpected output:\n%s", simpleFormat)
	}
}

func TestListFormatJSON(t *testing.T) {
	latest := "latest-version"
	versions := MetavisorVersions{
		Latest: latest,
		Versions: []string{
			latest,
			"other-version",
			"some-other-version",
		},
	}
	jsonFormat, err := FormatMetavisors(versions, true)
	if err != nil {
		t.Fatalf("Got error when formatting with JSON:\n%s", err)
	}

	r := MetavisorVersions{}
	err = json.Unmarshal([]byte(jsonFormat), &r)
	if err != nil {
		t.Fatalf("Got error when unmarshalling: %s", err)
	}
	if r.Latest != latest {
		t.Errorf("Latest is not corrects %s != %s", r.Latest, latest)
	}
}
