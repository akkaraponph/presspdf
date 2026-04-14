package presspdf

import (
	"fmt"
	"math"
	"strings"

	"github.com/akkaraponph/presspdf/internal/content"
	"github.com/akkaraponph/presspdf/internal/resources"
	"github.com/akkaraponph/presspdf/internal/state"
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

	// page boxes (optional: TrimBox, CropBox, BleedBox, ArtBox) in PDF points
	pageBoxes map[string][4]float64

	// file attachment annotations on this page
	attachAnnotations []attachAnnotation

	// tagged PDF (structure tree)
	nextMCID       int              // next marked content ID for this page
	structElements []*structElement // elements on this page
	currentTag     *structElement   // current open tag (for nesting)
}

// attachAnnotation represents a file attachment annotation on a page.
type attachAnnotation struct {
	attachment Attachment
	x, y, w, h float64 // rect in user units
}

// linkAnnotation represents a hyperlink annotation on a page.
type linkAnnotation struct {
	x, y, w, h float64 // rect in user units
	url         string  // non-empty for URL links
	anchor      string  // non-empty for internal links
	linkID      int     // >0 for integer-based internal links (set via Link)
}

// linkDest stores the target for an integer-based internal link.
type linkDest struct {
	page *Page   // target page (nil until SetLink is called)
	y    float64 // Y position in user units
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
	// Custom accept-page-break function: if it returns false, suppress break.
	if d.acceptPageBreakFunc != nil && !d.acceptPageBreakFunc() {
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

// RoundedRect draws a rectangle with rounded corners.
// r is the corner radius in user units. If r exceeds half the width or
// height it is clamped automatically.
// style: "D" (stroke), "F" (fill), "DF" or "FD" (fill and stroke).
func (p *Page) RoundedRect(x, y, w, h, r float64, style string) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	p.stream.RoundedRect(
		state.ToPointsX(x, k),
		state.ToPointsY(y+h, p.h, k), // bottom-left corner in PDF coords
		w*k,
		h*k,
		r*k,
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

// RoundedRectExt draws a rectangle with per-corner radii.
// rTL, rTR, rBR, rBL are the radii for top-left, top-right, bottom-right,
// and bottom-left corners respectively. A zero radius produces a square corner.
// style: "D" (stroke), "F" (fill), "DF" or "FD" (fill and stroke).
func (p *Page) RoundedRectExt(x, y, w, h, rTL, rTR, rBR, rBL float64, style string) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	p.stream.RoundedRectExt(
		state.ToPointsX(x, k),
		state.ToPointsY(y+h, p.h, k),
		w*k, h*k,
		rTL*k, rTR*k, rBR*k, rBL*k,
	)
	p.paintStyle(style)
}

// Circle draws a circle centered at (x, y) with radius r in user units.
// style: "D" (stroke), "F" (fill), "DF" or "FD" (fill and stroke).
func (p *Page) Circle(x, y, r float64, style string) {
	p.Ellipse(x, y, r, r, style)
}

// Ellipse draws an ellipse centered at (x, y) with horizontal radius rx
// and vertical radius ry in user units.
// style: "D" (stroke), "F" (fill), "DF" or "FD" (fill and stroke).
func (p *Page) Ellipse(x, y, rx, ry float64, style string) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k

	// Centre in PDF points.
	cx := state.ToPointsX(x, k)
	cy := state.ToPointsY(y, p.h, k)
	rxPt := rx * k
	ryPt := ry * k

	// Approximate a full ellipse with 4 cubic Bézier arcs.
	// κ = 4/3 * (√2 − 1) ≈ 0.5522847498
	const kappa = 0.5522847498

	kx := rxPt * kappa
	ky := ryPt * kappa

	// Start at the rightmost point.
	p.stream.MoveTo(cx+rxPt, cy)
	// Top-right quadrant (0° → 90° in PDF coords, i.e. rightward → upward).
	p.stream.CurveTo(cx+rxPt, cy+ky, cx+kx, cy+ryPt, cx, cy+ryPt)
	// Top-left quadrant.
	p.stream.CurveTo(cx-kx, cy+ryPt, cx-rxPt, cy+ky, cx-rxPt, cy)
	// Bottom-left quadrant.
	p.stream.CurveTo(cx-rxPt, cy-ky, cx-kx, cy-ryPt, cx, cy-ryPt)
	// Bottom-right quadrant.
	p.stream.CurveTo(cx+kx, cy-ryPt, cx+rxPt, cy-ky, cx+rxPt, cy)

	style = strings.ToUpper(style)
	switch style {
	case "F":
		p.stream.Fill()
	case "DF", "FD":
		p.stream.FillStroke()
	default:
		p.stream.Stroke()
	}
}

// EllipseRotated draws a rotated ellipse centered at (x, y) with horizontal
// radius rx, vertical radius ry, and rotation degRotate (degrees, clockwise).
// style: "D" (stroke), "F" (fill), "DF" or "FD" (fill and stroke).
func (p *Page) EllipseRotated(x, y, rx, ry, degRotate float64, style string) {
	if degRotate == 0 {
		p.Ellipse(x, y, rx, ry, style)
		return
	}
	p = p.active()
	if p.doc.err != nil {
		return
	}
	p.stream.SaveState()

	k := p.doc.k
	cx := state.ToPointsX(x, k)
	cy := state.ToPointsY(y, p.h, k)

	// Apply rotation around the centre point.
	rad := -degRotate * math.Pi / 180.0
	cosA := math.Cos(rad)
	sinA := math.Sin(rad)
	e := cx*(1-cosA) + cy*sinA
	f := cy*(1-cosA) - cx*sinA
	p.stream.ConcatMatrix(cosA, sinA, -sinA, cosA, e, f)

	// Draw the unrotated ellipse at the centre.
	rxPt := rx * k
	ryPt := ry * k
	const kappa = 0.5522847498
	kx := rxPt * kappa
	ky := ryPt * kappa

	p.stream.MoveTo(cx+rxPt, cy)
	p.stream.CurveTo(cx+rxPt, cy+ky, cx+kx, cy+ryPt, cx, cy+ryPt)
	p.stream.CurveTo(cx-kx, cy+ryPt, cx-rxPt, cy+ky, cx-rxPt, cy)
	p.stream.CurveTo(cx-rxPt, cy-ky, cx-kx, cy-ryPt, cx, cy-ryPt)
	p.stream.CurveTo(cx+kx, cy-ryPt, cx+rxPt, cy-ky, cx+rxPt, cy)

	p.paintStyle(style)
	p.stream.RestoreState()
}

// Arc draws an elliptical arc centered at (x, y) with horizontal radius rx
// and vertical radius ry, from startDeg to endDeg (both in degrees,
// counterclockwise from 3-o'clock). style: "D" (stroke), "F" (fill),
// "DF"/"FD" (fill and stroke). When filled, the arc is closed as a pie
// slice (lines from endpoints to center).
func (p *Page) Arc(x, y, rx, ry, startDeg, endDeg float64, style string) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	cx := state.ToPointsX(x, k)
	cy := state.ToPointsY(y, p.h, k)
	rxPt := rx * k
	ryPt := ry * k

	// In PDF coordinates, Y increases upward but our user angles assume
	// counterclockwise from 3-o'clock which maps naturally.
	p.arcPath(cx, cy, rxPt, ryPt, startDeg, endDeg)

	style = strings.ToUpper(style)
	needClose := style == "F" || style == "DF" || style == "FD"
	if needClose {
		// Close as pie slice: line to center and back.
		p.stream.LineTo(cx, cy)
		p.stream.ClosePath()
	}

	switch style {
	case "F":
		p.stream.Fill()
	case "DF", "FD":
		p.stream.FillStroke()
	default:
		p.stream.Stroke()
	}
}

