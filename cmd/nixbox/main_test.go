package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return out.String(), errb.String(), code
}

func TestCheckVerifiesTheProductionPair(t *testing.T) {
	out, errs, code := runCLI(t, "check", "-catalog", "../../bundles/nixbox/packages.catalog.json", "-lock", "../../bundles/nixbox/packages.lock.json")
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errs)
	}
	for _, want := range []string{"amd64", "arm64", "MiB"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q: %s", want, out)
		}
	}
}

func TestCheckFailsOnAStaleLock(t *testing.T) {
	dir := t.TempDir()
	catalog, _ := os.ReadFile("../../bundles/nixbox/packages.catalog.json")
	os.WriteFile(filepath.Join(dir, "c.json"), catalog, 0o644)
	os.WriteFile(filepath.Join(dir, "l.json"), []byte(`{"schema":1,"catalog_digest":"00","platforms":{}}`), 0o644)
	_, errs, code := runCLI(t, "check", "-catalog", filepath.Join(dir, "c.json"), "-lock", filepath.Join(dir, "l.json"))
	if code == 0 || !strings.Contains(errs, "digest") {
		t.Fatalf("exit %d: %s", code, errs)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{{}, {"bogus"}, {"check"}, {"check", "-catalog", "x"}, {"verify-store", "-arch", "ppc"}, {"roots", "-arch", "ppc"}, {"profile"}, {"init", "-arch", "ppc"}, {"init", "extra"}, {"profile", "-arch", "ppc"}} {
		if _, _, code := runCLI(t, args...); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
}

func TestRootsListsOneStorePathPerNixPackage(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		out, errs, code := runCLI(t, "roots", "-arch", arch, "-catalog", "../../bundles/nixbox/packages.catalog.json", "-lock", "../../bundles/nixbox/packages.lock.json")
		if code != 0 {
			t.Fatalf("%s: exit %d: %s", arch, code, errs)
		}
		lines := strings.Fields(out)
		if len(lines) != 8 {
			t.Errorf("%s: %d roots, want 8: %v", arch, len(lines), lines)
		}
		for _, line := range lines {
			if !strings.HasPrefix(line, "/nix/store/") {
				t.Errorf("not a store path: %q", line)
			}
		}
	}
}

func TestProfileRefusesAStoreThatDoesNotMatchTheLock(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(t.TempDir(), "shim")
	os.WriteFile(shim, []byte("#!/bin/sh\n"), 0o755)
	_, errs, code := runCLI(t, "profile", "-catalog", "../../bundles/nixbox/packages.catalog.json", "-lock", "../../bundles/nixbox/packages.lock.json",
		"-store", dir, "-out", filepath.Join(dir, "out"), "-shim", shim, "-arch", "arm64")
	if code != 1 || !strings.Contains(errs, "lacks") {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if _, err := os.Stat(filepath.Join(dir, "out")); err == nil {
		t.Error("wrote a profile for an unverified store")
	}
}

func TestInitNeedsTheImageCatalogAndLock(t *testing.T) {
	_, errs, code := runCLI(t, "init", "-source", t.TempDir(), "-target", t.TempDir())
	if code != 1 || errs == "" {
		t.Fatalf("exit %d: %s", code, errs)
	}
}
