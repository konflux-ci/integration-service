/*
Copyright 2022 Red Hat Inc.

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
package snapshot

import (
	"context"

	"github.com/konflux-ci/integration-service/helpers"
	"github.com/konflux-ci/integration-service/loader"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SnapshotOpts struct {
	ctx    context.Context
	client client.Client
	logger helpers.IntegrationLogger
	loader loader.ObjectLoader
}

func NewSnapshotOpts(ctx context.Context, client client.Client, logger helpers.IntegrationLogger, loader loader.ObjectLoader) SnapshotOpts {
	return SnapshotOpts{
		ctx:    ctx,
		client: client,
		logger: logger,
		loader: loader,
	}
}
