package folio

import (
	"fmt"
	"strings"

	"github.com/akkaraponph/folio/internal/content"
	"github.com/akkaraponph/folio/internal/resources"
	"github.com/akkaraponph/folio/internal/state"
)

// Page represents a single PDF page with drawing methods.
type Page struct {
	doc    *Document
	stream content.Stream
	size   PageSize
	w, h   float64 // page dimensions in user units

	// cursor position in user units
	x, y float64

	// page-local font state
	fontFamily string
	fontStyle  string
	fontSizePt float64
	fontEntry  *resources.FontEntry

	// next is a forwarding pointer set by auto page break. When a page
	// break occurs, the old page's next points to the new page so that
	// stale *Page references transparently follow to the active page.
	next *Page

	// links holds hyperlink annotations for this page.
	links []linkAnnotation
}

// linkAnnotation represents a hyperlink annotation on a page.
type linkAnnotation struct {
	x, y, w, h float64 // rect in user units
	url         string  // non-empty for URL links
	anchor      string  // non-empty for internal links
}

// --- Page break support ---

// active follows the forwarding chain to the current active page.
// When auto page break creates a new page, the old page's next field
// points to it. This lets stale *Page references seamlessly reach the
// latest page.
func (p *Page) active() *Page {
	for p.next != nil {
		p = p.next
	}
	return p
}

// checkPageBreak creates a new page if content of height h would overflow
// past the bottom margin. Returns the page to draw on (p itself or the
// newly created page).
func (p *Page) checkPageBreak(h float64) *Page {
	d := p.doc
	if !d.autoPageBreak || d.inHeader || d.inFooter {
		return p
	}
	// Would content overflow?
	if p.y+h <= p.h-d.bMargin {
		return p
	}
	// If we're already at the top margin (fresh page), allow overflow to
	// prevent an infinite loop when a single cell is taller than the page.
	if p.y <= d.tMargin {
		return p
	}
	np := d.AddPage(p.size)
	p.next = np
	return np
}

// PageBreakTrigger returns the Y position (in user units) at which an
// automatic page break would be triggered.
func (p *Page) PageBreakTrigger() float64 {
	return p.h - p.doc.bMargin
}

// --- Text methods ---

// TextAt draws text at the given position (in user units, top-left origin).
func (p *Page) TextAt(x, y float64, text string) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	fe := p.effectiveFontEntry()
	if fe == nil {
		p.doc.err = fmt.Errorf("TextAt: no font set")
		return
	}

	k := p.doc.k
	xPt := state.ToPointsX(x, k)
	// Position text baseline: y + 0.7 * fontSize (approximate ascender)
	fontSize := p.effectiveFontSizePt() / k
	yPt := state.ToPointsY(y+0.7*fontSize, p.h, k)

	// Handle text color and decoration
	d := p.doc
	tc := d.textColor
	needDeco := d.underline || d.strikethrough
	needState := !tc.IsBlack() || needDeco
	if needState {
		p.stream.SaveState()
		p.stream.SetFillColorRGB(tc.R, tc.G, tc.B)
	}

	p.stream.BeginText()
	p.stream.SetFont("F"+fe.Index, p.effectiveFontSizePt())
	p.stream.MoveText(xPt, yPt)
	p.emitText(fe, text)
	p.stream.EndText()

	if d.underline {
		p.drawTextDecoration(x, y+0.7*fontSize, text)
	}
	if d.strikethrough {
		p.drawTextDecoration(x, y+0.3*fontSize, text)
	}

	if needState {
		p.stream.RestoreState()
	}
}

// SetFont sets the font for this page.
func (p *Page) SetFont(family, style string, size float64) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	fe, err := p.doc.fonts.Register(family, style)
	if err != nil {
		p.doc.err = fmt.Errorf("Page.SetFont: %w", err)
		return
	}
	p.fontFamily = family
	p.fontStyle = style
	p.fontSizePt = size
	p.fontEntry = fe
	p.applyFont(fe, size)
}

// SetFontSize changes the font size for this page without changing the family or style.
func (p *Page) SetFontSize(size float64) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	fe := p.effectiveFontEntry()
	if fe == nil {
		p.doc.err = fmt.Errorf("Page.SetFontSize: no font set")
		return
	}
	p.fontSizePt = size
	p.applyFont(fe, size)
}

