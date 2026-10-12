package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPreservesFailureDiagnosticsWithoutCredentialArguments(t *testing.T) {
	dir := t.TempDir()
	command := filepath.Join(dir, "smoke-fixture")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf 'stage stdout\\n'\nprintf 'stage stderr\\n' >&2\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	_, err := run(command, "--password=argument-secret")
	if err == nil {
		t.Fatal("failure returned success")
	}
	for _, required := range []string{"smoke-fixture", "exit status 7", "stage stdout", "stage stderr"} {
		if !strings.Contains(err.Error(), required) {
			t.Fatalf("missing %q in %v", required, err)
		}
	}
	if strings.Contains(err.Error(), "argument-secret") {
		t.Fatal("credential argument exposed")
	}
}

func TestRunRetainsSuccessfulParserBytes(t *testing.T) {
	dir := t.TempDir()
	command := filepath.Join(dir, "inspect-fixture")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf '{\"Id\":\"exact\"}\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	output, err := run(command)
	if err != nil || string(output) != "{\"Id\":\"exact\"}\n" {
		t.Fatalf("output=%q error=%v", output, err)
	}
}

func TestRunBoundsAndRedactsFailureOutput(t *testing.T) {
	dir := t.TempDir()
	command := filepath.Join(dir, "large-fixture")
	t.Setenv("REPOSITORY_TOKEN", "fixture-registry-token")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nprintf '%s\\n' \"$REPOSITORY_TOKEN\"\ni=0\nwhile [ $i -lt 1000 ]; do printf '012345678901234567890123456789\\n'; i=$((i+1)); done\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	_, err := run(command)
	if err == nil || len(err.Error()) > 18000 {
		t.Fatalf("diagnostic was not bounded: %v", err)
	}
	if strings.Contains(err.Error(), "fixture-registry-token") {
		t.Fatal("credential output exposed")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Fatal("truncation was not disclosed")
	}
}

func TestRunStreamsCompleteStderrBeforeCompletion(t *testing.T) {
	dir := t.TempDir()
	command := filepath.Join(dir, "live-fixture")
	release := filepath.Join(dir, "release")
	script := "#!/bin/sh\nprintf 'ready\\n' >&2\ni=0\nwhile [ ! -f \"$RELEASE_FILE\" ]; do i=$((i+1)); [ $i -lt 100 ] || exit 9; sleep 0.01; done\ni=0\nwhile [ $i -lt 1000 ]; do printf '01234567890123456789\\n' >&2; i=$((i+1)); done\nprintf 'exact parser output\\n'\n"
	if err := os.WriteFile(command, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RELEASE_FILE", release)
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = writer
	defer func() { os.Stderr = original; reader.Close(); writer.Close() }()
	stream := make(chan []byte, 1)
	go func() {
		first := make([]byte, 6)
		_, _ = io.ReadFull(reader, first)
		_ = os.WriteFile(release, []byte("release"), 0600)
		rest, _ := io.ReadAll(reader)
		stream <- append(first, rest...)
	}()
	output, runErr := run(command)
	_ = writer.Close()
	got := <-stream
	if runErr != nil || string(output) != "exact parser output\n" {
		t.Fatalf("live stderr was delayed until completion: output=%q error=%v", output, runErr)
	}
	if len(got) != 6+1000*21 {
		t.Fatalf("successful stderr was truncated: %d bytes", len(got))
	}
}

func TestDiagnosticTailCaptureIsBoundedAndRetainsFinalReason(t *testing.T) {
	var capture diagnosticTail
	data := []byte(strings.Repeat("x", 20000) + "final failure reason")
	n, err := capture.Write(data)
	if err != nil || n != len(data) {
		t.Fatalf("write=%d error=%v", n, err)
	}
	if len(capture.data) > 8192 || !strings.HasSuffix(string(capture.data), "final failure reason") || !capture.truncated {
		t.Fatal("capture lost bounded final reason")
	}
}

func TestTruncatedStderrCannotExposeKnownCredentialSuffix(t *testing.T) {
	token := "fixture-boundary-registry-token"
	t.Setenv("REPOSITORY_TOKEN", token)
	var capture diagnosticTail
	_, _ = capture.Write([]byte(token + strings.Repeat("x", 8193-len(token))))
	diagnostic := capture.text()
	if strings.Contains(diagnostic, token[1:]) {
		t.Fatal("truncated stderr exposed credential suffix")
	}
	if !strings.Contains(diagnostic, "[REDACTED]") {
		t.Fatal("boundary credential was not redacted")
	}
}

func TestBoundaryRedactionChoosesLongestKnownCredentialSuffix(t *testing.T) {
	t.Setenv("REPOSITORY_TOKEN", "other-secret-x")
	t.Setenv("GITHUB_TOKEN", "prefix-x-long-known-credential")
	var capture diagnosticTail
	token := os.Getenv("GITHUB_TOKEN")
	_, _ = capture.Write([]byte(token + strings.Repeat("z", 8199-len(token))))
	if text := capture.text(); strings.Contains(text, "x-long-known-credential") {
		t.Fatal("short matching credential suffix hid longer credential")
	}
}

func TestCompleteBoundaryCredentialWinsOverShorterSuffix(t *testing.T) {
	t.Setenv("REPOSITORY_TOKEN", "other-secret-g")
	token := "ghp_fixture_complete_token"
	t.Setenv("GITHUB_TOKEN", token)
	var capture diagnosticTail
	_, _ = capture.Write([]byte("x" + token + strings.Repeat("z", 8192-len(token))))
	if text := capture.text(); strings.Contains(text, token[1:]) {
		t.Fatal("short suffix corrupted complete-token redaction")
	}
}
