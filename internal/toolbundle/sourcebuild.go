package toolbundle

import (
	"archive/tar"
	"bundledock/internal/toolconfig"
	"context"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/ulikunitz/xz"
)

// Bounds of the downloads and of the unpacked toolchain. The zig 0.17.0 archive is
// 53 MB packed and 360 MB in 20,777 members, the largest being the 172 MB compiler.
const (
	maxToolchainDownload = 128 << 20
	maxSourceDownload    = 64 << 20
	maxExtraDownload     = 8 << 20
	maxToolchainMembers  = 50000
	maxToolchainMember   = 256 << 20
	maxToolchainTotal    = 1 << 30
)

type sum struct {
	name string
	h    hash.Hash
	want string
}

func newSum(name string, h hash.Hash, want string) (sum, error) {
	b, err := hex.DecodeString(want)
	if err != nil || len(b) != h.Size() || want != strings.ToLower(want) {
		return sum{}, fmt.Errorf("invalid %s checksum", name)
	}
	return sum{name, h, want}, nil
}

// specSums lists every checksum a lock entry carries; all must match.
func specSums(s Spec) ([]sum, error) {
	var out []sum
	for _, c := range []struct {
		name string
		h    hash.Hash
		want string
	}{{"sha256", sha256.New(), s.SHA256}, {"sha512", sha512.New(), s.SHA512}, {"sha3-256", sha3.New256(), s.SHA3256}} {
		if c.want == "" {
			continue
		}
		v, err := newSum(c.name, c.h, c.want)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no checksum to verify")
	}
	return out, nil
}

// verifiedDownload fetches url into a temporary file and returns it rewound only
// when it is within limit bytes and every checksum matches. Nothing reads or
// extracts the file before that.
func verifiedDownload(ctx context.Context, client *http.Client, url string, limit int64, sums []sum) (*os.File, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("download HTTP %s", response.Status)
	}
	file, err := os.CreateTemp("", "bundledock-source-*")
	if err != nil {
		return nil, 0, err
	}
	// The name goes at once: the open descriptor keeps the data readable.
	os.Remove(file.Name())
	writers := []io.Writer{file}
	for _, s := range sums {
		writers = append(writers, s.h)
	}
	n, err := io.Copy(io.MultiWriter(writers...), io.LimitReader(response.Body, limit+1))
	if err == nil && n > limit {
		err = fmt.Errorf("download exceeds %d bytes", limit)
	}
	for _, s := range sums {
		if err == nil && hex.EncodeToString(s.h.Sum(nil)) != s.want {
			err = fmt.Errorf("%s checksum mismatch", s.name)
		}
	}
	if err == nil {
		_, err = file.Seek(0, io.SeekStart)
	}
	if err != nil {
		file.Close()
		return nil, 0, err
	}
	return file, n, nil
}

