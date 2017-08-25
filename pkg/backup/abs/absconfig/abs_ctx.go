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

package absconfig

import "errors"

// ABSContext represents the controller context around a ABS backend configuration
type ABSContext struct {
	ABSSecret    string
	ABSContainer string
}

// Validate validates a given ABSContext
func (a *ABSContext) Validate() error {
	allEmpty := len(a.ABSSecret) == 0 && len(a.ABSContainer) == 0
	allSet := len(a.ABSSecret) != 0 && len(a.ABSContainer) != 0

	if !(allEmpty || allSet) {
		return errors.New("ABS related values should be all set or all empty")
	}

	return nil
}
