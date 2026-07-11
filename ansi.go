package portalis

import (
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

// Parser parses ANSI escape sequences and updates a Screen.
type Parser struct {
	screen    *Screen
	state     ansiState
	buf       strings.Builder
	utf8Buf   []byte
	onCWD     func(string)
	lastCWD   string
}

type ansiState int

const (
	stateNormal ansiState = iota
	stateEscape
	stateCSI
	stateOSC
	statePaste
)

// NewParser creates a new ANSI parser for the given screen.
func NewParser(screen *Screen) *Parser {
	return &Parser{screen: screen}
}

// SetCWDCallback sets the callback invoked when the working directory changes.
func (p *Parser) SetCWDCallback(fn func(string)) {
	p.onCWD = fn
}

// flushUtf8 flushes any incomplete UTF-8 sequence as replacement chars.
func (p *Parser) flushUtf8() {
	if len(p.utf8Buf) == 0 {
		return
	}
	// Incomplete sequence — emit replacement characters.
	for range p.utf8Buf {
		p.screen.Put('\ufffd')
	}
	p.utf8Buf = p.utf8Buf[:0]
}

// utf8Valid returns true if the byte slice is a complete, valid UTF-8 sequence.
func utf8Valid(buf []byte) bool {
	return utf8.FullRune(buf) && utf8.Valid(buf)
}

// Feed feeds data into the parser.
func (p *Parser) Feed(data []byte) {
	for _, b := range data {
		p.feedByte(b)
	}
}

func (p *Parser) feedByte(b byte) {
	switch p.state {
	case stateNormal:
		if b == 0x1b {
			p.flushUtf8()
			p.state = stateEscape
			p.buf.Reset()
			return
		}
		if b == '\r' {
			p.flushUtf8()
			p.screen.Cursor.Col = 0
			p.screen.wrapPending = false
			return
		}
		if b == '\n' {
			p.flushUtf8()
			// If a wrap is pending, LF behaves like CR+LF.
			if p.screen.wrapPending {
				p.screen.wrapPending = false
				p.screen.Cursor.Col = 0
			}
			p.screen.Cursor.Row++
			if p.screen.Cursor.Row >= p.screen.Rows {
				p.screen.ScrollUp()
				p.screen.Cursor.Row = p.screen.Rows - 1
			}
			return
		}
		if b == '\t' {
			p.flushUtf8()
			p.screen.wrapPending = false
			next := (p.screen.Cursor.Col/8 + 1) * 8
			if next >= p.screen.Cols {
				next = p.screen.Cols - 1
			}
			p.screen.Cursor.Col = next
			return
		}
		if b == '\b' {
			p.flushUtf8()
			p.screen.wrapPending = false
			if p.screen.Cursor.Col > 0 {
				p.screen.Cursor.Col--
			}
			return
		}

		// Collect UTF-8 multi-byte sequences.
		if b >= 0x80 {
			p.utf8Buf = append(p.utf8Buf, b)
			if utf8Valid(p.utf8Buf) {
				r, _ := utf8.DecodeRune(p.utf8Buf)
				p.screen.Put(r)
				p.utf8Buf = p.utf8Buf[:0]
			}
			// Still collecting.
			return
		}

		// ASCII printable characters.
		if b >= 0x20 && b != 0x7f {
			p.screen.Put(rune(b))
		}

	case stateEscape:
		if b == '[' {
			p.state = stateCSI
			p.buf.Reset()
			return
		}
		if b == ']' {
			p.state = stateOSC
			p.buf.Reset()
			return
		}
		// Unknown escape, drop
		p.state = stateNormal

	case stateCSI:
		// Collect until final byte (0x40-0x7e)
		p.buf.WriteByte(b)
		if b >= 0x40 && b <= 0x7e {
			p.handleCSI(p.buf.String())
			p.state = stateNormal
			p.buf.Reset()
		}

	case stateOSC:
		if b == '\x07' {
			p.handleOSC(p.buf.String())
			p.state = stateNormal
			p.buf.Reset()
			return
		}
		if b == '\x1b' {
			// Expect \
			p.buf.WriteByte(b)
			return
		}
		if b == '\\' && p.buf.Len() > 0 && p.buf.String()[p.buf.Len()-1] == 0x1b {
			// Strip trailing ESC before handling.
			payload := p.buf.String()
			if len(payload) > 0 {
				payload = payload[:len(payload)-1]
			}
			p.handleOSC(payload)
			p.state = stateNormal
			p.buf.Reset()
			return
		}
		p.buf.WriteByte(b)

	case statePaste:
		// Bracketed paste: data is inserted literally until ESC[201~.
		if b == '\x1b' {
			p.state = stateEscape
			p.buf.Reset()
			return
		}
		p.screen.Put(rune(b))
	}
}

func (p *Parser) handleOSC(payload string) {
	if len(payload) < 2 {
		return
	}
	parts := strings.SplitN(payload, ";", 2)
	if len(parts) < 2 {
		return
	}
	if parts[0] != "7" {
		return
	}
	path := extractOSC7Path(parts[1])
	if path != "" && path != p.lastCWD {
		p.lastCWD = path
		if p.onCWD != nil {
			p.onCWD(path)
		}
	}
}

// extractOSC7Path extracts an absolute filesystem path from an OSC 7 payload.
// Supported forms:
//   file://hostname/path  → /path
//   /absolute/path        → /absolute/path
func extractOSC7Path(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "file://") {
		// Strip scheme and hostname.
		s = s[len("file://"):]
		if i := strings.IndexByte(s, '/'); i >= 0 {
			return s[i:]
		}
		return ""
	}
	if filepath.IsAbs(s) {
		return s
	}
	return ""
}

