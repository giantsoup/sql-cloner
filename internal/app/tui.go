package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/taylor/dbgold/internal/core"
)

const timeSecond = time.Second

type screen string

const (
	screenDashboard      screen = "dashboard"
	screenSnapshotPicker screen = "snapshot-picker"
	screenRestorePicker  screen = "restore-picker"
	screenConfirm        screen = "confirm"
	screenRunning        screen = "running"
	screenResult         screen = "result"
	screenDoctor         screen = "doctor"
	screenSettings       screen = "settings"
	screenOnboarding     screen = "onboarding"
)

type launchOptions struct {
	mode         screen
	initialQuery string
	yes          bool
}

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Filter   key.Binding
	Enter    key.Binding
	Back     key.Binding
	Refresh  key.Binding
	Snap     key.Binding
	Restore  key.Binding
	Doctor   key.Binding
	Settings key.Binding
	Quit     key.Binding
	Cancel   key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "move")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "move")),
		Filter:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Refresh:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Snap:     key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "snapshot")),
		Restore:  key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "restore")),
		Doctor:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "doctor")),
		Settings: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "settings")),
		Quit:     key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		Cancel:   key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "cancel job")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Filter, k.Enter, k.Refresh, k.Snap, k.Restore, k.Settings, k.Back, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Up, k.Down, k.Filter, k.Enter, k.Back}, {k.Refresh, k.Snap, k.Restore, k.Doctor, k.Settings, k.Quit, k.Cancel}}
}

type dashboardLoadedMsg struct {
	dbs       []core.Database
	snapshots []core.Snapshot
	doctor    core.DoctorReport
	err       error
}

type confirmDoneMsg struct {
	ok bool
}

type settingsSubmitMsg struct{}

type settingsSavedMsg struct {
	cfg core.Config
	err error
}

type jobEventMsg struct {
	line   string
	status string
	result *core.JobResult
	err    error
}

type elapsedMsg time.Time

type tuiSink struct {
	ch chan jobEventMsg
}

func (s tuiSink) Status(value string) {
	select {
	case s.ch <- jobEventMsg{status: value}:
	default:
	}
}

func (s tuiSink) LogLine(value string) {
	select {
	case s.ch <- jobEventMsg{line: value}:
	default:
	}
}

type confirmState struct {
	reason      string
	description string
	action      core.JobKind
	target      string
	cancelRun   bool
	startMySQL  bool
}

type settingsValues struct {
	SnapshotRoot               string
	LogRoot                    string
	MySQLSHStateHome           string
	MySQLStartTimeout          string
	MySQLHeartbeatInterval     string
	MySQLURI                   string
	MySQLHost                  string
	MySQLPort                  string
	MySQLSocket                string
	MySQLUser                  string
	MySQLPassword              string
	MySQLLoginPath             string
	MySQLService               string
	MySQLAssumeEmptyPassword   bool
	MySQLShellThreads          string
	MySQLCompression           string
	MySQLBytesPerChunk         string
	MySQLDeferIndexes          string
	MySQLSkipBinlog            bool
	MySQLAutoEnableLocalInfile bool
}

type model struct {
	ctx            context.Context
	service        *core.Service
	screen         screen
	width          int
	height         int
	keys           keyMap
	help           help.Model
	filter         textinput.Model
	dbTable        table.Model
	snapshotTable  table.Model
	logViewport    viewport.Model
	spin           spinner.Model
	styles         styles
	doctor         core.DoctorReport
	dbs            []core.Database
	snapshots      []core.Snapshot
	lastErr        error
	lastResult     *core.JobResult
	jobEvents      <-chan jobEventMsg
	cancelJob      context.CancelFunc
	jobStartedAt   time.Time
	jobLines       []string
	jobStatus      string
	jobSummary     []string
	confirm        *huh.Form
	confirmValue   bool
	confirmState   confirmState
	settingsForm   *huh.Form
	settingsDraft  settingsValues
	onboarding     bool
	initialQuery   string
	initialYes     bool
	dashboardFocus int
}

func runTUI(ctx context.Context, svc *core.Service, opts launchOptions) error {
	m := newModel(ctx, svc, opts)
	p := tea.NewProgram(m)
	_, err := p.Run()
	if err != nil && !errors.Is(err, tea.ErrInterrupted) {
		return err
	}
	return nil
}

func newModel(ctx context.Context, svc *core.Service, opts launchOptions) model {
	filter := textinput.New()
	filter.Prompt = "filter> "
	filter.Placeholder = "type to narrow"

	dbTable := newDBTable()
	snapshotTable := newSnapshotTable()
	logViewport := viewport.New()
	logViewport.SoftWrap = true
	logViewport.FillHeight = true

	spin := spinner.New(spinner.WithSpinner(spinner.MiniDot))

	styles := newStyles()

	return model{
		ctx:           ctx,
		service:       svc,
		screen:        opts.mode,
		keys:          defaultKeys(),
		help:          help.New(),
		filter:        filter,
		dbTable:       dbTable,
		snapshotTable: snapshotTable,
		logViewport:   logViewport,
		spin:          spin,
		styles:        styles,
		initialQuery:  opts.initialQuery,
		initialYes:    opts.yes,
	}
}

