package snapshot

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/api/v1beta2"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/tekton"
	"github.com/konflux-ci/operator-toolkit/metadata"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PrepareSnapshotForPipelineRun prepares the Snapshot for a given PipelineRun,
// component and application. In case the Snapshot can't be created, an error will be returned.
func PrepareSnapshotForPipelineRun(ctx context.Context, adapterClient client.Client, pipelineRun *tektonv1.PipelineRun, componentName string, componentGroup *v1beta2.ComponentGroup) (*applicationapiv1alpha1.Snapshot, error) {
	log := log.FromContext(ctx)

	newSnapshotComponent, err := getSnapshotComponentFromBuildPLR(pipelineRun, componentName, log)
	if err != nil {
		return nil, err
	}

	snapshot, err := PrepareSnapshot(ctx, adapterClient, componentGroup, newSnapshotComponent, log)
	if err != nil {
		return nil, err
	}

	prefixes := []string{gitops.BuildPipelineRunPrefix, gitops.TestLabelPrefix, gitops.CustomLabelPrefix, gitops.ReleaseLabelPrefix}
	gitops.CopySnapshotLabelsAndAnnotations(&componentGroup.ObjectMeta, snapshot, componentName, &pipelineRun.ObjectMeta, prefixes, false)

	snapshot.Labels[gitops.BuildPipelineRunNameLabel] = pipelineRun.Name
	if pipelineRun.Status.CompletionTime != nil {
		snapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel] = strconv.FormatInt(pipelineRun.Status.CompletionTime.Unix(), 10)
	} else {
		snapshot.Labels[gitops.BuildPipelineRunFinishTimeLabel] = strconv.FormatInt(time.Now().Unix(), 10)
	}

	if gitops.IsSnapshotCreatedByPACMergeQueueEvent(snapshot) {
		pullRequestNumber := gitops.ExtractPullRequestNumberFromMergeQueueSnapshot(snapshot)
		if pullRequestNumber != "" {
			snapshot.Labels[gitops.PipelineAsCodePullRequestAnnotation] = pullRequestNumber
			snapshot.Annotations[gitops.PipelineAsCodePullRequestAnnotation] = pullRequestNumber
		}
	}

	// Set BuildPipelineRunStartTime annotation with millisecond precision and override snapshot name
	var timestampMillis int64
	// Get the time
	if pipelineRun.Status.StartTime != nil {
		timestampMillis = pipelineRun.Status.StartTime.UnixMilli()
	} else {
		timestampMillis = time.Now().UnixMilli()
	}
	// Naming once, at the end
	snapshot.Annotations[gitops.BuildPipelineRunStartTime] = strconv.FormatInt(timestampMillis, 10)
	snapshot.Name = gitops.GenerateSnapshotNameWithTimestamp(componentGroup.Name, timestampMillis)

	// Set the integration workflow annotation based on the PipelineRun type
	if tekton.IsPLRCreatedByPACPushEvent(pipelineRun) {
		snapshot.Annotations[gitops.IntegrationWorkflowAnnotation] = gitops.IntegrationWorkflowPushValue
	} else {
		snapshot.Annotations[gitops.IntegrationWorkflowAnnotation] = gitops.IntegrationWorkflowPullRequestValue
	}

	return snapshot, nil
}

// CreateSnapshotWithCollisionHandling attempts to create a snapshot, retrying with a random suffix if collision occurs
func CreateSnapshotWithCollisionHandling(ctx context.Context, client client.Client, pipelineRun *tektonv1.PipelineRun, snapshot *applicationapiv1alpha1.Snapshot, componentGroup v1beta2.ComponentGroup, logger helpers.IntegrationLogger) error {
	originalName := snapshot.Name
	maxRetries := 5

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := client.Create(ctx, snapshot)
		if err == nil {
			// Success
			if attempt > 0 {
				logger.Info("Successfully created snapshot after collision retry",
					"originalName", originalName,
					"finalName", snapshot.Name,
					"attempts", attempt+1)
			}
			return nil
		}

		// Check if it's an "already exists" error
		if !k8sErrors.IsAlreadyExists(err) {
			// Not a collision error, return immediately
			return err
		}

		// Collision detected - generate new name with suffix
		if attempt < maxRetries-1 {
			suffix, suffixErr := generateRandomSuffix()
			if suffixErr != nil {
				logger.Error(suffixErr, "Failed to generate random suffix for snapshot name collision")
				return err // Return original collision error
			}

			// Extract timestamp from original name or use current time
			var timestampMillis int64
			if pipelineRun.Status.StartTime != nil {
				timestampMillis = pipelineRun.Status.StartTime.UnixMilli()
			} else {
				timestampMillis = time.Now().UnixMilli()
			}

			// Regenerate name with suffix
			snapshot.Name = gitops.GenerateSnapshotNameWithTimestamp(componentGroup.Name, timestampMillis, suffix)
			logger.Info("Snapshot name collision detected, retrying with suffix",
				"originalName", originalName,
				"newName", snapshot.Name,
				"attempt", attempt+1,
				"maxRetries", maxRetries)
		} else {
			// Max retries reached
			logger.Error(err, "Failed to create snapshot after max retries due to collisions",
				"originalName", originalName,
				"attempts", maxRetries)
			return err
		}
	}

	return fmt.Errorf("failed to create snapshot after %d attempts", maxRetries)
}