// FetchSourceBuild gathers everything the offline compile of one source-build tool
// needs: destination/toolchain (the pinned compiler for arch) and destination/src
// (the listed sources, headers and extra-source files). Every download is verified
// against the lock before it is read or unpacked. It compiles nothing.
func FetchSourceBuild(ctx context.Context, client *http.Client, c *toolconfig.Config, tool, arch, destination string, output io.Writer) error {
	p, ok := c.Catalog.Packages[tool]
	if !ok || p.Provider != "source-build" || p.Build == nil {
		return fmt.Errorf("%s is not a source-build tool", tool)
	}
	spec, ok := c.Lock.Platforms[arch][tool]
	toolchain, tcOK := c.Lock.Toolchains[p.Build.Toolchain][arch]
	if !ok || !tcOK {
		return fmt.Errorf("%s: the lock lacks the %s source or its toolchain", tool, arch)
	}
	if client == nil {
		client = &http.Client{Timeout: 300 * time.Second}
	}
	if err := os.MkdirAll(destination, 0755); err != nil {
		return err
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	if entries, err := fs.ReadDir(root.FS(), "."); err != nil || len(entries) != 0 {
		return fmt.Errorf("source-build destination must be empty; use a fresh directory")
	}

	fmt.Fprintln(output, "Fetching toolchain", p.Build.Toolchain, toolchain.Version)
	sums, err := specSums(toolchain)
	if err != nil {
		return fmt.Errorf("toolchain %s: %w", p.Build.Toolchain, err)
	}
	archive, _, err := verifiedDownload(ctx, client, toolchain.URL, maxToolchainDownload, sums)
	if err != nil {
		return fmt.Errorf("toolchain %s: %w", p.Build.Toolchain, err)
	}
	err = extractToolchain(root, archive, strings.TrimSuffix(path.Base(toolchain.URL), ".tar.xz"))
	archive.Close()
	if err != nil {
		return fmt.Errorf("toolchain %s: %w", p.Build.Toolchain, err)
	}

	fmt.Fprintln(output, "Fetching", tool, spec.Version, "sources")
	numeric, err := toolconfig.NumericVersion(spec.Version)
	if err != nil {
		return err
	}
	var extraNames []string
	for _, n := range p.Build.ExtraSources {
		for _, target := range p.ExtraSources[n].Files {
			extraNames = append(extraNames, target)
		}
	}
	sums, err = specSums(spec)
	if err != nil {
		return fmt.Errorf("%s: %w", tool, err)
	}
	source, size, err := verifiedDownload(ctx, client, spec.URL, maxSourceDownload, sums)
	if err != nil {
		return fmt.Errorf("%s: %w", tool, err)
	}
	if err := MakeDirectory(root, "src"); err != nil {
		source.Close()
		return err
	}
	srcRoot, err := root.OpenRoot("src")
	if err != nil {
		source.Close()
		return err
	}
	defer srcRoot.Close()
	err = ExtractBuildSources(srcRoot, source, size, strings.ReplaceAll(p.Build.ArchivePrefix, "{numeric}", numeric), p.Build.Sources, extraNames)
	source.Close()
	if err != nil {
		return fmt.Errorf("%s: %w", tool, err)
	}

	for _, n := range p.Build.ExtraSources {
		locked := c.Lock.ExtraSources[tool+"/"+n]
		files := make([]string, 0, len(locked.Files))
		for file := range locked.Files {
			files = append(files, file)
		}
		sort.Strings(files)
		for _, file := range files {
			fmt.Fprintln(output, "Fetching", tool+"/"+n, file, "at", locked.Commit)
			lf := locked.Files[file]
			want, err := newSum("sha256", sha256.New(), lf.SHA256)
			if err != nil {
				return fmt.Errorf("%s/%s %s: %w", tool, n, file, err)
			}
			data, _, err := verifiedDownload(ctx, client, lf.URL, maxExtraDownload, []sum{want})
			if err != nil {
				return fmt.Errorf("%s/%s %s: %w", tool, n, file, err)
			}
			err = WriteFile(srcRoot, p.ExtraSources[n].Files[file], data, 0644)
			data.Close()
			if err != nil {
				return fmt.Errorf("%s/%s %s: %w", tool, n, file, err)
			}
		}
	}
	return nil
}

// extractToolchain unpacks a verified compiler tar.xz below root/toolchain, keeping
// only directories and regular files of the single expected top directory, within
// fixed member, size and total bounds.
func extractToolchain(root *os.Root, source io.Reader, top string) error {
	reader, err := xz.NewReader(source)
	if err != nil {
		return err
	}
	limited := &io.LimitedReader{R: reader, N: maxToolchainTotal + 1}
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
		if limited.N <= 0 {
			return fmt.Errorf("toolchain archive exceeds %d bytes", int64(maxToolchainTotal))
		}
		if err := ValidPath(entry.Name); err != nil {
			return err
		}
		name := path.Clean(entry.Name)
		if len(seen) >= maxToolchainMembers {
			return fmt.Errorf("too many toolchain archive members")
		}
		if seen[name] {
			return fmt.Errorf("duplicate toolchain archive path %q", name)
		}
		seen[name] = true
		rel, ok := strings.CutPrefix(name, top)
		if name != top && (!ok || !strings.HasPrefix(rel, "/")) {
			return fmt.Errorf("toolchain member %q is outside %s", name, top)
		}
		rel = strings.TrimPrefix(rel, "/")
		switch entry.Typeflag {
		case tar.TypeDir:
			if rel != "" {
				if err := mkdir(root, "toolchain/"+rel); err != nil {
					return err
				}
			} else if err := mkdir(root, "toolchain"); err != nil {
				return err
			}
		case tar.TypeReg:
			if rel == "" {
				return fmt.Errorf("toolchain root %q is not a directory", top)
			}
			if entry.Size > maxToolchainMember {
				return fmt.Errorf("toolchain member %q too large", name)
			}
			mode := fs.FileMode(0644)
			if entry.FileInfo().Mode().Perm()&0111 != 0 {
				mode = 0755
			}
			if err := WriteFile(root, "toolchain/"+rel, archive, mode); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported toolchain archive entry %q", name)
		}
	}
	info, err := root.Lstat("toolchain/zig")
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0100 == 0 {
		return fmt.Errorf("toolchain archive lacks an executable zig below %s", top)
	}
	return nil
}
