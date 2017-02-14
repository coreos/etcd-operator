package backupapi

import (
	"net/url"
	"path"
)

const (
	APIV1 = "/v1"

	HTTPQueryVersionKey = "etcdVersion"
)

// NewBackupURL creates a URL struct for retrieving an existing backup.
func NewBackupURL(scheme, host, version string) *url.URL {
	u := &url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   path.Join(APIV1, "backup"),
	}
	uv := url.Values{}
	uv.Set(HTTPQueryVersionKey, version)
	u.RawQuery = uv.Encode()

	return u
}
