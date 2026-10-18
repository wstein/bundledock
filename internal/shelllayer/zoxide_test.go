package shelllayer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The fake initializer exposes the official cd/cdi command names and a hook.
// It deliberately emits executable output before failing to detect unsafe eval.
func TestZoxideInteractiveInitialization(t *testing.T) {
	for _, sh := range shells[:3] {
		t.Run(sh.name, func(t *testing.T) {
			executable, err := exec.LookPath(sh.name)
			if err != nil {
				t.Skip(err)
			}
			for _, mode := range []string{"enabled", "optout", "failed", "noninteractive", "manual", "manual-command", "missing"} {
				t.Run(mode, func(t *testing.T) {
					dir := layerDir(t)
					bin := t.TempDir()
					log := filepath.Join(bin, "log")
					payload := "function __zoxide_hook { :; }; function cd { builtin cd \"$@\"; }; function cdi { builtin cd \"$@\"; }"
					manual := "function __zoxide_hook { :; }"
					check := "type cdi >/dev/null 2>&1 && echo ACTIVE; builtin cd /; pwd"
					if sh.name == "fish" {
						payload = "function __zoxide_hook --on-event fish_prompt; end; function cd; builtin cd $argv; end; function cdi; builtin cd $argv; end"
						manual = "function __zoxide_hook --on-event fish_prompt; end"
						check = "functions -q cdi; and echo ACTIVE; builtin cd /; pwd"
					}
					body := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + log + "'\nprintf '%s\\n' '" + payload + "'\n"
					body = "#!/bin/sh\nprintf '%s|%s|%s\\n' \"$_ZO_ECHO\" \"$_ZO_RESOLVE_SYMLINKS\" \"$_ZO_DATA_DIR\" > '" + filepath.Join(bin, "options") + "'\n" + strings.TrimPrefix(body, "#!/bin/sh\n")
					if mode == "failed" {
						body += "exit 1\n"
					}
					if mode != "missing" {
						if err := os.WriteFile(filepath.Join(bin, "zoxide"), []byte(body), 0755); err != nil {
							t.Fatal(err)
						}
					}
					if mode == "enabled" {
						check += "\ncd " + bin + "\npwd\ncdi /\npwd\nbuiltin cd " + bin + "\npwd"
					}
					file := filepath.Join(dir, sh.file)
					script := "source " + file + "\nsource " + file + "\n" + check + "\necho PATH=$PATH"
					if sh.name == "fish" {
						script = "source " + file + "\nsource " + file + "\n" + check + "\necho PATH=$PATH"
					}
					if mode == "manual-command" {
						manual = strings.ReplaceAll(manual, "__zoxide_hook", "__zoxide_z")
					}
					if mode == "manual" || mode == "manual-command" {
						script = manual + "\n" + script
					}
					args := []string{"--norc", "--noprofile", "-i", "-c", script}
					if sh.name == "zsh" {
						args = []string{"-f", "-i", "-c", script}
					}
					if sh.name == "fish" {
						args = []string{"--no-config", "--interactive", "-c", script}
					}
					if mode == "noninteractive" {
						if sh.name == "bash" {
							args = []string{"--norc", "--noprofile", "-c", script}
						} else if sh.name == "zsh" {
							args = []string{"-f", "-c", script}
						} else {
							args = []string{"--no-config", "-c", script}
						}
					}
					cmd := exec.Command(executable, args...)
					cmd.Env = []string{"PATH=" + bin, "HOME=" + t.TempDir(), "TERM=dumb", "BDOCK_STARSHIP=0", "BDOCK_FASTFETCH=0", "BDOCK_SHORTCUTS=0", "_ZO_ECHO=1", "_ZO_RESOLVE_SYMLINKS=1", "_ZO_DATA_DIR=" + filepath.Join(bin, "data location")}
					if mode == "optout" {
						cmd.Env = append(cmd.Env, "BDOCK_ZOXIDE=0")
					}
					out, err := cmd.CombinedOutput()
					if err != nil {
						t.Fatalf("%v: %s", err, out)
					}
					enabled := mode == "enabled"
					if strings.Contains(string(out), "ACTIVE") != enabled {
						t.Fatalf("enabled=%v: %s", enabled, out)
					}
					if mode == "enabled" && strings.Count(string(out), "\n"+bin+"\n") != 2 {
						t.Fatalf("cd/cdi or native escape failed: %s", out)
					}
					if !strings.Contains(string(out), "PATH="+bin) {
						t.Fatalf("PATH changed: %s", out)
					}
					if mode == "enabled" || mode == "failed" {
						options, err := os.ReadFile(filepath.Join(bin, "options"))
						if err != nil || string(options) != "1|1|"+filepath.Join(bin, "data location")+"\n" {
							t.Fatalf("zoxide options changed: %q %v", options, err)
						}
					}
					calls, _ := os.ReadFile(log)
					want := ""
					if enabled || mode == "failed" {
						want = "init " + sh.name + " --cmd cd\n"
						if mode == "failed" {
							want += want
						}
					}
					if string(calls) != want {
						t.Fatalf("init calls %q, want %q", calls, want)
					}
				})
			}
		})
	}
}

