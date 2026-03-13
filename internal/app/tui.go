package app

import (
	"context"
	"errors"
	"fmt"
	"image/color"
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
	filterStyles := textinput.DefaultDarkStyles()
	filterStyles.Focused.Prompt = styles.titleAccent
	filterStyles.Focused.Text = styles.value
	filterStyles.Focused.Placeholder = styles.subtle
	filterStyles.Blurred.Prompt = styles.subtle
	filterStyles.Blurred.Text = styles.value
	filterStyles.Blurred.Placeholder = styles.subtle
	filter.SetStyles(filterStyles)

	dbTable := newDBTable(styles)
	snapshotTable := newSnapshotTable(styles)
	logViewport := viewport.New()
	logViewport.SoftWrap = false
	logViewport.FillHeight = true

	spin := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	spin.Style = styles.titleAccent

	m := model{
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
	m.help.Styles.ShortKey = styles.titleAccent
	m.help.Styles.ShortDesc = styles.subtle
	m.help.Styles.ShortSeparator = styles.subtle
	m.help.Styles.Ellipsis = styles.subtle
	m.help.Styles.FullKey = styles.titleAccent
	m.help.Styles.FullDesc = styles.subtle
	m.help.Styles.FullSeparator = styles.subtle

	if opts.initialQuery != "" {
		m.filter.SetValue(opts.initialQuery)
	}
	if opts.mode == screenSettings || opts.mode == screenOnboarding {
		m.openSettings(opts.mode == screenOnboarding)
	}

	return m
}

func (m model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.settingsForm != nil {
		cmds = append(cmds, m.settingsForm.Init())
	}
	cmds = append(cmds, loadDashboardCmd(m.ctx, m.service), spinTickCmd(m.spin))
	return tea.Batch(cmds...)
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
		m.doctor = msg.doctor
		m.dbs = msg.dbs
		m.snapshots = msg.snapshots
		m.syncTables()
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
			m.refreshLogViewport()
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
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			switch strings.ToLower(keyMsg.String()) {
			case "left", "h":
				m.confirmValue = true
			case "right", "l":
				m.confirmValue = false
			case "tab":
				m.confirmValue = !m.confirmValue
			case "y":
				return m.handleConfirmDone(confirmDoneMsg{ok: true})
			case "n":
				return m.handleConfirmDone(confirmDoneMsg{ok: false})
			default:
				switch {
				case key.Matches(keyMsg, m.keys.Enter):
					return m.handleConfirmDone(confirmDoneMsg{ok: m.confirmValue})
				case key.Matches(keyMsg, m.keys.Back):
					return m.handleConfirmDone(confirmDoneMsg{ok: false})
				}
			}
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
				cmds = append(cmds, m.openConfirm(confirmState{
					reason:      "Cancel this job?",
					description: "The current mysqlsh operation will stop after you confirm.",
					cancelRun:   true,
				}))
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
			cmds = append(cmds, m.openSettings(false))
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
		return m, m.openConfirm(confirmState{
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
		return m, m.openConfirm(confirmState{
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

func (m *model) openConfirm(state confirmState) tea.Cmd {
	m.confirmState = state
	m.confirmValue = true
	m.screen = screenConfirm
	return nil
}

func (m *model) openSettings(onboarding bool) tea.Cmd {
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

	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Key("snapshot_root").
			Title("Snapshot folder").
			Description("Storage and service defaults start here. Choose where snapshots live on this machine.").
			Value(&m.settingsDraft.SnapshotRoot).
			Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Key("log_root").Title("Log folder").Value(&m.settingsDraft.LogRoot).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Key("mysqlsh_state_home").Title("MySQL Shell state folder").Value(&m.settingsDraft.MySQLSHStateHome).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Key("mysql_service").Title("Homebrew service name").Value(&m.settingsDraft.MySQLService).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Key("mysql_start_timeout").Title("Start timeout (seconds)").Value(&m.settingsDraft.MySQLStartTimeout).Validate(validatePositiveInt),
		huh.NewInput().Key("mysql_heartbeat_interval").Title("Heartbeat interval (seconds)").Value(&m.settingsDraft.MySQLHeartbeatInterval).Validate(validatePositiveInt),
		huh.NewInput().
			Key("mysql_uri").
			Title("MySQL Shell URI").
			Description("Connection settings start here. Optional. Leave blank to use host, user, and socket settings instead.").
			Value(&m.settingsDraft.MySQLURI),
		huh.NewInput().Key("mysql_host").Title("MySQL host").Value(&m.settingsDraft.MySQLHost),
		huh.NewInput().Key("mysql_port").Title("MySQL port").Value(&m.settingsDraft.MySQLPort).Validate(validatePositiveInt),
		huh.NewInput().Key("mysql_socket").Title("MySQL socket").Value(&m.settingsDraft.MySQLSocket),
		huh.NewInput().Key("mysql_user").Title("MySQL user").Value(&m.settingsDraft.MySQLUser).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Key("mysql_login_path").Title("MySQL login path").Value(&m.settingsDraft.MySQLLoginPath),
		huh.NewInput().Key("mysql_password").Title("MySQL password").Password(true).Value(&m.settingsDraft.MySQLPassword),
		huh.NewConfirm().Key("mysql_assume_empty_password").Title("Try an empty password when no login path or password is set?").Value(&m.settingsDraft.MySQLAssumeEmptyPassword),
		huh.NewInput().
			Key("mysqlsh_threads").
			Title("MySQL Shell threads").
			Description("Dump and restore defaults start here. These settings control mysqlsh snapshot and restore behavior.").
			Value(&m.settingsDraft.MySQLShellThreads).
			Validate(validatePositiveInt),
		huh.NewInput().Key("mysqlsh_compression").Title("Compression").Value(&m.settingsDraft.MySQLCompression).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Key("mysqlsh_bytes_per_chunk").Title("Bytes per chunk").Description("Optional, for example 128M").Value(&m.settingsDraft.MySQLBytesPerChunk),
		huh.NewInput().Key("mysqlsh_defer_table_indexes").Title("Deferred indexes").Value(&m.settingsDraft.MySQLDeferIndexes).Validate(huh.ValidateNotEmpty()),
		huh.NewConfirm().Key("mysqlsh_skip_binlog").Title("Skip binlog on restore?").Value(&m.settingsDraft.MySQLSkipBinlog),
		huh.NewConfirm().
			Key("mysqlsh_auto_enable_local_infile").
			Title("Temporarily enable local_infile when a restore needs it?").
			Description("Last setting. Press enter once more after this field to save and continue.").
			Value(&m.settingsDraft.MySQLAutoEnableLocalInfile),
	))
	form.SubmitCmd = func() tea.Msg { return settingsSubmitMsg{} }
	form.CancelCmd = func() tea.Msg {
		return settingsSavedMsg{cfg: m.service.Config()}
	}
	form.WithShowHelp(false)
	form.WithTheme(newFormTheme(m.styles))
	m.settingsForm = form
	if onboarding {
		m.screen = screenOnboarding
	} else {
		m.screen = screenSettings
	}
	return m.settingsForm.Init()
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
		return m, m.openConfirm(confirmState{
			reason:      "Start local MySQL first?",
			description: fmt.Sprintf("%s is not reachable right now. Start %s with Homebrew before continuing?", m.service.Config().MySQLSocket, m.service.Config().MySQLService),
			action:      state.action,
			target:      state.target,
			startMySQL:  true,
		})
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
	m.refreshLogViewport()
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

func (m *model) refreshLogViewport() {
	width := m.logViewport.Width()
	if width <= 0 {
		m.logViewport.SetContent(strings.Join(m.jobLines, "\n"))
		return
	}
	m.logViewport.SetContent(m.renderLogContent(width))
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
	layout := m.layout()
	contentHeight := max(1, m.height-m.styles.frame.GetVerticalFrameSize())
	content := m.styles.frame.Render(boundedBlock(m.render(), layout.contentWidth, contentHeight, m.screenFillStyle()))
	view := tea.NewView(content)
	view.AltScreen = true
	view.BackgroundColor = m.styles.frame.GetBackground()
	view.ForegroundColor = m.styles.frame.GetForeground()
	view.WindowTitle = "dbgold"
	return view
}

func (m model) render() string {
	layout := m.layout()
	status := m.renderStatus(layout)
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

	return lipgloss.JoinVertical(lipgloss.Left, status, body, m.renderFooter(layout))
}

func (m model) renderStatus(layout layoutSpec) string {
	cardStyle := m.styles.statusCard.Copy().Width(layout.contentWidth)
	bodyWidth := max(1, layout.contentWidth-cardStyle.GetHorizontalFrameSize())
	fill := styleWithBackground(m.styles.statusPath, cardStyle.GetBackground())
	titleFill := styleWithBackground(m.styles.heroTitle, cardStyle.GetBackground())
	title := lipgloss.JoinHorizontal(
		lipgloss.Center,
		styleWithBackground(m.styles.heroEyebrow, cardStyle.GetBackground()).Render("dbgold"),
		" ",
		titleFill.Render(prettyLabel(string(m.screen))),
	)
	path := lipgloss.JoinHorizontal(
		lipgloss.Left,
		styleWithBackground(m.styles.statusLabel, cardStyle.GetBackground()).Render("Workspace"),
		" ",
		fill.Render(truncateMiddle(m.service.SnapshotRoot(), max(12, bodyWidth-11))),
	)
	return cardStyle.Render(lipgloss.JoinVertical(
		lipgloss.Left,
		boundedRendered(title, bodyWidth, titleFill),
		m.renderBadgeBlockWithFill(bodyWidth, fill,
			m.mysqlStatusBadge(),
			m.styles.badgeStrong.Render(prettyLabel(string(m.screen))),
			m.styles.badgeGhost.Render("tab switches focus"),
		),
		boundedRendered(path, bodyWidth, fill),
	))
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
		return m.renderConfirm(layout)
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

func (m model) renderHeadlineCard(width int, eyebrow, title, subtitle string, badges ...string) string {
	cardStyle := m.styles.hero.Copy().Width(width)
	bodyWidth := max(1, width-cardStyle.GetHorizontalFrameSize())
	fill := styleWithBackground(m.styles.heroSubtitle, cardStyle.GetBackground())
	eyebrowStyle := styleWithBackground(m.styles.heroEyebrow, cardStyle.GetBackground())
	titleStyle := styleWithBackground(m.styles.heroTitle, cardStyle.GetBackground())
	lines := []string{
		boundedRendered(eyebrowStyle.Render(eyebrow), bodyWidth, eyebrowStyle),
		boundedRendered(titleStyle.Render(title), bodyWidth, titleStyle),
	}
	if strings.TrimSpace(subtitle) != "" {
		lines = append(lines, boundedRendered(fill.Render(wrapText(subtitle, bodyWidth)), bodyWidth, fill))
	}
	if badgeBlock := m.renderBadgeBlockWithFill(bodyWidth, fill, badges...); badgeBlock != "" {
		lines = append(lines, badgeBlock)
	}
	return cardStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (m model) renderSectionCard(width int, title, subtitle, body string, active bool) string {
	cardStyle := m.styles.panel
	if active {
		cardStyle = m.styles.panelActive
	}
	cardStyle = cardStyle.Copy().Width(width)
	bodyWidth := max(1, width-cardStyle.GetHorizontalFrameSize())
	fill := styleWithBackground(m.styles.panelSubtitle, cardStyle.GetBackground())
	titleStyle := styleWithBackground(m.styles.panelTitle, cardStyle.GetBackground())
	header := boundedRendered(titleStyle.Render(title), bodyWidth, titleStyle)
	lines := []string{header}
	if strings.TrimSpace(subtitle) != "" {
		lines = append(lines, boundedRendered(fill.Render(wrapText(subtitle, bodyWidth)), bodyWidth, fill))
	}
	if strings.TrimSpace(body) != "" {
		lines = append(lines, body)
	}
	return cardStyle.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (m model) renderDashboard(layout layoutSpec) string {
	dbTable := m.dbTable
	applyDBTableLayout(&dbTable, m.panelInnerWidth(layout.dashboardPanelWide))
	dbTable.SetHeight(max(6, layout.bodyHeight-12))

	snapshotTable := m.snapshotTable
	applySnapshotTableLayout(&snapshotTable, m.panelInnerWidth(layout.dashboardPanelWide))
	snapshotTable.SetHeight(max(6, layout.bodyHeight-12))

	dbInnerWidth := m.panelInnerWidth(layout.dashboardPanelWide)
	snapshotInnerWidth := m.panelInnerWidth(layout.dashboardPanelWide)
	dbBody := []string{
		m.renderBadgeBlockWithFill(dbInnerWidth, styleWithBackground(m.styles.subtle, m.styles.panel.GetBackground()),
			m.styles.badgeStrong.Render(fmt.Sprintf("%d live databases", len(dbTable.Rows()))),
		),
		m.renderTableOrEmpty(dbTable.View(), len(dbTable.Rows()) == 0, "No databases found. Press r to reload or open doctor.", dbInnerWidth),
	}
	if m.service.Config().NeedsOnboarding() {
		dbBody = append([]string{
			boundedRendered(
				styleWithBackground(m.styles.error, m.styles.panel.GetBackground()).Render("Setup is not saved yet. Press c to finish first-run setup."),
				dbInnerWidth,
				styleWithBackground(m.styles.error, m.styles.panel.GetBackground()),
			),
		}, dbBody...)
	}
	dbPanel := m.renderSectionCard(
		layout.dashboardPanelWide,
		"Live databases",
		"Capture from the MySQL instance running on this machine.",
		lipgloss.JoinVertical(lipgloss.Left, dbBody...),
		m.dashboardFocus == 0,
	)
	snapshotPanel := m.renderSectionCard(
		layout.dashboardPanelWide,
		"Snapshots",
		"Restore from an existing MySQL Shell dump on disk.",
		lipgloss.JoinVertical(
			lipgloss.Left,
			m.renderBadgeBlockWithFill(snapshotInnerWidth, styleWithBackground(m.styles.subtle, m.styles.panel.GetBackground()),
				m.styles.badgeStrong.Render(fmt.Sprintf("%d saved snapshots", len(snapshotTable.Rows()))),
			),
			m.renderTableOrEmpty(snapshotTable.View(), len(snapshotTable.Rows()) == 0, "No snapshots yet. Press s to create the first one.", snapshotInnerWidth),
		),
		m.dashboardFocus == 1,
	)

	var body string
	if layout.stackDashboard {
		body = lipgloss.JoinVertical(lipgloss.Left, dbPanel, snapshotPanel)
	} else {
		body = lipgloss.JoinHorizontal(lipgloss.Top, dbPanel, snapshotPanel)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		body,
	)
}

func (m model) renderFooter(layout layoutSpec) string {
	helpView := m.help.View(m.keys)
	switch m.screen {
	case screenOnboarding:
		helpView = "Type to edit. Enter or tab moves forward. Shift+tab moves back. Left/right or y/n changes Yes/No. Press enter on the last field to save."
	case screenSettings:
		helpView = "Type to edit. Enter or tab moves forward. Shift+tab moves back. Left/right or y/n changes Yes/No. Press enter on the last field to save."
	case screenConfirm:
		helpView = "Enter confirms. Tab or left/right switches between Yes and No. Esc cancels."
	}
	barStyle := m.styles.helpBar.Copy().Width(layout.contentWidth)
	bodyWidth := max(1, layout.contentWidth-barStyle.GetHorizontalFrameSize())
	fill := styleWithBackground(m.styles.subtle, barStyle.GetBackground())
	content := lipgloss.JoinHorizontal(
		lipgloss.Center,
		styleWithBackground(m.styles.helpLabel, barStyle.GetBackground()).Render("Keys"),
		" ",
		fill.Render(wrapText(helpView, max(1, bodyWidth-5))),
	)
	return barStyle.Render(boundedRendered(content, bodyWidth, fill))
}

func (m model) renderPicker(layout layoutSpec, title, subtitle, filterView, tableView string) string {
	width := layout.mainWidth
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
	filterCard := m.renderSectionCard(
		layout.mainWidth,
		"Filter",
		"Fuzzy match by database or snapshot name.",
		boundedRendered(m.styles.filter.Render(filterView), m.panelInnerWidth(layout.mainWidth), styleWithBackground(m.styles.filter, m.styles.panel.GetBackground())),
		m.filter.Focused(),
	)
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeadlineCard(width, "Workflow", title, subtitle, badges...),
		filterCard,
		m.renderSectionCard(
			layout.mainWidth,
			title,
			"Press enter to continue with the selected row.",
			m.renderTableOrEmpty(tableView, count == 0, emptyText, m.panelInnerWidth(layout.mainWidth)),
			true,
		),
	)
}

func (m model) renderRunning(layout layoutSpec) string {
	width := layout.mainWidth
	title := fmt.Sprintf("%s %s", m.spin.View(), screenLabelForJob(m.confirmState.action))
	if m.lastResult != nil {
		title = fmt.Sprintf("%s %s", m.spin.View(), screenLabelForJob(m.lastResult.Kind))
	}
	elapsed := time.Since(m.jobStartedAt).Round(timeSecond)
	badges := []string{
		m.styles.badgeStrong.Render(screenLabelForJob(m.confirmState.action)),
		m.styles.badge.Render("target " + blankFallback(m.confirmState.target, "-")),
		m.styles.badge.Render("elapsed " + elapsed.String()),
		m.jobStatusBadge(),
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeadlineCard(width, "Progress", title, "Streaming mysqlsh output in real time.", badges...),
		m.renderSectionCard(
			layout.mainWidth,
			"Job log",
			"Latest output from mysqlsh and supporting commands.",
			m.logViewport.View(),
			true,
		),
	)
}

func (m model) renderLogContent(width int) string {
	if width <= 0 {
		return strings.Join(m.jobLines, "\n")
	}
	lines := make([]string, 0, len(m.jobLines))
	for _, line := range m.jobLines {
		lines = append(lines, renderWrappedBounded(line, width, m.styles.value)...)
	}
	if len(lines) == 0 {
		return boundedLine("", width, m.styles.value)
	}
	return strings.Join(lines, "\n")
}

func (m model) renderConfirm(layout layoutSpec) string {
	modalWidth := clamp(layout.mainWidth-8, 36, min(72, layout.mainWidth))
	bodyWidth := max(1, modalWidth-m.styles.modal.GetHorizontalFrameSize())
	title := boundedRendered(m.styles.heroTitle.Render(m.confirmState.reason), bodyWidth, m.styles.heroTitle)
	description := boundedRendered(m.styles.value.Render(wrapText(m.confirmState.description, bodyWidth)), bodyWidth, m.styles.value)
	help := boundedRendered(m.styles.subtle.Render("Enter confirms. Tab or left/right switches. Esc cancels."), bodyWidth, m.styles.subtle)

	yesStyle := m.styles.badgeOK
	noStyle := m.styles.badgeGhost
	if !m.confirmValue {
		yesStyle = m.styles.badgeGhost
		noStyle = m.styles.badgeError
	}
	actions := lipgloss.JoinHorizontal(
		lipgloss.Center,
		yesStyle.Render("Yes"),
		"  ",
		noStyle.Render("No"),
	)

	modal := m.styles.modal.Copy().Width(modalWidth).Render(lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		description,
		help,
		actions,
	))
	return lipgloss.PlaceHorizontal(layout.mainWidth, lipgloss.Center, modal)
}

func (m model) renderResult(layout layoutSpec) string {
	width := m.panelInnerWidth(layout.mainWidth)
	title := "Result"
	statusBadge := m.styles.badgeOK.Render("completed")
	if m.lastErr != nil {
		title = "Error"
		statusBadge = m.styles.badgeError.Render("failed")
	}
	lines := []string{}
	if m.lastErr != nil {
		lines = append(lines, m.renderValueBlock("Latest error", m.lastErr.Error(), width, m.styles.error))
	} else if m.lastResult != nil {
		lines = append(lines, boundedRendered(m.styles.success.Render(fmt.Sprintf("%s finished for %s in %s.", screenLabelForJob(m.lastResult.Kind), m.lastResult.Target, m.lastResult.Duration.Round(timeSecond))), width, m.styles.success))
		lines = append(lines, m.renderValueBlock("Log file", m.lastResult.LogPath, width, m.styles.code))
		if len(m.jobSummary) > 0 {
			lines = append(lines, m.renderBulletBlock("Summary", m.jobSummary, width))
		}
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeadlineCard(layout.mainWidth, "Run complete", title, "Review the final outcome, the persistent log path, and any summary emitted by mysqlsh.", statusBadge),
		m.renderSectionCard(layout.mainWidth, "Outcome", "Everything from the latest run is captured here.", lipgloss.JoinVertical(lipgloss.Left, lines...), true),
	)
}

func (m model) renderDoctor(layout layoutSpec) string {
	width := m.panelInnerWidth(layout.mainWidth)
	blocks := []string{
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
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeadlineCard(
			layout.mainWidth,
			"Diagnostics",
			"Doctor",
			"Use this screen to verify tools, paths, and local MySQL connectivity before running jobs.",
			m.mysqlStatusBadge(),
			m.styles.badge.Render(fmt.Sprintf("%d missing tools", len(m.doctor.MissingCommands))),
			m.styles.badge.Render(fmt.Sprintf("%d warnings", len(m.doctor.Warnings))),
		),
		m.renderSectionCard(layout.mainWidth, "Environment report", "Current runtime state and detected defaults.", lipgloss.JoinVertical(lipgloss.Left, blocks...), true),
	)
}

func (m model) renderSettings(layout layoutSpec) string {
	onboarding := m.onboarding || m.screen == screenOnboarding
	title := "Settings"
	subtitle := "These saved values fill in the app the next time it starts. Environment variables can still override them."
	badges := []string{
		m.styles.badgeStrong.Render("saved defaults"),
		m.styles.badge.Render("used on next launch"),
		m.styles.badge.Render("env vars override"),
	}
	if onboarding {
		title = "Finish Setup"
		subtitle = "This is the only required step. If these defaults already match this machine, save once to continue. Change only the fields that differ."
		badges = []string{
			m.styles.badgeStrong.Render("required once"),
			m.styles.badge.Render("save to continue"),
			m.styles.badge.Render("edit if needed"),
		}
	}
	lines := []string{m.renderHeadlineCard(layout.mainWidth, "Configuration", title, subtitle, badges...)}
	if m.settingsForm != nil {
		lines = append(lines, m.renderSectionCard(
			layout.mainWidth,
			"Saved defaults",
			"These values seed the app and can still be overridden by environment variables.",
			boundedRendered(m.settingsForm.View(), m.panelInnerWidth(layout.mainWidth), lipgloss.NewStyle()),
			true,
		))
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
		title = "What To Do"
		blocks = append(blocks,
			m.renderValueBlock("Config file", m.service.Config().ConfigPath, contentWidth, m.styles.code),
			m.renderBulletBlock("Required now", []string{
				"Review the setup form.",
				"If the defaults already match this machine, leave them as-is.",
				"Save at the bottom to finish setup and open the dashboard.",
			}, contentWidth),
			m.renderBulletBlock("Only change fields if needed", []string{
				"MySQL host, socket, user, password, or login path are different on this machine.",
				"The Homebrew MySQL service name is different.",
				"You keep snapshots or logs in a different folder.",
			}, contentWidth),
			m.renderValueBlock("Connection check", m.onboardingConnectionGuidance(), contentWidth, m.styles.value),
		)
	}
	if m.lastErr != nil && m.screen != screenResult && m.screen != screenOnboarding {
		blocks = append(blocks, m.renderValueBlock("Latest error", m.lastErr.Error(), contentWidth, m.styles.error))
	}
	if len(blocks) == 0 {
		blocks = append(blocks, boundedRendered(m.styles.subtle.Render("Select a row to see more detail here."), contentWidth, m.styles.subtle))
	}
	subtitle := "Context for the currently selected item."
	if m.screen == screenOnboarding {
		subtitle = "What to review before saving setup."
	}
	return m.renderSectionCard(
		panelWidth,
		title,
		subtitle,
		lipgloss.JoinVertical(lipgloss.Left, blocks...),
		true,
	)
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
	styles.Header = lipgloss.NewStyle().
		Foreground(appStyles.panelSubtitle.GetForeground()).
		BorderBottom(true).
		BorderForeground(appStyles.panel.GetBorderTopForeground()).
		Bold(true)
	styles.Cell = lipgloss.NewStyle().
		Foreground(appStyles.value.GetForeground())
	styles.Selected = lipgloss.NewStyle().
		Foreground(appStyles.frame.GetBackground()).
		Background(appStyles.badgeStrong.GetBackground()).
		Bold(true)
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
	styles.Header = lipgloss.NewStyle().
		Foreground(appStyles.panelSubtitle.GetForeground()).
		BorderBottom(true).
		BorderForeground(appStyles.panel.GetBorderTopForeground()).
		Bold(true)
	styles.Cell = lipgloss.NewStyle().
		Foreground(appStyles.value.GetForeground())
	styles.Selected = lipgloss.NewStyle().
		Foreground(appStyles.frame.GetBackground()).
		Background(appStyles.badgeStrong.GetBackground()).
		Bold(true)
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

	contentWidth := width - m.styles.frame.GetHorizontalFrameSize() - 1
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
		bodyHeight:         max(12, height-10),
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
	fill := styleWithBackground(valueStyle, m.styles.panel.GetBackground())
	labelStyle := styleWithBackground(m.styles.label, m.styles.panel.GetBackground())
	labelText := labelStyle.Render(label + ":")
	valueText := wrapText(blankFallback(value, "-"), max(1, width))
	inlineWidth := max(1, width-lipgloss.Width(labelText)-1)
	inlineValue := wrapText(blankFallback(value, "-"), inlineWidth)
	if !strings.Contains(inlineValue, "\n") {
		return boundedRendered(
			lipgloss.JoinHorizontal(
				lipgloss.Top,
				labelText,
				" ",
				fill.Render(inlineValue),
			),
			width,
			fill,
		)
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		boundedRendered(labelText, width, labelStyle),
		boundedRendered(fill.Render(valueText), width, fill),
	)
}

func (m model) renderBadgeBlock(width int, badges ...string) string {
	return m.renderBadgeBlockWithFill(width, m.screenFillStyle(), badges...)
}

func (m model) renderBadgeBlockWithFill(width int, filler lipgloss.Style, badges ...string) string {
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
	rows := []string{}
	current := []string{}
	currentWidth := 0
	for _, badge := range filtered {
		badgeWidth := lipgloss.Width(badge)
		addedWidth := badgeWidth
		if len(current) > 0 {
			addedWidth += 1
		}
		if len(current) > 0 && currentWidth+addedWidth > width {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center, interleaveSpaces(current)...))
			current = nil
			currentWidth = 0
			addedWidth = badgeWidth
		}
		if len(current) > 0 {
			currentWidth++
		}
		current = append(current, badge)
		currentWidth += badgeWidth
	}
	if len(current) > 0 {
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Center, interleaveSpaces(current)...))
	}
	return boundedRendered(strings.Join(rows, "\n"), width, filler)
}

func (m model) renderBulletBlock(label string, items []string, width int) string {
	if len(items) == 0 {
		return ""
	}
	labelStyle := styleWithBackground(m.styles.label, m.styles.panel.GetBackground())
	valueStyle := styleWithBackground(m.styles.value, m.styles.panel.GetBackground())
	lines := make([]string, 0, len(items))
	for _, item := range items {
		lines = append(lines, wrapText("• "+item, width))
	}
	return lipgloss.JoinVertical(
		lipgloss.Left,
		boundedRendered(labelStyle.Render(label), width, labelStyle),
		boundedRendered(valueStyle.Render(strings.Join(lines, "\n")), width, valueStyle),
	)
}

func (m model) renderTableOrEmpty(tableView string, empty bool, message string, width int) string {
	if !empty {
		return tableView
	}
	fill := styleWithBackground(m.styles.subtle, m.styles.panel.GetBackground())
	return lipgloss.JoinVertical(
		lipgloss.Left,
		boundedRendered(fill.Render(wrapText(message, width)), width, fill),
		boundedRendered(fill.Render(wrapText("Open doctor if MySQL or the snapshot folders look out of sync.", width)), width, fill),
		boundedRendered(fill.Render(wrapText("Press r after external changes to refresh these lists without leaving the app.", width)), width, fill),
	)
}

func (m model) mysqlStatusBadge() string {
	if m.doctor.MySQLReachable {
		return m.styles.badgeOK.Render("MySQL online")
	}
	return m.styles.badgeWarn.Render("MySQL offline")
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

func (m model) onboardingConnectionGuidance() string {
	switch {
	case m.lastErr == nil && m.doctor.MySQLReachable:
		return "MySQL responded with the current settings. If the folders also look right, save and continue."
	case m.lastErr == nil:
		return "dbgold has not confirmed the current connection yet. If this machine should already reach MySQL, review the connection fields before saving."
	case !m.doctor.MySQLReachable:
		return "MySQL is not reachable with the current settings yet. If that is unexpected, check the host, socket, service, user, login path, or password before saving."
	default:
		return fmt.Sprintf("dbgold could not finish a MySQL check with the current values. %s", summarizeErrorBriefly(m.lastErr))
	}
}

func summarizeErrorBriefly(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return ""
	}
	if idx := strings.LastIndex(msg, "exit status "); idx >= 0 {
		return strings.TrimSpace(msg[idx:]) + "."
	}
	lines := strings.FieldsFunc(msg, func(r rune) bool {
		return r == '\n' || r == '\r'
	})
	if len(lines) == 0 {
		return msg
	}
	last := strings.TrimSpace(lines[len(lines)-1])
	if strings.HasPrefix(last, "exit status") {
		return last + "."
	}
	first := strings.TrimSpace(lines[0])
	if first == "" {
		return msg
	}
	return first
}

func newFormTheme(appStyles styles) huh.Theme {
	return huh.ThemeFunc(func(bool) *huh.Styles {
		theme := huh.ThemeCharm(true)
		theme.FieldSeparator = lipgloss.NewStyle().SetString("\n")
		theme.Group.Title = appStyles.panelTitle
		theme.Group.Description = appStyles.panelSubtitle
		theme.Focused.Base = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(appStyles.panelActive.GetBorderTopForeground()).
			Padding(0, 1)
		theme.Focused.Card = theme.Focused.Base
		theme.Focused.Title = appStyles.titleAccent
		theme.Focused.Description = appStyles.subtle
		theme.Focused.TextInput.Prompt = appStyles.titleAccent
		theme.Focused.TextInput.Text = appStyles.value
		theme.Focused.TextInput.Placeholder = appStyles.subtle
		theme.Focused.FocusedButton = appStyles.badgeStrong
		theme.Focused.BlurredButton = appStyles.badgeGhost
		theme.Focused.Next = appStyles.badgeStrong
		theme.Blurred = theme.Focused
		theme.Blurred.Base = lipgloss.NewStyle().PaddingLeft(1)
		theme.Blurred.Card = theme.Blurred.Base
		theme.Help.ShortKey = appStyles.titleAccent
		theme.Help.ShortDesc = appStyles.subtle
		theme.Help.ShortSeparator = appStyles.subtle
		theme.Help.Ellipsis = appStyles.subtle
		theme.Help.FullKey = appStyles.titleAccent
		theme.Help.FullDesc = appStyles.subtle
		theme.Help.FullSeparator = appStyles.subtle
		return theme
	})
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

func renderWrappedBounded(value string, width int, style lipgloss.Style) []string {
	width = safeWidth(width)
	if width <= 0 {
		return []string{value}
	}
	wrapped := lipgloss.Wrap(value, width, " /_-=:,.()[]{}")
	parts := strings.Split(wrapped, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, boundedLine(part, width, style))
	}
	if len(lines) == 0 {
		return []string{boundedLine("", width, style)}
	}
	return lines
}

func boundedRendered(rendered string, width int, filler lipgloss.Style) string {
	if width <= 0 {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		lines[i] = boundedLine(line, width, filler)
	}
	return strings.Join(lines, "\n")
}

func boundedBlock(rendered string, width, height int, filler lipgloss.Style) string {
	if height <= 0 {
		return boundedRendered(rendered, width, filler)
	}
	lines := strings.Split(boundedRendered(rendered, width, filler), "\n")
	switch {
	case len(lines) > height:
		lines = lines[:height]
	case len(lines) < height:
		blank := boundedLine("", width, filler)
		for len(lines) < height {
			lines = append(lines, blank)
		}
	}
	return strings.Join(lines, "\n")
}

func boundedLine(line string, width int, filler lipgloss.Style) string {
	if width <= 0 {
		return line
	}
	visibleWidth := lipgloss.Width(line)
	if visibleWidth > width {
		line = lipgloss.NewStyle().MaxWidth(width).Render(line)
		visibleWidth = lipgloss.Width(line)
	}
	if visibleWidth < width {
		line += filler.Render(strings.Repeat(" ", width-visibleWidth))
	}
	return line
}

func safeWidth(width int) int {
	if width <= 1 {
		return width
	}
	return width - 1
}

func styleWithBackground(style lipgloss.Style, background color.Color) lipgloss.Style {
	return style.Copy().Background(background)
}

func (m model) screenFillStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Background(m.styles.frame.GetBackground()).
		Foreground(m.styles.frame.GetForeground())
}

func interleaveSpaces(parts []string) []string {
	if len(parts) == 0 {
		return nil
	}
	out := make([]string, 0, len(parts)*2-1)
	for i, part := range parts {
		if i > 0 {
			out = append(out, " ")
		}
		out = append(out, part)
	}
	return out
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
