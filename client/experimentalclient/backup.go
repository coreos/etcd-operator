package experimentalclient

import (
	"context"
	"encoding/json"
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

	// ServiceStatus returns the backup service status.
	ServiceStatus(ctx context.Context) (*backupapi.ServiceStatus, error)
}

type backupClient struct {
	client *http.Client
	scheme string
	// address of the backup service
	addr string
}

func NewBackup(c *http.Client, scheme, clusterName string) Backup {
	return &backupClient{
		client: c,
		scheme: scheme,
		addr:   k8sutil.BackupServiceAddr(clusterName),
	}
}

func NewBackupWithAddr(c *http.Client, scheme, addr string) Backup {
	return &backupClient{
		client: c,
		scheme: scheme,
		addr:   addr,
	}
}

func (b backupClient) Request(ctx context.Context) error {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s://%s/backupnow", b.scheme, path.Join(b.addr, backupapi.APIV1)), nil)
	if err != nil {
		return fmt.Errorf("request backup (%s) failed: %v", b.addr, err)
	}

	resp, err := b.client.Do(req.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("request backup (%s) failed: %v", b.addr, err)
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
	return fmt.Errorf("request backup (%s) failed: unexpected status code (%v), response (%s)", b.addr, resp.Status, errmsg)
}

func (b backupClient) Exist(ctx context.Context, v string) (bool, error) {
	req := &http.Request{
		Method: http.MethodHead,
		URL:    backupapi.NewBackupURL(b.scheme, b.addr, v),
	}

	resp, err := b.client.Do(req.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("check backup existence (%s) failed: %v", b.addr, err)
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
	return false, fmt.Errorf("check backup existence (%s) failed: unexpected status code (%v), response (%s)", b.addr, resp.Status, errmsg)
}

func (b backupClient) ServiceStatus(ctx context.Context) (*backupapi.ServiceStatus, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s://%s/status", b.scheme, path.Join(b.addr, backupapi.APIV1)), nil)
	if err != nil {
		return nil, fmt.Errorf("get service status (%s) failed: %v", b.addr, err)
	}

	resp, err := b.client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get service status (%s) failed: %v", b.addr, err)
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var status backupapi.ServiceStatus
		jd := json.NewDecoder(resp.Body)
		err = jd.Decode(&status)
		if err != nil {
			return nil, err
		}
		return &status, nil
	}

	var errmsg string
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		errmsg = fmt.Sprintf("fail to read response body: %v", err)
	} else {
		errmsg = string(body)
	}
	return nil, fmt.Errorf("get service status (%s) failed: unexpected status code (%v), response (%s)", b.addr, resp.Status, errmsg)
}
