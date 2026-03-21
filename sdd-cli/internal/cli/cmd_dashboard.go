package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli/errs"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/dashboard"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/store"
)

func runDashboard(args []string, stdout io.Writer, stderr io.Writer) error {
	port := "8811"
	for i, arg := range args {
		switch {
		case (arg == "--port" || arg == "-p") && i+1 < len(args):
			port = args[i+1]
		}
	}

	p, err := strconv.Atoi(port)
	if err != nil || p < 1024 || p > 65535 {
		return errs.Usage(fmt.Sprintf("invalid port: %s (must be 1024-65535)", port))
	}

	cwd, err := os.Getwd()
	if err != nil {
		return errs.WriteError(stderr, "dashboard", fmt.Errorf("get working directory: %w", err))
	}
	dbPath := filepath.Join(cwd, "openspec", ".cache", "sdd.db")
	changesDir := filepath.Join(cwd, "openspec", "changes")

	db, err := store.Open(dbPath)
	if err != nil {
		return errs.WriteError(stderr, "dashboard", fmt.Errorf("open store: %w", err))
	}
	defer db.Close()

	srv := dashboard.New(db, changesDir)
	addr := "0.0.0.0:" + port

	out := struct {
		Command string `json:"command"`
		Status  string `json:"status"`
		URL     string `json:"url"`
	}{
		Command: "dashboard",
		Status:  "running",
		URL:     "http://" + addr,
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Fprintln(stdout, string(data))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go srv.Hub().Run(ctx)

	slog.Info("dashboard started", "url", "http://"+addr)
	return srv.ListenAndServe(ctx, addr)
}
