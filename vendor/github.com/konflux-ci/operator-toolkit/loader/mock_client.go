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
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClientOperation represents the type of client operation to mock
type ClientOperation string

const (
	// OperationGet represents a Get operation
	OperationGet ClientOperation = "get"
	// OperationList represents a List operation
	OperationList ClientOperation = "list"
	// OperationCreate represents a Create operation
	OperationCreate ClientOperation = "create"
	// OperationUpdate represents an Update operation
	OperationUpdate ClientOperation = "update"
	// OperationPatch represents a Patch operation
	OperationPatch ClientOperation = "patch"
	// OperationDelete represents a Delete operation
	OperationDelete ClientOperation = "delete"
	// OperationDeleteAllOf represents a DeleteAllOf operation
	OperationDeleteAllOf ClientOperation = "deleteallof"
)

// ClientCallMock represents a mocked client call configuration
type ClientCallMock struct {
	// Operation is the type of client operation (create, update, etc.)
	Operation ClientOperation
	// ObjectType is the type of object this mock applies to (e.g., &v1.Pod{} or &v1.PodList{})
	// For most operations this should be a client.Object, but for List operations it can be a client.ObjectList
	ObjectType any
	// Err is the error to return (if any)
	Err error
	// Result is the object to return for Get operations or to use for modifications
	Result client.Object
	// SubResourceName is the name of the subresource (e.g., "status")
	SubResourceName string
}

// mockClient is a custom client that intercepts operations and returns mocked responses
type mockClient struct {
	client.Client
	mocks   []ClientCallMock
	context context.Context
}

// NewMockClient creates a new mock client that wraps the real client
func NewMockClient(realClient client.Client, ctx context.Context) client.Client {
	return &mockClient{
		Client:  realClient,
		context: ctx,
	}
}

// findMock finds a matching mock for the given operation and object type
func (m *mockClient) findMock(operation ClientOperation, obj client.Object) *ClientCallMock {
	// Check if there are mocks in the context
	if value := m.context.Value(mockClientCallsKey); value != nil {
		if mocks, ok := value.([]ClientCallMock); ok {
			objType := fmt.Sprintf("%T", obj)
			for i := range mocks {
				mock := &mocks[i]
				if mock.Operation == operation {
					mockObjType := fmt.Sprintf("%T", mock.ObjectType)
					if mockObjType == objType {
						return mock
					}
				}
			}
		}
	}
	return nil
}

// Get implements client.Client
func (m *mockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if mock := m.findMock(OperationGet, obj); mock != nil {
		if mock.Err != nil {
			return mock.Err
		}
		if mock.Result != nil {
			// Copy the mock result to the object
			obj.SetName(mock.Result.GetName())
			obj.SetNamespace(mock.Result.GetNamespace())
			obj.SetLabels(mock.Result.GetLabels())
			obj.SetAnnotations(mock.Result.GetAnnotations())
		}
		return nil
	}
	return m.Client.Get(ctx, key, obj, opts...)
}

// List implements client.Client
func (m *mockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	// For List operations, we need to check the underlying type
	if value := m.context.Value(mockClientCallsKey); value != nil {
		if mocks, ok := value.([]ClientCallMock); ok {
			listType := fmt.Sprintf("%T", list)
			for i := range mocks {
				mock := &mocks[i]
				if mock.Operation == OperationList {
					mockListType := fmt.Sprintf("%T", mock.ObjectType)
					if mockListType == listType {
						if mock.Err != nil {
							return mock.Err
						}
						// For successful List mocks, return empty list or custom result
						return nil
					}
				}
			}
		}
	}
	return m.Client.List(ctx, list, opts...)
}

// Create implements client.Client
func (m *mockClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if mock := m.findMock(OperationCreate, obj); mock != nil {
		return mock.Err
	}
	return m.Client.Create(ctx, obj, opts...)
}

// Delete implements client.Client
func (m *mockClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if mock := m.findMock(OperationDelete, obj); mock != nil {
		return mock.Err
	}
	return m.Client.Delete(ctx, obj, opts...)
}

