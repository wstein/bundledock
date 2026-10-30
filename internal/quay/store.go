package quay

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// DeleteTag untags exactly one staging tag through the registry v2 API with a
// robot account. Quay's tag-name route removes only this name; the manifest
// digest is never used, since deleting by digest would remove other tags. An
// already absent tag is not an error. The robot password is sent only to the
// registry token endpoint, never to the REST API.
func (c *Client) DeleteTag(ctx context.Context, repository, name, username, password string) error {
	// Reject aliases, digests and foreign repositories before any authentication request.
	if _, _, err := Identity(repository, name); err != nil {
		return err
	}
	if username == "" || password == "" {
		return fmt.Errorf("REPOSITORY_USERNAME and REPOSITORY_TOKEN required for deletion")
	}
	registry, err := c.registryToken(ctx, repository, "pull,push", username, password)
	if err != nil {
		return err
	}
	_, err = c.Do(ctx, http.MethodDelete, registry.path("manifests", name), http.Header{"Authorization": {"Bearer " + registry.token}}, nil, http.StatusAccepted)
	var status *StatusError
	if errors.As(err, &status) && status.Code == http.StatusNotFound {
		return nil
	}
	return err
}

// Store adapts a Client to TagStore. Repositories of bundles that have no
// publication record yet may be missing on Quay and then count as empty.
type Store struct {
	Client             *Client
	Username, Password string
	// Unpublished reports repositories whose absence is not yet an error.
	Unpublished func(repository string) bool
}

// Tags implements TagStore.
func (s Store) Tags(ctx context.Context, repository, name string) ([]Tag, error) {
	if !knownRepository(repository) {
		return nil, reasonError("Unknown repository")
	}
	if name != "" {
		if _, _, err := Identity(repository, name); err != nil {
			return nil, err
		}
	}
	tags, err := s.Client.Tags(ctx, repository, name)
	var status *StatusError
	if err != nil && s.Unpublished != nil && s.Unpublished(repository) && errors.As(err, &status) && (status.Code == 404 || status.Code == 401) {
		return []Tag{}, nil
	}
	return tags, err
}

// Delete implements TagStore.
func (s Store) Delete(ctx context.Context, repository, name string) error {
	return s.Client.DeleteTag(ctx, repository, name, s.Username, s.Password)
}
