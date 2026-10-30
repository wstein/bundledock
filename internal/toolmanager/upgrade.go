// Package toolmanager resolves stable upstream releases into an atomic tool lock.
package toolmanager

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Upgrade only writes packages.lock.json after every selected tool has resolved and
// verified both architectures. It never builds or activates an OCI tool volume.
func Upgrade(ctx context.Context, client *http.Client, directory, tool string, output io.Writer) error {
	lockPath := filepath.Join(directory, "packages.lock.json")
	c, err := toolconfig.LoadInputs(directory)
	if err != nil {
		return err
	}
	info, err := os.Lstat(lockPath)
	exists := err == nil
	mode := os.FileMode(0644)
	var before []byte
	// previous holds the pins of the lock on disk: source builds keep the toolchains
	// and extra sources of that lock, and a bump may not go backwards.
	var previous toolconfig.Lock
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !exists {
		if tool != "" {
			return fmt.Errorf("packages.lock.json is missing; run bdock tools upgrade without a tool")
		}
		c.Lock = toolconfig.Lock{Platforms: map[string]map[string]toolconfig.Spec{"amd64": {}, "arm64": {}}}
	} else {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("packages.lock.json must be a regular file")
		}
		mode = info.Mode().Perm()
		before, err = os.ReadFile(lockPath)
		if err != nil {
			return err
		}
		if tool != "" {
			c, err = toolconfig.LoadForUpgrade(directory)
			if err == nil {
				previous = c.Lock
			}
			if err != nil {
				return fmt.Errorf("targeted upgrade requires an unchanged package inventory; run bdock tools upgrade: %w", err)
			}
			if c.Lock.CatalogDigest != c.CatalogDigest() {
				return fmt.Errorf("package catalog changed; run bdock tools upgrade without a tool")
			}
		} else {
			if err := toolconfig.Decode(bytes.NewReader(before), &previous); err != nil {
				return err
			}
			if previous.Schema != 2 {
				return fmt.Errorf("unsupported lock schema %d", previous.Schema)
			}
			// Toolchains and extra sources are recipe pins of the owner: the
			// upgrade never resolves or edits them, it carries them over unchanged.
			c.Lock = toolconfig.Lock{Platforms: map[string]map[string]toolconfig.Spec{"amd64": {}, "arm64": {}}, Toolchains: previous.Toolchains, ExtraSources: previous.ExtraSources}
		}
	}
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	names := []string{}
	if tool != "" {
		if _, ok := c.Desired[tool]; !ok {
			return fmt.Errorf("unknown tool %q", tool)
		}
		names = append(names, tool)
	} else {
		for n := range c.Desired {
			names = append(names, n)
		}
		sort.Strings(names)
	}
	resolver := resolver{client: client}
	var report bytes.Buffer
	for _, n := range names {
		specs, err := resolver.resolve(ctx, c.Catalog.Packages[n], c.Desired[n])
		if err != nil {
			return fmt.Errorf("upgrade %s: %w", n, err)
		}
		if c.Catalog.Packages[n].Provider == "source-build" {
			if err := checkSourceBump(previous.Platforms["amd64"][n], specs["amd64"]); err != nil {
				return fmt.Errorf("upgrade %s: %w", n, err)
			}
		}
		for _, arch := range []string{"amd64", "arm64"} {
			c.Lock.Platforms[arch][n] = specs[arch]
		}
		fmt.Fprintf(&report, "%s %s\n", n, specs["amd64"].Version)
	}
	c.Bind()
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c.Lock, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	// Refuse to overwrite an edit made while network resolution was in progress.
	current, readErr := os.ReadFile(lockPath)
	if exists {
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(current, before) {
			return fmt.Errorf("packages.lock.json changed during upgrade")
		}
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("packages.lock.json appeared during upgrade")
	}
	if !bytes.Equal(data, before) {
		temporary, err := os.CreateTemp(directory, ".tools.lock-*")
		if err != nil {
			return err
		}
		defer os.Remove(temporary.Name())
		if err = temporary.Chmod(mode); err == nil {
			_, err = temporary.Write(data)
		}
		if err == nil {
			err = temporary.Sync()
		}
		closeErr := temporary.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if _, err := io.Copy(output, &report); err != nil {
			return err
		}
		if err := os.Rename(temporary.Name(), lockPath); err != nil {
			return err
		}
	} else {
		if _, err := io.Copy(output, &report); err != nil {
			return err
		}
	}
	return nil
}

// checkSourceBump allows a source-build pin to change only forward: a lower
// version, or new bytes under the same version, is a changed source, not a bump.
func checkSourceBump(old, next toolconfig.Spec) error {
	if old.Version == "" {
		return nil
	}
	oldVersion, err := toolconfig.ParseVersion(old.Version)
	if err != nil {
		return err
	}
	nextVersion, err := toolconfig.ParseVersion(next.Version)
	if err != nil {
		return err
	}
	switch nextVersion.Compare(oldVersion) {
	case -1:
		return fmt.Errorf("version %s is older than the locked %s", next.Version, old.Version)
	case 0:
		if old != next {
			return fmt.Errorf("the source of locked version %s changed upstream", old.Version)
		}
	}
	return nil
}
