package portalis

import (
	"encoding/binary"
	"hash/fnv"
	"strings"
	"testing"
	"time"

	xansi "github.com/charmbracelet/x/ansi"

	"github.com/shonenm/portalis/internal/stressfixture"
)

const (
	stressTimeout          = 500 * time.Millisecond
	stressExpectedChecksum = uint64(0xbe1bfb885413639f)
)

func TestANSIStressComplexStream(t *testing.T) {
	schemes := []struct {
		name  string
		sizes []int
	}{
		{name: "frame-sized"},
		{name: "pty-reads", sizes: []int{4096}},
		{name: "split-csi-utf8", sizes: []int{1, 2, 3, 5, 8, 13, 21, 34}},
	}

	for _, scheme := range schemes {
		t.Run(scheme.name, func(t *testing.T) {
			started := time.Now()
			screen := NewScreen(stressfixture.Rows, stressfixture.Cols)
			parser := NewParser(screen)

			for frame := 1; frame <= stressfixture.FrameCount; frame++ {
				stressfixture.FeedChunks(stressfixture.Frame(frame), scheme.sizes, parser.Feed)
			}

			elapsed := time.Since(started)
			if elapsed > stressTimeout {
				t.Fatalf("complex stream took %s, limit %s", elapsed, stressTimeout)
			}

			assertStressScreen(t, screen)
			checksum := stressScreenChecksum(screen)
			if checksum != stressExpectedChecksum {
				t.Fatalf("checksum = %016x, want %016x", checksum, stressExpectedChecksum)
			}
			t.Logf("stress checksum: %016x; elapsed: %s", checksum, elapsed)
		})
	}
}

func BenchmarkANSIStressComplexStream(b *testing.B) {
	frames := make([][]byte, stressfixture.FrameCount)
	for frame := 1; frame <= stressfixture.FrameCount; frame++ {
		frames[frame-1] = stressfixture.Frame(frame)
	}

	b.ReportAllocs()
	for range b.N {
		screen := NewScreen(stressfixture.Rows, stressfixture.Cols)
		parser := NewParser(screen)
		for _, frame := range frames {
			stressfixture.FeedChunks(frame, []int{4096}, parser.Feed)
		}
		_ = screen.Render()
	}
}

func assertStressScreen(t *testing.T, screen *Screen) {
	t.Helper()

	var plain strings.Builder
	for row := 0; row < screen.Rows; row++ {
		plain.WriteString(screen.LineText(row))
		plain.WriteByte('\n')
	}
	text := plain.String()
	if count := strings.Count(text, "FRAME "); count != 1 {
		t.Fatalf("dynamic status count = %d, want 1\n%s", count, text)
	}
	if !strings.Contains(text, "FRAME 0400") {
		t.Fatalf("missing final frame marker\n%s", text)
	}
	if strings.Contains(text, "FRAME 0399") {
		t.Fatalf("stale frame marker remains\n%s", text)
	}
	if !strings.Contains(text, "STRESS PASS") {
		t.Fatalf("missing completion marker\n%s", text)
	}

	renderedLines := strings.Split(screen.Render(), "\n")
	if len(renderedLines) != stressfixture.Rows {
		t.Fatalf("rendered rows = %d, want %d", len(renderedLines), stressfixture.Rows)
	}
	for row, line := range renderedLines {
		if width := xansi.StringWidth(line); width != stressfixture.Cols {
			t.Fatalf("row %d visible width = %d, want %d", row, width, stressfixture.Cols)
		}
	}

	for row, cells := range screen.Cells {
		for col, cell := range cells {
			if cell.Continuation {
				if col == 0 || cells[col-1].Continuation || cellDisplayWidth(cells[col-1]) != 2 {
					t.Fatalf("orphan continuation at row=%d col=%d", row, col)
				}
				continue
			}
			if cellDisplayWidth(cell) == 2 && (col+1 >= len(cells) || !cells[col+1].Continuation) {
				t.Fatalf("wide cell without continuation at row=%d col=%d", row, col)
			}
		}
	}
}

func stressScreenChecksum(screen *Screen) uint64 {
	hash := fnv.New64a()
	var encoded [8]byte
	for _, row := range screen.Cells {
		for _, cell := range row {
			binary.LittleEndian.PutUint32(encoded[:4], uint32(cell.Rune))
			encoded[4] = byte(cell.Style)
			if cell.Continuation {
				encoded[5] = 1
			} else {
				encoded[5] = 0
			}
			_, _ = hash.Write(encoded[:6])
			_, _ = hash.Write([]byte(cell.Combining))
			_, _ = hash.Write([]byte(cell.FG))
			_, _ = hash.Write([]byte(cell.BG))
		}
	}
	return hash.Sum64()
}
