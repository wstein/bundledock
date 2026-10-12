// tools-publish is the CI-only Docker adapter for native tool-image publication.
package main

import (
	"bundledock/internal/toolconfig"
	"bundledock/internal/toolpublish"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// diagnosticOutput bounds error text and removes known workflow credentials.
// Command arguments are deliberately omitted: they can contain credentials.
func diagnosticOutput(output []byte) string {
	text := string(output)
	for _, key := range []string{"REPOSITORY_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
		if value := os.Getenv(key); value != "" {
			text = strings.ReplaceAll(text, value, "[REDACTED]")
		}
	}
	const limit = 8192
	if len(text) > limit {
		text = text[:limit] + "\n[output truncated]"
	}
	return text
}

// diagnosticTail keeps only the last output bytes; Write still accepts the
// entire stream so it can be paired with the original live stderr writer.
type diagnosticTail struct {
	data      []byte
	truncated bool
}

func (tail *diagnosticTail) Write(data []byte) (int, error) {
	const limit = 8192
	size := len(data)
	if len(tail.data)+size > limit {
		tail.truncated = true
	}
	if size >= limit {
		tail.data = append(tail.data[:0], data[size-limit:]...)
	} else {
		excess := len(tail.data) + size - limit
		if excess > 0 {
			tail.data = tail.data[excess:]
		}
		tail.data = append(tail.data, data...)
	}
	return size, nil
}
func (tail *diagnosticTail) text() string {
	text := string(tail.data)
	if tail.truncated {
		// Trimming can cut through a credential before whole-value redaction.
		// Remove any known credential suffix at the capture's initial boundary.
		longest := 0
		for _, key := range []string{"REPOSITORY_TOKEN", "GITHUB_TOKEN", "GH_TOKEN"} {
			value := os.Getenv(key)
			for cut := 0; cut < len(value); cut++ {
				suffix := value[cut:]
				if len(suffix) > longest && strings.HasPrefix(text, suffix) {
					longest = len(suffix)
				}
			}
		}
		if longest > 0 {
			text = "[REDACTED]" + text[longest:]
		}
	}
	text = diagnosticOutput([]byte(text))
	if tail.truncated {
		return "[earlier stderr truncated]\n" + text
	}
	return text
}

func run(args ...string) ([]byte, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("external command required")
	}
	cmd := exec.Command(args[0], args[1:]...)
	var stderr diagnosticTail
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	output, err := cmd.Output()
	if err != nil {
		return output, fmt.Errorf("%s failed: %w\nstdout:\n%s\nstderr:\n%s", filepath.Base(args[0]), err, diagnosticOutput(output), stderr.text())
	}
	return output, nil
}

func readJSON(path string, value any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, value)
}

func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0600)
}

func copyFile(source, destination string) error {
	b, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, b, 0600)
}

func resolve(directory string, meta toolpublish.Metadata, runner func(...string) ([]byte, error)) error {
	// This is the only upstream resolution in a workflow run. No repository write
	// or push follows; all other jobs download this one lock/provenance artifact.
	bundle := toolpublish.BundleForMetadata(meta)
	args := []string{"go", "run", "./cmd/bundledock", "tools", "upgrade"}
	if bundle.Name != "toolbox" {
		args = append(args, "--directory", bundle.Directory)
	}
	if _, err := runner(args...); err != nil {
		return err
	}
	var err error
	meta.LockSHA256, err = toolpublish.HashFile(bundle.Directory + "/tools.lock.json")
	if err != nil {
		return err
	}
	if err = os.Mkdir(directory, 0700); err != nil {
		return err
	}
	if err = copyFile(bundle.Directory+"/tools.lock.json", filepath.Join(directory, "tools.lock.json")); err != nil {
		return err
	}
	return writeJSON(filepath.Join(directory, "provenance.json"), meta)
}

func artifactIdentity(meta toolpublish.Metadata, id, number, attempt, commit string) error {
	if id == "" || number == "" || attempt == "" || meta.RunID != id || !strings.HasSuffix(meta.Tag, "-b"+number+"."+attempt) || meta.Commit != commit {
		return fmt.Errorf("artifact belongs to another run, attempt or revision; re-run all jobs")
	}
	return nil
}

func publicationDecision(lock, event string, baseline func() (string, error)) (bool, error) {
	if event != "schedule" && event != "workflow_dispatch" {
		return false, fmt.Errorf("publication requires schedule or workflow_dispatch")
	}
	b, err := os.ReadFile(lock)
	if err != nil {
		return false, err
	}
	fresh, err := toolpublish.ToolDigest(b)
	if err != nil {
		return false, err
	}
	published, err := baseline()
	if err != nil {
		return false, err
	}
	return event == "workflow_dispatch" || fresh != published, nil
}

func publicationDecisionForBundle(lock, event, bundle string, baseline func() (string, error)) (bool, error) {
	publish, err := publicationDecision(lock, event, baseline)
	if (bundle == "code-server" || bundle == "node" || bundle == "bun") && event == "workflow_dispatch" && errors.Is(err, toolpublish.ErrBaselineAbsent) {
		return true, nil
	}
	return publish, err
}

