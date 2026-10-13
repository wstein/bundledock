package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestDefaultDestination(t *testing.T) {
	output, err := exec.Command("go", "run", ".", "--help").CombinedOutput()
	if err != nil || !strings.Contains(string(output), `default "/opt/bdock/toolbox"`) {
		t.Fatalf("builder destination default: %v\n%s", err, output)
	}
}
