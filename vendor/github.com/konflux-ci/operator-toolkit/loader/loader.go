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
	// ContextKey is a key that will be used within a context to reference a MockData item.
	ContextKey int

	// MockData represents the mocked data that will be returned by a mocked loader method.
	MockData struct {
		ContextKey ContextKey
		Err        error
		Resource   any
	}
)

// mockClientCallsKey is the context key for storing client call mocks
type mockClientCallsKeyType int

const mockClientCallsKey mockClientCallsKeyType = 0

// GetMockedContext creates a new context with the given MockData items.
// This function now supports both legacy loader mocks (via ContextKey) and
// new client operation mocks (via ClientCallMock).
func GetMockedContext(ctx context.Context, data []MockData) context.Context {
	for _, mockData := range data {
		ctx = context.WithValue(ctx, mockData.ContextKey, mockData)
	}

	return ctx
}

// GetMockedContextWithClient creates a new context with mocked client operations
// and returns both the context and a mock client that should be used in tests.
// This is the new enhanced function that supports mocking client operations.
func GetMockedContextWithClient(ctx context.Context, realClient client.Client, loaderMocks []MockData, clientMocks []ClientCallMock) (context.Context, client.Client) {
	// Add loader mocks (legacy support)
	for _, mockData := range loaderMocks {
		ctx = context.WithValue(ctx, mockData.ContextKey, mockData)
	}

	// Add client call mocks
	if len(clientMocks) > 0 {
		ctx = context.WithValue(ctx, mockClientCallsKey, clientMocks)
	}

	// Create and return mock client
	mockClient := NewMockClient(realClient, ctx)
	return ctx, mockClient
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
