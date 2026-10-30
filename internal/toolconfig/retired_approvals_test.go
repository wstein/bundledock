package toolconfig

import (
	"strings"
	"testing"
)

// The owner's merge is the approval of a source build; the retired lock.approvals map is an unknown
// key, rejected like the publish gate rejects it.
func TestLockWithRetiredApprovalsIsRejected(t *testing.T) {
	var lock Lock
	err := Decode(strings.NewReader(`{"schema":2,"platforms":{},"approvals":{"sqlite3":"`+strings.Repeat("a", 64)+`"}}`), &lock)
	if err == nil || !strings.Contains(err.Error(), "approvals") {
		t.Fatalf("err %v", err)
	}
}
