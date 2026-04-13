package folio

import "strings"

// CellStyle configures the visual appearance of table cells.
// Unset color fields default to black (0,0,0).
type CellStyle struct {
	FontFamily string  // font family (e.g. "helvetica")
	FontStyle  string  // font style ("", "B", "I", "BI")
	FontSize   float64 // font size in points
	TextColor  [3]int  // text RGB (0-255)
	FillColor  [3]int  // background fill RGB (0-255)
	DrawColor  [3]int  // border/stroke RGB (0-255)
	Fill       bool    // whether to fill the cell background
}

// TableCell represents a cell in a complex table row.
// Use with AddRow/AddHeader for colspan, rowspan, multi-line, and per-cell styling.
type TableCell struct {
	Text    string     // cell text content (supports \n for explicit line breaks)
	ColSpan int        // columns to span (default 1)
	RowSpan int        // rows to span (default 1)
	Align   string     // "L", "C", "R" — overrides column default
	Style   *CellStyle // per-cell style override (nil = use default)
}

// Table is a high-level helper for drawing tabular data.
// It manages column layout, header/body styling, alternating row colors,
// and automatic header repetition on page breaks.
//
// Two APIs are available:
//
// Simple (immediate drawing):
//
//	tbl.Header("#", "Name", "Amount")
//	tbl.Row("1", "Item", "100")
//
// Complex (buffered, supports colspan/rowspan/multi-line):
//
//	tbl.AddHeader(folio.TableCell{Text: "Report", ColSpan: 3, Align: "C"})
//	tbl.AddRow(folio.TableCell{Text: "1", RowSpan: 2}, folio.TableCell{Text: "Item"}, folio.TableCell{Text: "100"})
//	tbl.AddRow(folio.TableCell{Text: "Sub-item"}, folio.TableCell{Text: "50"})
//	tbl.Render()
type Table struct {
	doc  *Document
	page *Page
	x    float64 // left edge of the table

	widths []float64 // column widths in user units
	aligns []string  // per-column alignment ("L", "C", "R")
	rowH   float64   // row height in user units (also line height for multi-line)
	border string    // border style ("1", "LR", "", etc.)

	// Header
	header         []string
	repeatHeader   bool
	headerStyle    CellStyle
	hasHeaderStyle bool

	// Body
	bodyStyle    CellStyle
	hasBodyStyle bool

	// Alternating row fills: [even, odd]
	altFills [2][3]int
	hasAlt   bool

	rowIndex int

	// Complex table support (buffered rendering)
	buffered    []bufferedRow
	headerCount int     // number of header rows in buffered
	cellPadding float64 // inner cell padding in user units
	lineH       float64 // line height for multi-line text (0 = use rowH)
}

// bufferedRow stores a row for deferred rendering.
type bufferedRow struct {
	cells    []TableCell
	isHeader bool
}

// cellPlacement tracks which TableCell occupies each grid position.
type cellPlacement struct {
	cell    TableCell
	originR int // row index of the cell's origin
	originC int // column index of the cell's origin
}

// NewTable creates a table helper bound to the given document and page.
// The table's left edge is set to the page's current X position.
// Defaults: row height 8, border "1", header repetition on.
func NewTable(doc *Document, page *Page) *Table {
	return &Table{
		doc:          doc,
		page:         page,
		x:            page.GetX(),
		rowH:         8,
		border:       "1",
		repeatHeader: true,
	}
}

// SetWidths sets column widths in user units.
func (t *Table) SetWidths(widths ...float64) { t.widths = widths }

// SetAligns sets per-column text alignment ("L", "C", or "R").
// Columns without an alignment default to "L".
func (t *Table) SetAligns(aligns ...string) { t.aligns = aligns }

// SetRowHeight sets the height of each row in user units.
func (t *Table) SetRowHeight(h float64) { t.rowH = h }

// SetBorder sets the border style for all cells (e.g. "1", "LR", "").
func (t *Table) SetBorder(border string) { t.border = border }

// SetHeaderStyle sets the visual style for the header row.
func (t *Table) SetHeaderStyle(s CellStyle) {
	t.headerStyle = s
	t.hasHeaderStyle = true
}

// SetBodyStyle sets the visual style for body rows.
func (t *Table) SetBodyStyle(s CellStyle) {
	t.bodyStyle = s
	t.hasBodyStyle = true
}

// SetAlternateRows enables alternating row fill colors.
// even is used for rows 0, 2, 4, … and odd for rows 1, 3, 5, …
func (t *Table) SetAlternateRows(even, odd [3]int) {
	t.altFills = [2][3]int{even, odd}
	t.hasAlt = true
}

// SetRepeatHeader controls whether the header is redrawn after page breaks.
// Default is true.
func (t *Table) SetRepeatHeader(repeat bool) { t.repeatHeader = repeat }