func TestZoxideOptoutPreservesUserNavigation(t *testing.T) {
	for _, sh := range shells[:3] {
		t.Run(sh.name, func(t *testing.T) {
			executable, err := exec.LookPath(sh.name)
			if err != nil {
				t.Skip(err)
			}
			dir := layerDir(t)
			file := filepath.Join(dir, sh.file)
			setup := "function cd { echo USER_CD; }; alias cdi='echo USER_CDI'\n"
			call := "cd\ncdi\n"
			args := []string{"--norc", "--noprofile", "-i", "-c"}
			if sh.name == "zsh" {
				args = []string{"-f", "-i", "-c"}
			}
			if sh.name == "fish" {
				args = []string{"--no-config", "--interactive", "-c"}
				setup = "function cd; echo USER_CD; end\nfunction cdi; echo USER_CDI; end\n"
			}
			// A second source gives Bash/Zsh aliases a fresh parsing boundary.
			calls := filepath.Join(dir, "calls")
			if err := os.WriteFile(calls, []byte(call), 0600); err != nil {
				t.Fatal(err)
			}
			args = append(args, setup+"source "+file+"\nsource "+calls)
			cmd := exec.Command(executable, args...)
			cmd.Env = []string{"PATH=" + t.TempDir(), "HOME=" + t.TempDir(), "TERM=dumb", "BDOCK_ZOXIDE=0", "BDOCK_STARSHIP=0", "BDOCK_FASTFETCH=0", "BDOCK_SHORTCUTS=0"}
			out, err := cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(out), "USER_CD\nUSER_CDI\n") {
				t.Fatalf("user navigation changed: %v: %s", err, out)
			}
		})
	}
}