// arcPath emits Bézier curves approximating an elliptical arc.
// All coordinates are in PDF points. Angles are in degrees.
func (p *Page) arcPath(cx, cy, rx, ry, startDeg, endDeg float64) {
	// Normalize so startDeg < endDeg
	for endDeg < startDeg {
		endDeg += 360
	}

	// Split into segments of at most 90° each for Bézier accuracy.
	totalAngle := endDeg - startDeg
	nSegs := int(math.Ceil(totalAngle / 90.0))
	if nSegs < 1 {
		nSegs = 1
	}
	segAngle := totalAngle / float64(nSegs) * math.Pi / 180.0

	angle := startDeg * math.Pi / 180.0

	// First point
	startX := cx + rx*math.Cos(angle)
	startY := cy + ry*math.Sin(angle)
	p.stream.MoveTo(startX, startY)

	for i := 0; i < nSegs; i++ {
		a1 := angle
		a2 := angle + segAngle
		p.arcSegment(cx, cy, rx, ry, a1, a2)
		angle = a2
	}
}

// arcSegment emits a single cubic Bézier curve approximating an arc
// from angle a1 to a2 (radians) on an ellipse centered at (cx,cy).
func (p *Page) arcSegment(cx, cy, rx, ry, a1, a2 float64) {
	// Bézier approximation for a circular arc, scaled to ellipse.
	// alpha = 4/3 * tan((a2-a1)/4)
	da := a2 - a1
	alpha := 4.0 / 3.0 * math.Tan(da/4.0)

	cos1 := math.Cos(a1)
	sin1 := math.Sin(a1)
	cos2 := math.Cos(a2)
	sin2 := math.Sin(a2)

	// Control point 1
	cp1x := cx + rx*(cos1-alpha*sin1)
	cp1y := cy + ry*(sin1+alpha*cos1)

	// Control point 2
	cp2x := cx + rx*(cos2+alpha*sin2)
	cp2y := cy + ry*(sin2-alpha*cos2)

	// End point
	endX := cx + rx*cos2
	endY := cy + ry*sin2

	p.stream.CurveTo(cp1x, cp1y, cp2x, cp2y, endX, endY)
}

// --- Path building API ---
//
// These methods construct a path incrementally without painting it.
// Call DrawPath to actually stroke, fill, or both.

// Point represents a 2D point in user-unit coordinates.
type Point struct {
	X, Y float64
}

// Pt is a convenience constructor for Point.
func Pt(x, y float64) Point { return Point{X: x, Y: y} }

// MoveTo begins a new subpath at the given point (user units, top-left origin).
// This sets the current point without drawing anything.
func (p *Page) MoveTo(x, y float64) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	p.stream.MoveTo(state.ToPointsX(x, k), state.ToPointsY(y, p.h, k))
}

// LineTo appends a straight line segment from the current point to (x, y).
func (p *Page) LineTo(x, y float64) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	p.stream.LineTo(state.ToPointsX(x, k), state.ToPointsY(y, p.h, k))
}

