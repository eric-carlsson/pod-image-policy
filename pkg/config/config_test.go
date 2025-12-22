package config

import "testing"

func TestValidateValidateConfig(t *testing.T) {
	good := ValidateConfig{Rules: []ValidateRule{{Action: "warn", Message: "msg"}}}
	if err := validateValidateConfig(good); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := []struct {
		name string
		cfg  ValidateConfig
	}{
		{name: "missing action", cfg: ValidateConfig{Rules: []ValidateRule{{}}}},
		{name: "invalid action", cfg: ValidateConfig{Rules: []ValidateRule{{Action: "block"}}}},
		{name: "deny without message", cfg: ValidateConfig{Rules: []ValidateRule{{Action: "deny"}}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateValidateConfig(tc.cfg); err == nil {
				t.Fatalf("expected error, got none")
			}
		})
	}
}

func TestValidateMutateRulePlaceholders(t *testing.T) {
	repo := "foo/*"
	repl := "bar/{$1}"
	good := MutateRule{Match: ImageMatch{Repository: &repo}, Replace: ImageReplace{Repository: &repl}}
	if err := validateMutateRule(good); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tooHigh := "bar/{$2}"
	bad := MutateRule{Match: ImageMatch{Repository: &repo}, Replace: ImageReplace{Repository: &tooHigh}}
	if err := validateMutateRule(bad); err == nil {
		t.Fatalf("expected error, got none")
	}
}
