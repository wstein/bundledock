package toolbundle

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Fixtures: testdata/nixdb/gen.sh builds them with the sqlite3 shell. Both hold
// the Nix schema and the same 120 store paths in the same order; they differ in
// registrationTime (A: 1700000001.., B: 1780000007.., all 4-byte integers).
func nixDBCopy(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "nixdb", name))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "db.sqlite")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readNixDB(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func nixDBDiff(a, b []byte) int {
	n := 0
	for i := range a {
		if a[i] != b[i] {
			n++
		}
	}
	return n
}

func TestNormalizeNixDBMakesDifferentRegistrationTimesByteIdentical(t *testing.T) {
	a, b := nixDBCopy(t, "nixdb-a.sqlite"), nixDBCopy(t, "nixdb-b.sqlite")
	if bytes.Equal(readNixDB(t, a), readNixDB(t, b)) {
		t.Fatal("fixtures are identical before normalization")
	}
	for _, p := range []string{a, b} {
		if err := NormalizeNixDB(p, 1); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(readNixDB(t, a), readNixDB(t, b)) {
		t.Fatalf("still %d differing bytes after normalization", nixDBDiff(readNixDB(t, a), readNixDB(t, b)))
	}
}

func TestNormalizeNixDBTouchesOnlyTheTimesAndTheHeaderCounters(t *testing.T) {
	p := nixDBCopy(t, "nixdb-a.sqlite")
	before := readNixDB(t, p)
	if err := NormalizeNixDB(p, 1); err != nil {
		t.Fatal(err)
	}
	after := readNixDB(t, p)
	if len(after) != len(before) {
		t.Fatalf("size %d, want %d", len(after), len(before))
	}
	// 120 rows with a 4-byte time each, plus the two 4-byte header counters.
	if n := nixDBDiff(before, after); n == 0 || n > 120*4+8 {
		t.Fatalf("%d bytes changed, want 1..%d", n, 120*4+8)
	}
	// Idempotent.
	if err := NormalizeNixDB(p, 1); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, readNixDB(t, p)) {
		t.Fatal("second run changed the file")
	}
	// The epoch is a parameter.
	q := nixDBCopy(t, "nixdb-a.sqlite")
	if err := NormalizeNixDB(q, 1704067200); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(readNixDB(t, q), after) {
		t.Fatal("epoch ignored")
	}
}

// The patched file must still be a valid database with the fixed time (needs the
// sqlite3 shell; the byte-level tests above run everywhere).
func TestNormalizeNixDBKeepsTheDatabaseValid(t *testing.T) {
	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("no sqlite3 shell")
	}
	p := nixDBCopy(t, "nixdb-b.sqlite")
	want := func() string {
		out, err := exec.Command(sqlite, "-readonly", p, "select count(*), sum(narSize), group_concat(path) from ValidPaths; select count(*) from Refs;").Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}()
	if err := NormalizeNixDB(p, 1704067200); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(sqlite, "-readonly", p, "pragma integrity_check; select distinct registrationTime from ValidPaths; select count(*), sum(narSize), group_concat(path) from ValidPaths; select count(*) from Refs;").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got, w := string(out), "ok\n1704067200\n"+want; got != w {
		t.Fatalf("got %q, want %q", got, w)
	}
}

func TestNormalizeNixDBRejectsWhatItCannotMakeDeterministic(t *testing.T) {
	good := readNixDB(t, nixDBCopy(t, "nixdb-a.sqlite"))
	for name, tc := range map[string]struct {
		mutate func([]byte) []byte
		epoch  int64
	}{
		"not sqlite":          {func(b []byte) []byte { return []byte("hello world, not a database") }, 1},
		"empty":               {func(b []byte) []byte { return nil }, 1},
		"truncated":           {func(b []byte) []byte { return b[:len(b)-4096] }, 1},
		"partial last page":   {func(b []byte) []byte { return b[:len(b)-7] }, 1},
		"freelist not empty":  {func(b []byte) []byte { binary.BigEndian.PutUint32(b[36:], 1); return b }, 1},
		"stale page count":    {func(b []byte) []byte { binary.BigEndian.PutUint32(b[28:], 3); return b }, 1},
		"negative epoch":      {func(b []byte) []byte { return b }, -1},
		"epoch beyond 4 byte": {func(b []byte) []byte { return b }, 1 << 40},
		"no ValidPaths table": {func(b []byte) []byte { return []byte(strings.Replace(string(b), "ValidPaths (id", "ValidPathz (id", 1)) }, 1},
	} {
		p := filepath.Join(t.TempDir(), "db.sqlite")
		in := tc.mutate(append([]byte(nil), good...))
		if err := os.WriteFile(p, in, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := NormalizeNixDB(p, tc.epoch); err == nil {
			t.Errorf("%s: accepted", name)
		}
		if !bytes.Equal(readNixDB(t, p), in) {
			t.Errorf("%s: file modified although the call failed", name)
		}
	}
	if err := NormalizeNixDB(filepath.Join(t.TempDir(), "missing"), 1); err == nil {
		t.Error("missing file accepted")
	}
}

func TestNormalizeNixDBRefusesAnOpenJournal(t *testing.T) {
	for _, suffix := range []string{"-wal", "-journal"} {
		p := nixDBCopy(t, "nixdb-a.sqlite")
		if err := os.WriteFile(p+suffix, []byte("pending"), 0o644); err != nil {
			t.Fatal(err)
		}
		before := readNixDB(t, p)
		if err := NormalizeNixDB(p, 1); err == nil {
			t.Errorf("%s with content accepted", suffix)
		}
		if !bytes.Equal(before, readNixDB(t, p)) {
			t.Errorf("%s: file modified", suffix)
		}
	}
	// An empty WAL (what a closed WAL-mode database leaves) is fine.
	p := nixDBCopy(t, "nixdb-a.sqlite")
	if err := os.WriteFile(p+"-wal", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NormalizeNixDB(p, 1); err != nil {
		t.Errorf("empty WAL rejected: %v", err)
	}
}
