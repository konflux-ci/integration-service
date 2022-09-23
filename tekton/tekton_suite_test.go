package tekton_test

/*
Copyright 2022.
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

import (
	"context"
	"go/build"
	"path/filepath"
	"testing"

	"k8s.io/client-go/rest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	ctrl "sigs.k8s.io/controller-runtime"

	appstudiov1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	integrationalpha1 "github.com/redhat-appstudio/integration-service/api/v1alpha1"
	appstudioshared "github.com/redhat-appstudio/managed-gitops/appstudio-shared/apis/appstudio.redhat.com/v1alpha1"
	releasev1alpha1 "github.com/redhat-appstudio/release-service/api/v1alpha1"
	appstudiotest "github.com/redhat-appstudio/release-service/test"
	tektonv1beta1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestControllerSnapshot(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tekton Test Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx, cancel = context.WithCancel(context.TODO())

	//adding required CRDs
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "config", "crd", "bases"),
			filepath.Join(
				build.Default.GOPATH,
				"pkg", "mod", "github.com", "tektoncd",
				"pipeline@v0.35.0", "config",
			),
			filepath.Join(
				build.Default.GOPATH,
				"pkg", "mod", appstudiotest.GetRelativeDependencyPath("application-service"), "config", "crd", "bases",
			),
			filepath.Join(
				build.Default.GOPATH,
				"pkg", "mod", appstudiotest.GetRelativeDependencyPath("managed-gitops"), "config", "crd", "bases",
			),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	Expect(appstudiov1alpha1.AddToScheme(clientsetscheme.Scheme)).To(Succeed())
	Expect(tektonv1beta1.AddToScheme(clientsetscheme.Scheme)).To(Succeed())
	Expect(appstudioshared.AddToScheme(clientsetscheme.Scheme)).To(Succeed())
	Expect(releasev1alpha1.AddToScheme(clientsetscheme.Scheme)).To(Succeed())
	Expect(integrationalpha1.AddToScheme(clientsetscheme.Scheme)).To(Succeed())

	k8sManager, _ := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             clientsetscheme.Scheme,
		MetricsBindAddress: "0", // this disables metrics
		LeaderElection:     false,
	})

	k8sClient = k8sManager.GetClient()
	go func() {
		defer GinkgoRecover()
		Expect(k8sManager.Start(ctx)).To(Succeed())
	}()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
