package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"strait/internal/cli/client"
	clitui "strait/internal/cli/tui"
	"strait/internal/domain"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/spf13/cobra"
)

func newTUICommand(state *appState) *cobra.Command {
	var projectID string
	var interval time.Duration
	var runLimit int
	var eventLimit int

	cmd := &cobra.Command{
		Use:     "tui",
		Short:   "Launch interactive terminal dashboard",
		Long:    "Opens a live terminal UI with queue metrics, run explorer, and event timeline.",
		Example: "strait tui --project proj_1\n  strait tui --interval 3s --run-limit 30",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if interval <= 0 {
				return fmt.Errorf("interval must be greater than zero")
			}
			if runLimit <= 0 {
				return fmt.Errorf("run-limit must be greater than zero")
			}
			if eventLimit <= 0 {
				return fmt.Errorf("event-limit must be greater than zero")
			}

			if projectID == "" {
				projectID = state.opts.projectID
			}

			cli, err := newAPIClient(state)
			if err != nil {
				return err
			}

			d := newTUIDashboard(cli, projectID, runLimit, eventLimit)
			return d.run(cmd.Context(), interval)
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID for run explorer panel")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "refresh interval")
	cmd.Flags().IntVar(&runLimit, "run-limit", 50, "max runs to display")
	cmd.Flags().IntVar(&eventLimit, "event-limit", 25, "max events shown for selected run")

	return cmd
}

// tuiDashboard holds the state and widgets for the interactive terminal UI.
type tuiDashboard struct {
	cli        *client.Client
	projectID  string
	runLimit   int
	eventLimit int

	app        *tview.Application
	statsView  *tview.TextView
	runsTable  *tview.Table
	detailView *tview.TextView
	layout     *tview.Flex

	helpOverlay *tview.TextView
	pages       *tview.Pages

	mu            sync.Mutex
	selectedRunID string
	rowsByIndex   map[int]runRow
	showingHelp   bool
}

type runRow struct {
	ID     string
	Status domain.RunStatus
}

func newTUIDashboard(cli *client.Client, projectID string, runLimit, eventLimit int) *tuiDashboard {
	app := tview.NewApplication()

	header := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetText("[::b]Strait TUI[::-]  [yellow]?[-]:help  [yellow]Tab[-]:focus  [yellow]j/k[-]:nav  [yellow]t[-]:trigger  [yellow]c[-]:cancel  [yellow]r[-]:refresh  [yellow]q[-]:quit")

	statsView := tview.NewTextView().SetDynamicColors(true)
	statsView.SetBorder(true)
	statsView.SetTitle(" Queue ")

	runsTable := tview.NewTable().
		SetBorders(false).
		SetSelectable(true, false).
		SetFixed(1, 0)
	runsTable.SetBorder(true).SetTitle(" Runs ")

	detailView := tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	detailView.SetBorder(true)
	detailView.SetTitle(" Run Detail / Timeline ")

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 2, 0, false).
		AddItem(statsView, 5, 0, false).
		AddItem(runsTable, 0, 3, true).
		AddItem(detailView, 0, 2, false)

	helpOverlay := tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	helpOverlay.SetBorder(true).SetTitle(" Help ")
	helpOverlay.SetText(clitui.FormatHelp())

	pages := tview.NewPages().
		AddPage("main", layout, true, true).
		AddPage("help", helpCenter(helpOverlay), true, false)

	return &tuiDashboard{
		cli:         cli,
		projectID:   projectID,
		runLimit:    runLimit,
		eventLimit:  eventLimit,
		app:         app,
		statsView:   statsView,
		runsTable:   runsTable,
		detailView:  detailView,
		layout:      layout,
		helpOverlay: helpOverlay,
		pages:       pages,
		rowsByIndex: map[int]runRow{},
	}
}

// run starts the TUI event loop with periodic refresh.
func (d *tuiDashboard) run(ctx context.Context, interval time.Duration) error {
	d.runsTable.SetSelectionChangedFunc(func(row, _ int) {
		if row <= 0 {
			return
		}
		r, ok := d.rowsByIndex[row]
		if !ok {
			return
		}
		d.mu.Lock()
		d.selectedRunID = r.ID
		d.mu.Unlock()
		d.updateRunDetail(ctx, r.ID)
	})

	if err := d.refresh(ctx); err != nil {
		return err
	}

	done := make(chan struct{})
	go d.refreshLoop(ctx, interval, done)
	d.setupInputCapture(done)

	if err := d.app.SetRoot(d.pages, true).SetFocus(d.runsTable).Run(); err != nil {
		close(done)
		return err
	}

	return nil
}

