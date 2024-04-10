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

package loader

import (
	"context"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type (
	// ContextKey is a kye that will be used within a context to reference a MockData item.
	ContextKey int

	// MockData represents the mocked data that will be returned by a mocked loader method.
	MockData struct {
		ContextKey ContextKey
		Err        error
		Resource   any
	}
)

// GetMockedContext creates a new context with the given MockData items.
func GetMockedContext(ctx context.Context, data []MockData) context.Context {
	for _, mockData := range data {
		ctx = context.WithValue(ctx, mockData.ContextKey, mockData)
	}

	return ctx
}

// GetMockedResourceAndErrorFromContext returns the mocked data found in the context passed as an argument. The data is
// to be found in the contextDataKey key. If not there, a panic will be raised.
func GetMockedResourceAndErrorFromContext[T any](ctx context.Context, contextKey ContextKey, _ T) (T, error) {
	var resource T
	var err error

	value := ctx.Value(contextKey)
	if value == nil {
		panic("Mocked data not found in the context")
	}

	data, _ := value.(MockData)

	if data.Resource != nil {
		resource = data.Resource.(T)
	}

	if data.Err != nil {
		err = data.Err
	}

	return resource, err
}

// GetObject loads an object from the cluster. This is a generic function that requires the object to be passed as an
// argument. The object is modified during the invocation.
func GetObject(name, namespace string, cli client.Client, ctx context.Context, object client.Object) error {
	return cli.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, object)
}
