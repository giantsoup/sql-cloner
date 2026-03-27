package core

import (
	"fmt"
	"strconv"

	"github.com/taylor/dbgold/internal/execx"
)

func (s *Service) mysqlCommand(extraArgs ...string) execx.Command {
	args := s.mysqlAuthArgs()
	args = append(args, extraArgs...)
	return execx.Command{Name: "mysql", Args: args}
}

func (s *Service) mysqlAdminCommand(extraArgs ...string) execx.Command {
	args := s.mysqlAuthArgs()
	args = append(args, extraArgs...)
	return execx.Command{Name: "mysqladmin", Args: args}
}

func (s *Service) brewCommand(extraArgs ...string) execx.Command {
	return execx.Command{Name: "brew", Args: extraArgs}
}

func (s *Service) mysqlShellCommand(jsPath string) execx.Command {
	args := s.mysqlShellAuthArgs()
	args = append(args, "--file", jsPath)
	cmd := execx.Command{Name: "mysqlsh", Args: args}
	if s.cfg.MySQLSHStateHome != "" {
		cmd.Env = append(cmd.Env, "MYSQLSH_USER_CONFIG_HOME="+s.cfg.MySQLSHStateHome)
	}
	return cmd
}

func (s *Service) mysqlAuthArgs() []string {
	args := []string{"--host=" + s.cfg.MySQLHost, "--port=" + strconv.Itoa(s.cfg.MySQLPort), "--user=" + s.cfg.MySQLUser}
	if s.cfg.MySQLLoginPath != "" {
		args = append(args, "--login-path="+s.cfg.MySQLLoginPath)
	}
	if s.cfg.MySQLSocket != "" {
		args = append(args, "--socket="+s.cfg.MySQLSocket)
	}
	if s.cfg.MySQLPassword != "" {
		args = append(args, "--password="+s.cfg.MySQLPassword)
	} else if s.shouldAssumeEmptyPassword() {
		args = append(args, "--password=")
	}
	return args
}

func (s *Service) mysqlShellAuthArgs() []string {
	args := []string{"--mysql", "--js", "--quiet-start=2"}
	if s.cfg.MySQLURI != "" {
		return append(args, "--uri="+s.cfg.MySQLURI)
	}
	if s.cfg.MySQLLoginPath != "" {
		args = append(args, "--login-path="+s.cfg.MySQLLoginPath)
	}
	args = append(args,
		"--host="+s.cfg.MySQLHost,
		"--port="+strconv.Itoa(s.cfg.MySQLPort),
		"--user="+s.cfg.MySQLUser,
	)
	if s.cfg.MySQLSocket != "" {
		args = append(args, "--socket="+s.cfg.MySQLSocket)
	}
	if s.shouldAssumeEmptyPassword() {
		args = append(args, "--no-password")
	}
	return args
}

func (s *Service) dumpScript(db, outputDir string) string {
	script := fmt.Sprintf(`const databaseName = %q;
const snapshotDir = %q;
const threads = %d;
const compression = %q;
const bytesPerChunk = %q;

if (typeof session === 'undefined' || session === null) {
  throw new Error('No MySQL Shell session is available. Set MYSQLSH_URI, MYSQL_LOGIN_PATH, or host/user/socket variables.');
}

const databaseCheck = session.runSql(
  "SELECT SCHEMA_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = '" + databaseName + "'"
);

if (databaseCheck.fetchOne() === null) {
  throw new Error("Database '" + databaseName + "' does not exist on the connected MySQL server.");
}

const options = {
  threads,
  consistent: true,
  showProgress: true,
};

if (compression.length > 0 && compression !== "none") {
  options.compression = compression;
}

if (bytesPerChunk.length > 0) {
  options.bytesPerChunk = bytesPerChunk;
}

println("Preparing parallel dump for '" + databaseName + "' into '" + snapshotDir + "'.");
util.dumpSchemas([databaseName], snapshotDir, options);
println("Dump completed for '" + databaseName + "'.");
`, db, outputDir, s.cfg.MySQLShellThreads, s.cfg.MySQLCompression, s.cfg.MySQLBytesPerChunk)
	return script
}

func (s *Service) loadScript(db, inputDir string) string {
	return fmt.Sprintf(`const databaseName = %q;
const snapshotDir = %q;
const threads = %d;
const deferTableIndexes = %q;
const skipBinlog = %t;

if (typeof session === 'undefined' || session === null) {
  throw new Error('No MySQL Shell session is available. Set MYSQLSH_URI, MYSQL_LOGIN_PATH, or host/user/socket variables.');
}

try {
  session.runSql('USE mysql');
} catch (error) {
  println("Could not switch to mysql schema before drop: " + error.message);
}

const tick = String.fromCharCode(96);
session.runSql('DROP DATABASE IF EXISTS ' + tick + databaseName + tick);

const options = {
  threads,
  showProgress: true,
  resetProgress: true,
  deferTableIndexes,
  skipBinlog,
};

println("Loading dump for '" + databaseName + "' from '" + snapshotDir + "'.");
util.loadDump(snapshotDir, options);

const postCheck = session.runSql(
  "SELECT SCHEMA_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = '" + databaseName + "'"
);

if (postCheck.fetchOne() === null) {
  throw new Error("Restore completed without recreating database '" + databaseName + "'.");
}

println("Restore completed for '" + databaseName + "'.");
`, db, inputDir, s.cfg.MySQLShellThreads, s.cfg.MySQLDeferIndexes, s.cfg.MySQLSkipBinlog)
}

func (s *Service) shouldAssumeEmptyPassword() bool {
	return s.cfg.MySQLAssumeEmptyPassword && s.cfg.MySQLLoginPath == "" && s.cfg.MySQLPassword == ""
}
