package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	lipgloss "charm.land/lipgloss/v2"

	"github.com/taylor/dbgold/internal/core"
	"github.com/taylor/dbgold/internal/execx"
)

func TestDashboardNavigationEnter(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})
	m.dbs = []core.Database{{Name: "appdb", TableCount: 3, SizeBytes: 1024}}
	m.snapshots = []core.Snapshot{{Name: "appdb"}}
	m.syncTables()
	m.dashboardFocus = 1

	next, _ := m.handleEnter()
	updated := next.(model)
	if updated.screen != screenRestorePicker {
		t.Fatalf("expected restore picker, got %s", updated.screen)
	}
}

func TestFilterBehaviorForSnapshotPicker(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenSnapshotPicker})
	m.dbs = []core.Database{
		{Name: "alpha"},
		{Name: "customer-prod"},
	}
	m.filter.SetValue("cp")
	m.applyFilter()

	rows := m.dbTable.Rows()
	if len(rows) != 1 || rows[0][0] != "customer-prod" {
		t.Fatalf("unexpected filtered rows: %#v", rows)
	}
}

func TestConfirmCancelFlow(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenSnapshotPicker})
	m.openConfirm(confirmState{action: core.JobSnapshot, target: "appdb"})
	next, _ := m.handleConfirmDone(confirmDoneMsg{ok: false})
	updated := next.(model)
	if updated.screen != screenSnapshotPicker {
		t.Fatalf("expected snapshot picker after cancel, got %s", updated.screen)
	}
}

func TestConfirmDefaultsToYes(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenSnapshotPicker})
	m.openConfirm(confirmState{action: core.JobSnapshot, target: "appdb"})

	if !m.confirmValue {
		t.Fatal("expected confirm modal to default to Yes")
	}
}

func TestRunningResultTransition(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenRunning})
	result := &core.JobResult{Kind: core.JobSnapshot, Target: "appdb"}
	next, _ := m.Update(jobEventMsg{result: result})
	updated := next.(model)
	if updated.screen != screenResult {
		t.Fatalf("expected result screen, got %s", updated.screen)
	}
	if updated.lastResult == nil || updated.lastResult.Target != "appdb" {
		t.Fatalf("unexpected result %#v", updated.lastResult)
	}
}

func TestResponsiveLayoutKeepsPanelsInsideSmallViewport(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})
	m.width = 48
	m.height = 18
	m.resize()

	layout := m.layout()
	if layout.contentWidth > m.width-m.styles.frame.GetHorizontalFrameSize() {
		t.Fatalf("content width overflowed viewport: got %d", layout.contentWidth)
	}
	if m.dbTable.Width() > m.panelInnerWidth(layout.mainWidth) {
		t.Fatalf("db table wider than panel: got %d want <= %d", m.dbTable.Width(), m.panelInnerWidth(layout.mainWidth))
	}
	if !layout.stackSidebar {
		t.Fatalf("expected sidebar to stack on a narrow viewport")
	}
}

func TestResponsiveLayoutKeepsSidebarOnWideViewport(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})
	m.width = 160
	m.height = 40
	m.resize()

	layout := m.layout()
	if layout.stackSidebar {
		t.Fatalf("expected sidebar layout on a wide viewport")
	}
	if layout.mainWidth+layout.sidebarWidth+1 > layout.contentWidth {
		t.Fatalf("layout columns overflowed content width: main=%d sidebar=%d content=%d", layout.mainWidth, layout.sidebarWidth, layout.contentWidth)
	}
}

func TestOnboardingConnectionGuidanceForReachableMySQL(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenOnboarding})
	m.doctor.MySQLReachable = true

	msg := m.onboardingConnectionGuidance()
	if !strings.Contains(msg, "save and continue") {
		t.Fatalf("expected setup guidance to explain the next step, got %q", msg)
	}
}

func TestOnboardingConnectionGuidanceForConnectionFailure(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenOnboarding})
	m.lastErr = errors.New("mysql --host=127.0.0.1 ...\nexit status 1")

	msg := m.onboardingConnectionGuidance()
	if !strings.Contains(msg, "before saving") {
		t.Fatalf("expected setup guidance to explain what to fix, got %q", msg)
	}
}

func TestSummarizeErrorBrieflyReturnsExitStatus(t *testing.T) {
	err := errors.New("mysql --host=127.0.0.1 ...\nexit status 1")

	msg := summarizeErrorBriefly(err)
	if msg != "exit status 1." {
		t.Fatalf("unexpected summarized error %q", msg)
	}
}

