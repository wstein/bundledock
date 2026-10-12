// Package toolconfig defines the declarative tool inputs shared by upgrades and builds.
package toolconfig

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

// Only stable, complete SemVer versions are accepted. Prereleases never satisfy
// a desired range. Numeric comparison avoids lexical ordering and integer overflow.
type Version [3]uint64

var stableVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func ParseVersion(s string) (Version, error) {
	var v Version
	if !stableVersion.MatchString(s) {
		return v, fmt.Errorf("invalid stable version %q", s)
	}
	for i, p := range strings.Split(s, ".") {
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return v, fmt.Errorf("invalid version %q: %w", s, err)
		}
		v[i] = n
	}
	return v, nil
}
func (v Version) Compare(other Version) int {
	for i := range v {
		if v[i] < other[i] {
			return -1
		}
		if v[i] > other[i] {
			return 1
		}
	}
	return 0
}
func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) }

type Constraint struct {
	// A zero Upper leaves a minimum constraint unbounded.
	Lower, Upper Version
	Exact        bool
}

func (c Constraint) String() string {
	if c.Exact {
		return "==" + c.Lower.String()
	}
	if c.Upper == (Version{}) {
		return ">=" + c.Lower.String()
	}
	return ">=" + c.Lower.String() + ",<" + c.Upper.String()
}
func (c Constraint) Contains(s string) bool {
	v, err := ParseVersion(s)
	if err != nil {
		return false
	}
	if c.Exact {
		return v == c.Lower
	}
	return v.Compare(c.Lower) >= 0 && (c.Upper == (Version{}) || v.Compare(c.Upper) < 0)
}

type Desired map[string]Constraint

var requirement = regexp.MustCompile(`^([a-z][a-z0-9-]*)\s*(==|>=)([^\s]+)$`)

func ParseDesired(r io.Reader) (Desired, error) {
	desired := Desired{}
	scanner := bufio.NewScanner(r)
	for line := 1; scanner.Scan(); line++ {
		s := strings.TrimSpace(strings.SplitN(scanner.Text(), "#", 2)[0])
		if s == "" {
			continue
		}
		s = strings.ReplaceAll(s, " ", "")
		s = strings.ReplaceAll(s, "\t", "")
		m := requirement.FindStringSubmatch(s)
		if m == nil {
			return nil, fmt.Errorf("packages.txt line %d: expected name==x.y.z, name>=x.y.z or name>=x.y.z,<x.y.z", line)
		}
		if _, ok := desired[m[1]]; ok {
			return nil, fmt.Errorf("duplicate tool %s", m[1])
		}
		c := Constraint{Exact: m[2] == "=="}
		parts := strings.Split(m[3], ",<")
		if c.Exact && len(parts) != 1 || !c.Exact && len(parts) != 1 && len(parts) != 2 {
			return nil, fmt.Errorf("tool %s requires an exact pin, minimum or bounded range", m[1])
		}
		var err error
		c.Lower, err = ParseVersion(parts[0])
		if err != nil {
			return nil, err
		}
		if !c.Exact && len(parts) == 2 {
			c.Upper, err = ParseVersion(parts[1])
			if err != nil {
				return nil, err
			}
			if c.Lower.Compare(c.Upper) >= 0 {
				return nil, fmt.Errorf("empty range for %s", m[1])
			}
		}
		desired[m[1]] = c
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(desired) == 0 {
		return nil, fmt.Errorf("empty packages.txt")
	}
	return desired, nil
}
