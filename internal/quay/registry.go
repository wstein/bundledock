package quay

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
)

const maxMetadata = 8 << 20

const manifestAccept = "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"

var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)

// Registry reads manifests and blobs of one repository through the registry v2 API.
type Registry struct {
	client     *Client
	repository string
	token      string
}

// Registry opens a pull-only registry session. Without credentials the token
// is anonymous, which is enough for the public repositories.
func (c *Client) Registry(ctx context.Context, repository string) (*Registry, error) {
	return c.registryToken(ctx, repository, "pull", "", "")
}

// RegistryAs opens a pull-only session with a robot account when both
// credentials are set, else anonymously. The robot is only used on quay.io.
func (c *Client) RegistryAs(ctx context.Context, repository, username, password string) (*Registry, error) {
	if username == "" || password == "" {
		return c.Registry(ctx, repository)
	}
	return c.registryToken(ctx, repository, "pull", username, password)
}

func (c *Client) registryToken(ctx context.Context, repository, actions, username, password string) (*Registry, error) {
	query := url.Values{"service": {"quay.io"}, "scope": {"repository:" + Namespace + "/" + repository + ":" + actions}}
	header := http.Header{}
	if username != "" {
		if err := c.requireDefaultHost(); err != nil {
			return nil, err
		}
		header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(username+":"+password)))
	}
	raw, err := c.Do(ctx, http.MethodGet, "/v2/auth?"+query.Encode(), header, nil, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var data struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(raw, &data); err != nil || !tokenPattern.MatchString(data.Token) {
		return nil, fmt.Errorf("invalid registry authentication token")
	}
	return &Registry{client: c, repository: repository, token: data.Token}, nil
}

func (r *Registry) path(kind, ref string) string {
	return "/v2/" + Namespace + "/" + r.repository + "/" + kind + "/" + ref
}

// get fetches a manifest or blob. A blob may redirect once to signed https
// storage; no credential is sent there.
func (r *Registry) get(ctx context.Context, kind, ref string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.client.base+r.path(kind, ref), nil)
	if err != nil {
		return nil, fmt.Errorf("invalid registry request")
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", manifestAccept)
	req.Header.Set("User-Agent", "bdock-quay")
	response, err := r.client.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Quay request failed")
	}
	if kind == "blobs" && response.StatusCode >= 301 && response.StatusCode <= 308 && response.StatusCode != 304 && response.StatusCode != 305 && response.StatusCode != 306 {
		target, perr := url.Parse(response.Header.Get("Location"))
		response.Body.Close()
		if perr != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil || target.Port() != "" {
			return nil, fmt.Errorf("unsafe signed blob redirect")
		}
		signed, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("unsafe signed blob redirect")
		}
		if response, err = r.client.http.Do(signed); err != nil {
			return nil, fmt.Errorf("Quay request failed")
		}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, &StatusError{Code: response.StatusCode}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxMetadata+1))
	if err != nil || len(data) > maxMetadata {
		return nil, fmt.Errorf("registry metadata unreadable or too large")
	}
	if digest, ok := cutDigest(ref); ok {
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != digest {
			return nil, fmt.Errorf("registry content digest mismatch")
		}
	}
	return data, nil
}

func cutDigest(ref string) (string, bool) {
	if digestPattern.MatchString(ref) {
		return ref[len("sha256:"):], true
	}
	return "", false
}

// Manifest fetches a manifest by digest.
func (r *Registry) Manifest(ctx context.Context, digest string) ([]byte, error) {
	if !digestPattern.MatchString(digest) {
		return nil, fmt.Errorf("invalid digest")
	}
	return r.get(ctx, "manifests", digest)
}

// refPattern matches the references a version tag or alias read may use.
var refPattern = regexp.MustCompile(`^(?:sha256:[0-9a-f]{64}|\d{4}\.\d{2}\.\d{2}-b[1-9]\d*|\d{4}\.\d{2}\.\d{2}-b[1-9]\d*\.[1-9]\d*|(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)-b[1-9]\d*|\d{4}\.\d{2}\.\d{2}-[1-9]\d*-[1-9]\d*|stable|latest|lts|current)$`)

// ManifestRef fetches a manifest by version tag or alias name (or digest). The
// content is not verified against a name; the caller compares its digest.
func (r *Registry) ManifestRef(ctx context.Context, ref string) ([]byte, error) {
	if !refPattern.MatchString(ref) {
		return nil, fmt.Errorf("invalid registry metadata request")
	}
	return r.get(ctx, "manifests", ref)
}

// Blob fetches a blob by digest.
func (r *Registry) Blob(ctx context.Context, digest string) ([]byte, error) {
	if !digestPattern.MatchString(digest) {
		return nil, fmt.Errorf("invalid digest")
	}
	return r.get(ctx, "blobs", digest)
}

// Image is what the registry says about one manifest digest.
type Image struct {
	Architectures []string
	Version       string
	Revision      string
}

// Inspect reads a manifest or index and the image config for the labels.
func (r *Registry) Inspect(ctx context.Context, digest string) (Image, error) {
	raw, err := r.Manifest(ctx, digest)
	if err != nil {
		return Image{}, err
	}
	var doc struct {
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return Image{}, fmt.Errorf("malformed manifest")
	}
	var image Image
	configDigest := doc.Config.Digest
	if len(doc.Manifests) > 0 {
		first := ""
		for _, m := range doc.Manifests {
			image.Architectures = append(image.Architectures, m.Platform.OS+"/"+m.Platform.Architecture)
			if first == "" {
				first = m.Digest
			}
		}
		child, err := r.Manifest(ctx, first)
		if err != nil {
			return Image{}, err
		}
		var c struct {
			Config struct {
				Digest string `json:"digest"`
			} `json:"config"`
		}
		if err := json.Unmarshal(child, &c); err != nil {
			return Image{}, fmt.Errorf("malformed manifest")
		}
		configDigest = c.Config.Digest
	}
	if configDigest == "" {
		return image, nil
	}
	blob, err := r.Blob(ctx, configDigest)
	if err != nil {
		return Image{}, err
	}
	var config struct {
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
		Config       struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err := json.Unmarshal(blob, &config); err != nil {
		return Image{}, fmt.Errorf("malformed image config")
	}
	if len(image.Architectures) == 0 {
		image.Architectures = []string{config.OS + "/" + config.Architecture}
	}
	image.Version = config.Config.Labels["org.opencontainers.image.version"]
	image.Revision = config.Config.Labels["org.opencontainers.image.revision"]
	return image, nil
}
