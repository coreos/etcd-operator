package backupapi

import (
	"fmt"
	"net/url"
	"path"
)

const (
	APIV1 = "/v1"

	HTTPQueryVersionKey  = "etcdVersion"
	HTTPQueryRevisionKey = "etcdRevision"
)

// NewBackupURL creates a URL struct for retrieving an existing backup.
func NewBackupURL(scheme, host, version string, revision int64) *url.URL {
	u := &url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   path.Join(APIV1, "backup"),
	}
	uv := url.Values{}
	uv.Set(HTTPQueryVersionKey, version)
	if revision >= 0 {
		uv.Set(HTTPQueryRevisionKey, fmt.Sprintf("%d", revision))
	}
	u.RawQuery = uv.Encode()

	return u
}
