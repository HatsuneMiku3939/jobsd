package main

import "testing"

func shouldSkipWindowsLifecycleE2E(ciValue, githubActionsValue string) bool {
	return ciValue != "" || githubActionsValue != ""
}

func TestShouldSkipWindowsLifecycleE2E(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		ciValue            string
		githubActionsValue string
		want               bool
	}{
		{
			name: "local environment",
			want: false,
		},
		{
			name:    "generic ci environment",
			ciValue: "true",
			want:    true,
		},
		{
			name:               "github actions environment",
			githubActionsValue: "true",
			want:               true,
		},
		{
			name:               "both environments present",
			ciValue:            "1",
			githubActionsValue: "true",
			want:               true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := shouldSkipWindowsLifecycleE2E(tt.ciValue, tt.githubActionsValue)
			if got != tt.want {
				t.Fatalf("shouldSkipWindowsLifecycleE2E(%q, %q) = %t, want %t", tt.ciValue, tt.githubActionsValue, got, tt.want)
			}
		})
	}
}