func TestSummarizeErrorBrieflyReturnsInlineExitStatus(t *testing.T) {
	err := errors.New("mysql --host=127.0.0.1 ... exit status 1")

	msg := summarizeErrorBriefly(err)
	if msg != "exit status 1." {
		t.Fatalf("unexpected summarized error %q", msg)
	}
}

func TestRenderSettingsUsesOnboardingScreenState(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenOnboarding})
	m.screen = screenOnboarding
	m.onboarding = false
	m.width = 120
	m.height = 30
	m.resize()

	view := m.renderSettings(m.layout())
	if !strings.Contains(view, "Finish Setup") {
		t.Fatalf("expected onboarding title in setup screen, got %q", view)
	}
}

func TestNewModelPreparesOnboardingForm(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenOnboarding})

	if m.settingsForm == nil {
		t.Fatal("expected onboarding form to be prepared in the initial model")
	}
	if m.screen != screenOnboarding {
		t.Fatalf("expected onboarding screen, got %s", m.screen)
	}
}

func TestOnboardingRenderShowsInputFields(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenOnboarding})
	m.width = 140
	m.height = 40
	m.resize()

	view := m.renderSettings(m.layout())
	if !strings.Contains(view, "Snapshot folder") {
		t.Fatalf("expected onboarding form to render input fields, got %q", view)
	}
}

func TestViewUsesAppFrameColorsForTerminalSurface(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})

	view := m.View()
	if view.BackgroundColor != m.styles.frame.GetBackground() {
		t.Fatalf("expected terminal background color to match frame background")
	}
	if view.ForegroundColor != m.styles.frame.GetForeground() {
		t.Fatalf("expected terminal foreground color to match frame foreground")
	}
}

func TestRenderValueBlockCompactsShortValues(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})

	block := m.renderValueBlock("Database", "appdb", 40, m.styles.value)
	if strings.Contains(block, "\n") {
		t.Fatalf("expected short values to render inline, got %q", block)
	}
}

func TestRenderValueBlockWrapsLongValues(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})

	block := m.renderValueBlock("Snapshot root", "/a/very/long/path/that/should/not/fit/on/one/line", 18, m.styles.code)
	if !strings.Contains(block, "\n") {
		t.Fatalf("expected long values to wrap onto multiple lines, got %q", block)
	}
}

func TestDBTableViewFitsConfiguredWidth(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})
	m.dbs = []core.Database{
		{Name: "old_ufbacms_v3", TableCount: 15090797, SizeBytes: 4 << 30},
		{Name: "ufbacms_v3_codex_dusk", TableCount: 12967822, SizeBytes: 3 << 30},
	}

	width := 58
	applyDBTableLayout(&m.dbTable, width)
	m.dbTable.SetHeight(8)
	m.syncTables()

	if got := lipgloss.Width(m.dbTable.View()); got > width {
		t.Fatalf("expected db table width <= %d, got %d\n%s", width, got, m.dbTable.View())
	}
}

func TestSnapshotTableViewFitsConfiguredWidth(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})
	m.snapshots = []core.Snapshot{
		{Name: "ufbacms_v3_codex_dusk", UpdatedAt: testNow(), SizeBytes: 3 << 30},
	}

	width := 58
	applySnapshotTableLayout(&m.snapshotTable, width)
	m.snapshotTable.SetHeight(8)
	m.syncTables()

	if got := lipgloss.Width(m.snapshotTable.View()); got > width {
		t.Fatalf("expected snapshot table width <= %d, got %d\n%s", width, got, m.snapshotTable.View())
	}
}

func TestRenderConfirmUsesAppStyledActions(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})
	m.confirmState = confirmState{
		reason:      "Create a new snapshot?",
		description: "If this succeeds, the saved snapshot for appdb will be replaced.",
	}
	m.confirmValue = true
	m.width = 120
	m.height = 30
	m.resize()

	view := m.renderConfirm(m.layout())
	if !strings.Contains(view, "Create a new snapshot?") {
		t.Fatalf("expected confirm title in view, got %q", view)
	}
	if !strings.Contains(view, "Enter confirms.") {
		t.Fatalf("expected confirm help text in view, got %q", view)
	}
}

