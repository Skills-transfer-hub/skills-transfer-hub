package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// promptSecret reads a secret value from stdin. On a terminal it uses
// term.ReadPassword so the value is not echoed (no shell history, no shoulder
// surfing, no copy-paste leak). When stdin is not a TTY (CI, tests, piped
// input) it falls back to the caller-supplied bufio.Reader so we don't fight
// over the underlying buffer with the wizard's main reader.
//
// The label is printed as `<label> : ` before the read, with no trailing
// newline. Callers should announce the masking to the user before calling
// (e.g. "the input will be hidden").
func promptSecret(a *app, reader *bufio.Reader, label string) (string, error) {
	if _, err := fmt.Fprintf(a.stdout, "%s : ", label); err != nil {
		return "", err
	}

	if f, ok := a.stdin.(*os.File); ok {
		fd := int(f.Fd())
		if term.IsTerminal(fd) {
			b, err := term.ReadPassword(fd)
			fmt.Fprintln(a.stdout)
			if err != nil {
				return "", fmt.Errorf("lecture de la saisie échouée : %w", err)
			}
			return strings.TrimSpace(string(b)), nil
		}
	}

	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("lecture de la saisie échouée : %w", err)
	}
	return strings.TrimSpace(line), nil
}
