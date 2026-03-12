package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/taylor/dbgold/internal/core"
)

type runtimeFlags struct {
	yes   bool
	noTUI bool
	debug bool
}

type settingsView struct {
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

type plainSink struct{}

func (plainSink) Status(string) {}

func (plainSink) LogLine(line string) {
	fmt.Println(line)
}

func NewRootCommand(ctx context.Context, svc *core.Service) *cobra.Command {
	flags := runtimeFlags{
		yes:   svc.Config().Yes,
		noTUI: svc.Config().NoTUI,
		debug: svc.Config().Debug,
	}

	rootCmd := &cobra.Command{
		Use:   "dbgold",
		Short: "Local MySQL golden snapshot manager",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if flags.noTUI || !isInteractive() {
				if svc.Config().NeedsOnboarding() {
					fmt.Printf("First-run setup is incomplete. Review settings with: dbgold settings --no-tui\nConfig path: %s\n\n", svc.Config().ConfigPath)
				}
				return renderPlainDashboard(ctx, svc)
			}
			if svc.Config().NeedsOnboarding() {
				return runTUI(ctx, svc, launchOptions{mode: screenOnboarding})
			}
			return runTUI(ctx, svc, launchOptions{mode: screenDashboard})
		},
	}

	rootCmd.PersistentFlags().BoolVar(&flags.yes, "yes", flags.yes, "skip confirmations for destructive or service-start actions")
	rootCmd.PersistentFlags().BoolVar(&flags.noTUI, "no-tui", flags.noTUI, "force plain terminal output")
	rootCmd.PersistentFlags().BoolVar(&flags.debug, "debug", flags.debug, "enable verbose structured logs")

	rootCmd.AddCommand(newSnapshotCommand(ctx, svc, &flags))
	rootCmd.AddCommand(newRestoreCommand(ctx, svc, &flags))
	rootCmd.AddCommand(newListCommand(ctx, svc))
	rootCmd.AddCommand(newDoctorCommand(ctx, svc))
	rootCmd.AddCommand(newSettingsCommand(ctx, svc, &flags))
	rootCmd.AddCommand(newCompletionCommand(rootCmd))

	return rootCmd
}

func newSnapshotCommand(ctx context.Context, svc *core.Service, flags *runtimeFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "snapshot [db]",
		Short: "Take a new snapshot for a database",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return runActionEntry(ctx, svc, *flags, core.JobSnapshot, name)
		},
	}
}

func newRestoreCommand(ctx context.Context, svc *core.Service, flags *runtimeFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "restore [db]",
		Short: "Restore a snapshot into the local MySQL instance",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			return runActionEntry(ctx, svc, *flags, core.JobRestore, name)
		},
	}
}

func newListCommand(ctx context.Context, svc *core.Service) *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List live databases or snapshots",
	}

	listCmd.AddCommand(&cobra.Command{
		Use:   "dbs",
		Short: "List live databases",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbs, err := svc.DiscoverDatabases(ctx)
			if err != nil {
				return err
			}
			if len(dbs) == 0 {
				fmt.Println("No live databases found.")
				return nil
			}
			fmt.Printf("%-32s %-10s %-12s\n", "DATABASE", "TABLES", "SIZE")
			for _, db := range dbs {
				fmt.Printf("%-32s %-10d %-12s\n", db.Name, db.TableCount, core.FormatBytes(db.SizeBytes))
			}
			return nil
		},
	})

	listCmd.AddCommand(&cobra.Command{
		Use:   "snapshots",
		Short: "List stored snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			snapshots, err := svc.DiscoverSnapshots()
			if err != nil {
				return err
			}
			if len(snapshots) == 0 {
				fmt.Println("No snapshots found.")
				return nil
			}
			fmt.Printf("%-32s %-17s %-12s\n", "SNAPSHOT", "UPDATED", "SIZE")
			for _, snapshot := range snapshots {
				fmt.Printf("%-32s %-17s %-12s\n", snapshot.Name, core.FormatTime(snapshot.UpdatedAt), core.FormatBytes(snapshot.SizeBytes))
			}
			return nil
		},
	})

	return listCmd
}

