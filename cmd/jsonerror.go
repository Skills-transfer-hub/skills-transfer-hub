package cmd

import (
	"encoding/json"
	"fmt"
	"io"
)

// emitJSONError writes a stable {"error": "...", "code": N} object to w.
// Called by list/version/status when --json is active and the underlying
// command returned an error (FR-022). The caller still propagates the
// error so the process exits non-zero — emitJSONError only handles the
// stdout payload.
func emitJSONError(w io.Writer, err error, code int) {
	payload := struct {
		Error string `json:"error"`
		Code  int    `json:"code"`
	}{
		Error: err.Error(),
		Code:  code,
	}
	b, mErr := json.Marshal(payload)
	if mErr != nil {
		// Fallback to a minimal hand-rolled JSON if marshalling fails — should
		// never happen for these primitives, but safer than emitting nothing.
		fmt.Fprintf(w, `{"error":"json marshal failed","code":%d}`, code)
		return
	}
	fmt.Fprintln(w, string(b))
}
