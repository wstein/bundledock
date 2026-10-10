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

// CheckTagAbsent checks the final version before staging or promotion. Only an
// authenticated registry 404 permits publication; errors never imply absence.
// This check does not provide atomic or registry-enforced tag immutability.
func CheckTagAbsent(ctx context.Context, client *http.Client, image, tag, username, password string) error {
	if !imagePattern.MatchString(image) || !tagPattern.MatchString(tag) || username == "" || password == "" {
		return fmt.Errorf("valid version identity and Quay robot credentials required")
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
		return err
	}
	req.SetBasicAuth(username, password)
	response, err := guarded.Do(req)
	if err != nil {
		return fmt.Errorf("Quay pull-token request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Quay pull-token request returned HTTP %d", response.StatusCode)
	}
	var authorization struct {
		Token string `json:"token"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&authorization); err != nil {
		return fmt.Errorf("invalid Quay pull-token response")
	}
	if authorization.Token == "" {
		return fmt.Errorf("empty Quay pull-token response")
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodHead, "https://quay.io/v2/"+repository+"/manifests/"+tag, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+authorization.Token)
	req.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json")
	manifest, err := guarded.Do(req)
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
