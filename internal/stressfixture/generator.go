package stressfixture

import (
	"fmt"
	"strings"
)

const (
	Rows       = 30
	Cols       = 100
	FrameCount = 400
)

// Frame returns one deterministic terminal frame containing mixed ANSI,
// OSC, DEC line drawing, scrolling, editing, and Unicode sequences.
func Frame(number int) []byte {
	status := "RUNNING"
	if number == FrameCount {
		status = "STRESS PASS"
	}

	var frame strings.Builder
	frame.Grow(2048)
	frame.WriteString("\x1b[?2026h")
	frame.WriteString("\x1b[2J\x1b[H")
	frame.WriteString("\x1b[1;1H\x1b[1;38;2;214;93;14mPORTALIS ANSI STRESS\x1b[0m")
	frame.WriteString("\x1b[2;1H\x1b[38;5;39mSGR 256\x1b[0m | \x1b[38;2;120;200;80mTRUECOLOR\x1b[0m | \x1b[4mUNDERLINE\x1b[0m")
	frame.WriteString("\x1b[4;1HUnicode: 世界 ✅ ❤️ 👩‍💻 👍🏽 e\u0301")
	frame.WriteString("\x1b[6;1H\x1b(0lqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqk\x1b(B")
	frame.WriteString("\x1b[7;1H\x1b(0x\x1b(B DEC line drawing + cursor editing                 \x1b(0x\x1b(B")
	frame.WriteString("\x1b[8;1H\x1b(0mqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqj\x1b(B")

	frame.WriteString("\x1b[10;1HEDIT: ABCDE")
	frame.WriteString("\x1b[10;7H\x1b[2@XY\x1b[1P\x1b[1X")
	frame.WriteString("\x1b[11;1HRELATIVE")
	frame.WriteString("\x1b[2D>>\x1b[1C<\x1b[1A\x1b[1B")

	frame.WriteString("\x1b[13;20r")
	frame.WriteString("\x1b[13;1Hscroll-a\r\nscroll-b\r\nscroll-c\r\nscroll-d\r\nscroll-e\r\nscroll-f\r\nscroll-g\r\nscroll-h")
	frame.WriteString("\x1b[2S\x1b[1T")
	frame.WriteString(fmt.Sprintf("\x1b[15;1HSCROLL CHECK %04d", number))
	frame.WriteString(fmt.Sprintf("\x1b[16;1HINSERT %04d\x1b[1L\x1b[1M", number))
	frame.WriteString("\x1b[r")

	frame.WriteString("\x1b]7;file://localhost/Users/a/Space\x1b\\")
	frame.WriteString("\x1b[22;1H\x1b]8;;https://example.invalid/stress\x1b\\OSC 8 LINK\x1b]8;;\x1b\\")
	frame.WriteString(fmt.Sprintf("\x1b[25;1HChecksum seed: %08x", uint32(number*2654435761)))
	frame.WriteString(fmt.Sprintf("\x1b[27;1HFRAME %04d", number))
	frame.WriteString("\x1b[28;1H")
	frame.WriteString(status)
	frame.WriteString("\x1b[29;1HNo corruption | width=100 | rows=30")
	frame.WriteString("\x1b[30;1HREADY")
	frame.WriteString("\x1b[?2026l")
	return []byte(frame.String())
}

// FeedChunks splits data with a deterministic repeating size pattern.
func FeedChunks(data []byte, sizes []int, feed func([]byte)) {
	if len(sizes) == 0 {
		feed(data)
		return
	}
	offset := 0
	chunk := 0
	for offset < len(data) {
		size := sizes[chunk%len(sizes)]
		if size <= 0 {
			size = 1
		}
		end := offset + size
		if end > len(data) {
			end = len(data)
		}
		feed(data[offset:end])
		offset = end
		chunk++
	}
}
