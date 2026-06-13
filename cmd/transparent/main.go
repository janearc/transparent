package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"transparent/pkg/checker"
	"transparent/pkg/reporter"
	"transparent/pkg/state"
)

func main() {
	repoPath := flag.String("repo", ".", "Path to the git repository")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := state.NewStore(*repoPath + "/data.json")
	if err != nil {
		slog.Error("failed to init store", "error", err)
		os.Exit(1)
	}

	pollInterval := 15 * time.Minute
	commitInterval := 1 * time.Hour

	// Initialize HTTP server for health and metrics
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok"}`)
	})
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("control port failure", "error", err)
			cancel()
		}
	}()

	var lastTick time.Time

	events := store.GetEvents()
	if len(events) > 0 {
		lastTick = events[len(events)-1].Timestamp
	} else {
		lastTick = time.Now()
	}

	pollTicker := time.NewTicker(pollInterval)
	defer pollTicker.Stop()

	commitTicker := time.NewTicker(commitInterval)
	defer commitTicker.Stop()

	slog.Info("transparent daemon started", "pollInterval", pollInterval, "commitInterval", commitInterval)

	doPoll := func() {
		now := time.Now()
		elapsed := now.Sub(lastTick)

		// Determine if laptop was asleep (time jumped significantly)
		if !lastTick.IsZero() && elapsed > pollInterval+5*time.Minute {
			slog.Info("detected sleep period", "elapsed", elapsed)
			store.AddEvent(state.StatusAsleep, fmt.Sprintf("Asleep for %s", elapsed.Round(time.Minute)))
		}
		lastTick = now

		networkOk := checker.CheckNetwork(ctx)
		status := state.StatusOffline
		if networkOk {
			status = state.StatusOnline
		}

		slog.Info("network check completed", "status", status)
		store.AddEvent(status, "")
	}

	doCommit := func() {
		events := store.GetEvents()
		md := reporter.GenerateMarkdown(events)
		slog.Info("generating markdown and committing")
		err := reporter.CommitAndPush(ctx, *repoPath, *repoPath+"/uptime.md", md)
		if err != nil {
			slog.Error("failed to commit and push", "error", err)
		} else {
			slog.Info("commit and push attempt finished")
		}
	}

	// Do an immediate poll and commit attempt on startup
	doPoll()
	doCommit()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case sig := <-sigChan:
			slog.Info("received termination signal", "signal", sig)
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			server.Shutdown(shutdownCtx)
			return
		case <-ctx.Done():
			return
		case <-pollTicker.C:
			doPoll()
		case <-commitTicker.C:
			doCommit()
		}
	}
}
