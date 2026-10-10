// tools-publish is the CI-only Docker adapter for native tool-image publication.
package main

import (
	"bundledock/internal/toolpublish"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func run(args ...string) ([]byte, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	return cmd.Output()
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
	if _, err := runner("go", "run", "./cmd/bundledock", "tools", "upgrade"); err != nil {
		return err
	}
	var err error
	meta.LockSHA256, err = toolpublish.HashFile("tools.lock.json")
	if err != nil {
		return err
	}
	if err = os.Mkdir(directory, 0700); err != nil {
		return err
	}
	if err = copyFile("tools.lock.json", filepath.Join(directory, "tools.lock.json")); err != nil {
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

func execute(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("expected resolve, build, push, newest or promote")
	}
	flags := flag.NewFlagSet("tools-publish "+args[0], flag.ContinueOnError)
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
	lock := filepath.Join(*directory, "tools.lock.json")
	provenance := filepath.Join(*directory, "provenance.json")
	if args[0] == "resolve" {
		image, err := toolpublish.RepositoryImage(os.Getenv("REPOSITORY_URL"))
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		tag := now.Format("2006.01.02") + "-b" + os.Getenv("GITHUB_RUN_NUMBER") + "." + os.Getenv("GITHUB_RUN_ATTEMPT")
		meta, err := toolpublish.NewMetadata("tools.lock.json", image, tag, os.Getenv("GITHUB_SHA"), os.Getenv("GITHUB_SERVER_URL")+"/"+os.Getenv("GITHUB_REPOSITORY"), now.Format("2006-01-02T15:04:05Z"), os.Getenv("GITHUB_RUN_ID"))
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
	adapter := toolpublish.Adapter{Run: run, TagAbsent: func(image, tag string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		return toolpublish.CheckTagAbsent(ctx, nil, image, tag, os.Getenv("REPOSITORY_USERNAME"), os.Getenv("REPOSITORY_TOKEN"))
	}}
	switch args[0] {
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
		return toolpublish.CheckNewest(ctx, nil, os.Getenv("GITHUB_REPOSITORY"), os.Getenv("DEFAULT_BRANCH"), id, attempt, meta.Commit, os.Getenv("GITHUB_TOKEN"))
	case "build":
		if err := copyFile(lock, "tools.lock.json"); err != nil {
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
