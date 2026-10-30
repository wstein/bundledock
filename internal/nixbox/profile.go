package nixbox

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// LogicalStore is where the store appears in a consumer: the volume mounts at /nix.
const LogicalStore = "/nix/store"

var loaderNames = map[string]string{"amd64": "ld-linux-x86-64.so.2", "arm64": "ld-linux-aarch64.so.1"}

// OfficialDirectory is where the official release binaries live in a profile: the
// image build installs the bundles/nixbox/tools bundle (the toolbox recipes) there.
const OfficialDirectory = "libexec/nixbox"

// shimTarget is the profile command for an official binary: a relative link, so
// the profile works wherever the volume is mounted.
const shimTarget = "../libexec/nixbox-shim"

// BuildProfile writes a Nix-style profile into out. storeDir is the physical
// store (symlinks still name the logical /nix/store). The profile merges bin,
// share and etc of every non-runner nix package as symlinks, links the glibc
// loader and libraries of the runner packages, installs the shim and its table
// (generated from the catalog), and records the catalog and lock. The official
// binaries must already be installed below out/libexec/nixbox; a shim whose
// binary is missing or not executable fails the build.
func BuildProfile(c *Catalog, catalogData, lockData []byte, p PlatformLock, arch, storeDir, out string, shim []byte) error {
	loader, ok := loaderNames[arch]
	if !ok {
		return fmt.Errorf("unknown architecture %q", arch)
	}
	var packages []string
	for _, name := range c.NixNames() {
		if c.Packages[name].Role != RoleRunner {
			packages = append(packages, name)
		}
	}
	for _, sub := range []string{"bin", "share", "etc"} {
		for _, name := range packages {
			root, ok := p.Roots[name]
			if !ok {
				return fmt.Errorf("no root for %s", name)
			}
			phys := filepath.Join(storeDir, filepath.Base(root), sub)
			if _, err := os.Stat(phys); err != nil {
				continue
			}
			if err := mergeTree(phys, path.Join(root, sub), filepath.Join(out, sub)); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	glibc, gcc := p.Roots["glibc"], p.Roots["gcc-lib"]
	if glibc == "" || gcc == "" {
		return fmt.Errorf("lock has no glibc or gcc-lib root")
	}
	if _, err := os.Stat(filepath.Join(storeDir, filepath.Base(glibc), "lib", loader)); err != nil {
		return fmt.Errorf("glibc has no loader %s: %w", loader, err)
	}
	if err := os.MkdirAll(filepath.Join(out, "lib"), 0o755); err != nil {
		return err
	}
	for link, target := range map[string]string{
		"lib/ld.so": path.Join(glibc, "lib", loader),
		"lib/glibc": path.Join(glibc, "lib"),
		"lib/gcc":   path.Join(gcc, "lib"),
	} {
		if err := os.Symlink(target, filepath.Join(out, link)); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(out, "libexec"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "libexec", "nixbox-shim"), shim, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(out, "bin"), 0o755); err != nil {
		return err
	}
	var table strings.Builder
	for _, name := range c.OfficialNames() {
		link := filepath.Join(out, "bin", name)
		if _, err := os.Lstat(link); err == nil {
			return fmt.Errorf("shim %s would shadow a nix command", name)
		}
		if err := os.Symlink(shimTarget, link); err != nil {
			return err
		}
		pkg := c.Packages[name]
		info, err := os.Stat(filepath.Join(out, filepath.FromSlash(OfficialDirectory), filepath.FromSlash(pkg.Binary)))
		if err != nil {
			return fmt.Errorf("shim %s: %w", name, err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			return fmt.Errorf("shim %s: %s is not an executable file", name, pkg.Binary)
		}
		fmt.Fprintf(&table, "bin\t%s\t%s\n", name, pkg.Binary)
		var vars []string
		for v := range pkg.Env {
			vars = append(vars, v)
		}
		sort.Strings(vars)
		for _, v := range vars {
			fmt.Fprintf(&table, "env\t%s\t%s\t%s\n", name, v, pkg.Env[v])
		}
	}
	meta := filepath.Join(out, "share", "nixbox")
	if err := os.MkdirAll(meta, 0o755); err != nil {
		return err
	}
	for name, data := range map[string][]byte{"shims.tsv": []byte(table.String()), "catalog.json": catalogData, "lock.json": lockData} {
		if err := os.WriteFile(filepath.Join(meta, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// mergeTree links the files below phys into dst. logical is phys as the
// consumer sees it. Directories present in several packages are merged.
func mergeTree(phys, logical, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(phys)
	if err != nil {
		return err
	}
	for _, e := range entries {
		src := filepath.Join(phys, e.Name())
		target := path.Join(logical, e.Name())
		link := filepath.Join(dst, e.Name())
		info, err := os.Stat(src)
		if err != nil {
			// A dangling symlink inside a package stays a link to the same target.
			info = nil
		}
		if info != nil && info.IsDir() {
			if existing, err := os.Lstat(link); err == nil && !existing.IsDir() {
				return fmt.Errorf("%s: file and directory conflict", link)
			}
			if err := mergeTree(src, target, link); err != nil {
				return err
			}
			continue
		}
		if _, err := os.Lstat(link); err == nil {
			return fmt.Errorf("%s is provided by two packages", link)
		}
		if err := os.Symlink(target, link); err != nil {
			return err
		}
	}
	return nil
}
