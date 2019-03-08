// Copyright 2019 The etcd-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package writer

import (
	"context"
	"fmt"
	"io"
	"path"
	"strconv"

	"github.com/coreos/etcd-operator/pkg/backup/util"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

type ossWriter struct {
	oss *oss.Client
}

// NewOSSWriter creates a oss writer.
func NewOSSWriter(oss *oss.Client) Writer {
	return &ossWriter{oss: oss}
}

// Write writes the backup file to the given oss path, "<oss-bucket-name>/<key>".
func (ossw *ossWriter) Write(ctx context.Context, path string, r io.Reader) (int64, error) {
	// TODO: support context.
	bk, key, err := util.ParseBucketAndKey(path)
	if err != nil {
		return 0, err
	}

	// If bucket doesn't exist, we create it.
	exist, err := ossw.oss.IsBucketExist(bk)
	if err != nil {
		return 0, err
	} else if !exist {
		if err = ossw.oss.CreateBucket(bk); err != nil {
			return 0, fmt.Errorf("failed to create bucket, error: %v", err)
		}
	}

	bucket, err := ossw.oss.Bucket(bk)
	if err != nil {
		return 0, err
	}

	if err = bucket.PutObject(key, r); err != nil {
		return 0, err
	}

	rc, err := bucket.GetObject(key)
	if err != nil {
		return 0, fmt.Errorf("failed to get oss object: %v", err)
	}

	var resp *oss.Response
	var ok bool

	if resp, ok = rc.(*oss.Response); !ok {
		return 0, fmt.Errorf("the response type from GetObject(%s) is not *oss.Response", key)
	}

	defer resp.Close()

	clstr := resp.Headers.Get("content-length")
	if clstr == "" {
		return 0, fmt.Errorf("content-length not found in headers of response in GetObject")
	}

	cl, err := strconv.ParseInt(clstr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid content-length: %s", clstr)
	}

	return cl, nil
}

func (ossw *ossWriter) List(ctx context.Context, basePath string) ([]string, error) {
	// TODO: support context.
	bk, key, err := util.ParseBucketAndKey(basePath)
	if err != nil {
		return nil, err
	}

	bucket, err := ossw.oss.Bucket(bk)
	if err != nil {
		return nil, err
	}

	var objKeys []string

	marker := oss.Marker("")
	prefix := oss.Prefix(key)
	for {
		resp, err := bucket.ListObjects(oss.MaxKeys(1000), marker, prefix)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %v", err)
		}

		for _, obj := range resp.Objects {
			objKeys = append(objKeys, path.Join(bk, obj.Key))
		}

		prefix = oss.Prefix(resp.Prefix)
		marker = oss.Marker(resp.NextMarker)

		if !resp.IsTruncated {
			break
		}
	}

	return objKeys, nil
}

func (ossw *ossWriter) Delete(ctx context.Context, path string) error {
	// TODO: support context.
	bk, key, err := util.ParseBucketAndKey(path)
	if err != nil {
		return err
	}

	bucket, err := ossw.oss.Bucket(bk)
	if err != nil {
		return err
	}

	return bucket.DeleteObject(key)
}