// SetCellPadding sets inner cell padding in user units for complex tables.
// Applies to cells rendered via AddRow/AddHeader + Render.
func (t *Table) SetCellPadding(padding float64) { t.cellPadding = padding }

// SetLineHeight sets the line height for multi-line text within cells.
// If not set, defaults to the row height.
func (t *Table) SetLineHeight(h float64) { t.lineH = h }

// ---- Simple API (immediate drawing) ----

// Header draws the header row and stores the values for repetition on
// subsequent pages.
func (t *Table) Header(values ...string) {
	t.header = values
	t.drawHeader()
}

// Row draws a body row. When auto page break is enabled and the row would
// overflow, a new page is created and the header is repeated (if enabled).
func (t *Table) Row(values ...string) {
	p := t.page.active()
	d := t.doc

	// Table-level page break: detect overflow and handle header repetition
	// before drawing the row.
	if d.autoPageBreak && !d.inHeader && !d.inFooter {
		if p.y+t.rowH > p.h-d.bMargin && p.y > d.tMargin {
			np := d.AddPage(p.size)
			p.next = np
			if t.repeatHeader && t.header != nil {
				t.drawHeader()
			}
		}
	}

	// Apply body style.
	if t.hasBodyStyle {
		t.applyStyle(t.bodyStyle)
	}

	// Alternating row fill.
	fill := false
	if t.hasAlt {
		fc := t.altFills[t.rowIndex%2]
		d.SetFillColor(fc[0], fc[1], fc[2])
		fill = true
	} else if t.hasBodyStyle && t.bodyStyle.Fill {
		fill = true
	}

	t.drawCells(values, fill)
	t.rowIndex++
}

// drawHeader draws the header row with its styling, then re-applies
// body styling for subsequent rows.
func (t *Table) drawHeader() {
	if t.hasHeaderStyle {
		t.applyStyle(t.headerStyle)
	}
	fill := t.hasHeaderStyle && t.headerStyle.Fill
	t.drawCells(t.header, fill)
	if t.hasBodyStyle {
		t.applyStyle(t.bodyStyle)
	}
}

// drawCells draws a single row of cells using the current document state.
func (t *Table) drawCells(values []string, fill bool) {
	p := t.page.active()
	y := p.GetY()
	p.SetX(t.x)

	for i := range t.widths {
		val := ""
		if i < len(values) {
			val = values[i]
		}
		align := "L"
		if i < len(t.aligns) {
			align = t.aligns[i]
		}
		p.Cell(t.widths[i], t.rowH, val, t.border, align, fill, 0)
	}

	p = t.page.active()
	p.SetXY(t.x, y+t.rowH)
}

// ---- Complex API (buffered rendering) ----

// AddHeader buffers a header row for complex table rendering.
// Call Render() after all rows are added.
func (t *Table) AddHeader(cells ...TableCell) {
	t.buffered = append(t.buffered, bufferedRow{cells: cells, isHeader: true})
	t.headerCount++
}

// AddRow buffers a body row for complex table rendering.
// Call Render() after all rows are added.
func (t *Table) AddRow(cells ...TableCell) {
	t.buffered = append(t.buffered, bufferedRow{cells: cells, isHeader: false})
}

