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
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	ReasonEnvironmentNotInNamespace     = "EnvironmentNotInNamespace"
	ReasonMissingInfoInPipelineRunError = "MissingInfoInPipelineRunError"
	ReasonInvalidImageDigestError       = "InvalidImageDigest"
	ReasonMissingValidComponentError    = "MissingValidComponentError"
	ReasonUnknownError                  = "UnknownError"
)

type IntegrationError struct {
	Reason  string
	Message string
}

func (ie *IntegrationError) Error() string {
	return ie.Message
}

func getReason(err error) string {
	var integrationErr *IntegrationError
	if !errors.As(err, &integrationErr) {
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

func MissingInfoInPipelineRunError(pipelineRunName, paramName string) error {
	return &IntegrationError{
		Reason:  ReasonMissingInfoInPipelineRunError,
		Message: fmt.Sprintf("Missing info %s from pipelinerun %s", paramName, pipelineRunName),
	}
}

func IsMissingInfoInPipelineRunError(err error) bool {
	return getReason(err) == ReasonMissingInfoInPipelineRunError
}

func NewInvalidImageDigestError(componentName, image_digest string) error {
	return &IntegrationError{
		Reason:  ReasonInvalidImageDigestError,
		Message: fmt.Sprintf("%s is invalid container image digest from component %s", image_digest, componentName),
	}
}

func IsInvalidImageDigestError(err error) bool {
	return getReason(err) == ReasonInvalidImageDigestError
}

func NewMissingValidComponentError(componentName string) error {
	return &IntegrationError{
		Reason:  ReasonMissingValidComponentError,
		Message: fmt.Sprintf("The only one component %s is invalid, valid .Spec.ContainerImage is missing", componentName),
	}
}

func IsMissingValidComponentError(err error) bool {
	return getReason(err) == ReasonMissingValidComponentError
}

func HandleLoaderError(logger IntegrationLogger, err error, resource, from string) (ctrl.Result, error) {
	if k8serrors.IsNotFound(err) {
		logger.Info(fmt.Sprintf("Could not get %[1]s from %[2]s.  %[1]s may have been removed.  Declining to proceed with reconciliation due to the error: %[3]v", resource, from, err))
		return ctrl.Result{}, nil
	}

	logger.Error(err, fmt.Sprintf("Failed to get %s from the %s", resource, from))
	return ctrl.Result{}, err
}
