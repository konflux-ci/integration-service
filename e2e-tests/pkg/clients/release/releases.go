package release

import (
	"context"

	releaseApi "github.com/konflux-ci/release-service/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *ReleaseController) GetReleases(namespace string) (*releaseApi.ReleaseList, error) {
	releaseList := &releaseApi.ReleaseList{}
	opts := []client.ListOption{
		client.InNamespace(namespace),
	}
	err := r.KubeRest().List(context.Background(), releaseList, opts...)

	return releaseList, err
}
