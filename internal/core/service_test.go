package core

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/taylor/dbgold/internal/execx"
)

func TestEnsureMySQLAlreadyUp(t *testing.T) {
	runner := &fakeRunner{
		runFunc: func(ctx context.Context, cmd execx.Command) (execx.Result, error) {
			switch cmd.Name {
			case "mysqladmin":
				return execx.Result{Stdout: "mysqld is alive\n"}, nil
			case "mysql":
				return execx.Result{Stdout: "8.0.39\n"}, nil
			default:
				return execx.Result{}, nil
			}
		},
	}
	svc := newTestService(t, runner)

	started, err := svc.EnsureMySQL(context.Background(), RunOptions{})
	if err != nil {
		t.Fatalf("ensure mysql: %v", err)
	}
	if started {
		t.Fatal("expected mysql not to be started")
	}
	if runner.commandCount("brew") != 0 {
		t.Fatal("did not expect brew start command")
	}
}

func TestEnsureMySQLStartsService(t *testing.T) {
	pings := 0
	runner := &fakeRunner{
		runFunc: func(ctx context.Context, cmd execx.Command) (execx.Result, error) {
			switch cmd.Name {
			case "mysqladmin":
				pings++
				if pings < 3 {
					return execx.Result{}, errors.New("down")
				}
				return execx.Result{Stdout: "mysqld is alive\n"}, nil
			case "mysql":
				return execx.Result{Stdout: "8.0.39\n"}, nil
			case "brew":
				return execx.Result{}, nil
			default:
				return execx.Result{}, nil
			}
		},
	}
	svc := newTestService(t, runner)

	started, err := svc.EnsureMySQL(context.Background(), RunOptions{ApproveStartService: true})
	if err != nil {
		t.Fatalf("ensure mysql: %v", err)
	}
	if !started {
		t.Fatal("expected mysql service to be started")
	}
	if runner.commandCount("brew") != 1 {
		t.Fatalf("expected one brew start command, got %d", runner.commandCount("brew"))
	}
}

func TestRunSnapshotReplacesOnlyAfterTempSuccess(t *testing.T) {
	root := t.TempDir()
	logRoot := filepath.Join(root, "_logs")
	finalDir := filepath.Join(root, "appdb")
	mustMkdir(t, finalDir)
	if err := os.WriteFile(filepath.Join(finalDir, "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write old snapshot: %v", err)
	}

	runner := &fakeRunner{
		runFunc: mysqlUpRunner,
		streamFunc: func(ctx context.Context, cmd execx.Command, handler execx.StreamHandler) (execx.Result, error) {
			if cmd.Name != "mysqlsh" {
				return execx.Result{}, nil
			}
			if err := writeDumpFromScript(cmd.Args, "new snapshot"); err != nil {
				return execx.Result{}, err
			}
			handler.Stdout("rows dumped: 10")
			return execx.Result{}, nil
		},
	}
	svc := newConfiguredTestService(t, runner, Config{
		SnapshotRoot: root,
		LogRoot:      logRoot,
		MySQLHost:    defaultMySQLHost,
		MySQLPort:    defaultMySQLPort,
		MySQLUser:    defaultMySQLUser,
		MySQLService: defaultMySQLService,
	})

	result, err := svc.RunSnapshot(context.Background(), "appdb", RunOptions{ApproveStartService: true}, sinkRecorder{})
	if err != nil {
		t.Fatalf("run snapshot: %v", err)
	}
	if result.Target != "appdb" {
		t.Fatalf("unexpected result target %q", result.Target)
	}

	data, err := os.ReadFile(filepath.Join(finalDir, "dump.txt"))
	if err != nil {
		t.Fatalf("read new dump: %v", err)
	}
	if string(data) != "new snapshot" {
		t.Fatalf("unexpected dump contents %q", string(data))
	}
	if _, err := os.Stat(filepath.Join(finalDir, "old.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected old snapshot file removed, got err=%v", err)
	}
}

