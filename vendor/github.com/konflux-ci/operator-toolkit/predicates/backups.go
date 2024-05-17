/*
Copyright 2023 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package predicates

import (
	"github.com/konflux-ci/operator-toolkit/utils"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// IgnoreBackups implements a default create predicate function to ignore all create events triggered by backup tools.
type IgnoreBackups struct {
	predicate.Funcs
}

// Create returns false if the object associated with the create event contains any of the expected backup labels.
// It will return true otherwise.
func (IgnoreBackups) Create(e event.CreateEvent) bool {
	return !utils.IsObjectRestoredFromBackup(e.Object)
}
