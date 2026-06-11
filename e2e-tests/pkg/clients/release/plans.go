package release

import (
	"context"

	tektonutils "github.com/konflux-ci/release-service/tekton/utils"
	"k8s.io/apimachinery/pkg/runtime"

	releaseApi "github.com/konflux-ci/release-service/api/v1alpha1"
	releaseMetadata "github.com/konflux-ci/release-service/metadata"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateReleasePlan creates a new ReleasePlan using the given parameters.
func (r *ReleaseController) CreateReleasePlan(name, namespace, application, targetNamespace, autoReleaseLabel string, data *runtime.RawExtension, tenantPipeline *tektonutils.ParameterizedPipeline, finalPipeline *tektonutils.ParameterizedPipeline) (*releaseApi.ReleasePlan, error) {
	releasePlan := &releaseApi.ReleasePlan{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name,
			Name:         name,
			Namespace:    namespace,
			Labels: map[string]string{
				releaseMetadata.AutoReleaseLabel: autoReleaseLabel,
				releaseMetadata.AttributionLabel: "true",
			},
		},
		Spec: releaseApi.ReleasePlanSpec{
			Application:    application,
			Data:           data,
			TenantPipeline: tenantPipeline,
			FinalPipeline:  finalPipeline,
			Target:         targetNamespace,
		},
	}
	if autoReleaseLabel == "" || autoReleaseLabel == "true" {
		releasePlan.Labels[releaseMetadata.AutoReleaseLabel] = "true"
	} else {
		releasePlan.Labels[releaseMetadata.AutoReleaseLabel] = "false"
	}

	return releasePlan, r.KubeRest().Create(context.Background(), releasePlan)
}
