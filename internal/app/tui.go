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
		Filter:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "find")),
		Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Refresh:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload")),
		Snap:     key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "snapshot")),
		Restore:  key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "restore")),
		Doctor:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "doctor")),
		Settings: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "settings")),
		Quit:     key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		Cancel:   key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "cancel")),
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

type layoutSpec struct {
	contentWidth       int
	mainWidth          int
	sidebarWidth       int
	stackSidebar       bool
	stackDashboard     bool
	dashboardPanelWide int
	bodyHeight         int
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
	styles := newStyles()

	filter := textinput.New()
	filter.Prompt = "find> "
	filter.Placeholder = "Type to narrow the list"

	dbTable := newDBTable(styles)
	snapshotTable := newSnapshotTable(styles)
	logViewport := viewport.New()
	logViewport.SoftWrap = true
	logViewport.FillHeight = true

	spin := spinner.New(spinner.WithSpinner(spinner.MiniDot))

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
					reason:      "Cancel this job?",
					description: "The current mysqlsh operation will stop after you confirm.",
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
			reason:      "Create a new snapshot?",
			description: fmt.Sprintf("If this succeeds, the saved snapshot for %s will be replaced.", target),
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
			reason:      "Restore this snapshot?",
			description: fmt.Sprintf("The local database %s will be dropped, recreated, and loaded from disk.", target),
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
						return "These defaults come from the legacy scripts. Review them, change anything your machine needs, then save to continue."
					}
					return "Update the saved defaults for this machine. Environment variables still override them at runtime."
				}()),
		).Title("Overview"),
		huh.NewGroup(
			huh.NewInput().Key("snapshot_root").Title("Snapshot folder").Value(&m.settingsDraft.SnapshotRoot).Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Key("log_root").Title("Log folder").Value(&m.settingsDraft.LogRoot).Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Key("mysqlsh_state_home").Title("MySQL Shell state folder").Value(&m.settingsDraft.MySQLSHStateHome).Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Key("mysql_service").Title("Homebrew service name").Value(&m.settingsDraft.MySQLService).Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Key("mysql_start_timeout").Title("Start timeout (seconds)").Value(&m.settingsDraft.MySQLStartTimeout).Validate(validatePositiveInt),
			huh.NewInput().Key("mysql_heartbeat_interval").Title("Heartbeat interval (seconds)").Value(&m.settingsDraft.MySQLHeartbeatInterval).Validate(validatePositiveInt),
		).Title("Storage and service"),
		huh.NewGroup(
			huh.NewInput().Key("mysql_uri").Title("MySQL Shell URI").Description("Optional. Leave blank to use host, user, and socket settings instead.").Value(&m.settingsDraft.MySQLURI),
			huh.NewInput().Key("mysql_host").Title("MySQL host").Value(&m.settingsDraft.MySQLHost),
			huh.NewInput().Key("mysql_port").Title("MySQL port").Value(&m.settingsDraft.MySQLPort).Validate(validatePositiveInt),
			huh.NewInput().Key("mysql_socket").Title("MySQL socket").Value(&m.settingsDraft.MySQLSocket),
			huh.NewInput().Key("mysql_user").Title("MySQL user").Value(&m.settingsDraft.MySQLUser).Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Key("mysql_login_path").Title("MySQL login path").Value(&m.settingsDraft.MySQLLoginPath),
			huh.NewInput().Key("mysql_password").Title("MySQL password").Password(true).Value(&m.settingsDraft.MySQLPassword),
			huh.NewConfirm().Key("mysql_assume_empty_password").Title("Try an empty password when no login path or password is set?").Value(&m.settingsDraft.MySQLAssumeEmptyPassword),
		).Title("Connection"),
		huh.NewGroup(
			huh.NewInput().Key("mysqlsh_threads").Title("MySQL Shell threads").Value(&m.settingsDraft.MySQLShellThreads).Validate(validatePositiveInt),
			huh.NewInput().Key("mysqlsh_compression").Title("Compression").Value(&m.settingsDraft.MySQLCompression).Validate(huh.ValidateNotEmpty()),
			huh.NewInput().Key("mysqlsh_bytes_per_chunk").Title("Bytes per chunk").Description("Optional, for example 128M").Value(&m.settingsDraft.MySQLBytesPerChunk),
			huh.NewInput().Key("mysqlsh_defer_table_indexes").Title("Deferred indexes").Value(&m.settingsDraft.MySQLDeferIndexes).Validate(huh.ValidateNotEmpty()),
			huh.NewConfirm().Key("mysqlsh_skip_binlog").Title("Skip binlog on restore?").Value(&m.settingsDraft.MySQLSkipBinlog),
			huh.NewConfirm().Key("mysqlsh_auto_enable_local_infile").Title("Temporarily enable local_infile when a restore needs it?").Value(&m.settingsDraft.MySQLAutoEnableLocalInfile),
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
			reason:      "Start local MySQL first?",
			description: fmt.Sprintf("%s is not reachable right now. Start %s with Homebrew before continuing?", m.service.Config().MySQLSocket, m.service.Config().MySQLService),
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
	layout := m.layout()
	m.help.SetWidth(layout.contentWidth)

	tableWidth := m.panelInnerWidth(layout.mainWidth)
	if tableWidth < 8 {
		tableWidth = 8
	}
	tableHeight := max(8, layout.bodyHeight-8)

	applyDBTableLayout(&m.dbTable, tableWidth)
	m.dbTable.SetHeight(tableHeight)

	applySnapshotTableLayout(&m.snapshotTable, tableWidth)
	m.snapshotTable.SetHeight(tableHeight)

	filterWidth := tableWidth - 10
	if filterWidth < 8 {
		filterWidth = tableWidth
	}
	m.filter.SetWidth(filterWidth)
	m.logViewport.SetWidth(tableWidth)
	m.logViewport.SetHeight(tableHeight)
	if m.confirm != nil {
		m.confirm.WithWidth(clamp(layout.mainWidth-8, 1, min(72, layout.mainWidth)))
		m.confirm.WithHeight(max(8, layout.bodyHeight-10))
	}
	if m.settingsForm != nil {
		m.settingsForm.WithWidth(m.panelInnerWidth(layout.mainWidth))
		m.settingsForm.WithHeight(max(12, layout.bodyHeight-6))
	}
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
	layout := m.layout()
	status := m.renderStatus(layout)
	helpView := m.help.View(m.keys)
	center := m.renderCenter(layout)
	details := m.renderDetails(layout)

	var body string
	if layout.stackSidebar {
		body = lipgloss.JoinVertical(
			lipgloss.Left,
			m.styles.center.Copy().Width(layout.mainWidth).MarginRight(0).Render(center),
			m.styles.sidebar.Copy().Width(layout.mainWidth).Render(details),
		)
	} else {
		body = lipgloss.JoinHorizontal(
			lipgloss.Top,
			m.styles.center.Copy().Width(layout.mainWidth).Render(center),
			m.styles.sidebar.Copy().Width(layout.sidebarWidth).Render(details),
		)
	}

	return lipgloss.JoinVertical(lipgloss.Left, status, body, m.styles.helpBar.Copy().Width(layout.contentWidth).Render(helpView))
}

