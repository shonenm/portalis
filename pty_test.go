package portalis

import (
	"bufio"
	"bytes"
	"os"
	"testing"

	creackpty "github.com/creack/pty"
)

type traceRecorder struct {
	bytes.Buffer
	closed bool
}

func (r *traceRecorder) Close() error {
	r.closed = true
	return nil
}

func TestPtyReadLoopCopiesRawBytesToTrace(t *testing.T) {
	const raw = "\x1b[38;2;139;26;26mstatus\x1b[0m"
	recorder := &traceRecorder{}
	chunks := &traceRecorder{}
	p := &Pty{
		reader:         bufio.NewReader(bytes.NewBufferString(raw)),
		rawTrace:       recorder,
		rawTraceChunks: chunks,
		Output:         make(chan []byte, 1),
		Errors:         make(chan error, 1),
		done:           make(chan struct{}),
	}

	p.readLoop()

	if got := recorder.String(); got != raw {
		t.Fatalf("raw trace = %q, want %q", got, raw)
	}
	if !recorder.closed {
		t.Fatal("raw trace was not closed after read loop exit")
	}
	if got, want := chunks.String(), "27\n"; got != want {
		t.Fatalf("chunk trace = %q, want %q", got, want)
	}
	if !chunks.closed {
		t.Fatal("chunk trace was not closed after read loop exit")
	}
	if got := string(<-p.Output); got != raw {
		t.Fatalf("PTY output = %q, want %q", got, raw)
	}
}

func TestPtyListenPreservesReadBoundaries(t *testing.T) {
	p := &Pty{
		Output: make(chan []byte, 2),
		Errors: make(chan error, 1),
	}
	p.Output <- []byte("tmux")
	p.Output <- []byte("frame")

	for _, expected := range []string{"tmux", "frame"} {
		msg := p.Listen("session")()
		output, ok := msg.(PtyOutputMsg)
		if !ok {
			t.Fatalf("message type = %T, want PtyOutputMsg", msg)
		}
		if output.SessionID != "session" {
			t.Fatalf("session = %q, want session", output.SessionID)
		}
		if got := string(output.Data); got != expected {
			t.Fatalf("PTY output = %q, want %q", got, expected)
		}
	}
}

func TestPtyResizeAppliesEveryDistinctFinalSize(t *testing.T) {
	var applied []creackpty.Winsize
	p := &Pty{
		ptmx: &os.File{},
		setSize: func(_ *os.File, size *creackpty.Winsize) error {
			applied = append(applied, *size)
			return nil
		},
	}

	if err := p.Resize(24, 80); err != nil {
		t.Fatal(err)
	}
	if err := p.Resize(40, 120); err != nil {
		t.Fatal(err)
	}
	if len(applied) != 2 {
		t.Fatalf("applied resize count = %d, want 2", len(applied))
	}
	if got := applied[1]; got.Rows != 40 || got.Cols != 120 {
		t.Fatalf("final physical size = %dx%d, want 40x120", got.Rows, got.Cols)
	}
	if p.lastRows != 40 || p.lastCols != 120 {
		t.Fatalf("recorded size = %dx%d, want 40x120", p.lastRows, p.lastCols)
	}
}
