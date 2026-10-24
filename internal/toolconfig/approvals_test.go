package toolconfig

import (
	"strings"
	"testing"
)

// lock.approvals holds the owner's approved_digest of a source-build tool. The gate
// (scripts/bundle-publication.py) computes and checks it; Go only validates the shape
// so that a malformed or misplaced approval never loads.
func TestLockApprovalsAreValidated(t *testing.T) {
	good := strings.Repeat("ab", 32)
	for name, tc := range map[string]struct {
		approvals map[string]string
		ok        bool
	}{
		"none":          {nil, true},
		"source build":  {map[string]string{"sqlite3": good}, true},
		"short":         {map[string]string{"sqlite3": good[:63]}, false},
		"upper case":    {map[string]string{"sqlite3": strings.ToUpper(good)}, false},
		"not hex":       {map[string]string{"sqlite3": strings.Repeat("zz", 32)}, false},
		"prebuilt tool": {map[string]string{"gh": good}, false},
		"unknown tool":  {map[string]string{"nonesuch": good}, false},
		"empty":         {map[string]string{"sqlite3": ""}, false},
	} {
		t.Run(name, func(t *testing.T) {
			c := clone(t, linenoiseConfig(t))
			c.Lock.Approvals = tc.approvals
			err := c.Validate()
			if tc.ok != (err == nil) {
				t.Fatalf("ok=%v, error %v", tc.ok, err)
			}
		})
	}
}
