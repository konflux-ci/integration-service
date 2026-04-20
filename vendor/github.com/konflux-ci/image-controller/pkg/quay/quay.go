/*
Copyright 2023 Red Hat, Inc.

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
package quay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"
)

type QuayService interface {
	CreateRepository(repositoryRequest RepositoryRequest) (*Repository, error)
	DeleteRepository(organization, imageRepository string) (bool, error)
	ChangeRepositoryVisibility(organization, imageRepository, visibility string) error
	GetRobotAccount(organization string, robotName string) (*RobotAccount, error)
	CreateRobotAccount(organization string, robotName string) (*RobotAccount, error)
	DeleteRobotAccount(organization string, robotName string) (bool, error)
	AddPermissionsForRepositoryToAccount(organization, imageRepository, accountName string, isRobot, isWrite bool) error
	ListPermissionsForRepository(organization, imageRepository string) (map[string]UserAccount, error)
	AddReadPermissionsForRepositoryToTeam(organization, imageRepository, teamName string) error
	ListRepositoryPermissionsForTeam(organization, teamName string) ([]TeamPermission, error)
	AddUserToTeam(organization, teamName, userName string) (bool, error)
	RemoveUserFromTeam(organization, teamName, userName string) error
	DeleteTeam(organization, teamName string) error
	EnsureTeam(organization, teamName string) ([]Member, error)
	GetTeamMembers(organization, teamName string) ([]Member, error)
	RegenerateRobotAccountToken(organization string, robotName string) (*RobotAccount, error)
	GetAllRepositories(organization string) ([]Repository, error)
	GetAllRobotAccounts(organization string) ([]RobotAccount, error)
	GetTagsFromPage(organization, repository string, page int) ([]Tag, bool, error)
	DeleteTag(organization, repository, tag string) (bool, error)
	GetNotifications(organization, repository string) ([]Notification, error)
	CreateNotification(organization, repository string, notification Notification) (*Notification, error)
	UpdateNotification(organization, repository string, notificationUuid string, notification Notification) (*Notification, error)
	DeleteNotification(organization, repository string, notificationUuid string) (bool, error)
}

var _ QuayService = (*QuayClient)(nil)

type QuayClient struct {
	url        string
	httpClient *http.Client
	AuthToken  string
}

func NewQuayClient(c *http.Client, authToken, url string) *QuayClient {
	return &QuayClient{
		httpClient: c,
		AuthToken:  authToken,
		url:        url,
	}
}

// QuayResponse wraps http.Response in order to provide custom methods, e.g. GetJson
type QuayResponse struct {
	response *http.Response
}

func (r *QuayResponse) GetJson(obj interface{}) error {
	defer r.response.Body.Close()
	body, err := io.ReadAll(r.response.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %s", err)
	}
	if err := json.Unmarshal(body, obj); err != nil {
		return fmt.Errorf("failed to unmarshal response body: %s, got body: %s", err, string(body))
	}
	return nil
}

func (r *QuayResponse) GetStatusCode() int {
	return r.response.StatusCode
}

func (c *QuayClient) makeRequest(url, method string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.AuthToken))
	req.Header.Add("Content-Type", "application/json")
	return req, nil
}

func (c *QuayClient) doRequest(url, method string, body io.Reader) (*QuayResponse, error) {
	req, err := c.makeRequest(url, method, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to Do request: %w", err)
	}
	return &QuayResponse{response: resp}, nil
}

// CreateRepository creates a new Quay.io image repository.
func (c *QuayClient) CreateRepository(repositoryRequest RepositoryRequest) (*Repository, error) {
	url := fmt.Sprintf("%s/%s", c.url, "repository")

	b, err := json.Marshal(repositoryRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal repository request data: %w", err)
	}

	resp, err := c.doRequest(url, http.MethodPost, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}

	statusCode := resp.GetStatusCode()

	data := &Repository{}
	if err := resp.GetJson(data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response, got response code %d with error: %w", statusCode, err)
	}

	if statusCode != 200 {
		if statusCode == 402 {
			// Current plan doesn't allow private image repositories
			return nil, errors.New("payment required")
		} else if statusCode == 400 && data.ErrorMessage == "Repository already exists" {
			data.Name = repositoryRequest.Repository
		} else if data.ErrorMessage != "" {
			return data, errors.New(data.ErrorMessage)
		}
	}

	return data, nil
}

// DoesRepositoryExist checks if the specified image repository exists in quay.
func (c *QuayClient) DoesRepositoryExist(organization, imageRepository string) (bool, error) {
	url := fmt.Sprintf("%s/repository/%s/%s", c.url, organization, imageRepository)

	resp, err := c.doRequest(url, http.MethodGet, nil)
	if err != nil {
		return false, err
	}

	if resp.GetStatusCode() == 404 {
		return false, fmt.Errorf("repository %s does not exist in %s organization", imageRepository, organization)
	} else if resp.GetStatusCode() == 200 {
		return true, nil
	}

	data := &QuayError{}
	if err := resp.GetJson(data); err != nil {
		return false, err
	}
	if data.Error != "" {
		return false, errors.New(data.Error)
	}
	return false, errors.New(data.ErrorMessage)
}

// IsRepositoryPublic checks if the specified image repository has visibility public in quay.
func (c *QuayClient) IsRepositoryPublic(organization, imageRepository string) (bool, error) {
	url := fmt.Sprintf("%s/repository/%s/%s", c.url, organization, imageRepository)

	resp, err := c.doRequest(url, http.MethodGet, nil)
	if err != nil {
		return false, err
	}

	if resp.GetStatusCode() == 404 {
		return false, fmt.Errorf("repository %s does not exist in %s organization", imageRepository, organization)
	}

	if resp.GetStatusCode() == 200 {
		repo := &Repository{}
		if err := resp.GetJson(repo); err != nil {
			return false, err
		}
		if repo.IsPublic {
			return true, nil
		} else {
			return false, nil
		}
	}

	data := &QuayError{}
	if err := resp.GetJson(data); err != nil {
		return false, err
	}
	if data.Error != "" {
		return false, errors.New(data.Error)
	}
	return false, errors.New(data.ErrorMessage)
}

// DeleteRepository deletes specified image repository.
func (c *QuayClient) DeleteRepository(organization, imageRepository string) (bool, error) {
	url := fmt.Sprintf("%s/repository/%s/%s", c.url, organization, imageRepository)

	resp, err := c.doRequest(url, http.MethodDelete, nil)
	if err != nil {
		return false, err
	}

	statusCode := resp.GetStatusCode()

	if statusCode == 204 {
		return true, nil
	}
	if statusCode == 404 {
		return false, nil
	}

	data := &QuayError{}
	if err := resp.GetJson(data); err != nil {
		return false, err
	}
	if data.Error != "" {
		return false, errors.New(data.Error)
	}
	return false, errors.New(data.ErrorMessage)
}

// ChangeRepositoryVisibility makes existing repository public or private.
func (c *QuayClient) ChangeRepositoryVisibility(organization, imageRepositoryName, visibility string) error {
	if !(visibility == "public" || visibility == "private") {
		return fmt.Errorf("invalid repository visibility: %s", visibility)
	}

	// https://quay.io/api/v1/repository/user-org/repo-name/changevisibility
	url := fmt.Sprintf("%s/repository/%s/%s/changevisibility", c.url, organization, imageRepositoryName)
	requestData := strings.NewReader(fmt.Sprintf(`{"visibility": "%s"}`, visibility))

	resp, err := c.doRequest(url, http.MethodPost, requestData)
	if err != nil {
		return err
	}

	statusCode := resp.GetStatusCode()

	if statusCode == 200 {
		return nil
	}

	if statusCode == 402 {
		// Current plan doesn't allow private image repositories
		return errors.New("payment required")
	}

	data := &QuayError{}
	if err := resp.GetJson(data); err != nil {
		return err
	}
	if data.ErrorMessage != "" {
		return errors.New(data.ErrorMessage)
	}
	return errors.New(resp.response.Status)
}

func (c *QuayClient) GetRobotAccount(organization string, robotName string) (*RobotAccount, error) {
	url := fmt.Sprintf("%s/%s/%s/%s/%s", c.url, "organization", organization, "robots", robotName)

	resp, err := c.doRequest(url, http.MethodGet, nil)
	if err != nil {
		return nil, err
	}

	data := &RobotAccount{}
	if err := resp.GetJson(data); err != nil {
		return nil, err
	}

	if resp.GetStatusCode() != http.StatusOK {
		return nil, errors.New(data.Message)
	}

	return data, nil
}

// CreateRobotAccount creates a new Quay.io robot account in the organization.
func (c *QuayClient) CreateRobotAccount(organization string, robotName string) (*RobotAccount, error) {
	robotName, err := handleRobotName(robotName)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s/%s/%s/%s", c.url, "organization", organization, "robots", robotName)
	payload := strings.NewReader(`{"description": "Robot account for AppStudio Component"}`)
	resp, err := c.doRequest(url, http.MethodPut, payload)
	if err != nil {
		return nil, err
	}

	statusCode := resp.GetStatusCode()
	if statusCode >= 200 && statusCode <= 204 {
		// Success
		data := &RobotAccount{}
		if err := resp.GetJson(data); err != nil {
			return nil, err
		}
		return data, nil
	}
	// Handle errors

	data := &QuayError{}
	message := "Failed to create robot account"
	if err := resp.GetJson(data); err == nil {
		if data.Message != "" {
			message = data.Message
		} else if data.ErrorMessage != "" {
			message = data.ErrorMessage
		} else {
			message = data.Error
		}
	}

	// Handle robot account already exists case
	if statusCode == 400 && strings.Contains(message, "Existing robot with name") {
		return c.GetRobotAccount(organization, robotName)
	}

	return nil, fmt.Errorf("failed to create robot account. Status code: %d, message: %s", statusCode, message)
}

// DeleteRobotAccount deletes given Quay.io robot account in the organization.
func (c *QuayClient) DeleteRobotAccount(organization string, robotName string) (bool, error) {
	robotName, err := handleRobotName(robotName)
	if err != nil {
		return false, err
	}
	url := fmt.Sprintf("%s/organization/%s/robots/%s", c.url, organization, robotName)

	resp, err := c.doRequest(url, http.MethodDelete, nil)
	if err != nil {
		return false, err
	}

	if resp.GetStatusCode() == 204 {
		return true, nil
	}
	if resp.GetStatusCode() == 404 {
		return false, nil
	}

	data := &QuayError{}
	if err := resp.GetJson(data); err != nil {
		return false, err
	}
	if data.Error != "" {
		return false, errors.New(data.Error)
	}
	return false, errors.New(data.ErrorMessage)
}

// ListPermissionsForRepository list permissions for the given repository.
func (c *QuayClient) ListPermissionsForRepository(organization, imageRepository string) (map[string]UserAccount, error) {
	url := fmt.Sprintf("%s/repository/%s/%s/permissions/user", c.url, organization, imageRepository)

	resp, err := c.doRequest(url, http.MethodGet, nil)

	if err != nil {
		return nil, fmt.Errorf("failed to Do request, error: %s", err)
	}

	if resp.GetStatusCode() != 200 {
		var message string
		data := &QuayError{}
		if err := resp.GetJson(data); err == nil {
			if data.ErrorMessage != "" {
				message = data.ErrorMessage
			} else {
				message = data.Error
			}
		}
		return nil, fmt.Errorf("failed to get permissions for repository: %s, got status code %d, message: %s", imageRepository, resp.GetStatusCode(), message)
	}

	type Response struct {
		Permissions map[string]UserAccount `json:"permissions"`
	}
	var response Response
	if err := resp.GetJson(&response); err != nil {
		return nil, fmt.Errorf("failed to get permissions for repository: %s, got status code %d, message: %s", imageRepository, resp.GetStatusCode(), err.Error())
	}

	return response.Permissions, nil
}

// ListRepositoryPermissionsForTeam list permissions for the given team
func (c *QuayClient) ListRepositoryPermissionsForTeam(organization, teamName string) ([]TeamPermission, error) {
	url := fmt.Sprintf("%s/organization/%s/team/%s/permissions", c.url, organization, teamName)

	resp, err := c.doRequest(url, http.MethodGet, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to Do request, error: %s", err)
	}

	if resp.GetStatusCode() != 200 {
		var message string
		data := &QuayError{}
		if err := resp.GetJson(data); err == nil {
			if data.ErrorMessage != "" {
				message = data.ErrorMessage
			} else {
				message = data.Error
			}
		} else {
			message = err.Error()
		}
		return nil, fmt.Errorf("failed to get permissions for team: %s, got status code %d, message: %s", teamName, resp.GetStatusCode(), message)
	}

	type Response struct {
		Permissions []TeamPermission `json:"permissions"`
	}
	var response Response
	if err := resp.GetJson(&response); err != nil {
		return nil, fmt.Errorf("failed to get permissions for team: %s, got status code %d, message: %s", teamName, resp.GetStatusCode(), err.Error())
	}

	return response.Permissions, nil
}

// AddUserToTeam adds user to the given team
// bool return value is indicating if it is permanent error (user doesn't exist)
func (c *QuayClient) AddUserToTeam(organization, teamName, userName string) (bool, error) {
	url := fmt.Sprintf("%s/organization/%s/team/%s/members/%s", c.url, organization, teamName, userName)

	resp, err := c.doRequest(url, http.MethodPut, nil)
	if err != nil {
		return false, fmt.Errorf("failed to Do request, error: %s", err)
	}

	if resp.GetStatusCode() != 200 {
		// 400 is returned when user doesn't exist
		// 404 just in case
		if resp.GetStatusCode() == 400 || resp.GetStatusCode() == 404 {
			return true, fmt.Errorf("failed to add user: %s, to the team team: %s, user doesn't exist", userName, teamName)
		}

		var message string
		data := &QuayError{}
		if err := resp.GetJson(data); err == nil {
			if data.ErrorMessage != "" {
				message = data.ErrorMessage
			} else {
				message = data.Error
			}
		} else {
			message = err.Error()
		}
		return false, fmt.Errorf("failed to add user: %s, to the team team: %s, got status code %d, message: %s", userName, teamName, resp.GetStatusCode(), message)
	}
	return false, nil
}

// RemoveUserToTeam remove user from the given team
func (c *QuayClient) RemoveUserFromTeam(organization, teamName, userName string) error {
	url := fmt.Sprintf("%s/organization/%s/team/%s/members/%s", c.url, organization, teamName, userName)

	resp, err := c.doRequest(url, http.MethodDelete, nil)
	if err != nil {
		return fmt.Errorf("failed to Do request, error: %s", err)
	}

	// 400 is returned when user isn't anymore in the team
	// 404 is returned when user doesn't exist
	if resp.GetStatusCode() == 204 || resp.GetStatusCode() == 404 || resp.GetStatusCode() == 400 {
		return nil
	}

	var message string
	data := &QuayError{}
	if err := resp.GetJson(data); err == nil {
		if data.ErrorMessage != "" {
			message = data.ErrorMessage
		} else {
			message = data.Error
		}
	} else {
		message = err.Error()
	}
	return fmt.Errorf("failed to remove user: %s, from the team team: %s, got status code %d, message: %s", userName, teamName, resp.GetStatusCode(), message)
}

func (c *QuayClient) DeleteTeam(organization, teamName string) error {
	url := fmt.Sprintf("%s/organization/%s/team/%s", c.url, organization, teamName)

	resp, err := c.doRequest(url, http.MethodDelete, nil)
	if err != nil {
		return fmt.Errorf("failed to Do request, error: %s", err)
	}

	// 400 is returned when team doesn't exist
	// 404 just in case
	if resp.GetStatusCode() == 204 || resp.GetStatusCode() == 404 || resp.GetStatusCode() == 400 {
		return nil
	}

	var message string
	data := &QuayError{}
	if err := resp.GetJson(data); err == nil {
		if data.ErrorMessage != "" {
			message = data.ErrorMessage
		} else {
			message = data.Error
		}
	} else {
		message = err.Error()
	}
	return fmt.Errorf("failed to remove team: %s, got status code %d, message: %s", teamName, resp.GetStatusCode(), message)
}

// EnsureTeam ensures that team exists, if it doesn't it will create it
// returns list of team members
func (c *QuayClient) EnsureTeam(organization, teamName string) ([]Member, error) {
	members, err := c.GetTeamMembers(organization, teamName)
	if err != nil {
		return nil, err
	}
	// team exists
	if members != nil {
		return members, nil
	}

	// create team
	url := fmt.Sprintf("%s/organization/%s/team/%s", c.url, organization, teamName)
	body := strings.NewReader(`{"role": "member"}`)

	resp, err := c.doRequest(url, http.MethodPut, body)
	if err != nil {
		return nil, fmt.Errorf("failed to Do request, error: %s", err)
	}

	if resp.GetStatusCode() != 200 {
		var message string
		data := &QuayError{}
		if err := resp.GetJson(data); err == nil {
			if data.ErrorMessage != "" {
				message = data.ErrorMessage
			} else {
				message = data.Error
			}
		} else {
			message = err.Error()
		}
		return nil, fmt.Errorf("failed to create team: %s, got status code %d, message: %s", teamName, resp.GetStatusCode(), message)
	}

	members, err = c.GetTeamMembers(organization, teamName)
	if err != nil {
		return nil, err
	}
	return members, nil
}

// GetTeamMembers gets members of the team, when nil is returned that means that team doesn't exist
func (c *QuayClient) GetTeamMembers(organization, teamName string) ([]Member, error) {
	url := fmt.Sprintf("%s/organization/%s/team/%s/members", c.url, organization, teamName)

	resp, err := c.doRequest(url, http.MethodGet, nil)

	if err != nil {
		return nil, fmt.Errorf("failed to Do request, error: %s", err)
	}
	if resp.GetStatusCode() != 200 {
		// team doesn't exist
		if resp.GetStatusCode() == 404 {
			return nil, nil
		}

		var message string
		data := &QuayError{}
		if err := resp.GetJson(data); err == nil {
			if data.ErrorMessage != "" {
				message = data.ErrorMessage
			} else {
				message = data.Error
			}
		} else {
			message = err.Error()
		}
		return nil, fmt.Errorf("failed to get team members for team: %s, got status code %d, message: %s", teamName, resp.GetStatusCode(), message)
	}

	type Response struct {
		Members []Member `json:"members"`
	}
	var response Response

	if err := resp.GetJson(&response); err != nil {
		return nil, err
	}

	return response.Members, nil
}

// AddPermissionsForRepositoryToAccount allows given account to access to the given repository.
// If isWrite is true, then pull and push permissions are added, otherwise - pull access only.
func (c *QuayClient) AddPermissionsForRepositoryToAccount(organization, imageRepository, accountName string, isRobot, isWrite bool) error {
	var accountFullName string
	if isRobot {
		if robotName, err := handleRobotName(accountName); err == nil {
			accountFullName = organization + "+" + robotName
		} else {
			return err
		}
	} else {
		accountFullName = accountName
	}

	// url := "https://quay.io/api/v1/repository/redhat-appstudio/test-repo-using-api/permissions/user/redhat-appstudio+createdbysbose"
	url := fmt.Sprintf("%s/repository/%s/%s/permissions/user/%s", c.url, organization, imageRepository, accountFullName)

	role := "read"
	if isWrite {
		role = "write"
	}
	body := strings.NewReader(fmt.Sprintf(`{"role": "%s"}`, role))
	resp, err := c.doRequest(url, http.MethodPut, body)
	if err != nil {
		return err
	}

	if resp.GetStatusCode() != 200 {
		var message string
		data := &QuayError{}
		if err := resp.GetJson(data); err == nil {
			if data.ErrorMessage != "" {
				message = data.ErrorMessage
			} else {
				message = data.Error
			}
		}
		return fmt.Errorf("failed to add permissions to the account: %s. Status code: %d, message: %s", accountFullName, resp.GetStatusCode(), message)
	}
	return nil
}

// AddReadPermissionsForRepositoryToTeam allows given team read access to the given repository.
func (c *QuayClient) AddReadPermissionsForRepositoryToTeam(organization, imageRepository, teamName string) error {
	url := fmt.Sprintf("%s/repository/%s/%s/permissions/team/%s", c.url, organization, imageRepository, teamName)
	body := strings.NewReader(`{"role": "read"}`)

	resp, err := c.doRequest(url, http.MethodPut, body)
	if err != nil {
		return err
	}

	if resp.GetStatusCode() != 200 {
		var message string
		data := &QuayError{}
		if err := resp.GetJson(data); err == nil {
			if data.ErrorMessage != "" {
				message = data.ErrorMessage
			} else {
				message = data.Error
			}
		} else {
			message = err.Error()
		}
		return fmt.Errorf("failed to add permissions to the team: %s. Status code: %d, message: %s", teamName, resp.GetStatusCode(), message)
	}
	return nil
}

func (c *QuayClient) RegenerateRobotAccountToken(organization string, robotName string) (*RobotAccount, error) {
	url := fmt.Sprintf("%s/organization/%s/robots/%s/regenerate", c.url, organization, robotName)

	resp, err := c.doRequest(url, http.MethodPost, nil)
	if err != nil {
		return nil, err
	}

	data := &RobotAccount{}
	if err := resp.GetJson(data); err != nil {
		return nil, err
	}

	if resp.GetStatusCode() != http.StatusOK {
		return nil, errors.New(data.Message)
	}

	return data, nil
}

// GetAllRepositories returns all repositories of the DEFAULT_QUAY_ORG organization (used in e2e-tests)
// Returns all repositories of the DEFAULT_QUAY_ORG organization (used in e2e-tests)
func (c *QuayClient) GetAllRepositories(organization string) ([]Repository, error) {
	url, _ := neturl.Parse(fmt.Sprintf("%s/repository", c.url))
	values := neturl.Values{}
	values.Add("last_modified", "true")
	values.Add("namespace", organization)
	url.RawQuery = values.Encode()

	req, err := c.makeRequest(url.String(), http.MethodGet, nil)
	if err != nil {
		return nil, err
	}

	type Response struct {
		Repositories []Repository `json:"repositories"`
		NextPage     string       `json:"next_page"`
	}
	var response Response
	var repositories []Repository

	for {
		res, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to Do request, error: %s", err)
		}
		if res.StatusCode != 200 {
			return nil, fmt.Errorf("error getting repositories, got status code %d", res.StatusCode)
		}

		resp := QuayResponse{response: res}
		if err := resp.GetJson(&response); err != nil {
			return nil, err
		}

		repositories = append(repositories, response.Repositories...)

		if response.NextPage == "" || values.Get("next_page") == response.NextPage {
			break
		}

		values.Set("next_page", response.NextPage)
		req.URL.RawQuery = values.Encode()
	}
	return repositories, nil
}

// GetAllRobotAccounts returns all robot accounts of the DEFAULT_QUAY_ORG organization (used in e2e-tests)
func (c *QuayClient) GetAllRobotAccounts(organization string) ([]RobotAccount, error) {
	url := fmt.Sprintf("%s/organization/%s/robots", c.url, organization)

	resp, err := c.doRequest(url, http.MethodGet, nil)
	if err != nil {
		return nil, err
	}

	if resp.GetStatusCode() != 200 {
		return nil, fmt.Errorf("failed to get robot accounts. Status code: %d", resp.GetStatusCode())
	}

	type Response struct {
		Robots []RobotAccount
	}
	var response Response
	if err := resp.GetJson(&response); err != nil {
		return nil, err
	}
	return response.Robots, nil
}

// If robotName is in longform, return shortname
// e.g. `org+robot` will be changed to `robot`, `robot` will stay `robot`
func handleRobotName(robotName string) (string, error) {
	// Regexp from quay api `^([a-z0-9]+(?:[._-][a-z0-9]+)*)$` with one plus sign in the middle allowed (representing longname)
	r := regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*(?:\+[a-z0-9]+(?:[._-][a-z0-9]+)*)?$`)
	robotName = strings.TrimSpace(robotName)
	if !r.MatchString(robotName) {
		return "", fmt.Errorf("robot name is invalid, must match `^([a-z0-9]+(?:[._-][a-z0-9]+)*)$` (one plus sign in the middle is also allowed)")
	}
	if strings.Contains(robotName, "+") {
		robotName = strings.Split(robotName, "+")[1]
	}
	return robotName, nil
}

func (c *QuayClient) GetTagsFromPage(organization, repository string, page int) ([]Tag, bool, error) {
	url := fmt.Sprintf("%s/repository/%s/%s/tag/?page=%d", c.url, organization, repository, page)

	resp, err := c.doRequest(url, http.MethodGet, nil)
	if err != nil {
		return nil, false, err
	}

	statusCode := resp.GetStatusCode()
	if statusCode != 200 {
		return nil, false, fmt.Errorf("failed to get repository tags. Status code: %d", statusCode)
	}

	var response struct {
		Tags          []Tag `json:"tags"`
		Page          int   `json:"page"`
		HasAdditional bool  `json:"has_additional"`
	}
	err = resp.GetJson(&response)
	if err != nil {
		return nil, false, err
	}
	return response.Tags, response.HasAdditional, nil
}

func (c *QuayClient) DeleteTag(organization, repository, tag string) (bool, error) {
	url := fmt.Sprintf("%s/repository/%s/%s/tag/%s", c.url, organization, repository, tag)

	resp, err := c.doRequest(url, http.MethodDelete, nil)
	if err != nil {
		return false, err
	}

	if resp.GetStatusCode() == 204 {
		return true, nil
	}
	if resp.GetStatusCode() == 404 {
		return false, nil
	}

	data := &QuayError{}
	if err := resp.GetJson(data); err != nil {
		return false, err
	}
	if data.Error != "" {
		return false, errors.New(data.Error)
	}
	return false, errors.New(data.ErrorMessage)
}

func (c *QuayClient) GetNotifications(organization, repository string) ([]Notification, error) {
	url := fmt.Sprintf("%s/repository/%s/%s/notification/", c.url, organization, repository)

	resp, err := c.doRequest(url, http.MethodGet, nil)
	if err != nil {
		return nil, err
	}

	if resp.GetStatusCode() != 200 {
		return nil, fmt.Errorf("failed to get repository notifications. Status code: %d", resp.GetStatusCode())
	}

	var response struct {
		Notifications []Notification `json:"notifications"`
		Page          int            `json:"page"`
		HasAdditional bool           `json:"has_additional"`
	}
	// var notifications []Notification
	if err := resp.GetJson(&response); err != nil {
		return nil, err
	}
	return response.Notifications, nil
}

func (c *QuayClient) CreateNotification(organization, repository string, notification Notification) (*Notification, error) {
	allNotifications, err := c.GetNotifications(organization, repository)
	if err != nil {
		return nil, err
	}
	for _, currentNotification := range allNotifications {
		if currentNotification.Title == notification.Title {
			return &currentNotification, nil
		}
	}
	url := fmt.Sprintf("%s/repository/%s/%s/notification/", c.url, organization, repository)

	b, err := json.Marshal(notification)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal notification data: %w", err)
	}

	resp, err := c.doRequest(url, http.MethodPost, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if resp.GetStatusCode() != 201 {
		quay_error := &QuayError{}
		if err := resp.GetJson(quay_error); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("failed to create repository notification. Status code: %d, error: %s", resp.GetStatusCode(), quay_error.ErrorMessage)
	}
	var notificationResponse Notification
	if err := resp.GetJson(&notificationResponse); err != nil {
		return nil, err
	}
	return &notificationResponse, nil
}

func (c *QuayClient) DeleteNotification(organization, repository string, notificationUuid string) (bool, error) {
	res := false
	url := fmt.Sprintf("%s/repository/%s/%s/notification/%s", c.url, organization, repository, notificationUuid)

	resp, err := c.doRequest(url, http.MethodDelete, nil)
	if err != nil {
		return res, err
	}

	if resp.GetStatusCode() != 204 {
		quay_error := &QuayError{}
		if err := resp.GetJson(quay_error); err != nil {
			return res, err
		}
		return res, fmt.Errorf("failed to delete repository notification. Status code: %d, error: %s", resp.GetStatusCode(), quay_error.ErrorMessage)
	}

	return true, nil
}

func (c *QuayClient) UpdateNotification(organization, repository string, notificationUuid string, notification Notification) (*Notification, error) {
	_, err := c.DeleteNotification(
		organization,
		repository,
		notificationUuid)
	if err != nil {
		return nil, err
	}
	quayNotification, err := c.CreateNotification(
		organization,
		repository,
		notification)
	if err != nil {
		return nil, err
	}

	return quayNotification, nil
}
