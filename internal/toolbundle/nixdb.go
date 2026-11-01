package toolbundle

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"strings"
)

// NormalizeNixDB makes the Nix store database at path independent of the build
// time. It sets registrationTime of every row of the ValidPaths table to epoch
// and sets the file change counter and the version-valid-for number in the
// database header to 1. Both are patched in place in the file bytes: the
// registrationTime integer keeps its serial type and width (Nix writes the build
// time as a 4-byte integer, and epoch must fit that width), so no page, cell or
// offset moves. The file is written back only when every check passed.
//
// It does not touch the order of rows, the page layout or any other content. It
// refuses (and leaves the file unchanged) a database that is not a clean,
// closed database: a non-empty -wal or -journal file next to it, a non-empty
// freelist (freed pages may hold stale rows), a page count in the header that
// differs from the file size, or a ValidPaths page with free blocks or
// fragments (stale rows inside a page). Identical bytes for identical inputs
// then still depend on Nix inserting the same rows in the same order.
func NormalizeNixDB(path string, epoch int64) error {
	if epoch < 0 {
		return fmt.Errorf("negative epoch %d", epoch)
	}
	for _, suffix := range []string{"-wal", "-journal"} {
		if info, err := os.Stat(path + suffix); err == nil && info.Size() > 0 {
			return fmt.Errorf("%s%s is not empty: the database was not closed cleanly", path, suffix)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := normalizeNixDBBytes(data, epoch); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return os.WriteFile(path, data, 0o644)
}

const (
	sqliteHeaderSize = 100
	maxBTreeDepth    = 64
)

type sqliteFile struct {
	b                []byte
	pageSize, usable int
	pages            int
	timeCol          int
	visited          map[int]bool
	checkFreeBlocks  bool
}

func normalizeNixDBBytes(data []byte, epoch int64) error {
	if len(data) < sqliteHeaderSize || !bytes.HasPrefix(data, []byte("SQLite format 3\x00")) {
		return errors.New("not an SQLite database")
	}
	ps := int(binary.BigEndian.Uint16(data[16:18]))
	if ps == 1 {
		ps = 65536
	}
	if ps < 512 || ps&(ps-1) != 0 {
		return fmt.Errorf("invalid page size %d", ps)
	}
	if len(data)%ps != 0 {
		return fmt.Errorf("file size %d is not a multiple of the page size %d", len(data), ps)
	}
	pages := len(data) / ps
	if got := int(binary.BigEndian.Uint32(data[28:32])); got != pages {
		return fmt.Errorf("header page count %d differs from the file (%d pages)", got, pages)
	}
	if n := binary.BigEndian.Uint32(data[36:40]); n != 0 {
		return fmt.Errorf("freelist holds %d pages that may contain stale rows", n)
	}
	f := &sqliteFile{b: data, pageSize: ps, usable: ps - int(data[20]), pages: pages, visited: map[int]bool{}}
	if f.usable < 480 {
		return fmt.Errorf("usable page size %d too small", f.usable)
	}
	root, cols, err := f.findValidPaths()
	if err != nil {
		return err
	}
	f.timeCol = -1
	for i, c := range cols {
		if strings.EqualFold(c, "registrationTime") {
			f.timeCol = i
		}
	}
	if f.timeCol < 0 {
		return fmt.Errorf("ValidPaths has no registrationTime column (columns %v)", cols)
	}
	f.visited = map[int]bool{}
	f.checkFreeBlocks = true
	var fields []timeField
	if err := f.walkTable(root, 0, func(abs, local, total int) error {
		tf, err := f.locateTime(abs, local)
		if err != nil {
			return err
		}
		fields = append(fields, tf)
		return nil
	}); err != nil {
		return err
	}
	// Check every value before the first byte changes.
	for _, tf := range fields {
		if tf.width > 0 && tf.width < 8 && epoch >= int64(1)<<(8*uint(tf.width)-1) {
			return fmt.Errorf("epoch %d does not fit the %d-byte registrationTime field", epoch, tf.width)
		}
		if tf.width == 0 && int64(tf.constant) != epoch {
			return fmt.Errorf("registrationTime is the constant %d, which cannot become %d in place", tf.constant, epoch)
		}
	}
	for _, tf := range fields {
		for i := 0; i < tf.width; i++ {
			shift := 8 * uint(tf.width-1-i)
			data[tf.offset+i] = byte(uint64(epoch) >> shift)
		}
	}
	binary.BigEndian.PutUint32(data[24:28], 1) // file change counter
	binary.BigEndian.PutUint32(data[92:96], 1) // version-valid-for number
	return nil
}

type timeField struct {
	offset, width int
	constant      int // serial types 8 and 9 (width 0)
}

func (f *sqliteFile) page(n int) ([]byte, int, error) {
	if n < 1 || n > f.pages {
		return nil, 0, fmt.Errorf("page %d out of range 1..%d", n, f.pages)
	}
	base := (n - 1) * f.pageSize
	hdr := 0
	if n == 1 {
		hdr = sqliteHeaderSize
	}
	return f.b[base : base+f.pageSize], hdr, nil
}

func varint(b []byte) (uint64, int) {
	var v uint64
	for i := 0; i < 9 && i < len(b); i++ {
		if i == 8 {
			return v<<8 | uint64(b[i]), 9
		}
		v = v<<7 | uint64(b[i]&0x7f)
		if b[i]&0x80 == 0 {
			return v, i + 1
		}
	}
	return 0, 0
}

// walkTable calls fn for every cell of the table b-tree at root with the file
// offset and the length of the payload part stored on the page, and the full
// payload size, in rowid order.
func (f *sqliteFile) walkTable(root, depth int, fn func(abs, local, total int) error) error {
	if depth > maxBTreeDepth || f.visited[root] {
		return fmt.Errorf("b-tree cycle or depth over %d at page %d", maxBTreeDepth, root)
	}
	f.visited[root] = true
	pg, hdr, err := f.page(root)
	if err != nil {
		return err
	}
	typ := pg[hdr]
	n := int(binary.BigEndian.Uint16(pg[hdr+3 : hdr+5]))
	if f.checkFreeBlocks && (binary.BigEndian.Uint16(pg[hdr+1:hdr+3]) != 0 || pg[hdr+7] != 0) {
		return fmt.Errorf("page %d has free blocks or fragmented bytes that may hold stale rows", root)
	}
	switch typ {
	case 0x0d: // table leaf
		ptr := hdr + 8
		for i := 0; i < n; i++ {
			if ptr+2*i+2 > f.usable {
				return fmt.Errorf("page %d: cell pointer array overruns the page", root)
			}
			off := int(binary.BigEndian.Uint16(pg[ptr+2*i:]))
			if off < ptr+2*n || off >= f.usable {
				return fmt.Errorf("page %d: cell offset %d out of range", root, off)
			}
			size, l1 := varint(pg[off:f.usable])
			_, l2 := varint(pg[off+l1 : f.usable])
			if l1 == 0 || l2 == 0 {
				return fmt.Errorf("page %d: bad cell varint", root)
			}
			start := off + l1 + l2
			local := f.localSize(int(size))
			if start+local > f.usable {
				return fmt.Errorf("page %d: payload overruns the page", root)
			}
			if err := fn((root-1)*f.pageSize+start, local, int(size)); err != nil {
				return err
			}
		}
		return nil
	case 0x05: // table interior
		ptr := hdr + 12
		for i := 0; i < n; i++ {
			if ptr+2*i+2 > f.usable {
				return fmt.Errorf("page %d: cell pointer array overruns the page", root)
			}
			off := int(binary.BigEndian.Uint16(pg[ptr+2*i:]))
			if off < ptr+2*n || off+4 > f.usable {
				return fmt.Errorf("page %d: cell offset %d out of range", root, off)
			}
			if err := f.walkTable(int(binary.BigEndian.Uint32(pg[off:])), depth+1, fn); err != nil {
				return err
			}
		}
		return f.walkTable(int(binary.BigEndian.Uint32(pg[hdr+8:])), depth+1, fn)
	}
	return fmt.Errorf("page %d: type 0x%02x is not a table b-tree page", root, typ)
}

// localSize is the number of payload bytes stored on the page itself (SQLite
// file format, "Cell Format"); the rest is on overflow pages.
func (f *sqliteFile) localSize(p int) int {
	x := f.usable - 35
	if p <= x {
		return p
	}
	m := (f.usable-12)*32/255 - 23
	k := m + (p-m)%(f.usable-4)
	if k <= x {
		return k
	}
	return m
}

// recordTypes decodes the serial types of the record header in payload and
// returns them with the offset of the first column value. Only the local part
// of the payload is available, so a header on an overflow page is an error.
func recordTypes(payload []byte) ([]uint64, int, error) {
	hl, n := varint(payload)
	if n == 0 || int(hl) > len(payload) || int(hl) < n {
		return nil, 0, errors.New("record header outside the local payload")
	}
	var types []uint64
	for p := n; p < int(hl); {
		t, l := varint(payload[p:hl])
		if l == 0 {
			return nil, 0, errors.New("bad serial type")
		}
		types = append(types, t)
		p += l
	}
	return types, int(hl), nil
}

// serialLen is the length in bytes of a column value of serial type t, or -1
// for the reserved types 10 and 11.
func serialLen(t uint64) int {
	switch {
	case t == 0, t == 8, t == 9:
		return 0
	case t <= 4:
		return int(t)
	case t == 5:
		return 6
	case t == 6, t == 7:
		return 8
	case t >= 12 && t%2 == 0:
		return int(t-12) / 2
	case t >= 13:
		return int(t-13) / 2
	}
	return -1
}

// locateTime finds the registrationTime field of the ValidPaths record whose
// local payload starts at file offset abs.
func (f *sqliteFile) locateTime(abs, local int) (timeField, error) {
	payload := f.b[abs : abs+local]
	types, off, err := recordTypes(payload)
	if err != nil {
		return timeField{}, err
	}
	if f.timeCol >= len(types) {
		return timeField{}, fmt.Errorf("record has %d columns, registrationTime is column %d", len(types), f.timeCol)
	}
	for _, t := range types[:f.timeCol] {
		l := serialLen(t)
		if l < 0 {
			return timeField{}, fmt.Errorf("reserved serial type %d", t)
		}
		off += l
	}
	t := types[f.timeCol]
	switch {
	case t >= 1 && t <= 6:
		w := serialLen(t)
		if off+w > len(payload) {
			return timeField{}, errors.New("registrationTime lies on an overflow page")
		}
		return timeField{offset: abs + off, width: w}, nil
	case t == 8 || t == 9:
		return timeField{constant: int(t - 8)}, nil
	}
	return timeField{}, fmt.Errorf("registrationTime has serial type %d, not an integer", t)
}

// findValidPaths returns the root page and the column names of ValidPaths.
func (f *sqliteFile) findValidPaths() (int, []string, error) {
	var root int
	var cols []string
	found := false
	f.checkFreeBlocks = false
	err := f.walkTable(1, 0, func(abs, local, total int) error {
		payload := f.b[abs : abs+local]
		if found {
			return nil
		}
		if total != len(payload) {
			return nil // a long schema entry on overflow pages is not ours
		}
		types, off, err := recordTypes(payload)
		if err != nil || len(types) < 5 {
			return nil
		}
		vals := make([][]byte, len(types))
		for i, t := range types {
			l := serialLen(t)
			if l < 0 || off+l > len(payload) {
				return nil
			}
			vals[i] = payload[off : off+l]
			off += l
		}
		if string(vals[0]) != "table" || string(vals[1]) != "ValidPaths" || types[3] < 1 || types[3] > 4 {
			return nil
		}
		for _, b := range vals[3] {
			root = root<<8 | int(b)
		}
		cols = sqlColumns(string(vals[4]))
		found = true
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	if !found {
		return 0, nil, errors.New("no ValidPaths table in the schema")
	}
	return root, cols, nil
}

// sqlColumns returns the column names of a CREATE TABLE statement.
func sqlColumns(sql string) []string {
	open, end := strings.Index(sql, "("), strings.LastIndex(sql, ")")
	if open < 0 || end < open {
		return nil
	}
	var defs []string
	depth, start := 0, open+1
	for i := open + 1; i < end; i++ {
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				defs = append(defs, sql[start:i])
				start = i + 1
			}
		}
	}
	defs = append(defs, sql[start:end])
	var cols []string
	for _, d := range defs {
		fields := strings.Fields(d)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "PRIMARY", "UNIQUE", "CHECK", "FOREIGN", "CONSTRAINT":
			continue
		}
		cols = append(cols, strings.Trim(fields[0], "\"`[]'"))
	}
	return cols
}
