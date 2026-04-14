package presspdf

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/akkaraponph/presspdf/internal/resources"
	"github.com/akkaraponph/presspdf/internal/state"
)

// Document is the root object for creating a PDF.
type Document struct {
	// metadata
	title    string
	author   string
	subject  string
	creator  string
	producer string
	keywords string

	// dates (zero means use time.Now() during serialization)
	creationDate time.Time
	modDate      time.Time

	// XMP metadata (raw XML bytes, embedded as-is)
	xmpMetadata []byte

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

	// display mode
	zoomMode   string // "fullpage", "fullwidth", "real", "default"
	layoutMode string // "single", "continuous", "two", "default", PDF names

	// JavaScript
	javascript *string

	// alias replacement (applied to page streams before output)
	aliases      map[string]string
	aliasNbPages string // placeholder replaced with total page count

	// line style state
	capStyle  int // 0=butt, 1=round, 2=square
	joinStyle int // 0=miter, 1=round, 2=bevel

	// custom page break acceptance function
	acceptPageBreakFunc func() bool

	// default page boxes (applied to new pages if set)
	defPageBoxes map[string][4]float64 // "TrimBox", "CropBox", etc. → [llx, lly, urx, ury] in points

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
	drawColor     state.Color
	fillColor     state.Color
	textColor     state.Color
	lineWidth     float64
	underline          bool
	strikethrough      bool
	underlineThickness float64 // multiplier for underline weight (default 1.0)
	currentAlpha       float64 // current opacity (0.0–1.0), default 1.0
	currentBlendMode   string  // current blend mode, default "Normal"
	charSpacing        float64 // extra space between characters (Tc), in points
	wordSpacing        float64 // extra space added to ASCII space (Tw), in points
	textRise           float64 // vertical text baseline shift (Ts), in points
	lastCellH          float64 // height of the last Cell/Write output (for Ln)

	// alpha transparency states (ExtGState resources)
	alphaStates []*alphaEntry
	alphaByKey  map[string]*alphaEntry

	// gradient shadings
	gradients []*gradientEntry

	// page templates (Form XObjects)
	templates []*templateEntry

	// optional text segmenter used by MultiCell / wrapText. When nil, the
	// default behaviour splits on ASCII whitespace. Set this to plug in a
	// language-aware segmenter (e.g. a Thai word breaker) so line wrapping
	// respects word boundaries.
	wordBreaker WordBreakFunc

	// bookmarks/outlines
	outlines []*outlineEntry

	// link anchors (named destinations for internal links)
	anchors map[string]anchorDest

	// integer-based internal links (gofpdf-compatible)
	intLinks []linkDest

	// header/footer callbacks
	headerFunc     func(*Page)
	headerHomeMode bool // if true, reset XY to margins after header
	footerFunc     func(*Page)
	footerFuncLpi  func(*Page, bool) // variant with lastPage indicator
	inHeader       bool
	inFooter       bool
	lastPageClosed bool // true after final page footer has been called

	// form fields (AcroForms)
	formFields []*formField

	// digital signature
	sigState *signatureState

	// PDF/A conformance
	pdfaLevel *pdfaConf

	// tagged PDF (accessibility)
	tagged     bool
	structRoot *structElement

	// optional content groups (layers)
	layers        []layerEntry
	currentLayer  int  // -1 = no active layer
	openLayerPane bool // true = open layer panel on document open

	// spot colors (Separation color spaces)
	spotColors    []spotColorEntry
	spotColorMap  map[string]int // name → index in spotColors

	// file attachments
	attachments []Attachment

	// RTL text direction
	isRTL bool

	// catalogSort enables deterministic sorting of resource catalog keys
	catalogSort bool

	// encryption (password protection)
	encrypted   bool
	encryptAES  bool   // true = AES-256 (V=5 R=6), false = RC4-40 (V=1 R=2)
	userPw      string
	ownerPw     string
	permissions int32

	// error accumulation
	err error
}

// alphaEntry represents a registered alpha transparency ExtGState.
type alphaEntry struct {
	alpha     float64 // opacity 0.0–1.0
	blendMode string  // PDF blend mode: "Normal", "Multiply", etc.
	name      string  // resource name: "GS1", "GS2", ...
	objNum    int     // PDF object number, set during serialization
}