func (p *Parser) handleCSI(seq string) {
	if len(seq) == 0 {
		return
	}
	final := seq[len(seq)-1]
	rawParams := seq[:len(seq)-1]

	// Detect private marker (? for DEC private, >/< for other private forms).
	isPrivate := false
	if len(rawParams) > 0 {
		switch rawParams[0] {
		case '?', '>', '<', '=', '!':
			isPrivate = true
			rawParams = rawParams[1:]
		}
	}
	params := parseParams(rawParams)

	switch final {
	case 'm':
		if isPrivate {
			break // ignore private SGR
		}
		p.handleSGR(params)
	case 'H', 'f':
		row := 1
		col := 1
		if len(params) > 0 {
			row = params[0]
		}
		if len(params) > 1 {
			col = params[1]
		}
		p.screen.SetCursor(row-1, col-1)
	case 'J':
		n := 0
		if len(params) > 0 {
			n = params[0]
		}
		switch n {
		case 0:
			p.clearFromCursor()
		case 2:
			p.screen.Clear()
			p.screen.SetCursor(0, 0)
		}
	case 'K':
		n := 0
		if len(params) > 0 {
			n = params[0]
		}
		switch n {
		case 0:
			p.screen.ClearLine()
		case 1:
			p.screen.ClearLineLeft()
		case 2:
			p.screen.ClearLineAll()
		}
	case 'A':
		n := 1
		if len(params) > 0 && params[0] > 0 {
			n = params[0]
		}
		p.screen.SetCursor(p.screen.Cursor.Row-n, p.screen.Cursor.Col)
	case 'B':
		n := 1
		if len(params) > 0 && params[0] > 0 {
			n = params[0]
		}
		p.screen.SetCursor(p.screen.Cursor.Row+n, p.screen.Cursor.Col)
	case 'C':
		n := 1
		if len(params) > 0 && params[0] > 0 {
			n = params[0]
		}
		p.screen.SetCursor(p.screen.Cursor.Row, p.screen.Cursor.Col+n)
	case 'D':
		n := 1
		if len(params) > 0 && params[0] > 0 {
			n = params[0]
		}
		p.screen.SetCursor(p.screen.Cursor.Row, p.screen.Cursor.Col-n)
	case 'E':
		n := 1
		if len(params) > 0 && params[0] > 0 {
			n = params[0]
		}
		p.screen.SetCursor(p.screen.Cursor.Row+n, 0)
	case 'F':
		n := 1
		if len(params) > 0 && params[0] > 0 {
			n = params[0]
		}
		p.screen.SetCursor(p.screen.Cursor.Row-n, 0)
	case 'G':
		col := 1
		if len(params) > 0 && params[0] > 0 {
			col = params[0]
		}
		p.screen.SetCursor(p.screen.Cursor.Row, col-1)
	case 'M':
		// Reverse line feed / scroll down
		p.screen.ScrollUp() // simplified
	case 's':
		// Save cursor (DECSC) — only standard sequences.
		if !isPrivate {
			p.screen.SaveCursor()
		}
	case 'u':
		// Restore cursor (DECRC) — only standard sequences.
		// Private forms like CSI ? u or CSI > 7 u are kitty keyboard protocol.
		if !isPrivate {
			p.screen.RestoreCursor()
		}
	case 'r':
		// Set scroll region (DECSTBM)
		top := 1
		bottom := p.screen.Rows
		if len(params) > 0 {
			top = params[0]
		}
		if len(params) > 1 {
			bottom = params[1]
		}
		p.screen.SetScrollRegion(top, bottom)
	case 'h':
		// DECSET — set mode
		if isPrivate && len(params) > 0 {
			switch params[0] {
			case 1049:
				// Alternate screen buffer
				p.screen.EnterAltScreen()
			case 2026:
				// Synchronized output: suppress rendering until disabled.
				p.screen.SetSync(true)
			}
		}
	case 'l':
		// DECRST — reset mode
		if isPrivate && len(params) > 0 {
			switch params[0] {
			case 1049:
				// Exit alternate screen buffer
				p.screen.ExitAltScreen()
			case 2026:
				// End synchronized output: commit the current frame.
				p.screen.SetSync(false)
			}
		}
	case '~':
		// Bracketed paste sequences.
		if len(params) > 0 {
			switch params[0] {
			case 200:
				p.state = statePaste
			case 201:
				p.state = stateNormal
			}
		}
	}
}

