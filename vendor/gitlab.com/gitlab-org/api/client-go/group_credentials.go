package gitlab

import (
	"net/http"
	"time"
)

type (
	GroupCredentialsServiceInterface interface {
		// ListGroupPersonalAccessTokens lists all personal access tokens associated with enterprise users in a top-level group.
		//
		// GitLab API docs:
		// https://docs.gitlab.com/api/groups/#list-all-personal-access-tokens-for-a-group
		ListGroupPersonalAccessTokens(gid any, opt *ListGroupPersonalAccessTokensOptions, options ...RequestOptionFunc) ([]*GroupPersonalAccessToken, *Response, error)
	}

	// GroupCredentialsService handles communication with the top-level group
	// credentials inventory management endpoints of the GitLab API.
	//
	// GitLab API docs:
	// https://docs.gitlab.com/api/groups/#credentials-inventory-management
	GroupCredentialsService struct {
		client *Client
	}
)

// GroupPersonalAccessToken represents a group enterprise users personal access token.
//
// GitLab API docs:
// https://docs.gitlab.com/api/groups/#list-all-personal-access-tokens-for-a-group
type GroupPersonalAccessToken struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Revoked     bool       `json:"revoked"`
	CreatedAt   *time.Time `json:"created_at"`
	Description string     `json:"description"`
	Scopes      []string   `json:"scopes"`
	UserID      int64      `json:"user_id"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	Active      bool       `json:"active"`
	ExpiresAt   *ISOTime   `json:"expires_at"`
}

// ListGroupPersonalAccessTokensOptions represents the available
// ListGroupPersonalAccessTokens() options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/groups/#list-all-personal-access-tokens-for-a-group
type ListGroupPersonalAccessTokensOptions struct {
	ListOptions
	CreatedAfter   *ISOTime `url:"created_after,omitempty" json:"created_after,omitempty"`
	CreatedBefore  *ISOTime `url:"created_before,omitempty" json:"created_before,omitempty"`
	LastUsedAfter  *ISOTime `url:"last_used_after,omitempty" json:"last_used_after,omitempty"`
	LastUsedBefore *ISOTime `url:"last_used_before,omitempty" json:"last_used_before,omitempty"`
	Revoked        *bool    `url:"revoked,omitempty" json:"revoked,omitempty"`
	Search         *string  `url:"search,omitempty" json:"search,omitempty"`
	State          *string  `url:"state,omitempty" json:"state,omitempty"`
}

func (g *GroupCredentialsService) ListGroupPersonalAccessTokens(gid any, opt *ListGroupPersonalAccessTokensOptions, options ...RequestOptionFunc) ([]*GroupPersonalAccessToken, *Response, error) {
	return do[[]*GroupPersonalAccessToken](g.client,
		withMethod(http.MethodGet),
		withPath("groups/%s/manage/personal_access_tokens", GroupID{gid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}