func (m model) renderStatus(layout layoutSpec) string {
	mysqlState := "offline"
	if m.doctor.MySQLReachable {
		mysqlState = "online"
	}
	left := m.styles.statusLeft.Render("mysql " + mysqlState)
	right := m.styles.statusRight.Render(m.screenLabel())
	middleWidth := max(10, layout.contentWidth-lipgloss.Width(left)-lipgloss.Width(right))
	middle := m.styles.statusMiddle.Width(middleWidth).Render(truncateMiddle(m.service.SnapshotRoot(), max(8, middleWidth-2)))
	bar := lipgloss.JoinHorizontal(lipgloss.Top,
		left,
		middle,
		right,
	)
	return bar
}

func (m model) renderCenter(layout layoutSpec) string {
	switch m.screen {
	case screenDashboard:
		return m.renderDashboard(layout)
	case screenSnapshotPicker:
		return m.renderPicker(layout, "Create snapshot", "Choose a live database to capture.", m.filter.View(), m.dbTable.View())
	case screenRestorePicker:
		return m.renderPicker(layout, "Restore snapshot", "Choose a saved snapshot to load into local MySQL.", m.filter.View(), m.snapshotTable.View())
	case screenConfirm:
		if m.confirm == nil {
			return ""
		}
		modalWidth := clamp(layout.mainWidth-8, 1, min(72, layout.mainWidth))
		modal := m.styles.modal.Copy().Width(modalWidth).Render(m.confirm.View())
		return lipgloss.PlaceHorizontal(layout.mainWidth, lipgloss.Center, modal)
	case screenRunning:
		return m.renderRunning(layout)
	case screenResult:
		return m.renderResult(layout)
	case screenDoctor:
		return m.renderDoctor(layout)
	case screenSettings:
		return m.renderSettings(layout)
	case screenOnboarding:
		return m.renderSettings(layout)
	default:
		return ""
	}
}

