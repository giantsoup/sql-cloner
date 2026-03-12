package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taylor/dbgold/internal/execx"
)

func TestDiscoverSnapshotsFiltersSpecialDirs(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "_logs"))
	mustMkdir(t, filepath.Join(root, ".tmp-demo"))
	mustMkdir(t, filepath.Join(root, "tmp-demo"))
	valid := filepath.Join(root, "appdb")
	mustMkdir(t, valid)
	if err := os.WriteFile(filepath.Join(valid, "@.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write dump marker: %v", err)
	}
	if err := WriteSnapshotInfo(filepath.Join(valid, "snapshot.info"), Snapshot{Name: "appdb"}); err != nil {
		t.Fatalf("write snapshot info: %v", err)
	}

	snapshots, err := DiscoverSnapshots(root)
	if err != nil {
		t.Fatalf("discover snapshots: %v", err)
	}

	if len(snapshots) != 1 || snapshots[0].Name != "appdb" {
		t.Fatalf("unexpected snapshots: %+v", snapshots)
	}
}

func TestFilterNamesFuzzy(t *testing.T) {
	items := []string{"alpha", "beta", "customer-prod"}
	filtered := FilterNames(items, "cp")
	if len(filtered) != 1 || filtered[0] != "customer-prod" {
		t.Fatalf("unexpected filtered names: %#v", filtered)
	}
}

func TestValidDBName(t *testing.T) {
	if !ValidDBName("db_name-1$") {
		t.Fatal("expected valid database name")
	}
	if ValidDBName("bad name") {
		t.Fatal("expected invalid database name")
	}
}

func TestDiscoverDatabasesUsesTableSchemaQuery(t *testing.T) {
	runner := &discoverDBRunner{
		runFunc: func(_ context.Context, cmd execx.Command) (execx.Result, error) {
			if cmd.Name != "mysql" {
				t.Fatalf("unexpected command %q", cmd.Name)
			}
			query := lastArg(cmd.Args)
			if !strings.Contains(query, "SELECT TABLE_SCHEMA") {
				t.Fatalf("expected TABLE_SCHEMA select, got %q", query)
			}
			if strings.Contains(query, "SCHEMA_NAME") {
				t.Fatalf("did not expect SCHEMA_NAME in query, got %q", query)
			}
			return execx.Result{Stdout: "appdb\t12\t4096\n"}, nil
		},
	}

	svc := newConfiguredTestService(t, runner, Config{
		MySQLHost: defaultMySQLHost,
		MySQLPort: defaultMySQLPort,
		MySQLUser: defaultMySQLUser,
	})

	dbs, err := svc.DiscoverDatabases(context.Background())
	if err != nil {
		t.Fatalf("discover databases: %v", err)
	}
	if len(dbs) != 1 || dbs[0].Name != "appdb" || dbs[0].TableCount != 12 || dbs[0].SizeBytes != 4096 {
		t.Fatalf("unexpected databases: %+v", dbs)
	}
}

type discoverDBRunner struct {
	runFunc func(context.Context, execx.Command) (execx.Result, error)
}

func (r *discoverDBRunner) Run(ctx context.Context, cmd execx.Command) (execx.Result, error) {
	return r.runFunc(ctx, cmd)
}

func (*discoverDBRunner) Stream(context.Context, execx.Command, execx.StreamHandler) (execx.Result, error) {
	return execx.Result{}, nil
}

func lastArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[len(args)-1]
}
