package portalis

import (
	"os"
	"testing"

	creackpty "github.com/creack/pty"
)

func TestPtyListenCoalescesQueuedOutput(t *testing.T) {
	p := &Pty{
		Output: make(chan []byte, 4),
		Errors: make(chan error, 1),
	}
	p.Output <- []byte("tmux")
	p.Output <- []byte("-")
	p.Output <- []byte("frame")

	msg := p.Listen("session")()
	output, ok := msg.(PtyOutputMsg)
	if !ok {
		t.Fatalf("message type = %T, want PtyOutputMsg", msg)
	}
	if output.SessionID != "session" {
		t.Fatalf("session = %q, want session", output.SessionID)
	}
	if got := string(output.Data); got != "tmux-frame" {
		t.Fatalf("coalesced output = %q, want %q", got, "tmux-frame")
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
