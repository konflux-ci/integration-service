package forgejo

import (
	"codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v3"
)

// ForgejoClient wraps the Forgejo SDK client.
type ForgejoClient struct {
	client *forgejo.Client
	apiURL string
	token  string
}

// NewForgejoClient creates a new Forgejo client.
func NewForgejoClient(accessToken, baseURL, org string) (*ForgejoClient, error) {
	_ = org // reserved for callers passing default org; API paths use full owner/repo IDs

	client, err := forgejo.NewClient(baseURL, forgejo.SetToken(accessToken))
	if err != nil {
		return nil, err
	}

	return &ForgejoClient{
		client: client,
		apiURL: baseURL,
		token:  accessToken,
	}, nil
}

// GetClient returns the underlying Forgejo API client.
func (fc *ForgejoClient) GetClient() *forgejo.Client {
	return fc.client
}
