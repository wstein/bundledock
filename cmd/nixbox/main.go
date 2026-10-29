// nixbox verifies, assembles and initializes the nixbox bundle. It never fetches
// anything: store paths come from `nix copy`, and this tool only checks them.
package main

import (
	"bundledock/internal/nixbox"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

const usage = "usage: nixbox check|lock|roots|profile|init|verify-store ... (see docs/nixbox.md)"

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "check":
		err = check(args[1:], stdout)
	case "lock":
		err = lock(args[1:], stdout)
	case "verify-store":
		err = verifyStore(args[1:], stdout)
	case "init":
		err = initVolume(args[1:], stdout)
	case "profile":
		err = profile(args[1:], stdout)
	case "roots":
		err = roots(args[1:], stdout)
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if err == nil {
		return 0
	}
	if _, ok := err.(usageError); ok {
		fmt.Fprintln(stderr, err)
		return 2
	}
	fmt.Fprintln(stderr, "nixbox:", err)
	return 1
}

type usageError string

func (e usageError) Error() string { return string(e) }

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func load(catalogPath, lockPath string) (*nixbox.Catalog, []byte, *nixbox.Lock, error) {
	cdata, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil, nil, nil, err
	}
	c, err := nixbox.ParseCatalog(cdata)
	if err != nil {
		return nil, nil, nil, err
	}
	ldata, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, nil, nil, err
	}
	l, err := nixbox.ParseLock(ldata)
	if err != nil {
		return nil, nil, nil, err
	}
	return c, cdata, l, nil
}

// check verifies catalog and lock offline, including every cache signature.
func check(args []string, stdout io.Writer) error {
	fs := newFlags("check")
	catalog := fs.String("catalog", "", "catalog file")
	lockPath := fs.String("lock", "", "lock file")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *catalog == "" || *lockPath == "" {
		return usageError("usage: nixbox check -catalog FILE -lock FILE")
	}
	c, cdata, l, err := load(*catalog, *lockPath)
	if err != nil {
		return err
	}
	if err := nixbox.CheckLock(c, cdata, l, c.Cache.PublicKey); err != nil {
		return err
	}
	for _, arch := range []string{"amd64", "arm64"} {
		p := l.Platforms[arch]
		fmt.Fprintf(stdout, "%s: %d store paths, %.1f MiB, all signed by %s\n", arch, len(p.Paths), float64(p.ClosureSize())/(1<<20), c.Cache.PublicKey[:len("cache.nixos.org-1")])
	}
	return nil
}

// lock assembles a lock from nix eval and nix path-info output (see
// bundles/nixbox/scripts/refresh-lock.sh).
func lock(args []string, stdout io.Writer) error {
	fs := newFlags("lock")
	catalog := fs.String("catalog", "", "catalog file")
	dir := fs.String("input", "", "directory with roots-<arch>.json and pathinfo-<arch>.json")
	out := fs.String("output", "", "lock file to write")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *catalog == "" || *dir == "" || *out == "" {
		return usageError("usage: nixbox lock -catalog FILE -input DIR -output FILE")
	}
	cdata, err := os.ReadFile(*catalog)
	if err != nil {
		return err
	}
	c, err := nixbox.ParseCatalog(cdata)
	if err != nil {
		return err
	}
	roots, infos, err := nixbox.ReadLockInputs(*dir, c)
	if err != nil {
		return err
	}
	l, err := nixbox.Assemble(c, cdata, roots, infos, c.Cache.PublicKey)
	if err != nil {
		return err
	}
	data, err := nixbox.EncodeLock(l)
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s\n", *out)
	return nil
}