// Update implements client.Client
func (m *mockClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if mock := m.findMock(OperationUpdate, obj); mock != nil {
		return mock.Err
	}
	return m.Client.Update(ctx, obj, opts...)
}

// Patch implements client.Client
func (m *mockClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if mock := m.findMock(OperationPatch, obj); mock != nil {
		return mock.Err
	}
	return m.Client.Patch(ctx, obj, patch, opts...)
}

// DeleteAllOf implements client.Client
func (m *mockClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	if mock := m.findMock(OperationDeleteAllOf, obj); mock != nil {
		return mock.Err
	}
	return m.Client.DeleteAllOf(ctx, obj, opts...)
}

// Status implements client.Client
func (m *mockClient) Status() client.StatusWriter {
	return &mockStatusWriter{
		StatusWriter: m.Client.Status(),
		mockClient:   m,
	}
}

// SubResource implements client.Client
func (m *mockClient) SubResource(subResource string) client.SubResourceClient {
	return &mockSubResourceClient{
		SubResourceClient: m.Client.SubResource(subResource),
		mockClient:        m,
		subResourceName:   subResource,
	}
}

// Scheme implements client.Client
func (m *mockClient) Scheme() *runtime.Scheme {
	return m.Client.Scheme()
}

// RESTMapper implements client.Client
func (m *mockClient) RESTMapper() meta.RESTMapper {
	return m.Client.RESTMapper()
}

// GroupVersionKindFor implements client.Client
func (m *mockClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return m.Client.GroupVersionKindFor(obj)
}

// IsObjectNamespaced implements client.Client
func (m *mockClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return m.Client.IsObjectNamespaced(obj)
}

// mockStatusWriter wraps the status writer to intercept status updates
type mockStatusWriter struct {
	client.StatusWriter
	mockClient *mockClient
}

// Update implements client.StatusWriter
func (m *mockStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if mock := m.mockClient.findMock(OperationUpdate, obj); mock != nil && mock.SubResourceName == "status" {
		return mock.Err
	}
	return m.StatusWriter.Update(ctx, obj, opts...)
}

// Patch implements client.StatusWriter
func (m *mockStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	if mock := m.mockClient.findMock(OperationPatch, obj); mock != nil && mock.SubResourceName == "status" {
		return mock.Err
	}
	return m.StatusWriter.Patch(ctx, obj, patch, opts...)
}

// Create implements client.StatusWriter (added in newer versions of controller-runtime)
func (m *mockStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	if mock := m.mockClient.findMock(OperationCreate, obj); mock != nil && mock.SubResourceName == "status" {
		return mock.Err
	}
	return m.StatusWriter.Create(ctx, obj, subResource, opts...)
}

// mockSubResourceClient wraps the subresource client
type mockSubResourceClient struct {
	client.SubResourceClient
	mockClient      *mockClient
	subResourceName string
}

// Get implements client.SubResourceClient
func (m *mockSubResourceClient) Get(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceGetOption) error {
	if mock := m.mockClient.findMock(OperationGet, obj); mock != nil && mock.SubResourceName == m.subResourceName {
		return mock.Err
	}
	return m.SubResourceClient.Get(ctx, obj, subResource, opts...)
}

// Create implements client.SubResourceClient
func (m *mockSubResourceClient) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	if mock := m.mockClient.findMock(OperationCreate, obj); mock != nil && mock.SubResourceName == m.subResourceName {
		return mock.Err
	}
	return m.SubResourceClient.Create(ctx, obj, subResource, opts...)
}

// Update implements client.SubResourceClient
func (m *mockSubResourceClient) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if mock := m.mockClient.findMock(OperationUpdate, obj); mock != nil && mock.SubResourceName == m.subResourceName {
		return mock.Err
	}
	return m.SubResourceClient.Update(ctx, obj, opts...)
}

// Patch implements client.SubResourceClient
func (m *mockSubResourceClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	if mock := m.mockClient.findMock(OperationPatch, obj); mock != nil && mock.SubResourceName == m.subResourceName {
		return mock.Err
	}
	return m.SubResourceClient.Patch(ctx, obj, patch, opts...)
}
