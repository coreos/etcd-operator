// Copyright 2017 The etcd-operator Authors
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

package gcs

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"reflect"
	"sort"
	"testing"

	"cloud.google.com/go/storage"
)

var testKeyPath = "/tmp/gcs-key.json"

func TestGCS(t *testing.T) {
	testKey, err := ioutil.ReadFile(testKeyPath)
	if err != nil {
		t.Skipf("%v on %q", err, testKeyPath)
	}

	ctx, cancel := context.WithCancel(context.Background())

	g1, err := New(ctx,
		"etcd-operator-gcs-test",
		storage.ScopeFullControl,
		testKey,
		"namespace-1",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		g1.ctx = context.Background() // reset
		if err = g1.deleteBucket(); err != nil {
			t.Logf("delete-bucket error: %v", err)
		}
		if err = g1.Close(); err != nil {
			t.Logf("delete-bucket error: %v", err)
		}
	}()

	// create a test file
	k1, v1 := "test-key-1.json", []byte(`{"a":1000}`)
	if err = g1.Put(k1, v1); err != nil {
		t.Fatal(err)
	}
	var rc io.ReadCloser
	rc, err = g1.Get(k1)
	if err != nil {
		t.Fatal(err)
	}
	var v2 []byte
	v2, err = ioutil.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	rc.Close()
	if !bytes.Equal(v1, v2) {
		t.Fatalf("expected '%s', got '%s'", string(v1), string(v2))
	}

	// create another file
	k2, v2 := "test-key-2.json", []byte(`{"a":2000}`)
	if err = g1.Put(k2, v2); err != nil {
		t.Fatal(err)
	}

	// list files
	var keys1 []string
	keys1, err = g1.List()
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(keys1)
	if !reflect.DeepEqual(keys1, []string{k1, k2}) {
		t.Fatalf("unexpected key lists, got %v", keys1)
	}

	// create a new namespace
	var g2 *GCS
	g2, err = New(ctx,
		"etcd-operator-gcs-test",
		storage.ScopeFullControl,
		testKey,
		"namespace-2",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer g2.Close()

	// copy these files to a new namespace
	if err = g2.CopyPrefix(g1.prefix); err != nil {
		t.Fatal(err)
	}
	var keys2 []string
	keys2, err = g2.List()
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(keys2)
	if !reflect.DeepEqual(keys2, []string{k1, k2}) {
		t.Fatalf("unexpected key lists, got %v", keys2)
	}

	// empty the bucket before deletion
	if err = g1.Delete(k1); err != nil {
		t.Fatal(err)
	}
	if err = g1.Delete(k2); err != nil {
		t.Fatal(err)
	}
	if err = g2.Delete(k1); err != nil {
		t.Fatal(err)
	}
	if err = g2.Delete(k2); err != nil {
		t.Fatal(err)
	}

	// write with canceled context should fail
	cancel()
	err = g1.Put("test", []byte("test"))
	// reset for deferred actions
	if err != context.Canceled {
		t.Fatalf("expected %v, got %v", context.Canceled, err)
	}
}
