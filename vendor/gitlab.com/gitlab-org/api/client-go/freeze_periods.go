//
// Copyright 2021 Paul Cioanca
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
	"time"
)

// FreezePeriodsService handles the communication with the freeze periods
// related methods of the GitLab API.
//
// https://docs.gitlab.com/api/freeze_periods/
type (
	// FreezePeriodsServiceInterface defines all the API methods for the FreezePeriodsService
	FreezePeriodsServiceInterface interface {
		ListFreezePeriods(pid any, opt *ListFreezePeriodsOptions, options ...RequestOptionFunc) ([]*FreezePeriod, *Response, error)
		GetFreezePeriod(pid any, freezePeriod int, options ...RequestOptionFunc) (*FreezePeriod, *Response, error)
		CreateFreezePeriodOptions(pid any, opt *CreateFreezePeriodOptions, options ...RequestOptionFunc) (*FreezePeriod, *Response, error)
		UpdateFreezePeriodOptions(pid any, freezePeriod int, opt *UpdateFreezePeriodOptions, options ...RequestOptionFunc) (*FreezePeriod, *Response, error)
		DeleteFreezePeriod(pid any, freezePeriod int, options ...RequestOptionFunc) (*Response, error)
	}

	// FreezePeriodsService handles the communication with the freeze periods
	// related methods of the GitLab API.
	//
	// https://docs.gitlab.com/api/freeze_periods/
	FreezePeriodsService struct {
		client *Client
	}
)

var _ FreezePeriodsServiceInterface = (*FreezePeriodsService)(nil)

// FreezePeriod represents a freeze period object.
//
// GitLab API docs:
// https://docs.gitlab.com/api/freeze_periods/#list-freeze-periods
type FreezePeriod struct {
	ID           int        `json:"id"`
	FreezeStart  string     `json:"freeze_start"`
	FreezeEnd    string     `json:"freeze_end"`
	CronTimezone string     `json:"cron_timezone"`
	CreatedAt    *time.Time `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at"`
}

// ListFreezePeriodsOptions represents the available ListFreezePeriodsOptions()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/freeze_periods/#list-freeze-periods
type ListFreezePeriodsOptions ListOptions

// ListFreezePeriods gets a list of project freeze periods.
//
// GitLab API docs:
// https://docs.gitlab.com/api/freeze_periods/#list-freeze-periods
func (s *FreezePeriodsService) ListFreezePeriods(pid any, opt *ListFreezePeriodsOptions, options ...RequestOptionFunc) ([]*FreezePeriod, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/freeze_periods", PathEscape(project))

	req, err := s.client.NewRequest(http.MethodGet, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	var fp []*FreezePeriod
	resp, err := s.client.Do(req, &fp)
	if err != nil {
		return nil, resp, err
	}

	return fp, resp, nil
}

// GetFreezePeriod gets a specific freeze period for a project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/freeze_periods/#get-a-freeze-period-by-a-freeze_period_id
func (s *FreezePeriodsService) GetFreezePeriod(pid any, freezePeriod int, options ...RequestOptionFunc) (*FreezePeriod, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/freeze_periods/%d", PathEscape(project), freezePeriod)

	req, err := s.client.NewRequest(http.MethodGet, u, nil, options)
	if err != nil {
		return nil, nil, err
	}

	fp := new(FreezePeriod)
	resp, err := s.client.Do(req, fp)
	if err != nil {
		return nil, resp, err
	}

	return fp, resp, nil
}

// CreateFreezePeriodOptions represents the available CreateFreezePeriodOptions()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/freeze_periods/#create-a-freeze-period
type CreateFreezePeriodOptions struct {
	FreezeStart  *string `url:"freeze_start,omitempty" json:"freeze_start,omitempty"`
	FreezeEnd    *string `url:"freeze_end,omitempty" json:"freeze_end,omitempty"`
	CronTimezone *string `url:"cron_timezone,omitempty" json:"cron_timezone,omitempty"`
}

// CreateFreezePeriodOptions adds a freeze period to a specified project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/freeze_periods/#create-a-freeze-period
func (s *FreezePeriodsService) CreateFreezePeriodOptions(pid any, opt *CreateFreezePeriodOptions, options ...RequestOptionFunc) (*FreezePeriod, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/freeze_periods", PathEscape(project))

	req, err := s.client.NewRequest(http.MethodPost, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	fp := new(FreezePeriod)
	resp, err := s.client.Do(req, fp)
	if err != nil {
		return nil, resp, err
	}

	return fp, resp, nil
}

// UpdateFreezePeriodOptions represents the available UpdateFreezePeriodOptions()
// options.
//
// GitLab API docs:
// https://docs.gitlab.com/api/freeze_periods/#update-a-freeze-period
type UpdateFreezePeriodOptions struct {
	FreezeStart  *string `url:"freeze_start,omitempty" json:"freeze_start,omitempty"`
	FreezeEnd    *string `url:"freeze_end,omitempty" json:"freeze_end,omitempty"`
	CronTimezone *string `url:"cron_timezone,omitempty" json:"cron_timezone,omitempty"`
}

// UpdateFreezePeriodOptions edits a freeze period for a specified project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/freeze_periods/#update-a-freeze-period
func (s *FreezePeriodsService) UpdateFreezePeriodOptions(pid any, freezePeriod int, opt *UpdateFreezePeriodOptions, options ...RequestOptionFunc) (*FreezePeriod, *Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, nil, err
	}
	u := fmt.Sprintf("projects/%s/freeze_periods/%d", PathEscape(project), freezePeriod)

	req, err := s.client.NewRequest(http.MethodPut, u, opt, options)
	if err != nil {
		return nil, nil, err
	}

	fp := new(FreezePeriod)
	resp, err := s.client.Do(req, fp)
	if err != nil {
		return nil, resp, err
	}

	return fp, resp, nil
}

// DeleteFreezePeriod removes a freeze period from a project. This is an
// idempotent method and can be called multiple times. Either the hook is
// available or not.
//
// GitLab API docs:
// https://docs.gitlab.com/api/freeze_periods/#delete-a-freeze-period
func (s *FreezePeriodsService) DeleteFreezePeriod(pid any, freezePeriod int, options ...RequestOptionFunc) (*Response, error) {
	project, err := parseID(pid)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("projects/%s/freeze_periods/%d", PathEscape(project), freezePeriod)

	req, err := s.client.NewRequest(http.MethodDelete, u, nil, options)
	if err != nil {
		return nil, err
	}

	return s.client.Do(req, nil)
}
