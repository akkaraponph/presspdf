package folio

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/akkaraponph/folio/internal/resources"
	"github.com/akkaraponph/folio/internal/state"
)

// Document is the root object for creating a PDF.
type Document struct {
	// metadata
	title    string
	author   string
	subject  string
	creator  string
	producer string

	// configuration
	unit     state.Unit
	k        float64 // scale factor: points per user unit
	compress bool
	defSize  PageSize
	lMargin  float64 // left margin in user units
	tMargin  float64 // top margin
	rMargin  float64 // right margin
	bMargin       float64 // bottom margin
	cMargin       float64 // cell margin
	autoPageBreak bool    // automatic page breaking

	// pages
	pages       []*Page
	currentPage *Page

	// resources
	fonts  *resources.FontRegistry
	images *resources.ImageRegistry

	// current font state (carried across pages)
	fontFamily string
	fontStyle  string
	fontSizePt float64
	fontEntry  *resources.FontEntry

	// current drawing state (carried across pages)
	drawColor state.Color
	fillColor state.Color
	textColor state.Color
	lineWidth float64

	// optional text segmenter used by MultiCell / wrapText. When nil, the
	// default behaviour splits on ASCII whitespace. Set this to plug in a
	// language-aware segmenter (e.g. a Thai word breaker) so line wrapping
	// respects word boundaries.
	wordBreaker WordBreakFunc

	// link anchors (named destinations for internal links)
	anchors map[string]anchorDest

	// header/footer callbacks
	headerFunc     func(*Page)
	footerFunc     func(*Page)
	inHeader       bool
	inFooter       bool
	lastPageClosed bool // true after final page footer has been called

	// error accumulation
	err error
}

// anchorDest stores the target location for an internal link destination.
type anchorDest struct {
	page *Page   // the page containing the anchor
	y    float64 // Y position in user units
}

// WordBreakFunc segments a paragraph (a line containing no '\n') into
// units between which a line break is allowed. The returned segments are
// joined verbatim — the segmenter owns any whitespace — so a Thai
// segmenter can return dictionary words with no separators while a
// whitespace-preserving segmenter can return segments that include their
// trailing space.
type WordBreakFunc func(paragraph string) []string

// SetWordBreaker installs a custom word segmenter used by MultiCell when
// wrapping text. Pass nil to revert to the default whitespace-based
// splitting.
func (d *Document) SetWordBreaker(f WordBreakFunc) {
	d.wordBreaker = f
}

