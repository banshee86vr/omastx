package scan

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// Phase names the stage of a scan for SSE consumers (SPEC §2.4).
const (
	PhaseStarted     = "started"
	PhaseDiscovering = "discovering"
	PhaseResolving   = "resolving"
	PhaseDone        = "done"
	PhaseError       = "error"
)

// Event is one SSE progress message broadcast during a scan.
type Event struct {
	Phase   string `json:"phase"`
	Message string `json:"message"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Stats   *Stats `json:"stats,omitempty"`
}

// Terminal reports whether this event ends the scan stream.
func (e Event) Terminal() bool { return e.Phase == PhaseDone || e.Phase == PhaseError }

// Hub fans scan events out to any connected SSE subscribers, buffering the full
// event history per scan so a client that connects slightly after POST /scan
// still sees every step. Finished streams are reaped after a grace period.
type Hub struct {
	mu      sync.Mutex
	streams map[uuid.UUID]*stream
}

type stream struct {
	mu     sync.Mutex
	events []Event
	subs   map[chan Event]struct{}
	done   bool
}

func NewHub() *Hub {
	return &Hub{streams: map[uuid.UUID]*stream{}}
}

func (h *Hub) get(id uuid.UUID) *stream {
	h.mu.Lock()
	defer h.mu.Unlock()
	st := h.streams[id]
	if st == nil {
		st = &stream{subs: map[chan Event]struct{}{}}
		h.streams[id] = st
	}
	return st
}

// Subscribe returns a channel replaying the scan's events so far, then streaming
// new ones until the scan finishes. The returned func unsubscribes.
func (h *Hub) Subscribe(id uuid.UUID) (<-chan Event, func()) {
	st := h.get(id)
	st.mu.Lock()
	defer st.mu.Unlock()

	ch := make(chan Event, len(st.events)+64)
	for _, e := range st.events {
		ch <- e
	}
	if st.done {
		close(ch)
		return ch, func() {}
	}
	st.subs[ch] = struct{}{}
	return ch, func() {
		st.mu.Lock()
		defer st.mu.Unlock()
		if _, ok := st.subs[ch]; ok {
			delete(st.subs, ch)
			close(ch)
		}
	}
}

// Publish records an event and delivers it to current subscribers. On a terminal
// event, subscribers are closed and the stream is scheduled for cleanup.
func (h *Hub) Publish(id uuid.UUID, e Event) {
	st := h.get(id)
	st.mu.Lock()
	st.events = append(st.events, e)
	subs := make([]chan Event, 0, len(st.subs))
	for ch := range st.subs {
		subs = append(subs, ch)
	}
	terminal := e.Terminal()
	if terminal {
		st.done = true
		st.subs = map[chan Event]struct{}{}
	}
	st.mu.Unlock()

	for _, ch := range subs {
		if terminal {
			// Never drop the terminal event: SSE clients need it to stop waiting.
			select {
			case ch <- e:
			case <-time.After(2 * time.Second):
				// Subscriber wedged; close below so the HTTP handler can exit.
			}
			close(ch)
			continue
		}
		select {
		case ch <- e:
		default: // slow consumer: history + reconnect still deliver later events
		}
	}

	if terminal {
		time.AfterFunc(5*time.Minute, func() {
			h.mu.Lock()
			delete(h.streams, id)
			h.mu.Unlock()
		})
	}
}