// layerEntry represents an Optional Content Group (OCG) layer.
type layerEntry struct {
	name    string
	visible bool
	objNum  int // set during serialization
}

// spotColorEntry represents a Separation (spot) color definition.
type spotColorEntry struct {
	name         string
	c, m, y, k  float64 // CMYK components 0.0–1.0
	objNum       int     // set during serialization
}

// Attachment represents a file to embed in the PDF.
type Attachment struct {
	Content     []byte // file data
	Filename    string // display name
	Description string // optional description
	objNum      int    // set during serialization
}

// anchorDest stores the target location for an internal link destination.
type anchorDest struct {
	page *Page   // the page containing the anchor
	y    float64 // Y position in user units
}

// outlineEntry represents a bookmark in the PDF outline (navigation sidebar).
type outlineEntry struct {
	title string  // display title
	level int     // nesting level: 0 = top-level, 1 = child, etc.
	page  *Page   // target page
	y     float64 // target Y position in user units
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
		producer: "PressPDF",
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
		anchors:      make(map[string]anchorDest),
		alphaByKey:   make(map[string]*alphaEntry),
		aliases:      make(map[string]string),
		spotColorMap: make(map[string]int),
		currentLayer: -1,
		zoomMode:     "default",
		layoutMode:   "default",
		currentAlpha:       1.0,
		currentBlendMode:   "Normal",
		underlineThickness: 1.0,
		lineWidth:          0.2,
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

// SetKeywords sets the document keywords metadata (space-separated).
func (d *Document) SetKeywords(s string) { d.keywords = s }

// SetProducer overrides the document producer metadata (default "PressPDF").
func (d *Document) SetProducer(s string) { d.producer = s }

// SetCreationDate fixes the document's CreationDate metadata to a specific time.
// By default, the current time is used during serialization.
// Pass a zero-value time to revert to the default behavior.
func (d *Document) SetCreationDate(t time.Time) { d.creationDate = t }

// SetModificationDate fixes the document's ModDate metadata to a specific time.
// By default, the current time is used during serialization.
// Pass a zero-value time to revert to the default behavior.
func (d *Document) SetModificationDate(t time.Time) { d.modDate = t }

// SetXmpMetadata sets raw XMP metadata (XML bytes) to embed in the PDF.
// This is useful for PDF/A compliance or custom metadata schemas.
func (d *Document) SetXmpMetadata(xmp []byte) { d.xmpMetadata = xmp }

// SetDisplayMode controls how the PDF viewer opens the document.
//
// zoom: "fullpage" (fit whole page), "fullwidth" (fit width),
// "real" (100%), "default" (viewer default).
//
// layout: "single"/"SinglePage", "continuous"/"OneColumn",
// "two"/"TwoColumnLeft", "TwoColumnRight",
// "TwoPageLeft", "TwoPageRight", "default".
func (d *Document) SetDisplayMode(zoom, layout string) {
	if d.err != nil {
		return
	}
	if zoom == "" {
		zoom = "default"
	}
	if layout == "" {
		layout = "default"
	}
	switch zoom {
	case "fullpage", "fullwidth", "real", "default":
		d.zoomMode = zoom
	default:
		d.err = fmt.Errorf("SetDisplayMode: invalid zoom mode: %q", zoom)
		return
	}
	switch layout {
	case "single", "continuous", "two", "default",
		"SinglePage", "OneColumn", "TwoColumnLeft", "TwoColumnRight",
		"TwoPageLeft", "TwoPageRight":
		d.layoutMode = layout
	default:
		d.err = fmt.Errorf("SetDisplayMode: invalid layout mode: %q", layout)
	}
}

// SetCompression enables or disables zlib compression of page content
// streams. Compression is enabled by default and typically reduces
// file size by ~50%.
func (d *Document) SetCompression(compress bool) { d.compress = compress }

// AliasNbPages defines a placeholder string that will be replaced with the
// total page count in all page content streams when the document is
// serialized. An empty string defaults to "{nb}".
//
// Usage: page.Cell(0, 10, "Page 1 of {nb}", ...)
func (d *Document) AliasNbPages(alias string) {
	if alias == "" {
		alias = "{nb}"
	}
	d.aliasNbPages = alias
}

// RegisterAlias registers a (alias, replacement) pair. All occurrences of
// alias in page content streams are replaced with replacement during
// serialization.
func (d *Document) RegisterAlias(alias, replacement string) {
	d.aliases[alias] = replacement
}

// SetJavascript embeds Adobe JavaScript in the document. The script runs
// when the PDF is opened.
func (d *Document) SetJavascript(script string) {
	d.javascript = &script
}

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
	d.headerHomeMode = false
}

