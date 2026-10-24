package toolbundle

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The stub answers the exact statements of sqlite3-smoke.sh from canned data, so the
// test needs neither a SQLite binary nor a C compiler. Each field switches one
// behaviour; the zero value is a conforming build.
type sqliteStub struct {
	version    string
	options    string
	noFTS5     bool
	noRTree    bool
	loadExt    bool
	badSum     bool
	failDumb   bool
	failNoTerm bool
}

func (s sqliteStub) script(log string) string {
	b := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	return `#!/bin/sh
version='` + s.version + `'
options='` + s.options + `'
if [ "$1" = --version ]; then printf '%s\n' "$version"; exit 0; fi
case "$#" in
2) db=$1; sql=$2 ;;
1) db=$1; sql=$(cat); printf 'stdin TERM=%s\n' "${TERM-unset}" >> '` + log + `'
   if [ "${TERM-unset}" = dumb ] && [ ` + b(s.failDumb) + ` = 1 ]; then echo "unsupported terminal" >&2; exit 1; fi
   if [ "${TERM-unset}" = unset ] && [ ` + b(s.failNoTerm) + ` = 1 ]; then echo "no terminal" >&2; exit 1; fi ;;
*) exit 64 ;;
esac
test "$db" = :memory:
case "$sql" in
*load_extension*) if [ ` + b(s.loadExt) + ` = 1 ]; then echo ok; else echo "Parse error: no such function: load_extension" >&2; exit 1; fi ;;
*"pragma compile_options"*) printf '%s\n' $options ;;
*fts5*) if [ ` + b(s.noFTS5) + ` = 1 ]; then echo "Parse error: no such module: fts5" >&2; exit 1; fi; echo 1 ;;
*rtree*) if [ ` + b(s.noRTree) + ` = 1 ]; then echo "Parse error: no such module: rtree" >&2; exit 1; fi; echo 1 ;;
*"a+1"*) if [ ` + b(s.badSum) + ` = 1 ]; then echo 41; else echo 42; fi ;;
*"sqrt(16)"*) echo 4.0 ;;
*dbstat*) echo 1 ;;
*"1+1"*) echo 2 ;;
*"select 1"*) echo 1 ;;
*) echo "unexpected statement: $sql" >&2; exit 65 ;;
esac
`
}

const conformingOptions = "ENABLE_DBSTAT_VTAB ENABLE_FTS5 ENABLE_MATH_FUNCTIONS ENABLE_RTREE OMIT_LOAD_EXTENSION THREADSAFE=1"

func runSQLiteSmoke(t *testing.T, s sqliteStub, args ...string) (string, error, string) {
	t.Helper()
	script, err := filepath.Abs("../../bundles/toolbox/scripts/sqlite3-smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	bin, logDir := t.TempDir(), t.TempDir()
	log := filepath.Join(logDir, "log")
	if s.version == "" {
		s.version = "3.54.1 2026-10-16 abcdef (64-bit)"
	}
	if s.options == "" {
		s.options = conformingOptions
	}
	if err := os.WriteFile(filepath.Join(bin, "sqlite3"), []byte(s.script(log)), 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "TERM=xterm", "SQLITE_TMPDIR=/host", "SQLITE_HISTORY=/host/history")
	out, err := cmd.CombinedOutput()
	data, _ := os.ReadFile(log)
	return string(out), err, string(data)
}

func TestSQLite3SmokeAcceptsAConformingBuild(t *testing.T) {
	out, err, log := runSQLiteSmoke(t, sqliteStub{}, "3.54.1")
	if err != nil {
		t.Fatalf("conforming build failed: %v\n%s", err, out)
	}
	// The non-terminal checks run without a terminal and with TERM=dumb, never with the caller's TERM.
	if !strings.Contains(log, "stdin TERM=unset\n") || !strings.Contains(log, "stdin TERM=dumb\n") || strings.Contains(log, "xterm") {
		t.Fatalf("terminal handling %q", log)
	}
	if _, err, _ := runSQLiteSmoke(t, sqliteStub{}); err != nil {
		t.Fatalf("the expected version is optional: %v", err)
	}
}

func TestSQLite3SmokeRejectsDeviations(t *testing.T) {
	for name, tc := range map[string]struct {
		stub sqliteStub
		args []string
	}{
		"version differs from the lock": {sqliteStub{}, []string{"3.54.2"}},
		"malformed version":             {sqliteStub{version: "wrong"}, nil},
		"wrong arithmetic":              {sqliteStub{badSum: true}, nil},
		"missing fts5 module":           {sqliteStub{noFTS5: true}, nil},
		"missing rtree module":          {sqliteStub{noRTree: true}, nil},
		"extension loading available":   {sqliteStub{loadExt: true}, nil},
		"threadsafe option missing":     {sqliteStub{options: strings.Replace(conformingOptions, "THREADSAFE=1", "THREADSAFE=0", 1)}, nil},
		"fts5 option missing":           {sqliteStub{options: strings.Replace(conformingOptions, "ENABLE_FTS5", "", 1)}, nil},
		"extension option missing":      {sqliteStub{options: strings.Replace(conformingOptions, "OMIT_LOAD_EXTENSION", "", 1)}, nil},
		"fails without TERM":            {sqliteStub{failNoTerm: true}, nil},
		"fails with TERM=dumb":          {sqliteStub{failDumb: true}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			if out, err, _ := runSQLiteSmoke(t, tc.stub, tc.args...); err == nil {
				t.Fatalf("accepted:\n%s", out)
			}
		})
	}
}
