package tekton_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	hasv1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/api/v1alpha1"
	"github.com/redhat-appstudio/integration-service/gitops"
	tekton "github.com/redhat-appstudio/integration-service/tekton"
	appstudioshared "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
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
		hasApp                    *hasv1alpha1.Application
		hasSnapshot               *appstudioshared.ApplicationSnapshot
		hasComp                   *hasv1alpha1.Component
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
					Name:   "envname",
					Type:   "POC",
					Params: []string{},
				},
			},
		}
		Expect(k8sClient.Create(ctx, integrationTestScenario)).Should(Succeed())

		//create new integration pipeline run from integration test scenario
		newIntegrationPipelineRun = tekton.NewIntegrationPipelineRun(prefix, namespace, *integrationTestScenario)
		Expect(k8sClient.Create(ctx, newIntegrationPipelineRun.AsPipelineRun())).Should(Succeed())

		hasApp = &hasv1alpha1.Application{
			ObjectMeta: metav1.ObjectMeta{
				Name:      applicationName,
				Namespace: namespace,
			},
			Spec: hasv1alpha1.ApplicationSpec{
				DisplayName: "application-sample",
				Description: "This is an example application",
			},
		}
		Expect(k8sClient.Create(ctx, hasApp)).Should(Succeed())

		hasSnapshot = &appstudioshared.ApplicationSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "snapshot-sample",
				Namespace: "default",
				Labels: map[string]string{
					gitops.ApplicationSnapshotTypeLabel:      "component",
					gitops.ApplicationSnapshotComponentLabel: "component-sample",
				},
			},
			Spec: appstudioshared.ApplicationSnapshotSpec{
				Application: hasApp.Name,
				Components: []appstudioshared.ApplicationSnapshotComponent{
					{
						Name:           "component-sample",
						ContainerImage: "testimage",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, hasSnapshot)).Should(Succeed())

		hasComp = &hasv1alpha1.Component{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "component-sample",
				Namespace: "default",
			},
			Spec: hasv1alpha1.ComponentSpec{
				ComponentName:  "component-sample",
				Application:    "application-sample",
				ContainerImage: "",
				Source: hasv1alpha1.ComponentSource{
					ComponentSourceUnion: hasv1alpha1.ComponentSourceUnion{
						GitSource: &hasv1alpha1.GitSource{
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

		It("can append the scenario Name and Namespace to a IntegrationPipelineRun object and that these label key names match the correct label format", func() {
			newIntegrationPipelineRun.WithIntegrationLabels(integrationTestScenario)
			Expect(newIntegrationPipelineRun.Labels["test.appstudio.openshift.io/scenario"]).
				To(Equal(integrationTestScenario.Name))
			Expect(newIntegrationPipelineRun.Labels["pipelines.appstudio.openshift.io/type"]).
				To(Equal("test"))
			Expect(newIntegrationPipelineRun.Namespace).
				To(Equal(integrationTestScenario.Namespace))
		})

		It("can append labels that comes from ApplicationSnapshot to IntegrationPipelineRun and make sure that label value matches the snapshot name", func() {
			newIntegrationPipelineRun.WithApplicationSnapshot(hasSnapshot)
			Expect(newIntegrationPipelineRun.Labels["test.appstudio.openshift.io/snapshot"]).
				To(Equal(hasSnapshot.Name))
		})

		It("can append labels comming from Application and Component to IntegrationPipelineRun and making sure that label values matches application and component names", func() {
			newIntegrationPipelineRun.WithApplicationAndComponent(hasApp, hasComp)
			Expect(newIntegrationPipelineRun.Labels["test.appstudio.openshift.io/component"]).
				To(Equal(hasComp.Name))
			Expect(newIntegrationPipelineRun.Labels["test.appstudio.openshift.io/application"]).
				To(Equal(hasApp.Name))
		})

	})

})