// SetHeaderFuncMode is like SetHeaderFunc but with a homeMode flag.
// When homeMode is true, the cursor position is reset to the top-left
// margin (home position) after the header callback returns. When false,
// the cursor stays where the header left it (same as SetHeaderFunc).
func (d *Document) SetHeaderFuncMode(f func(*Page), homeMode bool) {
	d.headerFunc = f
	d.headerHomeMode = homeMode
}

// SetFooterFunc sets a function that is called at the bottom of every page.
// It runs on the outgoing page just before a new page is created (via
// AddPage or auto page break) and on the last page during serialization.
// Document state is saved/restored around the call. Use page.SetY with a
// negative value to position from the bottom (e.g. page.SetY(-15) places
// the cursor 15 user-units above the page edge).
func (d *Document) SetFooterFunc(f func(*Page)) {
	d.footerFunc = f
	d.footerFuncLpi = nil
}

// SetFooterFuncLpi is like SetFooterFunc but the callback receives an
// additional boolean indicating whether this is the last page. This is
// useful for conditional footer rendering (e.g., omitting "continued"
// text on the final page).
func (d *Document) SetFooterFuncLpi(f func(page *Page, lastPage bool)) {
	d.footerFuncLpi = f
	d.footerFunc = nil
}

// docState holds a snapshot of the mutable document-level visual state so
// it can be saved before and restored after header/footer callbacks.
type docState struct {
	fontFamily         string
	fontStyle          string
	fontSizePt         float64
	fontEntry          *resources.FontEntry
	drawColor          state.Color
	fillColor          state.Color
	textColor          state.Color
	lineWidth          float64
	underline          bool
	strikethrough      bool
	underlineThickness float64
	currentAlpha       float64
	charSpacing        float64
	wordSpacing        float64
	textRise           float64
	capStyle           int
	joinStyle          int
}