// CurveTo appends a cubic Bézier curve from the current point to (x, y)
// using control points (cx0, cy0) and (cx1, cy1).
func (p *Page) CurveTo(cx0, cy0, cx1, cy1, x, y float64) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	p.stream.CurveTo(
		state.ToPointsX(cx0, k), state.ToPointsY(cy0, p.h, k),
		state.ToPointsX(cx1, k), state.ToPointsY(cy1, p.h, k),
		state.ToPointsX(x, k), state.ToPointsY(y, p.h, k),
	)
}

// CurveToQuadratic appends a quadratic Bézier curve from the current point
// to (x, y) using control point (cx, cy). Internally this uses the PDF v
// operator (cubic Bézier with the first control point at the current point).
func (p *Page) CurveToQuadratic(cx, cy, x, y float64) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	p.stream.CurveToV(
		state.ToPointsX(cx, k), state.ToPointsY(cy, p.h, k),
		state.ToPointsX(x, k), state.ToPointsY(y, p.h, k),
	)
}

// ClosePath closes the current subpath by drawing a straight line back
// to the starting point (the most recent MoveTo).
func (p *Page) ClosePath() {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	p.stream.ClosePath()
}

// DrawPath paints the current path using the given style string.
//
// High-level styles: "D" (stroke), "F" (fill), "DF"/"FD" (fill and stroke).
//
// PDF path-painting operators are also accepted directly:
// "S" stroke, "s" close and stroke, "f" fill (nonzero winding),
// "f*" fill (even-odd), "B" fill and stroke (nonzero), "B*" fill and
// stroke (even-odd), "b" close+fill+stroke (nonzero),
// "b*" close+fill+stroke (even-odd).
func (p *Page) DrawPath(style string) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	switch strings.ToUpper(style) {
	case "F":
		p.stream.Fill()
	case "F*":
		p.stream.FillEvenOdd()
	case "DF", "FD":
		p.stream.FillStroke()
	case "DF*", "FD*":
		p.stream.FillStrokeEvenOdd()
	default:
		// Check lowercase operators that shouldn't be uppercased.
		switch style {
		case "s":
			p.stream.CloseAndStroke()
		case "b":
			p.stream.CloseFillStroke()
		case "b*":
			p.stream.CloseFillStrokeEvenOdd()
		default:
			// "D", "S", "", or anything else → stroke.
			p.stream.Stroke()
		}
	}
}

// --- Standalone curve methods ---

// Curve draws a quadratic Bézier curve from (x0, y0) to (x1, y1) with
// control point (cx, cy). style: "D" (stroke), "F" (fill), "DF"/"FD" (both).
func (p *Page) Curve(x0, y0, cx, cy, x1, y1 float64, style string) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	p.stream.MoveTo(state.ToPointsX(x0, k), state.ToPointsY(y0, p.h, k))
	p.stream.CurveToV(
		state.ToPointsX(cx, k), state.ToPointsY(cy, p.h, k),
		state.ToPointsX(x1, k), state.ToPointsY(y1, p.h, k),
	)
	p.paintStyle(style)
}

// CurveCubic draws a cubic Bézier curve from (x0, y0) to (x1, y1)
// with control points (cx0, cy0) and (cx1, cy1).
// style: "D" (stroke), "F" (fill), "DF"/"FD" (both).
func (p *Page) CurveCubic(x0, y0, cx0, cy0, cx1, cy1, x1, y1 float64, style string) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	p.stream.MoveTo(state.ToPointsX(x0, k), state.ToPointsY(y0, p.h, k))
	p.stream.CurveTo(
		state.ToPointsX(cx0, k), state.ToPointsY(cy0, p.h, k),
		state.ToPointsX(cx1, k), state.ToPointsY(cy1, p.h, k),
		state.ToPointsX(x1, k), state.ToPointsY(y1, p.h, k),
	)
	p.paintStyle(style)
}

// --- Polygon and Beziergon ---

// Polygon draws a closed polygon through the given points.
// At least 3 points are required; fewer is a no-op.
// style: "D" (stroke), "F" (fill), "DF"/"FD" (both).
func (p *Page) Polygon(points []Point, style string) {
	p = p.active()
	if p.doc.err != nil || len(points) < 3 {
		return
	}
	k := p.doc.k
	p.stream.MoveTo(
		state.ToPointsX(points[0].X, k),
		state.ToPointsY(points[0].Y, p.h, k),
	)
	for _, pt := range points[1:] {
		p.stream.LineTo(
			state.ToPointsX(pt.X, k),
			state.ToPointsY(pt.Y, p.h, k),
		)
	}
	// Close back to the first point.
	p.stream.LineTo(
		state.ToPointsX(points[0].X, k),
		state.ToPointsY(points[0].Y, p.h, k),
	)
	p.paintStyle(style)
}

// Beziergon draws a closed figure defined by cubic Bézier curve segments.
// The first point is the starting point. Each subsequent group of three
// points (control1, control2, endpoint) defines one curve segment.
// Therefore len(points) must be 1 + 3*n where n >= 1.
// style: "D" (stroke), "F" (fill), "DF"/"FD" (both).
func (p *Page) Beziergon(points []Point, style string) {
	p = p.active()
	if p.doc.err != nil || len(points) < 4 {
		return
	}
	k := p.doc.k
	p.stream.MoveTo(
		state.ToPointsX(points[0].X, k),
		state.ToPointsY(points[0].Y, p.h, k),
	)
	pts := points[1:]
	for len(pts) >= 3 {
		p.stream.CurveTo(
			state.ToPointsX(pts[0].X, k), state.ToPointsY(pts[0].Y, p.h, k),
			state.ToPointsX(pts[1].X, k), state.ToPointsY(pts[1].Y, p.h, k),
			state.ToPointsX(pts[2].X, k), state.ToPointsY(pts[2].Y, p.h, k),
		)
		pts = pts[3:]
	}
	p.paintStyle(style)
}

