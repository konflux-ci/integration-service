/*
Copyright 2026 Red Hat Inc.

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
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

const (
	// eventSourceComponent is the source component recorded on Events created by integration-service.
	eventSourceComponent = "integration-service"

	// maxEventMessageLength caps Event messages, matching the 1 KiB limit Kubernetes applies to event notes.
	maxEventMessageLength = 1024
)

// CreateWarningEvent records a Warning Event on obj, visible with `kubectl describe` and
// `kubectl get events`. The core/v1 Event is created directly with the given client, so no
// EventRecorder is needed. The Event is created in obj's namespace.
func CreateWarningEvent(ctx context.Context, c client.Client, obj client.Object, reason, message string) error {
	gvk, err := apiutil.GVKForObject(obj, c.Scheme())
	if err != nil {
		return fmt.Errorf("failed to determine kind of %s/%s for event: %w", obj.GetNamespace(), obj.GetName(), err)
	}
	if len(message) > maxEventMessageLength {
		message = message[:maxEventMessageLength-3] + "..."
	}

	now := metav1.Now()
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: obj.GetName() + ".",
			Namespace:    obj.GetNamespace(),
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion:      gvk.GroupVersion().String(),
			Kind:            gvk.Kind,
			Name:            obj.GetName(),
			Namespace:       obj.GetNamespace(),
			UID:             obj.GetUID(),
			ResourceVersion: obj.GetResourceVersion(),
		},
		Reason:         reason,
		Message:        message,
		Type:           corev1.EventTypeWarning,
		Source:         corev1.EventSource{Component: eventSourceComponent},
		FirstTimestamp: now,
		LastTimestamp:  now,
		Count:          1,
	}
	if err := c.Create(ctx, event); err != nil {
		return fmt.Errorf("failed to create %s event for %s %s/%s: %w", reason, gvk.Kind, obj.GetNamespace(), obj.GetName(), err)
	}
	return nil
}
