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
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type (
	RepositoriesServiceInterface interface {
		ListTree(pid any, opt *ListTreeOptions, options ...RequestOptionFunc) ([]*TreeNode, *Response, error)
		Blob(pid any, sha string, options ...RequestOptionFunc) ([]byte, *Response, error)
		RawBlobContent(pid any, sha string, options ...RequestOptionFunc) ([]byte, *Response, error)
		Archive(pid any, opt *ArchiveOptions, options ...RequestOptionFunc) ([]byte, *Response, error)
		StreamArchive(pid any, w io.Writer, opt *ArchiveOptions, options ...RequestOptionFunc) (*Response, error)
		Compare(pid any, opt *CompareOptions, options ...RequestOptionFunc) (*Compare, *Response, error)
		Contributors(pid any, opt *ListContributorsOptions, options ...RequestOptionFunc) ([]*Contributor, *Response, error)
		MergeBase(pid any, opt *MergeBaseOptions, options ...RequestOptionFunc) (*Commit, *Response, error)
		AddChangelog(pid any, opt *AddChangelogOptions, options ...RequestOptionFunc) (*Response, error)
		GenerateChangelogData(pid any, opt GenerateChangelogDataOptions, options ...RequestOptionFunc) (*ChangelogData, *Response, error)
	}

	// RepositoriesService handles communication with the repositories related
	// methods of the GitLab API.
	//
	// GitLab API docs: https://docs.gitlab.com/api/repositories/
	RepositoriesService struct {
		client *Client
	}
)

var _ RepositoriesServiceInterface = (*RepositoriesService)(nil)

// TreeNode represents a GitLab repository file or directory.
//
// GitLab API docs: https://docs.gitlab.com/api/repositories/
type TreeNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
	Mode string `json:"mode"`
}

func (t TreeNode) String() string {
	return Stringify(t)
}

// ListTreeOptions represents the available ListTree() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#list-repository-tree
type ListTreeOptions struct {
	ListOptions
	Path      *string `url:"path,omitempty" json:"path,omitempty"`
	Ref       *string `url:"ref,omitempty" json:"ref,omitempty"`
	Recursive *bool   `url:"recursive,omitempty" json:"recursive,omitempty"`
}