// --- ArcTo (path-based arc) ---

// ArcTo appends an elliptical arc to the current path. The arc is centered
// at (x, y) with horizontal radius rx and vertical radius ry. degRotate
// rotates the ellipse. degStart and degEnd define the arc sweep in degrees,
// measured counter-clockwise from the 3 o'clock position. If the arc's
// start point differs from the current path point, a connecting line is drawn.
//
// Unlike Arc, this method does NOT paint the path — call DrawPath afterwards.
func (p *Page) ArcTo(x, y, rx, ry, degRotate, degStart, degEnd float64) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	cx := state.ToPointsX(x, k)
	cy := state.ToPointsY(y, p.h, k)
	rxPt := rx * k
	ryPt := ry * k

	// Normalize so degStart < degEnd.
	for degEnd < degStart {
		degEnd += 360
	}
	totalAngle := degEnd - degStart
	nSegs := int(math.Ceil(totalAngle / 60.0))
	if nSegs < 2 {
		nSegs = 2
	}
	segAngle := totalAngle / float64(nSegs) * math.Pi / 180.0

	if degRotate != 0 {
		a := -degRotate * math.Pi / 180.0
		p.stream.SaveState()
		p.stream.ConcatMatrix(
			math.Cos(a), -math.Sin(a),
			math.Sin(a), math.Cos(a),
			cx, cy,
		)
		cx = 0
		cy = 0
	}

	angle := degStart * math.Pi / 180.0
	dtm := segAngle / 3.0

	// Compute the first point on the arc.
	a0 := cx + rxPt*math.Cos(angle)
	b0 := cy + ryPt*math.Sin(angle)
	c0 := -rxPt * math.Sin(angle)
	d0 := ryPt * math.Cos(angle)

	// Draw a line to the start of the arc (connects to existing path).
	p.stream.LineTo(a0, b0)

	for j := 1; j <= nSegs; j++ {
		t := (float64(j)*segAngle + degStart*math.Pi/180.0)
		a1 := cx + rxPt*math.Cos(t)
		b1 := cy + ryPt*math.Sin(t)
		c1 := -rxPt * math.Sin(t)
		d1 := ryPt * math.Cos(t)
		p.stream.CurveTo(
			a0+(c0*dtm), b0+(d0*dtm),
			a1-(c1*dtm), b1-(d1*dtm),
			a1, b1,
		)
		a0, b0, c0, d0 = a1, b1, c1, d1
	}

	if degRotate != 0 {
		p.stream.RestoreState()
	}
}

// paintStyle is a helper that emits the appropriate path-painting operator.
func (p *Page) paintStyle(style string) {
	style = strings.ToUpper(style)
	switch style {
	case "F":
		p.stream.Fill()
	case "DF", "FD":
		p.stream.FillStroke()
	default:
		p.stream.Stroke()
	}
}

// SetDashPattern sets the line dash pattern for subsequent strokes.
// dashArray contains alternating dash and gap lengths in user units.
// phase is the offset into the pattern at which the stroke begins.
// Call with an empty dashArray (or nil) and phase 0 to restore solid lines.
func (p *Page) SetDashPattern(dashArray []float64, phase float64) {
	p = p.active()
	k := p.doc.k
	pts := make([]float64, len(dashArray))
	for i, v := range dashArray {
		pts[i] = v * k
	}
	p.stream.SetDash(pts, phase*k)
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

// DrawImage draws a registered image with flow support.
// If flow is true, the image is positioned at the current cursor Y (ignoring
// the y parameter), a page break is triggered if needed, and the cursor
// advances past the image. If flow is false, behavior is identical to
// DrawImageRect.
func (p *Page) DrawImage(name string, x, y, w, h float64, flow bool) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	if flow {
		p = p.checkPageBreak(h)
		y = p.y
		p.DrawImageRect(name, x, y, w, h)
		p.y += h
	} else {
		p.DrawImageRect(name, x, y, w, h)
	}
}

// SetHomeXY resets the cursor to the top-left position (left margin, top margin).
func (p *Page) SetHomeXY() {
	p = p.active()
	p.x = p.doc.lMargin
	p.y = p.doc.tMargin
}

// RawWriteStr writes a raw PDF content stream string to the page.
// The string is written as-is with no coordinate conversion or escaping.
// Use this only when you know exactly what PDF operators to emit.
func (p *Page) RawWriteStr(s string) {
	p = p.active()
	p.stream.Raw(s)
}

// RawWriteBuf writes raw PDF content stream bytes to the page.
func (p *Page) RawWriteBuf(data []byte) {
	p = p.active()
	p.stream.Raw(string(data))
}

// --- Transforms ---

// TransformBegin saves the graphics state. Must be paired with
// TransformEnd after the transformed drawing operations.
func (p *Page) TransformBegin() {
	p = p.active()
	p.stream.SaveState()
}

// TransformEnd restores the graphics state saved by TransformBegin.
func (p *Page) TransformEnd() {
	p = p.active()
	p.stream.RestoreState()
}

