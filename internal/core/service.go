package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/log"

	"github.com/taylor/dbgold/internal/execx"
)

var metricLine = regexp.MustCompile(`(?i)(rows|bytes|tables|duration|schemas?)`)

type Service struct {
	cfg    Config
	runner execx.Runner
	logger log.Logger
	now    func() time.Time
}

func NewService(cfg Config, runner execx.Runner, logger log.Logger) *Service {
	return &Service{
		cfg:    cfg,
		runner: runner,
		logger: logger,
		now:    time.Now,
	}
}

func (s *Service) WithConfig(cfg Config) *Service {
	return &Service{
		cfg:    cfg,
		runner: s.runner,
		logger: s.logger,
		now:    s.now,
	}
}

func (s *Service) Config() Config {
	return s.cfg
}

func (s *Service) SnapshotRoot() string {
	return s.cfg.SnapshotRoot
}

func (s *Service) LogRoot() string {
	return s.cfg.LogRoot
}

func (s *Service) DiscoverSnapshots() ([]Snapshot, error) {
	return DiscoverSnapshots(s.cfg.SnapshotRoot)
}

func (s *Service) Doctor(ctx context.Context) (DoctorReport, error) {
	report := DoctorReport{
		MySQLService: s.cfg.MySQLService,
		MySQLSocket:  s.cfg.MySQLSocket,
		SnapshotRoot: s.cfg.SnapshotRoot,
		LogRoot:      s.cfg.LogRoot,
	}

	for _, cmd := range []string{"mysql", "mysqladmin", "mysqlsh", "brew"} {
		if _, err := execLookPath(cmd); err != nil {
			report.MissingCommands = append(report.MissingCommands, cmd)
		}
	}

	reachable, version, err := s.mysqlReachable(ctx)
	if err != nil {
		report.Warnings = append(report.Warnings, err.Error())
	}
	report.MySQLReachable = reachable
	report.MySQLVersion = version

	if _, err := os.Stat(s.cfg.SnapshotRoot); errors.Is(err, os.ErrNotExist) {
		report.Warnings = append(report.Warnings, "snapshot root does not exist yet")
	}
	return report, nil
}

func (s *Service) mysqlReachable(ctx context.Context) (bool, string, error) {
	if _, err := s.runner.Run(ctx, s.mysqlAdminCommand("ping")); err != nil {
		return false, "", nil
	}

	result, err := s.runner.Run(ctx, s.mysqlCommand("-N", "-B", "-e", "SELECT VERSION()"))
	if err != nil {
		return true, "", err
	}
	return true, strings.TrimSpace(result.Stdout), nil
}

func (s *Service) EnsureMySQL(ctx context.Context, options RunOptions) (bool, error) {
	reachable, _, err := s.mysqlReachable(ctx)
	if err != nil {
		return false, err
	}
	if reachable {
		return false, nil
	}
	if !options.ApproveStartService {
		return false, fmt.Errorf("mysql is not reachable; approve starting %s to continue", s.cfg.MySQLService)
	}

	if _, err := s.runner.Run(ctx, s.brewCommand("services", "start", s.cfg.MySQLService)); err != nil {
		return false, err
	}

	deadline := time.Now().Add(s.cfg.MySQLStartTimeout)
	for time.Now().Before(deadline) {
		reachable, _, _ := s.mysqlReachable(ctx)
		if reachable {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(1500 * time.Millisecond):
		}
	}

	return false, fmt.Errorf("mysql did not become reachable after starting %s", s.cfg.MySQLService)
}

