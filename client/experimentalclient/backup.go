package experimentalclient

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"

	"github.com/coreos/etcd-operator/pkg/backup/backupapi"
	"github.com/coreos/etcd-operator/pkg/util/k8sutil"
)

type Backup interface {
	// Request requests a backup. Request returns nil, once a backup is made successfully.
	// Or it returns an error.
	Request(ctx context.Context) error

	// Exist checks if there is a backup available for the specific version of etcd cluster.
	Exist(ctx context.Context, v string) (bool, error)
}

type backupClient struct {
	client *http.Client
	url    string
}

func NewBackup(c *http.Client, clusterName string) Backup {
	return &backupClient{
		client: c,
		url:    k8sutil.BackupServiceAddr(clusterName),
	}
}

func NewBackupWithURL(c *http.Client, url string) Backup {
	return &backupClient{
		client: c,
		url:    url,
	}
}

func (b backupClient) Request(ctx context.Context) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/backupnow", path.Join(b.url, backupapi.APIV1)), nil)
	if err != nil {
		return fmt.Errorf("request backup (%s) failed: %v", b.url, err)
	}
	req.WithContext(ctx)

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("request backup (%s) failed: %v", b.url, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	var errmsg string
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		errmsg = fmt.Sprintf("fail to read response body: %v", err)
	} else {
		errmsg = string(body)
	}
	return fmt.Errorf("request backup (%s) failed: unexpected status code (%v), response (%s)", b.url, resp.Status, errmsg)
}

func (b backupClient) Exist(ctx context.Context, v string) (bool, error) {
	req := &http.Request{
		Method: http.MethodHead,
		URL:    backupapi.NewBackupURL("http", b.url, v),
	}
	req.WithContext(ctx)

	resp, err := b.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("check backup existence (%s) failed: %v", b.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}

	var errmsg string
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		errmsg = fmt.Sprintf("fail to read response body: %v", err)
	} else {
		errmsg = string(body)
	}
	return false, fmt.Errorf("check backup existence (%s) failed: unexpected status code (%v), response (%s)", b.url, resp.Status, errmsg)
}