// Nu string hooks are parsed into the session before the first prompt. Run a
// real PTY: -c never evaluates pre_prompt hooks and cannot verify this contract.
func TestZoxideNushellFirstPrompt(t *testing.T) {
	nu, err := exec.LookPath("nu")
	if err != nil {
		t.Skip(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip(err)
	}
	fixture, err := filepath.Abs("testdata/zoxide-0.10.0.nu")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"enabled", "optout", "failed", "missing", "manual", "manual-hook", "late-optout", "late-manual", "late-missing"} {
		t.Run(mode, func(t *testing.T) {
			dir := layerDir(t)
			bin := t.TempDir()
			home := t.TempDir()
			log := filepath.Join(home, "zoxide.log")
			target := filepath.Join(home, "target space")
			if err := os.Mkdir(target, 0700); err != nil {
				t.Fatal(err)
			}
			body := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + log + "'\ncase \"$1\" in\ninit) /bin/cat '" + fixture + "' ;;\nquery) printf '%s\\n' '" + target + "' ;;\nesac\n"
			if mode == "failed" {
				body = "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + log + "'\nprintf \"print 'PARTIAL_EXECUTED'\\n\"\nexit 1\n"
			}
			if mode != "missing" {
				if err := os.WriteFile(filepath.Join(bin, "zoxide"), []byte(body), 0700); err != nil {
					t.Fatal(err)
				}
			}
			file := filepath.Join(dir, NuPath)
			setup := `$env.config.show_banner = false; $env.config.use_ansi_coloring = false; $env.config.history.max_size = 0; $env.PROMPT_COMMAND = {|| 'READY> '}; $env.PROMPT_COMMAND_RIGHT = {|| ''}; $env.PROMPT_INDICATOR = ''; $env.config.hooks.pre_prompt = ['$env.UNRELATED_HOOK = (($env.UNRELATED_HOOK? | default 0) + 1)']; `
			if mode == "optout" || mode == "late-optout" {
				setup += `alias cd = echo USER_CD; `
			}
			if mode == "manual" {
				setup += `def __zoxide_z [] {}; alias cd = echo USER_CD; `
			}
			if mode == "manual-hook" {
				setup += `$env.config.hooks.pre_prompt = ($env.config.hooks.pre_prompt | append {__zoxide_hook: true, code: {|| null }}); alias cd = echo USER_CD; `
			}
			if mode == "late-manual" {
				setup += `$env.config.hooks.pre_prompt = ($env.config.hooks.pre_prompt | append {code: 'def __zoxide_z [] {}; alias cd = echo USER_CD'}); `
			}
			setup += "source " + file + "; source " + file
			if mode == "late-missing" {
				setup += "; rm " + filepath.Join(bin, "zoxide")
			}
			if mode == "late-optout" {
				setup += `; $env.BDOCK_ZOXIDE = '0'`
			}
			lines := []string{`print $"FIRST_CDI:((which cdi | is-not-empty))"; print $"PENDING:(($env.config.hooks.pre_prompt | where {|h| try {$h.__bdock_zoxide_init} catch {false}} | length))"`}
			enabled := mode == "enabled"
			if enabled {
				// Inspect the fake log in the first user input, then leave the
				// initial directory before the next ordinary prompt hook.
				lines[0] = `print $"INITIAL_ADD:((open --raw ` + log + ` | lines | any {|line| $line == 'add -- ` + home + `'}))"; print $"FIRST_UNRELATED:($env.UNRELATED_HOOK)"; cd /; ` + lines[0]
				lines = append(lines, `cd keyword; print $"JUMP:($env.PWD)"; cdi anything; print $"FUZZY:($env.PWD)"; __bdock_native_cd --physical /; print $"NATIVE:($env.PWD)"; source `+file)
			} else if mode == "manual" || mode == "manual-hook" || mode == "late-manual" || mode == "optout" || mode == "late-optout" {
				lines = append(lines, `print (cd); print 'MANUAL_KEPT'`)
			}
			lines = append(lines, `print $"HOOKS:($env.config.hooks.pre_prompt | length)"; print $"PATH:($env.PATH | str join ':')"; print $"DATA:($env._ZO_DATA_DIR)"; print $"UNRELATED:($env.UNRELATED_HOOK > 0)"; exit`)
			env := map[string]string{"PATH": bin, "HOME": home, "XDG_CONFIG_HOME": home, "XDG_DATA_HOME": home, "XDG_CACHE_HOME": home, "XDG_DATA_DIRS": home, "TERM": "xterm", "BDOCK_STARSHIP": "0", "BDOCK_FASTFETCH": "0", "_ZO_DATA_DIR": filepath.Join(home, "data location")}
			if mode == "optout" {
				env["BDOCK_ZOXIDE"] = "0"
			}
			spec := map[string]any{"nu": nu, "cwd": home, "env": env, "setup": setup, "lines": lines}
			encoded, _ := json.Marshal(spec)
			input := filepath.Join(home, "spec.json")
			if err := os.WriteFile(input, encoded, 0600); err != nil {
				t.Fatal(err)
			}
			output, err := exec.Command(python, "testdata/nu_pty.py", input).CombinedOutput()
			if err != nil {
				t.Fatalf("PTY helper: %v: %s", err, output)
			}
			var result struct {
				Status int
				Sent   int
				Output string
			}
			if err := json.Unmarshal(output, &result); err != nil {
				t.Fatalf("%v: %s", err, output)
			}
			out := result.Output
			if result.Status != 0 || result.Sent != len(lines) || strings.Contains(out, "Error:") || strings.Contains(out, "PARTIAL_EXECUTED\n") {
				t.Fatalf("bad PTY result: %s", output)
			}
			for _, want := range []string{fmt.Sprintf("FIRST_CDI:%v\n", enabled), "PENDING:0\n", "PATH:" + bin + "\n", "DATA:" + env["_ZO_DATA_DIR"] + "\n", "UNRELATED:true\n"} {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q: %s", want, out)
				}
			}
			if enabled {
				for _, want := range []string{"INITIAL_ADD:true\n", "FIRST_UNRELATED:1\n", "JUMP:" + target + "\n", "FUZZY:" + target + "\n", "NATIVE:/\n", "HOOKS:2\n"} {
					if !strings.Contains(out, want) {
						t.Fatalf("missing %q: %s", want, out)
					}
				}
			}
			if (strings.Contains(mode, "manual") || strings.Contains(mode, "optout")) && !strings.Contains(out, "USER_CD\n") {
				t.Fatalf("manual alias lost: %s", out)
			}
			calls, _ := os.ReadFile(log)
			if enabled && (!strings.HasPrefix(string(calls), "init nushell --cmd cd\nadd -- "+home+"\n") || strings.Count(string(calls), "add -- "+home+"\n") != 1) {
				t.Fatalf("initial directory must be recorded once before first input: %s", calls)
			}
			wantCalls := 0
			if mode == "enabled" || mode == "late-optout" || mode == "late-manual" || mode == "late-missing" {
				wantCalls = 1
			}
			if mode == "failed" {
				wantCalls = 2
			}
			if got := strings.Count(string(calls), "init nushell --cmd cd\n"); got != wantCalls {
				t.Fatalf("init count %d want %d: %s", got, wantCalls, calls)
			}
			if enabled {
				for _, want := range []string{"query --exclude / -- keyword\n", "query --interactive -- anything\n"} {
					if !strings.Contains(string(calls), want) {
						t.Fatalf("missing query %q: %s", want, calls)
					}
				}
			}
		})
	}
}

func TestZoxideNushellScriptKeepsNativeCD(t *testing.T) {
	nu, err := exec.LookPath("nu")
	if err != nil {
		t.Skip(err)
	}
	dir := layerDir(t)
	home := t.TempDir()
	bin := t.TempDir()
	log := filepath.Join(home, "calls")
	if err := os.WriteFile(filepath.Join(bin, "zoxide"), []byte("#!/bin/sh\nprintf called > '"+log+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	script := `$env.config.hooks.pre_prompt = ['print untouched']; source ` + filepath.Join(dir, NuPath) + `; cd --physical /; print $"PWD:($env.PWD)"; print $"CDI_EMPTY:((which cdi | is-empty))"; print $"HOOKS:($env.config.hooks.pre_prompt | length)"`
	cmd := exec.Command(nu, "-n", "-c", script)
	cmd.Env = []string{"PATH=" + bin, "HOME=" + home, "BDOCK_STARSHIP=0", "BDOCK_FASTFETCH=0"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	for _, want := range []string{"PWD:/\n", "CDI_EMPTY:true\n", "HOOKS:1\n"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("missing %q: %s", want, out)
		}
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("noninteractive initializer invoked: %v", err)
	}
}
