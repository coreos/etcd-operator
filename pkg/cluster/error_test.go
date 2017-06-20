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

package cluster

import (
	"testing"

	"github.com/pkg/errors"
)

func TestWrapFatalError(t *testing.T) {
	tests := []struct {
		err     error
		isFatal bool
	}{{
		err:     &fatalError{},
		isFatal: true,
	}, {
		err:     newFatalError("some reason"),
		isFatal: true,
	}, {

		err:     errors.Wrap(newFatalError("cause"), "wrap"),
		isFatal: true,
	}, {
		err:     errors.New("not fatal"),
		isFatal: false,
	}}

	for i, tt := range tests {
		f := isFatalError(tt.err)
		if f != tt.isFatal {
			t.Errorf("#%d: isFatal want=%v, get=%v", i, tt.isFatal, f)
		}
	}
}
