package toolconfig

import "fmt"

// This single reviewed recipe is the user-authorized exception to prebuilt tools.
// A newer git-trim release requires a new reviewed recipe, never an implicit build.
const GitTrimProfile = "git-trim-source"
const GitTrimCommit = "65288617a637029573a5c3517abcb95b050ea02b"
const GitTrimArchiveSHA256 = "dffccf97f3801976a647f5d03629d49f17d3b930b322ee1056239084093b4e53"
const GitTrimCargoLockSHA256 = "86abff4cefcfcfa8c55b163c900030f820f036d2ad092f51a765e81c2ddb25f3"
const GitTrimBuilder = "rust:1.75.0-bookworm@sha256:87f3b2f93b82995443a1a558c234212dafe79cfdc3af956539610560369ddcd0"
const GitTrimRecipe = "git-trim-v0.4.3-rust1.75-bookworm-v1"

type GitTrimSource struct {
	Commit          string `json:"commit"`
	CargoLockSHA256 string `json:"cargo_lock_sha256"`
	Builder         string `json:"builder"`
	Recipe          string `json:"recipe"`
	Epoch           int64  `json:"source_date_epoch"`
}

func GitTrimSpec() Spec {
	return Spec{Version: "v0.4.3", URL: "https://codeload.github.com/foriequal0/git-trim/tar.gz/" + GitTrimCommit, SHA256: GitTrimArchiveSHA256, Verification: "tls-download-sha256", Source: &GitTrimSource{Commit: GitTrimCommit, CargoLockSHA256: GitTrimCargoLockSHA256, Builder: GitTrimBuilder, Recipe: GitTrimRecipe, Epoch: 1670865239}}
}

func ValidateGitTrimSpec(s Spec) error {
	want := GitTrimSpec()
	if s.Source == nil || *s.Source != *want.Source || s.Version != want.Version || s.URL != want.URL || s.SHA256 != want.SHA256 || s.SHA512 != "" || s.Verification != want.Verification {
		return fmt.Errorf("git-trim source recipe changed; source review required")
	}
	return nil
}

func ValidateGitTrimPackage(name string, p Package) error {
	if name != "git-trim" || p.Provider != "github" || p.Repository != "foriequal0/git-trim" || p.TagPrefix != "v" || p.Normalization != "semver" || p.Asset != "source.tar.gz" || len(p.Architectures) != 2 || p.Architectures["amd64"] != "x86_64-unknown-linux-gnu" || p.Architectures["arm64"] != "aarch64-unknown-linux-gnu" || len(p.AllowedHosts) != 1 || p.AllowedHosts[0] != "codeload.github.com" || p.Verification != "official-or-download" || p.BaseURL != "" || p.MetadataURL != "" || p.Sidecar != "" || p.Channel != "" || p.Install.Kind != "tar.gz" || p.Install.Profile != GitTrimProfile || len(p.Install.Members) != 1 || p.Install.Members["git-trim"] != "bin/git-trim" || p.Install.Preserve || len(p.Install.Aliases) != 0 || len(p.Install.ExcludeDirectories) != 0 || len(p.Install.ArchiveNotices) != 0 {
		return fmt.Errorf("invalid git-trim source exception recipe")
	}
	return nil
}
