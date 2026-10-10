package toolconfig

import (
	"fmt"
	"strings"
)

// Input pins one ordered GNU patch; executable commands never come from JSON.
type Input struct {
	URL          string `json:"url"`
	SHA256       string `json:"sha256"`
	Verification string `json:"verification"`
}

func BashBase(version string) (string, string, int, error) {
	v, err := ParseVersion(version)
	if err != nil || v[0] > 99 || v[1] > 99 || v[2] > 999 {
		return "", "", 0, fmt.Errorf("unsupported Bash version %q", version)
	}
	return fmt.Sprintf("%d.%d", v[0], v[1]), fmt.Sprintf("bash%d%d", v[0], v[1]), int(v[2]), nil
}

// ValidateBashInputs binds the effective version to a base and contiguous patches.
func ValidateBashInputs(s Spec, baseURL string) error {
	base, prefix, count, err := BashBase(s.Version)
	if err != nil {
		return err
	}
	if err := ValidateSpec(s); err != nil {
		return err
	}
	if s.URL != baseURL+"/bash-"+base+".tar.gz" || len(s.Patches) != count {
		return fmt.Errorf("Bash base/patch count differs from version")
	}
	for i, p := range s.Patches {
		want := fmt.Sprintf("%s/bash-%s-patches/%s-%03d", baseURL, base, prefix, i+1)
		if p.URL != want {
			return fmt.Errorf("Bash patches must be ordered and contiguous: expected %s", want)
		}
		if p.Verification != "tls-download-sha256" && p.Verification != "publisher-sha256" {
			return fmt.Errorf("unsupported Bash patch verification")
		}
		if err := ValidateSpec(Spec{Version: s.Version, URL: p.URL, SHA256: p.SHA256, Verification: p.Verification}); err != nil {
			return err
		}
	}
	return nil
}

func bashProbe(version, baseURL string) Spec {
	base, prefix, count, err := BashBase(version)
	if err != nil {
		return Spec{Version: version}
	}
	s := Spec{Version: version, URL: baseURL + "/bash-" + base + ".tar.gz", SHA256: strings.Repeat("0", 64), Verification: "tls-download-sha256"}
	for i := 1; i <= count; i++ {
		s.Patches = append(s.Patches, Input{URL: fmt.Sprintf("%s/bash-%s-patches/%s-%03d", baseURL, base, prefix, i), SHA256: strings.Repeat("0", 64), Verification: "tls-download-sha256"})
	}
	return s
}
