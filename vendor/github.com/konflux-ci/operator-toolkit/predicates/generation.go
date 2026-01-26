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

package predicates

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// GenerationUnchangedPredicate implements a default update predicate function on Generation unchanged.
//
// This predicate will skip update events that have a change in the object's metadata.generation field.
// The metadata.generation field of an object is incremented by the API server when writes are made to the spec field of
// an object. This allows a controller to ignore update events where the spec has unchanged, and only the metadata
// and/or status fields are changed.
type GenerationUnchangedPredicate struct {
	predicate.Funcs
}

// Update implements default UpdateEvent filter for validating generation change.
func (GenerationUnchangedPredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	return e.ObjectNew.GetGeneration() == e.ObjectOld.GetGeneration()
}

// GenerationUnchangedOnUpdatePredicate implements a default update predicate function on Generation unchanged.
//
// This predicate will skip any event except updates. In the case of update events that have a change in the
// object's metadata.generation field, those events will be skipped as well. The metadata.generation field of an object
// is incremented by the API server when writes are made to the spec field of an object. This allows a controller to
// ignore update events where the spec has unchanged, and only the metadata and/or status fields are changed.
type GenerationUnchangedOnUpdatePredicate struct {
	predicate.Funcs
}

// Create implements default CreateEvent filter for validating generation change.
func (GenerationUnchangedOnUpdatePredicate) Create(e event.CreateEvent) bool {
	return false
}

// Delete implements default DeleteEvent filter for validating generation change.
func (GenerationUnchangedOnUpdatePredicate) Delete(e event.DeleteEvent) bool {
	return false
}

// Generic implements default GenericEvent filter for validating generation change.
func (GenerationUnchangedOnUpdatePredicate) Generic(e event.GenericEvent) bool {
	return false
}

// Update implements default UpdateEvent filter for validating generation change.
func (GenerationUnchangedOnUpdatePredicate) Update(e event.UpdateEvent) bool {
	if e.ObjectOld == nil || e.ObjectNew == nil {
		return false
	}

	return e.ObjectNew.GetGeneration() == e.ObjectOld.GetGeneration()
}

// TypedGenerationChangedPredicate implements a default typed predicate function on Generation changes. Because only Update
// func is defined, Create, Delete, and Generic will return true
//
// This predicate will skip any event except updates. In the case of update events that have a change in the
// object's metadata.generation field, those events will be processed. The metadata.generation field of an object
// is incremented by the API server when writes are made to the spec field of an object. This allows a controller to
// only process update events where the spec has changed, ignoring updates where only metadata and/or status fields are changed.
type TypedGenerationChangedPredicate[T client.Object] struct {
	predicate.TypedFuncs[T]
}

// Update implements default TypedUpdateEvent filter for validating generation change.
func (TypedGenerationChangedPredicate[T]) Update(e event.TypedUpdateEvent[T]) bool {
	return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
}
