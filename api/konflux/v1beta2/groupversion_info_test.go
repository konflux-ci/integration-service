package v1beta2

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestAddToSchemeRegistersKonfluxResources(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	tests := []struct {
		name string
		obj  runtime.Object
		want schema.GroupVersionKind
	}{
		{
			name: "ComponentGroup",
			obj:  &ComponentGroup{},
			want: GroupVersion.WithKind("ComponentGroup"),
		},
		{
			name: "NudgeConfig",
			obj:  &NudgeConfig{},
			want: GroupVersion.WithKind("NudgeConfig"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gvks, _, err := scheme.ObjectKinds(tt.obj)
			if err != nil {
				t.Fatalf("ObjectKinds() error = %v", err)
			}
			if len(gvks) != 1 || gvks[0] != tt.want {
				t.Fatalf("ObjectKinds() = %v, want [%v]", gvks, tt.want)
			}
		})
	}
}
