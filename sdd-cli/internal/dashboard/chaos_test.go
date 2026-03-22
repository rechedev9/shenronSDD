package dashboard

import (
	"context"
	"math/rand"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/store"
)

// errorMetrics implements MetricsReader and returns errors from all methods.
type errorMetrics struct{}

func (e *errorMetrics) TokenSummary(_ context.Context) (*store.TokenStats, error) {
	return nil, context.Canceled
}

func (e *errorMetrics) PhaseTokensByChange(_ context.Context) ([]store.ChangeTokens, error) {
	return nil, context.Canceled
}

func (e *errorMetrics) RecentErrors(_ context.Context, _ int) ([]store.ErrorRow, error) {
	return nil, context.Canceled
}

func (e *errorMetrics) TokenHistory(_ context.Context, _ time.Time) ([]store.TokenHistoryRow, error) {
	return nil, context.Canceled
}

func (e *errorMetrics) PhaseDurations(_ context.Context) ([]store.PhaseDurationRow, error) {
	return nil, context.Canceled
}

func (e *errorMetrics) CacheHistory(_ context.Context, _ time.Time) ([]store.CacheHistoryRow, error) {
	return nil, context.Canceled
}

func (e *errorMetrics) VerifyHistory(_ context.Context, _ time.Time) ([]store.VerifyHistoryRow, error) {
	return nil, context.Canceled
}

// startTestHub creates a hub with an httptest.Server exposing /ws.
// Returns the ws URL and a cancel func that stops hub + server.
func startTestHub(t *testing.T, m MetricsReader) (string, context.CancelFunc) {
	t.Helper()
	srv, changesDir := newTestServer(t, m)
	_ = changesDir

	ts := httptest.NewServer(srv.routes())
	t.Cleanup(ts.Close)

	ctx, cancel := context.WithCancel(context.Background())
	go srv.Hub().Run(ctx)

	wsURL := "ws" + ts.URL[len("http"):] + "/ws"
	return wsURL, cancel
}

// TestChaosHubConcurrentClients dials 20 WebSocket connections simultaneously,
// reads one message each, then closes after a random delay.
// The hub must not panic or corrupt its client map under -race.
func TestChaosHubConcurrentClients(t *testing.T) {
	t.Parallel()
	wsURL, cancel := startTestHub(t, &fakeMetrics{
		stats: &store.TokenStats{TotalTokens: 42},
	})
	defer cancel()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			ctx, c := context.WithTimeout(context.Background(), 3*time.Second)
			defer c()

			conn, _, err := websocket.Dial(ctx, wsURL, nil)
			if err != nil {
				return // server may reject under load — not a test failure
			}

			// Read at least one message (the snapshot).
			_, _, _ = conn.Read(ctx)

			// Random delay before close to create contention.
			time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
			conn.Close(websocket.StatusNormalClosure, "")
		}()
	}

	wg.Wait()
}

// TestChaosHubBroadcastUnderContention has persistent readers and rapid
// connect/disconnect goroutines running simultaneously while the hub polls.
func TestChaosHubBroadcastUnderContention(t *testing.T) {
	t.Parallel()
	wsURL, cancel := startTestHub(t, &fakeMetrics{
		stats: &store.TokenStats{TotalTokens: 100, CacheHitPct: 50.0, ErrorCount: 1},
	})
	defer cancel()

	ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()

	// 5 persistent readers.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, _, err := websocket.Dial(ctx, wsURL, nil)
			if err != nil {
				return
			}
			defer conn.Close(websocket.StatusNormalClosure, "")
			for {
				_, _, err := conn.Read(ctx)
				if err != nil {
					return
				}
			}
		}()
	}

	// 10 rapid connect/disconnect goroutines.
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				c, _, err := websocket.Dial(ctx, wsURL, nil)
				if err != nil {
					return
				}
				time.Sleep(time.Duration(rand.Intn(5)) * time.Millisecond)
				c.Close(websocket.StatusNormalClosure, "")
			}
		}()
	}

	wg.Wait()
}

// TestChaosHubMetricsErrors verifies the hub doesn't panic when MetricsReader
// returns errors from all methods during concurrent client access.
func TestChaosHubMetricsErrors(t *testing.T) {
	t.Parallel()
	wsURL, cancel := startTestHub(t, &errorMetrics{})
	defer cancel()

	const n = 5
	var wg sync.WaitGroup
	wg.Add(n)

	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			ctx, c := context.WithTimeout(context.Background(), 2*time.Second)
			defer c()

			conn, _, err := websocket.Dial(ctx, wsURL, nil)
			if err != nil {
				return
			}
			defer conn.Close(websocket.StatusNormalClosure, "")

			// Read messages until timeout — hub may or may not send data.
			for {
				_, _, err := conn.Read(ctx)
				if err != nil {
					return
				}
			}
		}()
	}

	wg.Wait()
}
