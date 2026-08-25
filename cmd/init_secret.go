package cmd

import "sth/internal/tui"

func (a *app) secureSecretSession(fallback *tui.Session) (*tui.Session, func()) {
	if fallback.Interactive {
		return fallback, func() {}
	}
	terminalSession := tui.NewSession(a.stdin, a.stderr)
	if !terminalSession.Interactive {
		_ = terminalSession.Close()
		return fallback, func() {}
	}
	return terminalSession, func() {
		_ = terminalSession.Close()
	}
}
