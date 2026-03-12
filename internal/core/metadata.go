package core

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func ParseSnapshotInfo(path string) (Snapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return Snapshot{}, err
	}
	defer file.Close()

	snapshot := Snapshot{
		InfoPath: path,
		Path:     filepath.Dir(path),
		Name:     filepath.Base(filepath.Dir(path)),
		Fields:   map[string]string{},
		HasInfo:  true,
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var key string
		var value string
		if left, right, ok := strings.Cut(line, "="); ok {
			key, value = strings.TrimSpace(left), strings.TrimSpace(right)
		} else if left, right, ok := strings.Cut(line, ":"); ok {
			key, value = strings.TrimSpace(left), strings.TrimSpace(right)
		} else {
			continue
		}
		snapshot.Fields[key] = value
	}
	if err := scanner.Err(); err != nil {
		return Snapshot{}, err
	}

	snapshot.Name = firstField(snapshot.Name, snapshot.Fields, "database", "db", "schema")
	snapshot.CreatedAt = parseSnapshotTime(snapshot.Fields["created_at_utc"], snapshot.Fields["created_at"], snapshot.Fields["created"], snapshot.Fields["timestamp"])
	snapshot.SizeBytes = parseSnapshotSize(snapshot.Fields["size_bytes"], snapshot.Fields["size"])
	return snapshot, nil
}

func WriteSnapshotInfo(path string, snapshot Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	fields := map[string]string{
		"database":       snapshot.Name,
		"created_at_utc": snapshot.CreatedAt.UTC().Format(time.RFC3339),
	}
	for key, value := range snapshot.Fields {
		if strings.TrimSpace(value) == "" {
			continue
		}
		fields[key] = value
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(fmt.Sprintf("%s=%s\n", key, fields[key]))
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func parseSnapshotTime(values ...string) time.Time {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if ts, err := time.Parse(time.RFC3339, value); err == nil {
			return ts
		}
		if ts, err := time.Parse("2006-01-02 15:04:05", value); err == nil {
			return ts
		}
		if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
			return time.Unix(unix, 0).UTC()
		}
	}
	return time.Time{}
}

func parseSnapshotSize(values ...string) int64 {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if size, err := strconv.ParseInt(value, 10, 64); err == nil {
			return size
		}
	}
	return 0
}

func firstField(def string, fields map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(fields[key]); value != "" {
			return value
		}
	}
	return def
}