// Rotate applies a counterclockwise rotation of angleDeg degrees around
// the point (x, y) in user units. Must be called between TransformBegin
// and TransformEnd.
func (p *Page) Rotate(angleDeg, x, y float64) {
	p = p.active()
	k := p.doc.k
	cx := state.ToPointsX(x, k)
	cy := state.ToPointsY(y, p.h, k)

	rad := angleDeg * math.Pi / 180.0
	cosA := math.Cos(rad)
	sinA := math.Sin(rad)

	a := cosA
	b := sinA
	c := -sinA
	d := cosA
	e := cx*(1-cosA) + cy*sinA
	f := cy*(1-cosA) - cx*sinA

	p.stream.ConcatMatrix(a, b, c, d, e, f)
}

// Scale applies a scaling transform with factors sx and sy around the
// point (x, y) in user units. Must be called between TransformBegin
// and TransformEnd.
func (p *Page) Scale(sx, sy, x, y float64) {
	p = p.active()
	k := p.doc.k
	cx := state.ToPointsX(x, k)
	cy := state.ToPointsY(y, p.h, k)

	a := sx
	b := 0.0
	c := 0.0
	d := sy
	e := cx * (1 - sx)
	f := cy * (1 - sy)

	p.stream.ConcatMatrix(a, b, c, d, e, f)
}

// Skew applies a skew transform. angleX is the horizontal shear angle
// and angleY is the vertical shear angle (both in degrees), around the
// point (x, y) in user units. Must be called between TransformBegin
// and TransformEnd.
func (p *Page) Skew(angleX, angleY, x, y float64) {
	p = p.active()
	k := p.doc.k
	cx := state.ToPointsX(x, k)
	cy := state.ToPointsY(y, p.h, k)

	tanX := math.Tan(angleX * math.Pi / 180.0)
	tanY := math.Tan(angleY * math.Pi / 180.0)

	a := 1.0
	b := tanY
	c := tanX
	d := 1.0
	e := -cy * tanX
	f := -cx * tanY

	p.stream.ConcatMatrix(a, b, c, d, e, f)
}

// Translate applies a translation by (tx, ty) user units. Positive tx
// moves right, positive ty moves down. Must be called between
// TransformBegin and TransformEnd.
func (p *Page) Translate(tx, ty float64) {
	p = p.active()
	k := p.doc.k
	p.stream.ConcatMatrix(1, 0, 0, 1, tx*k, -ty*k)
}

// TextRotatedAt draws text at (x, y) rotated by angleDeg degrees
// counterclockwise. This is a convenience wrapper around TransformBegin,
// Rotate, TextAt, and TransformEnd.
func (p *Page) TextRotatedAt(x, y, angleDeg float64, text string) {
	p = p.active()
	p.TransformBegin()
	p.Rotate(angleDeg, x, y)
	p.TextAt(x, y, text)
	p.TransformEnd()
}

// --- Gradients ---

// LinearGradient fills a rectangle (x, y, w, h) in user units with a
// linear gradient. (x1, y1) and (x2, y2) define the gradient axis in
// user units (relative to the page, not the rect). stops defines the
// color stops along the axis.
func (p *Page) LinearGradient(x, y, w, h float64, x1, y1, x2, y2 float64, stops ...gradientStop) {
	p = p.active()
	d := p.doc
	if d.err != nil {
		return
	}
	k := d.k

	entry := &gradientEntry{
		name:   fmt.Sprintf("Sh%d", len(d.gradients)+1),
		gtype:  2, // axial
		x0:     state.ToPointsX(x1, k),
		y0:     state.ToPointsY(y1, p.h, k),
		x1:     state.ToPointsX(x2, k),
		y1:     state.ToPointsY(y2, p.h, k),
		colors: stops,
	}
	d.gradients = append(d.gradients, entry)

	// Clip to the rectangle and paint the shading.
	p.stream.SaveState()
	p.stream.Rect(
		state.ToPointsX(x, k),
		state.ToPointsY(y+h, p.h, k),
		w*k,
		h*k,
	)
	p.stream.Clip()
	p.stream.EndPath()
	p.stream.PaintShading(entry.name)
	p.stream.RestoreState()
}

// RadialGradient fills a rectangle (x, y, w, h) in user units with a
// radial gradient. (cx, cy) is the gradient center and r is the outer
// radius, all in user units. stops defines the color stops (0.0 = center,
// 1.0 = edge).
func (p *Page) RadialGradient(x, y, w, h float64, cx, cy, r float64, stops ...gradientStop) {
	p = p.active()
	d := p.doc
	if d.err != nil {
		return
	}
	k := d.k

	entry := &gradientEntry{
		name:   fmt.Sprintf("Sh%d", len(d.gradients)+1),
		gtype:  3, // radial
		x0:     state.ToPointsX(cx, k),
		y0:     state.ToPointsY(cy, p.h, k),
		x1:     state.ToPointsX(cx, k),
		y1:     state.ToPointsY(cy, p.h, k),
		r0:     0,
		r1:     r * k,
		colors: stops,
	}
	d.gradients = append(d.gradients, entry)

	// Clip to the rectangle and paint the shading.
	p.stream.SaveState()
	p.stream.Rect(
		state.ToPointsX(x, k),
		state.ToPointsY(y+h, p.h, k),
		w*k,
		h*k,
	)
	p.stream.Clip()
	p.stream.EndPath()
	p.stream.PaintShading(entry.name)
	p.stream.RestoreState()
}

