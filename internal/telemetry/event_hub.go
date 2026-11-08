package telemetry

import (
	"fmt"
	"sync"
	"time"
)

// QueryEvent represents a processed SQL query telemetry record.
type QueryEvent struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	SQL        string    `json:"sql"`
	Normalized string    `json:"normalized"`
	Status     string    `json:"status"` // "HIT", "MISS", "BYPASS", "INVALIDATE"
	DurationMs float64   `json:"duration_ms"`
	Tables     []string  `json:"tables"`
	Reason     string    `json:"reason,omitempty"`
}

const maxRecentEvents = 150

// EventHub manages real-time broadcast of query events to web clients and SSE streams.
type EventHub struct {
	mu      sync.RWMutex
	clients map[chan QueryEvent]struct{}
	recent  []QueryEvent
	seq     int64
}

// GlobalHub is the singleton event broadcaster.
var GlobalHub = NewEventHub()

// NewEventHub creates an initialized EventHub.
func NewEventHub() *EventHub {
	return &EventHub{
		clients: make(map[chan QueryEvent]struct{}),
		recent:  make([]QueryEvent, 0, maxRecentEvents),
	}
}

// Broadcast dispatches a query event to all active SSE subscribers and appends it to the recent buffer.
func (h *EventHub) Broadcast(e QueryEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.seq++
	if e.ID == "" {
		e.ID = fmt.Sprintf("evt-%d", h.seq)
	}
	if e.Tables == nil {
		e.Tables = []string{}
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	// Update recent ring buffer
	if len(h.recent) >= maxRecentEvents {
		h.recent = h.recent[1:]
	}
	h.recent = append(h.recent, e)

	// Send to all subscribers non-blockingly
	for ch := range h.clients {
		select {
		case ch <- e:
		default:
			// Client channel full, skip to avoid blocking proxy execution
		}
	}
}

// Subscribe registers an event listener channel.
func (h *EventHub) Subscribe() chan QueryEvent {
	h.mu.Lock()
	defer h.mu.Unlock()

	ch := make(chan QueryEvent, 50)
	h.clients[ch] = struct{}{}
	return ch
}

// Unsubscribe removes an event listener channel.
func (h *EventHub) Unsubscribe(ch chan QueryEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
}

// GetRecent returns a copy of the most recent query events.
func (h *EventHub) GetRecent() []QueryEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	res := make([]QueryEvent, len(h.recent))
	copy(res, h.recent)
	return res
}
