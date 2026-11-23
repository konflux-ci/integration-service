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

package testutil

import (
	"context"
	"time"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UpdateWithRetry wraps a k8s Update in an Eventually block that re-fetches
// the object before updating, preventing stale-UID race conditions in envtest.
// The mutate function is called after Get to re-apply the desired mutations on
// the freshly fetched object, then the object is updated. On conflict the
// entire Get-mutate-Update cycle is retried automatically.
func UpdateWithRetry(ctx context.Context, k8sClient client.Client, obj client.Object, mutate func()) {
	Eventually(func() error {
		key := types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}
		if err := k8sClient.Get(ctx, key, obj); err != nil {
			return err
		}
		mutate()
		return k8sClient.Update(ctx, obj)
	}, time.Second*20).Should(Succeed())
}