func TestRunSnapshotFailureKeepsPreviousSnapshot(t *testing.T) {
	root := t.TempDir()
	logRoot := filepath.Join(root, "_logs")
	finalDir := filepath.Join(root, "appdb")
	mustMkdir(t, finalDir)
	if err := os.WriteFile(filepath.Join(finalDir, "old.txt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write old snapshot: %v", err)
	}

	runner := &fakeRunner{
		runFunc: mysqlUpRunner,
		streamFunc: func(ctx context.Context, cmd execx.Command, handler execx.StreamHandler) (execx.Result, error) {
			return execx.Result{}, errors.New("mysqlsh dump failed")
		},
	}
	svc := newConfiguredTestService(t, runner, Config{
		SnapshotRoot: root,
		LogRoot:      logRoot,
		MySQLHost:    defaultMySQLHost,
		MySQLPort:    defaultMySQLPort,
		MySQLUser:    defaultMySQLUser,
		MySQLService: defaultMySQLService,
	})

	if _, err := svc.RunSnapshot(context.Background(), "appdb", RunOptions{ApproveStartService: true}, sinkRecorder{}); err == nil {
		t.Fatal("expected snapshot failure")
	}

	data, err := os.ReadFile(filepath.Join(finalDir, "old.txt"))
	if err != nil {
		t.Fatalf("read preserved snapshot: %v", err)
	}
	if string(data) != "old" {
		t.Fatalf("unexpected preserved contents %q", string(data))
	}
}

func TestRunRestoreInvalidSnapshotFailsBeforeDrop(t *testing.T) {
	root := t.TempDir()
	logRoot := filepath.Join(root, "_logs")
	mustMkdir(t, filepath.Join(root, "broken"))

	runner := &fakeRunner{runFunc: mysqlUpRunner}
	svc := newConfiguredTestService(t, runner, Config{
		SnapshotRoot: root,
		LogRoot:      logRoot,
		MySQLHost:    defaultMySQLHost,
		MySQLPort:    defaultMySQLPort,
		MySQLUser:    defaultMySQLUser,
		MySQLService: defaultMySQLService,
	})

	if _, err := svc.RunRestore(context.Background(), "broken", RunOptions{ApproveStartService: true}, sinkRecorder{}); err == nil {
		t.Fatal("expected restore preflight failure")
	}
	if runner.containsCommand("DROP DATABASE") {
		t.Fatal("did not expect drop database command on invalid snapshot")
	}
}

func TestRunRestoreEnablesAndRestoresLocalInfile(t *testing.T) {
	root := t.TempDir()
	logRoot := filepath.Join(root, "_logs")
	dumpDir := filepath.Join(root, "appdb")
	mustMkdir(t, dumpDir)
	if err := os.WriteFile(filepath.Join(dumpDir, "@.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write dump marker: %v", err)
	}

	var commands []string
	runner := &fakeRunner{
		runFunc: func(ctx context.Context, cmd execx.Command) (execx.Result, error) {
			commands = append(commands, cmd.String())
			switch {
			case cmd.Name == "mysqladmin":
				return execx.Result{Stdout: "mysqld is alive\n"}, nil
			case strings.Contains(cmd.String(), "SELECT VERSION()"):
				return execx.Result{Stdout: "8.0.39\n"}, nil
			case strings.Contains(cmd.String(), "SHOW GLOBAL VARIABLES LIKE 'local_infile'"):
				return execx.Result{Stdout: "local_infile\tOFF\n"}, nil
			default:
				return execx.Result{}, nil
			}
		},
		streamFunc: func(ctx context.Context, cmd execx.Command, handler execx.StreamHandler) (execx.Result, error) {
			handler.Stdout("rows imported: 10")
			return execx.Result{}, nil
		},
	}
	svc := newConfiguredTestService(t, runner, Config{
		SnapshotRoot: root,
		LogRoot:      logRoot,
		MySQLHost:    defaultMySQLHost,
		MySQLPort:    defaultMySQLPort,
		MySQLUser:    defaultMySQLUser,
		MySQLService: defaultMySQLService,
	})

	if _, err := svc.RunRestore(context.Background(), "appdb", RunOptions{ApproveStartService: true}, sinkRecorder{}); err != nil {
		t.Fatalf("run restore: %v", err)
	}

	joined := strings.Join(commands, "\n")
	if !strings.Contains(joined, "SET GLOBAL local_infile = ON") {
		t.Fatal("expected local_infile enable command")
	}
	if !strings.Contains(joined, "SET GLOBAL local_infile = OFF") {
		t.Fatal("expected local_infile reset command")
	}
}