func (m model) renderDashboard(layout layoutSpec) string {
	title := lipgloss.JoinHorizontal(
		lipgloss.Center,
		m.styles.title.Render("dbgold"),
		" ",
		m.styles.titleAccent.Render("Dashboard"),
	)
	lead := m.styles.subtle.Render("Fast local MySQL snapshots and restores, without shell scripts.")
	if m.service.Config().NeedsOnboarding() {
		lead = m.styles.error.Render("Setup is not saved yet. Press c to finish first-run setup.")
	}
	summary := m.renderBadgeRow(
		m.styles.badgeStrong.Render(fmt.Sprintf("%d databases", len(m.dbs))),
		m.styles.badge.Render(fmt.Sprintf("%d snapshots", len(m.snapshots))),
		m.mysqlStatusBadge(),
		m.styles.badge.Render("tab to switch"),
	)

	dbTable := m.dbTable
	applyDBTableLayout(&dbTable, m.panelInnerWidth(layout.dashboardPanelWide))
	dbTable.SetHeight(max(6, layout.bodyHeight-12))

	snapshotTable := m.snapshotTable
	applySnapshotTableLayout(&snapshotTable, m.panelInnerWidth(layout.dashboardPanelWide))
	snapshotTable.SetHeight(max(6, layout.bodyHeight-12))

	dbPanelStyle := m.styles.panel
	snapshotPanelStyle := m.styles.panel
	if m.dashboardFocus == 0 {
		dbPanelStyle = m.styles.panelActive
	} else {
		snapshotPanelStyle = m.styles.panelActive
	}

	dbPanel := dbPanelStyle.Copy().Width(layout.dashboardPanelWide).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.panelTitle.Render("Live databases"),
		m.styles.subtle.Render("Choose a database to capture from your local MySQL instance."),
		m.renderTableOrEmpty(dbTable.View(), len(dbTable.Rows()) == 0, "No databases found. Press r to reload or open doctor."),
	))
	snapshotPanel := snapshotPanelStyle.Copy().Width(layout.dashboardPanelWide).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.panelTitle.Render("Snapshots"),
		m.styles.subtle.Render("Choose a saved dump to restore into local MySQL."),
		m.renderTableOrEmpty(snapshotTable.View(), len(snapshotTable.Rows()) == 0, "No snapshots yet. Press s to create the first one."),
	))

	var body string
	if layout.stackDashboard {
		body = lipgloss.JoinVertical(lipgloss.Left, dbPanel, snapshotPanel)
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top, dbPanel, snapshotPanel)
	}

	return lipgloss.JoinVertical(lipgloss.Left, title, summary, lead, body)
}

func (m model) renderPicker(layout layoutSpec, title, subtitle, filterView, tableView string) string {
	filterValue := strings.TrimSpace(m.filter.Value())
	count := len(m.dbTable.Rows())
	emptyText := "No matching databases."
	if m.screen == screenRestorePicker {
		count = len(m.snapshotTable.Rows())
		emptyText = "No matching snapshots."
	}
	badges := []string{
		m.styles.badgeStrong.Render(fmt.Sprintf("%d shown", count)),
		m.styles.badge.Render("enter to continue"),
		m.styles.badge.Render("/ to filter"),
	}
	if filterValue != "" {
		badges = append(badges, m.styles.badgeWarn.Render("filter: "+filterValue))
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.title.Render(title),
		m.renderBadgeRow(badges...),
		m.styles.subtle.Render(subtitle),
		m.styles.filter.Render(filterView),
		m.styles.panel.Copy().Width(layout.mainWidth).Render(m.renderTableOrEmpty(tableView, count == 0, emptyText)),
	)
}

