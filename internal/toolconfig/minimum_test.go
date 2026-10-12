package toolconfig

import (
	"strings"
	"testing"
)

func TestMinimumStableConstraint(t *testing.T) {
	desired, err := ParseDesired(strings.NewReader("gh >=2.102.0 # minimum\n"))
	if err != nil {
		t.Fatal(err)
	}
	c := desired["gh"]
	if c.String() != ">=2.102.0" {
		t.Fatalf("canonical minimum: %s", c.String())
	}
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"2.101.99", false}, {"2.102.0", true}, {"3.0.0", true}, {"10.0.0", true},
		{"3.0.0-rc.1", false}, {"3.0.0+build", false},
	} {
		if got := c.Contains(tc.version); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.version, got, tc.want)
		}
	}
	for _, input := range []string{"gh>=2.0.0,<", "gh>=2.0.0,", "gh>=2.0.0,<3.0.0,<4.0.0", "gh==2.0.0,<3.0.0"} {
		if _, err := ParseDesired(strings.NewReader(input)); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}
