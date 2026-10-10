package toolbundle

import (
	"archive/tar"
	"bundledock/internal/toolconfig"
	"compress/gzip"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

// PrepareBashSource verifies every input before extracting or publishing patches.
// The fixed Docker compiler stage applies them in order; no catalog commands run.
func PrepareBashSource(ctx context.Context, client *http.Client, s Spec, destination string) error {
	base, _, _, err := toolconfig.BashBase(s.Version)
	if err != nil {
		return err
	}
	baseURL := strings.TrimSuffix(s.URL, "/bash-"+base+".tar.gz")
	if err := toolconfig.ValidateBashInputs(s, baseURL); err != nil {
		return err
	}
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	inputs := []Spec{s}
	for _, p := range s.Patches {
		inputs = append(inputs, Spec{URL: p.URL, SHA256: p.SHA256})
	}
	var files []*os.File
	defer func() {
		for _, f := range files {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	for _, input := range inputs {
		f, err := verifiedBashInput(ctx, client, input)
		if err != nil {
			return err
		}
		files = append(files, f)
	}
	if err := os.MkdirAll(destination, 0755); err != nil {
		return err
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	gz, err := gzip.NewReader(files[0])
	if err != nil {
		return err
	}
	defer gz.Close()
	limited := &io.LimitedReader{R: gz, N: 128<<20 + 1}
	archive := tar.NewReader(limited)
	seen := map[string]bool{}
	for {
		entry, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err := ValidPath(entry.Name); err != nil {
			return err
		}
		archiveName := path.Clean(entry.Name)
		if archiveName == "bash-"+base && entry.Typeflag == tar.TypeDir {
			continue
		}
		if !strings.HasPrefix(archiveName, "bash-"+base+"/") {
			return fmt.Errorf("unexpected Bash source member %q", entry.Name)
		}
		name := strings.TrimPrefix(archiveName, "bash-"+base+"/")
		if name == "" && entry.Typeflag == tar.TypeDir {
			continue
		}
		if seen[name] || len(seen) >= 16384 {
			return fmt.Errorf("duplicate or excessive Bash source members")
		}
		seen[name] = true
		switch entry.Typeflag {
		case tar.TypeDir:
			if err := mkdir(root, "source/"+name); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if entry.Size > 32<<20 {
				return fmt.Errorf("Bash source member too large")
			}
			mode := os.FileMode(0644)
			if entry.Mode&0111 != 0 {
				mode = 0755
			}
			if err := WriteFile(root, "source/"+name, archive, mode); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported Bash source entry %q", name)
		}
	}
	if _, err := io.Copy(io.Discard, limited); err != nil {
		return err
	}
	if limited.N == 0 {
		return fmt.Errorf("Bash source expands beyond limit")
	}
	if !seen["configure"] || !seen["patchlevel.h"] {
		return fmt.Errorf("missing Bash build sources")
	}
	for i, p := range s.Patches {
		if err := WriteFile(root, "patches/"+p.URL[strings.LastIndex(p.URL, "/")+1:], files[i+1], 0644); err != nil {
			return err
		}
	}
	return nil
}

func verifiedBashInput(ctx context.Context, client *http.Client, s Spec) (*os.File, error) {
	h, want, err := checksum(s)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp("", "bundledock-bash-*")
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(f.Name())
		}
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("Bash input HTTP %s", resp.Status)
	}
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 32<<20+1))
	closeErr := resp.Body.Close()
	if copyErr != nil {
		return nil, copyErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if n > 32<<20 {
		return nil, fmt.Errorf("Bash input too large")
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return nil, fmt.Errorf("Bash input checksum mismatch")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	ok = true
	return f, nil
}
