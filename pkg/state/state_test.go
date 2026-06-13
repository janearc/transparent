package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStore_NonExistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if store == nil {
		t.Fatal("expected store, got nil")
	}
	if store.Data.Services == nil {
		t.Fatal("expected services map to be initialized")
	}
}

func TestNewStore_OldSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	oldEvents := []Event{
		{Timestamp: time.Now(), Status: StatusOnline, Message: "test"},
	}
	data, _ := json.Marshal(oldEvents)
	err := os.WriteFile(path, data, 0644)
	if err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(store.Data.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(store.Data.Events))
	}
	if store.Data.Services == nil {
		t.Fatal("expected services map to be initialized")
	}
}

func TestNewStore_NewSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	newData := StoreData{
		Events: []Event{
			{Timestamp: time.Now(), Status: StatusOnline, Message: "test"},
		},
		Services: map[string][]ServiceSnapshot{
			"svc": {{Timestamp: time.Now(), Up: true, Commits: 1}},
		},
	}
	data, _ := json.Marshal(newData)
	err := os.WriteFile(path, data, 0644)
	if err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(store.Data.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(store.Data.Events))
	}
	if len(store.Data.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(store.Data.Services))
	}
}

func TestStore_LoadError_ReadFile(t *testing.T) {
	dir := t.TempDir()
	// path is a directory, ReadFile will fail
	store := &Store{path: dir}
	err := store.load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStore_LoadError_UnmarshalFail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	err := os.WriteFile(path, []byte("{invalid json"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	store := &Store{path: path}
	err = store.load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Also check that NewStore doesn't crash on invalid JSON and ensures Services map exists
	store2, err := NewStore(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if store2.Data.Services == nil {
		t.Fatal("expected services map to be initialized")
	}
}

func TestNewStore_NoServices(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	err := os.WriteFile(path, []byte(`{"events": [{"timestamp": "2026-06-13T10:00:00Z", "status": "online"}]}`), 0644)
	if err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if store.Data.Services == nil {
		t.Fatal("expected services to be initialized")
	}
}

func TestStore_SaveError(t *testing.T) {
	dir := t.TempDir()
	// path is a directory, WriteFile will fail
	store := &Store{path: dir}
	err := store.save()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestStore_SaveError_Marshal(t *testing.T) {
	// Not practically possible to test json.MarshalIndent failure with the current struct
	// unless we inject a channel or something, but we can't because it's strongly typed.
	// We'll rely on achieving 100% via other paths. The JSON marshal of StoreData won't fail.
}

func TestStore_AddEvent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store, _ := NewStore(path)

	err := store.AddEvent(StatusOnline, "all good")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	events := store.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Status != StatusOnline {
		t.Fatalf("expected status %v, got %v", StatusOnline, events[0].Status)
	}
}

func TestStore_AddEvent_Truncate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store, _ := NewStore(path)

	store.Data.Events = []Event{
		{Timestamp: time.Now().UTC().Add(-31 * 24 * time.Hour), Status: StatusOffline, Message: "old"},
		{Timestamp: time.Now().UTC().Add(-1 * 24 * time.Hour), Status: StatusOnline, Message: "new"},
	}

	err := store.AddEvent(StatusAsleep, "going to sleep")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	events := store.GetEvents()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Message != "new" {
		t.Fatalf("expected message 'new', got '%s'", events[0].Message)
	}
}

func TestStore_RecordServiceMetrics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store, _ := NewStore(path)

	err := store.RecordServiceMetrics("svc1", true, 10)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	history := store.GetServiceHistory("svc1")
	if len(history) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(history))
	}
	if history[0].Commits != 10 {
		t.Fatalf("expected 10 commits, got %d", history[0].Commits)
	}
}

func TestStore_RecordServiceMetrics_Truncate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	store, _ := NewStore(path)

	// Add 672 existing snapshots
	for i := 0; i < 672; i++ {
		store.Data.Services["svc1"] = append(store.Data.Services["svc1"], ServiceSnapshot{
			Timestamp: time.Now(),
			Up:        true,
			Commits:   i,
		})
	}

	err := store.RecordServiceMetrics("svc1", false, 999)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	history := store.GetServiceHistory("svc1")
	if len(history) != 672 {
		t.Fatalf("expected 672 snapshots, got %d", len(history))
	}
	if history[671].Commits != 999 {
		t.Fatalf("expected 999 commits as the last item, got %d", history[671].Commits)
	}
	if history[0].Commits != 1 { // 0th item should have been truncated
		t.Fatalf("expected 1 commit as the first item, got %d", history[0].Commits)
	}
}

func TestStore_OldSchemaFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Empty array, not considered valid old schema to migrate, falls through
	oldEvents := []Event{}
	data, _ := json.Marshal(oldEvents)
	err := os.WriteFile(path, data, 0644)
	if err != nil {
		t.Fatal(err)
	}

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(store.Data.Events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(store.Data.Events))
	}
}
