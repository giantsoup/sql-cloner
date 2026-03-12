package core

import (
	"os"
	"path/filepath"
	"testing"
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