func (m model) renderRunning(layout layoutSpec) string {
	title := fmt.Sprintf("%s %s", m.spin.View(), screenLabelForJob(m.confirmState.action))
	if m.lastResult != nil {
		title = fmt.Sprintf("%s %s", m.spin.View(), screenLabelForJob(m.lastResult.Kind))
	}
	elapsed := time.Since(m.jobStartedAt).Round(timeSecond)
	metrics := m.renderBadgeRow(
		m.styles.badgeStrong.Render(screenLabelForJob(m.confirmState.action)),
		m.styles.badge.Render("target "+blankFallback(m.confirmState.target, "-")),
		m.styles.badge.Render("elapsed "+elapsed.String()),
		m.jobStatusBadge(),
	)
	header := lipgloss.JoinVertical(lipgloss.Left,
		m.styles.title.Render(title),
		metrics,
		m.styles.subtle.Render("Streaming mysqlsh output in real time."),
	)
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		m.styles.panel.Copy().Width(layout.mainWidth).Render(lipgloss.JoinVertical(
			lipgloss.Left,
			m.styles.panelTitle.Render("Job log"),
			m.logViewport.View(),
		)),
	)
}

func (m model) renderResult(layout layoutSpec) string {
	title := "Result"
	statusBadge := m.styles.badgeOK.Render("completed")
	if m.lastErr != nil {
		title = "Error"
		statusBadge = m.styles.badgeError.Render("failed")
	}
	lines := []string{
		lipgloss.JoinHorizontal(lipgloss.Center, m.styles.title.Render(title), " ", statusBadge),
	}
	if m.lastErr != nil {
		lines = append(lines, m.renderValueBlock("Latest error", m.lastErr.Error(), m.panelInnerWidth(layout.mainWidth), m.styles.error))
	} else if m.lastResult != nil {
		lines = append(lines, m.styles.success.Render(fmt.Sprintf("%s finished for %s in %s.", screenLabelForJob(m.lastResult.Kind), m.lastResult.Target, m.lastResult.Duration.Round(timeSecond))))
		lines = append(lines, m.renderValueBlock("Log file", m.lastResult.LogPath, m.panelInnerWidth(layout.mainWidth), m.styles.code))
		if len(m.jobSummary) > 0 {
			lines = append(lines, m.renderBulletBlock("Summary", m.jobSummary, m.panelInnerWidth(layout.mainWidth)))
		}
	}
	return m.styles.panel.Copy().Width(layout.mainWidth).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (m model) renderDoctor(layout layoutSpec) string {
	width := m.panelInnerWidth(layout.mainWidth)
	blocks := []string{
		lipgloss.JoinHorizontal(
			lipgloss.Center,
			m.styles.title.Render("Doctor"),
			" ",
			m.mysqlStatusBadge(),
		),
		m.renderBadgeRow(
			m.styles.badge.Render(fmt.Sprintf("%d missing tools", len(m.doctor.MissingCommands))),
			m.styles.badge.Render(fmt.Sprintf("%d warnings", len(m.doctor.Warnings))),
		),
		m.renderValueBlock("MySQL reachable", fmt.Sprintf("%t", m.doctor.MySQLReachable), width, m.styles.value),
		m.renderValueBlock("MySQL service", blankFallback(m.doctor.MySQLService, "-"), width, m.styles.value),
		m.renderValueBlock("MySQL socket", blankFallback(m.doctor.MySQLSocket, "-"), width, m.styles.code),
		m.renderValueBlock("MySQL version", blankFallback(m.doctor.MySQLVersion, "-"), width, m.styles.value),
		m.renderValueBlock("Snapshot root", blankFallback(m.doctor.SnapshotRoot, "-"), width, m.styles.code),
		m.renderValueBlock("Log root", blankFallback(m.doctor.LogRoot, "-"), width, m.styles.code),
	}
	if len(m.doctor.MissingCommands) > 0 {
		blocks = append(blocks, m.renderBulletBlock("Missing tools", m.doctor.MissingCommands, width))
	}
	if len(m.doctor.Warnings) > 0 {
		blocks = append(blocks, m.renderBulletBlock("Warnings", m.doctor.Warnings, width))
	}
	return m.styles.panel.Copy().Width(layout.mainWidth).Render(lipgloss.JoinVertical(lipgloss.Left, blocks...))
}

func (m model) renderSettings(layout layoutSpec) string {
	title := "Settings"
	subtitle := "These saved values fill in the app the next time it starts. Environment variables can still override them."
	badges := []string{
		m.styles.badgeStrong.Render("saved config"),
		m.styles.badge.Render("env vars still win"),
	}
	if m.onboarding {
		title = "First-Run Setup"
		subtitle = "Review the defaults, adjust anything your machine needs, then save to enter the dashboard."
		badges = []string{
			m.styles.badgeStrong.Render("onboarding"),
			m.styles.badge.Render("review"),
			m.styles.badge.Render("save"),
			m.styles.badge.Render("continue"),
		}
	}
	lines := []string{
		m.styles.title.Render(title),
		m.renderBadgeRow(badges...),
		m.styles.subtle.Render(subtitle),
	}
	if m.settingsForm != nil {
		lines = append(lines, m.styles.panel.Copy().Width(layout.mainWidth).Render(m.settingsForm.View()))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m model) renderDetails(layout layoutSpec) string {
	panelWidth := layout.sidebarWidth
	if layout.stackSidebar {
		panelWidth = layout.mainWidth
	}
	contentWidth := m.panelInnerWidth(panelWidth)
	blocks := []string{}
	title := "Details"

	switch m.screen {
	case screenDashboard:
		if m.dashboardFocus == 1 {
			title = "Snapshot details"
			row := m.snapshotTable.SelectedRow()
			if len(row) > 0 {
				blocks = append(blocks,
					m.renderValueBlock("Snapshot", row[0], contentWidth, m.styles.value),
					m.renderValueBlock("Updated", row[1], contentWidth, m.styles.value),
					m.renderValueBlock("Size", row[2], contentWidth, m.styles.value),
				)
			}
			break
		}
		title = "Database details"
		fallthrough
	case screenSnapshotPicker:
		title = "Database details"
		row := m.dbTable.SelectedRow()
		if len(row) > 0 {
			blocks = append(blocks,
				m.renderValueBlock("Database", row[0], contentWidth, m.styles.value),
				m.renderValueBlock("Tables", row[1], contentWidth, m.styles.value),
				m.renderValueBlock("Size", row[2], contentWidth, m.styles.value),
			)
		}
	case screenRestorePicker:
		title = "Snapshot details"
		row := m.snapshotTable.SelectedRow()
		if len(row) > 0 {
			blocks = append(blocks,
				m.renderValueBlock("Snapshot", row[0], contentWidth, m.styles.value),
				m.renderValueBlock("Updated", row[1], contentWidth, m.styles.value),
				m.renderValueBlock("Size", row[2], contentWidth, m.styles.value),
			)
			for _, snapshot := range m.snapshots {
				if snapshot.Name == row[0] {
					keys := make([]string, 0, len(snapshot.Fields))
					for key := range snapshot.Fields {
						keys = append(keys, key)
					}
					sort.Strings(keys)
					for _, key := range keys {
						blocks = append(blocks, m.renderValueBlock(prettyLabel(key), snapshot.Fields[key], contentWidth, m.styles.value))
					}
					break
				}
			}
		}
	case screenRunning:
		title = "Job details"
		blocks = append(blocks,
			m.renderValueBlock("Action", screenLabelForJob(m.confirmState.action), contentWidth, m.styles.value),
			m.renderValueBlock("Target", m.confirmState.target, contentWidth, m.styles.value),
			m.renderValueBlock("Elapsed", time.Since(m.jobStartedAt).Round(timeSecond).String(), contentWidth, m.styles.value),
			m.renderValueBlock("Status", blankFallback(m.jobStatus, "starting"), contentWidth, m.styles.statusLine),
		)
		if len(m.jobSummary) > 0 {
			blocks = append(blocks, m.renderBulletBlock("Summary", m.jobSummary, contentWidth))
		}
	case screenSettings:
		title = "Current config"
		blocks = append(blocks,
			m.renderValueBlock("Config file", m.service.Config().ConfigPath, contentWidth, m.styles.code),
			m.renderValueBlock("Host", blankFallback(m.service.Config().MySQLHost, "-"), contentWidth, m.styles.value),
			m.renderValueBlock("Socket", blankFallback(m.service.Config().MySQLSocket, "-"), contentWidth, m.styles.code),
			m.renderValueBlock("User", blankFallback(m.service.Config().MySQLUser, "-"), contentWidth, m.styles.value),
			m.renderValueBlock("Snapshot root", m.service.Config().SnapshotRoot, contentWidth, m.styles.code),
		)
	case screenOnboarding:
		title = "Setup guide"
		blocks = append(blocks,
			m.renderValueBlock("Config file", m.service.Config().ConfigPath, contentWidth, m.styles.code),
			m.renderBulletBlock("Next steps", []string{
				"Review the defaults",
				"Save settings",
				"Use the dashboard",
			}, contentWidth),
		)
	}
	if m.lastErr != nil && m.screen != screenResult {
		blocks = append(blocks, m.renderValueBlock("Latest error", m.lastErr.Error(), contentWidth, m.styles.error))
	}
	if len(blocks) == 0 {
		blocks = append(blocks, m.styles.subtle.Render("Select a row to see more detail here."))
	}
	return m.styles.panel.Copy().Width(panelWidth).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.panelTitle.Render(title),
		lipgloss.JoinVertical(lipgloss.Left, blocks...),
	))
}