// SetFontStyle changes the font style for this page (e.g. "B", "I", "BI", "")
// without changing the family or size. The font family+style must already be registered.
func (p *Page) SetFontStyle(style string) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	family := p.fontFamily
	if family == "" {
		family = p.doc.fontFamily
	}
	if family == "" {
		p.doc.err = fmt.Errorf("Page.SetFontStyle: no font set")
		return
	}
	fe, ok := p.doc.fonts.Get(family, style)
	if !ok {
		var err error
		fe, err = p.doc.fonts.Register(family, style)
		if err != nil {
			p.doc.err = fmt.Errorf("Page.SetFontStyle: %w", err)
			return
		}
	}
	p.fontFamily = family
	p.fontStyle = style
	p.fontEntry = fe
	p.applyFont(fe, p.effectiveFontSizePt())
}

// GetFontFamily returns the effective font family for this page.
func (p *Page) GetFontFamily() string {
	p = p.active()
	if p.fontFamily != "" {
		return p.fontFamily
	}
	return p.doc.fontFamily
}

// GetFontStyle returns the effective font style for this page.
func (p *Page) GetFontStyle() string {
	p = p.active()
	if p.fontEntry != nil {
		return p.fontStyle
	}
	return p.doc.fontStyle
}

// GetFontSize returns the effective font size in points for this page.
func (p *Page) GetFontSize() float64 {
	return p.active().effectiveFontSizePt()
}

// GetStringWidth returns the width of s in user units using the current font.
func (p *Page) GetStringWidth(s string) float64 {
	p = p.active()
	fe := p.effectiveFontEntry()
	if fe == nil {
		return 0
	}
	var w int
	if fe.Type == "TTF" {
		w = resources.StringWidthUTF8(fe, s)
	} else {
		w = resources.StringWidth(fe, s)
	}
	return float64(w) * p.effectiveFontSizePt() / 1000.0 / p.doc.k
}

// --- Drawing methods ---

// Line draws a line segment from (x1,y1) to (x2,y2) in user units.
func (p *Page) Line(x1, y1, x2, y2 float64) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	p.stream.MoveTo(
		state.ToPointsX(x1, k),
		state.ToPointsY(y1, p.h, k),
	)
	p.stream.LineTo(
		state.ToPointsX(x2, k),
		state.ToPointsY(y2, p.h, k),
	)
	p.stream.Stroke()
}

// Rect draws a rectangle. style: "D" (draw/stroke), "F" (fill), "DF" or "FD" (both).
func (p *Page) Rect(x, y, w, h float64, style string) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	p.stream.Rect(
		state.ToPointsX(x, k),
		state.ToPointsY(y+h, p.h, k), // bottom-left corner in PDF coords
		w*k,
		h*k,
	)
	style = strings.ToUpper(style)
	switch style {
	case "F":
		p.stream.Fill()
	case "DF", "FD":
		p.stream.FillStroke()
	default: // "D" or ""
		p.stream.Stroke()
	}
}

// DrawImageRect draws a registered image at (x, y) with the given width and height.
func (p *Page) DrawImageRect(name string, x, y, w, h float64) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	entry, ok := p.doc.images.Get(name)
	if !ok {
		p.doc.err = fmt.Errorf("DrawImageRect: image %q not registered", name)
		return
	}

	k := p.doc.k
	wPt := w * k
	hPt := h * k
	xPt := state.ToPointsX(x, k)
	yPt := state.ToPointsY(y+h, p.h, k) // bottom-left of image in PDF coords

	p.stream.DrawImage("Im"+entry.Name, wPt, 0, 0, hPt, xPt, yPt)
}

// --- Cell and MultiCell ---

