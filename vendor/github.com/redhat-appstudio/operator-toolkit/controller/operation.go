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

package controller

import "time"

// OperationResult represents the result of a reconcile operation
type OperationResult struct {
	RequeueDelay   time.Duration
	RequeueRequest bool
	CancelRequest  bool
}

// Operation defines the syntax of functions invoked by the ReconcileHandler
type Operation func() (OperationResult, error)

// ContinueProcessing returns an (OperationResult, error) tuple instructing the reconcile loop to continue
// reconciling of the object.
func ContinueProcessing() (OperationResult, error) {
	return OperationResult{
		RequeueDelay:   0,
		RequeueRequest: false,
		CancelRequest:  false,
	}, nil
}

// Requeue returns an (OperationResult, error) tuple instructing the reconcile loop to requeue the object.
func Requeue() (OperationResult, error) {
	return RequeueWithError(nil)
}

// RequeueAfter returns an (OperationResult, error) tuple instructing the reconcile loop to requeue the object
// after the given delay.
func RequeueAfter(delay time.Duration, err error) (OperationResult, error) {
	return OperationResult{
		RequeueDelay:   delay,
		RequeueRequest: true,
		CancelRequest:  false,
	}, err
}

// RequeueOnErrorOrContinue returns an (OperationResult, error) tuple instructing the reconcile loop to requeue
// the object in case of an error or to continue reconciling the object.
func RequeueOnErrorOrContinue(err error) (OperationResult, error) {
	return OperationResult{
		RequeueDelay:   0,
		RequeueRequest: false,
		CancelRequest:  false,
	}, err
}

// RequeueOnErrorOrStop returns an (OperationResult, error) tuple instructing the reconcile loop to requeue
// the object in case of an error or to stop reconciling the object.
func RequeueOnErrorOrStop(err error) (OperationResult, error) {
	return OperationResult{
		RequeueDelay:   0,
		RequeueRequest: false,
		CancelRequest:  true,
	}, err
}

// RequeueWithError returns an (OperationResult, error) tuple instructing the reconcile loop to requeue the object
// with the given reconcile error.
func RequeueWithError(err error) (OperationResult, error) {
	return OperationResult{
		RequeueDelay:   0,
		RequeueRequest: true,
		CancelRequest:  false,
	}, err
}

// StopProcessing returns an (OperationResult, error) tuple instructing the reconcile loop to stop reconciling the object.
func StopProcessing() (OperationResult, error) {
	return OperationResult{
		RequeueDelay:   0,
		RequeueRequest: false,
		CancelRequest:  true,
	}, nil
}
