package core

import "time"

type Database struct {
	Name       string
	TableCount int
	SizeBytes  int64
}

type Snapshot struct {
	Name      string
	Path      string
	InfoPath  string
	CreatedAt time.Time
	SizeBytes int64
	UpdatedAt time.Time
	Fields    map[string]string
	HasInfo   bool
}

type DoctorReport struct {
	MySQLReachable  bool
	MySQLService    string
	MySQLSocket     string
	MySQLVersion    string
	SnapshotRoot    string
	LogRoot         string
	MissingCommands []string
	Warnings        []string
}

type JobKind string

const (
	JobSnapshot JobKind = "snapshot"
	JobRestore  JobKind = "restore"
)

type RunOptions struct {
	Yes                 bool
	ApproveStartService bool
	Debug               bool
}

type JobResult struct {
	Kind         JobKind
	Target       string
	LogPath      string
	Duration     time.Duration
	Summary      map[string]string
	Status       string
	StartedMySQL bool
}

type OutputSink interface {
	Status(string)
	LogLine(string)
}