func newDBTable(appStyles styles) table.Model {
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
	styles.Header = styles.Header.Foreground(appStyles.subtle.GetForeground()).Bold(true)
	styles.Cell = styles.Cell.Foreground(appStyles.value.GetForeground())
	styles.Selected = styles.Selected.Foreground(appStyles.frame.GetBackground()).Background(appStyles.statusRight.GetBackground()).Bold(true)
	t.SetStyles(styles)
	return t
}

func newSnapshotTable(appStyles styles) table.Model {
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
	styles.Header = styles.Header.Foreground(appStyles.subtle.GetForeground()).Bold(true)
	styles.Cell = styles.Cell.Foreground(appStyles.value.GetForeground())
	styles.Selected = styles.Selected.Foreground(appStyles.frame.GetBackground()).Background(appStyles.statusRight.GetBackground()).Bold(true)
	t.SetStyles(styles)
	return t
}

func (m model) layout() layoutSpec {
	width := m.width
	if width == 0 {
		width = 120
	}
	height := m.height
	if height == 0 {
		height = 32
	}

	contentWidth := width - m.styles.frame.GetHorizontalFrameSize()
	if contentWidth <= 0 {
		contentWidth = 1
	}
	stackSidebar := contentWidth < 120
	sidebarWidth := 0
	mainWidth := contentWidth
	if !stackSidebar {
		sidebarWidth = clamp(contentWidth/3, 34, 48)
		mainWidth = max(56, contentWidth-sidebarWidth-1)
	}

	stackDashboard := mainWidth < 96
	dashboardPanelWide := mainWidth
	if !stackDashboard {
		dashboardPanelWide = max(28, (mainWidth-1)/2)
	}

	return layoutSpec{
		contentWidth:       contentWidth,
		mainWidth:          mainWidth,
		sidebarWidth:       sidebarWidth,
		stackSidebar:       stackSidebar,
		stackDashboard:     stackDashboard,
		dashboardPanelWide: dashboardPanelWide,
		bodyHeight:         max(18, height-4),
	}
}

