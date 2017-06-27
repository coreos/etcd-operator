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

package backup

import (
	"bytes"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"github.com/Azure/azure-sdk-for-go/storage"
	"github.com/coreos/etcd-operator/pkg/backup/abs"
)

const (
	absIntegrationTestEnvVar = "RUN_ABS_INTEGRATION_TEST"
	absIntegrationTestNotSet = "skipping ABS integration test due to RUN_ABS_INTEGRATION_TEST not set"
)

var (
	accountName    = storage.StorageEmulatorAccountName
	accountKey     = storage.StorageEmulatorAccountKey
	DefaultBaseURL = "http://127.0.0.1:10000"
	container      = "testcontainer"
	prefix         = "testprefix"
	blobContents   = "ignore"
)

func TestABSBackendCredsDoNotExist(t *testing.T) {
	os.Setenv("AZURE_STORAGE_ACCOUNT", "")
	os.Setenv("AZURE_STORAGE_KEY", "")

	_, err := abs.New(container, prefix)

	if err == nil {
		t.Fatal(err)
	}
	if err.Error() != "missing required environment variable of AZURE_STORAGE_ACCOUNT" {
		t.Fatal(err, "expected the error to mention missing required environment variable of AZURE_STORAGE_ACCOUNT")
	}
}

func TestABSBackendContainerDoesNotExist(t *testing.T) {
	if os.Getenv(absIntegrationTestEnvVar) != "true" {
		t.Skip(absIntegrationTestNotSet)
	}

	storageClient, err := storage.NewClient(accountName, accountKey, DefaultBaseURL, "", false)
	if err != nil {
		t.Fatal(err)
	}

	_, err = abs.NewFromClient(container, prefix, &storageClient)
	if err == nil {
		t.Fatal(err)
	}
	if err.Error() != "container testcontainer does not exist" {
		t.Fatal(err)
	}
}

func TestABSBackendGetLatest(t *testing.T) {
	if os.Getenv(absIntegrationTestEnvVar) != "true" {
		t.Skip(absIntegrationTestNotSet)
	}

	storageClient, err := storage.NewClient(accountName, accountKey, DefaultBaseURL, "", false)
	if err != nil {
		t.Fatal(err)
	}
	blobServiceClient := storageClient.GetBlobService()

	// Create container
	cnt := blobServiceClient.GetContainerReference(container)
	options := storage.CreateContainerOptions{
		Access: storage.ContainerAccessTypePrivate,
	}
	_, err = cnt.CreateIfNotExists(&options)
	if err != nil {
		if accountName == storage.StorageEmulatorAccountName {
			t.Fatal(err, "Create container failed: If you are running with the emulator credentials, plaase make sure you have started the azurite storage emulator.")
		}
		t.Fatal(err, "Create container failed")
	}

	abs, err := abs.NewFromClient(container, prefix, &storageClient)
	if err != nil {
		t.Fatal(err)
	}
	ab := &absBackend{ABS: abs}

	if _, err := ab.save("3.1.0", 1, bytes.NewBuffer([]byte(blobContents))); err != nil {
		t.Fatal(err)
	}
	if _, err := ab.save("3.1.1", 2, bytes.NewBuffer([]byte(blobContents))); err != nil {
		t.Fatal(err)
	}

	// test getLatest
	name, err := ab.getLatest()
	if err != nil {
		t.Fatal(err)
	}

	expected := makeBackupName("3.1.1", 2)
	if name != expected {
		t.Errorf("lastest name = %s, want %s", name, expected)
	}

	// test total
	totalBackups, err := ab.total()
	if err != nil {
		t.Fatal(err)
	}
	if totalBackups != 2 {
		t.Errorf("total backups = %v, want %v", totalBackups, 2)
	}

	// test open
	rc, err := ab.open(name)
	if err != nil {
		t.Fatal(err)
	}

	b, err := ioutil.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()

	if string(b) != blobContents {
		t.Errorf("content = %s, want %s", string(b), blobContents)
	}

	// Delete container
	opts := storage.DeleteContainerOptions{}
	if err := cnt.Delete(&opts); err != nil {
		t.Fatal(err)
	}
}

func TestABSBackendPurge(t *testing.T) {
	if os.Getenv(absIntegrationTestEnvVar) != "true" {
		t.Skip(absIntegrationTestNotSet)
	}

	storageClient, err := storage.NewClient(accountName, accountKey, DefaultBaseURL, "", false)
	if err != nil {
		t.Fatal(err)
	}
	blobServiceClient := storageClient.GetBlobService()

	// Create container
	cnt := blobServiceClient.GetContainerReference(container)
	options := storage.CreateContainerOptions{
		Access: storage.ContainerAccessTypePrivate,
	}
	_, err = cnt.CreateIfNotExists(&options)
	if err != nil {
		if accountName == storage.StorageEmulatorAccountName {
			t.Fatal(err, "Create container failed: If you are running with the emulator credentials, plaase make sure you have started the azurite storage emulator.")
		}
		t.Fatal(err, "Create container failed")
	}

	abs, err := abs.NewFromClient(container, prefix, &storageClient)
	if err != nil {
		t.Fatal(err)
	}
	ab := &absBackend{ABS: abs}

	if _, err := ab.save("3.1.0", 1, bytes.NewBuffer([]byte(blobContents))); err != nil {
		t.Fatal(err)
	}
	if _, err := ab.save("3.1.0", 2, bytes.NewBuffer([]byte(blobContents))); err != nil {
		t.Fatal(err)
	}
	if err := ab.purge(1); err != nil {
		t.Fatal(err)
	}
	names, err := abs.List()
	if err != nil {
		t.Fatal(err)
	}
	leftFiles := []string{makeBackupName("3.1.0", 2)}
	if !reflect.DeepEqual(leftFiles, names) {
		t.Errorf("left files after purge, want=%v, get=%v", leftFiles, names)
	}
	if err := abs.Delete(makeBackupName("3.1.0", 2)); err != nil {
		t.Fatal(err)
	}

	// Delete container
	opts := storage.DeleteContainerOptions{}
	if err := cnt.Delete(&opts); err != nil {
		t.Fatal(err)
	}
}