// updateRunDetail fetches and displays the detail panel for a given run.
func (d *tuiDashboard) updateRunDetail(ctx context.Context, runID string) {
	if runID == "" {
		d.detailView.SetText("Select a run to inspect details.")
		return
	}

	run, runErr := d.cli.GetRun(ctx, runID)
	events, eventsErr := d.cli.ListRunEvents(ctx, runID, "", "")

	if runErr != nil {
		d.detailView.SetText(fmt.Sprintf("[red]run fetch error[-]: %v", runErr))
		return
	}
	if eventsErr != nil {
		d.detailView.SetText(fmt.Sprintf("[red]event fetch error[-]: %v", eventsErr))
		return
	}

	if len(events) > d.eventLimit {
		events = events[len(events)-d.eventLimit:]
	}

	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "[::b]Run[::-] %s\n", run.ID)
	_, _ = fmt.Fprintf(&b, "status: [yellow]%s[-]  attempt: %d  triggered_by: %s\n", run.Status, run.Attempt, run.TriggeredBy)
	_, _ = fmt.Fprintf(&b, "created: %s\n", run.CreatedAt.UTC().Format(time.RFC3339))
	if run.StartedAt != nil {
		_, _ = fmt.Fprintf(&b, "started: %s\n", run.StartedAt.UTC().Format(time.RFC3339))
	}
	if run.FinishedAt != nil {
		_, _ = fmt.Fprintf(&b, "finished: %s\n", run.FinishedAt.UTC().Format(time.RFC3339))
	}
	if run.Error != "" {
		_, _ = fmt.Fprintf(&b, "error: [red]%s[-]\n", run.Error)
	}

	b.WriteString("\n[::b]Recent Events[::-]\n")
	if len(events) == 0 {
		b.WriteString("(none)\n")
	}
	for _, e := range events {
		_, _ = fmt.Fprintf(&b, "%s  [%s/%s] %s\n", e.CreatedAt.UTC().Format(time.RFC3339), e.Level, e.Type, e.Message)
	}

	d.detailView.SetText(b.String())
}

// refresh fetches current stats and runs, then updates the UI.
func (d *tuiDashboard) refresh(ctx context.Context) error {
	stats, statsErr := d.cli.Stats(ctx)
	if statsErr != nil {
		return statsErr
	}

	runs := make([]domain.JobRun, 0)
	if d.projectID != "" {
		var runsErr error
		runs, runsErr = d.cli.ListRuns(ctx, d.projectID, "", d.runLimit, nil)
		if runsErr != nil {
			return runsErr
		}
	}

	d.app.QueueUpdateDraw(func() {
		d.statsView.SetText(fmt.Sprintf(
			"sampled: %s\nqueued: [yellow]%d[-]\nexecuting: [green]%d[-]\ndelayed: [orange]%d[-]",
			time.Now().UTC().Format(time.RFC3339), stats.Queued, stats.Executing, stats.Delayed,
		))

		d.runsTable.Clear()
		d.rowsByIndex = map[int]runRow{}
		headers := []string{"ID", "STATUS", "ATTEMPT", "CREATED"}
		for i, h := range headers {
			d.runsTable.SetCell(0, i, tview.NewTableCell(h).SetSelectable(false).SetAttributes(tcellAttrBold()))
		}

		if d.projectID == "" {
			d.runsTable.SetCell(1, 0, tview.NewTableCell("set --project or global --project to load runs"))
		} else {
			for i, run := range runs {
				row := i + 1
				d.runsTable.SetCell(row, 0, tview.NewTableCell(run.ID))
				d.runsTable.SetCell(row, 1, tview.NewTableCell(tviewStatusColor(run.Status)))
				d.runsTable.SetCell(row, 2, tview.NewTableCell(fmt.Sprintf("%d", run.Attempt)))
				d.runsTable.SetCell(row, 3, tview.NewTableCell(run.CreatedAt.UTC().Format(time.RFC3339)))
				d.rowsByIndex[row] = runRow{ID: run.ID, Status: run.Status}
			}
		}

		d.mu.Lock()
		current := d.selectedRunID
		d.mu.Unlock()
		if current == "" && len(d.rowsByIndex) > 0 {
			current = d.rowsByIndex[1].ID
			d.runsTable.Select(1, 0)
		}
		d.updateRunDetail(ctx, current)
	})

	return nil
}

