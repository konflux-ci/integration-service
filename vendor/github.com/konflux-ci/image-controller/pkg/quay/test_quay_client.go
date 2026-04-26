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
	. "github.com/onsi/ginkgo/v2"
)

const (
	TestQuayOrg = "user-workloads"
)

// TestQuayClient is a QuayClient for testing the controller
type TestQuayClient struct{}

var _ QuayService = (*TestQuayClient)(nil)

var (
	CreateRepositoryFunc                      func(repository RepositoryRequest) (*Repository, error)
	DeleteRepositoryFunc                      func(organization, imageRepository string) (bool, error)
	ChangeRepositoryVisibilityFunc            func(organization, imageRepository string, visibility string) error
	GetRobotAccountFunc                       func(organization string, robotName string) (*RobotAccount, error)
	CreateRobotAccountFunc                    func(organization string, robotName string) (*RobotAccount, error)
	DeleteRobotAccountFunc                    func(organization string, robotName string) (bool, error)
	AddPermissionsForRepositoryToAccountFunc  func(organization, imageRepository, accountName string, isRobot, isWrite bool) error
	ListPermissionsForRepositoryFunc          func(organization, imageRepository string) (map[string]UserAccount, error)
	AddReadPermissionsForRepositoryToTeamFunc func(organization, imageRepository, teamName string) error
	ListRepositoryPermissionsForTeamFunc      func(organization, teamName string) ([]TeamPermission, error)
	AddUserToTeamFunc                         func(organization, teamName, userName string) (bool, error)
	RemoveUserFromTeamFunc                    func(organization, teamName, userName string) error
	DeleteTeamFunc                            func(organization, teamName string) error
	EnsureTeamFunc                            func(organization, teamName string) ([]Member, error)
	GetTeamMembersFunc                        func(organization, teamName string) ([]Member, error)
	RegenerateRobotAccountTokenFunc           func(organization string, robotName string) (*RobotAccount, error)
	GetNotificationsFunc                      func(organization, repository string) ([]Notification, error)
	CreateNotificationFunc                    func(organization, repository string, notification Notification) (*Notification, error)
	UpdateNotificationFunc                    func(organization, repository string, notificationUuid string, notification Notification) (*Notification, error)
	DeleteNotificationFunc                    func(organization, repository string, notificationUuid string) (bool, error)
)

func ResetTestQuayClient() {
	CreateRepositoryFunc = func(repository RepositoryRequest) (*Repository, error) { return &Repository{}, nil }
	DeleteRepositoryFunc = func(organization, imageRepository string) (bool, error) { return true, nil }
	ChangeRepositoryVisibilityFunc = func(organization, imageRepository string, visibility string) error { return nil }
	GetRobotAccountFunc = func(organization, robotName string) (*RobotAccount, error) { return &RobotAccount{}, nil }
	CreateRobotAccountFunc = func(organization, robotName string) (*RobotAccount, error) { return &RobotAccount{}, nil }
	DeleteRobotAccountFunc = func(organization, robotName string) (bool, error) { return true, nil }
	AddPermissionsForRepositoryToAccountFunc = func(organization, imageRepository, accountName string, isRobot, isWrite bool) error { return nil }
	ListPermissionsForRepositoryFunc = func(organization, imageRepository string) (map[string]UserAccount, error) { return nil, nil }
	AddReadPermissionsForRepositoryToTeamFunc = func(organization, imageRepository, teamName string) error { return nil }
	ListRepositoryPermissionsForTeamFunc = func(organization, teamName string) ([]TeamPermission, error) { return []TeamPermission{}, nil }
	AddUserToTeamFunc = func(organization, teamName, userName string) (bool, error) { return false, nil }
	RemoveUserFromTeamFunc = func(organization, teamName, userName string) error { return nil }
	DeleteTeamFunc = func(organization, teamName string) error { return nil }
	EnsureTeamFunc = func(organization, teamName string) ([]Member, error) { return []Member{}, nil }
	GetTeamMembersFunc = func(organization, teamName string) ([]Member, error) { return []Member{}, nil }
	RegenerateRobotAccountTokenFunc = func(organization, robotName string) (*RobotAccount, error) { return &RobotAccount{}, nil }
	GetNotificationsFunc = func(organization, repository string) ([]Notification, error) { return []Notification{}, nil }
	CreateNotificationFunc = func(organization, repository string, notification Notification) (*Notification, error) {
		return &Notification{}, nil
	}
	UpdateNotificationFunc = func(organization, repository string, notificationUuid string, notification Notification) (*Notification, error) {
		return &Notification{}, nil
	}
	DeleteNotificationFunc = func(organization, repository string, notificationUuid string) (bool, error) {
		return true, nil
	}
}

