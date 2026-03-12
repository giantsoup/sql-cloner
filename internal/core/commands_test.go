package core

import (
	"context"
	"io"
	"testing"

	"github.com/taylor/dbgold/internal/execx"
)

func TestCommandBuilders(t *testing.T) {
	svc := NewService(Config{
		MySQLHost:          defaultMySQLHost,
		MySQLPort:          defaultMySQLPort,
		MySQLSocket:        "/tmp/mysql.sock",
		MySQLUser:          "root",
		MySQLPassword:      "secret",
		MySQLLoginPath:     "local-root",
		MySQLShellThreads:  8,
		MySQLCompression:   "zstd",
		MySQLBytesPerChunk: "32M",
		MySQLDeferIndexes:  "all",
		MySQLSkipBinlog:    true,
	}, noopRunner{}, NewLogger(io.Discard, false))

	mysqlCmd := svc.mysqlCommand("-N", "-B", "-e", "SELECT 1")
	if mysqlCmd.Name != "mysql" {
		t.Fatalf("unexpected mysql command name %q", mysqlCmd.Name)
	}
	if !contains(mysqlCmd.Args, "--socket=/tmp/mysql.sock") {
		t.Fatalf("expected socket arg, got %#v", mysqlCmd.Args)
	}
	if !contains(mysqlCmd.Args, "--host=127.0.0.1") {
		t.Fatalf("expected host arg, got %#v", mysqlCmd.Args)
	}
	if !contains(mysqlCmd.Args, "--login-path=local-root") {
		t.Fatalf("expected login-path arg, got %#v", mysqlCmd.Args)
	}
	if !contains(mysqlCmd.Args, "--password=secret") {
		t.Fatalf("expected password arg, got %#v", mysqlCmd.Args)
	}

	script := svc.dumpScript("appdb", "/tmp/dump")
	if !stringsContains(script, `util.dumpSchemas([databaseName], snapshotDir, options)`) {
		t.Fatalf("unexpected dump script %q", script)
	}
	if !stringsContains(script, `const threads = 8;`) {
		t.Fatalf("expected threads in dump script %q", script)
	}
	if !stringsContains(script, `options.bytesPerChunk = bytesPerChunk`) {
		t.Fatalf("expected bytesPerChunk support in dump script %q", script)
	}

	load := svc.loadScript("appdb", "/tmp/dump")
	if !stringsContains(load, `util.loadDump(snapshotDir, options)`) {
		t.Fatalf("unexpected load script %q", load)
	}
	if !stringsContains(load, `const skipBinlog = true;`) {
		t.Fatalf("expected skipBinlog in load script %q", load)
	}
}

type noopRunner struct{}

func (noopRunner) Run(context.Context, execx.Command) (execx.Result, error) {
	return execx.Result{}, nil
}

func (noopRunner) Stream(context.Context, execx.Command, execx.StreamHandler) (execx.Result, error) {
	return execx.Result{}, nil
}
