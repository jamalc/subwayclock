// Package clock runs the poll-and-animate loop shared by the simulator
// and the on-device firmware.
//
// It periodically fetches arrivals from the service and cycles the display
// through arrival groups, advancing the text phase (stop name → route name →
// headsign) within each group.
//
// It also owns the on-display rendering (arrival groups, status messages, the
// color palette, and drawing primitives), in render.go and colors.go. These
// are kept in this package until something else needs to reuse them; only Info
// and Error are exported, for the device bootstrap.
package clock

import (
	"image/color"
	"log"
	"time"

	"github.com/jamalc/subwayclock/internal/api"
	"github.com/jamalc/subwayclock/internal/client"
	"github.com/jamalc/subwayclock/internal/config"
	"github.com/jamalc/subwayclock/internal/input"
)

// textPhases is the number of text phases each group cycles through.
const textPhases = 3

// navigate applies a button press to the group cursor, moving it by whole
// pages of slots groups and wrapping around n groups. input.Up selects the
// previous page, input.Down the next. The cursor stays page-aligned (a multiple
// of slots), so the same groups always appear together and a short final page
// shows on its own. This is the single spot that decides what the buttons mean;
// future behaviors (e.g. a direction filter) grow here without touching the
// input.Source port or adapters.
func navigate(cursor int, b input.Button, n, slots int) int {
	if n == 0 || slots <= 0 {
		return 0
	}
	switch b {
	case input.Down:
		next := cursor + slots
		if next >= n {
			return 0 // past the last page: wrap to the first
		}
		return next
	case input.Up:
		prev := cursor - slots
		if prev < 0 {
			return ((n - 1) / slots) * slots // first page: wrap to the last page's start
		}
		return prev
	default:
		return cursor
	}
}

// Display is the subset of a panel the loop drives: a drivers.Displayer that
// can also be cleared. Both hub75.Device and simulator.Display satisfy it.
type Display interface {
	// Size returns the current size of the display.
	Size() (x, y int16)

	// SetPixel modifies the internal buffer.
	SetPixel(x, y int16, c color.RGBA)

	// Display sends the buffer (if any) to the screen.
	Display() error
	Clear()
}

// Run polls the arrivals service and animates the display. It loops forever
// and does not return under normal operation.
func Run(cfg config.Config, display Display, in input.Source) {
	if cfg.PollInterval == 0 || cfg.PhaseInterval == 0 || cfg.GroupInterval == 0 {
		log.Fatal("config must specify poll_interval, phase_interval, and group_interval")
	}

	c := client.NewClient(cfg.Host, cfg.Port)

	// slots is how many arrival groups fit on the display at once (one page).
	slots := cfg.Height / slotHeight

	var groups []api.FlatArrivalGroup
	// Start one page before the first so the immediate auto-cycle on the first
	// tick lands on page 0, showing the first slots groups.
	currentGroup := -slots
	textPhase := 0
	lastPoll := time.Now().Add(-cfg.PollInterval)
	lastGroup := time.Now().Add(-cfg.GroupInterval)
	lastPhase := time.Now().Add(-cfg.PhaseInterval)

	for {
		now := time.Now()

		if now.Sub(lastPoll) >= cfg.PollInterval {
			fetched, err := c.FetchArrivals(cfg.Stops)
			if err != nil {
				Warning(display, "Failed to fetch arrivals: "+err.Error())
				time.Sleep(2 * time.Second)
				continue
			}
			groups = fetched
			if len(groups) > 0 {
				// Keep the cursor in range and page-aligned after the group
				// count changes. Integer division preserves the negative
				// startup sentinel (-slots stays -slots).
				currentGroup = ((currentGroup % len(groups)) / slots) * slots
			} else {
				currentGroup = 0
			}
			lastPoll = now
		}

		if len(groups) > 0 {
			if btn := in.Poll(); btn != input.None {
				// Manual navigation: page the cursor and restart the
				// auto-cycle timers so the press isn't immediately undone.
				currentGroup = navigate(currentGroup, btn, len(groups), slots)
				textPhase = 0
				lastGroup = now
				lastPhase = now
			} else if now.Sub(lastGroup) >= cfg.GroupInterval {
				currentGroup = navigate(currentGroup, input.Down, len(groups), slots)
				textPhase = 0
				lastGroup = now
				lastPhase = now
			} else if now.Sub(lastPhase) >= cfg.PhaseInterval {
				textPhase = (textPhase + 1) % textPhases
				lastPhase = now
			}

			display.Clear()
			for i := 0; i < slots; i++ {
				idx := currentGroup + i
				if idx >= len(groups) {
					break // short final page: leave the remaining slots blank
				}
				group(display, groups[idx], textPhase, i)
			}
			display.Display()
		}

		time.Sleep(100 * time.Millisecond)
	}
}