// Render draws all buffered rows with full support for colspan, rowspan,
// multi-line text wrapping, and per-cell styling.
func (t *Table) Render() {
	if len(t.buffered) == 0 || len(t.widths) == 0 {
		return
	}

	numCols := len(t.widths)
	numRows := len(t.buffered)

	// Phase 1: Build cell placement grid.
	grid := make([][]*cellPlacement, numRows)
	for r := range grid {
		grid[r] = make([]*cellPlacement, numCols)
	}

	for r, row := range t.buffered {
		colIdx := 0
		for _, cell := range row.cells {
			// Skip columns occupied by a previous rowspan.
			for colIdx < numCols && grid[r][colIdx] != nil {
				colIdx++
			}
			if colIdx >= numCols {
				break
			}

			cs := cell.ColSpan
			if cs < 1 {
				cs = 1
			}
			rs := cell.RowSpan
			if rs < 1 {
				rs = 1
			}

			p := &cellPlacement{cell: cell, originR: r, originC: colIdx}

			// Mark all covered grid positions.
			for dr := 0; dr < rs && r+dr < numRows; dr++ {
				for dc := 0; dc < cs && colIdx+dc < numCols; dc++ {
					grid[r+dr][colIdx+dc] = p
				}
			}

			colIdx += cs
		}
	}

	// Phase 2: Calculate row heights.
	lineH := t.lineH
	if lineH <= 0 {
		lineH = t.rowH
	}

	rowHeights := make([]float64, numRows)
	for r := range rowHeights {
		rowHeights[r] = t.rowH // minimum height
	}

	// First pass: non-rowspan cells determine row heights.
	for r := 0; r < numRows; r++ {
		for c := 0; c < numCols; c++ {
			p := grid[r][c]
			if p == nil || p.originR != r || p.originC != c {
				continue
			}

			rs := p.cell.RowSpan
			if rs < 1 {
				rs = 1
			}
			if rs > 1 {
				continue // handled in second pass
			}

			cellW := t.cellSpanWidth(c, p.cell.ColSpan)
			textW := cellW - 2*t.cellPadding
			lines := t.wrapText(p.cell.Text, textW)
			needed := float64(len(lines))*lineH + 2*t.cellPadding
			if needed > rowHeights[r] {
				rowHeights[r] = needed
			}
		}
	}

	// Second pass: rowspan cells — add extra height to the last spanned row if needed.
	for r := 0; r < numRows; r++ {
		for c := 0; c < numCols; c++ {
			p := grid[r][c]
			if p == nil || p.originR != r || p.originC != c {
				continue
			}

			rs := p.cell.RowSpan
			if rs < 1 {
				rs = 1
			}
			if rs == 1 {
				continue
			}

			cellW := t.cellSpanWidth(c, p.cell.ColSpan)
			textW := cellW - 2*t.cellPadding
			lines := t.wrapText(p.cell.Text, textW)
			needed := float64(len(lines))*lineH + 2*t.cellPadding

			totalH := 0.0
			lastRow := r + rs - 1
			if lastRow >= numRows {
				lastRow = numRows - 1
			}
			for dr := r; dr <= lastRow; dr++ {
				totalH += rowHeights[dr]
			}
			if needed > totalH {
				rowHeights[lastRow] += needed - totalH
			}
		}
	}

	// Phase 3: Draw all rows.
	d := t.doc
	pg := t.page.active()
	y := pg.GetY()

	for r := 0; r < numRows; r++ {
		// Page break check (body rows only).
		if !t.buffered[r].isHeader && d.autoPageBreak && !d.inHeader && !d.inFooter {
			pg = t.page.active()
			if y+rowHeights[r] > pg.h-d.bMargin && y > d.tMargin {
				np := d.AddPage(pg.size)
				pg.next = np
				pg = t.page.active()
				y = d.tMargin

				// Repeat header rows.
				if t.repeatHeader && t.headerCount > 0 {
					for hr := 0; hr < t.headerCount && hr < numRows; hr++ {
						x := t.x
						for c := 0; c < numCols; c++ {
							pl := grid[hr][c]
							if pl != nil && pl.originR == hr && pl.originC == c {
								cw := t.cellSpanWidth(c, pl.cell.ColSpan)
								ch := t.rowSpanHeight(rowHeights, hr, pl.cell.RowSpan)
								t.drawComplexCell(x, y, cw, ch, pl.cell, true, c)
							}
							x += t.widths[c]
						}
						y += rowHeights[hr]
					}
				}
			}
		}

		// Draw cells whose origin is this row.
		x := t.x
		for c := 0; c < numCols; c++ {
			pl := grid[r][c]
			if pl != nil && pl.originR == r && pl.originC == c {
				cw := t.cellSpanWidth(c, pl.cell.ColSpan)
				ch := t.rowSpanHeight(rowHeights, r, pl.cell.RowSpan)
				t.drawComplexCell(x, y, cw, ch, pl.cell, t.buffered[r].isHeader, c)
			}
			x += t.widths[c]
		}
		y += rowHeights[r]
	}

	// Update cursor.
	pg = t.page.active()
	pg.SetXY(t.x, y)
}

// cellSpanWidth returns the total width for a cell spanning multiple columns.
func (t *Table) cellSpanWidth(startCol, colspan int) float64 {
	if colspan < 1 {
		colspan = 1
	}
	w := 0.0
	for i := 0; i < colspan && startCol+i < len(t.widths); i++ {
		w += t.widths[startCol+i]
	}
	return w
}

// rowSpanHeight returns the total height for a cell spanning multiple rows.
func (t *Table) rowSpanHeight(rowHeights []float64, startRow, rowspan int) float64 {
	if rowspan < 1 {
		rowspan = 1
	}
	h := 0.0
	for i := 0; i < rowspan && startRow+i < len(rowHeights); i++ {
		h += rowHeights[startRow+i]
	}
	return h
}

