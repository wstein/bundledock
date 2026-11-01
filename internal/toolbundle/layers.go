package toolbundle

import (
	"bundledock/internal/toolconfig"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// MetaLayer is the implicit topmost layer: everything the plan does not claim
// (manifest, checksums, license archive, shell layer, initializer, links).
const MetaLayer = "meta"

var layerName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// LayerPlan assigns every catalog tool to one image layer, bottom layer first.
// Rarely changing tools go low and frequently changing tools high, so that a
// tool update rewrites only the layers above it.
type LayerPlan struct {
	Schema int     `json:"schema"`
	Layers []Layer `json:"layers"`
}

type Layer struct {
	Name  string   `json:"name"`
	Tools []string `json:"tools"`
}

// ParseLayerPlan reads and validates a plan. Unknown fields fail.
func ParseLayerPlan(r io.Reader) (*LayerPlan, error) {
	data, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var p LayerPlan
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("layer plan: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("layer plan: trailing data")
	}
	if p.Schema != 1 || len(p.Layers) == 0 {
		return nil, fmt.Errorf("layer plan: need schema 1 and at least one layer")
	}
	names, tools := map[string]bool{MetaLayer: true}, map[string]string{}
	for _, l := range p.Layers {
		if !layerName.MatchString(l.Name) || names[l.Name] {
			return nil, fmt.Errorf("layer plan: invalid, duplicate or reserved layer name %q", l.Name)
		}
		names[l.Name] = true
		if len(l.Tools) == 0 {
			return nil, fmt.Errorf("layer plan: layer %q has no tools", l.Name)
		}
		for _, t := range l.Tools {
			if prev, ok := tools[t]; ok {
				return nil, fmt.Errorf("layer plan: tool %q is in layers %q and %q", t, prev, l.Name)
			}
			tools[t] = l.Name
		}
	}
	return &p, nil
}

// CheckCovers requires the plan to name exactly the catalog's tools, so a new
// tool cannot land in a layer by accident.
func (p *LayerPlan) CheckCovers(packages map[string]toolconfig.Package) error {
	planned := map[string]bool{}
	for _, l := range p.Layers {
		for _, t := range l.Tools {
			planned[t] = true
			if _, ok := packages[t]; !ok {
				return fmt.Errorf("layer plan names %q, which is not in the catalog", t)
			}
		}
	}
	var missing []string
	for name := range packages {
		if !planned[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("layer plan lacks catalog tools: %s", strings.Join(missing, ", "))
	}
	return nil
}

// OwnedPaths lists the bundle-relative files a tool installs: its members and
// aliases. Tools that preserve an archive tree or use a profile are rejected,
// because their files cannot be named from the catalog.
func OwnedPaths(p toolconfig.Package) ([]string, error) {
	if p.Install.Preserve || p.Install.Profile != "" {
		return nil, fmt.Errorf("layers support only member and alias installs")
	}
	var out []string
	for _, target := range p.Install.Members {
		out = append(out, target)
	}
	for alias := range p.Install.Aliases {
		out = append(out, alias)
	}
	for _, f := range out {
		if c := path.Clean(f); c != f || path.IsAbs(f) || f == "." || strings.HasPrefix(f, "../") {
			return nil, fmt.Errorf("unsafe install path %q", f)
		}
	}
	sort.Strings(out)
	return out, nil
}

// SplitLayers moves the files of each planned tool from root/bundle into
// out/NN-<layer>/bundle and then the whole remainder of root into the last
// directory out/NN-meta. It returns the directory names, bottom first. Content
// and directory modes are preserved; the caller normalizes mtimes afterward.
func SplitLayers(root, bundle, out string, plan *LayerPlan, packages map[string]toolconfig.Package) ([]string, error) {
	if err := plan.CheckCovers(packages); err != nil {
		return nil, err
	}
	if _, err := os.Lstat(out); err == nil {
		return nil, fmt.Errorf("layer output %s already exists", out)
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, err
	}
	var names []string
	for i, l := range plan.Layers {
		name := fmt.Sprintf("%02d-%s", i+1, l.Name)
		names = append(names, name)
		if err := os.Mkdir(filepath.Join(out, name), 0o755); err != nil {
			return nil, err
		}
		for _, tool := range l.Tools {
			owned, err := OwnedPaths(packages[tool])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", tool, err)
			}
			for _, rel := range owned {
				src := filepath.Join(root, bundle, filepath.FromSlash(rel))
				if _, err := os.Lstat(src); err != nil {
					return nil, fmt.Errorf("%s: %w", tool, err)
				}
				dst := filepath.Join(out, name, bundle, filepath.FromSlash(rel))
				if err := mkdirAllLike(root, filepath.Join(out, name), filepath.Dir(src)); err != nil {
					return nil, err
				}
				if err := moveFile(src, dst); err != nil {
					return nil, err
				}
			}
		}
	}
	meta := fmt.Sprintf("%02d-%s", len(plan.Layers)+1, MetaLayer)
	// The remainder is copied, not renamed: renaming a directory that comes from
	// a lower image layer fails with EXDEV on overlayfs.
	if err := copyTree(root, filepath.Join(out, meta)); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(root); err != nil {
		return nil, err
	}
	return append(names, meta), nil
}

// moveFile renames a file or symlink and falls back to copy and remove.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyEntry(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

// copyTree copies src to dst with content, modes and symlinks; mtimes are
// normalized afterward by the caller.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		return copyEntry(p, filepath.Join(dst, rel))
	})
}

func copyEntry(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case info.IsDir():
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		return os.Chmod(dst, info.Mode().Perm())
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case info.Mode().IsRegular():
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return os.Chmod(dst, info.Mode().Perm())
	}
	return fmt.Errorf("unsupported file type %s", src)
}

// mkdirAllLike creates dir (below srcRoot) under dstRoot with the same mode as
// each source directory.
func mkdirAllLike(srcRoot, dstRoot, dir string) error {
	rel, err := filepath.Rel(srcRoot, dir)
	if err != nil {
		return err
	}
	cur, dst := srcRoot, dstRoot
	if rel == "." {
		return nil
	}
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur, dst = filepath.Join(cur, part), filepath.Join(dst, part)
		if _, err := os.Lstat(dst); err == nil {
			continue
		}
		info, err := os.Lstat(cur)
		if err != nil {
			return err
		}
		if err := os.Mkdir(dst, info.Mode().Perm()); err != nil {
			return err
		}
		if err := os.Chmod(dst, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}
