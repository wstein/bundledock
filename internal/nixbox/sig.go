package nixbox

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// PathInfo is the part of `nix path-info --json --json-format 1` the lock keeps.
type PathInfo struct {
	NarHash    string   `json:"narHash"`
	NarSize    int64    `json:"narSize"`
	References []string `json:"references"`
	Signatures []string `json:"signatures"`
}

const nix32Alphabet = "0123456789abcdfghijklmnpqrsvwxyz"

// nix32 is Nix's base-32 text form of a hash.
func nix32(hash []byte) string {
	n := (len(hash)*8-1)/5 + 1
	out := make([]byte, 0, n)
	for i := n - 1; i >= 0; i-- {
		b := i * 5
		idx, shift := b/8, uint(b%8)
		c := hash[idx] >> shift
		if idx+1 < len(hash) {
			c |= hash[idx+1] << (8 - shift)
		}
		out = append(out, nix32Alphabet[c&0x1f])
	}
	return string(out)
}

// fingerprint is the string a binary cache signs for a store path.
func fingerprint(storePath string, info PathInfo) (string, error) {
	sri, ok := strings.CutPrefix(info.NarHash, "sha256-")
	if !ok {
		return "", fmt.Errorf("narHash %q is not an SRI sha256 hash", info.NarHash)
	}
	raw, err := base64.StdEncoding.DecodeString(sri)
	if err != nil || len(raw) != 32 {
		return "", fmt.Errorf("narHash %q is malformed", info.NarHash)
	}
	refs := append([]string(nil), info.References...)
	sort.Strings(refs)
	return "1;" + storePath + ";sha256:" + nix32(raw) + ";" + strconv.FormatInt(info.NarSize, 10) + ";" + strings.Join(refs, ","), nil
}

// VerifySignatures requires one signature by the named public key
// ("name:base64") over the path's fingerprint. Signatures by other keys are
// ignored; they never satisfy the check.
func VerifySignatures(storePath string, info PathInfo, publicKey string) error {
	name, keyText, ok := strings.Cut(publicKey, ":")
	if !ok {
		return fmt.Errorf("malformed public key")
	}
	key, err := base64.StdEncoding.DecodeString(keyText)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("malformed public key")
	}
	fp, err := fingerprint(storePath, info)
	if err != nil {
		return err
	}
	for _, sig := range info.Signatures {
		signer, text, ok := strings.Cut(sig, ":")
		if !ok || signer != name {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(text)
		if err != nil || len(raw) != ed25519.SignatureSize {
			continue
		}
		if ed25519.Verify(ed25519.PublicKey(key), []byte(fp), raw) {
			return nil
		}
	}
	return fmt.Errorf("%s: no valid %s signature", storePath, name)
}
