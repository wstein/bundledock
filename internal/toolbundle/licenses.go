package toolbundle

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

const (
	licenseArchive  = "licenses.tar.gz"
	licenseIndex    = "licenses.index.json"
	maxLicenseFile  = 16 << 20
	maxLicenseTotal = 256 << 20
	maxLicenseFiles = 100000
	maxLicenseIndex = 32 << 20
)

// LicenseFile records a complete retained notice, including nested dependencies.
type LicenseFile struct {
	Package string `json:"package"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
}

// LicenseIndex describes retained bytes, not the legal terms of any package.
// The archive hash binds the compressed bytes; SHA256SUMS covers this index.
type LicenseIndex struct {
	Schema  int           `json:"schema"`
	Archive string        `json:"archive"`
	SHA256  string        `json:"sha256"`
	Files   []LicenseFile `json:"files"`
}

func licensePackage(name string) (string, error) {
	parts := strings.Split(name, "/")
	if !fs.ValidPath(name) || len(parts) < 3 || parts[0] != "licenses" || strings.ContainsAny(name, "\\\r\n\x00") {
		return "", fmt.Errorf("invalid license member %q", name)
	}
	return parts[1], nil
}
func validLicenseHash(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == sha256.Size && value == strings.ToLower(value)
}

// WriteLicenseArchive compacts only the builder's staged licenses tree. Failure
// preserves the stage for inspection; successful exclusive writes precede removal.
func WriteLicenseArchive(root *os.Root) error {
	return writeLicenseArchive(root, maxLicenseTotal)
}

func writeLicenseArchive(root *os.Root, compressedLimit int64) error {
	for _, name := range []string{licenseArchive, licenseIndex} {
		if err := CheckPath(root, name); err != nil {
			return err
		}
		if _, err := root.Lstat(name); !os.IsNotExist(err) {
			if err != nil {
				return err
			}
			return fmt.Errorf("license output already exists: %s", name)
		}
	}
	index := LicenseIndex{Schema: 1, Archive: licenseArchive, Files: []LicenseFile{}}
	var names []string
	var total int64
	if err := CheckPath(root, "licenses"); err != nil {
		return err
	}
	_, statErr := root.Lstat("licenses")
	if statErr != nil && !os.IsNotExist(statErr) {
		return statErr
	}
	if statErr == nil {
		if err := fs.WalkDir(root.FS(), "licenses", func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := CheckPath(root, name); err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			if _, err := licensePackage(name); err != nil {
				return err
			}
			f, err := OpenFile(root, name)
			if err != nil {
				return err
			}
			info, err := f.Stat()
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			if err := checkLicenseLinks(info); err != nil {
				return err
			}
			total += info.Size()
			if info.Size() > maxLicenseFile || total > maxLicenseTotal || len(names) >= maxLicenseFiles {
				return fmt.Errorf("license stage exceeds size/count limits")
			}
			names = append(names, name)
			return nil
		}); err != nil {
			return err
		}
	}
	sort.Strings(names)
	archive, err := root.OpenFile(licenseArchive, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	h := sha256.New()
	gz := gzip.NewWriter(io.MultiWriter(archive, h))
	gz.ModTime = time.Unix(0, 0)
	tw := tar.NewWriter(gz)
	writeErr := func() error {
		for _, name := range names {
			f, err := OpenFile(root, name)
			if err != nil {
				return err
			}
			info, err := f.Stat()
			if err != nil {
				f.Close()
				return err
			}
			if err := checkLicenseLinks(info); err != nil {
				f.Close()
				return err
			}
			if info.Size() > maxLicenseFile {
				f.Close()
				return fmt.Errorf("oversized license")
			}
			if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0644, Size: info.Size(), ModTime: time.Unix(0, 0), Format: tar.FormatPAX}); err != nil {
				f.Close()
				return err
			}
			digest := sha256.New()
			n, err := io.Copy(io.MultiWriter(tw, digest), io.LimitReader(f, maxLicenseFile+1))
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			if n != info.Size() {
				return fmt.Errorf("license changed during packaging")
			}
			pkg, _ := licensePackage(name)
			index.Files = append(index.Files, LicenseFile{Package: pkg, Path: name, Size: n, SHA256: fmt.Sprintf("%x", digest.Sum(nil))})
		}
		return nil
	}()
	tarErr := tw.Close()
	gzipErr := gz.Close()
	modeErr := archive.Chmod(0644)
	closeErr := archive.Close()
	for _, err := range []error{writeErr, tarErr, gzipErr, modeErr, closeErr} {
		if err != nil {
			return err
		}
	}
	info, err := root.Stat(licenseArchive)
	if err != nil {
		return err
	}
	if info.Size() > compressedLimit {
		return fmt.Errorf("license archive exceeds compressed size limit")
	}
	index.SHA256 = fmt.Sprintf("%x", h.Sum(nil))
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxLicenseIndex {
		return fmt.Errorf("license index exceeds size limit")
	}
	if err := WriteFile(root, licenseIndex, bytes.NewReader(data), 0644); err != nil {
		return err
	}
	if statErr == nil {
		return root.RemoveAll("licenses")
	}
	return nil
}

func readLicenseFile(root *os.Root, name string, limit int64) ([]byte, error) {
	f, err := OpenFile(root, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("oversized %s", name)
	}
	return b, nil
}

func verifyLicenseSums(root *os.Root, index, archive []byte) error {
	data, err := readLicenseFile(root, "SHA256SUMS", 64<<20)
	if err != nil {
		return err
	}
	wanted := map[string]string{licenseIndex: fmt.Sprintf("%x", sha256.Sum256(index)), licenseArchive: fmt.Sprintf("%x", sha256.Sum256(archive))}
	seen := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 67 || line[64:66] != "  " {
			return fmt.Errorf("invalid SHA256SUMS line")
		}
		name := line[66:]
		if expected, ok := wanted[name]; ok {
			if seen[name] || line[:64] != expected {
				return fmt.Errorf("license checksum mismatch: %s", name)
			}
			seen[name] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(seen) != len(wanted) {
		return fmt.Errorf("SHA256SUMS must include license archive and index")
	}
	return nil
}

// ReadLicenses lists the plain index or writes one canonical member's complete
// bytes. It never extracts or initializes a volume and requires no external tools.
// No output is released until hashes, membership and bounded expansion verify.
func ReadLicenses(root *os.Root, requested string, output io.Writer) error {
	if requested != "" {
		if _, err := licensePackage(requested); err != nil {
			return err
		}
	}
	data, err := readLicenseFile(root, licenseIndex, maxLicenseIndex)
	if err != nil {
		return err
	}
	var index LicenseIndex
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&index); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing license index data")
	}
	if index.Schema != 1 || index.Archive != licenseArchive || !validLicenseHash(index.SHA256) || len(index.Files) > maxLicenseFiles || index.Files == nil {
		return fmt.Errorf("invalid license index")
	}
	var total int64
	previous := ""
	for _, file := range index.Files {
		pkg, err := licensePackage(file.Path)
		if err != nil || file.Package != pkg || file.Path <= previous || file.Size < 0 || file.Size > maxLicenseFile || !validLicenseHash(file.SHA256) {
			return fmt.Errorf("invalid license index member %q", file.Path)
		}
		previous = file.Path
		total += file.Size
		if total > maxLicenseTotal {
			return fmt.Errorf("license index exceeds expanded size limit")
		}
	}
	compressed, err := readLicenseFile(root, licenseArchive, maxLicenseTotal)
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(compressed)) != index.SHA256 {
		return fmt.Errorf("license archive hash mismatch")
	}
	if err := verifyLicenseSums(root, data, compressed); err != nil {
		return err
	}
	input := bytes.NewReader(compressed)
	gz, err := gzip.NewReader(input)
	if err != nil {
		return err
	}
	defer gz.Close()
	gz.Multistream(false)
	// Bound all tar bytes too, including unused padding and extended headers.
	expanded := &io.LimitedReader{R: gz, N: maxLicenseTotal*2 + 1}
	tr := tar.NewReader(expanded)
	var selected bytes.Buffer
	found := false
	for _, file := range index.Files {
		header, err := tr.Next()
		if err != nil {
			return fmt.Errorf("missing license member: %w", err)
		}
		if header.Typeflag != tar.TypeReg || header.Name != file.Path || header.Size != file.Size || header.Linkname != "" {
			return fmt.Errorf("license archive member mismatch: %s", file.Path)
		}
		h := sha256.New()
		var sink io.Writer = h
		if requested == file.Path {
			sink = io.MultiWriter(h, &selected)
			found = true
		}
		n, err := io.Copy(sink, tr)
		if err != nil {
			return err
		}
		if n != file.Size || fmt.Sprintf("%x", h.Sum(nil)) != file.SHA256 {
			return fmt.Errorf("license member hash/size mismatch: %s", file.Path)
		}
	}
	if _, err := tr.Next(); err != io.EOF {
		return fmt.Errorf("unexpected license archive member or invalid tar ending")
	}
	// Consume the gzip trailer to verify CRC, rejecting hidden extra tar bytes.
	tail, err := io.ReadAll(io.LimitReader(expanded, 1025))
	if err != nil {
		return err
	}
	if len(tail) > 1024 || len(bytes.Trim(tail, "\x00")) != 0 || expanded.N <= 0 || input.Len() != 0 {
		return fmt.Errorf("unexpected trailing or oversized license data")
	}
	if requested == "" {
		_, err = output.Write(data)
		return err
	}
	if !found {
		return fmt.Errorf("license member not found: %s", path.Clean(requested))
	}
	_, err = output.Write(selected.Bytes())
	return err
}
