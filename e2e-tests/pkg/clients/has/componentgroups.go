package has

import (
	"context"
	"fmt"
	"time"

	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/e2e-tests/pkg/utils"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	rclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// GetComponentGroup returns a component group given a name and namespace from kubernetes cluster.
func (h *HasController) GetComponentGroup(name string, namespace string) (*v1beta2.ComponentGroup, error) {
	componentGroup := v1beta2.ComponentGroup{
		Spec: v1beta2.ComponentGroupSpec{},
	}
	if err := h.KubeRest().Get(context.Background(), types.NamespacedName{Name: name, Namespace: namespace}, &componentGroup); err != nil {
		return nil, err
	}

	return &componentGroup, nil
}

// CreateComponentGroup creates a ComponentGroup object with a default timeout of
// 10 minutes. The timeout covers only the API write; controller reconciliation
// (status.globalCandidateList population) happens asynchronously and must be waited on separately.
func (h *HasController) CreateComponentGroup(name string, namespace string, components []v1beta2.ComponentReference) (*v1beta2.ComponentGroup, error) {
	return h.CreateComponentGroupWithTimeout(name, namespace, components, time.Minute*10)
}

// CreateComponentGroupWithTimeout creates a component group in the kubernetes cluster with a custom default time for creation.
func (h *HasController) CreateComponentGroupWithTimeout(name string, namespace string, components []v1beta2.ComponentReference, timeout time.Duration) (*v1beta2.ComponentGroup, error) {
	componentGroup := &v1beta2.ComponentGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1beta2.ComponentGroupSpec{
			Components: components,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := h.KubeRest().Create(ctx, componentGroup); err != nil {
		return nil, err
	}

	return componentGroup, nil
}

// DeleteComponentGroup delete a ComponentGroup resource from the namespace.
// Optionally, it can avoid returning an error if the resource did not exist:
// - specify 'false', if it's likely the ComponentGroup has already been deleted (for example, because the Namespace was deleted)
func (h *HasController) DeleteComponentGroup(name string, namespace string, reportErrorOnNotFound bool) error {
	componentGroup := v1beta2.ComponentGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	if err := h.KubeRest().Delete(context.Background(), &componentGroup); err != nil {
		if !k8sErrors.IsNotFound(err) || (k8sErrors.IsNotFound(err) && reportErrorOnNotFound) {
			return fmt.Errorf("error deleting a component group: %+v", err)
		}
	}
	return utils.WaitUntil(h.ComponentGroupDeleted(&componentGroup), 1*time.Minute)
}

// ComponentGroupDeleted check if a given componentGroup object was deleted successfully from the kubernetes cluster.
func (h *HasController) ComponentGroupDeleted(componentGroup *v1beta2.ComponentGroup) wait.ConditionFunc {
	return func() (bool, error) {
		_, err := h.GetComponentGroup(componentGroup.Name, componentGroup.Namespace)
		return err != nil && k8sErrors.IsNotFound(err), nil
	}
}

// DeleteAllComponentGroupsInASpecificNamespace removes all componentGroup CRs from a specific namespace. Useful when creating a lot of resources and want to remove all of them
func (h *HasController) DeleteAllComponentGroupsInASpecificNamespace(namespace string, timeout time.Duration) error {
	if err := h.KubeRest().DeleteAllOf(context.Background(), &v1beta2.ComponentGroup{}, rclient.InNamespace(namespace)); err != nil {
		return fmt.Errorf("error deleting componentGroups from the namespace %s: %+v", namespace, err)
	}

	return utils.WaitUntil(func() (done bool, err error) {
		componentGroupList, err := h.ListAllComponentGroups(namespace)
		if err != nil {
			return false, nil
		}
		return len(componentGroupList.Items) == 0, nil
	}, timeout)
}

// ListAllComponentGroups returns a list of all ComponentGroups in a given namespace.
func (h *HasController) ListAllComponentGroups(namespace string) (*v1beta2.ComponentGroupList, error) {
	componentGroupList := &v1beta2.ComponentGroupList{}
	err := h.KubeRest().List(context.Background(), componentGroupList, &rclient.ListOptions{Namespace: namespace})

	return componentGroupList, err
}
