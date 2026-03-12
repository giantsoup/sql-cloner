package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSnapshotRoot = "/opt/homebrew/var/db_snapshots/mysqlsh"
	defaultMySQLUser    = "root"
	defaultMySQLHost    = "127.0.0.1"
	defaultMySQLPort    = 3306
	defaultMySQLService = "mysql@8.0"
)

type Config struct {
	ConfigPath                 string        `json:"-"`
	Onboarded                  bool          `json:"onboarded"`
	SnapshotRoot               string        `json:"snapshot_root"`
	LogRoot                    string        `json:"log_root"`
	MySQLSHStateHome           string        `json:"mysqlsh_state_home"`
	MySQLHeartbeatInterval     time.Duration `json:"mysql_heartbeat_interval"`
	MySQLStartTimeout          time.Duration `json:"mysql_start_timeout"`
	MySQLURI                   string        `json:"mysql_uri"`
	MySQLSocket                string        `json:"mysql_socket"`
	MySQLUser                  string        `json:"mysql_user"`
	MySQLPassword              string        `json:"mysql_password"`
	MySQLLoginPath             string        `json:"mysql_login_path"`
	MySQLHost                  string        `json:"mysql_host"`
	MySQLPort                  int           `json:"mysql_port"`
	MySQLService               string        `json:"mysql_service"`
	MySQLAssumeEmptyPassword   bool          `json:"mysql_assume_empty_password"`
	MySQLShellThreads          int           `json:"mysqlsh_threads"`
	MySQLCompression           string        `json:"mysqlsh_compression"`
	MySQLBytesPerChunk         string        `json:"mysqlsh_bytes_per_chunk"`
	MySQLDeferIndexes          string        `json:"mysqlsh_defer_table_indexes"`
	MySQLSkipBinlog            bool          `json:"mysqlsh_skip_binlog"`
	MySQLAutoEnableLocalInfile bool          `json:"mysqlsh_auto_enable_local_infile"`
	Yes                        bool          `json:"yes"`
	NoTUI                      bool          `json:"no_tui"`
	Debug                      bool          `json:"debug"`
}

type settingsFile struct {
	Onboarded                  bool   `json:"onboarded"`
	SnapshotRoot               string `json:"snapshot_root"`
	LogRoot                    string `json:"log_root"`
	MySQLSHStateHome           string `json:"mysqlsh_state_home"`
	MySQLHeartbeatInterval     int    `json:"mysql_heartbeat_interval_seconds"`
	MySQLStartTimeout          int    `json:"mysql_start_timeout_seconds"`
	MySQLURI                   string `json:"mysql_uri"`
	MySQLSocket                string `json:"mysql_socket"`
	MySQLUser                  string `json:"mysql_user"`
	MySQLPassword              string `json:"mysql_password"`
	MySQLLoginPath             string `json:"mysql_login_path"`
	MySQLHost                  string `json:"mysql_host"`
	MySQLPort                  int    `json:"mysql_port"`
	MySQLService               string `json:"mysql_service"`
	MySQLAssumeEmptyPassword   bool   `json:"mysql_assume_empty_password"`
	MySQLShellThreads          int    `json:"mysqlsh_threads"`
	MySQLCompression           string `json:"mysqlsh_compression"`
	MySQLBytesPerChunk         string `json:"mysqlsh_bytes_per_chunk"`
	MySQLDeferIndexes          string `json:"mysqlsh_defer_table_indexes"`
	MySQLSkipBinlog            bool   `json:"mysqlsh_skip_binlog"`
	MySQLAutoEnableLocalInfile bool   `json:"mysqlsh_auto_enable_local_infile"`
	Yes                        bool   `json:"yes"`
	NoTUI                      bool   `json:"no_tui"`
	Debug                      bool   `json:"debug"`
}

func LoadConfigFromEnv() Config {
	cfg := DefaultConfig()
	cfg.ConfigPath = resolveConfigPath()

	if saved, err := LoadSettings(cfg.ConfigPath); err == nil {
		cfg = mergeConfig(cfg, saved)
		cfg.ConfigPath = resolveConfigPath()
	}

	applyEnvOverrides(&cfg)
	if cfg.LogRoot == "" {
		cfg.LogRoot = filepath.Join(cfg.SnapshotRoot, "_logs")
	}
	return cfg
}