// generateRandomSuffix generates a random 2-character alphanumeric suffix for collision handling
func generateRandomSuffix() (string, error) {
	const charset = "0123456789abcdefghijklmnopqrstuvwxyz"
	const suffixLength = 2
	suffix := make([]byte, suffixLength)
	for i := range suffix {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		suffix[i] = charset[num.Int64()]
	}
	return string(suffix), nil
}

// PrepareSnapshot prepares the Snapshot for a given componentGroup, components and the updated component (if any).
// In case the Snapshot can't be created, an error will be returned.
func PrepareSnapshot(ctx context.Context, adapterClient client.Client, componentGroup *v1beta2.ComponentGroup, newSnapshotComponent applicationapiv1alpha1.SnapshotComponent, log logr.Logger) (*applicationapiv1alpha1.Snapshot, error) {

	snapshotComponents, invalidComponents := getSnapshotComponentsFromGCL(componentGroup, log)
	upsertNewComponentImage(&snapshotComponents, &invalidComponents, newSnapshotComponent, log)

	if len(snapshotComponents) == 0 {
		return nil, helpers.NewMissingValidComponentError(joinInvalidComponentNamesAndVersions(invalidComponents))
	}
	snapshot := NewSnapshot(componentGroup, &snapshotComponents)

	// expose the source repo URL and SHA in the snapshot as annotation do we don't have to do lookup in integration tests
	if newSnapshotComponent.Source.GitSource != nil {
		if err := metadata.SetAnnotation(snapshot, gitops.SnapshotGitSourceRepoURLAnnotation, newSnapshotComponent.Source.GitSource.URL); err != nil {
			return nil, fmt.Errorf("failed to set annotation %s: %w", gitops.SnapshotGitSourceRepoURLAnnotation, err)
		}
	}

	// Annotate snapshot with warning about invalid components
	if len(invalidComponents) > 0 {
		if err := metadata.SetAnnotation(snapshot, helpers.CreateSnapshotAnnotationName, fmt.Sprintf("Component(s) '%s' is(are) not included in snapshot due to missing valid containerImage or git source", joinInvalidComponentNamesAndVersions(invalidComponents))); err != nil {
			return nil, fmt.Errorf("failed to set annotation %s: %w", gitops.SnapshotGitSourceRepoURLAnnotation, err)
		}
	}

	err := ctrl.SetControllerReference(componentGroup, snapshot, adapterClient.Scheme())
	if err != nil {
		return nil, err
	}

	return snapshot, nil
}

func getSnapshotComponentsFromGCL(componentGroup *v1beta2.ComponentGroup, log logr.Logger) ([]applicationapiv1alpha1.SnapshotComponent, []v1beta2.ComponentState) {
	var snapshotComponents []applicationapiv1alpha1.SnapshotComponent
	var invalidComponents []v1beta2.ComponentState
	for _, gclComponent := range componentGroup.Status.GlobalCandidateList {
		name := gclComponent.Name
		version := gclComponent.Version
		image := gclComponent.LastPromotedImage

		// If containerImage is empty, we have run into a race condition in
		// which multiple components are being built in close succession.
		// We omit this not-yet-built component from the snapshot rather than
		// including a component that is incomplete.
		if image == "" {
			// skip components that have not been added to GCL yet
			log.Info("component cannot be added to snapshot for application due to missing containerImage", "component.Name", gclComponent.Name)
			invalidComponents = append(invalidComponents, gclComponent)
			continue
		}
		err := gitops.ValidateImageDigest(image)
		if err != nil {
			log.Error(err, "component cannot be added to snapshot for ComponentGroup due to invalid digest in containerImage", "component.Name", gclComponent.Name)
			invalidComponents = append(invalidComponents, gclComponent)
			continue
		}

		// Get ComponentSource for the component which is not built in this pipeline
		componentSource := getComponentSourceFromGCLComponent(gclComponent)
		if err != nil {
			log.Error(err, "component cannot be added to snapshot for ComponentGroup due to missing git source", "component.Name", gclComponent.Name)
			invalidComponents = append(invalidComponents, gclComponent)
			continue
		}
		snapshotComponents = append(snapshotComponents, applicationapiv1alpha1.SnapshotComponent{
			Name:           name,
			Version:        version,
			ContainerImage: image,
			Source:         componentSource,
		},
		)
	}
	return snapshotComponents, invalidComponents
}

