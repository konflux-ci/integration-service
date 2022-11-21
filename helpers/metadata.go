package helpers

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HasAnnotation checks if a given annotation exists
func HasAnnotation(object client.Object, annotation string) bool {
	_, found := object.GetAnnotations()[annotation]

	return found
}

// HasAnnotationWithValue checks if an annotation exists and has the given value
func HasAnnotationWithValue(object client.Object, annotation, value string) bool {
	if labelValue, found := object.GetAnnotations()[annotation]; found && labelValue == value {
		return true
	}

	return false
}

// HasLabel checks if a given label exists
func HasLabel(object client.Object, label string) bool {
	_, found := object.GetLabels()[label]

	return found
}

// HasLabelWithValue checks if a label exists and has the given value
func HasLabelWithValue(object client.Object, label, value string) bool {
	if labelValue, found := object.GetLabels()[label]; found && labelValue == value {
		return true
	}

	return false
}

// CopyLabelsByPrefix copies all labels from a source object to a destination object where the key matches the specified prefix.
// If replacementPrefix is different from prefix, the prefix will be replaced while performing the copy.
func CopyLabelsByPrefix(src, dest *metav1.ObjectMeta, prefix, replacementPrefix string) {
	if src.GetLabels() == nil {
		return
	}
	if dest.GetLabels() == nil {
		dest.SetLabels(make(map[string]string))
	}
	for key, value := range src.GetLabels() {
		if strings.HasPrefix(key, prefix) {
			newKey := key
			if prefix != replacementPrefix {
				newKey = strings.Replace(key, prefix, replacementPrefix, 1)
			}
			dest.Labels[newKey] = value
		}
	}
}

// CopyAnnotationsByPrefix copies all annotations from a source object to a destination object where the key matches the specified prefix.
// If replacementPrefix is different from prefix, the prefix will be replaced while performing the copy.
func CopyAnnotationsByPrefix(src, dest *metav1.ObjectMeta, prefix, replacementPrefix string) {
	if src.GetAnnotations() == nil {
		return
	}
	if dest.GetAnnotations() == nil {
		dest.SetAnnotations(make(map[string]string))
	}
	for key, value := range src.GetAnnotations() {
		if strings.HasPrefix(key, prefix) {
			newKey := key
			if prefix != replacementPrefix {
				newKey = strings.Replace(key, prefix, replacementPrefix, 1)
			}
			dest.Annotations[newKey] = value
		}
	}
}
