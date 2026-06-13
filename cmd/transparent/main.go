package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"transparent/pkg/checker"
	"transparent/pkg/metrics"
	"transparent/pkg/reporter"
	"transparent/pkg/state"
	"transparent/pkg/telemetry"
)

func main() {
	repoPath := flag.String("repo", ".", "Path to the git repository")
	immediate := flag.Bool("immediate", false, "execute an immediate evaluation on startup")
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

	forceEval := make(chan struct{})

	// Initialize HTTP server for health and metrics
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("GET /metrics", telemetry.Handler())
	mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/markdown")
		http.ServeFile(w, r, *repoPath+"/REPORT/uptime.md")
	})
	mux.HandleFunc("POST /evaluate", func(w http.ResponseWriter, r *http.Request) {
		select {
		case forceEval <- struct{}{}:
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprintf(w, `{"status":"evaluation triggered"}`)
		default:
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprintf(w, `{"status":"evaluation already in progress"}`)
		}
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

	slog.Info("transparent daemon started", "pollInterval", pollInterval, "commitInterval", commitInterval, "immediate", *immediate)

	doPoll := func() {
		telemetry.Inc("transparent_polls_total")
		now := time.Now()
		elapsed := now.Sub(lastTick)

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

		// Record service metrics historically
		metricsData := metrics.Collect(ctx, "/work")
		for _, repo := range metricsData.Repos {
			for _, svc := range repo.Services {
				up := !strings.HasPrefix(svc.Status, "Exited") && !strings.HasPrefix(svc.Status, "Created")
				store.RecordServiceMetrics(svc.Name, up, repo.Commits)
			}
		}
	}

	doCommit := func() {
		telemetry.Inc("transparent_commits_total")
		events := store.GetEvents()
		metricsData := metrics.Collect(ctx, "/work")
		err := reporter.GenerateDashboard(*repoPath+"/REPORT", events, metricsData, store)
		if err != nil {
			slog.Error("failed to generate dashboard", "error", err)
			return
		}
		slog.Info("generating markdown, html, xml, json and committing locally")
		err = reporter.CommitDashboard(ctx, *repoPath)
		if err != nil {
			slog.Error("failed to commit dashboard", "error", err)
		} else {
			slog.Info("dashboard committed locally")
		}
	}

	if *immediate {
		slog.Info("executing immediate startup evaluation")
		doPoll()
		doCommit()
	}

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
		case <-forceEval:
			slog.Info("executing forced evaluation via control port")
			doPoll()
			doCommit()
		}
	}
}
