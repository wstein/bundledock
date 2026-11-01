package toolpublish

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuildArgsPinSourceDateEpoch(t *testing.T) {
	for _, bundle := range []string{"toolbox", "nixbox"} {
		b, err := SelectBundle(bundle)
		if err != nil {
			t.Fatal(err)
		}
		got := buildArgs(b, Metadata{Tag: "2026.11.01-b1"}, "amd64")
		want := []string{"docker", "build", "--pull", "--platform", "linux/amd64",
			"-f", "bundles/" + bundle + "/Dockerfile", "-t", "bundledock-" + bundle + ":build-2026.11.01-b1-amd64",
			"--build-arg", "SOURCE_DATE_EPOCH=" + SourceDateEpoch}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: args %q, want %q", bundle, got, want)
		}
		if got[buildTagIndex] != want[buildTagIndex] {
			t.Errorf("%s: buildTagIndex %d does not point at the tag", bundle, buildTagIndex)
		}
	}
}

// The epoch must not depend on the version: a per-version time would give every
// version different layer digests, including for unchanged tools.
func TestSourceDateEpochIsConstant(t *testing.T) {
	b, _ := SelectBundle("toolbox")
	a := buildArgs(b, Metadata{Tag: "2026.11.01-b1"}, "amd64")
	c := buildArgs(b, Metadata{Tag: "2026.12.24-b3"}, "amd64")
	if a[10] != c[10] {
		t.Fatalf("epoch differs between versions: %q vs %q", a[10], c[10])
	}
}

// The Dockerfile default must equal the publisher's constant.
func TestDockerfilesDeclareSourceDateEpoch(t *testing.T) {
	for _, bundle := range []string{"toolbox", "nixbox"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "bundles", bundle, "Dockerfile"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "\nARG SOURCE_DATE_EPOCH="+SourceDateEpoch+"\n") {
			t.Errorf("%s: Dockerfile lacks ARG SOURCE_DATE_EPOCH=%s", bundle, SourceDateEpoch)
		}
	}
}

func TestRealDockerfilesNormalizeAfterTheLastWrite(t *testing.T) {
	for _, bundle := range []string{"toolbox", "nixbox"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "bundles", bundle, "Dockerfile"))
		if err != nil {
			t.Fatal(err)
		}
		if problems := checkNormalizedBeforeCopy(string(data)); len(problems) != 0 {
			t.Errorf("%s: %s", bundle, strings.Join(problems, "; "))
		}
	}
}

const goodDockerfile = `FROM alpine AS builder
RUN build -out /image-root
COPY --from=x /bin/init /init-root/usr/bin/init
RUN tools normalize-mtimes -root /image-root && tools normalize-mtimes -root /init-root

FROM scratch
COPY --from=builder /image-root/ /
COPY --from=builder /init-root/ /
`

