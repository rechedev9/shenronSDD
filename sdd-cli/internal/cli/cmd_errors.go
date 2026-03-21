package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/rechedev9/shenronSDD/sdd-cli/internal/cli/errs"
	"github.com/rechedev9/shenronSDD/sdd-cli/internal/errlog"
)

func runErrors(args []string, stdout io.Writer, stderr io.Writer) error {
	jsonOut := false
	for _, arg := range args {
		switch {
		case arg == "--json":
			jsonOut = true
		default:
			return errs.Usage(fmt.Sprintf("unknown flag: %s", arg))
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return errs.WriteError(stderr, "errors", fmt.Errorf("get working directory: %w", err))
	}

	log := errlog.Load(cwd)

	if jsonOut {
		type errorGroup struct {
			Fingerprint string   `json:"fingerprint"`
			Count       int      `json:"count"`
			Command     string   `json:"command"`
			LastSeen    string   `json:"last_seen"`
			ErrorLines  []string `json:"error_lines"`
		}
		groups := make(map[string]*errorGroup)
		for _, e := range log.Entries {
			g, ok := groups[e.Fingerprint]
			if !ok {
				g = &errorGroup{
					Fingerprint: e.Fingerprint,
					Command:     e.Command,
					ErrorLines:  e.ErrorLines,
				}
				groups[e.Fingerprint] = g
			}
			g.Count++
			if e.Timestamp > g.LastSeen {
				g.LastSeen = e.Timestamp
				g.ErrorLines = e.ErrorLines
			}
		}

		sorted := make([]*errorGroup, 0, len(groups))
		for _, g := range groups {
			sorted = append(sorted, g)
		}
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Count > sorted[j].Count
		})

		out := struct {
			Command string        `json:"command"`
			Status  string        `json:"status"`
			Total   int           `json:"total"`
			Groups  []*errorGroup `json:"groups"`
		}{
			Command: "errors",
			Status:  "success",
			Total:   len(log.Entries),
			Groups:  sorted,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(stdout, string(data))
		return nil
	}

	if len(log.Entries) == 0 {
		fmt.Fprintln(stdout, "sdd errors: no recorded errors")
		return nil
	}

	counts := log.RecurringFingerprints(1)
	fmt.Fprintf(stdout, "sdd errors: %d entries, %d unique patterns\n\n", len(log.Entries), len(counts))
	start := 0
	if len(log.Entries) > 10 {
		start = len(log.Entries) - 10
	}
	for _, e := range log.Entries[start:] {
		fp := e.Fingerprint
		if len(fp) > 8 {
			fp = fp[:8]
		}
		fmt.Fprintf(stdout, "  %s  %-8s  exit=%d  %s  [%s]\n",
			e.Timestamp[:19], e.CommandName, e.ExitCode, e.Change, fp)
	}
	return nil
}
