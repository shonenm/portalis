package portalis

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/creack/pty"
)

func debugLog(format string, args ...interface{}) {
	_ = format
	_ = args
}

// Pty wraps a pseudoterminal and forwards output to a channel.
type Pty struct {
	cmd    *exec.Cmd
	ptmx   *os.File
	reader *bufio.Reader
	Output chan []byte
	Errors chan error
	done   chan struct{}

	lastRows int
	lastCols int
	setSize  func(*os.File, *pty.Winsize) error
}

// Spawn starts a new PTY with the given command, arguments and optional environment variables.
func Spawn(command string, args []string, env ...string) (*Pty, error) {
	return SpawnInDir(command, args, "", env...)
}

// SpawnInDir starts a new PTY in the given working directory.
func SpawnInDir(command string, args []string, dir string, env ...string) (*Pty, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	cmd.Env = append(cmd.Env, env...)
	if dir != "" {
		cmd.Dir = dir
	}
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}

	p := &Pty{
		cmd:    cmd,
		ptmx:   ptmx,
		reader: bufio.NewReader(ptmx),
		Output: make(chan []byte, 64),
		Errors: make(chan error, 1),
		done:   make(chan struct{}),
	}

	go p.readLoop()
	return p, nil
}

// Write sends data to the PTY.
func (p *Pty) Write(data []byte) error {
	if p.ptmx == nil {
		return fmt.Errorf("pty closed")
	}
	_, err := p.ptmx.Write(data)
	return err
}

// Resize resizes the PTY and signals the child process about the change.
func (p *Pty) Resize(rows, cols int) error {
	if p.ptmx == nil {
		return fmt.Errorf("pty closed")
	}
	if rows == p.lastRows && cols == p.lastCols {
		return nil
	}

	setSize := p.setSize
	if setSize == nil {
		setSize = pty.Setsize
	}
	if err := setSize(p.ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)}); err != nil {
		return err
	}
	p.lastRows = rows
	p.lastCols = cols
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Signal(syscall.SIGWINCH)
	}
	return nil
}

// Close closes the PTY and kills the process.
func (p *Pty) Close() error {
	close(p.done)
	if p.ptmx != nil {
		p.ptmx.Close()
	}
	if p.cmd != nil && p.cmd.Process != nil {
		p.cmd.Process.Kill()
		p.cmd.Process.Wait()
	}
	return nil
}

func (p *Pty) readLoop() {
	buf := make([]byte, 4096)
	defer close(p.Errors)
	for {
		select {
		case <-p.done:
			return
		default:
		}

		n, err := p.reader.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])

			if bytes.Contains(data, []byte("\x1b[6n")) {
				p.ptmx.Write([]byte("\x1b[1;1R"))
			}
			if bytes.Contains(data, []byte("\x1b[5n")) {
				p.ptmx.Write([]byte("\x1b[0n"))
			}

			select {
			case p.Output <- data:
			case <-p.done:
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				select {
				case p.Errors <- err:
				case <-p.done:
				}
			}
			return
		}
	}
}

// PtyOutputMsg is sent when new data arrives from the PTY.
type PtyOutputMsg struct {
	SessionID string
	Data      []byte
}

// PtyExitMsg is sent when the PTY process exits.
type PtyExitMsg struct {
	SessionID string
	Err       error
}

// SendBytes sends raw bytes to the PTY.
func SendBytes(p *Pty, data []byte) tea.Cmd {
	return func() tea.Msg {
		if p != nil {
			p.Write(data)
		}
		return nil
	}
}

const maxPtyOutputMessageSize = 64 * 1024

// Listen returns a bubbletea command that waits for PTY output. Chunks already
// queued at that moment are coalesced to avoid one full render per 4 KiB read.
func (p *Pty) Listen(sessionID string) tea.Cmd {
	return func() tea.Msg {
		select {
		case data := <-p.Output:
			combined := append([]byte(nil), data...)
			for len(combined) < maxPtyOutputMessageSize {
				select {
				case chunk := <-p.Output:
					combined = append(combined, chunk...)
				default:
					return PtyOutputMsg{SessionID: sessionID, Data: combined}
				}
			}
			return PtyOutputMsg{SessionID: sessionID, Data: combined}
		case err := <-p.Errors:
			return PtyExitMsg{SessionID: sessionID, Err: err}
		}
	}
}
