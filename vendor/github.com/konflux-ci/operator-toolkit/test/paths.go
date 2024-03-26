/*
Copyright 2023.

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

package test

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GetRelativeDependencyPath returns the relative system path for a given module. If the dependency is not
// found within the go.mod file and empty string is returned.
func GetRelativeDependencyPath(dependency string) string {
	path, _ := GetRelativeDependencyPathWithError(dependency)

	return path
}

// GetRelativeDependencyPathWithError returns the relative system path for a given module. If the dependency is not
// found within the go.mod file or the file doesn't exist, an error will be returned.
func GetRelativeDependencyPathWithError(dependency string) (string, error) {
	currentWorkDir, _ := os.Getwd()

	goModFilePath, err := FindGoModPath(currentWorkDir)
	if err != nil {
		return "", err
	}

	goModFile, err := os.Open(filepath.Clean(goModFilePath))
	if err != nil {
		return "", err
	}
	defer func() {
		_ = goModFile.Close()
	}()

	regex, err := regexp.Compile("[?: \\t]+(.+) (v[0-9\\.\\-a-fA-F]+)( \\/\\/ indirect)*")
	if err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(goModFile)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), dependency) {
			matches := regex.FindStringSubmatch(scanner.Text())
			if len(matches) > 2 {
				return fmt.Sprintf("%s@%s", matches[1], matches[2]), nil
			}
		}
	}

	return "", errors.New("dependency not found")
}

// FindGoModPath returns the go.mod file system path for the current project. If the file is not found
// an error will be returned.
func FindGoModPath(currentWorkDir string) (string, error) {
	for {
		currentWorkDir = filepath.Dir(currentWorkDir)
		if currentWorkDir != "/" {
			goModFilePath := currentWorkDir + "/go.mod"
			_, err := os.Stat(goModFilePath)
			if err == nil {
				return goModFilePath, nil
			}
		} else {
			return "", errors.New("go.mod file not found")
		}
	}
}