func TestRenderLogContentBoundsEveryLineToViewportWidth(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenRunning})
	m.jobLines = []string{
		"Preparing parallel dump for 'thirdufba' into '/opt/homebrew/var/db_snapshots/mysqlsh/thirdufba.tmp.2530189181'.",
		"NOTE: Table statistics not available for `thirdufba`.`categories`, chunking operation may not be optimal. Please consider running `ANALYZE TABLE `thirdufba`.`categories`;` first.",
	}

	rendered := m.renderLogContent(64)
	for _, line := range strings.Split(rendered, "\n") {
		if got := lipgloss.Width(line); got > safeWidth(64) {
			t.Fatalf("expected rendered log line width <= %d, got %d\n%q", safeWidth(64), got, line)
		}
	}
}

func TestRefreshLogViewportWrapsToCurrentWidth(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenRunning})
	m.logViewport.SetWidth(48)
	m.jobLines = []string{
		"Preparing parallel dump for 'thirdufba' into '/opt/homebrew/var/db_snapshots/mysqlsh/thirdufba.tmp.2530189181'.",
	}

	m.refreshLogViewport()
	for _, line := range strings.Split(m.logViewport.View(), "\n") {
		if got := lipgloss.Width(line); got > 48 {
			t.Fatalf("expected viewport line width <= 48, got %d\n%q", got, line)
		}
	}
}

func TestRenderBadgeBlockWrapsWithinWidth(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})

	rendered := m.renderBadgeBlock(28,
		m.styles.badgeStrong.Render("123 databases"),
		m.styles.badge.Render("44 snapshots"),
		m.styles.badgeWarn.Render("mysql offline"),
	)

	for _, line := range strings.Split(rendered, "\n") {
		if got := lipgloss.Width(line); got > 28 {
			t.Fatalf("expected badge line width <= 28, got %d\n%q", got, line)
		}
	}
}

func TestRenderTableOrEmptyWrapsWithinWidth(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})

	rendered := m.renderTableOrEmpty("", true, "No databases found. Press r to reload or open doctor.", 24)
	for _, line := range strings.Split(rendered, "\n") {
		if got := lipgloss.Width(line); got > 24 {
			t.Fatalf("expected empty-state line width <= 24, got %d\n%q", got, line)
		}
	}
}

func TestRenderRunningFitsWithinMainWidth(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenRunning})
	m.width = 140
	m.height = 36
	m.confirmState = confirmState{action: core.JobSnapshot, target: "old_ufbacms_v3"}
	m.jobStartedAt = time.Now().Add(-13 * time.Second)
	m.jobStatus = "writing table metadata - done"
	m.jobLines = []string{
		"Preparing parallel dump for 'old_ufbacms_v3' into '/opt/homebrew/var/db_snapshots/mysqlsh/old_ufbacms_v3.tmp.1183259261'.",
		"NOTE: Table statistics not available for `old_ufbacms_v3`.`categories`, chunking operation may not be optimal. Please consider running `ANALYZE TABLE `old_ufbacms_v3`.`categories`;` first.",
	}
	m.resize()

	rendered := m.renderRunning(m.layout())
	for _, line := range strings.Split(rendered, "\n") {
		if got := lipgloss.Width(line); got > m.layout().mainWidth {
			t.Fatalf("expected running screen line width <= %d, got %d\n%q", m.layout().mainWidth, got, line)
		}
	}
}

func TestRenderRunningScreenFitsWithinContentWidth(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenRunning})
	m.width = 140
	m.height = 36
	m.confirmState = confirmState{action: core.JobSnapshot, target: "old_ufbacms_v3"}
	m.jobStartedAt = time.Now().Add(-13 * time.Second)
	m.jobStatus = "writing table metadata - done"
	m.jobLines = []string{
		"Preparing parallel dump for 'old_ufbacms_v3' into '/opt/homebrew/var/db_snapshots/mysqlsh/old_ufbacms_v3.tmp.1183259261'.",
		"NOTE: Table statistics not available for `old_ufbacms_v3`.`categories`, chunking operation may not be optimal. Please consider running `ANALYZE TABLE `old_ufbacms_v3`.`categories`;` first.",
	}
	m.resize()

	rendered := m.render()
	for _, line := range strings.Split(rendered, "\n") {
		if got := lipgloss.Width(line); got > m.layout().contentWidth {
			t.Fatalf("expected full running screen line width <= %d, got %d\n%q", m.layout().contentWidth, got, line)
		}
	}
}

