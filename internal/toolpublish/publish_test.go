package toolpublish

import (
	"errors"
	"testing"
)

func TestSmokeFailurePreventsPush(t *testing.T) {
	var pushed bool
	adapter := Adapter{Run: func(args ...string) ([]byte, error) {
		if args[0] == "sh" {
			return nil, errors.New("native smoke failed")
		}
		if len(args) > 1 && args[1] == "push" {
			pushed = true
		}
		return nil, nil
	}}
	if err := adapter.Smoke("sha256:smoked-image", "consumer:test"); err == nil {
		t.Fatal("smoke failure must stop publication")
	}
	if pushed {
		t.Fatal("failed native smoke published an image")
	}
}
