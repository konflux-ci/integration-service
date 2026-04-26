package forgejo

import (
	"codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"
)

// ForgejoClient wraps the Forgejo SDK client
type ForgejoClient struct {
	client *forgejo.Client
}

// NewForgejoClient creates a new Forgejo client
func NewForgejoClient(accessToken, baseURL, org string) (*ForgejoClient, error) {
	client, err := forgejo.NewClient(baseURL, forgejo.SetToken(accessToken))
	if err != nil {
		return nil, err
	}

	return &ForgejoClient{
		client: client,
	}, nil
}
