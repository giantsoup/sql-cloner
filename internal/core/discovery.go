package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func DiscoverSnapshots(root string) ([]Snapshot, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	snapshots := make([]Snapshot, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "_logs" || strings.Contains(name, ".tmp.") || strings.HasPrefix(name, ".tmp-") || strings.HasPrefix(name, "tmp-") {
			continue
		}
		path := filepath.Join(root, name)
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}

		snapshot := Snapshot{
			Name:      name,
			Path:      path,
			InfoPath:  filepath.Join(path, "snapshot.info"),
			UpdatedAt: info.ModTime(),
			Fields:    map[string]string{},
		}

		if parsed, err := ParseSnapshotInfo(snapshot.InfoPath); err == nil {
			snapshot = parsed
			snapshot.Path = path
			snapshot.InfoPath = filepath.Join(path, "snapshot.info")
			snapshot.UpdatedAt = info.ModTime()
		}

		size, err := DirSize(path)
		if err != nil {
			return nil, err
		}
		if snapshot.SizeBytes == 0 {
			snapshot.SizeBytes = size
		}

		snapshots = append(snapshots, snapshot)
	}

	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].UpdatedAt.Equal(snapshots[j].UpdatedAt) {
			return snapshots[i].Name < snapshots[j].Name
		}
		return snapshots[i].UpdatedAt.After(snapshots[j].UpdatedAt)
	})

	return snapshots, nil
}

func DirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}

func (s *Service) DiscoverDatabases(ctx context.Context) ([]Database, error) {
	query := strings.Join([]string{
		"SELECT SCHEMA_NAME,",
		"COALESCE(SUM(TABLE_ROWS), 0),",
		"COALESCE(SUM(DATA_LENGTH + INDEX_LENGTH), 0)",
		"FROM information_schema.TABLES",
		"WHERE TABLE_SCHEMA NOT IN ('information_schema','mysql','performance_schema','sys')",
		"GROUP BY SCHEMA_NAME",
		"ORDER BY SCHEMA_NAME",
	}, " ")

	result, err := s.runner.Run(ctx, s.mysqlCommand("-N", "-B", "-e", query))
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	databases := make([]Database, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		db := Database{Name: parts[0]}
		if len(parts) > 1 {
			db.TableCount, _ = strconv.Atoi(parts[1])
		}
		if len(parts) > 2 {
			db.SizeBytes, _ = strconv.ParseInt(parts[2], 10, 64)
		}
		databases = append(databases, db)
	}
	return databases, nil
}

func ResolveExactName(items []string, query string) (string, bool) {
	query = strings.TrimSpace(query)
	for _, item := range items {
		if item == query {
			return item, true
		}
	}
	return "", false
}

func FilterNames(items []string, filter string) []string {
	filter = strings.TrimSpace(strings.ToLower(filter))
	if filter == "" {
		return append([]string(nil), items...)
	}

	filtered := make([]string, 0, len(items))
	for _, item := range items {
		if fuzzyMatch(strings.ToLower(item), filter) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func fuzzyMatch(value, filter string) bool {
	if strings.Contains(value, filter) {
		return true
	}
	pos := 0
	for _, want := range filter {
		found := false
		for pos < len(value) {
			if rune(value[pos]) == want {
				found = true
				pos++
				break
			}
			pos++
		}
		if !found {
			return false
		}
	}
	return true
}

func FormatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

func FormatTime(ts time.Time) string {
	if ts.IsZero() {
		return "-"
	}
	return ts.Local().Format("2006-01-02 15:04")
}
