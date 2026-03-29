package main

import (
	"context"
	"fmt"
	"os"

	"github.com/hatsunemiku3939/jobsd/internal/app"
	appversion "github.com/hatsunemiku3939/jobsd/version"
)

var (
	version   = appversion.Version
	commit    = "unknown"
	buildDate = "unknown"
)

type buildInfo = app.BuildInfo

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	return app.Execute(context.Background(), os.Stdout, os.Stderr, currentBuildInfo())
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
