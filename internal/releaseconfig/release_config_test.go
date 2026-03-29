package releaseconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type goreleaserConfig struct {
	Builds []struct {
		ID      string   `yaml:"id"`
		Ldflags []string `yaml:"ldflags"`
	} `yaml:"builds"`
	Archives []struct {
		ID           string   `yaml:"id"`
		NameTemplate string   `yaml:"name_template"`
		Files        []string `yaml:"files"`
	} `yaml:"archives"`
	NFPMS []struct {
		ID               string   `yaml:"id"`
		PackageName      string   `yaml:"package_name"`
		IDs              []string `yaml:"ids"`
		Formats          []string `yaml:"formats"`
		FileNameTemplate string   `yaml:"file_name_template"`
	} `yaml:"nfpms"`
	HomebrewCasks []struct {
		Name       string   `yaml:"name"`
		IDs        []string `yaml:"ids"`
		Repository struct {
			Owner string `yaml:"owner"`
			Name  string `yaml:"name"`
		} `yaml:"repository"`
	} `yaml:"homebrew_casks"`
}

func TestGoReleaserConfigEmbedsReleaseBuildMetadata(t *testing.T) {
	t.Parallel()

	config := loadGoReleaserConfig(t)

	for _, build := range config.Builds {
		if build.ID != "jobsd" {
			continue
		}

		required := []string{
			"-X github.com/hatsunemiku3939/jobsd/version.Version={{ .Version }}",
			"-X main.version={{ .Version }}",
			"-X main.commit={{ .FullCommit }}",
			"-X main.buildDate={{ .CommitDate }}",
		}

		for _, want := range required {
			if !ldflagsContain(build.Ldflags, want) {
				t.Fatalf("build %q is missing ldflags fragment %q", build.ID, want)
			}
		}

		return
	}

	t.Fatal("target build not found in .goreleaser.yaml")
}

func TestGoReleaserConfigUsesStableArchiveLayout(t *testing.T) {
	t.Parallel()

	config := loadGoReleaserConfig(t)
	if len(config.Archives) == 0 {
		t.Fatal("archives section not found in .goreleaser.yaml")
	}

	archive := config.Archives[0]
	const wantTemplate = "{{ .ProjectName }}_{{ .Version }}_{{ title .Os }}_{{ if eq .Arch \"amd64\" }}x86_64{{ else }}{{ .Arch }}{{ end }}"

	if archive.ID != "release-archives" {
		t.Fatalf("unexpected archive id: %s", archive.ID)
	}
	if archive.NameTemplate != wantTemplate {
		t.Fatalf("unexpected archive name template: %s", archive.NameTemplate)
	}

	for _, wantFile := range []string{"README*", "LICENSE*"} {
		if !stringSliceContains(archive.Files, wantFile) {
			t.Fatalf("archive files = %v, want %q", archive.Files, wantFile)
		}
	}
}

func TestGoReleaserConfigPublishesHomebrewCask(t *testing.T) {
	t.Parallel()

	config := loadGoReleaserConfig(t)
	if len(config.HomebrewCasks) != 1 {
		t.Fatalf("homebrew_casks = %d, want 1", len(config.HomebrewCasks))
	}

	cask := config.HomebrewCasks[0]
	if cask.Name != "jobsd" {
		t.Fatalf("unexpected cask name: %s", cask.Name)
	}
	if !stringSliceContains(cask.IDs, "release-archives") {
		t.Fatalf("cask ids = %v, want release-archives", cask.IDs)
	}
	if cask.Repository.Owner != "HatsuneMiku3939" || cask.Repository.Name != "homebrew-tap" {
		t.Fatalf("unexpected cask repository: %s/%s", cask.Repository.Owner, cask.Repository.Name)
	}
}

func TestGoReleaserConfigBuildsLinuxPackages(t *testing.T) {
	t.Parallel()

	config := loadGoReleaserConfig(t)
	if len(config.NFPMS) != 1 {
		t.Fatalf("nfpms = %d, want 1", len(config.NFPMS))
	}

	nfpm := config.NFPMS[0]
	if nfpm.ID != "linux-packages" {
		t.Fatalf("unexpected nfpm id: %s", nfpm.ID)
	}
	if nfpm.PackageName != "jobsd" {
		t.Fatalf("unexpected package name: %s", nfpm.PackageName)
	}
	if !stringSliceContains(nfpm.IDs, "jobsd") {
		t.Fatalf("nfpm ids = %v, want jobsd", nfpm.IDs)
	}
	for _, format := range []string{"deb", "rpm"} {
		if !stringSliceContains(nfpm.Formats, format) {
			t.Fatalf("nfpm formats = %v, want %q", nfpm.Formats, format)
		}
	}
	if nfpm.FileNameTemplate != "{{ .ConventionalFileName }}" {
		t.Fatalf("unexpected nfpm file name template: %s", nfpm.FileNameTemplate)
	}
}

func loadGoReleaserConfig(t *testing.T) goreleaserConfig {
	t.Helper()

	configPath := filepath.Join("..", "..", ".goreleaser.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read %s: %v", configPath, err)
	}

	var config goreleaserConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatalf("unmarshal %s: %v", configPath, err)
	}

	return config
}

func ldflagsContain(ldflags []string, want string) bool {
	for _, ldflag := range ldflags {
		if strings.Contains(ldflag, want) {
			return true
		}
	}

	return false
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}

	return false
}
