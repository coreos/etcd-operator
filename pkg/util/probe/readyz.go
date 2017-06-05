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

package probe

import (
	"net/http"
	"sync"
)

const (
	HTTPReadyzEndpoint = "/readyz"
)

var (
	mu    sync.Mutex
	ready = false
)

func SetReady() {
	mu.Lock()
	ready = true
	mu.Unlock()
}

// ReadyzHandler writes back the HTTP status code 200 if the operator is ready, and 500 otherwise
func ReadyzHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	isReady := ready
	mu.Unlock()
	if isReady {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
}
