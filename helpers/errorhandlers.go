/*
Copyright 2023.
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

package helpers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (l IntegrationLogger) HandleLoaderError(err error, resource, from string) (ctrl.Result, error) {
	if errors.IsNotFound(err) {
		l.Info(fmt.Sprintf("Could not get %[1]s from %[2]s.  %[1]s may have been removed.  Declining to proceed with reconciliation", resource, from))
		return ctrl.Result{}, nil
	}
	l.Error(err, fmt.Sprintf("Failed to get %s from the %s", resource, from))
	return ctrl.Result{}, err
}