func TestRunRestoreFailureStillResetsLocalInfile(t *testing.T) {
	root := t.TempDir()
	logRoot := filepath.Join(root, "_logs")
	dumpDir := filepath.Join(root, "appdb")
	mustMkdir(t, dumpDir)
	if err := os.WriteFile(filepath.Join(dumpDir, "@.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write dump marker: %v", err)
	}

	var commands []string
	runner := &fakeRunner{
		runFunc: func(ctx context.Context, cmd execx.Command) (execx.Result, error) {
			commands = append(commands, cmd.String())
			switch {
			case cmd.Name == "mysqladmin":
				return execx.Result{Stdout: "mysqld is alive\n"}, nil
			case strings.Contains(cmd.String(), "SELECT VERSION()"):
				return execx.Result{Stdout: "8.0.39\n"}, nil
			case strings.Contains(cmd.String(), "SHOW GLOBAL VARIABLES LIKE 'local_infile'"):
				return execx.Result{Stdout: "local_infile\tOFF\n"}, nil
			default:
				return execx.Result{}, nil
			}
		},
		streamFunc: func(ctx context.Context, cmd execx.Command, handler execx.StreamHandler) (execx.Result, error) {
			return execx.Result{}, errors.New("load failed")
		},
	}
	svc := newConfiguredTestService(t, runner, Config{
		SnapshotRoot: root,
		LogRoot:      logRoot,
		MySQLHost:    defaultMySQLHost,
		MySQLPort:    defaultMySQLPort,
		MySQLUser:    defaultMySQLUser,
		MySQLService: defaultMySQLService,
	})

	if _, err := svc.RunRestore(context.Background(), "appdb", RunOptions{ApproveStartService: true}, sinkRecorder{}); err == nil {
		t.Fatal("expected restore failure")
	}

	joined := strings.Join(commands, "\n")
	if !strings.Contains(joined, "SET GLOBAL local_infile = OFF") {
		t.Fatal("expected best-effort local_infile reset")
	}
}

func TestStreamHandlerSanitizesControlSequences(t *testing.T) {
	var gotLines []string
	sink := collectSink{
		status: func(line string) { gotLines = append(gotLines, "status:"+line) },
		log:    func(line string) { gotLines = append(gotLines, "log:"+line) },
	}
	summary := map[string]string{}

	handler := StreamHandlerFor(nil, sink, summary)
	handler.Stdout("\x1b[32m100%\x1b[0m rows\rprogress")
	handler.Stdout("plain output")

	if len(gotLines) < 4 {
		t.Fatalf("expected sanitized output callbacks, got %#v", gotLines)
	}
	if gotLines[0] != "log:100% rowsprogress" || gotLines[1] != "status:100% rowsprogress" {
		t.Fatalf("unexpected sanitized control output: %#v", gotLines)
	}
	if summary["stdout_01"] != "100% rowsprogress" {
		t.Fatalf("unexpected summary contents: %#v", summary)
	}
}

type fakeRunner struct {
	runFunc    func(context.Context, execx.Command) (execx.Result, error)
	streamFunc func(context.Context, execx.Command, execx.StreamHandler) (execx.Result, error)
	commands   []execx.Command
}

