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

type (
	// ValidationFunction defines the signature of validation functions that this validator can invoke.
	ValidationFunction func() *ValidationResult

	// ValidationResult is a struct containing whether a validation passed and what was the error in case that
	// it didn't pass.
	ValidationResult struct {
		Err   error
		Valid bool
	}
)

// Validate evaluates all the validation functions passed as an argument and returns a ValidationResult indicating
// whether the validation passed or not. In case of one of the functions failing the validation, the process will
// be interrupted and the error will be returned immediately.
func Validate(functions ...ValidationFunction) *ValidationResult {
	for _, function := range functions {
		result := function()
		if !result.Valid || result.Err != nil {
			return result
		}
	}

	return &ValidationResult{Valid: true}
}
