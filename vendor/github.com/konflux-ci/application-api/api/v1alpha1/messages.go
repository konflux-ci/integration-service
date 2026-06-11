//
// Copyright 2023 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

const (
	InvalidDNS1035Name = "invalid component name: %q: a component resource name must start with a lower case alphabetical character, be under 63 characters, and can only consist of lower case alphanumeric characters or ‘-’"

	InvalidDNS1123Subdomain = "invalid ingress domain: %q: an ingress domain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character"

	InvalidSchemeGitSourceURL = ": gitsource URL must be an absolute URL starting with an 'https/http' scheme "
	InvalidGithubVendorURL    = "gitsource URL %s must come from a supported vendor: %s"
	InvalidAPIURL             = ": API URL must be an absolute URL starting with an 'https' scheme "

	MissingIngressDomain = "ingress domain cannot be empty if cluster is of type Kubernetes"

	MissingGitOrImageSource = "a git source or an image source must be specified when creating a component"

	ComponentNameUpdateError   = "component name cannot be updated to %s"
	ApplicationNameUpdateError = "application name cannot be updated to %s"
	GitSourceUpdateError       = "git source cannot be updated to %+v"
	InvalidComponentError      = "runtime object is not of type Component"
)
