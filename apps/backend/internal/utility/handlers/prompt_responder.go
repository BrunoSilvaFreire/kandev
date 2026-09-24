package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/utility/dto"
)

// promptResponder writes a utility-prompt result either as one JSON body (the
// default) or, when the caller asked for application/x-ndjson, as a final
// {"result":<ExecutePromptResponse>} NDJSON frame preceded by progress frames.
type promptResponder struct {
	c       *gin.Context
	stream  bool
	mu      sync.Mutex
	flusher http.Flusher
	started bool
}

func newPromptResponder(c *gin.Context, stream bool) *promptResponder {
	w := &promptResponder{c: c, stream: stream}
	if stream {
		w.flusher, _ = c.Writer.(http.Flusher)
	}
	return w
}

// acceptsNDJSON reports whether the caller asked for the streaming form.
func acceptsNDJSON(c *gin.Context) bool {
	return strings.Contains(c.GetHeader("Accept"), "application/x-ndjson")
}

// beginStream starts the NDJSON stream: it sets the NDJSON content type just
// before the first write (so an early JSON error is not mislabeled) and emits
// the starting frame. It is a no-op for a JSON response.
func (w *promptResponder) beginStream() {
	if !w.stream {
		return
	}
	w.started = true
	w.c.Header("Content-Type", "application/x-ndjson")
	w.c.Header("Cache-Control", "no-cache")
	w.c.Header("X-Accel-Buffering", "no")
	w.write(map[string]any{"progress": map[string]any{"phase": "starting"}})
}

// progressFrame writes one progress frame. It is a no-op for a JSON response.
func (w *promptResponder) progressFrame(progress any) {
	if !w.stream {
		return
	}
	w.write(map[string]any{"progress": progress})
}

// finish writes the terminal result. Before the stream starts (or in JSON
// mode) it uses the caller's HTTP status; once streaming, the status is already
// 200 and the outcome rides the result frame.
func (w *promptResponder) finish(status int, resp dto.ExecutePromptResponse) {
	if !w.stream || !w.started {
		w.c.JSON(status, resp)
		return
	}
	w.write(map[string]any{"result": resp})
}

func (w *promptResponder) write(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.c.Writer.Write(append(data, '\n'))
	if w.flusher != nil {
		w.flusher.Flush()
	}
}
