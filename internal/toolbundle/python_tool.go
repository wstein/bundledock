package toolbundle

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"os"
	"strings"
)

//go:embed python-tool.py
var pythonToolRuntime string

func writePythonLauncher(root *os.Root, tool, launcher, requirements string) error {
	for _, file := range []struct {
		name, content string
		mode          os.FileMode
	}{
		{"bin/" + tool, launcher, 0755},
		{"libexec/python-tool.py", pythonToolRuntime, 0644},
		{"share/" + tool + "/" + tool + ".requirements.txt", requirements, 0644},
	} {
		if file.name == "libexec/python-tool.py" {
			existing, err := OpenFile(root, file.name)
			if err == nil {
				data, readErr := io.ReadAll(existing)
				closeErr := existing.Close()
				if readErr != nil {
					return readErr
				}
				if closeErr != nil {
					return closeErr
				}
				if !bytes.Equal(data, []byte(file.content)) {
					return fmt.Errorf("conflicting Python runtime helper")
				}
				continue
			}
			if !os.IsNotExist(err) {
				return err
			}
		}
		if err := WriteFile(root, file.name, strings.NewReader(file.content), file.mode); err != nil {
			return err
		}
	}
	return nil
}

func validatePythonLauncher(root *os.Root, tool, launcher, requirements string) error {
	for name, expected := range map[string]string{
		"bin/" + tool: launcher, "libexec/python-tool.py": pythonToolRuntime,
		"share/" + tool + "/" + tool + ".requirements.txt": requirements,
	} {
		f, err := OpenFile(root, name)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(f)
		closeErr := f.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if !bytes.Equal(data, []byte(expected)) {
			return fmt.Errorf("invalid Python tool runtime metadata: %s", name)
		}
	}
	return nil
}
