/*
Copyright 2026 Red Hat Inc.

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

package buildpipeline

import (
	"context"

	"github.com/konflux-ci/integration-service/loader"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type countingPipelineRunLoader struct {
	loader.ObjectLoader
	calls     int
	failAfter int
	failErr   error
}

func (c *countingPipelineRunLoader) GetPipelineRun(ctx context.Context, k8sClient client.Client, name, namespace string) (*tektonv1.PipelineRun, error) {
	c.calls++
	if c.failAfter > 0 && c.calls >= c.failAfter {
		return nil, c.failErr
	}
	return c.ObjectLoader.GetPipelineRun(ctx, k8sClient, name, namespace)
}