func (m model) panelInnerWidth(width int) int {
	innerWidth := width - m.styles.panel.GetHorizontalFrameSize()
	if innerWidth <= 0 {
		return 1
	}
	return innerWidth
}

func applyDBTableLayout(t *table.Model, width int) {
	t.SetWidth(width)
	t.SetColumns(dbColumns(width))
	t.UpdateViewport()
}

func applySnapshotTableLayout(t *table.Model, width int) {
	t.SetWidth(width)
	t.SetColumns(snapshotColumns(width))
	t.UpdateViewport()
}

func dbColumns(width int) []table.Column {
	if width <= 18 {
		return []table.Column{
			{Title: "Database", Width: max(4, width-8)},
			{Title: "Tbl", Width: 2},
			{Title: "Sz", Width: 4},
		}
	}
	tablesWidth := clamp(width/5, 3, 8)
	sizeWidth := clamp(width/4, 6, 10)
	nameWidth := width - tablesWidth - sizeWidth - 2
	if nameWidth < 8 {
		nameWidth = 8
		sizeWidth = max(4, width-nameWidth-tablesWidth-2)
	}
	return []table.Column{
		{Title: "Database", Width: nameWidth},
		{Title: "Tables", Width: tablesWidth},
		{Title: "Size", Width: sizeWidth},
	}
}

