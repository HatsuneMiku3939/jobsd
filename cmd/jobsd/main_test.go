package main

import (
	"testing"

	appversion "github.com/hatsunemiku3939/jobsd/version"
)

func TestCurrentBuildInfo(t *testing.T) {
	originalVersion := version
	originalCommit := commit
	originalBuildDate := buildDate

	t.Cleanup(func() {
		version = originalVersion
		commit = originalCommit
		buildDate = originalBuildDate
	})

	tests := []struct {
		name            string
		versionOverride string
		commitOverride  string
		buildDateValue  string
		want            buildInfo
	}{
		{
			name:            "falls back to package defaults",
			versionOverride: "",
			commitOverride:  "",
			buildDateValue:  "",
			want: buildInfo{
				Version:   appversion.Version,
				Commit:    "unknown",
				BuildDate: "unknown",
			},
		},
		{
			name:            "uses explicit version override",
			versionOverride: "v0.2.0",
			commitOverride:  "",
			buildDateValue:  "",
			want: buildInfo{
				Version:   "v0.2.0",
				Commit:    "unknown",
				BuildDate: "unknown",
			},
		},
		{
			name:            "uses all explicit values",
			versionOverride: "v1.2.3",
			commitOverride:  "abc1234",
			buildDateValue:  "2025-03-29T00:00:00Z",
			want: buildInfo{
				Version:   "v1.2.3",
				Commit:    "abc1234",
				BuildDate: "2025-03-29T00:00:00Z",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version = tt.versionOverride
			commit = tt.commitOverride
			buildDate = tt.buildDateValue

			got := currentBuildInfo()
			if got != tt.want {
				t.Fatalf("currentBuildInfo() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