func (m model) Init() tea.Cmd {
	if m.initialQuery != "" {
		m.filter.SetValue(m.initialQuery)
	}
	if m.screen == screenSettings || m.screen == screenOnboarding {
		m.openSettings(m.screen == screenOnboarding)
	}
	return tea.Batch(loadDashboardCmd(m.ctx, m.service), spinTickCmd(m.spin))
}

func loadDashboardCmd(ctx context.Context, svc *core.Service) tea.Cmd {
	return func() tea.Msg {
		dbs, dbErr := svc.DiscoverDatabases(ctx)
		snapshots, snapErr := svc.DiscoverSnapshots()
		doctor, docErr := svc.Doctor(ctx)
		err := firstError(dbErr, snapErr, docErr)
		return dashboardLoadedMsg{dbs: dbs, snapshots: snapshots, doctor: doctor, err: err}
	}
}

func spinTickCmd(s spinner.Model) tea.Cmd {
	return func() tea.Msg {
		return s.Tick()
	}
}

func elapsedTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return elapsedMsg(t)
	})
}

func waitForJobEventCmd(ch <-chan jobEventMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return jobEventMsg{}
		}
		return msg
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
	case dashboardLoadedMsg:
		m.lastErr = msg.err
		if msg.err == nil {
			m.doctor = msg.doctor
			m.dbs = msg.dbs
			m.snapshots = msg.snapshots
			m.syncTables()
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		cmds = append(cmds, cmd)
	case elapsedMsg:
		if m.screen == screenRunning {
			cmds = append(cmds, elapsedTickCmd())
		}
	case jobEventMsg:
		if msg.line != "" {
			m.jobLines = append(m.jobLines, msg.line)
			m.logViewport.SetContent(strings.Join(m.jobLines, "\n"))
			m.logViewport.GotoBottom()
		}
		if msg.status != "" {
			m.jobStatus = msg.status
		}
		if msg.result != nil || msg.err != nil {
			m.cancelJob = nil
			m.jobEvents = nil
			m.lastResult = msg.result
			m.lastErr = msg.err
			if msg.result != nil {
				m.jobSummary = orderedSummary(msg.result.Summary)
			}
			m.screen = screenResult
		} else if m.jobEvents != nil {
			cmds = append(cmds, waitForJobEventCmd(m.jobEvents))
		}
	case confirmDoneMsg:
		return m.handleConfirmDone(msg)
	case settingsSubmitMsg:
		return m.handleSettingsSubmit()
	case settingsSavedMsg:
		if msg.err != nil {
			m.lastErr = msg.err
			m.screen = screenResult
			return m, nil
		}
		m.service = m.service.WithConfig(msg.cfg)
		m.settingsForm = nil
		m.lastErr = nil
		m.screen = screenDashboard
		cmds = append(cmds, loadDashboardCmd(m.ctx, m.service))
	}

	switch m.screen {
	case screenConfirm:
		if m.confirm != nil {
			var cmd tea.Cmd
			updated, formCmd := m.confirm.Update(msg)
			if form, ok := updated.(*huh.Form); ok {
				m.confirm = form
			}
			cmd = formCmd
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	case screenSettings:
		if m.settingsForm != nil {
			updated, formCmd := m.settingsForm.Update(msg)
			if form, ok := updated.(*huh.Form); ok {
				m.settingsForm = form
			}
			cmds = append(cmds, formCmd)
		}
		return m, tea.Batch(cmds...)
	case screenOnboarding:
		if m.settingsForm != nil {
			updated, formCmd := m.settingsForm.Update(msg)
			if form, ok := updated.(*huh.Form); ok {
				m.settingsForm = form
			}
			cmds = append(cmds, formCmd)
		}
		return m, tea.Batch(cmds...)
	case screenRunning:
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok && key.Matches(keyMsg, m.keys.Cancel) {
			if m.cancelJob != nil {
				m.openConfirm(confirmState{
					reason:      "Cancel running job?",
					description: "The current mysqlsh work will be interrupted after confirmation.",
					cancelRun:   true,
				})
			}
			return m, tea.Batch(cmds...)
		}
		var cmd tea.Cmd
		m.logViewport, cmd = m.logViewport.Update(msg)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	if m.filter.Focused() {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok && key.Matches(keyMsg, m.keys.Back) {
			if m.filter.Value() == "" {
				m.filter.Blur()
			} else {
				m.filter.SetValue("")
				m.applyFilter()
			}
			return m, tea.Batch(cmds...)
		}
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.applyFilter()
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, m.keys.Quit) && m.screen != screenRunning:
			return m, tea.Quit
		case key.Matches(keyMsg, m.keys.Refresh):
			cmds = append(cmds, loadDashboardCmd(m.ctx, m.service))
		case key.Matches(keyMsg, m.keys.Filter) && (m.screen == screenSnapshotPicker || m.screen == screenRestorePicker):
			cmds = append(cmds, m.filter.Focus())
		case key.Matches(keyMsg, m.keys.Back):
			if m.screen == screenSnapshotPicker || m.screen == screenRestorePicker || m.screen == screenDoctor || m.screen == screenResult || m.screen == screenSettings {
				m.screen = screenDashboard
				m.lastErr = nil
			}
		case key.Matches(keyMsg, m.keys.Snap) && m.screen == screenDashboard:
			m.screen = screenSnapshotPicker
		case key.Matches(keyMsg, m.keys.Restore) && m.screen == screenDashboard:
			m.screen = screenRestorePicker
		case key.Matches(keyMsg, m.keys.Doctor) && m.screen == screenDashboard:
			m.screen = screenDoctor
		case key.Matches(keyMsg, m.keys.Settings) && m.screen == screenDashboard:
			m.openSettings(false)
		case key.Matches(keyMsg, m.keys.Enter):
			return m.handleEnter()
		}
	}

	switch m.screen {
	case screenDashboard:
		return m.updateDashboard(msg, cmds)
	case screenSnapshotPicker:
		return m.updateSnapshotPicker(msg, cmds)
	case screenRestorePicker:
		return m.updateRestorePicker(msg, cmds)
	default:
		return m, tea.Batch(cmds...)
	}
}

