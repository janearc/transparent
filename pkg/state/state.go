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

type Store struct {
	mu     sync.Mutex
	path   string
	Events []Event `json:"events"`
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path}
	s.load() // ignore error, file might not exist
	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.Events)
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.Events, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}

func (s *Store) AddEvent(status EventStatus, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Events = append(s.Events, Event{
		Timestamp: time.Now().UTC(),
		Status:    status,
		Message:   message,
	})
	return s.save()
}

func (s *Store) GetEvents() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := make([]Event, len(s.Events))
	copy(events, s.Events)
	return events
}
