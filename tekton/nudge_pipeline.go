/*
Copyright 2025 Red Hat Inc.

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

package tekton

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	tektonconsts "github.com/konflux-ci/integration-service/tekton/consts"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logger "sigs.k8s.io/controller-runtime/pkg/log"
)

// ConfigMap data keys for custom Renovate options, matching build-service constants.
const (
	renovateConfigMapAutomergeKey             = "automerge"
	renovateConfigMapCommitMessagePrefixKey   = "commitMessagePrefix"
	renovateConfigMapCommitMessageSuffixKey   = "commitMessageSuffix"
	renovateConfigMapFileMatchKey             = "fileMatch"
	renovateConfigMapAutomergeTypeKey         = "automergeType"
	renovateConfigMapPlatformAutomergeKey     = "platformAutomerge"
	renovateConfigMapIgnoreTestsKey           = "ignoreTests"
	renovateConfigMapGitLabIgnoreApprovalsKey = "gitLabIgnoreApprovals"
	renovateConfigMapAutomergeScheduleKey     = "automergeSchedule"
	renovateConfigMapLabelsKey                = "labels"
)

// RenovateRepository represents a single git repository for Renovate to process.
type RenovateRepository struct {
	Repository   string   `json:"repository"`
	BaseBranches []string `json:"baseBranches,omitempty"`
}

// CustomRenovateOptions holds per-component Renovate configuration read from a ConfigMap.
type CustomRenovateOptions struct {
	Automerge             bool     `json:"automerge,omitempty"`
	AutomergeType         string   `json:"automergeType,omitempty"`
	PlatformAutomerge     bool     `json:"platformAutomerge,omitempty"`
	IgnoreTests           bool     `json:"ignoreTests,omitempty"`
	CommitMessagePrefix   string   `json:"commitMessagePrefix,omitempty"`
	CommitMessageSuffix   string   `json:"commitMessageSuffix,omitempty"`
	FileMatch             []string `json:"fileMatch,omitempty"`
	GitLabIgnoreApprovals bool     `json:"gitLabIgnoreApprovals,omitempty"`
	AutomergeSchedule     []string `json:"automergeSchedule,omitempty"`
	Labels                []string `json:"labels,omitempty"`
}

// NudgeTarget represents a target source code repository to be updated by
// Renovate with the necessary credentials and repository information.
type NudgeTarget struct {
	ComponentName                  string
	ComponentCustomRenovateOptions *CustomRenovateOptions
	GitProvider                    string
	Username                       string
	GitAuthor                      string
	Token                          string
	Endpoint                       string
	Repositories                   []RenovateRepository
	ImageRepositoryHost            string
	ImageRepositoryUsername        string
	ImageRepositoryPassword        string
}

// NudgeBuildResult holds the data extracted from a build PipelineRun that is
// needed to generate Renovate nudge configurations.
type NudgeBuildResult struct {
	BuiltImageRepository     string
	BuiltImageTag            string
	Digest                   string
	FileMatches              string
	SourceComponentName      string
	GitRepoAtShaLink         string
	DistributionRepositories []string
}

// CustomManager configures a Renovate custom regex manager for matching image
// digest references in downstream repository files.
type CustomManager struct {
	FileMatch            []string `json:"fileMatch,omitempty"`
	CustomType           string   `json:"customType"`
	DatasourceTemplate   string   `json:"datasourceTemplate"`
	MatchStrings         []string `json:"matchStrings"`
	CurrentValueTemplate string   `json:"currentValueTemplate"`
	DepNameTemplate      string   `json:"depNameTemplate"`
}

// PackageRule configures Renovate's per-package behavior: branch naming,
// commit messages, PR content, and merge strategy.
type PackageRule struct {
	MatchPackagePatterns   []string `json:"matchPackagePatterns,omitempty"`
	MatchPackageNames      []string `json:"matchPackageNames,omitempty"`
	GroupName              string   `json:"groupName,omitempty"`
	BranchPrefix           string   `json:"branchPrefix,omitempty"`
	BranchTopic            string   `json:"branchTopic,omitempty"`
	AdditionalBranchPrefix string   `json:"additionalBranchPrefix,omitempty"`
	CommitMessageTopic     string   `json:"commitMessageTopic,omitempty"`
	CommitMessagePrefix    string   `json:"commitMessagePrefix,omitempty"`
	CommitMessageSuffix    string   `json:"commitMessageSuffix,omitempty"`
	CommitBody             string   `json:"commitBody,omitempty"`
	PRFooter               string   `json:"prFooter,omitempty"`
	PRHeader               string   `json:"prHeader,omitempty"`
	RecreateWhen           string   `json:"recreateWhen,omitempty"`
	RebaseWhen             string   `json:"rebaseWhen,omitempty"`
	Enabled                bool     `json:"enabled"`
}

// RenovateConfig is the top-level Renovate JSON configuration written to the
// ConfigMap that drives a nudge PipelineRun.
type RenovateConfig struct {
	GitProvider           string               `json:"platform"`
	Username              string               `json:"username"`
	GitAuthor             string               `json:"gitAuthor"`
	Onboarding            bool                 `json:"onboarding"`
	RequireConfig         string               `json:"requireConfig"`
	Repositories          []RenovateRepository `json:"repositories"`
	EnabledManagers       []string             `json:"enabledManagers"`
	Endpoint              string               `json:"endpoint"`
	CustomManagers        []CustomManager      `json:"customManagers,omitempty"`
	RegistryAliases       map[string]string    `json:"registryAliases,omitempty"`
	PackageRules          []PackageRule        `json:"packageRules,omitempty"`
	ForkProcessing        string               `json:"forkProcessing"`
	Extends               []string             `json:"extends"`
	DependencyDashboard   bool                 `json:"dependencyDashboard"`
	Labels                []string             `json:"labels"`
	Automerge             bool                 `json:"automerge"`
	AutomergeType         string               `json:"automergeType,omitempty"`
	PlatformAutomerge     bool                 `json:"platformAutomerge"`
	IgnoreTests           bool                 `json:"ignoreTests"`
	GitLabIgnoreApprovals bool                 `json:"gitLabIgnoreApprovals"`
	AutomergeSchedule     []string             `json:"automergeSchedule,omitempty"`
}

// DisableAllPackageRules is the catch-all rule that disables all packages; the
// subsequent enable rule overrides it for the specific built image only.
var DisableAllPackageRules = PackageRule{MatchPackagePatterns: []string{"*"}, Enabled: false}

// ExtractNudgeBuildResult reads the build pipeline results and annotations to
// produce the data needed for nudge Renovate config generation.
func ExtractNudgeBuildResult(plr *tektonv1.PipelineRun, component *applicationapiv1alpha1.Component) (*NudgeBuildResult, error) {
	outputImage, err := GetOutputImage(plr)
	if err != nil {
		return nil, fmt.Errorf("extracting IMAGE_URL from build PLR %s: %w", plr.Name, err)
	}

	digest, err := GetOutputImageDigest(plr)
	if err != nil {
		return nil, fmt.Errorf("extracting IMAGE_DIGEST from build PLR %s: %w", plr.Name, err)
	}

	repo := outputImage
	tag := ""
	if index := strings.LastIndex(outputImage, ":"); index != -1 {
		repo = outputImage[:index]
		tag = outputImage[index+1:]
	}

	nudgeFiles := ""
	if plr.Annotations != nil {
		nudgeFiles = plr.Annotations[tektonconsts.NudgeFilesAnnotation]
	}
	if nudgeFiles == "" {
		nudgeFiles = tektonconsts.DefaultNudgeFiles
	}

	gitRepoAtShaLink := ""
	if plr.Annotations != nil {
		gitRepoAtShaLink = plr.Annotations[tektonconsts.GitRepoAtShaAnnotation]
	}

	return &NudgeBuildResult{
		BuiltImageRepository: repo,
		BuiltImageTag:        tag,
		Digest:               digest,
		FileMatches:          nudgeFiles,
		SourceComponentName:  component.Name,
		GitRepoAtShaLink:     gitRepoAtShaLink,
	}, nil
}

// GenerateRenovateConfig produces the Renovate JSON config for a single nudge
// target, replicating the build-service generateRenovateConfigForNudge shape.
func GenerateRenovateConfig(target NudgeTarget, buildResult *NudgeBuildResult, simpleBranchName bool) RenovateConfig {
	fileMatchParts := strings.Split(buildResult.FileMatches, ",")
	for i := range fileMatchParts {
		fileMatchParts[i] = strings.TrimSpace(fileMatchParts[i])
	}

	var matchStrings []string
	registryAliases := make(map[string]string)
	var matchPackageNames []string

	matchStrings = append(matchStrings, regexp.QuoteMeta(buildResult.BuiltImageRepository)+"(:.*)?@(?<currentDigest>sha256:[a-f0-9]+)")
	matchPackageNames = append(matchPackageNames, buildResult.BuiltImageRepository)

	for _, drepo := range buildResult.DistributionRepositories {
		matchStrings = append(matchStrings, regexp.QuoteMeta(drepo)+"(:.*)?@(?<currentDigest>sha256:[a-f0-9]+)")
		matchPackageNames = append(matchPackageNames, drepo)
		registryAliases[drepo] = buildResult.BuiltImageRepository
	}

	customOpts := target.ComponentCustomRenovateOptions
	if customOpts == nil {
		customOpts = &CustomRenovateOptions{}
	}

	if customOpts.FileMatch != nil {
		fileMatchParts = customOpts.FileMatch
	}

	labels := []string{tektonconsts.DefaultRenovateLabel}
	if customOpts.Labels != nil {
		labels = append(labels, customOpts.Labels...)
	}

	customManagers := []CustomManager{{
		FileMatch:            fileMatchParts,
		CustomType:           "regex",
		DatasourceTemplate:   "docker",
		MatchStrings:         matchStrings,
		CurrentValueTemplate: buildResult.BuiltImageTag,
		DepNameTemplate:      buildResult.BuiltImageRepository,
	}}

	nudgingBranchName := fmt.Sprintf("%s-", target.ComponentName)
	if simpleBranchName {
		nudgingBranchName = ""
	}

	packageRules := []PackageRule{
		DisableAllPackageRules,
		{
			MatchPackageNames:      matchPackageNames,
			GroupName:              fmt.Sprintf("Component Update %s", buildResult.SourceComponentName),
			BranchPrefix:           tektonconsts.BranchPrefix,
			BranchTopic:            buildResult.SourceComponentName,
			AdditionalBranchPrefix: nudgingBranchName,
			CommitMessageTopic:     buildResult.SourceComponentName,
			PRFooter:               "To execute skipped test pipelines write comment `/ok-to-test`",
			PRHeader:               fmt.Sprintf("Image created from '%s'", buildResult.GitRepoAtShaLink),
			RecreateWhen:           "always",
			RebaseWhen:             "behind-base-branch",
			Enabled:                true,
			CommitBody:             fmt.Sprintf("Image created from '%s'\n\nSigned-off-by: %s", buildResult.GitRepoAtShaLink, target.GitAuthor),
			CommitMessagePrefix:    customOpts.CommitMessagePrefix,
			CommitMessageSuffix:    customOpts.CommitMessageSuffix,
		},
	}

	return RenovateConfig{
		GitProvider:           target.GitProvider,
		Username:              target.Username,
		GitAuthor:             target.GitAuthor,
		Onboarding:            false,
		RequireConfig:         "ignored",
		Repositories:          target.Repositories,
		EnabledManagers:       []string{"custom.regex"},
		Endpoint:              target.Endpoint,
		CustomManagers:        customManagers,
		RegistryAliases:       registryAliases,
		PackageRules:          packageRules,
		ForkProcessing:        "enabled",
		Extends:               []string{},
		DependencyDashboard:   false,
		Labels:                labels,
		Automerge:             customOpts.Automerge,
		PlatformAutomerge:     customOpts.PlatformAutomerge,
		IgnoreTests:           customOpts.IgnoreTests,
		AutomergeType:         customOpts.AutomergeType,
		GitLabIgnoreApprovals: customOpts.GitLabIgnoreApprovals,
		AutomergeSchedule:     customOpts.AutomergeSchedule,
	}
}

// CreateNudgePipelineRun creates the Renovate PipelineRun plus its supporting
// Secret and ConfigMap in the build PLR namespace.  The PipelineRun is created
// first so owner references can point back to it; Secret and ConfigMap are
// created afterward (Tekton will wait for them).
func CreateNudgePipelineRun(ctx context.Context, c client.Client, nudgingPLR *tektonv1.PipelineRun, targets []NudgeTarget, buildResult *NudgeBuildResult, simpleBranchName bool) error {
	log := logger.FromContext(ctx)

	if len(targets) == 0 {
		return nil
	}

	nameSuffix := deterministicSuffix(string(nudgingPLR.UID))
	name := fmt.Sprintf("renovate-pipeline-%s", nameSuffix)
	namespace := nudgingPLR.Namespace
	caConfigMapName := fmt.Sprintf("renovate-ca-%s", nameSuffix)

	secretTokens := map[string]string{}
	configmaps := map[string]string{}
	var renovateCmds []string

	for i, target := range targets {
		configKey := fmt.Sprintf("%s-%d.json", target.ComponentName, i)
		tokenKey := fmt.Sprintf("token-%d", i)
		imageKey := fmt.Sprintf("image-%d", i)
		secretTokens[tokenKey] = target.Token
		secretTokens[imageKey] = target.ImageRepositoryPassword

		renovateConfig := GenerateRenovateConfig(target, buildResult, simpleBranchName)
		config, err := json.Marshal(renovateConfig)
		if err != nil {
			return fmt.Errorf("marshalling renovate config for component %s: %w", target.ComponentName, err)
		}

		configmaps[configKey] = string(config)
		log.Info("created renovate config entry", "component", target.ComponentName, "configLength", len(config))

		hostRules := fmt.Sprintf("\"[{'matchHost':'%s','username':'%s','password':'${TOKEN_%s}'}]\"",
			target.ImageRepositoryHost, target.ImageRepositoryUsername, imageKey)

		renovateCmds = append(renovateCmds,
			fmt.Sprintf("RENOVATE_X_GITLAB_MERGE_REQUEST_DELAY=5000 RENOVATE_X_GITLAB_AUTO_MERGEABLE_CHECK_ATTEMPS=11 RENOVATE_PR_HOURLY_LIMIT=0 RENOVATE_PR_CONCURRENT_LIMIT=0 RENOVATE_TOKEN=$TOKEN_%s RENOVATE_CONFIG_FILE=/configs/%s RENOVATE_HOST_RULES=%s renovate",
				tokenKey, configKey, hostRules),
		)
	}
	if len(renovateCmds) == 0 {
		return nil
	}

	// Look for a CA bundle ConfigMap in the build-service namespace.
	allCaConfigMaps := &corev1.ConfigMapList{}
	if err := c.List(ctx, allCaConfigMaps,
		client.InNamespace(tektonconsts.BuildServiceNamespaceName),
		client.MatchingLabels{tektonconsts.CaConfigMapLabel: "true"},
	); err != nil {
		return fmt.Errorf("listing CA config maps in %s namespace: %w", tektonconsts.BuildServiceNamespaceName, err)
	}
	caConfigData := ""
	if len(allCaConfigMaps.Items) > 0 {
		log.Info("using CA config map", "name", allCaConfigMaps.Items[0].Name)
		caConfigData = allCaConfigMaps.Items[0].Data[tektonconsts.CaConfigMapKey]
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		StringData: secretTokens,
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: configmaps,
	}

	var caConfigMap *corev1.ConfigMap
	if caConfigData != "" {
		caConfigMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      caConfigMapName,
				Namespace: namespace,
			},
			Data: map[string]string{tektonconsts.CaConfigMapKey: caConfigData},
		}
	}

	// Determine the service account from the source build PLR.
	saName := nudgingPLR.Spec.TaskRunTemplate.ServiceAccountName
	if saName == "" {
		saName = "appstudio-pipeline"
	}
	if err := c.Get(ctx, types.NamespacedName{Name: saName, Namespace: namespace}, &corev1.ServiceAccount{}); err != nil {
		return fmt.Errorf("service account %s not found in namespace %s: %w", saName, namespace, err)
	}

	trueBool := true
	falseBool := false
	renovateImageUrl := os.Getenv(tektonconsts.RenovateImageEnvName)
	if renovateImageUrl == "" {
		renovateImageUrl = tektonconsts.DefaultRenovateImageUrl
	}

	pipelineRun := &tektonv1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				tektonconsts.NudgeTypeLabel: tektonconsts.NudgePipelineRunTypeValue,
			},
			Annotations: map[string]string{
				tektonconsts.NudgedComponentsAnnotation: joinNudgedComponentNames(targets),
				tektonconsts.NudgingCommitAnnotation:    extractCommitHash(buildResult.GitRepoAtShaLink),
				tektonconsts.NudgingComponentAnnotation: buildResult.SourceComponentName,
				tektonconsts.NudgingImageAnnotation:     fmt.Sprintf("%s:%s", buildResult.BuiltImageRepository, buildResult.BuiltImageTag),
				tektonconsts.NudgingPipelineAnnotation:  nudgingPLR.Name,
			},
		},
		Spec: tektonv1.PipelineRunSpec{
			PipelineSpec: &tektonv1.PipelineSpec{
				Tasks: []tektonv1.PipelineTask{{
					Name: "renovate",
					TaskSpec: &tektonv1.EmbeddedTask{
						TaskSpec: tektonv1.TaskSpec{
							Steps: []tektonv1.Step{{
								Name:  "renovate",
								Image: renovateImageUrl,
								EnvFrom: []corev1.EnvFromSource{
									{
										Prefix: "TOKEN_",
										SecretRef: &corev1.SecretEnvSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: name,
											},
										},
									},
								},
								Command: []string{"bash", "-c", strings.Join(renovateCmds, "; ")},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      name,
										MountPath: "/configs",
									},
								},
								SecurityContext: &corev1.SecurityContext{
									Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
									RunAsNonRoot:             &trueBool,
									AllowPrivilegeEscalation: &falseBool,
									SeccompProfile: &corev1.SeccompProfile{
										Type: corev1.SeccompProfileTypeRuntimeDefault,
									},
								},
							}},
							Volumes: []corev1.Volume{
								{
									Name: name,
									VolumeSource: corev1.VolumeSource{
										ConfigMap: &corev1.ConfigMapVolumeSource{
											LocalObjectReference: corev1.LocalObjectReference{Name: name},
										},
									},
								},
							},
						},
					},
				}},
			},
			TaskRunTemplate: tektonv1.PipelineTaskRunTemplate{
				ServiceAccountName: saName,
			},
		},
	}

	if caConfigData != "" {
		caVolume := corev1.Volume{
			Name: tektonconsts.CaVolumeMountName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: caConfigMapName},
					Items: []corev1.KeyToPath{
						{
							Key:  tektonconsts.CaConfigMapKey,
							Path: tektonconsts.CaFilePath,
						},
					},
				},
			},
		}
		caVolumeMount := corev1.VolumeMount{
			Name:      tektonconsts.CaVolumeMountName,
			MountPath: tektonconsts.CaMountPath,
			ReadOnly:  true,
		}
		pipelineRun.Spec.PipelineSpec.Tasks[0].TaskSpec.Volumes = append(
			pipelineRun.Spec.PipelineSpec.Tasks[0].TaskSpec.Volumes, caVolume)
		pipelineRun.Spec.PipelineSpec.Tasks[0].TaskSpec.Steps[0].VolumeMounts = append(
			pipelineRun.Spec.PipelineSpec.Tasks[0].TaskSpec.Steps[0].VolumeMounts, caVolumeMount)
	}

	// Create supporting resources before the PipelineRun so a partial failure
	// on retry won't leave a running PLR without its Secret/ConfigMap.
	if err := createIfNotExists(ctx, c, secret); err != nil {
		return fmt.Errorf("creating Secret %s: %w", name, err)
	}
	if err := createIfNotExists(ctx, c, configMap); err != nil {
		return fmt.Errorf("creating ConfigMap %s: %w", name, err)
	}
	if caConfigData != "" {
		if err := createIfNotExists(ctx, c, caConfigMap); err != nil {
			return fmt.Errorf("creating CA ConfigMap %s: %w", caConfigMapName, err)
		}
	}

	if err := createIfNotExists(ctx, c, pipelineRun); err != nil {
		return fmt.Errorf("creating nudge PipelineRun %s: %w", name, err)
	}

	log.Info("nudge PipelineRun created", "name", pipelineRun.Name, "targets", len(targets))
	return nil
}

// createIfNotExists creates the object, silently succeeding if it already exists.
func createIfNotExists(ctx context.Context, c client.Client, obj client.Object) error {
	if err := c.Create(ctx, obj); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

// deterministicSuffix returns a short hex string derived from the input,
// suitable for generating stable resource names from a build PLR UID.
func deterministicSuffix(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])[:12]
}

// ReadCustomRenovateConfigMap reads custom Renovate options from a ConfigMap
// referenced by the component annotation, falling back to the namespace-wide
// ConfigMap.  Returns a zero-value struct (not nil) when no ConfigMap exists.
func ReadCustomRenovateConfigMap(ctx context.Context, c client.Client, component *applicationapiv1alpha1.Component) (*CustomRenovateOptions, error) {
	log := logger.FromContext(ctx)
	opts := &CustomRenovateOptions{}

	cmName := tektonconsts.NamespaceWideRenovateConfigMapName
	if component.Annotations != nil {
		if perComponent := component.Annotations[tektonconsts.CustomRenovateConfigMapAnnotation]; perComponent != "" {
			cmName = perComponent
		}
	}

	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, types.NamespacedName{Name: cmName, Namespace: component.Namespace}, cm); err != nil {
		if errors.IsNotFound(err) {
			if cmName != tektonconsts.NamespaceWideRenovateConfigMapName {
				log.Info("custom renovate config map in component annotation doesn't exist", "configMapName", cmName)
			}
			return opts, nil
		}
		return opts, err
	}
	log.Info("using custom renovate config map", "configMapName", cmName)

	if v, ok := cm.Data[renovateConfigMapAutomergeKey]; ok {
		if parsed, err := strconv.ParseBool(v); err == nil {
			opts.Automerge = parsed
		}
	}

	if v, ok := cm.Data[renovateConfigMapPlatformAutomergeKey]; ok {
		if parsed, err := strconv.ParseBool(v); err == nil {
			opts.PlatformAutomerge = parsed
		}
	} else {
		// Renovate default is true
		opts.PlatformAutomerge = true
	}

	if v, ok := cm.Data[renovateConfigMapIgnoreTestsKey]; ok {
		if parsed, err := strconv.ParseBool(v); err == nil {
			opts.IgnoreTests = parsed
		}
	}

	if v, ok := cm.Data[renovateConfigMapGitLabIgnoreApprovalsKey]; ok {
		if parsed, err := strconv.ParseBool(v); err == nil {
			opts.GitLabIgnoreApprovals = parsed
		}
	}

	if v, ok := cm.Data[renovateConfigMapAutomergeTypeKey]; ok {
		opts.AutomergeType = v
	}

	if v, ok := cm.Data[renovateConfigMapCommitMessagePrefixKey]; ok {
		opts.CommitMessagePrefix = v
	}

	if v, ok := cm.Data[renovateConfigMapCommitMessageSuffixKey]; ok {
		opts.CommitMessageSuffix = v
	}

	if v, ok := cm.Data[renovateConfigMapFileMatchKey]; ok {
		parts := strings.Split(v, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		opts.FileMatch = parts
	}

	if v, ok := cm.Data[renovateConfigMapAutomergeScheduleKey]; ok {
		parts := strings.Split(v, ";")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		opts.AutomergeSchedule = parts
	}

	if v, ok := cm.Data[renovateConfigMapLabelsKey]; ok {
		parts := strings.Split(v, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		opts.Labels = parts
	}

	return opts, nil
}

// RandomString returns a cryptographically random hex string of the given length.
func RandomString(length int) string {
	bytes := make([]byte, length/2+1)
	if _, err := rand.Read(bytes); err != nil {
		panic("failed to read from random generator")
	}
	return hex.EncodeToString(bytes)[:length]
}

func joinNudgedComponentNames(targets []NudgeTarget) string {
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, t.ComponentName)
	}
	return strings.Join(names, " ")
}

var commitHashRegex = regexp.MustCompile(`[a-f0-9]{40}`)

func extractCommitHash(commitUrl string) string {
	parts := strings.Split(commitUrl, "/")
	if len(parts) < 2 {
		return ""
	}
	candidate := parts[len(parts)-1]
	if !commitHashRegex.MatchString(candidate) {
		return ""
	}
	return candidate
}
