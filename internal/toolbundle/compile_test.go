package toolbundle

import (
	"bundledock/internal/testdir"
	"bundledock/internal/toolconfig"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func sqlitePackage(t *testing.T) toolconfig.Package {
	t.Helper()
	return newSourceBuildFixture(t).config.Catalog.Packages["sqlite3"]
}

func TestCompileArgsAreDerivedFromTheRecipe(t *testing.T) {
	p := sqlitePackage(t)
	got, err := CompileArgs(p, "amd64", "/w/toolchain/zig", "/w/src", "/out/sqlite3")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/w/toolchain/zig", "cc", "-target", "x86_64-linux-musl", "-static", "-Os", "-ffile-prefix-map=/w/src=.",
		"-DHAVE_LINENOISE=1", "-DSQLITE_THREADSAFE=1", "shell.c", "sqlite3.c", "linenoise.c", "-lm", "-lpthread", "-o", "/out/sqlite3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv\n got %q\nwant %q", got, want)
	}
	got, err = CompileArgs(p, "arm64", "/z", "/w/src", "/o")
	if err != nil || got[3] != "aarch64-linux-musl" {
		t.Fatalf("arm64 target: %q %v", got, err)
	}
	p.Build.OutputForm = "static-pie"
	got, _ = CompileArgs(p, "amd64", "/z", "/w/src", "/o")
	if got[4] != "-static-pie" {
		t.Fatalf("static-pie form: %q", got)
	}
}

func TestCompileArgsRejectWhatTheVocabularyForbids(t *testing.T) {
	for name, mutate := range map[string]func(p *toolconfig.Package){
		"define":      func(p *toolconfig.Package) { p.Build.Defines = append(p.Build.Defines, "X=$(id)") },
		"cflag":       func(p *toolconfig.Package) { p.Build.Cflags = []string{"-include", "/etc/passwd"} },
		"lib":         func(p *toolconfig.Package) { p.Build.Libs = []string{"-Wl,-rpath"} },
		"source":      func(p *toolconfig.Package) { p.Build.Sources = []string{"../x.c"} },
		"not static":  func(p *toolconfig.Package) { p.Build.Static = false },
		"toolchain":   func(p *toolconfig.Package) { p.Build.Toolchain = "gcc" },
		"output form": func(p *toolconfig.Package) { p.Build.OutputForm = "pie" },
		"output name": func(p *toolconfig.Package) { p.Build.Output = "-o" },
	} {
		t.Run(name, func(t *testing.T) {
			p := sqlitePackage(t)
			mutate(&p)
			if _, err := CompileArgs(p, "amd64", "/z", "/w/src", "/o"); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	if _, err := CompileArgs(sqlitePackage(t), "riscv64", "/z", "/w/src", "/o"); err == nil {
		t.Fatal("accepted an unknown architecture")
	}
}

// fakeZig stands in for the compiler: it records its argument vector and a few
// environment variables and writes the file named by -o. Nothing is compiled.
func fakeZig(t *testing.T, dir string) string {
	t.Helper()
	script := `#!/bin/sh
{ printf '%s\n' "$@"; printf 'PWD=%s\n' "$PWD"; /usr/bin/env | /usr/bin/sort; } > "` + filepath.Join(dir, "record") + `"
while [ $# -gt 0 ]; do [ "$1" = -o ] && out=$2; shift; done
printf 'binary' > "$out"
`
	if err := os.MkdirAll(filepath.Join(dir, "toolchain"), 0o755); err != nil {
		t.Fatal(err)
	}
	zig := filepath.Join(dir, "toolchain", "zig")
	if err := os.WriteFile(zig, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	return zig
}

func TestCompileRunsTheCompilerInTheSourceDirectoryWithAScrubbedEnvironment(t *testing.T) {
	t.Setenv("SECRET_TOKEN", "must-not-leak")
	t.Setenv("HTTPS_PROXY", "http://proxy.invalid")
	f := newSourceBuildFixture(t)
	work := testdir.New(t)
	fakeZig(t, work)
	out := filepath.Join(testdir.New(t), "out")
	if err := Compile(context.Background(), f.config, "sqlite3", "amd64", work, out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(out, "sqlite3")); err != nil || string(data) != "binary" {
		t.Fatalf("output %q %v", data, err)
	}
	record, _ := os.ReadFile(filepath.Join(work, "record"))
	text := string(record)
	for _, want := range []string{"cc\n-target\nx86_64-linux-musl\n-static\n", "shell.c\nsqlite3.c\nlinenoise.c\n", "PWD=" + filepath.Join(work, "src"), "SOURCE_DATE_EPOCH=1\n", "ZIG_GLOBAL_CACHE_DIR=", "ZIG_LOCAL_CACHE_DIR="} {
		if !strings.Contains(text, want) {
			t.Errorf("compiler record lacks %q:\n%s", want, text)
		}
	}
	for _, leaked := range []string{"SECRET_TOKEN", "must-not-leak", "HTTPS_PROXY"} {
		if strings.Contains(text, leaked) {
			t.Errorf("environment leaked %s", leaked)
		}
	}
}

func TestCompileFailsWithoutAnOutputOrWithAFailingCompiler(t *testing.T) {
	f := newSourceBuildFixture(t)
	for name, script := range map[string]string{
		"exit status":           "#!/bin/sh\nexit 3\n",
		"no output":             "#!/bin/sh\nexit 0\n",
		"output is a directory": "#!/bin/sh\nwhile [ $# -gt 0 ]; do [ \"$1\" = -o ] && mkdir \"$2\"; shift; done\n",
	} {
		t.Run(name, func(t *testing.T) {
			work := testdir.New(t)
			zig := fakeZig(t, work)
			if err := os.WriteFile(zig, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := Compile(context.Background(), f.config, "sqlite3", "amd64", work, filepath.Join(testdir.New(t), "out"), &bytes.Buffer{}); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	if err := Compile(context.Background(), f.config, "nope", "amd64", testdir.New(t), testdir.New(t), &bytes.Buffer{}); err == nil {
		t.Fatal("accepted an unknown tool")
	}
}
