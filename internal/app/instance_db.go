package app

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hatsunemiku3939/jobsd/internal/config"
	"github.com/hatsunemiku3939/jobsd/internal/sqlite"
)

type instanceDB struct {
	Paths          config.Paths
	DB             *sql.DB
	Jobs           *sqlite.JobStore
	Runs           *sqlite.RunStore
	Metadata       *sqlite.MetadataStore
	HookDeliveries *sqlite.RunHookDeliveryStore
}

func openInstanceDB(ctx context.Context, instance string) (*instanceDB, func() error, error) {
	paths, err := config.ResolvePaths(instance)
	if err != nil {
		return nil, nil, err
	}

	db, err := sqlite.Open(paths.DatabasePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open instance database: %w", err)
	}

	if err := sqlite.Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("migrate instance database: %w", err)
	}

	instanceDB := &instanceDB{
		Paths:          paths,
		DB:             db,
		Jobs:           sqlite.NewJobStore(db),
		Runs:           sqlite.NewRunStore(db),
		Metadata:       sqlite.NewMetadataStore(db),
		HookDeliveries: sqlite.NewRunHookDeliveryStore(db),
	}

	return instanceDB, db.Close, nil
}