func newDoctorCommand(ctx context.Context, svc *core.Service) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local MySQL and snapshot prerequisites",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := svc.Doctor(ctx)
			if err != nil {
				return err
			}
			fmt.Printf("MySQL reachable: %t\n", report.MySQLReachable)
			fmt.Printf("MySQL service:   %s\n", report.MySQLService)
			fmt.Printf("MySQL socket:    %s\n", report.MySQLSocket)
			fmt.Printf("MySQL version:   %s\n", blankDash(report.MySQLVersion))
			fmt.Printf("Snapshot root:   %s\n", report.SnapshotRoot)
			fmt.Printf("Log root:        %s\n", report.LogRoot)
			if len(report.MissingCommands) > 0 {
				fmt.Printf("Missing tools:   %s\n", strings.Join(report.MissingCommands, ", "))
			}
			for _, warning := range report.Warnings {
				fmt.Printf("Warning:         %s\n", warning)
			}
			return nil
		},
	}
}

func newSettingsCommand(ctx context.Context, svc *core.Service, flags *runtimeFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "settings",
		Short: "View or edit persisted dbgold settings",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.noTUI || !isInteractive() {
				if svc.Config().NeedsOnboarding() {
					fmt.Println("First-run setup has not been completed yet. The defaults below come from the legacy snapshot and restore scripts.")
				}
				view := settingsView{
					Onboarded:                  svc.Config().Onboarded,
					SnapshotRoot:               svc.Config().SnapshotRoot,
					LogRoot:                    svc.Config().LogRoot,
					MySQLSHStateHome:           svc.Config().MySQLSHStateHome,
					MySQLHeartbeatInterval:     int(svc.Config().MySQLHeartbeatInterval / time.Second),
					MySQLStartTimeout:          int(svc.Config().MySQLStartTimeout / time.Second),
					MySQLURI:                   svc.Config().MySQLURI,
					MySQLSocket:                svc.Config().MySQLSocket,
					MySQLUser:                  svc.Config().MySQLUser,
					MySQLPassword:              svc.Config().MySQLPassword,
					MySQLLoginPath:             svc.Config().MySQLLoginPath,
					MySQLHost:                  svc.Config().MySQLHost,
					MySQLPort:                  svc.Config().MySQLPort,
					MySQLService:               svc.Config().MySQLService,
					MySQLAssumeEmptyPassword:   svc.Config().MySQLAssumeEmptyPassword,
					MySQLShellThreads:          svc.Config().MySQLShellThreads,
					MySQLCompression:           svc.Config().MySQLCompression,
					MySQLBytesPerChunk:         svc.Config().MySQLBytesPerChunk,
					MySQLDeferIndexes:          svc.Config().MySQLDeferIndexes,
					MySQLSkipBinlog:            svc.Config().MySQLSkipBinlog,
					MySQLAutoEnableLocalInfile: svc.Config().MySQLAutoEnableLocalInfile,
					Yes:                        svc.Config().Yes,
					NoTUI:                      svc.Config().NoTUI,
					Debug:                      svc.Config().Debug,
				}
				data, err := json.MarshalIndent(view, "", "  ")
				if err != nil {
					return err
				}
				fmt.Printf("Config path: %s\n", svc.Config().ConfigPath)
				fmt.Println(string(data))
				return nil
			}
			mode := screenSettings
			if svc.Config().NeedsOnboarding() {
				mode = screenOnboarding
			}
			return runTUI(ctx, svc, launchOptions{mode: mode})
		},
	}
}

func newCompletionCommand(rootCmd *cobra.Command) *cobra.Command {
	completionCmd := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
	}

	completionCmd.AddCommand(&cobra.Command{
		Use:   "zsh",
		Short: "Generate zsh completion",
		RunE: func(cmd *cobra.Command, args []string) error {
			return rootCmd.GenZshCompletion(os.Stdout)
		},
	})

	return completionCmd
}

func runActionEntry(ctx context.Context, svc *core.Service, flags runtimeFlags, kind core.JobKind, query string) error {
	if flags.noTUI || !isInteractive() {
		if strings.TrimSpace(query) == "" {
			return fmt.Errorf("%s requires an exact database name in non-interactive mode", kind)
		}
		return runPlainAction(ctx, svc, flags, kind, query)
	}

	if resolved, exact := resolveActionTarget(ctx, svc, kind, query); exact {
		return runPlainAction(ctx, svc, flags, kind, resolved)
	}

	screen := screenSnapshotPicker
	if kind == core.JobRestore {
		screen = screenRestorePicker
	}
	return runTUI(ctx, svc, launchOptions{
		mode:         screen,
		initialQuery: query,
		yes:          flags.yes,
	})
}

