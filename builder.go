package foliopdf

// TextBuilder provides a fluent API for drawing text on a page.
// Create one with Page.Text(s), configure with chained calls, and
// finalize with Draw().
type TextBuilder struct {
	page       *Page
	text       string
	x, y       float64
	hasPos     bool
	fontFamily string
	fontStyle  string
	fontSize   float64
	r, g, b    int
	hasColor   bool
}

// Text starts a fluent text builder for the given string.
func (p *Page) Text(s string) *TextBuilder {
	return &TextBuilder{
		page: p.active(),
		text: s,
	}
}

// At sets the position where the text will be drawn.
func (tb *TextBuilder) At(x, y float64) *TextBuilder {
	tb.x = x
	tb.y = y
	tb.hasPos = true
	return tb
}

// Font sets the font family and size. Use Bold/Italic separately for style.
func (tb *TextBuilder) Font(family string, size float64) *TextBuilder {
	tb.fontFamily = family
	tb.fontSize = size
	return tb
}

// Size sets the font size in points.
func (tb *TextBuilder) Size(pt float64) *TextBuilder {
	tb.fontSize = pt
	return tb
}

// Bold sets the font style to bold.
func (tb *TextBuilder) Bold() *TextBuilder {
	if tb.fontStyle == "I" || tb.fontStyle == "BI" {
		tb.fontStyle = "BI"
	} else {
		tb.fontStyle = "B"
	}
	return tb
}

// Italic sets the font style to italic.
func (tb *TextBuilder) Italic() *TextBuilder {
	if tb.fontStyle == "B" || tb.fontStyle == "BI" {
		tb.fontStyle = "BI"
	} else {
		tb.fontStyle = "I"
	}
	return tb
}

// Color sets the text color using 0-255 RGB values.
func (tb *TextBuilder) Color(r, g, b int) *TextBuilder {
	tb.r = r
	tb.g = g
	tb.b = b
	tb.hasColor = true
	return tb
}

// Draw finalizes the text builder and renders the text on the page.
// Font and color changes are applied before drawing and restored after.
func (tb *TextBuilder) Draw() {
	p := tb.page
	doc := p.doc

	// Save state.
	savedFamily := doc.fontFamily
	savedStyle := doc.fontStyle
	savedSize := doc.fontSizePt
	savedColor := doc.textColor

	// Apply font if specified.
	if tb.fontFamily != "" || tb.fontSize > 0 || tb.fontStyle != "" {
		family := tb.fontFamily
		if family == "" {
			family = doc.fontFamily
		}
		style := tb.fontStyle
		if style == "" && tb.fontStyle == "" {
			style = doc.fontStyle
		}
		size := tb.fontSize
		if size <= 0 {
			size = doc.fontSizePt
		}
		doc.SetFont(family, style, size)
	}

	// Apply color if specified.
	if tb.hasColor {
		doc.SetTextColor(tb.r, tb.g, tb.b)
	}

	// Position and draw.
	if tb.hasPos {
		p.TextAt(tb.x, tb.y, tb.text)
	} else {
		p.Write(p.doc.fontSizePt/p.doc.k, tb.text)
	}

	// Restore state.
	if tb.fontFamily != "" || tb.fontSize > 0 || tb.fontStyle != "" {
		doc.SetFont(savedFamily, savedStyle, savedSize)
	}
	if tb.hasColor {
		doc.textColor = savedColor
	}
}

// ShapeBuilder provides a fluent API for drawing shapes on a page.
// Create one with Page.Shape(), configure with a shape method and
// optional style calls, then finalize with Draw().
type ShapeBuilder struct {
	page      *Page
	shapeType string // "rect", "roundedrect", "circle", "ellipse", "line"
	x, y      float64
	w, h      float64
	r         float64  // radius for circle
	rx, ry    float64  // radii for ellipse
	x2, y2    float64  // end point for line
	style     string   // "D", "F", "DF"
	drawR     int      // stroke color
	drawG     int
	drawB     int
	hasDraw   bool
	fillR     int // fill color
	fillG     int
	fillB     int
	hasFill   bool
	lineW     float64
	hasLineW  bool
}

// Shape starts a fluent shape builder.
func (p *Page) Shape() *ShapeBuilder {
	return &ShapeBuilder{
		page:  p.active(),
		style: "D", // default: stroke only
	}
}

// Rect configures a rectangle at (x, y) with size (w, h).
func (sb *ShapeBuilder) Rect(x, y, w, h float64) *ShapeBuilder {
	sb.shapeType = "rect"
	sb.x = x
	sb.y = y
	sb.w = w
	sb.h = h
	return sb
}

