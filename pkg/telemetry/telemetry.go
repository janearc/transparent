package telemetry

import (
	"fmt"
	"net/http"
	"sync"
)

// Registry holds internal telemetry counters for transparent
type Registry struct {
	mu      sync.RWMutex
	counters map[string]int64
}

var globalRegistry = &Registry{
	counters: make(map[string]int64),
}

func Inc(key string) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.counters[key]++
}

func Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		globalRegistry.mu.RLock()
		defer globalRegistry.mu.RUnlock()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		for key, val := range globalRegistry.counters {
			fmt.Fprintf(w, "%s %d\n", key, val)
		}
	}
}

func Reset() {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.counters = make(map[string]int64)
}
