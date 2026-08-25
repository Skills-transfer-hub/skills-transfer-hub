package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"sth/internal/timing"
)

type Session struct {
	Stdout      io.Writer
	Interactive bool
	MaxWidth    int

	stdinFile  *os.File
	ownedTTY   *os.File
	reader     *bufio.Reader
	stdinTTY   bool
	readSecret func(int) ([]byte, error)
}

func NewSession(stdin io.Reader, stdout io.Writer) *Session {
	session := &Session{Stdout: stdout, readSecret: term.ReadPassword}
	inputFile, inputIsFile := stdin.(*os.File)
	if inputIsFile {
		session.stdinFile = inputFile
		session.stdinTTY = isTerminalFile(inputFile)
	}
	outputFile, outputIsFile := stdout.(*os.File)
	session.Interactive = session.stdinTTY && outputIsFile && isTerminalFile(outputFile)
	session.reader = bufio.NewReader(stdin)
	if session.Interactive {
		session.MaxWidth = terminalWidth(stdout)
		EnableColors()
	}
	timing.Mark(fmt.Sprintf("session interactive=%v", session.Interactive))
	return session
}

func (session *Session) Close() error {
	if session.ownedTTY != nil {
		return session.ownedTTY.Close()
	}
	return nil
}

func isTerminalFile(file *os.File) bool {
	return term.IsTerminal(int(file.Fd()))
}

func terminalWidth(writer io.Writer) int {
	file, isFile := writer.(*os.File)
	if !isFile {
		return 0
	}
	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil {
		return 0
	}
	return width
}

func (session *Session) readLine() (string, error) {
	line, err := session.reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read user input: %w", err)
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" && err == io.EOF {
		return "", ErrAborted
	}
	return trimmed, nil
}

func (session *Session) RunProgram(model tea.Model) (tea.Model, error) {
	options := []tea.ProgramOption{tea.WithOutput(session.Stdout)}
	inputFile := session.ttyFile()
	if inputFile != nil {
		options = append(options, tea.WithInput(inputFile))
	} else {
		options = append(options, tea.WithInput(nil))
	}
	programModel := model
	if _, autoQuits := model.(finisher); autoQuits {
		programModel = wrapAutoQuit(model)
	}
	program := tea.NewProgram(programModel, options...)
	runStart := time.Now()
	finalModel, err := program.Run()
	timing.Mark(fmt.Sprintf("tui-run %dms", time.Since(runStart).Milliseconds()))
	if err != nil {
		return nil, err
	}
	return unwrapAutoQuit(finalModel), nil
}

func (session *Session) ttyFile() *os.File {
	if session.stdinFile == nil {
		return nil
	}
	if session.ownedTTY != nil {
		return session.ownedTTY
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return session.stdinFile
	}
	session.ownedTTY = tty
	return tty
}