func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		SnapshotRoot:               defaultSnapshotRoot,
		LogRoot:                    filepath.Join(defaultSnapshotRoot, "_logs"),
		MySQLSHStateHome:           filepath.Join(home, ".mysqlsh-tools"),
		MySQLHeartbeatInterval:     5 * time.Second,
		MySQLStartTimeout:          45 * time.Second,
		MySQLUser:                  defaultMySQLUser,
		MySQLHost:                  defaultMySQLHost,
		MySQLPort:                  defaultMySQLPort,
		MySQLService:               defaultMySQLService,
		MySQLAssumeEmptyPassword:   true,
		MySQLShellThreads:          defaultThreads(),
		MySQLCompression:           "none",
		MySQLBytesPerChunk:         "",
		MySQLDeferIndexes:          "all",
		MySQLSkipBinlog:            false,
		MySQLAutoEnableLocalInfile: true,
	}
}

func LoadSettings(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var stored settingsFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return Config{}, err
	}
	cfg := Config{
		Onboarded:                  stored.Onboarded,
		SnapshotRoot:               stored.SnapshotRoot,
		LogRoot:                    stored.LogRoot,
		MySQLSHStateHome:           stored.MySQLSHStateHome,
		MySQLHeartbeatInterval:     time.Duration(stored.MySQLHeartbeatInterval) * time.Second,
		MySQLStartTimeout:          time.Duration(stored.MySQLStartTimeout) * time.Second,
		MySQLURI:                   stored.MySQLURI,
		MySQLSocket:                stored.MySQLSocket,
		MySQLUser:                  stored.MySQLUser,
		MySQLPassword:              stored.MySQLPassword,
		MySQLLoginPath:             stored.MySQLLoginPath,
		MySQLHost:                  stored.MySQLHost,
		MySQLPort:                  stored.MySQLPort,
		MySQLService:               stored.MySQLService,
		MySQLAssumeEmptyPassword:   stored.MySQLAssumeEmptyPassword,
		MySQLShellThreads:          stored.MySQLShellThreads,
		MySQLCompression:           stored.MySQLCompression,
		MySQLBytesPerChunk:         stored.MySQLBytesPerChunk,
		MySQLDeferIndexes:          stored.MySQLDeferIndexes,
		MySQLSkipBinlog:            stored.MySQLSkipBinlog,
		MySQLAutoEnableLocalInfile: stored.MySQLAutoEnableLocalInfile,
		Yes:                        stored.Yes,
		NoTUI:                      stored.NoTUI,
		Debug:                      stored.Debug,
	}
	cfg.ConfigPath = path
	return cfg, nil
}

