// Copyright 2016 The etcd-operator Authors
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

import "errors"

var (
	errNoBackupExist     = errors.New("no backup exist for disaster recovery")
	errInvalidMemberName = errors.New("the format of member's name is invalid")

	errUnexpectedUnreadyMember = errors.New("unexpected unready member for selfhosted cluster")
)
