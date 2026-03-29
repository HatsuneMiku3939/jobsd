package main

import (
	"fmt"
	"os"

	appversion "github.com/hatsunemiku3939/jobsd/version"
)

var (
	version   = appversion.Version
	commit    = "unknown"
	buildDate = "unknown"
)

type buildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	_ = currentBuildInfo()
	return nil
}

func currentBuildInfo() buildInfo {
	info := buildInfo{
		Version:   version,
		Commit:    commit,
		BuildDate: buildDate,
	}

	if info.Version == "" {
		info.Version = appversion.Version
	}
	if info.Commit == "" {
		info.Commit = "unknown"
	}
	if info.BuildDate == "" {
		info.BuildDate = "unknown"
	}

	return info
}
