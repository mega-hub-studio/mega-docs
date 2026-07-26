package server

import (
	"encoding/json"
	"errors"
	"net/http"
)

// stream writes Server-Sent Events. It exists so handlers say what they mean —
// s.send("token", …) — instead of repeating the header/framing/flush dance.
type stream struct {
	w        http.ResponseWriter
	f        http.Flusher
	writeErr error
}

// newStream writes the SSE headers and returns a writer. It fails only when the
// ResponseWriter can't flush, which means the client can't be streamed to at all.
func newStream(w http.ResponseWriter) (*stream, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming unsupported")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // don't let a reverse proxy buffer the stream
	return &stream{w: w, f: f}, nil
}

// send emits one event, JSON-encoding v. The first write error is remembered and
// every later call becomes a no-op, so a handler can emit a whole answer without
// checking after each frame: once the client is gone, the rest is dropped quietly.
func (s *stream) send(event string, v any) {
	if s.writeErr != nil {
		return
	}
	body, err := json.Marshal(v)
	if err != nil {
		s.writeErr = err
		return
	}
	for _, part := range [][]byte{[]byte("event: " + event + "\ndata: "), body, []byte("\n\n")} {
		if _, err := s.w.Write(part); err != nil {
			s.writeErr = err
			return
		}
	}
	s.f.Flush()
}
