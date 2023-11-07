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

package helpers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	ReasonEnvironmentNotInNamespace = "EnvironmentNotInNamespace"
	ReasonUnknownError              = "UnknownError"
)

type IntegrationError struct {
	Reason  string
	Message string
}

func (ie *IntegrationError) Error() string {
	return ie.Message
}

func getReason(err error) string {
	integrationErr, ok := err.(*IntegrationError)
	if !ok {
		return ReasonUnknownError
	}
	return integrationErr.Reason
}

func NewEnvironmentNotInNamespaceError(environment, namespace string) error {
	return &IntegrationError{
		Reason:  ReasonEnvironmentNotInNamespace,
		Message: fmt.Sprintf("Environment %s not found in namespace %s", environment, namespace),
	}
}

func IsEnvironmentNotInNamespaceError(err error) bool {
	return getReason(err) == ReasonEnvironmentNotInNamespace
}

func HandleLoaderError(logger IntegrationLogger, err error, resource, from string) (ctrl.Result, error) {
	if errors.IsNotFound(err) {
		logger.Info(fmt.Sprintf("Could not get %[1]s from %[2]s.  %[1]s may have been removed.  Declining to proceed with reconciliation", resource, from))
		return ctrl.Result{}, nil
	}
	logger.Error(err, fmt.Sprintf("Failed to get %s from the %s", resource, from))
	return ctrl.Result{}, err
}