// --- Clipping ---

// ClipRect begins a rectangular clipping operation at (x, y) with width w
// and height h in user units. If outline is true, the rectangle border is
// stroked. All subsequent drawing is clipped to this rectangle until
// ClipEnd is called.
func (p *Page) ClipRect(x, y, w, h float64, outline ...bool) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	p.stream.SaveState()
	p.stream.Rect(
		state.ToPointsX(x, k),
		state.ToPointsY(y+h, p.h, k),
		w*k,
		h*k,
	)
	p.stream.Clip()
	if len(outline) > 0 && outline[0] {
		p.stream.Stroke()
	} else {
		p.stream.EndPath()
	}
}

// ClipCircle begins a circular clipping operation centered at (x, y) with
// radius r in user units. If outline is true, the border is stroked.
// Call ClipEnd to restore unclipped operations.
func (p *Page) ClipCircle(x, y, r float64, outline ...bool) {
	o := len(outline) > 0 && outline[0]
	p.ClipEllipse(x, y, r, r, o)
}

// ClipEllipse begins an elliptical clipping operation centered at (x, y)
// with horizontal radius rx and vertical radius ry in user units.
// If outline is true, the border is stroked.
// Call ClipEnd to restore unclipped operations.
func (p *Page) ClipEllipse(x, y, rx, ry float64, outline ...bool) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	cx := state.ToPointsX(x, k)
	cy := state.ToPointsY(y, p.h, k)
	rxPt := rx * k
	ryPt := ry * k

	const kappa = 0.5522847498
	kx := rxPt * kappa
	ky := ryPt * kappa

	p.stream.SaveState()
	p.stream.MoveTo(cx+rxPt, cy)
	p.stream.CurveTo(cx+rxPt, cy+ky, cx+kx, cy+ryPt, cx, cy+ryPt)
	p.stream.CurveTo(cx-kx, cy+ryPt, cx-rxPt, cy+ky, cx-rxPt, cy)
	p.stream.CurveTo(cx-rxPt, cy-ky, cx-kx, cy-ryPt, cx, cy-ryPt)
	p.stream.CurveTo(cx+kx, cy-ryPt, cx+rxPt, cy-ky, cx+rxPt, cy)

	p.stream.Clip()
	if len(outline) > 0 && outline[0] {
		p.stream.Stroke()
	} else {
		p.stream.EndPath()
	}
}

// ClipRoundedRect begins a clipping operation with a rounded rectangle
// at (x, y) with width w, height h, and uniform corner radius r in user
// units. If outline is true, the rectangle border is stroked with the
// current draw color and line width. All subsequent drawing is clipped
// to this shape until ClipEnd is called.
func (p *Page) ClipRoundedRect(x, y, w, h, r float64, outline bool) {
	p.ClipRoundedRectExt(x, y, w, h, r, r, r, r, outline)
}

// ClipRoundedRectExt begins a clipping operation with a rounded rectangle
// that has independent corner radii: rTL (top-left), rTR (top-right),
// rBR (bottom-right), rBL (bottom-left). If outline is true, the border
// is stroked. Call ClipEnd to restore unclipped operations.
func (p *Page) ClipRoundedRectExt(x, y, w, h, rTL, rTR, rBR, rBL float64, outline bool) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	k := p.doc.k
	p.stream.SaveState()
	p.stream.RoundedRectExt(
		state.ToPointsX(x, k),
		state.ToPointsY(y+h, p.h, k),
		w*k, h*k,
		rTL*k, rTR*k, rBR*k, rBL*k,
	)
	p.stream.Clip()
	if outline {
		p.stream.Stroke()
	} else {
		p.stream.EndPath()
	}
}

// ClipText begins a clipping operation using text rendered at (x, y) as
// the clip mask. The current font is used. If outline is true, the text
// characters are stroked with the current draw color. All subsequent
// drawing is clipped to the text glyphs until ClipEnd is called.
func (p *Page) ClipText(x, y float64, text string, outline bool) {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	fe := p.effectiveFontEntry()
	if fe == nil {
		p.doc.err = fmt.Errorf("ClipText: no font set")
		return
	}

	k := p.doc.k
	xPt := state.ToPointsX(x, k)
	fontSize := p.effectiveFontSizePt() / k
	yPt := state.ToPointsY(y+0.7*fontSize, p.h, k)

	// Text rendering mode: 5 = stroke + clip, 7 = clip only.
	mode := 7
	if outline {
		mode = 5
	}

	p.stream.SaveState()
	p.stream.BeginText()
	p.stream.SetFont("F"+fe.Index, p.effectiveFontSizePt())
	p.stream.MoveText(xPt, yPt)
	p.stream.SetTextRendering(mode)
	p.emitText(fe, text)
	p.stream.EndText()
}

// ClipPolygon begins a clipping operation within the polygon defined by
// the given points. The polygon is automatically closed. If outline is
// true, the polygon border is stroked. Call ClipEnd to restore unclipped
// operations.
func (p *Page) ClipPolygon(points []Point, outline bool) {
	p = p.active()
	if p.doc.err != nil || len(points) < 3 {
		return
	}
	k := p.doc.k
	p.stream.SaveState()
	p.stream.MoveTo(
		state.ToPointsX(points[0].X, k),
		state.ToPointsY(points[0].Y, p.h, k),
	)
	for _, pt := range points[1:] {
		p.stream.LineTo(
			state.ToPointsX(pt.X, k),
			state.ToPointsY(pt.Y, p.h, k),
		)
	}
	p.stream.ClosePath()
	p.stream.Clip()
	if outline {
		p.stream.Stroke()
	} else {
		p.stream.EndPath()
	}
}

