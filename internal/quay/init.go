package quay

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	siteURL    = "https://wstein.github.io/bundledock/"
	sourceURL  = "https://github.com/wstein/bundledock"
	maxPurpose = 1500
)

// purposeOf returns the one-line purpose of a bundle: the front matter
// description of docs/<bundle>.md, else its first paragraph.
func purposeOf(root, bundle string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(root, "docs", bundle+".md"))
	if err != nil {
		return "", fmt.Errorf("cannot read the purpose of %s: %w", bundle, err)
	}
	lines := strings.Split(string(raw), "\n")
	body := lines
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				body = lines[i+1:]
				break
			}
			if value, ok := strings.CutPrefix(lines[i], "description:"); ok {
				if v := strings.Trim(strings.TrimSpace(value), `"'`); v != "" {
					return clip(v), nil
				}
			}
		}
	}
	var paragraph []string
	for _, line := range body {
		line = strings.TrimSpace(line)
		skip := line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "<!--") || strings.HasPrefix(line, "[Back") || strings.HasPrefix(line, "---")
		if skip {
			if len(paragraph) > 0 {
				break
			}
			continue
		}
		paragraph = append(paragraph, line)
	}
	if len(paragraph) == 0 {
		return "", fmt.Errorf("docs/%s.md has no description or paragraph to use as the purpose", bundle)
	}
	return clip(strings.Join(paragraph, " ")), nil
}

func clip(s string) string {
	if utf8.RuneCountInString(s) <= maxPurpose {
		return s
	}
	return string([]rune(s)[:maxPurpose])
}

// DraftDescription builds a draft description from the bundle name and purpose.
func DraftDescription(root string, b Bundle) (string, error) {
	purpose, err := purposeOf(root, b.Name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("# BundleDock %s\n\n%s\n\n- Documentation: %s\n- Source and issues: %s\n", b.Name, purpose, siteURL, sourceURL), nil
}

// InitDescription writes a draft bundles/<name>/description.md and returns its
// path and size in characters. It never overwrites an existing file.
func InitDescription(root string, b Bundle) (string, int, error) {
	path := DescriptionPath(root, b.Name)
	draft, err := DraftDescription(root, b)
	if err != nil {
		return "", 0, err
	}
	if n := utf8.RuneCountInString(draft); n > MaxDescription {
		return "", 0, fmt.Errorf("draft exceeds %d characters", MaxDescription)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return "", 0, fmt.Errorf("%s already exists; edit it instead", path)
	}
	if err != nil {
		return "", 0, err
	}
	if _, err := file.WriteString(draft); err != nil {
		file.Close()
		return "", 0, err
	}
	return path, utf8.RuneCountInString(draft), file.Close()
}
