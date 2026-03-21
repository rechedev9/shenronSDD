package store

import (
	"context"
	"path/filepath"
	"time"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/errlog"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/events"
)

// RegisterSubscribers wires SQLite event subscribers to the broker.
// Safe to call with nil broker or nil store (no-op).
func RegisterSubscribers(broker *events.Broker, s *Store) {
	if broker == nil || s == nil {
		return
	}

	// PhaseAssembled subscriber — inserts into phase_events
	broker.Subscribe(events.PhaseAssembled, func(e events.Event) {
		p, ok := e.Payload.(events.PhaseAssembledPayload)
		if !ok {
			return
		}
		_ = s.InsertPhaseEvent(context.Background(), PhaseEvent{ // best-effort telemetry; don't block pipeline
			Timestamp:  time.Now().UTC(),
			Change:     filepath.Base(p.ChangeDir),
			Phase:      p.Phase,
			Bytes:      p.Bytes,
			Tokens:     p.Tokens,
			Cached:     p.Cached,
			DurationMs: p.DurationMs,
		})
	})

	// VerifyFailed subscriber — loops over Results, inserts each into verify_events
	broker.Subscribe(events.VerifyFailed, func(e events.Event) {
		p, ok := e.Payload.(events.VerifyFailedPayload)
		if !ok {
			return
		}
		for _, cmd := range p.Results {
			_ = s.InsertVerifyEvent(context.Background(), VerifyEvent{ // best-effort telemetry
				Timestamp:   time.Now().UTC(),
				Change:      p.Change,
				CommandName: cmd.Name,
				Command:     cmd.Command,
				ExitCode:    cmd.ExitCode,
				ErrorLines:  cmd.ErrorLines,
				Fingerprint: errlog.Fingerprint(cmd.Command, cmd.ErrorLines),
			})
		}
	})
}
