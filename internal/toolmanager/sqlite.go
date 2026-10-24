package toolmanager

import (
	"bundledock/internal/toolconfig"
	"context"
	"encoding/hex"
	"fmt"
	"path"
	"strconv"
	"strings"
)

const productHeader = "PRODUCT,VERSION,RELATIVE-URL,SIZE,SHA3-HASH"

// sqliteProduct is the amalgamation row of the machine-readable product data
// embedded in https://www.sqlite.org/download.html.
type sqliteProduct struct {
	Version string
	Path    string
	SHA3256 string
}

// parseSQLiteProducts reads the product data comment of the download page and
// returns the one sqlite-amalgamation row, which must carry the current version
// in the constraint, a path of the pinned grammar whose numeric part matches the
// version, a positive size and a lower-case SHA3-256.
func parseSQLiteProducts(page []byte, c toolconfig.Constraint) (sqliteProduct, error) {
	var zero sqliteProduct
	lines := strings.Split(string(page), "\n")
	header := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == productHeader {
			if header >= 0 {
				return zero, fmt.Errorf("product data has more than one header")
			}
			header = i
		}
	}
	if header < 0 {
		return zero, fmt.Errorf("product data header not found")
	}
	var rows []sqliteProduct
	for _, l := range lines[header+1:] {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "-->") {
			break
		}
		if l == "" {
			continue
		}
		fields := strings.Split(l, ",")
		if len(fields) != 5 {
			return zero, fmt.Errorf("malformed product data line %q", l)
		}
		if !strings.HasPrefix(path.Base(fields[2]), "sqlite-amalgamation-") {
			continue
		}
		row := sqliteProduct{Version: fields[1], Path: fields[2], SHA3256: fields[4]}
		if size, err := strconv.ParseUint(fields[3], 10, 63); err != nil || size == 0 {
			return zero, fmt.Errorf("invalid size %q in the amalgamation row", fields[3])
		}
		if sum, err := hex.DecodeString(row.SHA3256); err != nil || len(sum) != 32 || row.SHA3256 != strings.ToLower(row.SHA3256) {
			return zero, fmt.Errorf("invalid SHA3-256 hash in the amalgamation row")
		}
		rows = append(rows, row)
	}
	switch len(rows) {
	case 0:
		return zero, fmt.Errorf("product data has no sqlite-amalgamation row")
	case 1:
	default:
		return zero, fmt.Errorf("product data has more than one sqlite-amalgamation row")
	}
	row := rows[0]
	if _, err := toolconfig.ParseVersion(row.Version); err != nil {
		return zero, fmt.Errorf("amalgamation row version: %w", err)
	}
	if !toolconfig.ValidSourcePath("sqlite-product-data", row.Path) {
		return zero, fmt.Errorf("amalgamation row path %q is not a year directory and an amalgamation file", row.Path)
	}
	numeric, err := toolconfig.NumericVersion(row.Version)
	if err != nil || !strings.HasSuffix(row.Path, "-"+numeric+".zip") {
		return zero, fmt.Errorf("amalgamation row path %q does not match the numeric form of version %s", row.Path, row.Version)
	}
	if !c.Contains(row.Version) {
		return zero, fmt.Errorf("the current SQLite release %s does not satisfy %s", row.Version, c.String())
	}
	return row, nil
}

// sourceBuild resolves the source archive of a source-build package from the
// publisher's product data. Only the archive pin is produced: toolchains and
// extra sources are never resolved here.
func (r resolver) sourceBuild(ctx context.Context, p toolconfig.Package, c toolconfig.Constraint) (map[string]toolconfig.Spec, error) {
	page, _, err := r.metadata(ctx, p.VersionSource.URL)
	if err != nil {
		return nil, err
	}
	row, err := parseSQLiteProducts(page, c)
	if err != nil {
		return nil, err
	}
	s, err := r.pin(ctx, p.BaseURL+"/"+row.Path, p.AllowedHosts, row.SHA3256, "publisher-sha3-256")
	if err != nil {
		return nil, err
	}
	s.Version, s.Toolchain = row.Version, p.Build.Toolchain
	return map[string]toolconfig.Spec{"amd64": s, "arm64": s}, nil
}