func (d *Document) saveDocState() docState {
	return docState{
		fontFamily:    d.fontFamily,
		fontStyle:     d.fontStyle,
		fontSizePt:    d.fontSizePt,
		fontEntry:     d.fontEntry,
		drawColor:     d.drawColor,
		fillColor:     d.fillColor,
		textColor:     d.textColor,
		lineWidth:     d.lineWidth,
		underline:          d.underline,
		strikethrough:      d.strikethrough,
		underlineThickness: d.underlineThickness,
		currentAlpha:       d.currentAlpha,
		charSpacing:   d.charSpacing,
		wordSpacing:   d.wordSpacing,
		textRise:      d.textRise,
		capStyle:      d.capStyle,
		joinStyle:     d.joinStyle,
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
	d.underline = s.underline
	d.strikethrough = s.strikethrough
	d.underlineThickness = s.underlineThickness
	d.currentAlpha = s.currentAlpha
	d.charSpacing = s.charSpacing
	d.wordSpacing = s.wordSpacing
	d.textRise = s.textRise
	d.capStyle = s.capStyle
	d.joinStyle = s.joinStyle
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
	// In homeMode, reset cursor to top-left margin after the header.
	if d.headerHomeMode {
		p.x = d.lMargin
		p.y = d.tMargin
	}
}

// callFooter invokes the footer callback on p, wrapped in state save/restore.
// lastPage indicates whether this is the final page (for SetFooterFuncLpi).
func (d *Document) callFooter(p *Page, lastPage ...bool) {
	if d.footerFunc == nil && d.footerFuncLpi == nil {
		return
	}
	saved := d.saveDocState()
	p.stream.SaveState()
	d.inFooter = true
	if d.footerFuncLpi != nil {
		isLast := len(lastPage) > 0 && lastPage[0]
		d.footerFuncLpi(p, isLast)
	} else {
		d.footerFunc(p)
	}
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
	d.callFooter(d.currentPage, true)
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

// SetAlpha sets the opacity for subsequent drawing operations.
// alpha ranges from 0.0 (fully transparent) to 1.0 (fully opaque).
// blendMode optionally specifies a PDF blend mode. Pass "" or "Normal"
// for normal compositing. Supported modes: "Normal", "Multiply", "Screen",
// "Overlay", "Darken", "Lighten", "ColorDodge", "ColorBurn", "HardLight",
// "SoftLight", "Difference", "Exclusion", "Hue", "Saturation", "Color",
// "Luminosity".
func (d *Document) SetAlpha(alpha float64, blendMode ...string) {
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	d.currentAlpha = alpha

	bm := "Normal"
	if len(blendMode) > 0 && blendMode[0] != "" {
		bm = blendMode[0]
	}
	d.currentBlendMode = bm

	// Register the alpha state (dedup by value + blend mode).
	key := fmt.Sprintf("%.3f %s", alpha, bm)
	entry, ok := d.alphaByKey[key]
	if !ok {
		entry = &alphaEntry{
			alpha:     alpha,
			blendMode: bm,
			name:      fmt.Sprintf("GS%d", len(d.alphaStates)+1),
		}
		d.alphaStates = append(d.alphaStates, entry)
		d.alphaByKey[key] = entry
	}

	// Emit the gs operator on the current page.
	if d.currentPage != nil {
		d.currentPage.stream.SetExtGState(entry.name)
	}
}

// SetCharSpacing sets extra space (in points) inserted between each character
// (PDF Tc operator). A value of 0 restores normal spacing.
func (d *Document) SetCharSpacing(spacing float64) {
	d.charSpacing = spacing
	if d.currentPage != nil {
		d.currentPage.stream.SetCharSpacing(spacing)
	}
}

// SetWordSpacing sets extra space (in points) added to every ASCII space
// character (PDF Tw operator). A value of 0 restores normal spacing.
func (d *Document) SetWordSpacing(spacing float64) {
	d.wordSpacing = spacing
	if d.currentPage != nil {
		d.currentPage.stream.SetWordSpacing(spacing)
	}
}

// SetTextRise shifts the text baseline vertically by the given amount in
// points (PDF Ts operator). Positive values move text up (superscript),
// negative values move text down (subscript). A value of 0 restores normal.
func (d *Document) SetTextRise(rise float64) {
	d.textRise = rise
	if d.currentPage != nil {
		d.currentPage.stream.SetTextRise(rise)
	}
}

// gradientEntry holds a registered gradient shading definition.
type gradientEntry struct {
	name   string  // resource name: "Sh1", "Sh2", ...
	gtype  int     // PDF shading type: 2=axial (linear), 3=radial
	x0, y0 float64 // start point (or center for radial) in PDF points
	x1, y1 float64 // end point (or edge center for radial) in PDF points
	r0, r1 float64 // start/end radii (radial only) in PDF points
	colors []gradientStop
	objNum int // set during serialization
}

// gradientStop defines a color at a position along a gradient (0.0–1.0).
type gradientStop struct {
	pos     float64
	r, g, b float64 // 0.0–1.0
}

// GradientStop creates a gradient color stop at the given position (0.0–1.0)
// with RGB values (0–255).
func GradientStop(pos float64, r, g, b int) gradientStop {
	return gradientStop{
		pos: pos,
		r:   float64(r) / 255.0,
		g:   float64(g) / 255.0,
		b:   float64(b) / 255.0,
	}
}

// AddBookmark adds a bookmark (PDF outline entry) at the current cursor
// position of the current page. level controls nesting: 0 = top-level,
// 1 = child of the most recent level-0 bookmark, and so on.
// Bookmarks appear in the navigation sidebar of PDF viewers.
func (d *Document) AddBookmark(title string, level int) {
	if d.currentPage == nil {
		return
	}
	d.outlines = append(d.outlines, &outlineEntry{
		title: title,
		level: level,
		page:  d.currentPage,
		y:     d.currentPage.y,
	})
}

// SetProtection enables password protection on the PDF.
// userPw is required to open the document (empty string = no open password).
// ownerPw is required to change permissions.
// permissions is a bitmask: PermPrint (4), PermModify (8), PermCopy (16), PermAnnotate (32).
// Use PermAll for all permissions. When only ownerPw is set, the document
// opens freely but modifications require the owner password.
func (d *Document) SetProtection(userPw, ownerPw string, permissions int) {
	d.encrypted = true
	d.userPw = userPw
	d.ownerPw = ownerPw
	// Standard permission value: must set reserved high bits per PDF spec.
	d.permissions = int32(permissions) | int32(-4)
}

// SetProtectionAES256 sets AES-256 password protection on the document.
// This uses the PDF 2.0 standard security handler (V=5, R=6) with
// AES-256-CBC encryption, which is significantly stronger than the
// RC4-based SetProtection.
func (d *Document) SetProtectionAES256(userPw, ownerPw string, permissions int) {
	d.encrypted = true
	d.encryptAES = true
	d.userPw = userPw
	d.ownerPw = ownerPw
	d.permissions = int32(permissions) | int32(-4)
}

// Permission flag constants for PDF standard security handler.
const (
	PermPrint    = 1 << 2
	PermModify   = 1 << 3
	PermCopy     = 1 << 4
	PermAnnotate = 1 << 5
	PermAll      = PermPrint | PermModify | PermCopy | PermAnnotate
)

// SetUnderline enables or disables underlining for subsequent text.
func (d *Document) SetUnderline(on bool) { d.underline = on }

// SetStrikethrough enables or disables strikethrough for subsequent text.
func (d *Document) SetStrikethrough(on bool) { d.strikethrough = on }

// SetUnderlineThickness sets a multiplier for the underline weight.
// The default is 1.0. Values > 1 make the underline thicker, < 1 thinner.
func (d *Document) SetUnderlineThickness(thickness float64) {
	if thickness <= 0 {
		thickness = 1.0
	}
	d.underlineThickness = thickness
}

// SetTextRenderingMode sets the PDF text rendering mode on the current page.
// Common modes: 0 = fill (default), 1 = stroke, 2 = fill+stroke,
// 3 = invisible, 4 = fill+clip, 5 = stroke+clip, 6 = fill+stroke+clip,
// 7 = clip only.
func (d *Document) SetTextRenderingMode(mode int) {
	if mode < 0 || mode > 7 {
		return
	}
	if d.currentPage != nil {
		d.currentPage.stream.SetTextRendering(mode)
	}
}

// SetCellMargin sets the cell margin (padding around text inside cells)
// in user units.
func (d *Document) SetCellMargin(margin float64) {
	d.cMargin = margin
}

// GetCellMargin returns the current cell margin in user units.
func (d *Document) GetCellMargin() float64 {
	return d.cMargin
}

// SetLineWidth sets the line width in user units.
func (d *Document) SetLineWidth(w float64) {
	d.lineWidth = w
	if d.currentPage != nil {
		d.currentPage.stream.SetLineWidth(w * d.k)
	}
}

// SetLineCapStyle sets the line cap style for subsequent drawing.
// style: "butt" (0), "round" (1), or "square" (2).
func (d *Document) SetLineCapStyle(style string) {
	var v int
	switch style {
	case "butt":
		v = 0
	case "round":
		v = 1
	case "square":
		v = 2
	default:
		return
	}
	d.capStyle = v
	if d.currentPage != nil {
		d.currentPage.stream.SetLineCap(v)
	}
}

// SetLineJoinStyle sets the line join style for subsequent drawing.
// style: "miter" (0), "round" (1), or "bevel" (2).
func (d *Document) SetLineJoinStyle(style string) {
	var v int
	switch style {
	case "miter":
		v = 0
	case "round":
		v = 1
	case "bevel":
		v = 2
	default:
		return
	}
	d.joinStyle = v
	if d.currentPage != nil {
		d.currentPage.stream.SetLineJoin(v)
	}
}

// GetLineWidth returns the current line width in user units.
func (d *Document) GetLineWidth() float64 {
	return d.lineWidth
}

// GetDrawColor returns the current stroke color as 0-255 RGB values.
func (d *Document) GetDrawColor() (int, int, int) {
	return int(d.drawColor.R*255 + 0.5), int(d.drawColor.G*255 + 0.5), int(d.drawColor.B*255 + 0.5)
}

// GetFillColor returns the current fill color as 0-255 RGB values.
func (d *Document) GetFillColor() (int, int, int) {
	return int(d.fillColor.R*255 + 0.5), int(d.fillColor.G*255 + 0.5), int(d.fillColor.B*255 + 0.5)
}

// GetTextColor returns the current text color as 0-255 RGB values.
func (d *Document) GetTextColor() (int, int, int) {
	return int(d.textColor.R*255 + 0.5), int(d.textColor.G*255 + 0.5), int(d.textColor.B*255 + 0.5)
}

// GetAlpha returns the current opacity (0.0 = fully transparent, 1.0 = fully opaque).
func (d *Document) GetAlpha() float64 {
	return d.currentAlpha
}

// SetPage switches the active drawing target to the given page number (1-based).
// This allows drawing on a previously created page. The cursor position is
// preserved from the last time that page was active.
func (d *Document) SetPage(pageNum int) {
	if d.err != nil {
		return
	}
	if pageNum < 1 || pageNum > len(d.pages) {
		d.err = fmt.Errorf("SetPage: page %d out of range (1–%d)", pageNum, len(d.pages))
		return
	}
	d.currentPage = d.pages[pageNum-1]
}

// SetPageBox sets a named page box on the specified page (1-based) or on
// all future pages (if pageNum is 0). Supported box types: "TrimBox",
// "CropBox", "BleedBox", "ArtBox". Coordinates are in user units relative
// to the top-left corner.
func (d *Document) SetPageBox(boxType string, pageNum int, x, y, wd, ht float64) {
	if d.err != nil {
		return
	}
	// Normalize box type name
	switch boxType {
	case "trim", "TrimBox":
		boxType = "TrimBox"
	case "crop", "CropBox":
		boxType = "CropBox"
	case "bleed", "BleedBox":
		boxType = "BleedBox"
	case "art", "ArtBox":
		boxType = "ArtBox"
	default:
		d.err = fmt.Errorf("SetPageBox: unknown box type %q", boxType)
		return
	}
	// Convert to PDF points (bottom-left origin)
	k := d.k
	var pageH float64
	if pageNum > 0 && pageNum <= len(d.pages) {
		pageH = d.pages[pageNum-1].size.HeightPt
	} else if pageNum == 0 {
		pageH = d.defSize.HeightPt
	} else {
		d.err = fmt.Errorf("SetPageBox: page %d out of range", pageNum)
		return
	}
	llx := x * k
	lly := pageH - (y+ht)*k
	urx := (x + wd) * k
	ury := pageH - y*k
	box := [4]float64{llx, lly, urx, ury}

	if pageNum == 0 {
		// Set as default for future pages
		if d.defPageBoxes == nil {
			d.defPageBoxes = make(map[string][4]float64)
		}
		d.defPageBoxes[boxType] = box
	} else {
		p := d.pages[pageNum-1]
		if p.pageBoxes == nil {
			p.pageBoxes = make(map[string][4]float64)
		}
		p.pageBoxes[boxType] = box
	}
}

// SetAcceptPageBreakFunc sets a custom function that controls automatic page
// breaking. When set, this function is called instead of the default behavior
// to determine if a page break should occur. Return true to allow the break,
// false to suppress it.
func (d *Document) SetAcceptPageBreakFunc(fn func() bool) {
	d.acceptPageBreakFunc = fn
}

// GetPageSize returns the width and height of the current page in user units.
func (d *Document) GetPageSize() (float64, float64) {
	if d.currentPage == nil {
		return d.defSize.WidthPt / d.k, d.defSize.HeightPt / d.k
	}
	return d.currentPage.w, d.currentPage.h
}

// GetMargins returns the current left, top, right, and bottom margins
// in user units.
func (d *Document) GetMargins() (left, top, right, bottom float64) {
	return d.lMargin, d.tMargin, d.rMargin, d.bMargin
}

// --- Layers (Optional Content Groups) ---

// AddLayer creates a new optional content group (layer).
// name is the display name; visible controls initial visibility.
// Returns a layer ID for use with BeginLayer.
func (d *Document) AddLayer(name string, visible bool) int {
	id := len(d.layers)
	d.layers = append(d.layers, layerEntry{name: name, visible: visible})
	return id
}

// BeginLayer starts drawing content in the specified layer. All drawing
// operations until EndLayer will be part of this layer. Layers can be
// toggled on/off in PDF viewers that support Optional Content Groups.
func (d *Document) BeginLayer(id int) {
	if d.err != nil || d.currentPage == nil {
		return
	}
	if id < 0 || id >= len(d.layers) {
		return
	}
	if d.currentLayer >= 0 {
		d.EndLayer()
	}
	d.currentLayer = id
	d.currentPage.stream.BeginOptionalContent(fmt.Sprintf("OC%d", id))
}

// EndLayer ends the current layer started by BeginLayer.
func (d *Document) EndLayer() {
	if d.currentLayer < 0 || d.currentPage == nil {
		return
	}
	d.currentPage.stream.EndMarkedContent()
	d.currentLayer = -1
}

// OpenLayerPane causes the PDF viewer to open the layers panel when
// the document is opened.
func (d *Document) OpenLayerPane() {
	d.openLayerPane = true
}

// --- Spot Colors ---

// AddSpotColor registers a named spot color with CMYK values (0–100 each).
// Use the name with SetDrawSpotColor, SetFillSpotColor, or SetTextSpotColor.
func (d *Document) AddSpotColor(name string, c, m, y, k int) {
	if d.err != nil {
		return
	}
	if _, exists := d.spotColorMap[name]; exists {
		d.err = fmt.Errorf("AddSpotColor: %q already registered", name)
		return
	}
	idx := len(d.spotColors)
	d.spotColors = append(d.spotColors, spotColorEntry{
		name: name,
		c:    float64(clampByte(c)) / 100.0,
		m:    float64(clampByte(m)) / 100.0,
		y:    float64(clampByte(y)) / 100.0,
		k:    float64(clampByte(k)) / 100.0,
	})
	d.spotColorMap[name] = idx
}

// clampByte clamps v to 0–100 range.
func clampByte(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// SetDrawSpotColor sets the stroke color to a registered spot color.
// tint ranges from 0 (no ink) to 100 (full ink).
func (d *Document) SetDrawSpotColor(name string, tint int) {
	if d.err != nil {
		return
	}
	idx, ok := d.spotColorMap[name]
	if !ok {
		d.err = fmt.Errorf("SetDrawSpotColor: spot color %q not registered", name)
		return
	}
	if d.currentPage != nil {
		d.currentPage.stream.Raw(fmt.Sprintf("/CS%d CS %.3f SCN", idx+1, float64(clampByte(tint))/100.0))
	}
}

// SetFillSpotColor sets the fill color to a registered spot color.
// tint ranges from 0 (no ink) to 100 (full ink).
func (d *Document) SetFillSpotColor(name string, tint int) {
	if d.err != nil {
		return
	}
	idx, ok := d.spotColorMap[name]
	if !ok {
		d.err = fmt.Errorf("SetFillSpotColor: spot color %q not registered", name)
		return
	}
	if d.currentPage != nil {
		d.currentPage.stream.Raw(fmt.Sprintf("/CS%d cs %.3f scn", idx+1, float64(clampByte(tint))/100.0))
	}
}

// SetTextSpotColor sets the text color to a registered spot color.
// tint ranges from 0 (no ink) to 100 (full ink).
func (d *Document) SetTextSpotColor(name string, tint int) {
	if d.err != nil {
		return
	}
	idx, ok := d.spotColorMap[name]
	if !ok {
		d.err = fmt.Errorf("SetTextSpotColor: spot color %q not registered", name)
		return
	}
	if d.currentPage != nil {
		d.currentPage.stream.Raw(fmt.Sprintf("/CS%d cs %.3f scn", idx+1, float64(clampByte(tint))/100.0))
	}
}

// --- Attachments ---

// SetAttachments sets document-level file attachments. These are embedded
// in the PDF and appear in the viewer's attachment panel.
func (d *Document) SetAttachments(attachments []Attachment) {
	d.attachments = attachments
}

// AddAttachmentAnnotation adds a file attachment annotation on the current
// page at the specified rectangle (in user units). The attachment appears
// as a clickable icon.
func (d *Document) AddAttachmentAnnotation(a Attachment, x, y, w, h float64) {
	if d.err != nil || d.currentPage == nil {
		return
	}
	a.objNum = 0 // will be set during serialization
	d.currentPage.attachAnnotations = append(d.currentPage.attachAnnotations, attachAnnotation{
		attachment: a,
		x:         x,
		y:         y,
		w:         w,
		h:         h,
	})
}

// --- Integer-based Internal Links ---

// AddLink creates a new internal link placeholder and returns its ID.
// The link's destination must be set later with SetLink.
func (d *Document) AddLink() int {
	d.intLinks = append(d.intLinks, linkDest{})
	return len(d.intLinks) // 1-based
}

// SetLink sets the destination for an internal link created by AddLink.
// linkID is the value returned by AddLink. y is the target Y position
// in user units; use -1 for the current position. page is the 1-based
// page number; use 0 for the current page.
func (d *Document) SetLink(linkID int, y float64, page int) {
	if linkID < 1 || linkID > len(d.intLinks) {
		return
	}
	idx := linkID - 1
	if y == -1 && d.currentPage != nil {
		y = d.currentPage.y
	}
	if page == 0 {
		if d.currentPage != nil {
			d.intLinks[idx] = linkDest{page: d.currentPage, y: y}
		}
	} else if page >= 1 && page <= len(d.pages) {
		d.intLinks[idx] = linkDest{page: d.pages[page-1], y: y}
	}
}

// --- Catalog Sort ---

// SetCatalogSort enables or disables deterministic sorting of internal
// resource catalog keys. When enabled, PDF output is reproducible across
// runs (given the same inputs and fixed creation/modification dates).
// This is useful for testing and version control of generated PDFs.
func (d *Document) SetCatalogSort(flag bool) {
	d.catalogSort = flag
}

// --- RTL Text ---

// RTL enables right-to-left text mode. Text rendered with Cell, Write,
// and MultiCell will have its rune order reversed.
func (d *Document) RTL() { d.isRTL = true }

// LTR disables right-to-left text mode (the default).
func (d *Document) LTR() { d.isRTL = false }

// --- Raw PDF Access ---

// GetConversionRatio returns the conversion factor from user units to PDF
// points. Multiply a user-unit value by this ratio to get points.
func (d *Document) GetConversionRatio() float64 {
	return d.k
}

// RegisterImageFromFile registers an image from a file path.
// The format is auto-detected from the file contents.
func (d *Document) RegisterImageFromFile(name, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("RegisterImageFromFile: %w", err)
	}
	defer f.Close()
	return d.RegisterImage(name, f)
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

	// Apply default page boxes if set
	if len(d.defPageBoxes) > 0 {
		if p.pageBoxes == nil {
			p.pageBoxes = make(map[string][4]float64, len(d.defPageBoxes))
		}
		for k, v := range d.defPageBoxes {
			p.pageBoxes[k] = v
		}
	}

	// Emit initial page state
	p.stream.SetLineWidth(d.lineWidth * d.k)
	p.stream.SetLineCap(d.capStyle)
	p.stream.SetLineJoin(d.joinStyle)

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

	// Apply alpha if non-default
	if d.currentAlpha != 1.0 || d.currentBlendMode != "Normal" {
		key := fmt.Sprintf("%.3f %s", d.currentAlpha, d.currentBlendMode)
		if entry, ok := d.alphaByKey[key]; ok {
			p.stream.SetExtGState(entry.name)
		}
	}

	// Apply typography state if non-default
	if d.charSpacing != 0 {
		p.stream.SetCharSpacing(d.charSpacing)
	}
	if d.wordSpacing != 0 {
		p.stream.SetWordSpacing(d.wordSpacing)
	}
	if d.textRise != 0 {
		p.stream.SetTextRise(d.textRise)
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
