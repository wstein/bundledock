package toolpublish

import "testing"

func TestRepositoryImageNormalizesNamespacePrefix(t *testing.T) {
	for _, prefix := range []string{"quay.io/bundledock", "quay.io/bundledock/", "quay.io/another_namespace", "quay.io/another.namespace/"} {
		want := prefix
		if want[len(want)-1] == '/' {
			want = want[:len(want)-1]
		}
		want += "/toolbox"
		got, err := RepositoryImage(prefix)
		if err != nil || got != want {
			t.Fatalf("RepositoryImage(%q) = %q, %v; want %q", prefix, got, err, want)
		}
	}
}

func TestRepositoryImageRejectsAmbiguousDestinations(t *testing.T) {
	for _, prefix := range []string{"", "quay.io/", "https://quay.io/bundledock/", "docker.io/bundledock", "quay.io/bundledock/toolbox", "quay.io/bundledock//", "quay.io/bundledock:stable", "quay.io/bundledock@sha256:abc", "quay.io/Bundledock", "quay.io/-bundledock", "quay.io/bundledock/extra", " quay.io/bundledock", "quay.io/bundledock\n", "quay.io/bundledock;echo hi"} {
		if _, err := RepositoryImage(prefix); err == nil {
			t.Fatalf("accepted invalid REPOSITORY_URL %q", prefix)
		}
	}
}

func TestCodeServerRepositoryAndNativeBuildSelection(t *testing.T) {
	image, err := RepositoryImageForBundle("quay.io/example", "code-server")
	if err != nil || image != "quay.io/example/code-server" {
		t.Fatalf("image %q %v", image, err)
	}
	if _, err := RepositoryImageForBundle("quay.io/example", "../toolbox"); err == nil {
		t.Fatal("accepted unsafe bundle")
	}
}
