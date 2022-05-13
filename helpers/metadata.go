package helpers

import "sigs.k8s.io/controller-runtime/pkg/client"

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