func snapshotColumns(width int) []table.Column {
	if width <= 22 {
		return []table.Column{
			{Title: "Snapshot", Width: max(6, width-11)},
			{Title: "When", Width: 4},
			{Title: "Sz", Width: 5},
		}
	}
	updatedWidth := clamp(width/3, 8, 16)
	sizeWidth := clamp(width/4, 6, 10)
	nameWidth := width - updatedWidth - sizeWidth - 2
	if nameWidth < 8 {
		nameWidth = 8
		updatedWidth = max(6, width-nameWidth-sizeWidth-2)
	}
	return []table.Column{
		{Title: "Snapshot", Width: nameWidth},
		{Title: "Updated", Width: updatedWidth},
		{Title: "Size", Width: sizeWidth},
	}
}

func (m model) renderValueBlock(label, value string, width int, valueStyle lipgloss.Style) string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.label.Render(label),
		valueStyle.Render(wrapText(blankFallback(value, "-"), width)),
	)
}

func (m model) renderBadgeRow(badges ...string) string {
	filtered := make([]string, 0, len(badges))
	for _, badge := range badges {
		if strings.TrimSpace(badge) == "" {
			continue
		}
		filtered = append(filtered, badge)
	}
	if len(filtered) == 0 {
		return ""
	}
	row := make([]string, 0, len(filtered)*2-1)
	for i, badge := range filtered {
		if i > 0 {
			row = append(row, " ")
		}
		row = append(row, badge)
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, row...)
}

func (m model) renderBulletBlock(label string, items []string, width int) string {
	if len(items) == 0 {
		return ""
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, wrapText("- "+item, width))
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.label.Render(label),
		m.styles.value.Render(strings.Join(lines, "\n")),
	)
}

func (m model) renderTableOrEmpty(tableView string, empty bool, message string) string {
	if !empty {
		return tableView
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.subtle.Render(message),
		m.styles.subtle.Render("Open doctor if MySQL or the snapshot folders look out of sync."),
	)
}

func (m model) mysqlStatusBadge() string {
	if m.doctor.MySQLReachable {
		return m.styles.badgeOK.Render("mysql online")
	}
	return m.styles.badgeWarn.Render("mysql offline")
}

func (m model) jobStatusBadge() string {
	status := blankFallback(m.jobStatus, "starting")
	switch strings.ToLower(status) {
	case "done", "completed", "ready", "finished":
		return m.styles.badgeOK.Render(status)
	case "failed", "error", "cancelled":
		return m.styles.badgeError.Render(status)
	default:
		return m.styles.badgeWarn.Render(status)
	}
}

func (m model) screenLabel() string {
	switch m.screen {
	case screenSnapshotPicker:
		return "choose database"
	case screenRestorePicker:
		return "choose snapshot"
	case screenConfirm:
		return "confirm"
	case screenRunning:
		return "running"
	case screenResult:
		return "result"
	case screenDoctor:
		return "doctor"
	case screenSettings:
		return "settings"
	case screenOnboarding:
		return "setup"
	default:
		return "dashboard"
	}
}

func screenLabelForJob(kind core.JobKind) string {
	switch kind {
	case core.JobSnapshot:
		return "Snapshot"
	case core.JobRestore:
		return "Restore"
	default:
		return "Job"
	}
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func truncateMiddle(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 6 {
		return string(runes[:width])
	}
	prefix := (width - 3) / 2
	suffix := width - 3 - prefix
	return string(runes[:prefix]) + "..." + string(runes[len(runes)-suffix:])
}

func wrapText(value string, width int) string {
	if width <= 0 {
		return value
	}
	lines := strings.Split(value, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			wrapped = append(wrapped, "")
			continue
		}
		wrapped = append(wrapped, lipgloss.Wrap(line, width, " /_-=:,."))
	}
	return strings.Join(wrapped, "\n")
}

func prettyLabel(value string) string {
	if value == "" {
		return "-"
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
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