// Cell draws a single-line cell at the current cursor position.
// w: cell width (0 = extend to right margin)
// h: cell height
// text: cell text
// border: "" (none), "1" (full), or combination of "L","T","R","B"
// align: "L" (left, default), "C" (center), "R" (right)
// fill: if true, fill background with current fill color
// ln: 0 = cursor right, 1 = next line, 2 = below
func (p *Page) Cell(w, h float64, text, border, align string, fill bool, ln int) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	p = p.checkPageBreak(h)
	d := p.doc
	k := d.k

	if w == 0 {
		w = p.w - d.rMargin - p.x
	}

	// Draw fill/border (isolated so fill color doesn't leak into text)
	if fill || border == "1" {
		p.stream.SaveState()
		fc := d.fillColor
		p.stream.SetFillColorRGB(fc.R, fc.G, fc.B)
		if fill {
			p.stream.Rect(
				state.ToPointsX(p.x, k),
				state.ToPointsY(p.y+h, p.h, k),
				w*k, h*k,
			)
			if border == "1" {
				p.stream.FillStroke()
			} else {
				p.stream.Fill()
			}
		} else if border == "1" {
			p.stream.Rect(
				state.ToPointsX(p.x, k),
				state.ToPointsY(p.y+h, p.h, k),
				w*k, h*k,
			)
			p.stream.Stroke()
		}
		p.stream.RestoreState()
	}

	// Individual borders
	if strings.Contains(border, "L") {
		p.drawBorderLine(p.x, p.y, p.x, p.y+h)
	}
	if strings.Contains(border, "T") {
		p.drawBorderLine(p.x, p.y, p.x+w, p.y)
	}
	if strings.Contains(border, "R") {
		p.drawBorderLine(p.x+w, p.y, p.x+w, p.y+h)
	}
	if strings.Contains(border, "B") {
		p.drawBorderLine(p.x, p.y+h, p.x+w, p.y+h)
	}

	// Draw text
	if text != "" {
		fe := p.effectiveFontEntry()
		if fe == nil {
			d.err = fmt.Errorf("Cell: no font set")
			return
		}
		fontSize := p.effectiveFontSizePt()
		fontSizeUser := fontSize / k

		// Text X position based on alignment
		var dx float64
		sw := p.GetStringWidth(text)
		switch strings.ToUpper(align) {
		case "C":
			dx = (w - sw) / 2
		case "R":
			dx = w - d.cMargin - sw
		default: // "L"
			dx = d.cMargin
		}

		// Text Y: vertically center in cell
		textX := state.ToPointsX(p.x+dx, k)
		textY := state.ToPointsY(p.y+0.5*h+0.3*fontSizeUser, p.h, k)

		// Always set text color explicitly since fill color may have been
		// changed by background drawing above.
		tc := d.textColor
		p.stream.SaveState()
		p.stream.SetFillColorRGB(tc.R, tc.G, tc.B)

		p.stream.BeginText()
		p.stream.SetFont("F"+fe.Index, fontSize)
		p.stream.MoveText(textX, textY)
		p.emitText(fe, text)
		p.stream.EndText()

		if d.underline {
			p.drawTextDecoration(p.x+dx, p.y+0.5*h+0.3*fontSizeUser, text)
		}
		if d.strikethrough {
			p.drawTextDecoration(p.x+dx, p.y+0.5*h-0.1*fontSizeUser, text)
		}

		p.stream.RestoreState()
	}

	// Advance cursor
	switch ln {
	case 0:
		p.x += w
	case 1:
		p.x = d.lMargin
		p.y += h
	case 2:
		p.y += h
	}
}

// MultiCell draws multi-line text with automatic word wrapping.
// w: cell width (0 = extend to right margin)
// h: line height
// text: text content (may contain \n)
// border: "" (none), "1" (full), or combination of "L","T","R","B"
// align: "L", "C", "R", "J" (justified)
// fill: if true, fill background with current fill color
func (p *Page) MultiCell(w, h float64, text, border, align string, fill bool) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	d := p.doc

	if w == 0 {
		w = p.w - d.rMargin - p.x
	}

	fe := p.effectiveFontEntry()
	if fe == nil {
		d.err = fmt.Errorf("MultiCell: no font set")
		return
	}
	fontSize := p.effectiveFontSizePt()

	// Available width in 1/1000 font units
	wmax := (w - 2*d.cMargin) * 1000.0 / fontSize * d.k

	// Split text into lines, then wrap each line
	lines := p.wrapText(text, fe, wmax)

	for i, line := range lines {
		p = p.active()

		// Determine borders for this line
		b := ""
		if border == "1" {
			if i == 0 {
				b = "LTR"
			} else if i == len(lines)-1 {
				b = "LRB"
			} else {
				b = "LR"
			}
		} else {
			if i == 0 && strings.Contains(border, "T") {
				b += "T"
			}
			if strings.Contains(border, "L") {
				b += "L"
			}
			if strings.Contains(border, "R") {
				b += "R"
			}
			if i == len(lines)-1 && strings.Contains(border, "B") {
				b += "B"
			}
		}

		p.Cell(w, h, line, b, align, fill, 2)
	}

	// Move to left margin
	p = p.active()
	p.x = d.lMargin
}