func ResetTestQuayClientToFails() {
	CreateRepositoryFunc = func(repository RepositoryRequest) (*Repository, error) {
		defer GinkgoRecover()
		Fail("CreateRepositoryFunc invoked")
		return nil, nil
	}
	DeleteRepositoryFunc = func(organization, imageRepository string) (bool, error) {
		defer GinkgoRecover()
		Fail("DeleteRepository invoked")
		return true, nil
	}
	ChangeRepositoryVisibilityFunc = func(organization, imageRepository string, visibility string) error {
		defer GinkgoRecover()
		Fail("ChangeRepositoryVisibility invoked")
		return nil
	}
	GetRobotAccountFunc = func(organization, robotName string) (*RobotAccount, error) {
		defer GinkgoRecover()
		Fail("GetRobotAccount invoked")
		return nil, nil
	}
	CreateRobotAccountFunc = func(organization, robotName string) (*RobotAccount, error) {
		defer GinkgoRecover()
		Fail("CreateRobotAccount invoked")
		return nil, nil
	}
	DeleteRobotAccountFunc = func(organization, robotName string) (bool, error) {
		defer GinkgoRecover()
		Fail("DeleteRobotAccount invoked")
		return true, nil
	}
	AddPermissionsForRepositoryToAccountFunc = func(organization, imageRepository, accountName string, isRobot, isWrite bool) error {
		defer GinkgoRecover()
		Fail("AddPermissionsForRepositoryToAccount invoked")
		return nil
	}
	ListPermissionsForRepositoryFunc = func(organization, imageRepository string) (map[string]UserAccount, error) {
		defer GinkgoRecover()
		Fail("ListPermissionsForRepository invoked")
		return nil, nil
	}
	AddReadPermissionsForRepositoryToTeamFunc = func(organization, imageRepository, teamName string) error {
		defer GinkgoRecover()
		Fail("AddPermissionsForRepositoryToTeam invoked")
		return nil
	}
	ListRepositoryPermissionsForTeamFunc = func(organization, teamName string) ([]TeamPermission, error) {
		defer GinkgoRecover()
		Fail("ListRepositoryPermissionsForTeam invoked")
		return nil, nil
	}
	AddUserToTeamFunc = func(organization, teamName, userName string) (bool, error) {
		defer GinkgoRecover()
		Fail("AddUserToTeam invoked")
		return false, nil
	}
	RemoveUserFromTeamFunc = func(organization, teamName, userName string) error {
		defer GinkgoRecover()
		Fail("RemoveUserFromTeam invoked")
		return nil
	}
	DeleteTeamFunc = func(organization, teamName string) error {
		defer GinkgoRecover()
		Fail("DeleteTeam invoked")
		return nil
	}
	EnsureTeamFunc = func(organization, teamName string) ([]Member, error) {
		defer GinkgoRecover()
		Fail("EnsureTeam invoked")
		return nil, nil
	}
	GetTeamMembersFunc = func(organization, teamName string) ([]Member, error) {
		defer GinkgoRecover()
		Fail("GetTeamMembers invoked")
		return nil, nil
	}
	RegenerateRobotAccountTokenFunc = func(organization, robotName string) (*RobotAccount, error) {
		defer GinkgoRecover()
		Fail("RegenerateRobotAccountToken invoked")
		return nil, nil
	}
	GetNotificationsFunc = func(organization, repository string) ([]Notification, error) {
		defer GinkgoRecover()
		Fail("GetNotificationsFunc invoked")
		return nil, nil
	}
	CreateNotificationFunc = func(organization, repository string, notification Notification) (*Notification, error) {
		defer GinkgoRecover()
		Fail("CreateNotification invoked")
		return nil, nil
	}
	UpdateNotificationFunc = func(organization, repository string, notificationUuid string, notification Notification) (*Notification, error) {
		defer GinkgoRecover()
		Fail("UpdateNotification invoked")
		return nil, nil
	}
	DeleteNotificationFunc = func(organization, repository string, notificationUuid string) (bool, error) {
		defer GinkgoRecover()
		Fail("DeleteNotification invoked")
		return true, nil
	}
}

