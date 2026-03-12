package core

import "testing"

func TestLoadConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("DBGOLD_CONFIG_PATH", t.TempDir()+"/settings.json")
	t.Setenv("MYSQL_SNAPSHOT_ROOT", "")
	t.Setenv("MYSQL_LOG_ROOT", "")
	t.Setenv("MYSQL_SOCKET", "")
	t.Setenv("MYSQL_USER", "")
	t.Setenv("MYSQL_HOST", "")

	cfg := LoadConfigFromEnv()

	if cfg.SnapshotRoot != defaultSnapshotRoot {
		t.Fatalf("expected default snapshot root, got %q", cfg.SnapshotRoot)
	}
	if cfg.LogRoot != defaultSnapshotRoot+"/_logs" {
		t.Fatalf("expected default log root, got %q", cfg.LogRoot)
	}
	if cfg.MySQLHost != defaultMySQLHost {
		t.Fatalf("expected default mysql host, got %q", cfg.MySQLHost)
	}
	if cfg.MySQLUser != defaultMySQLUser {
		t.Fatalf("expected default mysql user, got %q", cfg.MySQLUser)
	}
	if cfg.MySQLShellThreads < 4 || cfg.MySQLShellThreads > 16 {
		t.Fatalf("expected bounded default threads, got %d", cfg.MySQLShellThreads)
	}
	if cfg.MySQLCompression != "none" {
		t.Fatalf("expected default compression none, got %q", cfg.MySQLCompression)
	}
	if !cfg.MySQLAssumeEmptyPassword {
		t.Fatal("expected blank-password assumption by default")
	}
}

func TestLoadConfigFromEnvAliases(t *testing.T) {
	t.Setenv("DBGOLD_CONFIG_PATH", t.TempDir()+"/settings.json")
	t.Setenv("MYSQL_SNAPSHOT_ROOT", "/tmp/snapshots")
	t.Setenv("MYSQL_LOG_ROOT", "/tmp/snapshots/_logs")
	t.Setenv("MYSQL_SOCKET", "/tmp/mysql.sock")
	t.Setenv("MYSQL_USER", "tester")
	t.Setenv("MYSQL_PWD", "secret")
	t.Setenv("MYSQL_LOGIN_PATH", "local-root")
	t.Setenv("MYSQLSH_THREADS", "8")
	t.Setenv("MYSQLSH_SKIP_BINLOG", "false")

	cfg := LoadConfigFromEnv()

	if cfg.SnapshotRoot != "/tmp/snapshots" {
		t.Fatalf("unexpected snapshot root %q", cfg.SnapshotRoot)
	}
	if cfg.LogRoot != "/tmp/snapshots/_logs" {
		t.Fatalf("unexpected log root %q", cfg.LogRoot)
	}
	if cfg.MySQLSocket != "/tmp/mysql.sock" {
		t.Fatalf("unexpected socket %q", cfg.MySQLSocket)
	}
	if cfg.MySQLUser != "tester" {
		t.Fatalf("unexpected user %q", cfg.MySQLUser)
	}
	if cfg.MySQLPassword != "secret" {
		t.Fatalf("unexpected password %q", cfg.MySQLPassword)
	}
	if cfg.MySQLLoginPath != "local-root" {
		t.Fatalf("unexpected login path %q", cfg.MySQLLoginPath)
	}
	if cfg.MySQLShellThreads != 8 {
		t.Fatalf("unexpected threads %d", cfg.MySQLShellThreads)
	}
	if cfg.MySQLSkipBinlog {
		t.Fatal("expected skip binlog to be false")
	}
}

func TestSaveAndLoadSettings(t *testing.T) {
	path := t.TempDir() + "/settings.json"
	cfg := DefaultConfig()
	cfg.ConfigPath = path
	cfg.Onboarded = true
	cfg.SnapshotRoot = "/tmp/custom-snaps"
	cfg.MySQLHost = "db.internal"
	cfg.MySQLPort = 4406

	if err := SaveSettings(cfg); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	loaded, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}

	if loaded.SnapshotRoot != "/tmp/custom-snaps" {
		t.Fatalf("unexpected snapshot root %q", loaded.SnapshotRoot)
	}
	if loaded.MySQLHost != "db.internal" || loaded.MySQLPort != 4406 {
		t.Fatalf("unexpected mysql target %#v", loaded)
	}
	if !loaded.Onboarded {
		t.Fatal("expected onboarded flag to persist")
	}
}

func TestNeedsOnboarding(t *testing.T) {
	path := t.TempDir() + "/settings.json"
	cfg := DefaultConfig()
	cfg.ConfigPath = path
	if !cfg.NeedsOnboarding() {
		t.Fatal("expected onboarding when config file does not exist")
	}

	cfg.Onboarded = true
	if err := SaveSettings(cfg); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if cfg.NeedsOnboarding() {
		t.Fatal("did not expect onboarding after saved settings")
	}
}