func (m model) updateDashboard(msg tea.Msg, cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(keyMsg, m.keys.Up):
			if m.dashboardFocus == 0 {
				m.dbTable.MoveUp(1)
			} else {
				m.snapshotTable.MoveUp(1)
			}
		case key.Matches(keyMsg, m.keys.Down):
			if m.dashboardFocus == 0 {
				m.dbTable.MoveDown(1)
			} else {
				m.snapshotTable.MoveDown(1)
			}
		case keyMsg.String() == "tab":
			m.dashboardFocus = (m.dashboardFocus + 1) % 2
		}
	}
	return m, tea.Batch(cmds...)
}

func (m model) updateSnapshotPicker(msg tea.Msg, cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.dbTable, cmd = m.dbTable.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m model) updateRestorePicker(msg tea.Msg, cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.snapshotTable, cmd = m.snapshotTable.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenDashboard:
		if m.dashboardFocus == 0 {
			m.screen = screenSnapshotPicker
		} else {
			m.screen = screenRestorePicker
		}
	case screenSnapshotPicker:
		row := m.dbTable.SelectedRow()
		if len(row) == 0 {
			return m, nil
		}
		target := row[0]
		if m.initialYes {
			return m.startJob(core.JobSnapshot, target, true)
		}
		m.openConfirm(confirmState{
			reason:      "Create snapshot?",
			description: fmt.Sprintf("A successful dump will replace the existing snapshot for %s.", target),
			action:      core.JobSnapshot,
			target:      target,
		})
	case screenRestorePicker:
		row := m.snapshotTable.SelectedRow()
		if len(row) == 0 {
			return m, nil
		}
		target := row[0]
		m.openConfirm(confirmState{
			reason:      "Restore snapshot?",
			description: fmt.Sprintf("The local database %s will be dropped and recreated before load.", target),
			action:      core.JobRestore,
			target:      target,
		})
	case screenResult:
		m.screen = screenDashboard
	case screenSettings:
		return m, nil
	case screenOnboarding:
		return m, nil
	}
	return m, nil
}

func (m *model) openConfirm(state confirmState) {
	m.confirmState = state
	m.confirmValue = false
	confirm := huh.NewConfirm().
		Key("confirm").
		Title(state.reason).
		Description(state.description).
		Affirmative("Yes").
		Negative("No").
		Value(&m.confirmValue)

	form := huh.NewForm(huh.NewGroup(confirm))
	form.SubmitCmd = func() tea.Msg {
		return confirmDoneMsg{ok: m.confirmValue}
	}
	form.CancelCmd = func() tea.Msg {
		return confirmDoneMsg{ok: false}
	}
	form.WithShowHelp(false)
	form.WithTheme(huh.ThemeFunc(huh.ThemeCharm))
	m.confirm = form
	m.screen = screenConfirm
}

