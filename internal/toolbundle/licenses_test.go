package toolbundle

import (
	"archive/tar"
	"bundledock/internal/testdir"
	"bundledock/internal/toolconfig"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func licenseFixture(t *testing.T, reverse bool) (string, *os.Root) {
	t.Helper()
	dir := testdir.New(t)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	names := []string{"licenses/tool/SOURCE", "licenses/tool/LICENSE", "licenses/tool/dependencies/a-1/COPYING", "licenses/other/LICENSE"}
	if reverse {
		for i, j := 0, len(names)-1; i < j; i, j = i+1, j-1 {
			names[i], names[j] = names[j], names[i]
		}
	}
	for _, name := range names {
		if err := WriteFile(root, name, strings.NewReader("full original text: "+name+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(filepath.Join(dir, name), time.Now(), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteFile(root, "private-not-a-notice", strings.NewReader("do not archive"), 0644); err != nil {
		t.Fatal(err)
	}
	return dir, root
}

func TestLicenseArchiveDeterministicAndComplete(t *testing.T) {
	var firstArchive, firstIndex []byte
	for _, reverse := range []bool{false, true} {
		dir, root := licenseFixture(t, reverse)
		if err := WriteLicenseArchive(root); err != nil {
			t.Fatal(err)
		}
		archive, err := os.ReadFile(filepath.Join(dir, "licenses.tar.gz"))
		if err != nil {
			t.Fatal(err)
		}
		index, err := os.ReadFile(filepath.Join(dir, "licenses.index.json"))
		if err != nil {
			t.Fatal(err)
		}
		if reverse && (!bytes.Equal(archive, firstArchive) || !bytes.Equal(index, firstIndex)) {
			t.Fatal("notice creation order or mtime changed archive/index")
		}
		firstArchive, firstIndex = archive, index
		if _, err := os.Stat(filepath.Join(dir, "licenses")); !os.IsNotExist(err) {
			t.Fatal("loose licenses remain", err)
		}
		gz, err := gzip.NewReader(bytes.NewReader(archive))
		if err != nil {
			t.Fatal(err)
		}
		if !gz.ModTime.IsZero() || gz.Name != "" || gz.Comment != "" || gz.OS != 255 {
			t.Fatalf("unstable gzip header: %+v", gz.Header)
		}
		tr := tar.NewReader(gz)
		previous := ""
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			if h.Name <= previous || h.Uid != 0 || h.Gid != 0 || h.Mode != 0644 || h.ModTime.Unix() != 0 || h.Typeflag != tar.TypeReg {
				t.Fatalf("noncanonical header: %+v", h)
			}
			previous = h.Name
			if !strings.HasPrefix(h.Name, "licenses/") {
				t.Fatal("non-notice leaked", h.Name)
			}
		}
		if err := WriteManifest(root, "amd64", map[string]Spec{}); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := ReadLicenses(root, "licenses/tool/dependencies/a-1/COPYING", &out); err != nil {
			t.Fatal(err)
		}
		if out.String() != "full original text: licenses/tool/dependencies/a-1/COPYING\n" {
			t.Fatal("full nested notice changed")
		}
		out.Reset()
		if err := ReadLicenses(root, "", &out); err != nil || !bytes.Equal(out.Bytes(), index) {
			t.Fatalf("index listing: %v %q", err, out.Bytes())
		}
		sums, err := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"licenses.tar.gz", "licenses.index.json"} {
			if !strings.Contains(string(sums), "  "+name+"\n") {
				t.Fatal("unhashed notice artifact", name)
			}
		}
	}
}

func TestLicenseArchiveRefusesUnsafeStageAndOverwrite(t *testing.T) {
	for _, scenario := range []string{"archive exists", "index exists", "symlink", "hardlink", "bad package path"} {
		t.Run(scenario, func(t *testing.T) {
			dir, root := licenseFixture(t, false)
			switch scenario {
			case "archive exists":
				os.WriteFile(filepath.Join(dir, "licenses.tar.gz"), []byte("protected"), 0644)
			case "index exists":
				os.WriteFile(filepath.Join(dir, "licenses.index.json"), []byte("protected"), 0644)
			case "symlink":
				os.Symlink("../../private-not-a-notice", filepath.Join(dir, "licenses/tool/link"))
			case "hardlink":
				os.Link(filepath.Join(dir, "private-not-a-notice"), filepath.Join(dir, "licenses/tool/hardlink"))
			case "bad package path":
				os.WriteFile(filepath.Join(dir, "licenses/NOTICE"), []byte("unowned"), 0644)
			}
			if err := WriteLicenseArchive(root); err == nil {
				t.Fatal("unsafe stage accepted")
			}
			got, err := os.ReadFile(filepath.Join(dir, "licenses/tool/LICENSE"))
			if err != nil || !strings.HasPrefix(string(got), "full original text") {
				t.Fatal("stage lost on failure", err)
			}
			for _, name := range []string{"licenses.tar.gz", "licenses.index.json"} {
				if (scenario == "archive exists" && name == "licenses.tar.gz") || (scenario == "index exists" && name == "licenses.index.json") {
					got, err := os.ReadFile(filepath.Join(dir, name))
					if err != nil || string(got) != "protected" {
						t.Fatal("overwrote existing", name, err)
					}
				}
			}
		})
	}
}

func hostileLicenses(t *testing.T, names []string, kind byte, claimedSize int64, indexChange string) *os.Root {
	t.Helper()
	dir := testdir.New(t)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	tw := tar.NewWriter(gz)
	index := LicenseIndex{Schema: 1, Archive: "licenses.tar.gz"}
	for _, name := range names {
		body := []byte("full notice\n")
		size := int64(len(body))
		if claimedSize > 0 {
			size = claimedSize
		}
		h := &tar.Header{Name: name, Mode: 0644, Typeflag: kind, Size: size}
		if kind == tar.TypeSymlink || kind == tar.TypeLink {
			h.Linkname = "../../outside"
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 && claimedSize == 0 {
			if _, err := tw.Write(body); err != nil {
				t.Fatal(err)
			}
		}
		index.Files = append(index.Files, LicenseFile{Package: "tool", Path: name, Size: size, SHA256: fmt.Sprintf("%x", sha256.Sum256(body))})
	}
	// Deliberately truncated oversized fixtures are rejected at the header.
	if claimedSize == 0 {
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	index.SHA256 = fmt.Sprintf("%x", sha256.Sum256(compressed.Bytes()))
	switch indexChange {
	case "archive hash":
		index.SHA256 = strings.Repeat("0", 64)
	case "member hash":
		index.Files[0].SHA256 = strings.Repeat("0", 64)
	case "size":
		index.Files[0].Size++
	case "package":
		index.Files[0].Package = "other"
	case "archive path":
		index.Archive = "../outside"
	}
	data, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(root, "licenses.tar.gz", bytes.NewReader(compressed.Bytes()), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(root, "licenses.index.json", bytes.NewReader(data), 0644); err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(root, "amd64", map[string]Spec{}); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestLicenseReaderRejectsCorruptionAndUnsafeMembers(t *testing.T) {
	for _, scenario := range []string{"archive hash", "member hash", "size", "package", "archive path", "traversal", "duplicate", "symlink", "hardlink", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			names := []string{"licenses/tool/LICENSE"}
			kind := byte(tar.TypeReg)
			size := int64(0)
			switch scenario {
			case "traversal":
				names[0] = "licenses/tool/../../outside"
			case "duplicate":
				names = append(names, names[0])
			case "symlink":
				kind = tar.TypeSymlink
			case "hardlink":
				kind = tar.TypeLink
			case "oversized":
				size = 1 << 30
			}
			root := hostileLicenses(t, names, kind, size, scenario)
			var out bytes.Buffer
			if err := ReadLicenses(root, "licenses/tool/LICENSE", &out); err == nil {
				t.Fatal("unsafe archive accepted")
			}
			if out.Len() != 0 {
				t.Fatal("released bytes before verification")
			}
		})
	}
	dir, root := licenseFixture(t, false)
	_ = dir
	if err := WriteLicenseArchive(root); err != nil {
		t.Fatal(err)
	}
	if err := WriteManifest(root, "amd64", map[string]Spec{}); err != nil {
		t.Fatal(err)
	}
	for _, request := range []string{"../LICENSE", "LICENSE", "licenses/tool/../other/LICENSE", "/licenses/tool/LICENSE", "licenses/tool/missing"} {
		if err := ReadLicenses(root, request, io.Discard); err == nil {
			t.Fatal("invalid or ambiguous request accepted", request)
		}
	}
}

// Read a completed installer result; Extract-only fixtures still inspect stage files.
func readInstalledFile(t *testing.T, dir, name string) ([]byte, error) {
	t.Helper()
	if !strings.HasPrefix(name, "licenses/") {
		return os.ReadFile(filepath.Join(dir, name))
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	var output bytes.Buffer
	err = ReadLicenses(root, name, &output)
	return output.Bytes(), err
}

func TestBuildRefusesExistingNoticeContent(t *testing.T) {
	dir, root := licenseFixture(t, false)
	if err := Build(context.Background(), nil, map[string]Spec{}, map[string]toolconfig.Package{}, nil, "amd64", dir, io.Discard); err == nil {
		t.Fatal("builder repackaged preexisting user files")
	}
	if _, err := root.Lstat("licenses/tool/LICENSE"); err != nil {
		t.Fatal("builder changed protected notices", err)
	}
	if _, err := root.Lstat("licenses.tar.gz"); !os.IsNotExist(err) {
		t.Fatal("builder wrote into nonempty destination", err)
	}
}

func TestLicenseArchiveCompressedLimitPreservesStage(t *testing.T) {
	dir, root := licenseFixture(t, false)
	if err := writeLicenseArchive(root, 1); err == nil {
		t.Fatal("writer emitted an archive its reader would reject")
	}
	if _, err := os.Stat(filepath.Join(dir, "licenses/tool/LICENSE")); err != nil {
		t.Fatal("size failure removed staged full notice", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "licenses.index.json")); !os.IsNotExist(err) {
		t.Fatal("size failure committed an index", err)
	}
}
