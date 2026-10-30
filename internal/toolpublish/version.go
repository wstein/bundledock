package toolpublish

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var toolboxDatePattern = regexp.MustCompile(`^(\d{4}\.\d{2}\.\d{2})-b([1-9]\d*)$`)

var upstreamPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)
var singletonTagPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)-b([1-9]\d*)$`)

// SingletonVersion reads the official release from the resolved two-platform
// lock, rather than the desired constraint or the workflow's run counters.
func SingletonVersion(data []byte, bundle string) (string, error) {
	b, err := SelectBundle(bundle)
	if err != nil || b.DateTagged() {
		return "", fmt.Errorf("singleton bundle required")
	}
	var lock toolconfig.Lock
	if err := toolconfig.Decode(bytes.NewReader(data), &lock); err != nil {
		return "", err
	}
	if lock.Schema != 2 || len(lock.Platforms) != 2 {
		return "", fmt.Errorf("two-platform singleton lock required")
	}
	version := ""
	for _, arch := range []string{"amd64", "arm64"} {
		tools := lock.Platforms[arch]
		if len(tools) != 1 {
			return "", fmt.Errorf("one tool per native platform required")
		}
		tool := b.Name
		if tool == "node-current" {
			tool = "node"
		}
		pin, ok := tools[tool]
		if !ok {
			return "", fmt.Errorf("singleton tool identity mismatch")
		}
		v := pin.Version
		if b.Name == "codex" {
			v = strings.TrimPrefix(v, "rust-v")
		} else if b.Name == "bun" {
			v = strings.TrimPrefix(v, "bun-v")
		} else {
			v = strings.TrimPrefix(v, "v")
		}
		if !upstreamPattern.MatchString(v) || (version != "" && v != version) {
			return "", fmt.Errorf("matching stable upstream versions required")
		}
		version = v
	}
	return version, nil
}

// NextSingletonTag advances packaging revisions only for this upstream release.
// Historical date tags and tags for other releases remain untouched.
func NextSingletonTag(version string, tags []string) (string, error) {
	if !upstreamPattern.MatchString(version) {
		return "", fmt.Errorf("stable upstream version required")
	}
	var revision uint64
	for _, tag := range tags {
		if !strings.HasPrefix(tag, version+"-b") || !singletonTagPattern.MatchString(tag) {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimPrefix(tag, version+"-b"), 10, 64)
		if err != nil || n == ^uint64(0) {
			return "", fmt.Errorf("packaging revision exhausted")
		}
		if n > revision {
			revision = n
		}
	}
	return fmt.Sprintf("%s-b%d", version, revision+1), nil
}

// AllocateSingletonTag enumerates authenticated registry tags before native
// work. A complete listing is required; failures never imply a first revision.
// Publication concurrency and the existing absence checks still guard races.
func AllocateSingletonTag(ctx context.Context, client *http.Client, image string, lock []byte, username, password string, reserved ...string) (string, error) {
	version, err := SingletonVersion(lock, bundleForImage(image).Name)
	if err != nil {
		return "", err
	}
	return allocateRegistryTag(ctx, client, image, username, password, reserved, func(tags []string) (string, error) { return NextSingletonTag(version, tags) })
}

// NextToolboxTag reserves only the new date/revision namespace. Historical
// run/attempt tags remain untouched and do not inflate the new daily counter.
func NextToolboxTag(date string, tags []string) (string, error) {
	parsed, err := time.Parse("2006.01.02", date)
	if err != nil || parsed.Format("2006.01.02") != date {
		return "", fmt.Errorf("canonical UTC date required")
	}
	var revision uint64
	for _, tag := range tags {
		if strings.HasPrefix(tag, "staging-") {
			if !strings.HasSuffix(tag, "-amd64") && !strings.HasSuffix(tag, "-arm64") {
				continue
			}
			tag = strings.TrimPrefix(tag, "staging-")
			tag = strings.TrimSuffix(strings.TrimSuffix(tag, "-amd64"), "-arm64")
		}
		match := toolboxDatePattern.FindStringSubmatch(tag)
		if match == nil || match[1] != date {
			continue
		}
		n, err := strconv.ParseUint(match[2], 10, 64)
		if err != nil || n == ^uint64(0) {
			return "", fmt.Errorf("daily packaging revision exhausted")
		}
		if n > revision {
			revision = n
		}
	}
	return fmt.Sprintf("%s-b%d", date, revision+1), nil
}

// AllocateToolboxTag allocates the daily tag of a date-tagged bundle (toolbox or nixbox).
func AllocateToolboxTag(ctx context.Context, client *http.Client, image, date, username, password string, reserved ...string) (string, error) {
	if !strings.HasSuffix(image, "/toolbox") && !strings.HasSuffix(image, "/nixbox") {
		return "", fmt.Errorf("toolbox or nixbox repository required")
	}
	if _, err := NextToolboxTag(date, nil); err != nil {
		return "", err
	}
	return allocateRegistryTag(ctx, client, image, username, password, reserved, func(tags []string) (string, error) { return NextToolboxTag(date, tags) })
}

func allocateRegistryTag(ctx context.Context, client *http.Client, image, username, password string, reserved []string, next func([]string) (string, error)) (string, error) {
	registry, err := newQuayReader(ctx, client, image, username, password)
	if err != nil {
		return "", err
	}
	tags := append([]string{}, reserved...)
	last := ""
	for page := 0; page < 1000; page++ {
		response, err := registry.request(ctx, http.MethodGet, "tags", "list?n=1000&last="+url.QueryEscape(last))
		if err != nil {
			return "", err
		}
		data, readErr := readBounded(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || readErr != nil {
			return "", fmt.Errorf("complete registry version listing required (HTTP %d)", response.StatusCode)
		}
		var listing struct {
			Name string          `json:"name"`
			Tags json.RawMessage `json:"tags"`
		}
		if err := json.Unmarshal(data, &listing); err != nil || listing.Name != registry.repository {
			return "", fmt.Errorf("invalid registry version listing")
		}
		var listed []string
		if len(listing.Tags) == 0 || json.Unmarshal(listing.Tags, &listed) != nil {
			return "", fmt.Errorf("invalid registry tag list")
		}
		tags = append(tags, listed...)
		if len(listed) < 1000 && response.Header.Get("Link") == "" {
			return next(tags)
		}
		if len(listed) == 0 || listed[len(listed)-1] <= last {
			return "", fmt.Errorf("registry version pagination did not advance")
		}
		last = listed[len(listed)-1]
	}
	return "", fmt.Errorf("registry version listing exceeds page limit")
}