func (m *model) openSettings(onboarding bool) {
	m.onboarding = onboarding
	cfg := m.service.Config()
	m.settingsDraft = settingsValues{
		SnapshotRoot:               cfg.SnapshotRoot,
		LogRoot:                    cfg.LogRoot,
		MySQLSHStateHome:           cfg.MySQLSHStateHome,
		MySQLStartTimeout:          strconv.Itoa(int(cfg.MySQLStartTimeout / time.Second)),
		MySQLHeartbeatInterval:     strconv.Itoa(int(cfg.MySQLHeartbeatInterval / time.Second)),
		MySQLURI:                   cfg.MySQLURI,
		MySQLHost:                  cfg.MySQLHost,
		MySQLPort:                  strconv.Itoa(cfg.MySQLPort),
		MySQLSocket:                cfg.MySQLSocket,
		MySQLUser:                  cfg.MySQLUser,
		MySQLPassword:              cfg.MySQLPassword,
		MySQLLoginPath:             cfg.MySQLLoginPath,
		MySQLService:               cfg.MySQLService,
		MySQLAssumeEmptyPassword:   cfg.MySQLAssumeEmptyPassword,
		MySQLShellThreads:          strconv.Itoa(cfg.MySQLShellThreads),
		MySQLCompression:           cfg.MySQLCompression,
		MySQLBytesPerChunk:         cfg.MySQLBytesPerChunk,
		MySQLDeferIndexes:          cfg.MySQLDeferIndexes,
		MySQLSkipBinlog:            cfg.MySQLSkipBinlog,
		MySQLAutoEnableLocalInfile: cfg.MySQLAutoEnableLocalInfile,
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title(func() string {
					if onboarding {
						return "Welcome to dbgold"
					}
					return "Settings"
				}()).
				Description(func() string {
					if onboarding {
						return "This quick setup uses the legacy shell-script defaults as starting values. Review them, adjust anything your environment needs, and press Enter through the form to save."
					}
					return "Update saved settings for this machine. Environment variables still override these values at runtime."
				}()),
		).Title("Overview"),
		huh.NewGroup(
			huh.NewInput().Key("snapshot_root").Title("Snapshot root").Value(&m.settingsDraft.SnapshotRoot).Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Key("log_root").Title("Log root").Value(&m.settingsDraft.LogRoot).Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Key("mysqlsh_state_home").Title("MySQL Shell state home").Value(&m.settingsDraft.MySQLSHStateHome).Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Key("mysql_service").Title("MySQL service").Value(&m.settingsDraft.MySQLService).Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Key("mysql_start_timeout").Title("Start timeout (seconds)").Value(&m.settingsDraft.MySQLStartTimeout).Validate(validatePositiveInt),
			huh.NewInput().Key("mysql_heartbeat_interval").Title("Heartbeat interval (seconds)").Value(&m.settingsDraft.MySQLHeartbeatInterval).Validate(validatePositiveInt),
		).Title("Paths and service"),
		huh.NewGroup(
			huh.NewInput().Key("mysql_uri").Title("MySQL Shell URI").Description("Optional. Leave blank to use host/user/socket fields.").Value(&m.settingsDraft.MySQLURI),
			huh.NewInput().Key("mysql_host").Title("MySQL host").Value(&m.settingsDraft.MySQLHost),
			huh.NewInput().Key("mysql_port").Title("MySQL port").Value(&m.settingsDraft.MySQLPort).Validate(validatePositiveInt),
			huh.NewInput().Key("mysql_socket").Title("MySQL socket").Value(&m.settingsDraft.MySQLSocket),
			huh.NewInput().Key("mysql_user").Title("MySQL user").Value(&m.settingsDraft.MySQLUser).Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Key("mysql_login_path").Title("MySQL login path").Value(&m.settingsDraft.MySQLLoginPath),
			huh.NewInput().Key("mysql_password").Title("MySQL password").Password(true).Value(&m.settingsDraft.MySQLPassword),
			huh.NewConfirm().Key("mysql_assume_empty_password").Title("Assume empty password when no login-path or password is configured?").Value(&m.settingsDraft.MySQLAssumeEmptyPassword),
		).Title("Connection"),
		huh.NewGroup(
			huh.NewInput().Key("mysqlsh_threads").Title("MySQL Shell threads").Value(&m.settingsDraft.MySQLShellThreads).Validate(validatePositiveInt),
			huh.NewInput().Key("mysqlsh_compression").Title("Dump compression").Value(&m.settingsDraft.MySQLCompression).Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Key("mysqlsh_bytes_per_chunk").Title("Bytes per chunk").Description("Optional, e.g. 128M").Value(&m.settingsDraft.MySQLBytesPerChunk),
			huh.NewInput().Key("mysqlsh_defer_table_indexes").Title("Defer table indexes").Value(&m.settingsDraft.MySQLDeferIndexes).Validate(huh.ValidateNotEmpty()),
			huh.NewConfirm().Key("mysqlsh_skip_binlog").Title("Skip binlog on restore?").Value(&m.settingsDraft.MySQLSkipBinlog),
			huh.NewConfirm().Key("mysqlsh_auto_enable_local_infile").Title("Auto-enable local_infile when needed?").Value(&m.settingsDraft.MySQLAutoEnableLocalInfile),
		).Title("Dump and restore"),
	)
	form.SubmitCmd = func() tea.Msg { return settingsSubmitMsg{} }
	form.CancelCmd = func() tea.Msg {
		return settingsSavedMsg{cfg: m.service.Config()}
	}
	form.WithShowHelp(false)
	form.WithTheme(huh.ThemeFunc(huh.ThemeCharm))
	m.settingsForm = form
	if onboarding {
		m.screen = screenOnboarding
	} else {
		m.screen = screenSettings
	}
}

