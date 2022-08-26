package tekton_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTekton(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Tekton Suite")
}