func runPlainAction(ctx context.Context, svc *core.Service, flags runtimeFlags, kind core.JobKind, db string) error {
	resolved, exact := resolveActionTarget(ctx, svc, kind, db)
	if !exact {
		return fmt.Errorf("%q does not exactly match an available %s target", db, kind)
	}

	runOpts := core.RunOptions{
		Yes:   flags.yes,
		Debug: flags.debug,
	}

	if !flags.yes {
		if kind == core.JobRestore {
			ok, err := confirmText(fmt.Sprintf("Restore snapshot %q into local MySQL and replace the database?", resolved))
			if err != nil {
				return err
			}
			if !ok {
				return errors.New("restore cancelled")
			}
		}

		startedPrompt, err := maybeApproveServiceStart(ctx, svc)
		if err != nil {
			return err
		}
		runOpts.ApproveStartService = startedPrompt
	} else {
		runOpts.ApproveStartService = true
	}

	sink := &plainSink{}
	var (
		result core.JobResult
		err    error
	)
	switch kind {
	case core.JobSnapshot:
		result, err = svc.RunSnapshot(ctx, resolved, runOpts, sink)
	case core.JobRestore:
		result, err = svc.RunRestore(ctx, resolved, runOpts, sink)
	default:
		return fmt.Errorf("unsupported action %q", kind)
	}
	if err != nil {
		return err
	}

	fmt.Printf("%s finished for %s in %s\n", strings.Title(string(kind)), result.Target, result.Duration.Round(timeSecond))
	fmt.Printf("Log: %s\n", result.LogPath)
	for _, line := range orderedSummary(result.Summary) {
		fmt.Printf("Summary: %s\n", line)
	}
	return nil
}

func maybeApproveServiceStart(ctx context.Context, svc *core.Service) (bool, error) {
	report, err := svc.Doctor(ctx)
	if err != nil {
		return false, err
	}
	if report.MySQLReachable {
		return false, nil
	}
	ok, err := confirmText("Local MySQL is down. Start mysql@8.0 with Homebrew?")
	if err != nil {
		return false, err
	}
	if !ok {
		return false, errors.New("mysql start declined")
	}
	return true, nil
}

func confirmText(prompt string) (bool, error) {
	fmt.Printf("%s [y/N]: ", prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

func resolveActionTarget(ctx context.Context, svc *core.Service, kind core.JobKind, query string) (string, bool) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", false
	}

	if kind == core.JobSnapshot {
		dbs, err := svc.DiscoverDatabases(ctx)
		if err != nil {
			return "", false
		}
		names := make([]string, 0, len(dbs))
		for _, db := range dbs {
			names = append(names, db.Name)
		}
		return core.ResolveExactName(names, query)
	}

	snapshots, err := svc.DiscoverSnapshots()
	if err != nil {
		return "", false
	}
	names := make([]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		names = append(names, snapshot.Name)
	}
	return core.ResolveExactName(names, query)
}

func renderPlainDashboard(ctx context.Context, svc *core.Service) error {
	report, err := svc.Doctor(ctx)
	if err != nil {
		return err
	}
	dbs, err := svc.DiscoverDatabases(ctx)
	dbErr := err
	snapshots, err := svc.DiscoverSnapshots()
	snapErr := err

	fmt.Printf("MySQL:          %s\n", map[bool]string{true: "up", false: "down"}[report.MySQLReachable])
	fmt.Printf("Snapshot root:  %s\n", svc.SnapshotRoot())
	if snapErr != nil {
		fmt.Printf("Snapshots:      unavailable (%v)\n", snapErr)
	} else {
		fmt.Printf("Snapshots:      %d\n", len(snapshots))
	}
	if dbErr != nil {
		fmt.Printf("Live databases: unavailable (%v)\n", dbErr)
	} else {
		fmt.Printf("Live databases: %d\n", len(dbs))
	}
	return nil
}

func blankDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