func (m model) handleSettingsSubmit() (tea.Model, tea.Cmd) {
	draft := m.settingsDraft
	current := m.service.Config()
	return m, func() tea.Msg {
		cfg, err := buildConfigFromSettings(current, draft)
		if err != nil {
			return settingsSavedMsg{err: err}
		}
		if err := core.SaveSettings(cfg); err != nil {
			return settingsSavedMsg{err: err}
		}
		return settingsSavedMsg{cfg: cfg}
	}
}

func (m model) handleConfirmDone(msg confirmDoneMsg) (tea.Model, tea.Cmd) {
	state := m.confirmState
	m.confirm = nil

	if !msg.ok {
		if state.cancelRun {
			m.screen = screenRunning
		} else if state.action == core.JobSnapshot {
			m.screen = screenSnapshotPicker
		} else if state.action == core.JobRestore {
			m.screen = screenRestorePicker
		} else {
			m.screen = screenDashboard
		}
		return m, nil
	}

	if state.cancelRun {
		if m.cancelJob != nil {
			m.cancelJob()
		}
		m.screen = screenRunning
		return m, nil
	}

	if !state.startMySQL && !m.initialYes && !m.doctor.MySQLReachable {
		m.openConfirm(confirmState{
			reason:      "Start local MySQL?",
			description: fmt.Sprintf("%s is not reachable. Start %s with Homebrew first?", m.service.Config().MySQLSocket, m.service.Config().MySQLService),
			action:      state.action,
			target:      state.target,
			startMySQL:  true,
		})
		return m, nil
	}

	return m.startJob(state.action, state.target, state.startMySQL || m.initialYes)
}

func (m model) startJob(kind core.JobKind, target string, approveStart bool) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(m.ctx)
	ch := make(chan jobEventMsg, 64)

	runOpts := core.RunOptions{
		Yes:                 m.initialYes || kind == core.JobSnapshot,
		ApproveStartService: approveStart || m.initialYes,
		Debug:               m.service.Config().Debug,
	}

	m.cancelJob = cancel
	m.jobEvents = ch
	m.jobStartedAt = time.Now()
	m.jobLines = nil
	m.jobSummary = nil
	m.jobStatus = "starting"
	m.lastErr = nil
	m.lastResult = nil
	m.screen = screenRunning

	go func() {
		defer close(ch)
		sink := tuiSink{ch: ch}
		var (
			result core.JobResult
			err    error
		)
		switch kind {
		case core.JobSnapshot:
			result, err = m.service.RunSnapshot(ctx, target, runOpts, sink)
		case core.JobRestore:
			result, err = m.service.RunRestore(ctx, target, runOpts, sink)
		}
		if err != nil {
			ch <- jobEventMsg{err: err}
			return
		}
		ch <- jobEventMsg{result: &result}
	}()

	return m, tea.Batch(waitForJobEventCmd(ch), elapsedTickCmd())
}

func (m *model) resize() {
	if m.width == 0 || m.height == 0 {
		return
	}
	m.help.SetWidth(m.width)

	centerWidth := max(40, m.width-36)
	tableWidth := max(20, centerWidth-4)
	tableHeight := max(8, m.height-12)
	m.dbTable.SetWidth(tableWidth)
	m.dbTable.SetHeight(tableHeight)
	m.snapshotTable.SetWidth(tableWidth)
	m.snapshotTable.SetHeight(tableHeight)
	m.filter.SetWidth(max(20, tableWidth-10))
	m.logViewport.SetWidth(tableWidth)
	m.logViewport.SetHeight(tableHeight)
}

func (m *model) syncTables() {
	dbRows := make([]table.Row, 0, len(m.dbs))
	for _, db := range m.filteredDBs() {
		dbRows = append(dbRows, table.Row{
			db.Name,
			fmt.Sprintf("%d", db.TableCount),
			core.FormatBytes(db.SizeBytes),
		})
	}
	snapRows := make([]table.Row, 0, len(m.snapshots))
	for _, snapshot := range m.filteredSnapshots() {
		snapRows = append(snapRows, table.Row{
			snapshot.Name,
			core.FormatTime(snapshot.UpdatedAt),
			core.FormatBytes(snapshot.SizeBytes),
		})
	}
	m.dbTable.SetRows(dbRows)
	m.snapshotTable.SetRows(snapRows)
}

