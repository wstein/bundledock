// Package quay is a small client for the public bundledock namespace on Quay.
//
// Reads use the anonymous REST API and never carry credentials. Writes are
// separate, explicit methods that take their token as an argument.
package quay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	// DefaultBaseURL is the production Quay endpoint.
	DefaultBaseURL = "https://quay.io"
	// Namespace is the Quay organization that owns all bundle repositories.
	Namespace = "bundledock"

	maxBody  = 4 << 20
	maxPages = 1000
	timeout  = 30 * time.Second
)

// ErrCredentialHost is returned when a secret would leave for another host.
var ErrCredentialHost = fmt.Errorf("credentials are only sent to %s", DefaultBaseURL)

func (c *Client) requireDefaultHost() error {
	if c.base != DefaultBaseURL {
		return ErrCredentialHost
	}
	return nil
}

// StatusError reports an unexpected HTTP status. It never carries a body.
type StatusError struct{ Code int }

func (e *StatusError) Error() string { return fmt.Sprintf("Quay HTTP %d", e.Code) }

// Client talks to one Quay endpoint.
type Client struct {
	http *http.Client
	base string
}

// New returns a client. A nil httpClient and an empty baseURL select the defaults.
// The client never follows redirects and has a 30 second timeout unless the
// injected client sets its own.
func New(httpClient *http.Client, baseURL string) *Client {
	guarded := http.Client{Timeout: timeout}
	if httpClient != nil {
		guarded = *httpClient
		if guarded.Timeout == 0 {
			guarded.Timeout = timeout
		}
	}
	guarded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{http: &guarded, base: baseURL}
}

// Do sends one request and returns the body, which is limited to 4 MiB.
// A status other than want is a *StatusError.
func (c *Client) Do(ctx context.Context, method, path string, header http.Header, body []byte, want int) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return nil, fmt.Errorf("invalid Quay request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "bdock-quay")
	for key, values := range header {
		req.Header[key] = values
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Quay request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		return nil, &StatusError{Code: response.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("Quay response unreadable")
	}
	if len(raw) > maxBody {
		return nil, fmt.Errorf("Quay response too large")
	}
	return raw, nil
}

// Tag is one active tag of a repository.
type Tag struct {
	Name           string `json:"name"`
	ManifestDigest string `json:"manifest_digest"`
	LastModified   string `json:"last_modified"`
	IsManifestList bool   `json:"is_manifest_list"`
	Immutable      bool   `json:"immutable"`
}

// Modified parses the Quay timestamp (RFC 1123 with a numeric zone).
func (t Tag) Modified() (time.Time, error) {
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC3339} {
		if parsed, err := time.Parse(layout, t.LastModified); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid tag timestamp")
}

// Tags lists the active tags of a repository. A non-empty name asks for that
// one tag only.
func (c *Client) Tags(ctx context.Context, repository, name string) ([]Tag, error) {
	var result []Tag
	seen := map[string]bool{}
	for page := 1; page <= maxPages; page++ {
		query := url.Values{"onlyActiveTags": {"true"}, "limit": {"100"}, "page": {fmt.Sprint(page)}}
		if name != "" {
			query.Set("specificTag", name)
		}
		raw, err := c.Do(ctx, http.MethodGet, "/api/v1/repository/"+Namespace+"/"+repository+"/tag/?"+query.Encode(), nil, nil, http.StatusOK)
		if err != nil {
			return nil, err
		}
		var data struct {
			Page          int   `json:"page"`
			Tags          []Tag `json:"tags"`
			HasAdditional *bool `json:"has_additional"`
		}
		if err := json.Unmarshal(raw, &data); err != nil || data.Page != page || data.Tags == nil || data.HasAdditional == nil {
			return nil, fmt.Errorf("malformed Quay pagination")
		}
		for _, tag := range data.Tags {
			if tag.Name == "" || seen[tag.Name] || (name != "" && tag.Name != name) {
				return nil, fmt.Errorf("duplicate or invalid tag")
			}
			seen[tag.Name] = true
			result = append(result, tag)
		}
		if !*data.HasAdditional {
			return result, nil
		}
		if len(data.Tags) == 0 {
			return nil, fmt.Errorf("empty additional page")
		}
	}
	return nil, fmt.Errorf("Quay pagination limit exceeded")
}

// Repository is the metadata of a repository.
type Repository struct {
	Description string `json:"description"`
	IsPublic    bool   `json:"is_public"`
}

// Repository reads the repository metadata, anonymously.
func (c *Client) Repository(ctx context.Context, repository string) (*Repository, error) {
	raw, err := c.Do(ctx, http.MethodGet, "/api/v1/repository/"+Namespace+"/"+repository, nil, nil, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var result Repository
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("malformed Quay repository response")
	}
	return &result, nil
}
