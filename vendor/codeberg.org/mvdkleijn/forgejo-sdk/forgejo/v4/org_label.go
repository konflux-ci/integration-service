// Copyright 2026 The Forgejo Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package forgejo

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// ListOrgLabels list an organization's labels
func (c *Client) ListOrgLabels(org string, opt ListLabelsOptions) ([]*Label, *Response, error) {
	if err := escapeValidatePathSegments(&org); err != nil {
		return nil, nil, err
	}
	opt.setDefaults()
	labels := make([]*Label, 0, opt.PageSize)
	resp, err := c.getParsedResponse("GET", fmt.Sprintf("/orgs/%s/labels?%s", org, opt.getURLQuery().Encode()), nil, nil, &labels)
	return labels, resp, err
}

// GetOrgLabel get one label of an organization by id
func (c *Client) GetOrgLabel(org string, id int64) (*Label, *Response, error) {
	if err := escapeValidatePathSegments(&org); err != nil {
		return nil, nil, err
	}
	label := new(Label)
	resp, err := c.getParsedResponse("GET", fmt.Sprintf("/orgs/%s/labels/%d", org, id), nil, nil, label)
	return label, resp, err
}

// CreateOrgLabel create one label for an organization
func (c *Client) CreateOrgLabel(org string, opt CreateLabelOption) (*Label, *Response, error) {
	if err := escapeValidatePathSegments(&org); err != nil {
		return nil, nil, err
	}
	if err := opt.Validate(); err != nil {
		return nil, nil, err
	}
	if len(opt.Color) == 6 {
		if err := c.checkServerVersionGreaterThanOrEqual(version1_12_0); err != nil {
			opt.Color = "#" + opt.Color
		}
	}
	body, err := json.Marshal(&opt)
	if err != nil {
		return nil, nil, err
	}
	label := new(Label)
	resp, err := c.getParsedResponse("POST", fmt.Sprintf("/orgs/%s/labels", org), jsonHeader, bytes.NewReader(body), label)
	return label, resp, err
}

// EditOrgLabel modify one label of an organization with options
func (c *Client) EditOrgLabel(org string, id int64, opt EditLabelOption) (*Label, *Response, error) {
	if err := escapeValidatePathSegments(&org); err != nil {
		return nil, nil, err
	}
	if err := opt.Validate(); err != nil {
		return nil, nil, err
	}
	body, err := json.Marshal(&opt)
	if err != nil {
		return nil, nil, err
	}
	label := new(Label)
	resp, err := c.getParsedResponse("PATCH", fmt.Sprintf("/orgs/%s/labels/%d", org, id), jsonHeader, bytes.NewReader(body), label)
	return label, resp, err
}

// DeleteOrgLabel delete one label of an organization by id
func (c *Client) DeleteOrgLabel(org string, id int64) (*Response, error) {
	if err := escapeValidatePathSegments(&org); err != nil {
		return nil, err
	}
	_, resp, err := c.getResponse("DELETE", fmt.Sprintf("/orgs/%s/labels/%d", org, id), nil, nil)
	return resp, err
}