// Write draws text at the current cursor position with word-wrapping. Unlike
// Cell, it does not draw a cell box (no border or background). The cursor
// advances to the right of the drawn text, staying on the same line until
// a word would overflow the right margin, at which point it wraps.
//
// h is the line height used when wrapping to a new line.
//
// This method enables mixed-format inline text: call Write, change the font
// or style, then call Write again to continue on the same line.
//
//	page.Write(6, "This is ")
//	doc.SetFontStyle("B")
//	page.Write(6, "bold")
//	doc.SetFontStyle("")
//	page.Write(6, " and this is normal.")
func (p *Page) Write(h float64, text string) {
	p = p.active()
	d := p.doc
	if d.err != nil {
		return
	}

	fe := p.effectiveFontEntry()
	if fe == nil {
		d.err = fmt.Errorf("Write: no font set")
		return
	}

	text = strings.ReplaceAll(text, "\r\n", "\n")
	runes := []rune(text)
	n := len(runes)
	fontSize := p.effectiveFontSizePt()
	k := d.k

	// runeW returns the width of a single rune in user units.
	runeW := func(r rune) float64 {
		var w int
		if fe.Type == "TTF" && fe.TTF != nil {
			ch := int(r)
			if ch < len(fe.TTF.CharWidths) {
				w = fe.TTF.CharWidths[ch]
			}
		} else if r < 256 {
			w = fe.Widths[byte(r)]
		}
		return float64(w) * fontSize / 1000.0 / k
	}

	sep := -1  // index of last space in current segment
	i := 0     // current rune position
	j := 0     // start of current segment
	lineW := 0.0 // accumulated width of runes[j..i) in user units

	for i < n {
		p = p.active()
		r := runes[i]

		// Explicit newline
		if r == '\n' {
			p.writeSegment(string(runes[j:i]), h)
			p = p.active()
			p.x = d.lMargin
			p.y += h
			p = p.checkPageBreak(h)
			i++
			j = i
			sep = -1
			lineW = 0
			continue
		}

		if r == ' ' {
			sep = i
		}

		cw := runeW(r)
		avail := p.w - d.rMargin - p.x

		if lineW+cw > avail && lineW > 0 {
			// Line overflow — choose break strategy.
			if sep >= j {
				// Break at last space.
				p.writeSegment(string(runes[j:sep]), h)
				i = sep + 1
				j = i
			} else if p.x > d.lMargin {
				// No space found but we started mid-line. Wrap to the left
				// margin where the full line width is available, then
				// re-measure the segment from scratch.
				i = j
			} else {
				// At left margin, no space — force break before current rune.
				if i == j {
					// Single rune wider than line: output it to avoid looping.
					p.writeSegment(string(runes[j:i+1]), h)
					i++
					j = i
				} else {
					p.writeSegment(string(runes[j:i]), h)
					j = i
				}
			}
			// Advance to next line.
			p = p.active()
			p.x = d.lMargin
			p.y += h
			p = p.checkPageBreak(h)
			sep = -1
			lineW = 0
		} else {
			lineW += cw
			i++
		}
	}

	// Output remaining segment.
	if j < n {
		p = p.active()
		p.writeSegment(string(runes[j:n]), h)
	}
}

// writeSegment draws text at the current cursor position and advances the
// cursor by the string width. It handles text color, underline, and
// strikethrough but draws no cell border or background.
func (p *Page) writeSegment(text string, h float64) {
	if text == "" {
		return
	}
	p = p.active()
	d := p.doc
	k := d.k
	fontSize := p.effectiveFontSizePt()
	fontSizeUser := fontSize / k
	fe := p.effectiveFontEntry()
	if fe == nil {
		return
	}

	sw := p.GetStringWidth(text)

	// Text baseline position: vertically centred in the line height.
	textX := state.ToPointsX(p.x, k)
	textY := state.ToPointsY(p.y+0.5*h+0.3*fontSizeUser, p.h, k)

	tc := d.textColor
	needDeco := d.underline || d.strikethrough
	needState := !tc.IsBlack() || needDeco
	if needState {
		p.stream.SaveState()
		p.stream.SetFillColorRGB(tc.R, tc.G, tc.B)
	}

	p.stream.BeginText()
	p.stream.SetFont("F"+fe.Index, fontSize)
	p.stream.MoveText(textX, textY)
	p.emitText(fe, text)
	p.stream.EndText()

	if d.underline {
		p.drawTextDecoration(p.x, p.y+0.5*h+0.3*fontSizeUser, text)
	}
	if d.strikethrough {
		p.drawTextDecoration(p.x, p.y+0.5*h-0.1*fontSizeUser, text)
	}

	if needState {
		p.stream.RestoreState()
	}

	// Advance cursor horizontally.
	p.x += sw
}