func TestCheckNormalizedBeforeCopy(t *testing.T) {
	if p := checkNormalizedBeforeCopy(goodDockerfile); len(p) != 0 {
		t.Fatalf("good Dockerfile rejected: %v", p)
	}
	// A different root that merely starts with the normalized name is not a write to it.
	sibling := strings.Replace(goodDockerfile, "\n\nFROM scratch", "\nCOPY --from=x /y /image-root-extra/y\n\nFROM scratch", 1)
	if p := checkNormalizedBeforeCopy(sibling); len(p) != 0 {
		t.Errorf("write to /image-root-extra counted as a write to /image-root: %v", p)
	}
	// Instruction names are case-insensitive in a Dockerfile.
	lower := strings.NewReplacer("RUN ", "run ", "COPY ", "copy ", "FROM ", "from ", " AS ", " as ").Replace(goodDockerfile)
	if p := checkNormalizedBeforeCopy(lower); len(p) != 0 {
		t.Errorf("lowercase instructions rejected: %v", p)
	}
	// Joined continuation lines and a trailing statement separator are normal forms.
	joined := strings.Replace(goodDockerfile, "RUN tools normalize-mtimes -root /image-root && tools normalize-mtimes -root /init-root", "RUN set -eu; \\\n    tools normalize-mtimes -root /image-root; \\\n    tools normalize-mtimes -root /init-root", 1)
	if p := checkNormalizedBeforeCopy(joined); len(p) != 0 {
		t.Errorf("continuation form rejected: %v", p)
	}
	for name, mutate := range map[string]func(string) string{
		"write after normalize": func(s string) string {
			return strings.Replace(s, "\n\nFROM scratch", "\nCOPY --from=x /late /image-root/late\n\nFROM scratch", 1)
		},
		"normalize commented out": func(s string) string {
			return strings.Replace(s, "RUN tools normalize", "# RUN tools normalize", 1)
		},
		"normalize missing for one root": func(s string) string {
			return strings.Replace(s, " && tools normalize-mtimes -root /init-root", "", 1)
		},
		"write later in the same RUN": func(s string) string {
			return strings.Replace(s, "-root /init-root", "-root /init-root && touch /init-root/x", 1)
		},
		"normalize only echoed": func(s string) string {
			return strings.Replace(s, "RUN tools normalize-mtimes -root /image-root", "RUN echo tools normalize-mtimes -root /image-root", 1)
		},
		"normalize hidden in a trailing shell comment": func(string) string {
			return `FROM alpine AS builder
RUN build -out /image-root /init-root
RUN tools normalize-mtimes -root /image-root && true # ; tools normalize-mtimes -root /init-root

FROM scratch
COPY --from=builder /image-root/ /
COPY --from=builder /init-root/ /
`
		},
		"normalize failure ignored": func(s string) string {
			return strings.Replace(s, "-root /init-root", "-root /init-root || true", 1)
		},
		"sibling prefix is not the same root": func(s string) string {
			return strings.Replace(strings.Replace(s, "-root /image-root", "-root /image", 1), "COPY --from=builder /image-root/ /", "COPY --from=builder /image-root/ /", 1)
		},
		"lowercase copy of an unknown root": func(s string) string {
			return s + "copy --from=builder /other/ /\n"
		},
		"normalize runs before the split that writes the layers": func(string) string {
			return `FROM alpine AS builder
RUN tools normalize-mtimes -root /layers
RUN tools split-layers -root /image-root -out /layers

FROM scratch
COPY --from=builder /layers/01-a/ /
`
		},
		"final stage copies an unknown root": func(s string) string {
			return s + "COPY --from=builder /other/ /\n"
		},
	} {
		if p := checkNormalizedBeforeCopy(mutate(goodDockerfile)); len(p) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
}

const nixDBDockerfile = `FROM alpine AS fetch
RUN nix copy --to /image-root \
    && mv /image-root/nix /out/opt/nixbox \
    && tools normalize-nix-db -db /out/opt/nixbox/var/nix/db/db.sqlite \
    && tools normalize-mtimes -root /out

FROM scratch
COPY --from=fetch /out/ /
`

func TestCheckNixDBNormalized(t *testing.T) {
	if p := checkNixDBNormalized(nixDBDockerfile); len(p) != 0 {
		t.Fatalf("good Dockerfile rejected: %v", p)
	}
	// Continuation lines with ';' and lowercase instruction names are normal forms.
	semi := strings.ReplaceAll(nixDBDockerfile, "\\\n    && ", "; \\\n    ")
	if p := checkNixDBNormalized(semi); len(p) != 0 {
		t.Errorf("';' form rejected: %v", p)
	}
	for name, mutate := range map[string]func(string) string{
		"no nix database step": func(s string) string {
			return strings.Replace(s, "    && tools normalize-nix-db -db /out/opt/nixbox/var/nix/db/db.sqlite \\\n", "", 1)
		},
		"step commented out": func(s string) string {
			return strings.Replace(s, "    && tools normalize-nix-db", "    # && tools normalize-nix-db", 1)
		},
		"step only echoed": func(s string) string {
			return strings.Replace(s, "&& tools normalize-nix-db", "&& echo tools normalize-nix-db", 1)
		},
		"failure ignored": func(s string) string {
			return strings.Replace(s, "db.sqlite \\", "db.sqlite || true \\", 1)
		},
		"database written after the step": func(s string) string {
			return strings.Replace(s, "    && tools normalize-mtimes", "    && nix-store --load-db < /dump && echo x >> /out/opt/nixbox/var/nix/db/db.sqlite \\\n    && tools normalize-mtimes", 1)
		},
		"step after the mtime step": func(s string) string {
			return strings.Replace(strings.Replace(s, "    && tools normalize-nix-db -db /out/opt/nixbox/var/nix/db/db.sqlite \\\n", "", 1), "-root /out\n", "-root /out \\\n    && tools normalize-nix-db -db /out/opt/nixbox/var/nix/db/db.sqlite\n", 1)
		},
		"other file than the Nix database": func(s string) string {
			return strings.Replace(s, "var/nix/db/db.sqlite", "var/nix/db/schema", 1)
		},
		"database outside the copied root": func(s string) string {
			return strings.Replace(s, "-db /out/opt", "-db /elsewhere/opt", 1)
		},
		"no mtime step": func(s string) string {
			return strings.Replace(s, "    && tools normalize-mtimes -root /out", "", 1)
		},
	} {
		if p := checkNixDBNormalized(mutate(nixDBDockerfile)); len(p) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
}

// Each rejection names its own reason, not a follow-on problem.
func TestCheckNixDBNormalizedNamesTheReason(t *testing.T) {
	// The key is the expected text, a "|" and a label for the case.
	for key, mutate := range map[string]func(string) string{
		"outside the roots": func(s string) string { return strings.Replace(s, "-db /out/opt", "-db /elsewhere/opt", 1) },
		"not a path ending": func(s string) string { return strings.Replace(s, "var/nix/db/db.sqlite", "var/nix/db/schema", 1) },
		"after normalize-mtimes": func(s string) string {
			return strings.Replace(strings.Replace(s, "    && tools normalize-nix-db -db /out/opt/nixbox/var/nix/db/db.sqlite \\\n", "", 1), "-root /out\n", "-root /out \\\n    && tools normalize-nix-db -db /out/opt/nixbox/var/nix/db/db.sqlite\n", 1)
		},
		"after normalize-nix-db|a parent directory is copied over after the step": func(s string) string {
			return strings.Replace(s, "    && tools normalize-mtimes", "    && cp -a /src/nixbox /out/opt/ \\\n    && tools normalize-mtimes", 1)
		},
		"after normalize-nix-db|a later RUN removes and copies the parent": func(s string) string {
			return strings.Replace(s, "    && tools normalize-mtimes -root /out\n", "\nRUN rm -rf /out/opt/nixbox && cp -a /x /out/opt/nixbox\nRUN tools normalize-mtimes -root /out\n", 1)
		},
		"after normalize-nix-db|the whole root is copied over after the step": func(s string) string {
			return strings.Replace(s, "    && tools normalize-mtimes", "    && cp -a /src/. /out/ \\\n    && tools normalize-mtimes", 1)
		},
		"after normalize-nix-db": func(s string) string {
			return strings.Replace(s, "    && tools normalize-mtimes", "    && echo x >> /out/opt/nixbox/var/nix/db/db.sqlite \\\n    && tools normalize-mtimes", 1)
		},
	} {
		want, _, _ := strings.Cut(key, "|")
		if p := strings.Join(checkNixDBNormalized(mutate(nixDBDockerfile)), "; "); !strings.Contains(p, want) {
			t.Errorf("%s: want a problem containing %q, got %q", key, want, p)
		}
	}
}

func TestRealNixboxDockerfileNormalizesTheNixDatabase(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "bundles", "nixbox", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if problems := checkNixDBNormalized(string(data)); len(problems) != 0 {
		t.Error(strings.Join(problems, "; "))
	}
}
