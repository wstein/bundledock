package quay

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// scriptedGit answers git commands by their first arguments and records them.
type scriptedGit struct {
	calls   []string
	answers map[string]string
	fail    map[string]bool
}

func (s *scriptedGit) run(_ context.Context, _ string, args ...string) (string, error) {
	line := strings.Join(args, " ")
	s.calls = append(s.calls, line)
	for prefix := range s.fail {
		if strings.HasPrefix(line, prefix) {
			return "", fmt.Errorf("Git operation failed: %s", args[0])
		}
	}
	for prefix, out := range s.answers {
		if strings.HasPrefix(line, prefix) {
			return out, nil
		}
	}
	return "", nil
}

func (s *scriptedGit) cli() *GitCLI { return &GitCLI{Run: s.run} }

func (s *scriptedGit) called(prefix string) int {
	n := 0
	for _, c := range s.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func officialRemotes() map[string]string {
	return map[string]string{"remote get-url --all origin": SourceURL + ".git\n", "remote get-url --push --all origin": "git@github.com:wstein/bundledock\n"}
}

func TestVerifySourceNeedsOfficialOriginAndAncestorOfMain(t *testing.T) {
	commit := strings.Repeat("a", 40)
	s := &scriptedGit{answers: officialRemotes()}
	if err := s.cli().VerifySource(context.Background(), commit); err != nil {
		t.Fatal(err)
	}
	if s.called("fetch --no-tags origin refs/heads/main:refs/bundledock-verify/") != 1 || s.called("cat-file -e "+commit+"^{commit}") != 1 ||
		s.called("merge-base --is-ancestor "+commit+" refs/bundledock-verify/") != 1 || s.called("update-ref -d refs/bundledock-verify/") != 1 {
		t.Fatalf("calls = %q", s.calls)
	}
	for name, answers := range map[string]map[string]string{
		"foreign fetch url": {"remote get-url --all origin": "https://example.com/x\n", "remote get-url --push --all origin": SourceURL + "\n"},
		"foreign push url":  {"remote get-url --all origin": SourceURL + "\n", "remote get-url --push --all origin": "https://example.com/x\n"},
		"two urls":          {"remote get-url --all origin": SourceURL + "\n" + SourceURL + "\n", "remote get-url --push --all origin": SourceURL + "\n"},
		"no url":            {"remote get-url --all origin": "", "remote get-url --push --all origin": SourceURL + "\n"},
	} {
		s := &scriptedGit{answers: answers}
		if err := s.cli().VerifySource(context.Background(), commit); err == nil || s.called("fetch") != 0 {
			t.Errorf("%s: err = %v calls = %q", name, err, s.calls)
		}
	}
	// A commit that is not an ancestor of main fails, and the temporary ref is still removed.
	s = &scriptedGit{answers: officialRemotes(), fail: map[string]bool{"merge-base": true}}
	if err := s.cli().VerifySource(context.Background(), commit); err == nil || s.called("update-ref -d") != 1 {
		t.Fatalf("err = %v calls = %q", err, s.calls)
	}
}

func tagObject(commit, name, annotation string) string {
	return "object " + commit + "\ntype commit\ntag " + name + "\ntagger A <a@example.com> 1 +0000\n\n" + annotation
}

func TestRemoteTagVerifiesTheAnnotatedTagSemantically(t *testing.T) {
	commit, name, annotation := strings.Repeat("a", 40), "toolbox/"+pubTag, "BundleDock toolbox/"+pubTag+"\n\nImage: x\n"
	oid := strings.Repeat("9", 40)
	base := func() *scriptedGit {
		return &scriptedGit{answers: map[string]string{
			"ls-remote --refs origin refs/tags/" + name: oid + "\trefs/tags/" + name + "\n",
			"rev-parse refs/bundledock-verify/":         oid + "\n",
			"cat-file tag refs/bundledock-verify/":      tagObject(commit, name, annotation),
		}}
	}
	s := base()
	if found, err := s.cli().RemoteTag(context.Background(), name, commit, annotation); err != nil || !found {
		t.Fatalf("found = %v err = %v", found, err)
	}
	if s.called("update-ref -d refs/bundledock-verify/") != 1 {
		t.Fatalf("calls = %q", s.calls)
	}
	// No remote tag is not an error.
	s = &scriptedGit{}
	if found, err := s.cli().RemoteTag(context.Background(), name, commit, annotation); err != nil || found || s.called("fetch") != 0 {
		t.Fatalf("found = %v err = %v", found, err)
	}
	for tcase, mutate := range map[string]func(*scriptedGit){
		"other ref": func(s *scriptedGit) {
			s.answers["ls-remote --refs origin refs/tags/"+name] = oid + "\trefs/tags/other\n"
		},
		"two lines": func(s *scriptedGit) {
			s.answers["ls-remote --refs origin refs/tags/"+name] += oid + "\trefs/tags/" + name + "\n"
		},
		"changed": func(s *scriptedGit) { s.answers["rev-parse refs/bundledock-verify/"] = strings.Repeat("8", 40) + "\n" },
		"other commit": func(s *scriptedGit) {
			s.answers["cat-file tag refs/bundledock-verify/"] = tagObject(strings.Repeat("b", 40), name, annotation)
		},
		"other name": func(s *scriptedGit) {
			s.answers["cat-file tag refs/bundledock-verify/"] = tagObject(commit, "toolbox/x", annotation)
		},
		"other message": func(s *scriptedGit) {
			s.answers["cat-file tag refs/bundledock-verify/"] = tagObject(commit, name, annotation+"extra\n")
		},
		"no message": func(s *scriptedGit) {
			s.answers["cat-file tag refs/bundledock-verify/"] = "object " + commit + "\ntype commit\n"
		},
		"not a commit": func(s *scriptedGit) {
			s.answers["cat-file tag refs/bundledock-verify/"] = strings.Replace(tagObject(commit, name, annotation), "type commit", "type tree", 1)
		},
		"fetch fails": func(s *scriptedGit) { s.fail = map[string]bool{"fetch": true} },
	} {
		s := base()
		mutate(s)
		if found, err := s.cli().RemoteTag(context.Background(), name, commit, annotation); err == nil || found {
			t.Errorf("%s: found = %v err = %v", tcase, found, err)
		}
	}
}

func TestGitCLIPassesTheTokenOnlyThroughTheEnvironment(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-token")
	var header string
	for _, e := range gitEnvironment() {
		if value, ok := strings.CutPrefix(e, "GIT_CONFIG_VALUE_0="); ok {
			header = value
		}
	}
	if !strings.HasPrefix(header, "AUTHORIZATION: basic ") || strings.Contains(header, "secret-token") {
		t.Fatalf("header = %q", header)
	}
	t.Setenv("GITHUB_TOKEN", "")
	for _, e := range gitEnvironment() {
		if strings.Contains(e, "extraheader") {
			t.Fatal("token header without a token")
		}
	}
}