func TestViewFillsContentWidthForRunningScreen(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenRunning})
	m.width = 140
	m.height = 36
	m.confirmState = confirmState{action: core.JobSnapshot, target: "old_ufbacms_v3"}
	m.jobStartedAt = time.Now().Add(-13 * time.Second)
	m.jobStatus = "writing table metadata - done"
	m.jobLines = []string{
		"Preparing parallel dump for 'old_ufbacms_v3' into '/opt/homebrew/var/db_snapshots/mysqlsh/old_ufbacms_v3.tmp.1183259261'.",
	}
	m.resize()

	view := m.View()
	for _, line := range strings.Split(view.Content, "\n") {
		if got := lipgloss.Width(line); got > m.width {
			t.Fatalf("expected view line width <= %d, got %d\n%q", m.width, got, line)
		}
	}
}

func TestLayoutReservesLastTerminalColumn(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})
	m.width = 140
	m.height = 36

	layout := m.layout()
	if layout.contentWidth != m.width-m.styles.frame.GetHorizontalFrameSize()-1 {
		t.Fatalf("expected reserved last column, got content width %d", layout.contentWidth)
	}
}

func TestSafeWidthLeavesOneColumnWhenPossible(t *testing.T) {
	if got := safeWidth(20); got != 19 {
		t.Fatalf("expected safe width 19, got %d", got)
	}
	if got := safeWidth(1); got != 1 {
		t.Fatalf("expected safe width 1, got %d", got)
	}
}

func TestBoundedRenderedUsesProvidedWidthWithoutShrinking(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})

	rendered := boundedRendered("abc", 5, m.styles.value)
	if got := lipgloss.Width(rendered); got != 5 {
		t.Fatalf("expected bounded width 5, got %d: %q", got, rendered)
	}
}

func TestViewFillsTerminalHeightForShorterScreen(t *testing.T) {
	m := newModel(context.Background(), testService(t), launchOptions{mode: screenResult})
	m.width = 140
	m.height = 36
	m.lastResult = &core.JobResult{
		Kind:     core.JobSnapshot,
		Target:   "mainsite",
		Duration: 4 * time.Second,
		LogPath:  "/opt/homebrew/var/db_snapshots/mysqlsh/_logs/mainsite.snapshot.log",
	}
	m.jobSummary = []string{"1 schemas will be dumped and within them 11 tables, 0 views."}
	m.resize()

	view := m.View()
	if got := len(strings.Split(view.Content, "\n")); got != m.height {
		t.Fatalf("expected view height %d, got %d", m.height, got)
	}
}

