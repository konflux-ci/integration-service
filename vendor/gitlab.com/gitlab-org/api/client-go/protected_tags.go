//
// Copyright 2021, Sander van Harmelen
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
//

package gitlab

import (
	"fmt"
	"net/http"
)

type (
	ProtectedTagsServiceInterface interface {
		ListProtectedTags(pid any, opt *ListProtectedTagsOptions, options ...RequestOptionFunc) ([]*ProtectedTag, *Response, error)
		GetProtectedTag(pid any, tag string, options ...RequestOptionFunc) (*ProtectedTag, *Response, error)
		ProtectRepositoryTags(pid any, opt *ProtectRepositoryTagsOptions, options ...RequestOptionFunc) (*ProtectedTag, *Response, error)
		UnprotectRepositoryTags(pid any, tag string, options ...RequestOptionFunc) (*Response, error)
	}

	// ProtectedTagsService handles communication with the protected tag methods
	// of the GitLab API.
	//
	// GitLab API docs:
	// https://docs.gitlab.com/api/protected_tags/
	ProtectedTagsService struct {
		client *Client
	}
)

var _ ProtectedTagsServiceInterface = (*ProtectedTagsService)(nil)

// ProtectedTag represents a protected tag.
//
// GitLab API docs:
// https://docs.gitlab.com/api/protected_tags/
type ProtectedTag struct {
	Name               string                  `json:"name"`
	CreateAccessLevels []*TagAccessDescription `json:"create_access_levels"`
}

// TagAccessDescription represents the access description for a protected tag.
//
// GitLab API docs:
// https://docs.gitlab.com/api/protected_tags/
type TagAccessDescription struct {
	ID                     int              `json:"id"`
	UserID                 int              `json:"user_id"`
	GroupID                int              `json:"group_id"`
	AccessLevel            AccessLevelValue `json:"access_level"`
	AccessLevelDescription string           `json:"access_level_description"`
}

// ListProtectedTagsOptions represents the available ListProtectedTags()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/protected_tags/#list-protected-tags
type ListProtectedTagsOptions ListOptions

// ListProtectedTags returns a list of protected tags from a project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/protected_tags/#list-protected-tags
func (s *ProtectedTagsService) ListProtectedTags(pid any, opt *ListProtectedTagsOptions, options ...RequestOptionFunc) ([]*ProtectedTag, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/protected_tags", PathEscape(project))

	req, err := s.client.NewRequest(http.MethodGet, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	var pts []*ProtectedTag
	resp, err := s.client.Do(req, &pts)
	if err != nil {
		return nil, resp, err
	}

	return pts, resp, nil
}

// GetProtectedTag returns a single protected tag or wildcard protected tag.
//
// GitLab API docs:
// https://docs.gitlab.com/api/protected_tags/#get-a-single-protected-tag-or-wildcard-protected-tag
func (s *ProtectedTagsService) GetProtectedTag(pid any, tag string, options ...RequestOptionFunc) (*ProtectedTag, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/protected_tags/%s", PathEscape(project), PathEscape(tag))

	req, err := s.client.NewRequest(http.MethodGet, u, nil, options)
	if err != nil {
		return nil, nil, err
	}

	pt := new(ProtectedTag)
	resp, err := s.client.Do(req, pt)
	if err != nil {
		return nil, resp, err
	}

	return pt, resp, nil
}

// ProtectRepositoryTagsOptions represents the available ProtectRepositoryTags()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/protected_tags/#protect-repository-tags
type ProtectRepositoryTagsOptions struct {
	Name              *string                   `url:"name,omitempty" json:"name,omitempty"`
	CreateAccessLevel *AccessLevelValue         `url:"create_access_level,omitempty" json:"create_access_level,omitempty"`
	AllowedToCreate   *[]*TagsPermissionOptions `url:"allowed_to_create,omitempty" json:"allowed_to_create,omitempty"`
}

// TagsPermissionOptions represents a protected tag permission option.
//
// GitLab API docs:
// https://docs.gitlab.com/api/protected_tags/#protect-repository-tags
type TagsPermissionOptions struct {
	UserID      *int              `url:"user_id,omitempty" json:"user_id,omitempty"`
	GroupID     *int              `url:"group_id,omitempty" json:"group_id,omitempty"`
	AccessLevel *AccessLevelValue `url:"access_level,omitempty" json:"access_level,omitempty"`
}

// ProtectRepositoryTags protects a single repository tag or several project
// repository tags using a wildcard protected tag.
//
// GitLab API docs:
// https://docs.gitlab.com/api/protected_tags/#protect-repository-tags
func (s *ProtectedTagsService) ProtectRepositoryTags(pid any, opt *ProtectRepositoryTagsOptions, options ...RequestOptionFunc) (*ProtectedTag, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/protected_tags", PathEscape(project))

	req, err := s.client.NewRequest(http.MethodPost, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	pt := new(ProtectedTag)
	resp, err := s.client.Do(req, pt)
	if err != nil {
		return nil, resp, err
	}

	return pt, resp, nil
}

// UnprotectRepositoryTags unprotects the given protected tag or wildcard
// protected tag.
//
// GitLab API docs:
// https://docs.gitlab.com/api/protected_tags/#unprotect-repository-tags
func (s *ProtectedTagsService) UnprotectRepositoryTags(pid any, tag string, options ...RequestOptionFunc) (*Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("projects/%s/protected_tags/%s", PathEscape(project), PathEscape(tag))

	req, err := s.client.NewRequest(http.MethodDelete, u, nil, options)
	if err != nil {
		return nil, err
	}

	return s.client.Do(req, nil)
}
