package toolconfig

import (
	"bytes"
	"crypto/sha3"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func h(n string) string { return strings.Repeat(n, 64) }

func TestValidateSpecSHA3(t *testing.T) {
	ok := Spec{Version: "1.0.0", URL: "https://x/", SHA256: h("a"), SHA3256: h("b"), Verification: "publisher-sha3-256"}
	if err := ValidateSpec(ok); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]Spec{
		"no sha3":        {Version: "1", SHA256: h("a"), Verification: "publisher-sha3-256"},
		"non-hex sha3":   {Version: "1", SHA256: h("a"), SHA3256: strings.Repeat("z", 64), Verification: "publisher-sha3-256"},
		"sha3 w/o class": {Version: "1", SHA256: h("a"), SHA3256: h("b"), Verification: "publisher-sha256"},
		"short sha3":     {Version: "1", SHA256: h("a"), SHA3256: strings.Repeat("b", 62), Verification: "publisher-sha3-256"},
		"long sha3":      {Version: "1", SHA256: h("a"), SHA3256: strings.Repeat("b", 66), Verification: "publisher-sha3-256"},
		"sha512 + sha3":  {Version: "1", SHA512: strings.Repeat("a", 128), SHA3256: h("b"), Verification: "publisher-sha3-256"},
	} {
		if err := ValidateSpec(s); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The design pins SHA3-256 as a validated hash class; the standard library
// must provide it at the pinned toolchain (.go-version) and the go.mod minimum.
func TestSHA3256AvailableInStandardLibrary(t *testing.T) {
	sum := sha3.Sum256(nil)
	if got := hex.EncodeToString(sum[:]); got != "a7ffc6f8bf1ed76651c14756a061d662f580ff4de43b49fa82d80a4b80f8434a" {
		t.Fatalf("SHA3-256 of the empty input: %s", got)
	}
}

// Every production lock decodes and re-encodes with the repository serializer
// (the one bdock tools upgrade uses) to the same bytes: the optional
// source-build fields are omitted when empty, so existing locks are untouched.
func TestExistingLocksRoundTripByteIdentical(t *testing.T) {
	locks, err := filepath.Glob("../../bundles/*/packages.lock.json")
	// nixbox is a Nix store bundle with its own lock format (internal/nixbox).
	locks = withoutNixbox(locks)
	if err != nil || len(locks) != 9 {
		t.Fatalf("expected the nine bundle locks, got %v (%v)", locks, err)
	}
	for _, path := range locks {
		t.Run(filepath.Base(filepath.Dir(path)), func(t *testing.T) {
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var lock Lock
			if err := Decode(bytes.NewReader(want), &lock); err != nil {
				t.Fatal(err)
			}
			got, err := json.MarshalIndent(lock, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(append(got, '\n'), want) {
				t.Fatal("re-encoded lock differs from the committed bytes")
			}
		})
	}
}

// The catalog digest of every production catalog must also be unchanged: Load
// is strict and re-checks it, and the new Package fields are omitted when empty.
func TestExistingCatalogsKeepDigest(t *testing.T) {
	for _, bundle := range []string{"agy", "bun", "claude", "code-server", "codex", "node", "node-current", "opentofu", "toolbox"} {
		if _, err := Load("../../bundles/" + bundle); err != nil {
			t.Fatalf("%s: %v", bundle, err)
		}
	}
}