func (f *fakeRunner) Run(ctx context.Context, cmd execx.Command) (execx.Result, error) {
	f.commands = append(f.commands, cmd)
	if f.runFunc == nil {
		return execx.Result{}, nil
	}
	return f.runFunc(ctx, cmd)
}

func (f *fakeRunner) Stream(ctx context.Context, cmd execx.Command, handler execx.StreamHandler) (execx.Result, error) {
	f.commands = append(f.commands, cmd)
	if f.streamFunc == nil {
		return execx.Result{}, nil
	}
	return f.streamFunc(ctx, cmd, handler)
}

func (f *fakeRunner) commandCount(name string) int {
	count := 0
	for _, cmd := range f.commands {
		if cmd.Name == name {
			count++
		}
	}
	return count
}

func (f *fakeRunner) containsCommand(fragment string) bool {
	for _, cmd := range f.commands {
		if strings.Contains(cmd.String(), fragment) {
			return true
		}
	}
	return false
}

func newTestService(t *testing.T, runner execx.Runner) *Service {
	t.Helper()
	return newConfiguredTestService(t, runner, Config{
		SnapshotRoot: t.TempDir(),
		LogRoot:      filepath.Join(t.TempDir(), "_logs"),
		MySQLHost:    defaultMySQLHost,
		MySQLPort:    defaultMySQLPort,
		MySQLUser:    defaultMySQLUser,
		MySQLService: defaultMySQLService,
	})
}

func newConfiguredTestService(t *testing.T, runner execx.Runner, cfg Config) *Service {
	t.Helper()
	if cfg.MySQLShellThreads == 0 {
		cfg.MySQLShellThreads = 4
	}
	if cfg.MySQLCompression == "" {
		cfg.MySQLCompression = "none"
	}
	if cfg.MySQLDeferIndexes == "" {
		cfg.MySQLDeferIndexes = "all"
	}
	if cfg.MySQLHeartbeatInterval == 0 {
		cfg.MySQLHeartbeatInterval = 5 * time.Second
	}
	if cfg.MySQLStartTimeout == 0 {
		cfg.MySQLStartTimeout = 2 * time.Second
	}
	cfg.MySQLAutoEnableLocalInfile = true
	svc := NewService(cfg, runner, NewLogger(io.Discard, false))
	svc.now = func() time.Time { return time.Date(2026, 3, 12, 20, 0, 0, 0, time.UTC) }
	return svc
}

func mysqlUpRunner(ctx context.Context, cmd execx.Command) (execx.Result, error) {
	switch {
	case cmd.Name == "mysqladmin":
		return execx.Result{Stdout: "mysqld is alive\n"}, nil
	case strings.Contains(cmd.String(), "SELECT VERSION()"):
		return execx.Result{Stdout: "8.0.39\n"}, nil
	case strings.Contains(cmd.String(), "SHOW GLOBAL VARIABLES LIKE 'local_infile'"):
		return execx.Result{Stdout: "local_infile\tON\n"}, nil
	default:
		return execx.Result{}, nil
	}
}

func writeDumpFromScript(args []string, contents string) error {
	scriptPath := scriptPathFromArgs(args)
	data, err := os.ReadFile(scriptPath)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`const snapshotDir = "([^"]+)"`)
	matches := re.FindStringSubmatch(string(data))
	if len(matches) != 2 {
		return errors.New("dump output path not found in script")
	}
	outputDir := matches[1]
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "@.json"), []byte("{}"), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outputDir, "dump.txt"), []byte(contents), 0o644)
}

func scriptPathFromArgs(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--file" {
			return args[i+1]
		}
	}
	return ""
}

type sinkRecorder struct{}

func (sinkRecorder) Status(string)  {}
func (sinkRecorder) LogLine(string) {}

type collectSink struct {
	status func(string)
	log    func(string)
}

func (s collectSink) Status(value string) {
	if s.status != nil {
		s.status(value)
	}
}

func (s collectSink) LogLine(value string) {
	if s.log != nil {
		s.log(value)
	}
}
