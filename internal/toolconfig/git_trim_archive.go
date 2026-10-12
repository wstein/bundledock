package toolconfig

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"
)

// ReadGitTrimArchive verifies before extraction, and never executes upstream code.
func ReadGitTrimArchive(data []byte, expectedSHA256 string) (map[string][]byte, error) {
	if len(data) > 8<<20 || fmt.Sprintf("%x", sha256.Sum256(data)) != expectedSHA256 {
		return nil, fmt.Errorf("git-trim source checksum mismatch or size exceeded")
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	bounded := &io.LimitedReader{R: gz, N: 32<<20 + 1}
	t := tar.NewReader(bounded)
	files := map[string][]byte{}
	prefix := "git-trim-" + GitTrimCommit + "/"
	for {
		h, err := t.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		if !strings.HasPrefix(h.Name, prefix) {
			return nil, fmt.Errorf("unexpected git-trim source root")
		}
		n := strings.TrimSuffix(strings.TrimPrefix(h.Name, prefix), "/")
		if n == "" && h.Typeflag == tar.TypeDir {
			continue
		}
		if !validPath(n) || n == ".git" || strings.HasPrefix(n, ".git/") {
			return nil, fmt.Errorf("unsafe git-trim source path")
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if h.Typeflag != tar.TypeReg || h.Size < 0 || h.Size > 8<<20 {
			return nil, fmt.Errorf("unsupported git-trim source member")
		}
		if _, exists := files[n]; exists {
			return nil, fmt.Errorf("duplicate git-trim source member")
		}
		b, err := io.ReadAll(t)
		if err != nil {
			return nil, err
		}
		files[n] = b
	}
	if bounded.N <= 0 {
		return nil, fmt.Errorf("git-trim source expansion exceeded")
	}
	if fmt.Sprintf("%x", sha256.Sum256(files["Cargo.lock"])) != GitTrimCargoLockSHA256 {
		return nil, fmt.Errorf("git-trim Cargo.lock changed; source review required")
	}
	if !bytes.Contains(files["Cargo.toml"], []byte("name = \"git-trim\"\n")) || !bytes.Contains(files["Cargo.toml"], []byte("version = \"0.4.3\"\n")) || len(files["LICENSE"]) == 0 {
		return nil, fmt.Errorf("git-trim source manifest or license mismatch")
	}
	return files, nil
}