func TestAllScreensStayWithinViewportBounds(t *testing.T) {
	tests := []struct {
		name  string
		model func(t *testing.T) model
	}{
		{
			name: "dashboard",
			model: func(t *testing.T) model {
				m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})
				m.width = 140
				m.height = 36
				m.dbs = []core.Database{{Name: "mainsite", TableCount: 11, SizeBytes: 1770}, {Name: "old_ufbacms_v3", TableCount: 15090797, SizeBytes: 4 << 30}}
				m.snapshots = []core.Snapshot{{Name: "mainsite", UpdatedAt: testNow(), SizeBytes: 1770}}
				m.doctor.MySQLReachable = true
				m.syncTables()
				m.resize()
				return m
			},
		},
		{
			name: "snapshot-picker",
			model: func(t *testing.T) model {
				m := newModel(context.Background(), testService(t), launchOptions{mode: screenSnapshotPicker})
				m.width = 140
				m.height = 36
				m.dbs = []core.Database{{Name: "customer-prod", TableCount: 200, SizeBytes: 8 << 20}, {Name: "mainsite", TableCount: 11, SizeBytes: 1770}}
				m.filter.SetValue("cp")
				m.syncTables()
				m.resize()
				return m
			},
		},
		{
			name: "restore-picker",
			model: func(t *testing.T) model {
				m := newModel(context.Background(), testService(t), launchOptions{mode: screenRestorePicker})
				m.width = 140
				m.height = 36
				m.snapshots = []core.Snapshot{{Name: "ufbacms_v3_codex_dusk", UpdatedAt: testNow(), SizeBytes: 3 << 30}}
				m.filter.SetValue("codex")
				m.syncTables()
				m.resize()
				return m
			},
		},
		{
			name: "confirm",
			model: func(t *testing.T) model {
				m := newModel(context.Background(), testService(t), launchOptions{mode: screenDashboard})
				m.width = 140
				m.height = 36
				_ = m.openConfirm(confirmState{
					reason:      "Restore this snapshot?",
					description: "The local database mainsite will be dropped, recreated, and loaded from disk.",
					action:      core.JobRestore,
					target:      "mainsite",
				})
				m.resize()
				return m
			},
		},
		{
			name: "running",
			model: func(t *testing.T) model {
				m := newModel(context.Background(), testService(t), launchOptions{mode: screenRunning})
				m.width = 140
				m.height = 36
				m.confirmState = confirmState{action: core.JobSnapshot, target: "old_ufbacms_v3"}
				m.jobStartedAt = time.Now().Add(-13 * time.Second)
				m.jobStatus = "writing table metadata - done"
				m.jobLines = []string{
					"Preparing parallel dump for 'old_ufbacms_v3' into '/opt/homebrew/var/db_snapshots/mysqlsh/old_ufbacms_v3.tmp.1183259261'.",
					"NOTE: Table statistics not available for `old_ufbacms_v3`.`categories`, chunking operation may not be optimal. Please consider running `ANALYZE TABLE `old_ufbacms_v3`.`categories`;` first.",
				}
				m.resize()
				m.refreshLogViewport()
				return m
			},
		},
		{
			name: "result",
			model: func(t *testing.T) model {
				m := newModel(context.Background(), testService(t), launchOptions{mode: screenResult})
				m.width = 140
				m.height = 36
				m.lastResult = &core.JobResult{
					Kind:     core.JobSnapshot,
					Target:   "mainsite",
					Duration: 4 * time.Second,
					LogPath:  "/opt/homebrew/var/db_snapshots/mysqlsh/_logs/mainsite.snapshot.log",
				}
				m.jobSummary = []string{
					"1 schemas will be dumped and within them 11 tables, 0 views.",
					"100% (171 rows / ~170 rows), 0.00 rows/s, 0.00 B/s uncompressed, 0.00 B/s compressed",
				}
				m.resize()
				return m
			},
		},
		{
			name: "doctor",
			model: func(t *testing.T) model {
				m := newModel(context.Background(), testService(t), launchOptions{mode: screenDoctor})
				m.width = 140
				m.height = 36
				m.doctor = core.DoctorReport{
					MySQLReachable:  true,
					MySQLService:    "mysql@8.0",
					MySQLSocket:     "/tmp/mysql.sock",
					MySQLVersion:    "8.0.39",
					SnapshotRoot:    "/opt/homebrew/var/db_snapshots/mysqlsh",
					LogRoot:         "/opt/homebrew/var/db_snapshots/mysqlsh/_logs",
					MissingCommands: []string{"brew"},
					Warnings:        []string{"snapshot root does not exist yet"},
				}
				m.resize()
				return m
			},
		},
		{
			name: "settings",
			model: func(t *testing.T) model {
				m := newModel(context.Background(), testService(t), launchOptions{mode: screenSettings})
				m.width = 140
				m.height = 40
				m.resize()
				return m
			},
		},
		{
			name: "onboarding",
			model: func(t *testing.T) model {
				m := newModel(context.Background(), testService(t), launchOptions{mode: screenOnboarding})
				m.width = 140
				m.height = 40
				m.resize()
				return m
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.model(t)
			assertViewWithinBounds(t, m)
		})
	}
}

func assertViewWithinBounds(t *testing.T, m model) {
	t.Helper()
	view := m.View()
	lines := strings.Split(view.Content, "\n")
	if len(lines) != m.height {
		t.Fatalf("expected view height %d, got %d", m.height, len(lines))
	}
	for _, line := range lines {
		if got := lipgloss.Width(line); got > m.width {
			t.Fatalf("expected view line width <= %d, got %d\n%q", m.width, got, line)
		}
	}
}

func testService(t *testing.T) *core.Service {
	t.Helper()
	cfg := core.Config{
		SnapshotRoot: "/tmp/snapshots",
		LogRoot:      "/tmp/snapshots/_logs",
		MySQLSocket:  "/tmp/mysql.sock",
		MySQLUser:    "root",
		MySQLService: "mysql@8.0",
	}
	return core.NewService(cfg, noopRunner{}, core.NewLogger(io.Discard, false))
}

type noopRunner struct{}

func (noopRunner) Run(context.Context, execx.Command) (execx.Result, error) {
	return execx.Result{}, nil
}

func (noopRunner) Stream(context.Context, execx.Command, execx.StreamHandler) (execx.Result, error) {
	return execx.Result{}, nil
}

func testNow() time.Time {
	return time.Date(2026, time.March, 12, 13, 40, 0, 0, time.Local)
}