func (s *Service) RunSnapshot(ctx context.Context, db string, options RunOptions, sink OutputSink) (JobResult, error) {
	startedAt := s.now()
	if !ValidDBName(db) {
		return JobResult{}, fmt.Errorf("invalid database name %q", db)
	}

	startedService, err := s.EnsureMySQL(ctx, options)
	if err != nil {
		return JobResult{}, err
	}

	if err := os.MkdirAll(s.cfg.SnapshotRoot, 0o755); err != nil {
		return JobResult{}, err
	}
	if err := os.MkdirAll(s.cfg.LogRoot, 0o755); err != nil {
		return JobResult{}, err
	}

	finalDir := filepath.Join(s.cfg.SnapshotRoot, db)
	tempDir, err := os.MkdirTemp(s.cfg.SnapshotRoot, db+".tmp.")
	if err != nil {
		return JobResult{}, err
	}
	logPath := filepath.Join(s.cfg.LogRoot, db+".snapshot.log")

	logFile, err := os.Create(logPath)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return JobResult{}, err
	}
	defer logFile.Close()

	jsPath, err := s.writeTempJS("dump-"+db+"-*.js", s.dumpScript(db, tempDir))
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return JobResult{}, err
	}
	defer os.Remove(jsPath)

	sink.Status("dumping schema with mysqlsh")

	summary := map[string]string{}
	handler := StreamHandlerFor(logFile, sink, summary)
	if _, err := s.runner.Stream(ctx, s.mysqlShellCommand(jsPath), handler); err != nil {
		_ = os.RemoveAll(tempDir)
		return JobResult{}, err
	}

	size, err := DirSize(tempDir)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return JobResult{}, err
	}

	info := Snapshot{
		Name:      db,
		Path:      tempDir,
		InfoPath:  filepath.Join(tempDir, "snapshot.info"),
		CreatedAt: s.now().UTC(),
		SizeBytes: size,
		Fields: map[string]string{
			"tool":         "mysqlsh util.dumpSchemas",
			"threads":      strconv.Itoa(s.cfg.MySQLShellThreads),
			"compression":  s.cfg.MySQLCompression,
			"snapshot_dir": finalDir,
			"log_file":     logPath,
		},
	}
	if err := WriteSnapshotInfo(info.InfoPath, info); err != nil {
		_ = os.RemoveAll(tempDir)
		return JobResult{}, err
	}

	backupDir := finalDir + ".bak"
	_ = os.RemoveAll(backupDir)
	if _, err := os.Stat(finalDir); err == nil {
		if err := os.Rename(finalDir, backupDir); err != nil {
			_ = os.RemoveAll(tempDir)
			return JobResult{}, err
		}
	}
	if err := os.Rename(tempDir, finalDir); err != nil {
		if _, statErr := os.Stat(backupDir); statErr == nil {
			_ = os.Rename(backupDir, finalDir)
		}
		_ = os.RemoveAll(tempDir)
		return JobResult{}, err
	}
	_ = os.RemoveAll(backupDir)

	return JobResult{
		Kind:         JobSnapshot,
		Target:       db,
		LogPath:      logPath,
		Duration:     s.now().Sub(startedAt),
		Summary:      summary,
		Status:       "snapshot complete",
		StartedMySQL: startedService,
	}, nil
}