func (c TestQuayClient) CreateRepository(repositoryRequest RepositoryRequest) (*Repository, error) {
	return CreateRepositoryFunc(repositoryRequest)
}
func (c TestQuayClient) DeleteRepository(organization, imageRepository string) (bool, error) {
	return DeleteRepositoryFunc(organization, imageRepository)
}
func (TestQuayClient) ChangeRepositoryVisibility(organization, imageRepository string, visibility string) error {
	return ChangeRepositoryVisibilityFunc(organization, imageRepository, visibility)
}
func (c TestQuayClient) GetRobotAccount(organization string, robotName string) (*RobotAccount, error) {
	return GetRobotAccountFunc(organization, robotName)
}
func (c TestQuayClient) CreateRobotAccount(organization string, robotName string) (*RobotAccount, error) {
	return CreateRobotAccountFunc(organization, robotName)
}
func (c TestQuayClient) DeleteRobotAccount(organization string, robotName string) (bool, error) {
	return DeleteRobotAccountFunc(organization, robotName)
}
func (c TestQuayClient) AddPermissionsForRepositoryToAccount(organization, imageRepository, accountName string, isRobot, isWrite bool) error {
	return AddPermissionsForRepositoryToAccountFunc(organization, imageRepository, accountName, isRobot, isWrite)
}
func (c TestQuayClient) ListPermissionsForRepository(organization, imageRepository string) (map[string]UserAccount, error) {
	return ListPermissionsForRepositoryFunc(organization, imageRepository)
}
func (c TestQuayClient) AddReadPermissionsForRepositoryToTeam(organization, imageRepository, teamName string) error {
	return AddReadPermissionsForRepositoryToTeamFunc(organization, imageRepository, teamName)
}
func (c TestQuayClient) ListRepositoryPermissionsForTeam(organization, teamName string) ([]TeamPermission, error) {
	return ListRepositoryPermissionsForTeamFunc(organization, teamName)
}
func (c TestQuayClient) AddUserToTeam(organization, teamName, userName string) (bool, error) {
	return AddUserToTeamFunc(organization, teamName, userName)
}
func (c TestQuayClient) RemoveUserFromTeam(organization, teamName, userName string) error {
	return RemoveUserFromTeamFunc(organization, teamName, userName)
}
func (c TestQuayClient) DeleteTeam(organization, teamName string) error {
	return DeleteTeamFunc(organization, teamName)
}
func (c TestQuayClient) EnsureTeam(organization, teamName string) ([]Member, error) {
	return EnsureTeamFunc(organization, teamName)
}
func (c TestQuayClient) GetTeamMembers(organization, teamName string) ([]Member, error) {
	return GetTeamMembersFunc(organization, teamName)
}
func (c TestQuayClient) RegenerateRobotAccountToken(organization string, robotName string) (*RobotAccount, error) {
	return RegenerateRobotAccountTokenFunc(organization, robotName)
}
func (c TestQuayClient) GetAllRepositories(organization string) ([]Repository, error) {
	return nil, nil
}
func (c TestQuayClient) GetAllRobotAccounts(organization string) ([]RobotAccount, error) {
	return nil, nil
}
func (TestQuayClient) DeleteTag(organization string, repository string, tag string) (bool, error) {
	return true, nil
}
func (TestQuayClient) GetTagsFromPage(organization string, repository string, page int) ([]Tag, bool, error) {
	return nil, false, nil
}
func (TestQuayClient) GetNotifications(organization string, repository string) ([]Notification, error) {
	return GetNotificationsFunc(organization, repository)
}
func (TestQuayClient) CreateNotification(organization, repository string, notification Notification) (*Notification, error) {
	return CreateNotificationFunc(organization, repository, notification)
}
func (TestQuayClient) DeleteNotification(organization, repository string, notificationUuid string) (bool, error) {
	return DeleteNotificationFunc(organization, repository, notificationUuid)
}
func (TestQuayClient) UpdateNotification(organization, repository string, notificationUuid string, notification Notification) (*Notification, error) {
	return UpdateNotificationFunc(organization, repository, notificationUuid, notification)
}
