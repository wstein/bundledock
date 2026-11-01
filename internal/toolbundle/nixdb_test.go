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
	for _, off := range []int{24, 92} { // change counter, version-valid-for
		if binary.BigEndian.Uint32(before[off:]) == 1 || binary.BigEndian.Uint32(after[off:]) != 1 {
			t.Errorf("header counter at %d: %d -> %d, want a change to 1", off, binary.BigEndian.Uint32(before[off:]), binary.BigEndian.Uint32(after[off:]))
		}
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
		out, err := exec.Command(sqlite, p, "select count(*), sum(narSize), group_concat(path) from ValidPaths; select count(*) from Refs;").Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}()
	if err := NormalizeNixDB(p, 1704067200); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(sqlite, p, "pragma integrity_check; select distinct registrationTime from ValidPaths; select count(*), sum(narSize), group_concat(path) from ValidPaths; select count(*) from Refs;").CombinedOutput()
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
		"not sqlite":         {func(b []byte) []byte { return []byte("hello world, not a database") }, 1},
		"empty":              {func(b []byte) []byte { return nil }, 1},
		"truncated":          {func(b []byte) []byte { return b[:len(b)-4096] }, 1},
		"partial last page":  {func(b []byte) []byte { return b[:len(b)-7] }, 1},
		"freelist not empty": {func(b []byte) []byte { binary.BigEndian.PutUint32(b[36:], 1); return b }, 1},
		"stale page count":   {func(b []byte) []byte { binary.BigEndian.PutUint32(b[28:], 3); return b }, 1},
		"free block in a page": {func(b []byte) []byte {
			for pg := 1; pg < len(b)/4096; pg++ {
				binary.BigEndian.PutUint16(b[pg*4096+1:], 0x0ff0)
			}
			return b
		}, 1},
		"fragmented bytes in a page": {func(b []byte) []byte {
			for pg := 1; pg < len(b)/4096; pg++ {
				b[pg*4096+7] = 3
			}
			return b
		}, 1},
		"negative epoch":      {func(b []byte) []byte { return b }, -1},
		"epoch beyond 4 byte": {func(b []byte) []byte { return b }, 1 << 40},
		"no ValidPaths table": {func(b []byte) []byte { return []byte(strings.ReplaceAll(string(b), "ValidPaths", "ValidPathz")) }, 1},
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

// nixdb-widths.sqlite holds registrationTime as the constant 1 and as 1, 2, 3, 4
// and 6 byte integers, in the third column. Every width is patched in place.
func TestNormalizeNixDBPatchesEveryIntegerWidthInPlace(t *testing.T) {
	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("no sqlite3 shell")
	}
	p := nixDBCopy(t, "nixdb-widths.sqlite")
	if err := NormalizeNixDB(p, 1); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(sqlite, p, "pragma integrity_check; select group_concat(registrationTime), group_concat(hash) from ValidPaths;").CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got, want := string(out), "ok\n1,1,1,1,1,1|h1,h2,h3,h4,h5,h6\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	// 1 is the constant serial type 9 in the fixture, so another epoch cannot be
	// written in place.
	q := nixDBCopy(t, "nixdb-widths.sqlite")
	if err := NormalizeNixDB(q, 2); err == nil {
		t.Error("constant registrationTime changed to another epoch")
	}
}

