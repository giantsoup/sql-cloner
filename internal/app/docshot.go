package app

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/taylor/dbgold/internal/core"
	"github.com/taylor/dbgold/internal/execx"
)

const (
	docsScreenWidth = 124
	dashboardHeight = 32
	restoreHeight   = 30
)

// RenderDocsScreen renders a stable TUI screen with dummy data for README assets.
func RenderDocsScreen(name string) (string, error) {
	cfg := core.Config{
		Onboarded:    true,
		ConfigPath:   "/dev/null",
		SnapshotRoot: "/tmp/dbgold-docs/snapshots",
		LogRoot:      "/tmp/dbgold-docs/snapshots/_logs",
		MySQLSocket:  "/tmp/mysql.sock",
		MySQLUser:    "root",
		MySQLHost:    "127.0.0.1",
		MySQLPort:    3306,
		MySQLService: "mysql@8.0",
	}

	m := newModel(
		context.Background(),
		core.NewService(cfg, docshotRunner{}, core.NewLogger(io.Discard, false)),
		launchOptions{mode: screenDashboard},
	)
	m.width = docsScreenWidth
	m.dbs = []core.Database{
		{Name: "acme_core", TableCount: 124, SizeBytes: 80_772_096},
		{Name: "billing_edge", TableCount: 58, SizeBytes: 18_874_368},
		{Name: "reporting_demo", TableCount: 212, SizeBytes: 136_314_880},
	}
	m.snapshots = []core.Snapshot{
		{
			Name:      "acme_core",
			UpdatedAt: time.Date(2026, time.March, 23, 7, 20, 0, 0, time.Local),
			CreatedAt: time.Date(2026, time.March, 23, 14, 20, 0, 0, time.UTC),
			SizeBytes: 80_772_096,
			Fields: map[string]string{
				"compression": "zstd",
				"threads":     "8",
				"tool":        "mysqlsh util.dumpSchemas",
			},
		},
		{
			Name:      "billing_edge",
			UpdatedAt: time.Date(2026, time.March, 23, 5, 5, 0, 0, time.Local),
			CreatedAt: time.Date(2026, time.March, 23, 12, 5, 0, 0, time.UTC),
			SizeBytes: 18_874_368,
			Fields: map[string]string{
				"compression": "none",
				"threads":     "4",
				"tool":        "mysqlsh util.dumpSchemas",
			},
		},
		{
			Name:      "reporting_demo",
			UpdatedAt: time.Date(2026, time.March, 22, 13, 40, 0, 0, time.Local),
			CreatedAt: time.Date(2026, time.March, 22, 20, 40, 0, 0, time.UTC),
			SizeBytes: 136_314_880,
			Fields: map[string]string{
				"compression": "zstd",
				"threads":     "12",
				"tool":        "mysqlsh util.dumpSchemas",
			},
		},
	}
	m.dashboardFocus = 1
	m.syncTables()

	switch name {
	case "dashboard":
		m.height = dashboardHeight
		m.screen = screenDashboard
	case "restore":
		m.height = restoreHeight
		m.screen = screenRestorePicker
		m.filter.SetValue("demo")
		m.applyFilter()
	default:
		return "", fmt.Errorf("unknown docs screen %q", name)
	}

	m.resize()
	return m.View().Content, nil
}

type docshotRunner struct{}

func (docshotRunner) Run(context.Context, execx.Command) (execx.Result, error) {
	return execx.Result{}, nil
}

func (docshotRunner) Stream(context.Context, execx.Command, execx.StreamHandler) (execx.Result, error) {
	return execx.Result{}, nil
}
