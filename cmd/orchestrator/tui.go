package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"orchestrator/internal/domain"

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
		Example: "orchestrator tui --project proj_1\n  orchestrator tui --interval 3s --run-limit 30",
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

			app := tview.NewApplication()

			header := tview.NewTextView().
				SetDynamicColors(true).
				SetWrap(true).
				SetText("[::b]Orchestrator TUI[::-]  [yellow]Tab[-]:focus  [yellow]j/k[-]:nav  [yellow]Enter[-]:inspect  [yellow]r[-]:refresh  [yellow]q[-]:quit")

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

			type runRow struct {
				ID     string
				Status domain.RunStatus
			}
			rowsByIndex := map[int]runRow{}
			selectedRunID := ""
			var selectedMu sync.Mutex

			updateRunDetail := func(runID string) {
				if runID == "" {
					detailView.SetText("Select a run to inspect details.")
					return
				}

				run, runErr := cli.GetRun(cmd.Context(), runID)
				events, eventsErr := cli.ListRunEvents(cmd.Context(), runID, "", "")

				if runErr != nil {
					detailView.SetText(fmt.Sprintf("[red]run fetch error[-]: %v", runErr))
					return
				}
				if eventsErr != nil {
					detailView.SetText(fmt.Sprintf("[red]event fetch error[-]: %v", eventsErr))
					return
				}

				if len(events) > eventLimit {
					events = events[len(events)-eventLimit:]
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

				detailView.SetText(b.String())
			}

			runsTable.SetSelectionChangedFunc(func(row, _ int) {
				if row <= 0 {
					return
				}
				r, ok := rowsByIndex[row]
				if !ok {
					return
				}
				selectedMu.Lock()
				selectedRunID = r.ID
				selectedMu.Unlock()
				updateRunDetail(r.ID)
			})

			refresh := func() error {
				stats, statsErr := cli.Stats(cmd.Context())
				if statsErr != nil {
					return statsErr
				}

				runs := make([]domain.JobRun, 0)
				if projectID != "" {
					var runsErr error
					runs, runsErr = cli.ListRuns(cmd.Context(), projectID, "", runLimit, nil)
					if runsErr != nil {
						return runsErr
					}
				}

				app.QueueUpdateDraw(func() {
					statsView.SetText(fmt.Sprintf(
						"sampled: %s\nqueued: [yellow]%d[-]\nexecuting: [green]%d[-]\ndelayed: [orange]%d[-]",
						time.Now().UTC().Format(time.RFC3339), stats.Queued, stats.Executing, stats.Delayed,
					))

					runsTable.Clear()
					rowsByIndex = map[int]runRow{}
					headers := []string{"ID", "STATUS", "ATTEMPT", "CREATED"}
					for i, h := range headers {
						runsTable.SetCell(0, i, tview.NewTableCell(h).SetSelectable(false).SetAttributes(tcellAttrBold()))
					}

					if projectID == "" {
						runsTable.SetCell(1, 0, tview.NewTableCell("set --project or global --project to load runs"))
					} else {
						for i, run := range runs {
							row := i + 1
							runsTable.SetCell(row, 0, tview.NewTableCell(run.ID))
							runsTable.SetCell(row, 1, tview.NewTableCell(tviewStatusColor(run.Status)))
							runsTable.SetCell(row, 2, tview.NewTableCell(fmt.Sprintf("%d", run.Attempt)))
							runsTable.SetCell(row, 3, tview.NewTableCell(run.CreatedAt.UTC().Format(time.RFC3339)))
							rowsByIndex[row] = runRow{ID: run.ID, Status: run.Status}
						}
					}

					selectedMu.Lock()
					current := selectedRunID
					selectedMu.Unlock()
					if current == "" && len(rowsByIndex) > 0 {
						current = rowsByIndex[1].ID
						runsTable.Select(1, 0)
					}
					updateRunDetail(current)
				})

				return nil
			}

			if err := refresh(); err != nil {
				return err
			}

			done := make(chan struct{})
			go func() {
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for {
					select {
					case <-done:
						return
					case <-ticker.C:
						_ = refresh()
					}
				}
			}()

			focusOrder := []tview.Primitive{runsTable, detailView}
			focusIndex := 0

			app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
				switch event.Key() {
				case tcell.KeyTab:
					focusIndex = (focusIndex + 1) % len(focusOrder)
					app.SetFocus(focusOrder[focusIndex])
					return nil
				case tcell.KeyCtrlC:
					close(done)
					app.Stop()
					return nil
				}

				switch event.Rune() {
				case 'q':
					close(done)
					app.Stop()
					return nil
				case 'r':
					_ = refresh()
					return nil
				case 'j':
					row, col := runsTable.GetSelection()
					if row < runsTable.GetRowCount()-1 {
						runsTable.Select(row+1, col)
					}
					return nil
				case 'k':
					row, col := runsTable.GetSelection()
					if row > 1 {
						runsTable.Select(row-1, col)
					}
					return nil
				}

				return event
			})

			if err := app.SetRoot(layout, true).SetFocus(runsTable).Run(); err != nil {
				close(done)
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&projectID, "project", "", "project ID for run explorer panel")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "refresh interval")
	cmd.Flags().IntVar(&runLimit, "run-limit", 50, "max runs to display")
	cmd.Flags().IntVar(&eventLimit, "event-limit", 25, "max events shown for selected run")

	return cmd
}

func tcellAttrBold() tcell.AttrMask {
	return tcell.AttrBold
}

// tviewStatusColor wraps a run status in tview color tags.
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