// thaiIsClusterContinuation reports whether r must stay attached to the
// preceding base character and therefore cannot start a new line. This
// covers above/below vowels, tone marks, and trailing vowel/iteration marks.
func thaiIsClusterContinuation(r rune) bool {
	switch r {
	case 0x0E30, 0x0E32, 0x0E33, 0x0E45, 0x0E46: // ะ า ำ ๅ ๆ
		return true
	case 0x0E31, 0x0E47, 0x0E4D: // ั ็ ํ
		return true
	}
	if r >= 0x0E34 && r <= 0x0E3A { // ิ ี ึ ื ฺ ุ ู
		return true
	}
	if r >= 0x0E48 && r <= 0x0E4C { // ่ ้ ๊ ๋ ์
		return true
	}
	return false
}

// thaiIsLeadingVowel reports whether r is a Thai pre-vowel (written to the
// left of its base consonant). A line must not end on one of these — the
// pre-vowel has to move to the next line together with its consonant.
func thaiIsLeadingVowel(r rune) bool {
	return r >= 0x0E40 && r <= 0x0E44 // เ แ โ ใ ไ
}

// wrapText splits text by newlines, then wraps each line to fit within wmax
// (measured in 1/1000 font units).
func (p *Page) wrapText(text string, fe *resources.FontEntry, wmax float64) []string {
	var lines []string
	paragraphs := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")

	isTTF := fe.Type == "TTF"

	stringWidth := func(s string) int {
		if isTTF {
			return resources.StringWidthUTF8(fe, s)
		}
		return resources.StringWidth(fe, s)
	}
	spaceWidth := func() int {
		if isTTF && fe.TTF != nil {
			return fe.TTF.CharWidths[' ']
		}
		return fe.Widths[' ']
	}

	for _, para := range paragraphs {
		if para == "" {
			lines = append(lines, "")
			continue
		}

		line := ""
		lineWidth := 0

		// breakLongWord splits a word whose width exceeds wmax at cluster
		// boundaries. Required for scripts without inter-word spacing (e.g.
		// Thai) and for accidental unbroken runs. Break points respect Thai
		// cluster rules: tone marks / above-below vowels stay with their
		// base consonant, and pre-vowels (เ แ โ ใ ไ) move with their
		// following consonant.
		breakLongWord := func(word string) {
			runes := []rune(word)
			start := 0
			curW := 0
			lastBreak := -1

			for i := 0; i < len(runes); i++ {
				r := runes[i]
				rw := stringWidth(string(r))

				// Record a potential break-before-i if valid.
				if i > start &&
					!thaiIsClusterContinuation(r) &&
					!thaiIsLeadingVowel(runes[i-1]) {
					lastBreak = i
				}

				if curW+rw > int(wmax) && i > start {
					breakAt := lastBreak
					if breakAt <= start {
						// No cluster-safe break found — force-break at i
						// (one rune at least already fits).
						breakAt = i
					}
					lines = append(lines, string(runes[start:breakAt]))
					start = breakAt
					curW = 0
					lastBreak = -1
					// Reprocess runes[breakAt..] on the new line.
					i = start - 1
					continue
				}

				curW += rw
			}

			if start < len(runes) {
				line = string(runes[start:])
				lineWidth = curW
			}
		}

		if p.doc.wordBreaker != nil {
			// Custom segmenter path — segments are joined verbatim. The
			// segmenter owns whitespace, so no space is inserted between
			// segments (this is what Thai word breakers want).
			segments := p.doc.wordBreaker(para)
			for _, seg := range segments {
				if seg == "" {
					continue
				}
				segWidth := stringWidth(seg)

				if lineWidth+segWidth > int(wmax) && line != "" {
					// Trim trailing space on the broken line so justified
					// text doesn't have a hanging gap.
					lines = append(lines, strings.TrimRight(line, " "))
					line = ""
					lineWidth = 0
					// Skip a pure-whitespace segment at the start of a line.
					if strings.TrimSpace(seg) == "" {
						continue
					}
				}

				if segWidth > int(wmax) {
					if line != "" {
						lines = append(lines, strings.TrimRight(line, " "))
						line = ""
						lineWidth = 0
					}
					breakLongWord(seg)
					continue
				}

				line += seg
				lineWidth += segWidth
			}
			if line != "" {
				lines = append(lines, line)
			}
			continue
		}

		words := strings.Fields(para)
		for _, word := range words {
			wordWidth := stringWidth(word)
			sw := 0
			if line != "" {
				sw = spaceWidth()
			}

			if lineWidth+sw+wordWidth > int(wmax) && line != "" {
				lines = append(lines, line)
				line = ""
				lineWidth = 0
			}

			// If the word itself is wider than wmax, break it at rune boundaries.
			if wordWidth > int(wmax) {
				if line != "" {
					lines = append(lines, line)
					line = ""
					lineWidth = 0
				}
				breakLongWord(word)
				continue
			}

			if line != "" {
				line += " "
				lineWidth += sw
			}
			line += word
			lineWidth += wordWidth
		}
		if line != "" {
			lines = append(lines, line)
		}
	}

	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

// drawBorderLine draws a single border line segment.
func (p *Page) drawBorderLine(x1, y1, x2, y2 float64) {
	k := p.doc.k
	p.stream.MoveTo(state.ToPointsX(x1, k), state.ToPointsY(y1, p.h, k))
	p.stream.LineTo(state.ToPointsX(x2, k), state.ToPointsY(y2, p.h, k))
	p.stream.Stroke()
}

// --- Dimension getters ---

// Width returns the page width in user units.
func (p *Page) Width() float64 { return p.active().w }

// Height returns the page height in user units.
func (p *Page) Height() float64 { return p.active().h }

// --- Cursor methods ---

// SetX sets the X cursor position.
func (p *Page) SetX(x float64) { p.active().x = x }

// SetY sets the Y cursor position. A negative value is interpreted as
// distance from the bottom of the page (e.g. -15 means 15 user-units
// above the page edge), which is convenient for footer positioning.
func (p *Page) SetY(y float64) {
	q := p.active()
	if y < 0 {
		y = q.h + y
	}
	q.y = y
}

// SetXY sets both cursor positions. A negative y is interpreted as
// distance from the bottom of the page.
func (p *Page) SetXY(x, y float64) {
	q := p.active()
	if y < 0 {
		y = q.h + y
	}
	q.x = x
	q.y = y
}

// GetX returns the current X cursor position.
func (p *Page) GetX() float64 { return p.active().x }

// GetY returns the current Y cursor position.
func (p *Page) GetY() float64 { return p.active().y }

// --- Text decoration ---

// drawTextDecoration draws a thin filled rectangle for underline or
// strikethrough. xUser and yUser give the reference point in user units;
// yUser is adjusted by the font's underline position (Up) so the caller
// only needs to pass the logical origin (baseline for underline, middle
// for strikethrough).
func (p *Page) drawTextDecoration(xUser, yUser float64, text string) {
	fe := p.effectiveFontEntry()
	if fe == nil {
		return
	}
	fontSize := p.effectiveFontSizePt()
	k := p.doc.k
	fontSizeUser := fontSize / k
	sw := p.GetStringWidth(text)

	up := float64(fe.Up) // typically negative (e.g. -100)
	ut := float64(fe.Ut) // typically positive (e.g. 50)

	x1Pt := xUser * k
	y1Pt := (p.h - (yUser - up/1000.0*fontSizeUser)) * k
	wPt := sw * k
	hPt := -ut / 1000.0 * fontSize // negative = extends downward in PDF

	p.stream.Rect(x1Pt, y1Pt, wPt, hPt)
	p.stream.Fill()
}

// --- Links ---

// LinkURL creates a URL hyperlink annotation covering the rectangle
// (x, y, w, h) in user units. Clicking this region in a PDF viewer
// opens the given URL.
func (p *Page) LinkURL(x, y, w, h float64, url string) {
	p = p.active()
	p.links = append(p.links, linkAnnotation{x: x, y: y, w: w, h: h, url: url})
}

// LinkAnchor creates an internal link annotation covering the rectangle
// (x, y, w, h) in user units. Clicking this region navigates to the
// named anchor set by AddAnchor.
func (p *Page) LinkAnchor(x, y, w, h float64, anchor string) {
	p = p.active()
	p.links = append(p.links, linkAnnotation{x: x, y: y, w: w, h: h, anchor: anchor})
}

// AddAnchor registers the current cursor position as a named destination.
// Use LinkAnchor on any page to create a clickable link that navigates here.
func (p *Page) AddAnchor(name string) {
	p = p.active()
	p.doc.anchors[name] = anchorDest{page: p, y: p.y}
}

// --- Internal ---

// emitText writes text to the content stream using the appropriate encoding.
// For TTF fonts, text is hex-encoded as Unicode code points (CIDs).
// Thai tone marks that follow above-vowels are raised using the Ts (text-rise) operator.
func (p *Page) emitText(fe *resources.FontEntry, text string) {
	if fe.Type != "TTF" {
		p.stream.ShowText(pdfEscape(text))
		return
	}

	fe.AddUsedRunes(text)

	runes := []rune(text)
	raised := thaiNeedsRaise(runes)

	if len(raised) == 0 {
		// No Thai tone marks need raising — emit as single hex string
		p.stream.ShowTextHex(textToHex(text))
		return
	}

	// Split text into segments: normal runs and raised tone marks.
	// Raised marks use the Ts (text rise) operator to shift upward.
	rise := 0.23 * p.effectiveFontSizePt()
	i := 0
	inRise := false
	for i < len(runes) {
		if raised[i] {
			if !inRise {
				p.stream.SetTextRise(rise)
				inRise = true
			}
			p.stream.ShowTextHex(runeToHex(runes[i]))
			i++
		} else {
			if inRise {
				p.stream.SetTextRise(0)
				inRise = false
			}
			// Collect consecutive non-raised runes
			j := i
			for j < len(runes) && !raised[j] {
				j++
			}
			p.stream.ShowTextHex(textToHex(string(runes[i:j])))
			i = j
		}
	}
	if inRise {
		p.stream.SetTextRise(0)
	}
}

// textToHex converts a UTF-8 string to hex-encoded Unicode code points.
// Each rune becomes a 4-digit hex value (2-byte big-endian).
func textToHex(s string) string {
	var buf strings.Builder
	buf.Grow(len(s) * 4)
	for _, r := range s {
		fmt.Fprintf(&buf, "%04X", r)
	}
	return buf.String()
}

// runeToHex converts a single rune to a 4-digit hex string.
func runeToHex(r rune) string {
	return fmt.Sprintf("%04X", r)
}

// Thai combining mark constants.
var thaiAboveVowels = map[rune]bool{
	0x0E31: true, // ั mai han akat
	0x0E34: true, // ิ sara i
	0x0E35: true, // ี sara ii
	0x0E36: true, // ึ sara ue
	0x0E37: true, // ื sara uee
	0x0E47: true, // ็ mai taikhu
	0x0E4D: true, // ํ nikhahit
}

var thaiToneMarks = map[rune]bool{
	0x0E48: true, // ่ mai ek
	0x0E49: true, // ้ mai tho
	0x0E4A: true, // ๊ mai tri
	0x0E4B: true, // ๋ mai chattawa
	0x0E4C: true, // ์ thanthakat
}

// thaiNeedsRaise returns the indices of tone marks that need vertical raising
// because they follow above-vowels.
func thaiNeedsRaise(runes []rune) map[int]bool {
	raised := make(map[int]bool)
	for i, r := range runes {
		if thaiToneMarks[r] && i > 0 && thaiAboveVowels[runes[i-1]] {
			raised[i] = true
		}
	}
	return raised
}

// applyFont emits the font change to the content stream.
func (p *Page) applyFont(fe *resources.FontEntry, sizePt float64) {
	p.stream.BeginText()
	p.stream.SetFont("F"+fe.Index, sizePt)
	p.stream.EndText()
}

// effectiveFontEntry returns the page-level font, falling back to document.
func (p *Page) effectiveFontEntry() *resources.FontEntry {
	if p.fontEntry != nil {
		return p.fontEntry
	}
	return p.doc.fontEntry
}

// effectiveFontSizePt returns the page-level font size, falling back to document.
func (p *Page) effectiveFontSizePt() float64 {
	if p.fontSizePt > 0 {
		return p.fontSizePt
	}
	return p.doc.fontSizePt
}
