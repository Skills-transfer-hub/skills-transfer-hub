// Package log provides a thin verbose-output sink shared by the cmd and http
// layers. It deliberately exposes a tiny interface (3 methods) so neither
// layer depends on a concrete logger and so the no-op default has zero cost.
package log

import (
	"fmt"
	"io"
	"strings"
)

// Verbose emits diagnostic lines on stderr when --verbose / --debug is active.
// All output is prefixed with "[verbose] " so callers can distinguish it from
// normal command stderr.
type Verbose interface {
	HTTP(method, url string, status int)
	Path(label, abs string)
	Decision(format string, args ...any)
}

// NewVerbose builds a sink. When enabled is false a no-op implementation is
// returned and the io.Writer is ignored.
func NewVerbose(w io.Writer, enabled bool) Verbose {
	if !enabled || w == nil {
		return noop{}
	}
	return &writer{w: w}
}

type noop struct{}

func (noop) HTTP(method, url string, status int)    {}
func (noop) Path(label, abs string)                 {}
func (noop) Decision(format string, args ...any)    {}

type writer struct {
	w io.Writer
}

func (v *writer) HTTP(method, url string, status int) {
	method = strings.TrimSpace(method)
	if method == "" {
		method = "GET"
	}
	if status > 0 {
		fmt.Fprintf(v.w, "[verbose] %s %s → %d\n", method, url, status)
		return
	}
	fmt.Fprintf(v.w, "[verbose] %s %s\n", method, url)
}

func (v *writer) Path(label, abs string) {
	fmt.Fprintf(v.w, "[verbose] path %s → %s\n", label, abs)
}

func (v *writer) Decision(format string, args ...any) {
	fmt.Fprintf(v.w, "[verbose] "+format+"\n", args...)
}
