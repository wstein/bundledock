package nixbox

import (
	"crypto/ed25519"
	"encoding/base64"
	"strings"
	"testing"
)

// A real cache.nixos.org record (path-info of zstd-1.5.7-bin, nixpkgs e62746b).
var realInfo = PathInfo{
	NarHash: "sha256-yOXkKK/xzGw2X2QPhEgJcVlERPLZeHOIz6X6nti1h2A=",
	NarSize: 387976,
	References: []string{
		"/nix/store/3advb2s77lmajxkks58va12s2bfafhwy-zstd-1.5.7-bin",
		"/nix/store/74nxb0rnf6lm102ax6ax9b4cpyvrcifq-gnugrep-3.12",
		"/nix/store/91qsbvx8w31rd255w8ikficjqg4v3fh4-zstd-1.5.7",
		"/nix/store/czx7cscpnqqmc9nwmbpnv2zcvck64mmf-bash-5.3p20",
		"/nix/store/nhphqkv18x8fpf3flz0bsm1h554pawn7-glibc-2.44-57",
		"/nix/store/nhrkwapk0zzzmc8x1f2k24qpyz9gy5qb-gcc-16.2.0-lib",
	},
	Signatures: []string{"cache.nixos.org-1:dMreVu8talAfTx93H2uAwRPy8weKTqJknUBRzW6U9bYGF7ddX8U4nwkUXX37vrioQxuG1zZEtTPsgPk+8cSbCw=="},
}

const realPath = "/nix/store/3advb2s77lmajxkks58va12s2bfafhwy-zstd-1.5.7-bin"

func TestNix32KnownVector(t *testing.T) {
	// nix hash convert --to nix32 of the empty-input sha256.
	raw, _ := base64.StdEncoding.DecodeString("47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=")
	if got := nix32(raw); got != "0mdqa9w1p6cmli6976v4wi0sw9r4p5prkj7lzfd1877wk11c9c73" {
		t.Fatalf("nix32 = %s", got)
	}
}

func TestVerifySignatureAcceptsTheRealCacheRecord(t *testing.T) {
	if err := VerifySignatures(realPath, realInfo, CachePublicKey); err != nil {
		t.Fatal(err)
	}
}

func TestVerifySignatureRejectsAnyChange(t *testing.T) {
	mutations := map[string]func(PathInfo) (string, PathInfo){
		"path": func(i PathInfo) (string, PathInfo) { return strings.Replace(realPath, "zstd", "zstf", 1), i },
		"size": func(i PathInfo) (string, PathInfo) { i.NarSize++; return realPath, i },
		"hash": func(i PathInfo) (string, PathInfo) {
			i.NarHash = "sha256-yOXkKK/xzGw2X2QPhEgJcVlERPLZeHOIz6X6nti1h3A="
			return realPath, i
		},
		"reference": func(i PathInfo) (string, PathInfo) { i.References = i.References[:5]; return realPath, i },
		"no sig":    func(i PathInfo) (string, PathInfo) { i.Signatures = nil; return realPath, i },
		"other name": func(i PathInfo) (string, PathInfo) {
			i.Signatures = []string{"evil-1:" + strings.SplitN(i.Signatures[0], ":", 2)[1]}
			return realPath, i
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			p, info := mutate(realInfo)
			if err := VerifySignatures(p, info, CachePublicKey); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	t.Run("wrong key", func(t *testing.T) {
		pub, _, _ := ed25519.GenerateKey(nil)
		key := "cache.nixos.org-1:" + base64.StdEncoding.EncodeToString(pub)
		if err := VerifySignatures(realPath, realInfo, key); err == nil {
			t.Fatal("accepted a signature under another key")
		}
	})
}

func TestVerifySignatureFindsTheMatchingOneAmongSeveral(t *testing.T) {
	info := realInfo
	info.Signatures = []string{"other-1:AAAA", "cache.nixos.org-1:" + base64.StdEncoding.EncodeToString(make([]byte, 64)), realInfo.Signatures[0]}
	if err := VerifySignatures(realPath, info, CachePublicKey); err != nil {
		t.Fatal(err)
	}
}
