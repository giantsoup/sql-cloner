package app

import (
	"context"
	"io"
	"testing"

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
