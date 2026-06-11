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

// IgnoreAllPredicate implements a default update predicate function to ignore all events.
//
// This predicate will skip any kind of event. It will be useful in cases in which a For clause has to be defined but
// no reconcile should happen over those resources.
type IgnoreAllPredicate struct {
	predicate.Funcs
}

// Create implements default CreateEvent filter to ignore any resource create.
func (IgnoreAllPredicate) Create(e event.CreateEvent) bool {
	return false
}

// Delete implements default DeleteEvent filter to ignore any resource delete.
func (IgnoreAllPredicate) Delete(e event.DeleteEvent) bool {
	return false
}

// Generic implements default GenericEvent filter to ignore any resource generic.
func (IgnoreAllPredicate) Generic(e event.GenericEvent) bool {
	return false
}

// Update implements default UpdateEvent filter to ignore any resource update.
func (IgnoreAllPredicate) Update(e event.UpdateEvent) bool {
	return false
}

// NewObjectsPredicate implements a default update predicate function on Generation unchanged.
//
// This predicate will skip update events that have a change in the object's metadata.generation field.
// The metadata.generation field of an object is incremented by the API server when writes are made to the spec field of
// an object. This allows a controller to ignore update events where the spec has unchanged, and only the metadata
// and/or status fields are changed.
type NewObjectsPredicate struct {
	predicate.Funcs
}

// Delete implements default DeleteEvent filter to ignore any resource delete.
func (NewObjectsPredicate) Delete(e event.DeleteEvent) bool {
	return false
}

// Generic implements default GenericEvent filter to ignore any resource generic.
func (NewObjectsPredicate) Generic(e event.GenericEvent) bool {
	return false
}

// Update implements default UpdateEvent filter to ignore any resource update.
func (NewObjectsPredicate) Update(e event.UpdateEvent) bool {
	return false
}

// TypedNewObjectsPredicate implements a default typed predicate function on new objects only.
//
// This predicate will skip update, delete, and generic events, and only trigger on object creation.
// This is the typed equivalent of NewObjectsPredicate.
type TypedNewObjectsPredicate[T client.Object] struct {
	predicate.TypedFuncs[T]
}

// Delete implements default TypedDeleteEvent filter to ignore any resource delete.
func (TypedNewObjectsPredicate[T]) Delete(e event.TypedDeleteEvent[T]) bool {
	return false
}

// Generic implements default TypedGenericEvent filter to ignore any resource generic.
func (TypedNewObjectsPredicate[T]) Generic(e event.TypedGenericEvent[T]) bool {
	return false
}

// Update implements default TypedUpdateEvent filter to ignore any resource update.
func (TypedNewObjectsPredicate[T]) Update(e event.TypedUpdateEvent[T]) bool {
	return false
}