func execute(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("expected resolve, plan, build, push, newest or promote")
	}
	flags := flag.NewFlagSet("tools-publish "+args[0], flag.ContinueOnError)
	bundleName := flags.String("bundle", "toolbox", "publication bundle (toolbox, code-server, node or bun)")
	directory := flags.String("directory", "resolved", "resolved artifact directory")
	arch := flags.String("architecture", "", "native target architecture")
	consumer := flags.String("consumer", "mcr.microsoft.com/devcontainers/go:2-1.27-trixie", "consumer baseline")
	receipts := flags.String("receipts", "receipts", "published receipt artifact directory")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	bundle, err := toolpublish.SelectBundle(*bundleName)
	if err != nil {
		return err
	}
	if args[0] == "validate" {
		_, err := toolconfig.Load(bundle.Directory)
		return err
	}
	lock := filepath.Join(*directory, "tools.lock.json")
	provenance := filepath.Join(*directory, "provenance.json")
	if args[0] == "resolve" {
		image, err := toolpublish.RepositoryImageForBundle(os.Getenv("REPOSITORY_URL"), bundle.Name)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		tag := now.Format("2006.01.02") + "-b" + os.Getenv("GITHUB_RUN_NUMBER") + "." + os.Getenv("GITHUB_RUN_ATTEMPT")
		meta, err := toolpublish.NewMetadata(bundle.Directory+"/tools.lock.json", image, tag, os.Getenv("GITHUB_SHA"), os.Getenv("GITHUB_SERVER_URL")+"/"+os.Getenv("GITHUB_REPOSITORY"), now.Format("2006-01-02T15:04:05Z"), os.Getenv("GITHUB_RUN_ID"))
		if err != nil {
			return err
		}
		return resolve(*directory, meta, run)
	}
	var meta toolpublish.Metadata
	if err := readJSON(provenance, &meta); err != nil {
		return err
	}
	if err := artifactIdentity(meta, os.Getenv("GITHUB_RUN_ID"), os.Getenv("GITHUB_RUN_NUMBER"), os.Getenv("GITHUB_RUN_ATTEMPT"), os.Getenv("GITHUB_SHA")); err != nil {
		return err
	}
	if !strings.HasSuffix(meta.Image, "/"+bundle.Name) {
		return fmt.Errorf("artifact belongs to a different bundle")
	}
	adapter := toolpublish.Adapter{Run: run, TagAbsent: func(image, tag string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return toolpublish.CheckTagAbsent(ctx, nil, image, tag, os.Getenv("REPOSITORY_USERNAME"), os.Getenv("REPOSITORY_TOKEN"))
	}}
	switch args[0] {
	case "plan":
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		publish, err := publicationDecisionForBundle(lock, os.Getenv("GITHUB_EVENT_NAME"), bundle.Name, func() (string, error) {
			return toolpublish.PublishedToolDigest(ctx, nil, meta.Image, os.Getenv("REPOSITORY_USERNAME"), os.Getenv("REPOSITORY_TOKEN"), func(published toolpublish.Metadata) ([]byte, error) {
				return toolpublish.PublishedLockArtifact(ctx, nil, os.Getenv("GITHUB_REPOSITORY"), os.Getenv("DEFAULT_BRANCH"), os.Getenv("GITHUB_TOKEN"), published)
			})
		})
		if err != nil {
			return err
		}
		output, err := os.OpenFile(os.Getenv("GITHUB_OUTPUT"), os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(output, "publish=%t\n", publish)
		closeErr := output.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if !publish {
			fmt.Println("All resolved tools match the published baseline; skipping native builds and publication.")
		}
		return nil
	case "newest":
		id, err := strconv.ParseInt(os.Getenv("GITHUB_RUN_ID"), 10, 64)
		if err != nil {
			return err
		}
		attempt, err := strconv.ParseInt(os.Getenv("GITHUB_RUN_ATTEMPT"), 10, 64)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return toolpublish.CheckNewestForWorkflow(ctx, nil, os.Getenv("GITHUB_REPOSITORY"), os.Getenv("DEFAULT_BRANCH"), id, attempt, meta.Commit, os.Getenv("GITHUB_TOKEN"), bundle.Workflow)
	case "build":
		if err := copyFile(lock, bundle.Directory+"/tools.lock.json"); err != nil {
			return err
		}
		if _, err := toolconfig.Load(bundle.Directory); err != nil {
			return err
		}
		result, err := adapter.Build(meta, lock, *arch, *consumer)
		if err != nil {
			return err
		}
		return writeJSON("smoked-"+*arch+".json", result)
	case "push":
		if *arch != "amd64" && *arch != "arm64" {
			return fmt.Errorf("unsupported architecture")
		}
		var receipt toolpublish.Receipt
		if err := readJSON("smoked-"+*arch+".json", &receipt); err != nil {
			return err
		}
		if receipt.Architecture != *arch {
			return fmt.Errorf("receipt architecture mismatch")
		}
		result, err := adapter.Push(meta, lock, receipt)
		if err != nil {
			return err
		}
		return writeJSON("published-"+*arch+".json", result)
	case "promote":
		paths, err := filepath.Glob(filepath.Join(*receipts, "*", "published-*.json"))
		if err != nil {
			return err
		}
		var published []toolpublish.Receipt
		for _, path := range paths {
			var receipt toolpublish.Receipt
			if err = readJSON(path, &receipt); err != nil {
				return err
			}
			published = append(published, receipt)
		}
		result, err := adapter.Promote(meta, lock, published)
		if err != nil {
			return err
		}
		if err = writeJSON("publication.json", result); err != nil {
			return err
		}
		if summary := os.Getenv("GITHUB_STEP_SUMMARY"); summary != "" {
			f, err := os.OpenFile(summary, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(f, "Published `%s`\n\nVersion tag: `%s`; stable and latest verified at the same registry digest.\n\nLock SHA256: `%s`.\n", result.Reference, meta.Tag, meta.LockSHA256)
			closeErr := f.Close()
			if err != nil {
				return err
			}
			return closeErr
		}
		return nil
	default:
		return fmt.Errorf("unknown operation %q", args[0])
	}
}

func main() {
	if err := execute(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
