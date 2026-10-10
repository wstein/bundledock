package toolpublish

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type quayReader struct {
	client            *http.Client
	repository, token string
}

func newQuayReader(ctx context.Context, client *http.Client, image, username, password string) (*quayReader, error) {
	if !imagePattern.MatchString(image) || username == "" || password == "" {
		return nil, fmt.Errorf("valid image and Quay robot credentials required")
	}
	guarded := http.Client{Timeout: 30 * time.Second}
	if client != nil {
		guarded = *client
	}
	guarded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	repository := strings.TrimPrefix(image, "quay.io/")
	query := url.Values{"service": {"quay.io"}, "scope": {"repository:" + repository + ":pull"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://quay.io/v2/auth?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(username, password)
	response, err := guarded.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Quay pull-token request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Quay pull-token request returned HTTP %d", response.StatusCode)
	}
	var authorization struct {
		Token string `json:"token"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&authorization); err != nil {
		return nil, fmt.Errorf("invalid Quay pull-token response")
	}
	if authorization.Token == "" {
		return nil, fmt.Errorf("empty Quay pull-token response")
	}
	return &quayReader{client: &guarded, repository: repository, token: authorization.Token}, nil
}

func (q *quayReader) request(ctx context.Context, method, kind, ref string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, "https://quay.io/v2/"+q.repository+"/"+kind+"/"+ref, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+q.token)
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json")
	response, err := q.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Quay metadata request failed")
	}
	return response, nil
}

// CheckTagAbsent checks the final version before staging or promotion. Only an
// authenticated registry 404 permits publication; errors never imply absence.
// This check does not provide atomic or registry-enforced tag immutability.
func CheckTagAbsent(ctx context.Context, client *http.Client, image, tag, username, password string) error {
	if !tagPattern.MatchString(tag) {
		return fmt.Errorf("valid version identity required")
	}
	registry, err := newQuayReader(ctx, client, image, username, password)
	if err != nil {
		return err
	}
	manifest, err := registry.request(ctx, http.MethodHead, "manifests", tag)
	if err != nil {
		return fmt.Errorf("Quay version check failed: %w", err)
	}
	defer manifest.Body.Close()
	switch manifest.StatusCode {
	case http.StatusNotFound:
		return nil
	case http.StatusOK:
		return fmt.Errorf("version tag %s:%s already exists; start a fresh workflow run", image, tag)
	default:
		return fmt.Errorf("Quay version check returned HTTP %d", manifest.StatusCode)
	}
}
