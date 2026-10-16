/*
Copyright 2024 Red Hat Inc.

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

package oci

import (
	"encoding/json"
	"fmt"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
)

// FetchPipelineRunFromOCIArtifact fetches a PipelineRun JSON from an OCI artifact
// and returns the unmarshaled PipelineRun struct. The OCI artifact should contain
// a PipelineRun JSON file as its layer content.
func FetchPipelineRunFromOCIArtifact(imageName, tag string) (*tektonv1.PipelineRun, error) {
	ref, err := name.ParseReference(imageName + ":" + tag)
	if err != nil {
		return nil, fmt.Errorf("parse image reference: %w", err)
	}

	// Referrers are attached to the manifest identified by the tag, so resolve
	// the tag before looking for PipelineRun artifacts.
	descriptor, err := remote.Head(ref)
	if err != nil {
		return nil, fmt.Errorf("resolve image reference: %w", err)
	}

	referrers, err := remote.Referrers(
		ref.Context().Digest(descriptor.Digest.String()),
		remote.WithFilter("artifactType", "application/vnd.appstudio.pipelinerun+json"),
	)
	if err != nil {
		return nil, fmt.Errorf("discover PipelineRun artifacts: %w", err)
	}

	manifest, err := referrers.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("read PipelineRun artifacts: %w", err)
	}
	if len(manifest.Manifests) == 0 {
		return nil, fmt.Errorf("no PipelineRun artifact found for %s", ref.Name())
	}

	// Registries return referrers in discovery order; the first matching
	// artifact is the latest one, as with `oras discover`.
	artifactRef := ref.Context().Digest(manifest.Manifests[0].Digest.String())
	artifact, err := remote.Get(artifactRef)
	if err != nil {
		return nil, fmt.Errorf("fetch PipelineRun artifact: %w", err)
	}

	image, err := artifact.Image()
	if err != nil {
		return nil, fmt.Errorf("read PipelineRun artifact manifest: %w", err)
	}
	layers, err := image.Layers()
	if err != nil {
		return nil, fmt.Errorf("read PipelineRun artifact layers: %w", err)
	}
	if len(layers) == 0 {
		return nil, fmt.Errorf("PipelineRun artifact contains no layers")
	}

	content, err := layers[0].Uncompressed()
	if err != nil {
		return nil, fmt.Errorf("open PipelineRun layer: %w", err)
	}
	defer content.Close()

	pipelineRun := new(tektonv1.PipelineRun)
	if err := json.NewDecoder(content).Decode(pipelineRun); err != nil {
		return nil, fmt.Errorf("unmarshal PipelineRun JSON: %w", err)
	}
	return pipelineRun, nil
}

// UpdatePipelineRunNamespace updates the namespace of a PipelineRun struct.
// This mutates the PipelineRun in-place.
func UpdatePipelineRunNamespace(plr *tektonv1.PipelineRun, namespace string) {
	plr.Namespace = namespace
}