func (m *model) applyFilter() {
	m.syncTables()
}

func (m model) filteredDBs() []core.Database {
	filter := strings.TrimSpace(m.filter.Value())
	if filter == "" || (m.screen != screenSnapshotPicker && m.screen != screenDashboard) {
		return append([]core.Database(nil), m.dbs...)
	}
	names := make([]string, 0, len(m.dbs))
	lookup := map[string]core.Database{}
	for _, db := range m.dbs {
		names = append(names, db.Name)
		lookup[db.Name] = db
	}
	filtered := core.FilterNames(names, filter)
	out := make([]core.Database, 0, len(filtered))
	for _, name := range filtered {
		out = append(out, lookup[name])
	}
	return out
}

func (m model) filteredSnapshots() []core.Snapshot {
	filter := strings.TrimSpace(m.filter.Value())
	if filter == "" || (m.screen != screenRestorePicker && m.screen != screenDashboard) {
		return append([]core.Snapshot(nil), m.snapshots...)
	}
	names := make([]string, 0, len(m.snapshots))
	lookup := map[string]core.Snapshot{}
	for _, snapshot := range m.snapshots {
		names = append(names, snapshot.Name)
		lookup[snapshot.Name] = snapshot
	}
	filtered := core.FilterNames(names, filter)
	out := make([]core.Snapshot, 0, len(filtered))
	for _, name := range filtered {
		out = append(out, lookup[name])
	}
	return out
}

func (m model) View() tea.View {
	content := m.styles.frame.Render(m.render())
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "dbgold"
	return view
}

func (m model) render() string {
	status := m.renderStatus()
	helpView := m.help.View(m.keys)
	center := m.renderCenter()
	details := m.renderDetails()
	body := lipgloss.JoinHorizontal(lipgloss.Top, m.styles.center.Render(center), m.styles.sidebar.Render(details))
	return lipgloss.JoinVertical(lipgloss.Left, status, body, m.styles.helpBar.Render(helpView))
}

func (m model) renderStatus() string {
	mysqlState := "down"
	if m.doctor.MySQLReachable {
		mysqlState = "up"
	}
	mode := string(m.screen)
	left := fmt.Sprintf(" mysql %s ", mysqlState)
	middle := fmt.Sprintf(" %s ", m.service.SnapshotRoot())
	right := fmt.Sprintf(" %s ", mode)
	bar := lipgloss.JoinHorizontal(lipgloss.Top,
		m.styles.statusLeft.Render(left),
		m.styles.statusMiddle.Width(max(0, m.width-lipgloss.Width(left)-lipgloss.Width(right))).Render(middle),
		m.styles.statusRight.Render(right),
	)
	return bar
}

func (m model) renderCenter() string {
	switch m.screen {
	case screenDashboard:
		return m.renderDashboard()
	case screenSnapshotPicker:
		return m.renderPicker("Snapshot picker", "Choose a live database to snapshot.", m.filter.View(), m.dbTable.View())
	case screenRestorePicker:
		return m.renderPicker("Restore picker", "Choose a snapshot to restore.", m.filter.View(), m.snapshotTable.View())
	case screenConfirm:
		if m.confirm == nil {
			return ""
		}
		return m.styles.modal.Render(m.confirm.View())
	case screenRunning:
		return m.renderRunning()
	case screenResult:
		return m.renderResult()
	case screenDoctor:
		return m.renderDoctor()
	case screenSettings:
		return m.renderSettings()
	case screenOnboarding:
		return m.renderSettings()
	default:
		return ""
	}
}

func (m model) renderDashboard() string {
	title := m.styles.title.Render("Dashboard")
	lead := m.styles.subtle.Render("Keyboard-first snapshot orchestration for local MySQL.")
	if m.service.Config().NeedsOnboarding() {
		lead = m.styles.error.Render("Setup is not saved yet. Press c to finish first-run configuration.")
	}
	dbPanel := m.styles.panel.Render(lipgloss.JoinVertical(lipgloss.Left,
		m.styles.panelTitle.Render("Live databases"),
		m.dbTable.View(),
	))
	snapshotPanel := m.styles.panel.Render(lipgloss.JoinVertical(lipgloss.Left,
		m.styles.panelTitle.Render("Snapshots"),
		m.snapshotTable.View(),
	))
	return lipgloss.JoinVertical(lipgloss.Left, title, lead, lipgloss.JoinHorizontal(lipgloss.Top, dbPanel, snapshotPanel))
}

func (m model) renderPicker(title, subtitle, filterView, tableView string) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		m.styles.title.Render(title),
		m.styles.subtle.Render(subtitle),
		m.styles.filter.Render(filterView),
		m.styles.panel.Render(tableView),
	)
}