// refreshLoop runs refresh on a timer until done is closed.
func (d *tuiDashboard) refreshLoop(ctx context.Context, interval time.Duration, done <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			_ = d.refresh(ctx)
		}
	}
}

// setupInputCapture configures keyboard shortcuts for the TUI.
func (d *tuiDashboard) setupInputCapture(done chan struct{}) {
	focusOrder := []tview.Primitive{d.runsTable, d.detailView}
	focusIndex := 0

	d.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyTab:
			focusIndex = (focusIndex + 1) % len(focusOrder)
			d.app.SetFocus(focusOrder[focusIndex])
			return nil
		case tcell.KeyCtrlC:
			close(done)
			d.app.Stop()
			return nil
		default:
			// pass through all other keys
		}

		switch event.Rune() {
		case '?':
			d.showingHelp = !d.showingHelp
			if d.showingHelp {
				d.pages.ShowPage("help")
				d.app.SetFocus(d.helpOverlay)
			} else {
				d.pages.HidePage("help")
				d.app.SetFocus(d.runsTable)
			}
			return nil
		case 'q':
			if d.showingHelp {
				d.showingHelp = false
				d.pages.HidePage("help")
				d.app.SetFocus(d.runsTable)
				return nil
			}
			close(done)
			d.app.Stop()
			return nil
		case 'r':
			_ = d.refresh(context.Background())
			return nil
		case 't':
			d.mu.Lock()
			runID := d.selectedRunID
			d.mu.Unlock()
			if runID != "" {
				go d.triggerSelectedJob(context.Background(), runID)
			}
			return nil
		case 'c':
			d.mu.Lock()
			runID := d.selectedRunID
			d.mu.Unlock()
			if runID != "" {
				go d.cancelSelectedRun(context.Background(), runID)
			}
			return nil
		case 'j':
			row, col := d.runsTable.GetSelection()
			if row < d.runsTable.GetRowCount()-1 {
				d.runsTable.Select(row+1, col)
			}
			return nil
		case 'k':
			row, col := d.runsTable.GetSelection()
			if row > 1 {
				d.runsTable.Select(row-1, col)
			}
			return nil
		}

		return event
	})
}

func tcellAttrBold() tcell.AttrMask {
	return tcell.AttrBold
}

// tviewStatusColor wraps a run status in tview color tags.
// helpCenter creates a centered help overlay using tview Pages.
func helpCenter(content *tview.TextView) *tview.Flex {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(content, 20, 1, true).
			AddItem(nil, 0, 1, false),
			60, 1, true).
		AddItem(nil, 0, 1, false)
}

// triggerSelectedJob re-triggers the job associated with the selected run.
func (d *tuiDashboard) triggerSelectedJob(ctx context.Context, runID string) {
	run, err := d.cli.GetRun(ctx, runID)
	if err != nil {
		d.detailView.SetText(fmt.Sprintf("[red]trigger error[-]: %v", err))
		return
	}

	_, triggerErr := d.cli.TriggerJob(ctx, run.JobID, client.TriggerJobRequest{
		Payload: run.Payload,
	}, "")
	if triggerErr != nil {
		d.detailView.SetText(fmt.Sprintf("[red]trigger error[-]: %v", triggerErr))
		return
	}

	d.detailView.SetText(fmt.Sprintf("[green]triggered job %s from run %s[-]", run.JobID, runID))
	_ = d.refresh(ctx)
}

// cancelSelectedRun cancels the selected run.
func (d *tuiDashboard) cancelSelectedRun(ctx context.Context, runID string) {
	_, err := d.cli.CancelRun(ctx, runID)
	if err != nil {
		d.detailView.SetText(fmt.Sprintf("[red]cancel error[-]: %v", err))
		return
	}

	d.detailView.SetText(fmt.Sprintf("[yellow]canceled run %s[-]", runID))
	_ = d.refresh(ctx)
}

func tviewStatusColor(status domain.RunStatus) string {
	switch status {
	case domain.StatusCompleted:
		return "[green]" + string(status) + "[-]"
	case domain.StatusFailed, domain.StatusSystemFailed, domain.StatusCrashed:
		return "[red]" + string(status) + "[-]"
	case domain.StatusExecuting, domain.StatusQueued, domain.StatusDequeued:
		return "[yellow]" + string(status) + "[-]"
	case domain.StatusDelayed, domain.StatusWaiting:
		return "[blue]" + string(status) + "[-]"
	case domain.StatusCanceled, domain.StatusExpired, domain.StatusTimedOut:
		return "[gray]" + string(status) + "[-]"
	default:
		return string(status)
	}
}
