package core

import (
	"path/filepath"
	"testing"
	"time"
)

func TestParseAndWriteSnapshotInfo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.info")
	now := time.Date(2026, 3, 12, 20, 0, 0, 0, time.UTC)

	original := Snapshot{
		Name:      "demo_db",
		Path:      dir,
		InfoPath:  path,
		CreatedAt: now,
		Fields: map[string]string{
			"source": "test",
		},
	}

	if err := WriteSnapshotInfo(path, original); err != nil {
		t.Fatalf("write snapshot info: %v", err)
	}

	parsed, err := ParseSnapshotInfo(path)
	if err != nil {
		t.Fatalf("parse snapshot info: %v", err)
	}

	if parsed.Name != "demo_db" {
		t.Fatalf("unexpected name %q", parsed.Name)
	}
	if !parsed.CreatedAt.Equal(now) {
		t.Fatalf("unexpected created_at %s", parsed.CreatedAt)
	}
	if parsed.Fields["source"] != "test" {
		t.Fatalf("unexpected extra field %q", parsed.Fields["source"])
	}
}

func TestParseSnapshotInfoColonFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot.info")
	content := "database: colon_db\ncreated_at_utc: 2026-03-12T20:00:00Z\nsize_bytes: 123\n"
	if err := osWriteFile(path, []byte(content)); err != nil {
		t.Fatalf("write file: %v", err)
	}

	parsed, err := ParseSnapshotInfo(path)
	if err != nil {
		t.Fatalf("parse snapshot info: %v", err)
	}

	if parsed.Name != "colon_db" || parsed.SizeBytes != 123 {
		t.Fatalf("unexpected parsed snapshot: %+v", parsed)
	}
}