func SaveSettings(cfg Config) error {
	path := cfg.ConfigPath
	if strings.TrimSpace(path) == "" {
		path = resolveConfigPath()
	}
	cfg.ConfigPath = path

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	stored := settingsFile{
		Onboarded:                  cfg.Onboarded,
		SnapshotRoot:               cfg.SnapshotRoot,
		LogRoot:                    cfg.LogRoot,
		MySQLSHStateHome:           cfg.MySQLSHStateHome,
		MySQLHeartbeatInterval:     int(cfg.MySQLHeartbeatInterval / time.Second),
		MySQLStartTimeout:          int(cfg.MySQLStartTimeout / time.Second),
		MySQLURI:                   cfg.MySQLURI,
		MySQLSocket:                cfg.MySQLSocket,
		MySQLUser:                  cfg.MySQLUser,
		MySQLPassword:              cfg.MySQLPassword,
		MySQLLoginPath:             cfg.MySQLLoginPath,
		MySQLHost:                  cfg.MySQLHost,
		MySQLPort:                  cfg.MySQLPort,
		MySQLService:               cfg.MySQLService,
		MySQLAssumeEmptyPassword:   cfg.MySQLAssumeEmptyPassword,
		MySQLShellThreads:          cfg.MySQLShellThreads,
		MySQLCompression:           cfg.MySQLCompression,
		MySQLBytesPerChunk:         cfg.MySQLBytesPerChunk,
		MySQLDeferIndexes:          cfg.MySQLDeferIndexes,
		MySQLSkipBinlog:            cfg.MySQLSkipBinlog,
		MySQLAutoEnableLocalInfile: cfg.MySQLAutoEnableLocalInfile,
		Yes:                        cfg.Yes,
		NoTUI:                      cfg.NoTUI,
		Debug:                      cfg.Debug,
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

func resolveConfigPath() string {
	if path := strings.TrimSpace(os.Getenv("DBGOLD_CONFIG_PATH")); path != "" {
		return path
	}

	configHome, err := os.UserConfigDir()
	if err == nil && configHome != "" {
		return filepath.Join(configHome, "dbgold", "settings.json")
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "settings.json"
	}
	return filepath.Join(home, ".config", "dbgold", "settings.json")
}

func mergeConfig(base, override Config) Config {
	base.ConfigPath = firstNonEmpty(override.ConfigPath, base.ConfigPath)
	base.Onboarded = override.Onboarded
	base.SnapshotRoot = firstNonEmpty(override.SnapshotRoot, base.SnapshotRoot)
	base.LogRoot = firstNonEmpty(override.LogRoot, base.LogRoot)
	base.MySQLSHStateHome = firstNonEmpty(override.MySQLSHStateHome, base.MySQLSHStateHome)
	base.MySQLHeartbeatInterval = firstDuration(override.MySQLHeartbeatInterval, base.MySQLHeartbeatInterval)
	base.MySQLStartTimeout = firstDuration(override.MySQLStartTimeout, base.MySQLStartTimeout)
	base.MySQLURI = firstNonEmpty(override.MySQLURI, base.MySQLURI)
	base.MySQLSocket = firstNonEmpty(override.MySQLSocket, base.MySQLSocket)
	base.MySQLUser = firstNonEmpty(override.MySQLUser, base.MySQLUser)
	base.MySQLPassword = firstNonEmpty(override.MySQLPassword, base.MySQLPassword)
	base.MySQLLoginPath = firstNonEmpty(override.MySQLLoginPath, base.MySQLLoginPath)
	base.MySQLHost = firstNonEmpty(override.MySQLHost, base.MySQLHost)
	if override.MySQLPort != 0 {
		base.MySQLPort = override.MySQLPort
	}
	base.MySQLService = firstNonEmpty(override.MySQLService, base.MySQLService)
	if override.MySQLShellThreads != 0 {
		base.MySQLShellThreads = override.MySQLShellThreads
	}
	base.MySQLCompression = firstNonEmpty(override.MySQLCompression, base.MySQLCompression)
	if override.MySQLBytesPerChunk != "" || base.MySQLBytesPerChunk == "" {
		base.MySQLBytesPerChunk = override.MySQLBytesPerChunk
	}
	base.MySQLDeferIndexes = firstNonEmpty(override.MySQLDeferIndexes, base.MySQLDeferIndexes)
	base.MySQLAssumeEmptyPassword = override.MySQLAssumeEmptyPassword
	base.MySQLSkipBinlog = override.MySQLSkipBinlog
	base.MySQLAutoEnableLocalInfile = override.MySQLAutoEnableLocalInfile
	base.Yes = override.Yes
	base.NoTUI = override.NoTUI
	base.Debug = override.Debug
	return base
}

func applyEnvOverrides(cfg *Config) {
	if value, ok := envString("MYSQL_SNAPSHOT_ROOT", "DBSNAP_SNAPSHOT_ROOT", "DBSNAP_ROOT", "DBSNAP_DIR", "DBGOLD_SNAPSHOT_ROOT"); ok {
		cfg.SnapshotRoot = value
	}
	if value, ok := envString("MYSQL_LOG_ROOT", "DBSNAP_LOG_ROOT", "DBGOLD_LOG_ROOT"); ok {
		cfg.LogRoot = value
	}
	if value, ok := envString("MYSQLSH_USER_CONFIG_HOME"); ok {
		cfg.MySQLSHStateHome = value
	}
	if value, ok := envInt("MYSQLSH_HEARTBEAT_INTERVAL"); ok {
		cfg.MySQLHeartbeatInterval = time.Duration(value) * time.Second
	}
	if value, ok := envInt("MYSQL_START_TIMEOUT"); ok {
		cfg.MySQLStartTimeout = time.Duration(value) * time.Second
	}
	if value, ok := envString("MYSQLSH_URI"); ok {
		cfg.MySQLURI = value
	}
	if value, ok := envString("MYSQL_SOCKET", "MYSQL_UNIX_PORT", "DBSNAP_MYSQL_SOCKET", "DBGOLD_MYSQL_SOCKET"); ok {
		cfg.MySQLSocket = value
	}
	if value, ok := envString("MYSQL_USER", "DBSNAP_MYSQL_USER", "DBGOLD_MYSQL_USER"); ok {
		cfg.MySQLUser = value
	}
	if value, ok := envString("MYSQL_PASSWORD", "DBSNAP_MYSQL_PASSWORD", "MYSQL_PWD", "DBGOLD_MYSQL_PASSWORD"); ok {
		cfg.MySQLPassword = value
	}
	if value, ok := envString("MYSQL_LOGIN_PATH", "DBSNAP_MYSQL_LOGIN_PATH", "DBGOLD_MYSQL_LOGIN_PATH"); ok {
		cfg.MySQLLoginPath = value
	}
	if value, ok := envString("MYSQL_HOST", "DBSNAP_MYSQL_HOST", "DBGOLD_MYSQL_HOST"); ok {
		cfg.MySQLHost = value
	}
	if value, ok := envInt("MYSQL_PORT", "DBSNAP_MYSQL_PORT", "MYSQL_TCP_PORT", "DBGOLD_MYSQL_PORT"); ok {
		cfg.MySQLPort = value
	}
	if value, ok := envString("MYSQL_SERVICE_NAME", "DBSNAP_MYSQL_SERVICE", "DBGOLD_MYSQL_SERVICE"); ok {
		cfg.MySQLService = value
	}
	if value, ok := envBool("MYSQL_ASSUME_EMPTY_PASSWORD"); ok {
		cfg.MySQLAssumeEmptyPassword = value
	}
	if value, ok := envInt("MYSQLSH_THREADS", "DBSNAP_MYSQLSH_THREADS", "DBGOLD_MYSQLSH_THREADS"); ok {
		cfg.MySQLShellThreads = value
	}
	if value, ok := envString("MYSQLSH_COMPRESSION", "DBSNAP_MYSQLSH_COMPRESSION", "DBGOLD_MYSQLSH_COMPRESSION"); ok {
		cfg.MySQLCompression = value
	}
	if value, ok := envString("MYSQLSH_BYTES_PER_CHUNK", "DBSNAP_MYSQLSH_CHUNK_SIZE", "DBGOLD_MYSQLSH_CHUNK_SIZE"); ok {
		cfg.MySQLBytesPerChunk = value
	}
	if value, ok := envString("MYSQLSH_DEFER_TABLE_INDEXES", "DBSNAP_MYSQLSH_DEFER_INDEXES", "DBGOLD_MYSQLSH_DEFER_INDEXES"); ok {
		cfg.MySQLDeferIndexes = value
	}
	if value, ok := envBool("MYSQLSH_SKIP_BINLOG", "DBSNAP_MYSQLSH_SKIP_BINLOG", "DBGOLD_MYSQLSH_SKIP_BINLOG"); ok {
		cfg.MySQLSkipBinlog = value
	}
	if value, ok := envBool("MYSQLSH_AUTO_ENABLE_LOCAL_INFILE"); ok {
		cfg.MySQLAutoEnableLocalInfile = value
	}
	if value, ok := envBool("DBGOLD_YES"); ok {
		cfg.Yes = value
	}
	if value, ok := envBool("DBGOLD_NO_TUI"); ok {
		cfg.NoTUI = value
	}
	if value, ok := envBool("DBGOLD_DEBUG"); ok {
		cfg.Debug = value
	}
}

func (c Config) NeedsOnboarding() bool {
	if !c.Onboarded {
		return true
	}
	_, err := os.Stat(c.ConfigPath)
	return err != nil
}

func defaultThreads() int {
	threads := runtime.NumCPU()
	if threads > 16 {
		return 16
	}
	if threads < 4 {
		return 4
	}
	return threads
}

func envString(keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			return value, true
		}
	}
	return "", false
}

func envInt(keys ...string) (int, bool) {
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func envBool(keys ...string) (bool, bool) {
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "1", "true", "yes", "y", "on":
			return true, true
		case "0", "false", "no", "n", "off":
			return false, true
		}
	}
	return false, false
}

func ValidateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.SnapshotRoot) == "" {
		return errors.New("snapshot root is required")
	}
	if strings.TrimSpace(cfg.MySQLUser) == "" {
		return errors.New("mysql user is required")
	}
	if strings.TrimSpace(cfg.MySQLHost) == "" && strings.TrimSpace(cfg.MySQLSocket) == "" && strings.TrimSpace(cfg.MySQLURI) == "" {
		return errors.New("configure either mysql host, socket, or mysqlsh uri")
	}
	if cfg.MySQLPort <= 0 && strings.TrimSpace(cfg.MySQLURI) == "" {
		return errors.New("mysql port must be positive")
	}
	if cfg.MySQLShellThreads <= 0 {
		return errors.New("mysqlsh threads must be positive")
	}
	if cfg.MySQLHeartbeatInterval <= 0 {
		return errors.New("heartbeat interval must be positive")
	}
	if cfg.MySQLStartTimeout <= 0 {
		return errors.New("mysql start timeout must be positive")
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
