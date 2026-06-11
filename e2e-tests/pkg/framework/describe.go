package framework

import (
	ginkgo "github.com/onsi/ginkgo/v2"
)

func IntegrationServiceSuiteDescribe(text string, args ...interface{}) bool {
	return ginkgo.Describe("[integration-service-suite "+text+"]", args, ginkgo.Ordered)
}
