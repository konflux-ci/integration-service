package tekton_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	tekton "github.com/redhat-appstudio/integration-service/tekton"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ExtraParams struct {
	Name  string
	Value tektonv1beta1.ArrayOrString
}

var _ = Describe("Integration pipeline", func() {

	const (
		prefix                  = "testpipeline"
		namespace               = "default"
		PipelineTypeIntegration = "integration"
		applicationName         = "application-sample"
		SampleRepoLink          = "https://github.com/devfile-samples/devfile-sample-java-springboot-basic"
	)
	var (
		hasApp                    *applicationapiv1alpha1.Application
		hasSnapshot               *applicationapiv1alpha1.Snapshot
		hasComp                   *applicationapiv1alpha1.Component
		newIntegrationPipelineRun *tekton.IntegrationPipelineRun
		integrationTestScenario   *v1alpha1.IntegrationTestScenario
		extraParams               *ExtraParams
	)

	BeforeEach(func() {

		extraParams = &ExtraParams{
			Name: "extraConfigPath",
			Value: tektonv1beta1.ArrayOrString{
				Type:      tektonv1beta1.ParamTypeString,
				StringVal: "path/to/extra/config.yaml",
			},
		}

		integrationTestScenario = &v1alpha1.IntegrationTestScenario{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "example-pass",
				Namespace: "default",

				Labels: map[string]string{
					"test.appstudio.openshift.io/optional": "false",
				},
			},
			Spec: v1alpha1.IntegrationTestScenarioSpec{
				Application: "application-sample",
				Bundle:      "quay.io/kpavic/test-bundle:component-pipeline-pass",
				Pipeline:    "component-pipeline-pass",
				Environment: v1alpha1.TestEnvironment{
					Name: "envname",
					Type: "POC",
					//Params: []string{},
					Configuration: applicationapiv1alpha1.EnvironmentConfiguration{
						Env: []applicationapiv1alpha1.EnvVarPair{},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())

		//create new integration pipeline run from integration test scenario
		newIntegrationPipelineRun = tekton.NewIntegrationPipelineRun(prefix, namespace, *integrationTestScenario)
		Expect(k8sClient.Create(ctx, newIntegrationPipelineRun.AsPipelineRun())).Should(Succeed())

		hasApp = &applicationapiv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      applicationName,
				Namespace: namespace,
			},
			Spec: applicationapiv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

		hasSnapshot = &applicationapiv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.SnapshotTypeLabel:      "component",
					gitops.SnapshotComponentLabel: "component-sample",
				},
			},
			Spec: applicationapiv1alpha1.SnapshotSpec{
				Application: hasApp.Name,
				Components: []applicationapiv1alpha1.SnapshotComponent{
					{
						Name:           "component-sample",
						ContainerImage: "testimage",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		hasComp = &applicationapiv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-sample",
				Namespace: "default",
			},
			Spec: applicationapiv1alpha1.ComponentSpec{
				ComponentName:  "component-sample",
				Application:    "application-sample",
				ContainerImage: "",
				Source: applicationapiv1alpha1.ComponentSource{
					ComponentSourceUnion: applicationapiv1alpha1.ComponentSourceUnion{
						GitSource: &applicationapiv1alpha1.GitSource{
							URL: SampleRepoLink,
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasComp)).Should(Succeed())
	})

	AfterEach(func() {
		_ = k8sClient.Delete(ctx, integrationTestScenario)
		_ = k8sClient.Delete(ctx, hasApp)
		_ = k8sClient.Delete(ctx, hasSnapshot)
		_ = k8sClient.Delete(ctx, hasComp)
		_ = k8sClient.Delete(ctx, newIntegrationPipelineRun.AsPipelineRun())
	})

	Context("When managing a new IntegrationPipelineRun", func() {
		It("can create a IntegrationPipelineRun and the returned object name is prefixed with the provided GenerateName", func() {
			Expect(newIntegrationPipelineRun.ObjectMeta.Name).
				Should(HavePrefix(prefix))
			Expect(newIntegrationPipelineRun.ObjectMeta.Namespace).To(Equal(namespace))
		})

		It("can append extra params to IntegrationPipelineRun and these parameters are present in the object Specs", func() {
			newIntegrationPipelineRun.WithExtraParam(extraParams.Name, extraParams.Value)
			Expect(newIntegrationPipelineRun.Spec.Params[0].Name).To(Equal(extraParams.Name))
			Expect(newIntegrationPipelineRun.Spec.Params[0].Value.StringVal).
				To(Equal(extraParams.Value.StringVal))
		})

		It("can append the scenario Name, optional flag and Namespace to a IntegrationPipelineRun object and that these label key names match the correct label format", func() {
			newIntegrationPipelineRun.WithIntegrationLabels(integrationTestScenario)
			Expect(newIntegrationPipelineRun.Labels["test.appstudio.openshift.io/scenario"]).
				To(Equal(integrationTestScenario.Name))
			Expect(newIntegrationPipelineRun.Labels["pipelines.appstudio.openshift.io/type"]).
				To(Equal("test"))
			Expect(newIntegrationPipelineRun.Labels["test.appstudio.openshift.io/optional"]).
				To(Equal("false"))
			Expect(newIntegrationPipelineRun.Namespace).
				To(Equal(integrationTestScenario.Namespace))
		})

		It("can append labels that comes from Snapshot to IntegrationPipelineRun and make sure that label value matches the snapshot name", func() {
			newIntegrationPipelineRun.WithSnapshot(hasSnapshot)
			Expect(newIntegrationPipelineRun.Labels["appstudio.openshift.io/snapshot"]).
				To(Equal(hasSnapshot.Name))
		})

		It("can append labels comming from Application and Component to IntegrationPipelineRun and making sure that label values matches application and component names", func() {
			newIntegrationPipelineRun.WithApplicationAndComponent(hasApp, hasComp)
			Expect(newIntegrationPipelineRun.Labels["appstudio.openshift.io/component"]).
				To(Equal(hasComp.Name))
			Expect(newIntegrationPipelineRun.Labels["appstudio.openshift.io/application"]).
				To(Equal(hasApp.Name))
		})

		It("provides parameters from IntegrationTestScenario to the PipelineRun", func() {
			scenarioParams := []v1alpha1.PipelineParameter{
				{
					Name:  "ADDITIONAL_PARAMETER",
					Value: "custom value",
				},
				{
					Name:   "MULTIVALUE_PARAMETER",
					Values: []string{"value1", "value2"},
				},
			}

			newIntegrationPipelineRun.WithExtraParams(scenarioParams)
			Expect(newIntegrationPipelineRun.Spec.Params[0].Name).To(Equal(scenarioParams[0].Name))
			Expect(newIntegrationPipelineRun.Spec.Params[0].Value.StringVal).To(Equal(scenarioParams[0].Value))
			Expect(newIntegrationPipelineRun.Spec.Params[1].Name).To(Equal(scenarioParams[1].Name))
			Expect(newIntegrationPipelineRun.Spec.Params[1].Value.ArrayVal).To(Equal(scenarioParams[1].Values))
		})

	})

})