// ClipEnd ends a clipping operation that was started with ClipRect,
// ClipRoundedRect, ClipRoundedRectExt, ClipText, ClipEllipse,
// ClipCircle, or ClipPolygon. Each Clip* call must be balanced by
// exactly one ClipEnd. Clipping operations can be nested.
func (p *Page) ClipEnd() {
	p = p.active()
	if p.doc.err != nil {
		return
	}
	p.stream.RestoreState()
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

	// Track last cell height for Ln(-1).
	d.lastCellH = h

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
	d.lastCellH = h
}

// Writef is like Write but uses printf-style formatting.
func (p *Page) Writef(h float64, format string, args ...any) {
	p.Write(h, fmt.Sprintf(format, args...))
}

// WriteLinkString writes text that, when clicked, opens the given URL.
// h is the line height. The text is rendered inline (with word wrapping)
// and a clickable link annotation is placed over it.
func (p *Page) WriteLinkString(h float64, text, url string) {
	p = p.active()
	d := p.doc
	if d.err != nil {
		return
	}
	// Record where text starts, draw it, then add a link annotation.
	startX := p.GetX()
	startY := p.GetY()
	p.Write(h, text)
	p = p.active()
	endX := p.GetX()

	// Simple case: single-line link (no page break during Write).
	sw := endX - startX
	if sw > 0 {
		p.links = append(p.links, linkAnnotation{
			x: startX, y: startY, w: sw, h: h,
			url: url,
		})
	}
}

// WriteAligned writes multi-line text with horizontal alignment.
// width is the box width (0 = full page width minus margins).
// h is the line height. align is "L", "C", or "R".
func (p *Page) WriteAligned(width, h float64, text, align string) {
	p = p.active()
	d := p.doc
	if d.err != nil {
		return
	}

	if width == 0 {
		width = p.w - d.lMargin - d.rMargin
	}

	lines := p.SplitText(text, width)
	savedLM := d.lMargin
	savedRM := d.rMargin

	for _, line := range lines {
		lineW := p.GetStringWidth(line)
		switch strings.ToUpper(align) {
		case "C":
			d.lMargin = savedLM + (width-lineW)/2
			p.x = d.lMargin
			p.Write(h, line)
			d.lMargin = savedLM
		case "R":
			d.lMargin = savedLM + width - lineW - 2.01*d.cMargin
			p.x = d.lMargin
			p.Write(h, line)
			d.lMargin = savedLM
		default: // "L"
			d.rMargin = p.w - savedLM - width
			p.x = savedLM
			p.Write(h, line)
			d.rMargin = savedRM
		}
		p = p.active()
	}
}

// SubWrite writes text as superscript or subscript. h is the line height.
// fontSize is the size in points for the sub/superscript text.
// offset is the vertical offset in points: positive = superscript (up),
// negative = subscript (down).
func (p *Page) SubWrite(h float64, text string, fontSize, offset float64) {
	p = p.active()
	d := p.doc
	if d.err != nil {
		return
	}

	oldSize := d.fontSizePt
	k := d.k

	// Change to sub/super font size.
	d.SetFontSize(fontSize)

	// Calculate vertical offset in user units.
	yShift := ((fontSize-oldSize)/k)*0.3 + offset/k
	savedX := p.GetX()
	savedY := p.GetY()
	p.SetXY(savedX, savedY-yShift)

	p.Write(h, text)

	// Restore position and font size.
	p = p.active()
	endX := p.GetX()
	p.SetXY(endX, savedY)
	d.SetFontSize(oldSize)
}

// Cellf is like Cell but uses printf-style formatting for the text content.
func (p *Page) Cellf(w, h float64, format string, args ...any) {
	p.Cell(w, h, fmt.Sprintf(format, args...), "", "L", false, 0)
}

// Ln performs a line break. The cursor X moves back to the left margin.
// h is the vertical distance to advance. A negative value of h uses the
// height of the last Cell or Write output.
func (p *Page) Ln(h float64) {
	p = p.active()
	d := p.doc
	p.x = d.lMargin
	if h < 0 {
		p.y += d.lastCellH
	} else {
		p.y += h
	}
}

// SplitText splits text into lines that fit within the given width (in user
// units) using the current font. This is useful for calculating the total
// height of wrapped text for layout purposes.
func (p *Page) SplitText(text string, width float64) []string {
	p = p.active()
	d := p.doc
	fe := p.effectiveFontEntry()
	if fe == nil || d.err != nil {
		return nil
	}

	fontSize := p.effectiveFontSizePt()
	k := d.k
	maxW := (width - 2*d.cMargin) * 1000.0 / (fontSize / k)

	runes := []rune(text)
	n := len(runes)
	// Trim trailing newlines.
	for n > 0 && runes[n-1] == '\n' {
		n--
	}
	runes = runes[:n]

	var lines []string
	sep := -1
	i, j := 0, 0
	l := 0.0

	for i < n {
		c := runes[i]

		if c == '\n' {
			lines = append(lines, string(runes[j:i]))
			i++
			j = i
			sep = -1
			l = 0
			continue
		}

		l += p.runeWidth1000(fe, c)

		if c == ' ' || isCJK(c) {
			sep = i
		}

		if l > maxW {
			if sep == -1 {
				if i == j {
					i++
				}
				sep = i
			} else {
				i = sep + 1
			}
			lines = append(lines, string(runes[j:sep]))
			sep = -1
			j = i
			l = 0
		} else {
			i++
		}
	}
	if i != j {
		lines = append(lines, string(runes[j:i]))
	}
	return lines
}