func parseParams(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	var out []int
	for _, part := range parts {
		if part == "" {
			out = append(out, 0)
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			out = append(out, 0)
			continue
		}
		out = append(out, n)
	}
	return out
}

func (p *Parser) clearFromCursor() {
	row := p.screen.Cursor.Row
	col := p.screen.Cursor.Col
	for c := col; c < p.screen.Cols; c++ {
		p.screen.Cells[row][c] = Cell{}
	}
	for r := row + 1; r < p.screen.Rows; r++ {
		for c := 0; c < p.screen.Cols; c++ {
			p.screen.Cells[r][c] = Cell{}
		}
	}
}

func (p *Parser) handleSGR(params []int) {
	if len(params) == 0 {
		params = []int{0}
	}
	for i := 0; i < len(params); i++ {
		code := params[i]
		switch {
		case code == 0:
			p.screen.Cursor.FG = ""
			p.screen.Cursor.BG = ""
			p.screen.Cursor.Style = 0
		case code == 1:
			p.screen.Cursor.Style |= StyleBold
		case code == 2:
			p.screen.Cursor.Style |= StyleDim
		case code == 3:
			p.screen.Cursor.Style |= StyleItalic
		case code == 4:
			p.screen.Cursor.Style |= StyleUnderline
		case code == 5:
			p.screen.Cursor.Style |= StyleBlink
		case code == 7:
			p.screen.Cursor.Style |= StyleReverse
		case code == 8:
			p.screen.Cursor.Style |= StyleHidden
		case code == 9:
			p.screen.Cursor.Style |= StyleStrikethrough
		case code == 22:
			p.screen.Cursor.Style &^= StyleBold | StyleDim
		case code == 23:
			p.screen.Cursor.Style &^= StyleItalic
		case code == 24:
			p.screen.Cursor.Style &^= StyleUnderline
		case code == 25:
			p.screen.Cursor.Style &^= StyleBlink
		case code == 27:
			p.screen.Cursor.Style &^= StyleReverse
		case code == 28:
			p.screen.Cursor.Style &^= StyleHidden
		case code == 29:
			p.screen.Cursor.Style &^= StyleStrikethrough
		case code >= 30 && code <= 37:
			p.screen.Cursor.FG = ansi256Color(code - 30)
		case code == 38:
			if i+2 < len(params) && params[i+1] == 5 {
				p.screen.Cursor.FG = ansi256Color(params[i+2])
				i += 2
			} else if i+4 < len(params) && params[i+1] == 2 {
				p.screen.Cursor.FG = lipgloss.Color(rgb(params[i+2], params[i+3], params[i+4]))
				i += 4
			}
		case code == 39:
			p.screen.Cursor.FG = ""
		case code >= 40 && code <= 47:
			p.screen.Cursor.BG = ansi256Color(code - 40)
		case code == 48:
			if i+2 < len(params) && params[i+1] == 5 {
				p.screen.Cursor.BG = ansi256Color(params[i+2])
				i += 2
			} else if i+4 < len(params) && params[i+1] == 2 {
				p.screen.Cursor.BG = lipgloss.Color(rgb(params[i+2], params[i+3], params[i+4]))
				i += 4
			}
		case code == 49:
			p.screen.Cursor.BG = ""
		case code >= 90 && code <= 97:
			p.screen.Cursor.FG = ansi256Color(code - 90 + 8)
		case code >= 100 && code <= 107:
			p.screen.Cursor.BG = ansi256Color(code - 100 + 8)
		}
	}
}

func ansi256Color(n int) lipgloss.Color {
	if n < 0 || n > 255 {
		return ""
	}
	if n < 16 {
		// Standard colors
		colors := []string{
			"#000000", "#800000", "#008000", "#808000",
			"#000080", "#800080", "#008080", "#c0c0c0",
			"#808080", "#ff0000", "#00ff00", "#ffff00",
			"#0000ff", "#ff00ff", "#00ffff", "#ffffff",
		}
		return lipgloss.Color(colors[n])
	}
	if n < 232 {
		// 6x6x6 color cube
		n -= 16
		r := n / 36
		g := (n / 6) % 6
		b := n % 6
		return lipgloss.Color(rgb(r*42+12, g*42+12, b*42+12))
	}
	// Grayscale
	v := (n-232)*10 + 8
	return lipgloss.Color(rgb(v, v, v))
}

func rgb(r, g, b int) string {
	return "#" + hexByte(r) + hexByte(g) + hexByte(b)
}

func hexByte(v int) string {
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	h := strconv.FormatInt(int64(v), 16)
	if len(h) == 1 {
		return "0" + h
	}
	return h
}
