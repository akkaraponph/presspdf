package folio

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

// Table is a high-level helper for drawing tabular data.
// It manages column layout, header/body styling, alternating row colors,
// and automatic header repetition on page breaks.
//
// Usage:
//
//	tbl := folio.NewTable(doc, page)
//	tbl.SetWidths(40, 100, 40)
//	tbl.SetAligns("C", "L", "R")
//	tbl.SetHeaderStyle(folio.CellStyle{...})
//	tbl.Header("#", "Description", "Amount")
//	tbl.Row("1", "Item one", "100.00")
//	tbl.Row("2", "Item two", "200.00")
type Table struct {
	doc  *Document
	page *Page
	x    float64 // left edge of the table

	widths []float64 // column widths in user units
	aligns []string  // per-column alignment ("L", "C", "R")
	rowH   float64   // row height in user units
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