// New creates a new Document with the given options.
// Defaults: mm units, compression enabled, 10mm margins, A4 page size.
func New(opts ...Option) *Document {
	d := &Document{
		producer: "Folio",
		unit:     state.UnitMM,
		k:        state.ScaleFactor(state.UnitMM),
		compress: true,
		defSize:  A4,
		lMargin:  10,
		tMargin:  10,
		rMargin:  10,
		bMargin:  10,
		cMargin:  2,
		fonts:    resources.NewFontRegistry(),
		images:   resources.NewImageRegistry(),
		anchors:  make(map[string]anchorDest),
		lineWidth: 0.2,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// SetTitle sets the document title metadata.
func (d *Document) SetTitle(s string) { d.title = s }

// SetAuthor sets the document author metadata.
func (d *Document) SetAuthor(s string) { d.author = s }

// SetSubject sets the document subject metadata.
func (d *Document) SetSubject(s string) { d.subject = s }

// SetCreator sets the document creator metadata.
func (d *Document) SetCreator(s string) { d.creator = s }

// SetMargins sets the left, top, and right margins in user units.
func (d *Document) SetMargins(left, top, right float64) {
	d.lMargin = left
	d.tMargin = top
	d.rMargin = right
}

// SetAutoPageBreak enables or disables automatic page breaking.
// When enabled, Cell and MultiCell automatically create a new page when
// content would overflow past the bottom margin. margin sets the bottom
// margin distance from the page edge (in user units).
func (d *Document) SetAutoPageBreak(auto bool, margin float64) {
	d.autoPageBreak = auto
	d.bMargin = margin
}

// CurrentPage returns the most recently added page (the active drawing target).
func (d *Document) CurrentPage() *Page {
	return d.currentPage
}

// PageCount returns the number of pages in the document.
func (d *Document) PageCount() int {
	return len(d.pages)
}

// PageNo returns the current page number (1-based).
func (d *Document) PageNo() int {
	return len(d.pages)
}

// SetHeaderFunc sets a function that is called at the top of every new page.
// The callback receives the new page and may draw a header (title, rule,
// logo, etc.). Document state (font, colors) is automatically saved before
// and restored after the callback, so the header cannot leak visual state
// into the page body. The cursor Y position is left where the header ends,
// so body content starts below it.
func (d *Document) SetHeaderFunc(f func(*Page)) {
	d.headerFunc = f
}

// SetFooterFunc sets a function that is called at the bottom of every page.
// It runs on the outgoing page just before a new page is created (via
// AddPage or auto page break) and on the last page during serialization.
// Document state is saved/restored around the call. Use page.SetY with a
// negative value to position from the bottom (e.g. page.SetY(-15) places
// the cursor 15 user-units above the page edge).
func (d *Document) SetFooterFunc(f func(*Page)) {
	d.footerFunc = f
}

// docState holds a snapshot of the mutable document-level visual state so
// it can be saved before and restored after header/footer callbacks.
type docState struct {
	fontFamily string
	fontStyle  string
	fontSizePt float64
	fontEntry  *resources.FontEntry
	drawColor  state.Color
	fillColor  state.Color
	textColor  state.Color
	lineWidth  float64
}

func (d *Document) saveDocState() docState {
	return docState{
		fontFamily: d.fontFamily,
		fontStyle:  d.fontStyle,
		fontSizePt: d.fontSizePt,
		fontEntry:  d.fontEntry,
		drawColor:  d.drawColor,
		fillColor:  d.fillColor,
		textColor:  d.textColor,
		lineWidth:  d.lineWidth,
	}
}

func (d *Document) restoreDocState(s docState) {
	d.fontFamily = s.fontFamily
	d.fontStyle = s.fontStyle
	d.fontSizePt = s.fontSizePt
	d.fontEntry = s.fontEntry
	d.drawColor = s.drawColor
	d.fillColor = s.fillColor
	d.textColor = s.textColor
	d.lineWidth = s.lineWidth
}

// callHeader invokes the header callback on p, wrapped in a graphics-state
// save/restore and document-state save/restore.
func (d *Document) callHeader(p *Page) {
	if d.headerFunc == nil {
		return
	}
	saved := d.saveDocState()
	p.stream.SaveState()
	d.inHeader = true
	d.headerFunc(p)
	d.inHeader = false
	p.stream.RestoreState()
	d.restoreDocState(saved)
	// Re-apply the restored font to the page stream (the PDF Q operator
	// restores graphics state but not the text font).
	if d.fontEntry != nil {
		p.applyFont(d.fontEntry, d.fontSizePt)
	}
}

// callFooter invokes the footer callback on p, wrapped in state save/restore.
func (d *Document) callFooter(p *Page) {
	if d.footerFunc == nil {
		return
	}
	saved := d.saveDocState()
	p.stream.SaveState()
	d.inFooter = true
	d.footerFunc(p)
	d.inFooter = false
	p.stream.RestoreState()
	d.restoreDocState(saved)
	if d.fontEntry != nil {
		p.applyFont(d.fontEntry, d.fontSizePt)
	}
}

// closeDoc finalises the last page (calls its footer) before serialisation.
func (d *Document) closeDoc() {
	if d.lastPageClosed || d.currentPage == nil {
		return
	}
	d.callFooter(d.currentPage)
	d.lastPageClosed = true
}

// SetFont sets the current font. Core fonts are auto-registered.
// For TTF fonts, call AddUTF8Font first, then use the same family name here.
// family: "helvetica", "courier", "times", "arial", "zapfdingbats", or a TTF family name
// style: "", "B", "I", "BI"
// size: font size in points
func (d *Document) SetFont(family, style string, size float64) {
	if d.err != nil {
		return
	}

	// Try to get existing font first (handles both core and TTF)
	fe, ok := d.fonts.Get(family, style)
	if !ok {
		// Not found: try registering as core font
		var err error
		fe, err = d.fonts.Register(family, style)
		if err != nil {
			d.err = fmt.Errorf("SetFont: %w", err)
			return
		}
	}

	d.fontFamily = family
	d.fontStyle = style
	d.fontSizePt = size
	d.fontEntry = fe

	// If a page is active, emit the font change to its content stream
	if d.currentPage != nil {
		d.currentPage.applyFont(fe, size)
	}
}

// AddUTF8FontFromFile registers a TrueType font from a file path.
// family is the name used with SetFont. style is "", "B", "I", or "BI".
func (d *Document) AddUTF8FontFromFile(family, style, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("AddUTF8FontFromFile: %w", err)
	}
	return d.AddUTF8Font(family, style, data)
}

// SetFontSize changes the font size without changing the family or style.
func (d *Document) SetFontSize(size float64) {
	if d.err != nil {
		return
	}
	if d.fontEntry == nil {
		d.err = fmt.Errorf("SetFontSize: no font set")
		return
	}
	d.fontSizePt = size
	if d.currentPage != nil {
		d.currentPage.applyFont(d.fontEntry, size)
	}
}

// SetFontStyle changes the font style (e.g. "B", "I", "BI", "") without
// changing the family or size. The font family+style must already be registered.
func (d *Document) SetFontStyle(style string) {
	if d.err != nil {
		return
	}
	if d.fontFamily == "" {
		d.err = fmt.Errorf("SetFontStyle: no font set")
		return
	}
	fe, ok := d.fonts.Get(d.fontFamily, style)
	if !ok {
		// Try registering as core font
		var err error
		fe, err = d.fonts.Register(d.fontFamily, style)
		if err != nil {
			d.err = fmt.Errorf("SetFontStyle: %w", err)
			return
		}
	}
	d.fontStyle = style
	d.fontEntry = fe
	if d.currentPage != nil {
		d.currentPage.applyFont(fe, d.fontSizePt)
	}
}

// GetFontFamily returns the current font family name.
func (d *Document) GetFontFamily() string { return d.fontFamily }

// GetFontStyle returns the current font style ("", "B", "I", or "BI").
func (d *Document) GetFontStyle() string { return d.fontStyle }

// GetFontSize returns the current font size in points.
func (d *Document) GetFontSize() float64 { return d.fontSizePt }

// SetDrawColor sets the stroke color using 0-255 RGB values.
func (d *Document) SetDrawColor(r, g, b int) {
	d.drawColor = state.ColorFromRGB(r, g, b)
	if d.currentPage != nil {
		d.currentPage.stream.SetStrokeColorRGB(d.drawColor.R, d.drawColor.G, d.drawColor.B)
	}
}

// SetFillColor sets the fill color using 0-255 RGB values.
func (d *Document) SetFillColor(r, g, b int) {
	d.fillColor = state.ColorFromRGB(r, g, b)
	if d.currentPage != nil {
		d.currentPage.stream.SetFillColorRGB(d.fillColor.R, d.fillColor.G, d.fillColor.B)
	}
}

// SetTextColor sets the text color using 0-255 RGB values.
func (d *Document) SetTextColor(r, g, b int) {
	d.textColor = state.ColorFromRGB(r, g, b)
}

// SetLineWidth sets the line width in user units.
func (d *Document) SetLineWidth(w float64) {
	d.lineWidth = w
	if d.currentPage != nil {
		d.currentPage.stream.SetLineWidth(w * d.k)
	}
}

// AddPage adds a new page with the given size and returns it.
// If a footer function is set, it is called on the outgoing page first.
// If a header function is set, it is called on the new page after creation.
func (d *Document) AddPage(size PageSize) *Page {
	if d.err != nil {
		return &Page{doc: d}
	}

	// Call footer on the outgoing page (skip if we're already inside a
	// header/footer to prevent recursion).
	if d.currentPage != nil && !d.inHeader && !d.inFooter {
		d.callFooter(d.currentPage)
	}

	p := &Page{
		doc:  d,
		size: size,
		w:    size.WidthPt / d.k,
		h:    size.HeightPt / d.k,
		x:    d.lMargin,
		y:    d.tMargin,
	}

	d.pages = append(d.pages, p)
	d.currentPage = p
	d.lastPageClosed = false

	// Emit initial page state
	p.stream.SetLineWidth(d.lineWidth * d.k)
	p.stream.SetLineCap(0)
	p.stream.SetLineJoin(0)

	// Restore font if one was set
	if d.fontEntry != nil {
		p.applyFont(d.fontEntry, d.fontSizePt)
		p.fontFamily = d.fontFamily
		p.fontStyle = d.fontStyle
		p.fontSizePt = d.fontSizePt
		p.fontEntry = d.fontEntry
	}

	// Apply colors if non-default
	if !d.drawColor.IsBlack() {
		p.stream.SetStrokeColorRGB(d.drawColor.R, d.drawColor.G, d.drawColor.B)
	}
	if !d.fillColor.IsBlack() {
		p.stream.SetFillColorRGB(d.fillColor.R, d.fillColor.G, d.fillColor.B)
	}

	// Call header on the new page (skip if inside header/footer).
	if !d.inHeader && !d.inFooter {
		d.callHeader(p)
	}

	return p
}

// AddUTF8Font registers a TrueType font from raw bytes for UTF-8 text support.
// family is the name used with SetFont (e.g., "thsarabun").
// style is "", "B", "I", or "BI".
func (d *Document) AddUTF8Font(family, style string, data []byte) error {
	if d.err != nil {
		return d.err
	}
	ttf, err := resources.ParseTTF(data)
	if err != nil {
		d.err = fmt.Errorf("AddUTF8Font: %w", err)
		return d.err
	}
	_, err = d.fonts.RegisterTTF(family, style, ttf)
	if err != nil {
		d.err = fmt.Errorf("AddUTF8Font: %w", err)
		return d.err
	}
	return nil
}

// RegisterImage registers an image from a reader for later use.
// The format is auto-detected from the first bytes of data.
// Supported formats: JPEG and PNG (including RGBA with transparency).
func (d *Document) RegisterImage(name string, r io.Reader) error {
	if d.err != nil {
		return d.err
	}
	// Read enough to detect the format, then recombine for the decoder.
	buf, err := io.ReadAll(r)
	if err != nil {
		d.err = fmt.Errorf("RegisterImage: %w", err)
		return d.err
	}
	reader := bytes.NewReader(buf)

	var regErr error
	if isPNG(buf) {
		_, regErr = d.images.RegisterPNG(name, reader)
	} else {
		_, regErr = d.images.RegisterJPEG(name, reader)
	}
	if regErr != nil {
		d.err = fmt.Errorf("RegisterImage: %w", regErr)
	}
	return regErr
}

// isPNG checks if data starts with the PNG magic bytes.
func isPNG(data []byte) bool {
	return len(data) >= 4 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G'
}

// WriteTo serializes the PDF and writes it to w.
func (d *Document) WriteTo(w io.Writer) (int64, error) {
	if d.err != nil {
		return 0, d.err
	}
	d.closeDoc()
	pw, err := d.serialize()
	if err != nil {
		return 0, err
	}
	return pw.WriteTo(w)
}

// Save writes the PDF to a file.
func (d *Document) Save(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = d.WriteTo(f)
	return err
}

// Bytes returns the serialized PDF as a byte slice.
func (d *Document) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	_, err := d.WriteTo(&buf)
	return buf.Bytes(), err
}

// Err returns the first accumulated error, if any.
func (d *Document) Err() error { return d.err }

// pdfDate formats a time.Time as a PDF date string: D:YYYYMMDDHHmmSS
func pdfDate(t time.Time) string {
	return fmt.Sprintf("D:%04d%02d%02d%02d%02d%02d",
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second())
}