// ListTree gets a list of repository files and directories in a project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#list-repository-tree
func (s *RepositoriesService) ListTree(pid any, opt *ListTreeOptions, options ...RequestOptionFunc) ([]*TreeNode, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/tree", PathEscape(project))

	req, err := s.client.NewRequest(http.MethodGet, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	var t []*TreeNode
	resp, err := s.client.Do(req, &t)
	if err != nil {
		return nil, resp, err
	}

	return t, resp, nil
}

// Blob gets information about blob in repository like size and content. Note
// that blob content is Base64 encoded.
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#get-a-blob-from-repository
func (s *RepositoriesService) Blob(pid any, sha string, options ...RequestOptionFunc) ([]byte, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/blobs/%s", PathEscape(project), url.PathEscape(sha))

	req, err := s.client.NewRequest(http.MethodGet, u, nil, options)
	if err != nil {
		return nil, nil, err
	}

	var b bytes.Buffer
	resp, err := s.client.Do(req, &b)
	if err != nil {
		return nil, resp, err
	}

	return b.Bytes(), resp, err
}

// RawBlobContent gets the raw file contents for a blob by blob SHA.
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#raw-blob-content
func (s *RepositoriesService) RawBlobContent(pid any, sha string, options ...RequestOptionFunc) ([]byte, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/blobs/%s/raw", PathEscape(project), url.PathEscape(sha))

	req, err := s.client.NewRequest(http.MethodGet, u, nil, options)
	if err != nil {
		return nil, nil, err
	}

	var b bytes.Buffer
	resp, err := s.client.Do(req, &b)
	if err != nil {
		return nil, resp, err
	}

	return b.Bytes(), resp, err
}

// ArchiveOptions represents the available Archive() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#get-file-archive
type ArchiveOptions struct {
	Format *string `url:"-" json:"-"`
	Path   *string `url:"path,omitempty" json:"path,omitempty"`
	SHA    *string `url:"sha,omitempty" json:"sha,omitempty"`
}

// Archive gets an archive of the repository.
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#get-file-archive
func (s *RepositoriesService) Archive(pid any, opt *ArchiveOptions, options ...RequestOptionFunc) ([]byte, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/archive", PathEscape(project))

	// Set an optional format for the archive.
	if opt != nil && opt.Format != nil {
		u = fmt.Sprintf("%s.%s", u, *opt.Format)
	}

	req, err := s.client.NewRequest(http.MethodGet, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	var b bytes.Buffer
	resp, err := s.client.Do(req, &b)
	if err != nil {
		return nil, resp, err
	}

	return b.Bytes(), resp, err
}

// StreamArchive streams an archive of the repository to the provided
// io.Writer.
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#get-file-archive
func (s *RepositoriesService) StreamArchive(pid any, w io.Writer, opt *ArchiveOptions, options ...RequestOptionFunc) (*Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/archive", PathEscape(project))

	// Set an optional format for the archive.
	if opt != nil && opt.Format != nil {
		u = fmt.Sprintf("%s.%s", u, *opt.Format)
	}

	req, err := s.client.NewRequest(http.MethodGet, u, opt, options)
	if err != nil {
		return nil, err
	}

	return s.client.Do(req, w)
}

// Compare represents the result of a comparison of branches, tags or commits.
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#compare-branches-tags-or-commits
type Compare struct {
	Commit         *Commit   `json:"commit"`
	Commits        []*Commit `json:"commits"`
	Diffs          []*Diff   `json:"diffs"`
	CompareTimeout bool      `json:"compare_timeout"`
	CompareSameRef bool      `json:"compare_same_ref"`
	WebURL         string    `json:"web_url"`
}

func (c Compare) String() string {
	return Stringify(c)
}

// CompareOptions represents the available Compare() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#compare-branches-tags-or-commits
type CompareOptions struct {
	From     *string `url:"from,omitempty" json:"from,omitempty"`
	To       *string `url:"to,omitempty" json:"to,omitempty"`
	Straight *bool   `url:"straight,omitempty" json:"straight,omitempty"`
	Unidiff  *bool   `url:"unidiff,omitempty" json:"unidiff,omitempty"`
}

// Compare compares branches, tags or commits.
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#compare-branches-tags-or-commits
func (s *RepositoriesService) Compare(pid any, opt *CompareOptions, options ...RequestOptionFunc) (*Compare, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/compare", PathEscape(project))

	req, err := s.client.NewRequest(http.MethodGet, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	c := new(Compare)
	resp, err := s.client.Do(req, c)
	if err != nil {
		return nil, resp, err
	}

	return c, resp, nil
}

// Contributor represents a GitLab contributor.
//
// GitLab API docs: https://docs.gitlab.com/api/repositories/#contributors
type Contributor struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Commits   int    `json:"commits"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

func (c Contributor) String() string {
	return Stringify(c)
}

// ListContributorsOptions represents the available ListContributors() options.
//
// GitLab API docs: https://docs.gitlab.com/api/repositories/#contributors
type ListContributorsOptions struct {
	ListOptions
	OrderBy *string `url:"order_by,omitempty" json:"order_by,omitempty"`
	Sort    *string `url:"sort,omitempty" json:"sort,omitempty"`
}

// Contributors gets the repository contributors list.
//
// GitLab API docs: https://docs.gitlab.com/api/repositories/#contributors
func (s *RepositoriesService) Contributors(pid any, opt *ListContributorsOptions, options ...RequestOptionFunc) ([]*Contributor, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/contributors", PathEscape(project))

	req, err := s.client.NewRequest(http.MethodGet, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	var c []*Contributor
	resp, err := s.client.Do(req, &c)
	if err != nil {
		return nil, resp, err
	}

	return c, resp, nil
}

// MergeBaseOptions represents the available MergeBase() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#merge-base
type MergeBaseOptions struct {
	Ref *[]string `url:"refs[],omitempty" json:"refs,omitempty"`
}

// MergeBase gets the common ancestor for 2 refs (commit SHAs, branch
// names or tags).
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#merge-base
func (s *RepositoriesService) MergeBase(pid any, opt *MergeBaseOptions, options ...RequestOptionFunc) (*Commit, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/merge_base", PathEscape(project))

	req, err := s.client.NewRequest(http.MethodGet, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	c := new(Commit)
	resp, err := s.client.Do(req, c)
	if err != nil {
		return nil, resp, err
	}

	return c, resp, nil
}

// AddChangelogOptions represents the available AddChangelog() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#add-changelog-data-to-a-changelog-file
type AddChangelogOptions struct {
	Version    *string  `url:"version,omitempty" json:"version,omitempty"`
	Branch     *string  `url:"branch,omitempty" json:"branch,omitempty"`
	ConfigFile *string  `url:"config_file,omitempty" json:"config_file,omitempty"`
	Date       *ISOTime `url:"date,omitempty" json:"date,omitempty"`
	File       *string  `url:"file,omitempty" json:"file,omitempty"`
	From       *string  `url:"from,omitempty" json:"from,omitempty"`
	Message    *string  `url:"message,omitempty" json:"message,omitempty"`
	To         *string  `url:"to,omitempty" json:"to,omitempty"`
	Trailer    *string  `url:"trailer,omitempty" json:"trailer,omitempty"`
}

// AddChangelog generates changelog data based on commits in a repository.
//
// Gitlab API docs:
// https://docs.gitlab.com/api/repositories/#add-changelog-data-to-a-changelog-file
func (s *RepositoriesService) AddChangelog(pid any, opt *AddChangelogOptions, options ...RequestOptionFunc) (*Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/changelog", PathEscape(project))

	req, err := s.client.NewRequest(http.MethodPost, u, opt, options)
	if err != nil {
		return nil, err
	}

	return s.client.Do(req, nil)
}

// ChangelogData represents the generated changelog data.
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#generate-changelog-data
type ChangelogData struct {
	Notes string `json:"notes"`
}

func (c ChangelogData) String() string {
	return Stringify(c)
}

// GenerateChangelogDataOptions represents the available GenerateChangelogData()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/repositories/#generate-changelog-data
type GenerateChangelogDataOptions struct {
	Version    *string  `url:"version,omitempty" json:"version,omitempty"`
	ConfigFile *string  `url:"config_file,omitempty" json:"config_file,omitempty"`
	Date       *ISOTime `url:"date,omitempty" json:"date,omitempty"`
	From       *string  `url:"from,omitempty" json:"from,omitempty"`
	To         *string  `url:"to,omitempty" json:"to,omitempty"`
	Trailer    *string  `url:"trailer,omitempty" json:"trailer,omitempty"`
}

// GenerateChangelogData generates changelog data based on commits in a
// repository, without committing them to a changelog file.
//
// Gitlab API docs:
// https://docs.gitlab.com/api/repositories/#generate-changelog-data
func (s *RepositoriesService) GenerateChangelogData(pid any, opt GenerateChangelogDataOptions, options ...RequestOptionFunc) (*ChangelogData, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/repository/changelog", project)

	req, err := s.client.NewRequest(http.MethodGet, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	cd := new(ChangelogData)
	resp, err := s.client.Do(req, cd)
	if err != nil {
		return nil, resp, err
	}

	return cd, resp, nil
}