// RoundedRect configures a rounded rectangle at (x, y) with size (w, h) and corner radius r.
func (sb *ShapeBuilder) RoundedRect(x, y, w, h, r float64) *ShapeBuilder {
	sb.shapeType = "roundedrect"
	sb.x = x
	sb.y = y
	sb.w = w
	sb.h = h
	sb.r = r
	return sb
}

// Circle configures a circle at (cx, cy) with radius r.
func (sb *ShapeBuilder) Circle(cx, cy, r float64) *ShapeBuilder {
	sb.shapeType = "circle"
	sb.x = cx
	sb.y = cy
	sb.r = r
	return sb
}

// Ellipse configures an ellipse at (cx, cy) with radii (rx, ry).
func (sb *ShapeBuilder) Ellipse(cx, cy, rx, ry float64) *ShapeBuilder {
	sb.shapeType = "ellipse"
	sb.x = cx
	sb.y = cy
	sb.rx = rx
	sb.ry = ry
	return sb
}

// Line configures a line from (x1, y1) to (x2, y2).
func (sb *ShapeBuilder) Line(x1, y1, x2, y2 float64) *ShapeBuilder {
	sb.shapeType = "line"
	sb.x = x1
	sb.y = y1
	sb.x2 = x2
	sb.y2 = y2
	return sb
}

// Stroke sets the draw style to stroke only.
func (sb *ShapeBuilder) Stroke() *ShapeBuilder {
	sb.style = "D"
	return sb
}

// Fill sets the draw style to fill only.
func (sb *ShapeBuilder) Fill() *ShapeBuilder {
	sb.style = "F"
	return sb
}

// FillStroke sets the draw style to both fill and stroke.
func (sb *ShapeBuilder) FillStroke() *ShapeBuilder {
	sb.style = "DF"
	return sb
}

// StrokeColor sets the stroke color (0-255 RGB).
func (sb *ShapeBuilder) StrokeColor(r, g, b int) *ShapeBuilder {
	sb.drawR = r
	sb.drawG = g
	sb.drawB = b
	sb.hasDraw = true
	return sb
}

// FillColor sets the fill color (0-255 RGB).
func (sb *ShapeBuilder) FillColor(r, g, b int) *ShapeBuilder {
	sb.fillR = r
	sb.fillG = g
	sb.fillB = b
	sb.hasFill = true
	return sb
}

// LineWidth sets the line width for the shape.
func (sb *ShapeBuilder) LineWidth(w float64) *ShapeBuilder {
	sb.lineW = w
	sb.hasLineW = true
	return sb
}

// Draw finalizes the shape builder and renders the shape on the page.
// Color and line width changes are applied before drawing and restored after.
func (sb *ShapeBuilder) Draw() {
	p := sb.page
	doc := p.doc

	// Save state.
	savedDraw := doc.drawColor
	savedFill := doc.fillColor
	savedLineW := doc.lineWidth

	// Apply colors and line width.
	if sb.hasDraw {
		doc.SetDrawColor(sb.drawR, sb.drawG, sb.drawB)
	}
	if sb.hasFill {
		doc.SetFillColor(sb.fillR, sb.fillG, sb.fillB)
	}
	if sb.hasLineW {
		doc.SetLineWidth(sb.lineW)
	}

	// Draw the shape.
	switch sb.shapeType {
	case "rect":
		p.Rect(sb.x, sb.y, sb.w, sb.h, sb.style)
	case "roundedrect":
		p.RoundedRect(sb.x, sb.y, sb.w, sb.h, sb.r, sb.style)
	case "circle":
		p.Circle(sb.x, sb.y, sb.r, sb.style)
	case "ellipse":
		p.Ellipse(sb.x, sb.y, sb.rx, sb.ry, sb.style)
	case "line":
		p.Line(sb.x, sb.y, sb.x2, sb.y2)
	}

	// Restore state.
	if sb.hasDraw {
		doc.drawColor = savedDraw
		if doc.currentPage != nil {
			doc.currentPage.stream.SetStrokeColorRGB(savedDraw.R, savedDraw.G, savedDraw.B)
		}
	}
	if sb.hasFill {
		doc.fillColor = savedFill
		if doc.currentPage != nil {
			doc.currentPage.stream.SetFillColorRGB(savedFill.R, savedFill.G, savedFill.B)
		}
	}
	if sb.hasLineW {
		doc.SetLineWidth(savedLineW)
	}
}