func TestSQLColumns(t *testing.T) {
	upstream, err := os.ReadFile(filepath.Join("testdata", "nixdb", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	validPaths := string(upstream)[:strings.Index(string(upstream), ");")+2]
	for name, tc := range map[string]struct {
		sql  string
		want string
	}{
		"upstream Nix schema with comments": {validPaths, "id path hash registrationTime deriver narSize ultimate sigs ca"},
		"block comment":                     {"create table T (a int, /* c, d */ b text, c integer)", "a b c"},
		"line comment with comma and paren": {"create table T (a int, -- e, f)\n b text)", "a b"},
		"table constraints":                 {"create table T (a integer, b text, primary key (a, b), unique (b), check (a > 0), foreign key (a) references X(id), constraint k check (b != ''))", "a b"},
		"quoted names":                      {"create table T (\"a b\" int, `c` int, [d] int, 'e' int)", "a b c d e"},
		"type with arguments":               {"create table T (a numeric(10, 2), b text)", "a b"},
		"string literal with comma":         {"create table T (a text default 'x,--y)', b text)", "a b"},
		"nested default":                    {"create table T (a int default (1 + (2)), b text)", "a b"},
	} {
		if got := strings.Join(sqlColumns(tc.sql), " "); got != tc.want {
			t.Errorf("%s: got %q, want %q", name, got, tc.want)
		}
	}
}

// staleTimes counts the 4-byte big-endian values in [lo, hi] anywhere in the
// file: registrationTime bytes that survived, in a cell or in a stale gap.
func staleTimes(b []byte, lo, hi uint32) int {
	n := 0
	for i := 0; i+4 <= len(b); i++ {
		if v := binary.BigEndian.Uint32(b[i:]); v >= lo && v <= hi {
			n++
		}
	}
	return n
}

// With secure_delete off SQLite leaves the old bytes of moved or changed rows in
// the unallocated gap of a page (between the cell pointer array and the cell
// content). After a normalization no registration time may be left anywhere.
func TestNormalizeNixDBClearsStaleRowBytes(t *testing.T) {
	for _, name := range []string{"nixdb-a.sqlite", "nixdb-split.sqlite", "nixdb-overflow.sqlite"} {
		p := nixDBCopy(t, name)
		before := staleTimes(readNixDB(t, p), 1700000000, 1702000000)
		if before == 0 {
			t.Fatalf("%s: fixture holds no registration time", name)
		}
		if err := NormalizeNixDB(p, 1); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if n := staleTimes(readNixDB(t, p), 1700000000, 1702000000); n != 0 {
			t.Errorf("%s: %d registration times left (of %d)", name, n, before)
		}
	}
	p := nixDBCopy(t, "nixdb-b.sqlite")
	if err := NormalizeNixDB(p, 1); err != nil {
		t.Fatal(err)
	}
	if n := staleTimes(readNixDB(t, p), 1780000000, 1780001000); n != 0 {
		t.Errorf("nixdb-b.sqlite: %d registration times left", n)
	}
}

// The same rows with different times and a different history of the page must
// give the same bytes: here the split fixture is compared with itself shifted.
func TestNormalizeNixDBKeepsSplitAndUpdatedDatabasesValid(t *testing.T) {
	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("no sqlite3 shell")
	}
	for _, name := range []string{"nixdb-split.sqlite", "nixdb-overflow.sqlite"} {
		p := nixDBCopy(t, name)
		query := "select count(*), sum(narSize), sum(length(sigs)), group_concat(path) from ValidPaths; select count(*) from Refs;"
		want, err := exec.Command(sqlite, p, query).Output()
		if err != nil {
			t.Fatal(err)
		}
		if err := NormalizeNixDB(p, 1); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		got, err := exec.Command(sqlite, p, "pragma integrity_check; select distinct registrationTime from ValidPaths;"+query).CombinedOutput()
		if err != nil || string(got) != "ok\n1\n"+string(want) {
			t.Errorf("%s: %v\n%s\nwant ok, 1, %s", name, err, got, want)
		}
	}
}

// A history of updates and deletes leaves free blocks and fragments whose old
// bytes cannot be located reliably: the call refuses, and changes nothing.
func TestNormalizeNixDBRefusesFreeBlocksOfARealHistory(t *testing.T) {
	p := nixDBCopy(t, "nixdb-update.sqlite")
	before := readNixDB(t, p)
	err := NormalizeNixDB(p, 1)
	if err == nil || !strings.Contains(err.Error(), "free blocks") {
		t.Fatalf("got %v, want a free blocks error", err)
	}
	if !bytes.Equal(before, readNixDB(t, p)) {
		t.Fatal("file modified")
	}
}