// runeWidth1000 returns the width of a rune in 1/1000 font-size units.
func (p *Page) runeWidth1000(fe *resources.FontEntry, r rune) float64 {
	if fe.Type == "TTF" && fe.TTF != nil {
		ch := int(r)
		if ch < len(fe.TTF.CharWidths) {
			return float64(fe.TTF.CharWidths[ch])
		}
		return 0
	}
	if r < 256 {
		return float64(fe.Widths[byte(r)])
	}
	return 0
}

// SplitLines splits text (as bytes) into lines that fit within the given
// width in user units. This is the byte-based counterpart of SplitText,
// provided for compatibility with gofpdf.
func (p *Page) SplitLines(txt []byte, w float64) [][]byte {
	lines := p.SplitText(string(txt), w)
	result := make([][]byte, len(lines))
	for i, line := range lines {
		result[i] = []byte(line)
	}
	return result
}

// isCJK reports whether a rune is in a CJK ideograph range where line
// breaks are allowed between any two characters.
func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0x3000 && r <= 0x303F) || // CJK Symbols and Punctuation
		(r >= 0xFF00 && r <= 0xFFEF) // Halfwidth and Fullwidth Forms
}

// RichText draws inline text with simple HTML-like markup for styling.
// Supported tags: <b> (bold), <i> (italic), <u> (underline), <s> (strikethrough).
// Tags can be nested. The current font family and size are preserved; only
// style (bold/italic) and decoration (underline/strikethrough) change.
//
//	page.RichText(6, "This is <b>bold</b> and <i>italic</i> text.")
func (p *Page) RichText(h float64, markup string) {
	p = p.active()
	d := p.doc
	if d.err != nil {
		return
	}

	// Save initial state to restore after.
	origStyle := d.fontStyle
	origUnder := d.underline
	origStrike := d.strikethrough

	// Parse and render segments.
	bold := strings.Contains(origStyle, "B")
	italic := strings.Contains(origStyle, "I")
	under := origUnder
	strike := origStrike

	applyState := func() {
		style := ""
		if bold {
			style += "B"
		}
		if italic {
			style += "I"
		}
		d.SetFontStyle(style)
		d.SetUnderline(under)
		d.SetStrikethrough(strike)
	}

	i := 0
	n := len(markup)

	for i < n {
		// Look for next '<'
		tagStart := strings.IndexByte(markup[i:], '<')
		if tagStart < 0 {
			// No more tags — output remaining text.
			p = p.active()
			p.Write(h, markup[i:])
			break
		}

		// Output text before the tag.
		if tagStart > 0 {
			p = p.active()
			p.Write(h, markup[i:i+tagStart])
		}
		i += tagStart

		// Find closing '>'
		tagEnd := strings.IndexByte(markup[i:], '>')
		if tagEnd < 0 {
			// Malformed — output rest as literal text.
			p = p.active()
			p.Write(h, markup[i:])
			break
		}

		tagLiteral := markup[i : i+tagEnd+1] // e.g. "<b>" or "<unknown>"
		tag := strings.ToLower(markup[i+1 : i+tagEnd])
		i += tagEnd + 1

		switch tag {
		case "b":
			bold = true
			applyState()
		case "/b":
			bold = false
			applyState()
		case "i":
			italic = true
			applyState()
		case "/i":
			italic = false
			applyState()
		case "u":
			under = true
			applyState()
		case "/u":
			under = false
			applyState()
		case "s":
			strike = true
			applyState()
		case "/s":
			strike = false
			applyState()
		default:
			// Unknown tag — output as literal text.
			p = p.active()
			p.Write(h, tagLiteral)
		}
	}

	// Restore original state.
	d.SetFontStyle(origStyle)
	d.SetUnderline(origUnder)
	d.SetStrikethrough(origStrike)
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
	ut := float64(fe.Ut) * p.doc.underlineThickness // typically positive (e.g. 50)

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

// Link creates a clickable area that navigates to an integer-based internal
// link destination (set by Document.SetLink). linkID is the value returned
// by Document.AddLink.
func (p *Page) Link(x, y, w, h float64, linkID int) {
	p = p.active()
	p.links = append(p.links, linkAnnotation{x: x, y: y, w: w, h: h, linkID: linkID})
}

// WriteLinkID writes text that navigates to an integer-based internal link.
// h is the line height. linkID is the value returned by Document.AddLink.
func (p *Page) WriteLinkID(h float64, text string, linkID int) {
	p = p.active()
	d := p.doc
	if d.err != nil {
		return
	}
	startX := p.GetX()
	startY := p.GetY()
	p.Write(h, text)
	p = p.active()
	endX := p.GetX()

	sw := endX - startX
	if sw > 0 {
		p.links = append(p.links, linkAnnotation{
			x: startX, y: startY, w: sw, h: h,
			linkID: linkID,
		})
	}
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
	// RTL: reverse rune order for right-to-left text.
	if p.doc.isRTL {
		runes := []rune(text)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		text = string(runes)
	}

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
