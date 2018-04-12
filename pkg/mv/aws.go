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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

const (
	prodBucketName   = "metavisor-prod-net"
	prodBucketRegion = "us-west-2"
	mvPrefix         = "metavisor"

	latestKey = "latest/amis.json"
	keySuffix = "/amis.json"
)

type mvVersions []string

func (v mvVersions) Len() int      { return len(v) }
func (v mvVersions) Swap(i, j int) { v[i], v[j] = v[j], v[i] }

type byVersion struct{ mvVersions }

func (v byVersion) Less(i, j int) bool {
	// Verison has format e.g. metavisor-2-19-49-g617a92b81
	p1 := strings.Split(v.mvVersions[i], "-")
	p2 := strings.Split(v.mvVersions[j], "-")
	maj1, _ := strconv.Atoi(p1[1])
	min1, _ := strconv.Atoi(p1[2])
	b1, _ := strconv.Atoi(p1[3])
	maj2, _ := strconv.Atoi(p2[1])
	min2, _ := strconv.Atoi(p2[2])
	b2, _ := strconv.Atoi(p2[3])
	if maj1 < maj2 {
		return true
	}
	if maj1 > maj2 {
		return false
	}

	if min1 < min2 {
		return true
	}
	if min1 > min2 {
		return false
	}

	return b1 <= b2
}

func awsGetMVVersions(ctx context.Context) (MetavisorVersions, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(prodBucketRegion),
	})
	if err != nil {
		return MetavisorVersions{}, err
	}
	s3C := s3.New(sess)
	mvs, err := listAllMetavisors(ctx, s3C)
	if err != nil {
		return MetavisorVersions{}, err
	}
	latest, err := determineLatest(ctx, s3C, mvs)
	if err != nil {
		latest = ""
	}
	return MetavisorVersions{
		Latest:   latest,
		Versions: mvs,
	}, nil
}

func listAllMetavisors(ctx context.Context, client *s3.S3) (mvVersions, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(prodBucketName),
		Prefix: aws.String(mvPrefix),
	}
	versions := map[string]struct{}{}
	err := client.ListObjectsV2PagesWithContext(ctx, input, func(out *s3.ListObjectsV2Output, last bool) bool {
		for _, obj := range out.Contents {
			versions[strings.Split(*obj.Key, "/")[0]] = struct{}{}
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	versionSlice := mvVersions{}
	for key := range versions {
		versionSlice = append(versionSlice, key)
	}
	sort.Sort(sort.Reverse(byVersion{versionSlice}))
	return versionSlice, nil
}

func determineLatest(ctx context.Context, client *s3.S3, allVersions []string) (string, error) {
	latest, err := getObjectBody(ctx, client, latestKey)
	if err != nil {
		return "", err
	}
	for i := range allVersions {
		v, err := getObjectBody(ctx, client, fmt.Sprintf("%s%s", allVersions[i], keySuffix))
		if err != nil {
			return "", err
		}
		if reflect.DeepEqual(latest, v) {
			return allVersions[i], nil
		}
	}
	return "", errors.New("No latest version")
}

// This below is a temporary hack to make sure we support both the old and
// the new coming structure of the amis.json file in the S3 bucket. The
// custom unmarshal function makes sure to use the new format if it's there,
// otherwise fall back to use old format
type amisImage struct {
	Region string `json:"region"`
	ID     string `json:"id"`
}

type mvAMIMap map[string]string

func (m *mvAMIMap) UnmarshalJSON(b []byte) error {
	str := string(b)
	if strings.Contains(str, "images") {
		tmp := struct {
			Images []amisImage `json:"images"`
		}{}
		err := json.Unmarshal(b, &tmp)
		if err != nil {
			return err
		}
		res := make(map[string]string)
		for _, img := range tmp.Images {
			res[img.Region] = img.ID
		}
		*m = res
	} else {
		tmp := make(map[string]string)
		err := json.Unmarshal(b, &tmp)
		if err != nil {
			return err
		}
		*m = tmp
	}
	return nil
}

func getObjectBody(ctx context.Context, client *s3.S3, key string) (map[string]string, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(prodBucketName),
		Key:    aws.String(key),
	}
	amis, err := client.GetObjectWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(amis.Body)
	amisMap := mvAMIMap{}
	err = decoder.Decode(&amisMap)
	if err != nil {
		return nil, err
	}
	return amisMap, nil
}
