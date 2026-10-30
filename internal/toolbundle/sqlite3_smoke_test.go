package toolbundle

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"bundledock/internal/testdir"
)

// The stub answers the exact statements of sqlite3-smoke.sh from canned data, so the
// test needs neither a SQLite binary nor a C compiler. Each field switches one
// behaviour; the zero value is a conforming build.
type sqliteStub struct {
	version     string
	options     string
	noFTS5      bool
	noRTree     bool
	loadExt     bool
	badSum      bool
	failDumb    bool
	failNoTerm  bool
	noLinenoise bool
}

func (s sqliteStub) script(log string) string {
	b := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	marker := "# built with cons25 terminal handling"
	if s.noLinenoise {
		marker = "# no line editor"
	}
	return `#!/bin/sh
` + marker + `
version='` + s.version + `'
options='` + s.options + `'
# Read TERM from the exported environment: macOS /bin/sh sets an unexported
# TERM=dumb when none was passed, which would hide the unset case.
term=$(env | sed -n 's/^TERM=//p'); term=${term:-unset}
if [ "$1" = --version ]; then printf '%s\n' "$version"; exit 0; fi
case "$#" in
2) db=$1; sql=$2; printf 'arg TERM=%s: %s\n' "$term" "$sql" >> '` + log + `' ;;
1) db=$1; sql=$(cat); printf 'stdin TERM=%s: %s\n' "$term" "$sql" >> '` + log + `'
   if [ "$term" = dumb ] && [ ` + b(s.failDumb) + ` = 1 ]; then echo "unsupported terminal" >&2; exit 1; fi
   if [ "$term" = unset ] && [ ` + b(s.failNoTerm) + ` = 1 ]; then echo "no terminal" >&2; exit 1; fi ;;
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
	bin, logDir := testdir.New(t), testdir.New(t)
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
	if !strings.Contains(log, "stdin TERM=unset: select 1+1;\n") || !strings.Contains(log, "stdin TERM=dumb: select 1+1;\n") || strings.Contains(log, "xterm") {
		t.Fatalf("terminal handling %q", log)
	}
	// Every check must actually run: a script that silently drops one fails here.
	for _, want := range []string{"select sqrt(16);", "dbstat", "fts5", "rtree", "pragma compile_options;", "load_extension", "arg TERM=dumb: select 1\n", "a+1"} {
		if !strings.Contains(log, want) {
			t.Errorf("the smoke never ran %q:\n%s", want, log)
		}
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
		"rtree option missing":          {sqliteStub{options: strings.Replace(conformingOptions, "ENABLE_RTREE", "", 1)}, nil},
		"dbstat option missing":         {sqliteStub{options: strings.Replace(conformingOptions, "ENABLE_DBSTAT_VTAB", "", 1)}, nil},
		"math option missing":           {sqliteStub{options: strings.Replace(conformingOptions, "ENABLE_MATH_FUNCTIONS", "", 1)}, nil},
		"linenoise not linked in":       {sqliteStub{noLinenoise: true}, nil},
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

// The consumer smoke passes the version the lock pins to the sqlite3 smoke, so a binary
// that reports another version fails.
func TestToolboxSmokePassesTheLockedSQLiteVersion(t *testing.T) {
	data, err := os.ReadFile("../../bundles/toolbox/scripts/smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`--env SQLITE3_VERSION="$sqlite3_version"`, `sh /sqlite3-smoke.sh "$SQLITE3_VERSION"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("smoke.sh lacks %q", want)
		}
	}
	line := ""
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, "sqlite3_version=$(") {
			line = l
		}
	}
	if line == "" {
		t.Fatal("smoke.sh does not read the locked version")
	}
	scripts, err := filepath.Abs("../../bundles/toolbox/scripts")
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("sh", "-c", "script_dir="+scripts+"\n"+line+"\nprintf '%s' \"$sqlite3_version\"").CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	lock, err := os.ReadFile("../../bundles/toolbox/packages.lock.json")
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Platforms map[string]map[string]struct{ Version string } `json:"platforms"`
	}
	if err := json.Unmarshal(lock, &parsed); err != nil {
		t.Fatal(err)
	}
	if want := parsed.Platforms["amd64"]["sqlite3"].Version; want == "" || string(out) != want {
		t.Fatalf("smoke.sh reads %q, the lock pins %q", out, want)
	}
}
