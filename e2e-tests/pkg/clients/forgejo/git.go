package forgejo

import (
	"encoding/base64"
	"fmt"
	"strings"

	"codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"
)

// CreateFile creates a new file in a repository
func (fc *ForgejoClient) CreateFile(projectID, pathToFile, content, branchName string) (*forgejo.FileResponse, error) {
	owner, repo := splitProjectID(projectID)

	opts := forgejo.CreateFileOptions{
		FileOptions: forgejo.FileOptions{
			Message:    "e2e test commit message",
			BranchName: branchName,
		},
		Content: base64.StdEncoding.EncodeToString([]byte(content)),
	}

	fileResp, _, err := fc.client.CreateFile(owner, repo, pathToFile, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create file %s: %w", pathToFile, err)
	}

	return fileResp, nil
}

// splitProjectID splits a projectID in format "owner/repo" into owner and repo
func splitProjectID(projectID string) (string, string) {
	parts := strings.SplitN(projectID, "/", 2)
	if len(parts) != 2 {
		return projectID, ""
	}
	return parts[0], parts[1]
}