// verifyStore checks a store directory against the lock for an architecture.
func verifyStore(args []string, stdout io.Writer) error {
	fs := newFlags("verify-store")
	catalog := fs.String("catalog", "", "catalog file")
	lockPath := fs.String("lock", "", "lock file")
	arch := fs.String("arch", runtime.GOARCH, "amd64 or arm64")
	store := fs.String("store", "/nix/store", "store directory")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *catalog == "" || *lockPath == "" || (*arch != "amd64" && *arch != "arm64") {
		return usageError("usage: nixbox verify-store -catalog FILE -lock FILE [-arch amd64|arm64] [-store DIR]")
	}
	c, cdata, l, err := load(*catalog, *lockPath)
	if err != nil {
		return err
	}
	if err := nixbox.CheckLock(c, cdata, l, c.Cache.PublicKey); err != nil {
		return err
	}
	p := l.Platforms[*arch]
	if err := nixbox.VerifyStore(p, *store); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s: %d store paths verified by NAR hash\n", *arch, len(p.Paths))
	return nil
}

// roots prints the store paths `nix copy` must fetch, one per line, sorted.
func roots(args []string, stdout io.Writer) error {
	fs := newFlags("roots")
	catalog := fs.String("catalog", "", "catalog file")
	lockPath := fs.String("lock", "", "lock file")
	arch := fs.String("arch", runtime.GOARCH, "amd64 or arm64")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *catalog == "" || *lockPath == "" || (*arch != "amd64" && *arch != "arm64") {
		return usageError("usage: nixbox roots -catalog FILE -lock FILE [-arch amd64|arm64]")
	}
	c, cdata, l, err := load(*catalog, *lockPath)
	if err != nil {
		return err
	}
	if err := nixbox.CheckLock(c, cdata, l, c.Cache.PublicKey); err != nil {
		return err
	}
	var paths []string
	for _, path := range l.Platforms[*arch].Roots {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		fmt.Fprintln(stdout, path)
	}
	return nil
}

// profile writes the Nix profile (symlinks, loader links, shim and table) for
// an architecture. The store must already verify against the lock.
func profile(args []string, stdout io.Writer) error {
	fs := newFlags("profile")
	catalog := fs.String("catalog", "", "catalog file")
	lockPath := fs.String("lock", "", "lock file")
	arch := fs.String("arch", runtime.GOARCH, "amd64 or arm64")
	store := fs.String("store", "", "physical store directory")
	out := fs.String("out", "", "profile directory to create")
	shimPath := fs.String("shim", "", "shim script to install")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || *catalog == "" || *lockPath == "" || *store == "" || *out == "" || *shimPath == "" || (*arch != "amd64" && *arch != "arm64") {
		return usageError("usage: nixbox profile -catalog FILE -lock FILE -store DIR -out DIR -shim FILE [-arch amd64|arm64]")
	}
	c, cdata, l, err := load(*catalog, *lockPath)
	if err != nil {
		return err
	}
	if err := nixbox.CheckLock(c, cdata, l, c.Cache.PublicKey); err != nil {
		return err
	}
	ldata, err := os.ReadFile(*lockPath)
	if err != nil {
		return err
	}
	shim, err := os.ReadFile(*shimPath)
	if err != nil {
		return err
	}
	p := l.Platforms[*arch]
	if err := nixbox.VerifyStore(p, *store); err != nil {
		return err
	}
	if err := nixbox.BuildProfile(c, cdata, ldata, p, *arch, *store, *out, shim); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "profile written to %s\n", *out)
	return nil
}

// initVolume is the carrier entrypoint: it fills the volume mounted at -target
// from the image tree at -source, using the catalog and lock the image carries.
func initVolume(args []string, stdout io.Writer) error {
	fs := newFlags("init")
	source := fs.String("source", "/opt/bdock/nixbox", "image tree (store, var/nix, profile)")
	target := fs.String("target", "/bundle", "mounted volume directory")
	arch := fs.String("arch", runtime.GOARCH, "amd64 or arm64")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || (*arch != "amd64" && *arch != "arm64") {
		return usageError("usage: nixbox init [-source DIR] [-target DIR] [-arch amd64|arm64]")
	}
	meta := filepath.Join(*source, "profile", "share", "nixbox")
	c, cdata, l, err := load(filepath.Join(meta, "catalog.json"), filepath.Join(meta, "lock.json"))
	if err != nil {
		return err
	}
	if err := nixbox.CheckLock(c, cdata, l, c.Cache.PublicKey); err != nil {
		return err
	}
	return nixbox.Initialize(*source, *target, l.Platforms[*arch], stdout)
}