func (s *Service) RunRestore(ctx context.Context, db string, options RunOptions, sink OutputSink) (JobResult, error) {
	startedAt := s.now()
	if !ValidDBName(db) {
		return JobResult{}, fmt.Errorf("invalid database name %q", db)
	}

	startedService, err := s.EnsureMySQL(ctx, options)
	if err != nil {
		return JobResult{}, err
	}

	snapshotDir := filepath.Join(s.cfg.SnapshotRoot, db)
	if err := ValidateSnapshotDir(snapshotDir); err != nil {
		return JobResult{}, err
	}

	if err := os.MkdirAll(s.cfg.LogRoot, 0o755); err != nil {
		return JobResult{}, err
	}
	logPath := filepath.Join(s.cfg.LogRoot, db+".restore.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return JobResult{}, err
	}
	defer logFile.Close()

	restoreLocalInfile := false
	localInfileOn, err := s.localInfileOn(ctx)
	if err != nil {
		return JobResult{}, err
	}
	if !localInfileOn {
		if !s.cfg.MySQLAutoEnableLocalInfile {
			return JobResult{}, errors.New("the target server has GLOBAL local_infile=OFF; set MYSQLSH_AUTO_ENABLE_LOCAL_INFILE=1 or enable local_infile before restoring")
		}
		restoreLocalInfile = true
		sink.Status("enabling local_infile")
		if err := s.setLocalInfile(ctx, true); err != nil {
			return JobResult{}, err
		}
		defer func() {
			_ = s.setLocalInfile(context.Background(), false)
		}()
	}

	sink.Status("dropping and recreating target schema")
	if _, err := s.runner.Run(ctx, s.mysqlCommand("-e", fmt.Sprintf("DROP DATABASE IF EXISTS `%s`; CREATE DATABASE `%s`;", db, db))); err != nil {
		return JobResult{}, err
	}

	jsPath, err := s.writeTempJS("load-"+db+"-*.js", s.loadScript(db, snapshotDir))
	if err != nil {
		return JobResult{}, err
	}
	defer os.Remove(jsPath)

	sink.Status("loading dump with mysqlsh")
	summary := map[string]string{}
	handler := StreamHandlerFor(logFile, sink, summary)
	if _, err := s.runner.Stream(ctx, s.mysqlShellCommand(jsPath), handler); err != nil {
		if restoreLocalInfile {
			_ = s.setLocalInfile(context.Background(), false)
		}
		return JobResult{}, err
	}

	if restoreLocalInfile {
		sink.Status("restoring local_infile")
		if err := s.setLocalInfile(ctx, false); err != nil {
			return JobResult{}, err
		}
	}

	return JobResult{
		Kind:         JobRestore,
		Target:       db,
		LogPath:      logPath,
		Duration:     s.now().Sub(startedAt),
		Summary:      summary,
		Status:       "restore complete",
		StartedMySQL: startedService,
	}, nil
}

func StreamHandlerFor(logFile *os.File, sink OutputSink, summary map[string]string) execx.StreamHandler {
	writeLine := func(prefix, line string) {
		if line == "" {
			return
		}
		if logFile != nil {
			_, _ = logFile.WriteString(line + "\n")
		}
		if sink != nil {
			sink.LogLine(line)
			sink.Status(line)
		}
		if metricLine.MatchString(line) {
			summary[prefix+fmt.Sprintf("%02d", len(summary)+1)] = line
		}
	}

	return execx.StreamHandler{
		Stdout: func(line string) { writeLine("stdout_", line) },
		Stderr: func(line string) { writeLine("stderr_", line) },
	}
}

func (s *Service) localInfileOn(ctx context.Context) (bool, error) {
	result, err := s.runner.Run(ctx, s.mysqlCommand("-N", "-B", "-e", "SHOW GLOBAL VARIABLES LIKE 'local_infile'"))
	if err != nil {
		return false, err
	}
	output := strings.TrimSpace(result.Stdout)
	return strings.HasSuffix(strings.ToUpper(output), "ON"), nil
}

func (s *Service) setLocalInfile(ctx context.Context, enabled bool) error {
	value := "OFF"
	if enabled {
		value = "ON"
	}
	_, err := s.runner.Run(ctx, s.mysqlCommand("-e", "SET GLOBAL local_infile = "+value))
	return err
}

func ValidateSnapshotDir(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("snapshot %q is not readable: %w", filepath.Base(path), err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("snapshot %q is empty", filepath.Base(path))
	}

	hasDumpMarker := false
	for _, entry := range entries {
		name := entry.Name()
		if name == "@.json" || name == "metadata.json" || strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".zst") {
			hasDumpMarker = true
			break
		}
	}
	if !hasDumpMarker {
		return fmt.Errorf("snapshot %q does not look like a mysqlsh dump", filepath.Base(path))
	}
	return nil
}

func ValidDBName(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '$' || r == '.':
		default:
			return false
		}
	}
	return true
}

func (s *Service) writeTempJS(pattern, contents string) (string, error) {
	file, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	defer file.Close()

	if _, err := file.WriteString(contents); err != nil {
		return "", err
	}
	return file.Name(), nil
}

var execLookPath = func(file string) (string, error) {
	return execLookPathImpl(file)
}
