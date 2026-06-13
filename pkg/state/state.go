package state

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type EventStatus string

const (
	StatusOnline  EventStatus = "online"
	StatusOffline EventStatus = "offline"
	StatusAsleep  EventStatus = "asleep"
)

type Event struct {
	Timestamp time.Time   `json:"timestamp"`
	Status    EventStatus `json:"status"`
	Message   string      `json:"message,omitempty"`
}

type ServiceSnapshot struct {
	Timestamp time.Time `json:"timestamp"`
	Up        bool      `json:"up"`
	Commits   int       `json:"commits"`
}

type StoreData struct {
	Events   []Event                      `json:"events"`
	Services map[string][]ServiceSnapshot `json:"services"`
}

type Store struct {
	mu   sync.Mutex
	path string
	Data StoreData
}

func NewStore(path string) (*Store, error) {
	s := &Store{
		path: path,
		Data: StoreData{
			Services: make(map[string][]ServiceSnapshot),
		},
	}
	s.load() // ignore error, file might not exist
	if s.Data.Services == nil {
		s.Data.Services = make(map[string][]ServiceSnapshot)
	}
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	// Try parsing into new schema. For backwards compatibility we might lose old events.
	// But since it's just a local file, it's fine.
	// Actually, let's try mapping the old schema format to new if needed.
	// We'll just let it fail or parse what it can.
	var oldEvents []Event
	if err := json.Unmarshal(data, &oldEvents); err == nil && len(oldEvents) > 0 && oldEvents[0].Status != "" {
		// It was the old schema, migrate it
		s.Data.Events = oldEvents
		s.Data.Services = make(map[string][]ServiceSnapshot)
		return nil
	}

	return json.Unmarshal(data, &s.Data)
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.Data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *Store) AddEvent(status EventStatus, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Data.Events = append(s.Data.Events, Event{
		Timestamp: time.Now().UTC(),
		Status:    status,
		Message:   message,
	})

	// Truncate host events older than 30 days
	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	filtered := []Event{}
	for _, e := range s.Data.Events {
		if e.Timestamp.After(cutoff) {
			filtered = append(filtered, e)
		}
	}
	s.Data.Events = filtered

	return s.save()
}

func (s *Store) GetEvents() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := make([]Event, len(s.Data.Events))
	copy(events, s.Data.Events)
	return events
}

func (s *Store) RecordServiceMetrics(serviceName string, up bool, totalCommits int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	history := s.Data.Services[serviceName]
	history = append(history, ServiceSnapshot{
		Timestamp: time.Now().UTC(),
		Up:        up,
		Commits:   totalCommits,
	})

	// Keep up to 672 snapshots (7 days at 15m intervals)
	if len(history) > 672 {
		history = history[len(history)-672:]
	}
	s.Data.Services[serviceName] = history

	return s.save()
}

func (s *Store) GetServiceHistory(serviceName string) []ServiceSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	hist := s.Data.Services[serviceName]
	out := make([]ServiceSnapshot, len(hist))
	copy(out, hist)
	return out
}
