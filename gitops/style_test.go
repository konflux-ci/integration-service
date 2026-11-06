package gitops_test

import (
	"context"

	applicationapiv1alpha1 "github.com/konflux-ci/application-api/api/v1alpha1"
	"github.com/konflux-ci/integration-service/gitops"
	"github.com/konflux-ci/integration-service/pkg/keys"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Owned snapshot metadata", func() {
	It("reads, writes, and deletes either owned style while preserving raw metadata", func() {
		obj := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				keys.Test("value").Old: "old",
				keys.Test("value").New: "new",
			},
			Annotations: map[string]string{
				keys.Test("annotation").Old: "old",
				keys.Test("annotation").New: "new",
			},
		}}

		value, found := gitops.LabelValue(obj, keys.Test("value").Old)
		Expect(found).To(BeTrue())
		Expect(value).To(Equal("new"))
		value, found = gitops.AnnotationValue(obj, keys.Test("annotation").Old)
		Expect(found).To(BeTrue())
		Expect(value).To(Equal("new"))
		Expect(gitops.HasOwnedLabel(obj, keys.Test("value").Old)).To(BeTrue())
		Expect(gitops.HasOwnedAnnotation(obj, keys.Test("annotation").Old)).To(BeTrue())
		Expect(gitops.HasOwnedLabelValue(obj, keys.Test("value").Old, "new")).To(BeTrue())
		Expect(gitops.HasOwnedAnnotationValue(obj, keys.Test("annotation").Old, "new")).To(BeTrue())

		Expect(gitops.SetOwnedLabel(obj, keys.Test("value").Old, "written")).To(Succeed())
		Expect(obj.Labels).To(HaveKeyWithValue(keys.Test("value").New, "written"))
		Expect(obj.Labels).NotTo(HaveKey(keys.Test("value").Old))
		Expect(gitops.SetOwnedAnnotation(obj, keys.Test("annotation").Old, "written")).To(Succeed())
		Expect(obj.Annotations).To(HaveKeyWithValue(keys.Test("annotation").New, "written"))
		Expect(obj.Annotations).NotTo(HaveKey(keys.Test("annotation").Old))

		obj.Labels[keys.Test("value").New] = "new"
		obj.Annotations[keys.Test("annotation").New] = "new"
		Expect(gitops.DeleteOwnedLabel(obj, keys.Test("value").Old)).To(Succeed())
		Expect(gitops.DeleteOwnedAnnotation(obj, keys.Test("annotation").Old)).To(Succeed())
		Expect(obj.Labels).NotTo(HaveKey(keys.Test("value").Old))
		Expect(obj.Labels).NotTo(HaveKey(keys.Test("value").New))
		Expect(obj.Annotations).NotTo(HaveKey(keys.Test("annotation").Old))
		Expect(obj.Annotations).NotTo(HaveKey(keys.Test("annotation").New))

		Expect(gitops.SetOwnedLabel(obj, "custom.example/label", "value")).To(Succeed())
		Expect(gitops.DeleteOwnedLabel(obj, "custom.example/label")).To(Succeed())
		Expect(gitops.SetOwnedAnnotation(obj, "custom.example/annotation", "value")).To(Succeed())
		Expect(gitops.DeleteOwnedAnnotation(obj, "custom.example/annotation")).To(Succeed())
		Expect(gitops.DeleteOwnedLabel(nil, keys.Test("value").Old)).NotTo(Succeed())
		Expect(gitops.DeleteOwnedAnnotation(nil, keys.Test("annotation").Old)).NotTo(Succeed())
	})

	It("writes owned metadata in the object's existing style", func() {
		oldObject := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Labels:      map[string]string{keys.Test("style").Old: "old"},
			Annotations: map[string]string{keys.Test("style").Old: "old"},
		}}
		newObject := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{
			Labels:      map[string]string{keys.Test("style").New: "new"},
			Annotations: map[string]string{keys.Test("style").New: "new"},
		}}

		Expect(gitops.SetOwnedLabel(oldObject, keys.Test("style").Old, "written")).To(Succeed())
		Expect(oldObject.Labels).To(HaveKeyWithValue(keys.Test("style").Old, "written"))
		Expect(oldObject.Labels).NotTo(HaveKey(keys.Test("style").New))
		Expect(gitops.SetOwnedAnnotation(oldObject, keys.Test("style").Old, "written")).To(Succeed())
		Expect(oldObject.Annotations).To(HaveKeyWithValue(keys.Test("style").Old, "written"))
		Expect(oldObject.Annotations).NotTo(HaveKey(keys.Test("style").New))

		Expect(gitops.SetOwnedLabel(newObject, keys.Test("style").Old, "written")).To(Succeed())
		Expect(newObject.Labels).To(HaveKeyWithValue(keys.Test("style").New, "written"))
		Expect(newObject.Labels).NotTo(HaveKey(keys.Test("style").Old))
		Expect(gitops.SetOwnedAnnotation(newObject, keys.Test("style").Old, "written")).To(Succeed())
		Expect(newObject.Annotations).To(HaveKeyWithValue(keys.Test("style").New, "written"))
		Expect(newObject.Annotations).NotTo(HaveKey(keys.Test("style").Old))
	})

	It("AnnotateSnapshot writes new-style metadata for a new-style Snapshot", func() {
		snapshot := &applicationapiv1alpha1.Snapshot{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "konflux-ci.dev/v1alpha1",
				Kind:       "Snapshot",
			},
			ObjectMeta: metav1.ObjectMeta{Name: "snapshot", Namespace: "default"},
		}
		scheme := runtime.NewScheme()
		Expect(applicationapiv1alpha1.AddToScheme(scheme)).To(Succeed())
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snapshot).Build()

		Expect(gitops.AnnotateSnapshot(context.Background(), snapshot, keys.Test("status").Old, "passed", cl)).To(Succeed())
		Expect(snapshot.Annotations).To(HaveKeyWithValue(keys.Test("status").New, "passed"))
		Expect(snapshot.Annotations).NotTo(HaveKey(keys.Test("status").Old))
	})

	It("AnnotateComponent writes new-style metadata for a new-style Component", func() {
		component := &applicationapiv1alpha1.Component{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "konflux-ci.dev/v1alpha1",
				Kind:       "Component",
			},
			ObjectMeta: metav1.ObjectMeta{Name: "component", Namespace: "default"},
		}
		scheme := runtime.NewScheme()
		Expect(applicationapiv1alpha1.AddToScheme(scheme)).To(Succeed())
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(component).Build()

		Expect(gitops.AnnotateComponent(context.Background(), component, keys.Test("status").Old, "passed", cl)).To(Succeed())
		Expect(component.Annotations).To(HaveKeyWithValue(keys.Test("status").New, "passed"))
		Expect(component.Annotations).NotTo(HaveKey(keys.Test("status").Old))
	})
})
