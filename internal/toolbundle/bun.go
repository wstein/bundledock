package toolbundle

import (
	"archive/zip"
	"bundledock/internal/toolconfig"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

const BunProfile = "bun-runtime"

func ValidateBunSpec(s Spec, arch string) error {
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("invalid Bun architecture")
	}
	if err := toolconfig.ValidateSpec(s); err != nil {
		return err
	}
	v := strings.TrimPrefix(s.Version, "bun-v")
	if _, err := toolconfig.ParseVersion(v); err != nil {
		return err
	}
	a := "x64-baseline"
	if arch == "arm64" {
		a = "aarch64"
	}
	if s.Version != "bun-v"+v || s.URL != "https://github.com/oven-sh/bun/releases/download/"+s.Version+"/bun-linux-"+a+".zip" || s.Verification != "publisher-sha256" || s.SHA256 == "" {
		return fmt.Errorf("invalid official Bun provenance")
	}
	return nil
}
func bunLinkTarget(root *os.Root, name, target string) error {
	if name != "payload/bunx" || target != "bun" {
		return fmt.Errorf("unsupported Bun link %q", name)
	}
	f, err := OpenFile(root, "payload/bun")
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Mode()&0111 == 0 {
		return fmt.Errorf("Bun target is not executable")
	}
	return nil
}
func writeBunLaunchers(root *os.Root) error {
	for _, name := range []string{"bun", "bunx"} {
		// bunx dispatch uses the executable basename, as in the upstream installer.
		script := "#!/bin/sh\nset -eu\nexec \"${0%/*}/../payload/" + name + "\" \"$@\"\n"
		if err := WriteFile(root, "bin/"+name, strings.NewReader(script), 0755); err != nil {
			return err
		}
	}
	return nil
}

// extractBunZIP preserves every regular payload file beneath the exact official
// platform root. No upstream link or special entry is accepted. The single
// generated bunx link supplies Bun's native basename dispatch.
func extractBunZIP(root *os.Root, source io.ReaderAt, size int64, expectedRoot string) error {
	if expectedRoot != "bun-linux-x64-baseline" && expectedRoot != "bun-linux-aarch64" {
		return fmt.Errorf("invalid Bun ZIP root")
	}
	z, err := zip.NewReader(source, size)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	var total uint64
	for _, e := range z.File {
		if err := ValidPath(e.Name); err != nil {
			return err
		}
		clean := path.Clean(e.Name)
		parts := strings.SplitN(clean, "/", 2)
		if parts[0] != expectedRoot || seen[clean] || len(seen) >= 32768 {
			return fmt.Errorf("invalid or duplicate Bun ZIP path %q", e.Name)
		}
		seen[clean] = true
		if len(parts) == 1 {
			if !e.Mode().IsDir() {
				return fmt.Errorf("Bun ZIP root must be directory")
			}
			continue
		}
		name := "payload/" + parts[1]
		if name == "payload/bunx" || strings.HasPrefix(name, "payload/bunx/") {
			return fmt.Errorf("reserved Bun link path")
		}
		if e.Mode().IsDir() {
			if err := mkdir(root, name); err != nil {
				return err
			}
			continue
		}
		if !e.Mode().IsRegular() || e.UncompressedSize64 > 512<<20 {
			return fmt.Errorf("unsupported Bun ZIP member")
		}
		total += e.UncompressedSize64
		if total > 2<<30 {
			return fmt.Errorf("Bun ZIP payload too large")
		}
		mode := os.FileMode(0644)
		if e.Mode()&0111 != 0 {
			mode = 0755
		}
		f, err := e.Open()
		if err != nil {
			return err
		}
		writeErr := WriteFile(root, name, io.LimitReader(f, 512<<20+1), mode)
		closeErr := f.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if err := bunLinkTarget(root, "payload/bunx", "bun"); err != nil {
		return err
	}
	if err := root.Symlink("bun", "payload/bunx"); err != nil {
		return err
	}
	return writeBunLaunchers(root)
}

func WriteBunManifest(root *os.Root, arch string, tools map[string]Spec) error {
	return writePayloadManifest(root, arch, tools, BunProfile)
}