func (m model) renderRunning() string {
	title := fmt.Sprintf("%s %s", m.spin.View(), strings.Title(string(m.confirmState.action)))
	if m.lastResult != nil {
		title = fmt.Sprintf("%s %s", m.spin.View(), strings.Title(string(m.lastResult.Kind)))
	}
	elapsed := time.Since(m.jobStartedAt).Round(timeSecond)
	header := lipgloss.JoinVertical(lipgloss.Left,
		m.styles.title.Render(title),
		m.styles.subtle.Render(fmt.Sprintf("target: %s | elapsed: %s", m.confirmState.target, elapsed)),
		m.styles.statusLine.Render("status: "+blankFallback(m.jobStatus, "running")),
	)
	return lipgloss.JoinVertical(lipgloss.Left, header, m.styles.panel.Render(m.logViewport.View()))
}

func (m model) renderResult() string {
	title := "Result"
	if m.lastErr != nil {
		title = "Error"
	}
	lines := []string{m.styles.title.Render(title)}
	if m.lastErr != nil {
		lines = append(lines, m.styles.error.Render(m.lastErr.Error()))
	} else if m.lastResult != nil {
		lines = append(lines, m.styles.success.Render(fmt.Sprintf("%s finished for %s in %s", strings.Title(string(m.lastResult.Kind)), m.lastResult.Target, m.lastResult.Duration.Round(timeSecond))))
		lines = append(lines, m.styles.subtle.Render(m.lastResult.LogPath))
		lines = append(lines, m.jobSummary...)
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m model) renderDoctor() string {
	report := []string{
		m.styles.title.Render("Doctor"),
		fmt.Sprintf("MySQL reachable: %t", m.doctor.MySQLReachable),
		fmt.Sprintf("MySQL service:   %s", m.doctor.MySQLService),
		fmt.Sprintf("MySQL socket:    %s", m.doctor.MySQLSocket),
		fmt.Sprintf("MySQL version:   %s", blankFallback(m.doctor.MySQLVersion, "-")),
		fmt.Sprintf("Snapshot root:   %s", m.doctor.SnapshotRoot),
		fmt.Sprintf("Log root:        %s", m.doctor.LogRoot),
	}
	if len(m.doctor.MissingCommands) > 0 {
		report = append(report, "Missing tools: "+strings.Join(m.doctor.MissingCommands, ", "))
	}
	report = append(report, m.doctor.Warnings...)
	return m.styles.panel.Render(strings.Join(report, "\n"))
}

func (m model) renderSettings() string {
	title := "Settings"
	subtitle := "Saved settings seed the app for future launches. Environment variables still override them."
	if m.onboarding {
		title = "First-Run Setup"
		subtitle = "Review the defaults, change anything your environment needs, then save to continue into the dashboard."
	}
	lines := []string{
		m.styles.title.Render(title),
		m.styles.subtle.Render(subtitle),
	}
	if m.settingsForm != nil {
		lines = append(lines, m.styles.panel.Render(m.settingsForm.View()))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m model) renderDetails() string {
	lines := []string{m.styles.panelTitle.Render("Details")}
	switch m.screen {
	case screenSnapshotPicker, screenDashboard:
		row := m.dbTable.SelectedRow()
		if len(row) > 0 {
			lines = append(lines, "Database: "+row[0], "Tables: "+row[1], "Size: "+row[2])
		}
	case screenRestorePicker:
		row := m.snapshotTable.SelectedRow()
		if len(row) > 0 {
			lines = append(lines, "Snapshot: "+row[0], "Updated: "+row[1], "Size: "+row[2])
			for _, snapshot := range m.snapshots {
				if snapshot.Name == row[0] {
					keys := make([]string, 0, len(snapshot.Fields))
					for key := range snapshot.Fields {
						keys = append(keys, key)
					}
					sort.Strings(keys)
					for _, key := range keys {
						lines = append(lines, fmt.Sprintf("%s: %s", key, snapshot.Fields[key]))
					}
					break
				}
			}
		}
	case screenRunning:
		lines = append(lines,
			"Action: "+string(m.confirmState.action),
			"Target: "+m.confirmState.target,
			"Elapsed: "+time.Since(m.jobStartedAt).Round(timeSecond).String(),
			"Status: "+blankFallback(m.jobStatus, "starting"),
		)
		if len(m.jobSummary) > 0 {
			lines = append(lines, "")
			lines = append(lines, m.jobSummary...)
		}
	case screenSettings:
		lines = append(lines,
			"Config file: "+m.service.Config().ConfigPath,
			"Host: "+blankFallback(m.service.Config().MySQLHost, "-"),
			"Socket: "+blankFallback(m.service.Config().MySQLSocket, "-"),
			"User: "+blankFallback(m.service.Config().MySQLUser, "-"),
			"Snapshot root: "+m.service.Config().SnapshotRoot,
		)
	case screenOnboarding:
		lines = append(lines,
			"Config file: "+m.service.Config().ConfigPath,
			"Step 1: review the defaults",
			"Step 2: save settings",
			"Step 3: use the dashboard",
		)
	}
	if m.lastErr != nil && m.screen != screenResult {
		lines = append(lines, "", "Error:", m.lastErr.Error())
	}
	return m.styles.panel.Render(strings.Join(lines, "\n"))
}

func newDBTable() table.Model {
	t := table.New(
		table.WithColumns([]table.Column{
			{Title: "Database", Width: 26},
			{Title: "Tables", Width: 8},
			{Title: "Size", Width: 12},
		}),
		table.WithRows(nil),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true)
	styles.Selected = styles.Selected.Bold(true)
	t.SetStyles(styles)
	return t
}

func newSnapshotTable() table.Model {
	t := table.New(
		table.WithColumns([]table.Column{
			{Title: "Snapshot", Width: 26},
			{Title: "Updated", Width: 18},
			{Title: "Size", Width: 12},
		}),
		table.WithRows(nil),
		table.WithFocused(true),
		table.WithHeight(12),
	)
	styles := table.DefaultStyles()
	styles.Header = styles.Header.Bold(true)
	styles.Selected = styles.Selected.Bold(true)
	t.SetStyles(styles)
	return t
}

func orderedSummary(summary map[string]string) []string {
	if len(summary) == 0 {
		return nil
	}
	keys := make([]string, 0, len(summary))
	for key := range summary {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, summary[key])
	}
	return lines
}

func blankFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func firstError(errors ...error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func isInteractive() bool {
	stdinInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	stdoutInfo, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stdinInfo.Mode()&os.ModeCharDevice) != 0 && (stdoutInfo.Mode()&os.ModeCharDevice) != 0
}

func buildConfigFromSettings(base core.Config, values settingsValues) (core.Config, error) {
	cfg := base
	cfg.SnapshotRoot = strings.TrimSpace(values.SnapshotRoot)
	cfg.LogRoot = strings.TrimSpace(values.LogRoot)
	cfg.MySQLSHStateHome = strings.TrimSpace(values.MySQLSHStateHome)
	cfg.MySQLURI = strings.TrimSpace(values.MySQLURI)
	cfg.MySQLHost = strings.TrimSpace(values.MySQLHost)
	cfg.MySQLSocket = strings.TrimSpace(values.MySQLSocket)
	cfg.MySQLUser = strings.TrimSpace(values.MySQLUser)
	cfg.MySQLPassword = values.MySQLPassword
	cfg.MySQLLoginPath = strings.TrimSpace(values.MySQLLoginPath)
	cfg.MySQLService = strings.TrimSpace(values.MySQLService)
	cfg.MySQLCompression = strings.TrimSpace(values.MySQLCompression)
	cfg.MySQLBytesPerChunk = strings.TrimSpace(values.MySQLBytesPerChunk)
	cfg.MySQLDeferIndexes = strings.TrimSpace(values.MySQLDeferIndexes)
	cfg.MySQLAssumeEmptyPassword = values.MySQLAssumeEmptyPassword
	cfg.MySQLSkipBinlog = values.MySQLSkipBinlog
	cfg.MySQLAutoEnableLocalInfile = values.MySQLAutoEnableLocalInfile

	port, err := strconv.Atoi(strings.TrimSpace(values.MySQLPort))
	if err != nil {
		return cfg, fmt.Errorf("mysql port must be a number")
	}
	cfg.MySQLPort = port

	threads, err := strconv.Atoi(strings.TrimSpace(values.MySQLShellThreads))
	if err != nil {
		return cfg, fmt.Errorf("mysqlsh threads must be a number")
	}
	cfg.MySQLShellThreads = threads

	startTimeout, err := strconv.Atoi(strings.TrimSpace(values.MySQLStartTimeout))
	if err != nil {
		return cfg, fmt.Errorf("start timeout must be a number of seconds")
	}
	cfg.MySQLStartTimeout = time.Duration(startTimeout) * time.Second

	heartbeat, err := strconv.Atoi(strings.TrimSpace(values.MySQLHeartbeatInterval))
	if err != nil {
		return cfg, fmt.Errorf("heartbeat interval must be a number of seconds")
	}
	cfg.MySQLHeartbeatInterval = time.Duration(heartbeat) * time.Second

	if cfg.LogRoot == "" {
		cfg.LogRoot = core.DefaultConfig().LogRoot
	}
	if cfg.ConfigPath == "" {
		cfg.ConfigPath = base.ConfigPath
	}
	cfg.Onboarded = true
	return cfg, core.ValidateConfig(cfg)
}

func validatePositiveInt(value string) error {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || number <= 0 {
		return errors.New("enter a positive integer")
	}
	return nil
}
