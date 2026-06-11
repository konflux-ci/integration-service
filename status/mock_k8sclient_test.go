/*
Copyright 2024 Red Hat Inc.

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

package status_test

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// type MockReporter struct{}

type MockK8sClient struct {
	getInterceptor     func(key client.ObjectKey, obj client.Object)
	listInterceptor    func(list client.ObjectList)
	genericInterceptor func(obj client.Object)
	err                error
}

func (m *MockK8sClient) Apply(ctx context.Context, patch runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	return nil
}

func (c *MockK8sClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}

func (c *MockK8sClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return true, nil
}

func (c *MockK8sClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.listInterceptor != nil {
		c.listInterceptor(list)
	}
	return c.err
}

func (c *MockK8sClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if c.getInterceptor != nil {
		c.getInterceptor(key, obj)
	}
	return c.err
}

func (c *MockK8sClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.genericInterceptor != nil {
		c.genericInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.genericInterceptor != nil {
		c.genericInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if c.genericInterceptor != nil {
		c.genericInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.genericInterceptor != nil {
		c.genericInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	if c.genericInterceptor != nil {
		c.genericInterceptor(obj)
	}
	return c.err
}

func (c *MockK8sClient) Status() client.SubResourceWriter {
	panic("implement me")
}

func (c *MockK8sClient) SubResource(subResource string) client.SubResourceClient {
	panic("implement me")
}

func (c *MockK8sClient) Scheme() *runtime.Scheme {
	panic("implement me")
}

func (c *MockK8sClient) RESTMapper() meta.RESTMapper {
	panic("implement me")
}
