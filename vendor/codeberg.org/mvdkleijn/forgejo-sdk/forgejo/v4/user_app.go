// Copyright 2024 The Forgejo Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package forgejo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// AccessTokenScope represents the scope for an access token.
type AccessTokenScope string

const (
	// specials
	AccessTokenScopeAll        AccessTokenScope = "all"
	AccessTokenScopePublicOnly AccessTokenScope = "public-only"
	AccessTokenScopeSudo       AccessTokenScope = "sudo"

	// normal scopes
	AccessTokenScopeActivitypubRead   AccessTokenScope = "read:activitypub"
	AccessTokenScopeActivitypubWrite  AccessTokenScope = "write:activitypub"
	AccessTokenScopeAdminRead         AccessTokenScope = "read:admin"
	AccessTokenScopeAdminWrite        AccessTokenScope = "write:admin"
	AccessTokenScopeIssueRead         AccessTokenScope = "read:issue"
	AccessTokenScopeIssueWrite        AccessTokenScope = "write:issue"
	AccessTokenScopeMiscRead          AccessTokenScope = "read:misc"
	AccessTokenScopeMiscWrite         AccessTokenScope = "write:misc"
	AccessTokenScopeNotificationRead  AccessTokenScope = "read:notification"
	AccessTokenScopeNotificationWrite AccessTokenScope = "write:notification"
	AccessTokenScopeOrganizationRead  AccessTokenScope = "read:organization"
	AccessTokenScopeOrganizationWrite AccessTokenScope = "write:organization"
	AccessTokenScopePackageRead       AccessTokenScope = "read:package"
	AccessTokenScopePackageWrite      AccessTokenScope = "write:package"
	AccessTokenScopeRepositoryRead    AccessTokenScope = "read:repository"
	AccessTokenScopeRepositoryWrite   AccessTokenScope = "write:repository"
	AccessTokenScopeUserRead          AccessTokenScope = "read:user"
	AccessTokenScopeUserWrite         AccessTokenScope = "write:user"
)

// AccessToken represents an API access token.
type AccessToken struct {
	ID             int64              `json:"id"`
	Name           string             `json:"name"`
	Token          string             `json:"sha1"`
	TokenLastEight string             `json:"token_last_eight"`
	Scopes         []AccessTokenScope `json:"scopes"`
	Repositories   []RepositoryMeta   `json:"repositories"`
}

// ListAccessTokensOptions options for listing a users's access tokens
type ListAccessTokensOptions struct {
	ListOptions
}

// ListAccessTokens lists all the access tokens of user
func (c *Client) ListAccessTokens(user string, opts ListAccessTokensOptions) ([]*AccessToken, *Response, error) {
	opts.setDefaults()
	tokens := make([]*AccessToken, 0, opts.PageSize)
	resp, err := c.getParsedResponse("GET", fmt.Sprintf("/users/%s/tokens?%s", url.PathEscape(user), opts.getURLQuery().Encode()), jsonHeader, nil, &tokens)
	return tokens, resp, err
}

// RepoTargetOption options when create access token
type RepoTargetOption struct {
	Name  string `json:"name"`
	Owner string `json:"owner"`
}

// CreateAccessTokenOption options when create access token
type CreateAccessTokenOption struct {
	Name         string             `json:"name"`
	Scopes       []AccessTokenScope `json:"scopes"`
	Repositories []RepoTargetOption `json:"repositories,omitempty"`
}

// CreateAccessToken create one access token with options
func (c *Client) CreateAccessToken(user string, opt CreateAccessTokenOption) (*AccessToken, *Response, error) {
	body, err := json.Marshal(&opt)
	if err != nil {
		return nil, nil, err
	}
	t := new(AccessToken)
	resp, err := c.getParsedResponse("POST", fmt.Sprintf("/users/%s/tokens", url.PathEscape(user)), jsonHeader, bytes.NewReader(body), t)
	return t, resp, err
}

// DeleteAccessTokenByID deletes a token identified by its ID.
func (c *Client) DeleteAccessTokenByID(ctx context.Context, user string, id int64) (Response, error) {
	token := fmt.Sprintf("%d", id)
	_, resp, err := c.getResponseWithContext(ctx, "DELETE", fmt.Sprintf("/users/%s/tokens/%s", url.PathEscape(user), url.PathEscape(token)), jsonHeader, nil)
	return resp, err
}

// DeleteAccessTokenByName deletes a token identified by its name.
func (c *Client) DeleteAccessTokenByName(ctx context.Context, user, name string) (Response, error) {
	if err := c.checkServerVersionGreaterThanOrEqual(version1_13_0); err != nil {
		return Response{}, err
	}
	_, resp, err := c.getResponseWithContext(ctx, "DELETE", fmt.Sprintf("/users/%s/tokens/%s", url.PathEscape(user), url.PathEscape(name)), jsonHeader, nil)
	return resp, err
}

// DeleteAccessToken delete token, identified by ID and if not available by name.
//
// Deprecated: Use DeleteAccessTokenByID or DeleteAccessTokenByName instead.
// This will be removed in a future major release.
func (c *Client) DeleteAccessToken(user string, value any) (*Response, error) {
	var token string

	switch v := value.(type) {
	case int64:
		token = fmt.Sprintf("%d", v)
	case string:
		if err := c.checkServerVersionGreaterThanOrEqual(version1_13_0); err != nil {
			return nil, err
		}
		token = v
	default:
		return nil, fmt.Errorf("only string and int64 supported")
	}

	_, resp, err := c.getResponse("DELETE", fmt.Sprintf("/users/%s/tokens/%s", url.PathEscape(user), url.PathEscape(token)), jsonHeader, nil)
	return resp, err
}