func upsertNewComponentImage(snapshotComponents *[]applicationapiv1alpha1.SnapshotComponent, invalidComponents *[]v1beta2.ComponentState, updatedComponent applicationapiv1alpha1.SnapshotComponent, log logr.Logger) {
	for i, snapshotComponent := range *snapshotComponents {
		if snapshotComponent.Name == updatedComponent.Name {
			if snapshotComponent.Version == "" || snapshotComponent.Version == updatedComponent.Version {
				// TODO: can this be removed?
				err := gitops.ValidateImageDigest(updatedComponent.ContainerImage)
				if err != nil {
					log.Error(err, "component cannot be added to snapshot for ComponentGroup due to invalid digest in containerImage", "component.Name", updatedComponent.Name)
					*invalidComponents = append(*invalidComponents, v1beta2.ComponentState{
						Name:    updatedComponent.Name,
						Version: updatedComponent.Version,
					})
					continue
				}

				// replace snapshotComponent
				*snapshotComponents = slices.Replace(*snapshotComponents, i, i+1, updatedComponent)
				//(*snapshotComponents)[i] = updatedComponent
				return
			}
		}
	}

	// if the component is replacing an invalid component, then we don't care that the old component was invalid
	for i, invalidComponent := range *invalidComponents {
		if invalidComponent.Name == updatedComponent.Name {
			if invalidComponent.Version == "" || invalidComponent.Version == updatedComponent.Version {
				// remove component from invalid list
				*snapshotComponents = append(*snapshotComponents, updatedComponent)
				// We don't care about the loop skipping problem here because we always exit
				// immediately after deleting the component from the list
				*invalidComponents = slices.Delete(*invalidComponents, i, i+1)

				return
			}
		}
	}

	// If the component is not in the list this is probably because the component has not been added to the GCL yet
	// In this case we should append the component
	*snapshotComponents = append(*snapshotComponents, updatedComponent)
}

// NewSnapshot creates a new snapshot based on the supplied ComponentGroup and components
func NewSnapshot(componentGroup *v1beta2.ComponentGroup, snapshotComponents *[]applicationapiv1alpha1.SnapshotComponent) *applicationapiv1alpha1.Snapshot {
	// Use fallback timestamp (current time) - will be overridden in prepareSnapshotForPipelineRun
	// if BuildPipelineRunStartTime is available
	fallbackTimestamp := time.Now().UnixMilli()

	snapshot := &applicationapiv1alpha1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gitops.GenerateSnapshotNameWithTimestamp(componentGroup.Name, fallbackTimestamp),
			Namespace: componentGroup.Namespace,
		},
		Spec: applicationapiv1alpha1.SnapshotSpec{
			ComponentGroup: componentGroup.Name,
			Components:     *snapshotComponents,
		},
	}
	return snapshot
}

func getComponentSourceFromGCLComponent(gclComponent v1beta2.ComponentState) applicationapiv1alpha1.ComponentSource {
	// NOTE: if we need to fall back on data from component CR we can do it in here
	componentSource := applicationapiv1alpha1.ComponentSource{
		ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
			GitSource: &applicationapiv1alpha1.GitSource{
				URL:      gclComponent.URL,
				Revision: gclComponent.LastPromotedCommit,
			},
		},
	}
	return componentSource
}

func getSnapshotComponentFromBuildPLR(pipelineRun *tektonv1.PipelineRun, componentName string, log logr.Logger) (applicationapiv1alpha1.SnapshotComponent, error) {
	containerImage, err := tekton.GetImagePullSpecFromPipelineRun(pipelineRun)
	if err != nil {
		return applicationapiv1alpha1.SnapshotComponent{}, err
	}
	err = gitops.ValidateImageDigest(containerImage)
	if err != nil {
		log.Error(err, "component cannot be added to snapshot for ComponentGroup due to invalid digest in containerImage", "component.Name", componentName)
		return applicationapiv1alpha1.SnapshotComponent{}, errors.Join(helpers.NewInvalidImageDigestError(componentName, containerImage), err)
	}
	componentSource, err := tekton.GetComponentSourceFromPipelineRun(pipelineRun)
	if err != nil {
		return applicationapiv1alpha1.SnapshotComponent{}, err
	}
	componentVersion, err := tekton.GetComponentVersionFromPipelineRun(pipelineRun)
	if err != nil {
		return applicationapiv1alpha1.SnapshotComponent{}, err
	}

	return applicationapiv1alpha1.SnapshotComponent{
		Name:           componentName,
		Version:        componentVersion,
		ContainerImage: containerImage,
		Source:         *componentSource,
	}, nil
}

func joinInvalidComponentNamesAndVersions(invalidComponents []v1beta2.ComponentState) string {
	var sb strings.Builder
	for _, component := range invalidComponents {
		sb.WriteString(fmt.Sprintf("%s (version %s)", component.Name, component.Version))
	}
	return sb.String()
}