// drawComplexCell draws a single cell at absolute coordinates with full
// support for multi-line text, alignment, borders, and per-cell styling.
func (t *Table) drawComplexCell(x, y, w, h float64, cell TableCell, isHeader bool, colIdx int) {
	d := t.doc
	pg := t.page.active()

	// Determine effective style.
	var style *CellStyle
	if cell.Style != nil {
		style = cell.Style
	} else if isHeader && t.hasHeaderStyle {
		style = &t.headerStyle
	} else if !isHeader && t.hasBodyStyle {
		style = &t.bodyStyle
	}

	// Save document state.
	savedFont := d.GetFontFamily()
	savedStyle := d.GetFontStyle()
	savedSize := d.GetFontSize()

	// Apply style.
	if style != nil {
		if style.FontFamily != "" {
			d.SetFont(style.FontFamily, style.FontStyle, style.FontSize)
		}
		d.SetTextColor(style.TextColor[0], style.TextColor[1], style.TextColor[2])
		d.SetDrawColor(style.DrawColor[0], style.DrawColor[1], style.DrawColor[2])
	}

	// Alternating row fill for body cells without explicit style.
	fill := false
	if style != nil && style.Fill {
		d.SetFillColor(style.FillColor[0], style.FillColor[1], style.FillColor[2])
		fill = true
	} else if !isHeader && t.hasAlt {
		// Use body row index for alternating colors.
		// (Not perfect with rowspan, but reasonable.)
	}

	// Draw fill.
	if fill {
		pg.Rect(x, y, w, h, "F")
	}

	// Draw border.
	if t.border == "1" {
		pg.Rect(x, y, w, h, "D")
	} else if t.border != "" {
		if strings.Contains(t.border, "L") {
			pg.Line(x, y, x, y+h)
		}
		if strings.Contains(t.border, "T") {
			pg.Line(x, y, x+w, y)
		}
		if strings.Contains(t.border, "R") {
			pg.Line(x+w, y, x+w, y+h)
		}
		if strings.Contains(t.border, "B") {
			pg.Line(x, y+h, x+w, y+h)
		}
	}

	// Draw text.
	if cell.Text != "" {
		lineH := t.lineH
		if lineH <= 0 {
			lineH = t.rowH
		}
		padding := t.cellPadding
		if padding <= 0 {
			// Use document's cell margin as default.
			padding = d.cMargin
		}

		textW := w - 2*padding
		lines := t.wrapText(cell.Text, textW)

		// Vertical centering of text block.
		textBlockH := float64(len(lines)) * lineH
		startY := y + (h-textBlockH)/2

		// Determine alignment.
		align := cell.Align
		if align == "" && colIdx < len(t.aligns) {
			align = t.aligns[colIdx]
		}
		if align == "" {
			align = "L"
		}

		for i, line := range lines {
			lineY := startY + float64(i)*lineH
			// Use Cell for each line — position cursor and draw.
			pg = t.page.active()
			pg.SetXY(x, lineY)

			var dx float64
			sw := pg.GetStringWidth(line)
			switch strings.ToUpper(align) {
			case "C":
				dx = (w - sw) / 2
			case "R":
				dx = w - padding - sw
			default:
				dx = padding
			}
			pg.SetX(x + dx)
			// Draw text only (no border/fill — already drawn above).
			pg.Cell(w, lineH, line, "", "", false, 0)
			// Reset X so next line draws correctly.
		}
	}

	// Restore font state.
	if style != nil && style.FontFamily != "" && savedFont != "" {
		d.SetFont(savedFont, savedStyle, savedSize)
	}
}

// wrapText splits text into lines that fit within maxWidth user units.
// Handles explicit \n newlines and word-wrapping.
func (t *Table) wrapText(text string, maxWidth float64) []string {
	if text == "" {
		return []string{""}
	}

	pg := t.page.active()
	paragraphs := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var lines []string

	for _, para := range paragraphs {
		if para == "" {
			lines = append(lines, "")
			continue
		}

		if maxWidth <= 0 {
			lines = append(lines, para)
			continue
		}

		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}

		currentLine := words[0]
		for _, word := range words[1:] {
			test := currentLine + " " + word
			if pg.GetStringWidth(test) > maxWidth {
				lines = append(lines, currentLine)
				currentLine = word
			} else {
				currentLine = test
			}
		}
		lines = append(lines, currentLine)
	}

	return lines
}

// applyStyle applies a CellStyle to the document.
func (t *Table) applyStyle(s CellStyle) {
	if s.FontFamily != "" {
		t.doc.SetFont(s.FontFamily, s.FontStyle, s.FontSize)
	}
	t.doc.SetTextColor(s.TextColor[0], s.TextColor[1], s.TextColor[2])
	t.doc.SetDrawColor(s.DrawColor[0], s.DrawColor[1], s.DrawColor[2])
	if s.Fill {
		t.doc.SetFillColor(s.FillColor[0], s.FillColor[1], s.FillColor[2])
	}
}
