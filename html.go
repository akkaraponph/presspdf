package presspdf

import (
	"strconv"
	"strings"
)

// HTMLOption configures the HTML renderer.
type HTMLOption func(*htmlRenderer)

// WithHTMLWidth sets the content width for HTML rendering.
// If zero (default), the available width between margins is used.
func WithHTMLWidth(w float64) HTMLOption {
	return func(r *htmlRenderer) {
		r.contentWidth = w
	}
}

// WithHTMLLineHeight sets the default line height for HTML rendering.
// Default is 6.0 (in user units, typically mm).
func WithHTMLLineHeight(h float64) HTMLOption {
	return func(r *htmlRenderer) {
		r.lineHeight = h
	}
}

// HTML renders a subset of HTML onto the page starting at the current
// cursor position.
//
// Supported tags:
//   - Headings: <h1> through <h6>
//   - Block elements: <p>, <div>, <blockquote>, <center>, <pre>
//   - Inline formatting: <b>/<strong>, <i>/<em>, <u>, <s>/<del>/<strike>, <small>, <mark>, <code>, <span>
//   - Text positioning: <sup>, <sub>, <br>, <hr>
//   - Lists: <ul>/<ol>/<li> (including nested lists)
//   - Tables: <table>/<thead>/<tbody>/<tr>/<td>/<th>
//   - Links: <a href="...">
//   - Legacy: <font color="..." size="..." face="...">
//   - Stylesheet: <style> with element, class (.cls), and ID (#id) selectors
//
// Supported inline CSS (via style attribute):
//
//	color, background-color, font-size, font-family, font-weight,
//	font-style, text-align, text-decoration, line-height,
//	margin-top, margin-bottom, padding-left.
func (p *Page) HTML(html string, opts ...HTMLOption) {
	p = p.active()
	doc := p.doc
	r := &htmlRenderer{
		page:       p,
		lineHeight: 6,
	}
	for _, opt := range opts {
		opt(r)
	}
	if r.contentWidth == 0 {
		r.contentWidth = p.w - doc.lMargin - doc.rMargin
	}

	nodes := parseHTML(html)

	// Unwrap document structure: skip <!doctype>, <html>, <head>, <body>.
	nodes = unwrapDocument(nodes)

	// Set parent pointers for descendant selector matching.
	setParentPointers(nodes, nil)

	// Extract <style> blocks from the full tree before rendering.
	r.extractStyles(nodes)

	r.renderNodes(nodes)
}

// htmlNode represents a parsed HTML element or text node.
type htmlNode struct {
	tag      string
	attrs    map[string]string
	children []*htmlNode
	text     string // non-empty for text nodes
	parent   *htmlNode
}

// htmlRenderer holds rendering state.
type htmlRenderer struct {
	page         *Page
	lineHeight   float64
	contentWidth float64 // available width for content
	listDepth    int     // nesting depth for lists
	inheritAlign string  // inherited text-align for child elements

	// stylesheet parsed from <style> blocks
	stylesheet map[string]cssStyle // selector -> style
}

// --- HTML Parser ---

// parseHTML parses an HTML string into a tree of nodes.
// This is a minimal parser supporting the subset of tags we render.
func parseHTML(s string) []*htmlNode {
	p := &htmlParser{input: s}
	return p.parseNodes()
}

// unwrapDocument flattens document structure tags like <!doctype>, <html>,
// <head>, and <body> so the renderer sees content nodes directly.
// <head> content (except <style>) is discarded.
func unwrapDocument(nodes []*htmlNode) []*htmlNode {
	var result []*htmlNode
	for _, n := range nodes {
		switch n.tag {
		case "!doctype":
			// Skip entirely.
			continue
		case "html":
			// Recurse into children.
			result = append(result, unwrapDocument(n.children)...)
		case "head":
			// Keep only <style> children from <head>.
			for _, child := range n.children {
				if child.tag == "style" {
					result = append(result, child)
				}
			}
		case "body":
			// Unwrap body: use its children directly.
			result = append(result, n.children...)
		default:
			result = append(result, n)
		}
	}
	return result
}

// setParentPointers recursively sets parent references on all nodes.
func setParentPointers(nodes []*htmlNode, parent *htmlNode) {
	for _, n := range nodes {
		n.parent = parent
		setParentPointers(n.children, n)
	}
}

// extractStyles recursively finds and processes all <style> nodes in the tree.
func (r *htmlRenderer) extractStyles(nodes []*htmlNode) {
	for i := 0; i < len(nodes); i++ {
		n := nodes[i]
		if n.tag == "style" {
			r.parseStyleBlock(n)
			// Remove <style> from the tree so it isn't rendered.
			nodes[i] = &htmlNode{text: ""}
			continue
		}
		if len(n.children) > 0 {
			r.extractStyles(n.children)
		}
	}
}

type htmlParser struct {
	input string
	pos   int
}

func (p *htmlParser) parseNodes() []*htmlNode {
	var nodes []*htmlNode
	for p.pos < len(p.input) {
		if p.input[p.pos] == '<' {
			// Check for comment <!-- ... -->
			if p.pos+3 < len(p.input) && p.input[p.pos:p.pos+4] == "<!--" {
				p.skipComment()
				continue
			}
			// Check for closing tag.
			if p.pos+1 < len(p.input) && p.input[p.pos+1] == '/' {
				break // let parent handle the closing tag
			}
			node := p.parseElement()
			if node != nil {
				nodes = append(nodes, node)
			}
		} else {
			text := p.parseText()
			if text != "" {
				nodes = append(nodes, &htmlNode{text: text})
			}
		}
	}
	return nodes
}

func (p *htmlParser) parseText() string {
	start := p.pos
	for p.pos < len(p.input) && p.input[p.pos] != '<' {
		p.pos++
	}
	text := p.input[start:p.pos]
	text = decodeEntities(text)
	return text
}

// decodeEntities replaces HTML entities with their character equivalents.
func decodeEntities(text string) string {
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&apos;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", "\u00a0")
	text = strings.ReplaceAll(text, "&mdash;", "\u2014")
	text = strings.ReplaceAll(text, "&ndash;", "\u2013")
	text = strings.ReplaceAll(text, "&bull;", "\u2022")
	text = strings.ReplaceAll(text, "&hellip;", "\u2026")
	text = strings.ReplaceAll(text, "&copy;", "\u00a9")
	text = strings.ReplaceAll(text, "&reg;", "\u00ae")
	text = strings.ReplaceAll(text, "&trade;", "\u2122")
	// Numeric character references: &#NNN; and &#xHH;
	for {
		idx := strings.Index(text, "&#")
		if idx < 0 {
			break
		}
		end := strings.IndexByte(text[idx:], ';')
		if end < 0 {
			break
		}
		ref := text[idx+2 : idx+end]
		var r rune
		if len(ref) > 0 && (ref[0] == 'x' || ref[0] == 'X') {
			n, err := strconv.ParseInt(ref[1:], 16, 32)
			if err != nil {
				break
			}
			r = rune(n)
		} else {
			n, err := strconv.ParseInt(ref, 10, 32)
			if err != nil {
				break
			}
			r = rune(n)
		}
		text = text[:idx] + string(r) + text[idx+end+1:]
	}
	return text
}

func (p *htmlParser) parseElement() *htmlNode {
	if p.pos >= len(p.input) || p.input[p.pos] != '<' {
		return nil
	}
	p.pos++ // skip '<'

	// Read tag name.
	tag := p.readWord()
	tag = strings.ToLower(tag)

	// Read attributes.
	attrs := p.readAttrs()

	// Self-closing tag?
	selfClose := false
	p.skipWhitespace()
	if p.pos < len(p.input) && p.input[p.pos] == '/' {
		selfClose = true
		p.pos++
	}

	// Skip '>'.
	if p.pos < len(p.input) && p.input[p.pos] == '>' {
		p.pos++
	}

	node := &htmlNode{tag: tag, attrs: attrs}

	// Void elements (no closing tag).
	if selfClose || isVoidElement(tag) {
		return node
	}

	// For <style> and <pre>, read raw content differently.
	if tag == "style" {
		node.children = []*htmlNode{{text: p.readRawContent(tag)}}
		return node
	}

	// Parse children until closing tag.
	node.children = p.parseNodes()

	// Consume closing tag </tag>.
	p.consumeClosingTag(tag)

	return node
}

// readRawContent reads text until </tag> without parsing child elements.
func (p *htmlParser) readRawContent(tag string) string {
	closer := "</" + tag + ">"
	idx := strings.Index(strings.ToLower(p.input[p.pos:]), closer)
	if idx < 0 {
		// No closing tag found, consume rest.
		s := p.input[p.pos:]
		p.pos = len(p.input)
		return s
	}
	s := p.input[p.pos : p.pos+idx]
	p.pos += idx
	p.consumeClosingTag(tag)
	return s
}

func (p *htmlParser) readWord() string {
	start := p.pos
	for p.pos < len(p.input) && !isHTMLDelim(p.input[p.pos]) {
		p.pos++
	}
	return p.input[start:p.pos]
}

func (p *htmlParser) readAttrs() map[string]string {
	attrs := make(map[string]string)
	for {
		p.skipWhitespace()
		if p.pos >= len(p.input) || p.input[p.pos] == '>' || p.input[p.pos] == '/' {
			break
		}
		name := p.readWord()
		if name == "" {
			break
		}
		name = strings.ToLower(name)

		p.skipWhitespace()
		if p.pos < len(p.input) && p.input[p.pos] == '=' {
			p.pos++ // skip '='
			p.skipWhitespace()
			val := p.readAttrValue()
			attrs[name] = val
		} else {
			attrs[name] = name // boolean attribute
		}
	}
	return attrs
}

func (p *htmlParser) readAttrValue() string {
	if p.pos >= len(p.input) {
		return ""
	}
	quote := p.input[p.pos]
	if quote == '"' || quote == '\'' {
		p.pos++ // skip opening quote
		start := p.pos
		for p.pos < len(p.input) && p.input[p.pos] != quote {
			p.pos++
		}
		val := p.input[start:p.pos]
		if p.pos < len(p.input) {
			p.pos++ // skip closing quote
		}
		return val
	}
	// Unquoted value.
	return p.readWord()
}

func (p *htmlParser) consumeClosingTag(tag string) {
	if p.pos+2 >= len(p.input) || p.input[p.pos] != '<' || p.input[p.pos+1] != '/' {
		return
	}
	// Skip past </tag>
	end := strings.IndexByte(p.input[p.pos:], '>')
	if end >= 0 {
		p.pos += end + 1
	}
}

func (p *htmlParser) skipWhitespace() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t' || p.input[p.pos] == '\n' || p.input[p.pos] == '\r') {
		p.pos++
	}
}

func (p *htmlParser) skipComment() {
	p.pos += 4 // skip "<!--"
	end := strings.Index(p.input[p.pos:], "-->")
	if end >= 0 {
		p.pos += end + 3
	} else {
		p.pos = len(p.input)
	}
}

func isHTMLDelim(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '>' || b == '/' || b == '='
}

func isVoidElement(tag string) bool {
	switch tag {
	case "br", "hr", "img", "meta", "link", "input":
		return true
	}
	// Treat <!DOCTYPE> and similar declarations as void.
	if len(tag) > 0 && tag[0] == '!' {
		return true
	}
	return false
}

// --- Stylesheet parsing ---

// parseStyleBlock extracts CSS rules from a <style> element.
func (r *htmlRenderer) parseStyleBlock(n *htmlNode) {
	if r.stylesheet == nil {
		r.stylesheet = make(map[string]cssStyle)
	}
	for _, child := range n.children {
		if child.text == "" {
			continue
		}
		r.parseCSS(child.text)
	}
}

// parseCSS parses a CSS string into the renderer's stylesheet.
// Supports simple selectors: element (p, h1), class (.cls), ID (#id).
func (r *htmlRenderer) parseCSS(css string) {
	// Strip CSS comments.
	for {
		start := strings.Index(css, "/*")
		if start < 0 {
			break
		}
		end := strings.Index(css[start:], "*/")
		if end < 0 {
			css = css[:start]
			break
		}
		css = css[:start] + css[start+end+2:]
	}

	// Parse rules: selector { declarations }
	for {
		css = strings.TrimSpace(css)
		if css == "" {
			break
		}
		braceOpen := strings.IndexByte(css, '{')
		if braceOpen < 0 {
			break
		}
		braceClose := strings.IndexByte(css[braceOpen:], '}')
		if braceClose < 0 {
			break
		}
		braceClose += braceOpen

		selector := strings.TrimSpace(css[:braceOpen])
		body := css[braceOpen+1 : braceClose]
		css = css[braceClose+1:]

		style := parseInlineStyle(body)

		// Handle comma-separated selectors.
		for _, sel := range strings.Split(selector, ",") {
			sel = strings.TrimSpace(sel)
			if sel != "" {
				r.stylesheet[sel] = style
			}
		}
	}
}

// resolveStyle computes the effective CSS style for a node by merging
// stylesheet rules and inline style. Inline style wins.
func (r *htmlRenderer) resolveStyle(n *htmlNode) cssStyle {
	var merged cssStyle

	if r.stylesheet != nil {
		// Match by tag.
		if s, ok := r.stylesheet[n.tag]; ok {
			merged = mergeCSS(merged, s)
		}
		// Match by class.
		if classes, ok := n.attrs["class"]; ok {
			for _, cls := range strings.Fields(classes) {
				if s, ok := r.stylesheet["."+cls]; ok {
					merged = mergeCSS(merged, s)
				}
			}
		}
		// Match by ID.
		if id, ok := n.attrs["id"]; ok {
			if s, ok := r.stylesheet["#"+id]; ok {
				merged = mergeCSS(merged, s)
			}
		}

		// Match descendant selectors (e.g. ".company-info h1").
		for sel, s := range r.stylesheet {
			parts := strings.Fields(sel)
			if len(parts) < 2 {
				continue
			}
			// Check if the node matches the last part of the selector.
			last := parts[len(parts)-1]
			if !nodeMatchesSelector(n, last) {
				continue
			}
			// Walk up ancestors to match remaining parts right-to-left.
			if matchDescendantSelector(n.parent, parts[:len(parts)-1]) {
				merged = mergeCSS(merged, s)
			}
		}
	}

	// Inline style overrides.
	inline := parseInlineStyle(n.attrs["style"])
	merged = mergeCSS(merged, inline)

	// Apply inherited text-align if not explicitly set.
	if merged.textAlign == "" && r.inheritAlign != "" {
		merged.textAlign = r.inheritAlign
	}

	return merged
}

// nodeMatchesSelector checks if a node matches a simple CSS selector
// (tag name, .class, or #id).
func nodeMatchesSelector(n *htmlNode, sel string) bool {
	if n == nil || n.tag == "" {
		return false
	}
	if strings.HasPrefix(sel, ".") {
		cls := sel[1:]
		for _, c := range strings.Fields(n.attrs["class"]) {
			if c == cls {
				return true
			}
		}
		return false
	}
	if strings.HasPrefix(sel, "#") {
		return n.attrs["id"] == sel[1:]
	}
	return n.tag == sel
}

// matchDescendantSelector walks up the ancestor chain to match remaining
// selector parts. Each part must match some ancestor (not necessarily direct parent).
func matchDescendantSelector(ancestor *htmlNode, parts []string) bool {
	if len(parts) == 0 {
		return true
	}
	for cur := ancestor; cur != nil; cur = cur.parent {
		if nodeMatchesSelector(cur, parts[len(parts)-1]) {
			if matchDescendantSelector(cur.parent, parts[:len(parts)-1]) {
				return true
			}
		}
	}
	return false
}

// mergeCSS merges src into dst; non-zero values in src override dst.
func mergeCSS(dst, src cssStyle) cssStyle {
	if src.hasColor {
		dst.color = src.color
		dst.hasColor = true
	}
	if src.fontSize > 0 {
		dst.fontSize = src.fontSize
	}
	if src.fontFamily != "" {
		dst.fontFamily = src.fontFamily
	}
	if src.fontWeight != "" {
		dst.fontWeight = src.fontWeight
	}
	if src.fontStyleCSS != "" {
		dst.fontStyleCSS = src.fontStyleCSS
	}
	if src.textDecoration != "" {
		dst.textDecoration = src.textDecoration
	}
	if src.textAlign != "" {
		dst.textAlign = src.textAlign
	}
	if src.lineHeight > 0 {
		dst.lineHeight = src.lineHeight
	}
	if src.hasBgColor {
		dst.backgroundColor = src.backgroundColor
		dst.hasBgColor = true
	}
	if src.marginTop != 0 {
		dst.marginTop = src.marginTop
	}
	if src.marginBottom != 0 {
		dst.marginBottom = src.marginBottom
	}
	if src.paddingTop != 0 {
		dst.paddingTop = src.paddingTop
	}
	if src.paddingLeft != 0 {
		dst.paddingLeft = src.paddingLeft
	}
	if src.paddingBottom != 0 {
		dst.paddingBottom = src.paddingBottom
	}
	if src.textTransform != "" {
		dst.textTransform = src.textTransform
	}
	if src.borderTop {
		dst.borderTop = true
		dst.borderTopWidth = src.borderTopWidth
		dst.borderTopColor = src.borderTopColor
	}
	if src.borderBottom {
		dst.borderBottom = true
		dst.borderBottomWidth = src.borderBottomWidth
		dst.borderBottomColor = src.borderBottomColor
	}
	if src.display != "" {
		dst.display = src.display
	}
	if src.justifyContent != "" {
		dst.justifyContent = src.justifyContent
	}
	if src.float_ != "" {
		dst.float_ = src.float_
	}
	if src.widthPct != 0 {
		dst.widthPct = src.widthPct
	}
	if src.clear_ != "" {
		dst.clear_ = src.clear_
	}
	return dst
}

// --- Rendering ---

// renderNodes renders a list of HTML nodes.
func (r *htmlRenderer) renderNodes(nodes []*htmlNode) {
	for _, n := range nodes {
		if n.text != "" {
			r.renderText(n.text)
			continue
		}
		r.renderElement(n)
	}
}

func (r *htmlRenderer) renderText(text string) {
	// Collapse whitespace like a browser.
	text = collapseWhitespace(text)
	if text == "" {
		return
	}
	p := r.page.active()
	p.Write(r.lineHeight, text)
}

func collapseWhitespace(s string) string {
	var buf strings.Builder
	lastWasSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !lastWasSpace {
				buf.WriteByte(' ')
				lastWasSpace = true
			}
		} else {
			buf.WriteRune(r)
			lastWasSpace = false
		}
	}
	return buf.String()
}

// styleSnapshot captures the current document style state for later restoration.
type styleSnapshot struct {
	fontFamily    string
	fontStyle     string
	fontSizePt    float64
	textColor     [3]int
	fillColor     [3]int
	underline     bool
	strikethrough bool
	textRise      float64
	lineHeight    float64
}

func (r *htmlRenderer) saveStyle() styleSnapshot {
	doc := r.page.doc
	return styleSnapshot{
		fontFamily: doc.fontFamily,
		fontStyle:  doc.fontStyle,
		fontSizePt: doc.fontSizePt,
		textColor: [3]int{
			int(doc.textColor.R * 255),
			int(doc.textColor.G * 255),
			int(doc.textColor.B * 255),
		},
		fillColor: [3]int{
			int(doc.fillColor.R * 255),
			int(doc.fillColor.G * 255),
			int(doc.fillColor.B * 255),
		},
		underline:     doc.underline,
		strikethrough: doc.strikethrough,
		textRise:      doc.textRise,
		lineHeight:    r.lineHeight,
	}
}

func (r *htmlRenderer) restoreSnapshot(ss styleSnapshot) {
	doc := r.page.doc
	if doc.fontFamily != ss.fontFamily || doc.fontStyle != ss.fontStyle || doc.fontSizePt != ss.fontSizePt {
		doc.SetFont(ss.fontFamily, ss.fontStyle, ss.fontSizePt)
	}
	doc.SetTextColor(ss.textColor[0], ss.textColor[1], ss.textColor[2])
	doc.SetFillColor(ss.fillColor[0], ss.fillColor[1], ss.fillColor[2])
	doc.SetUnderline(ss.underline)
	doc.SetStrikethrough(ss.strikethrough)
	doc.textRise = ss.textRise
	r.lineHeight = ss.lineHeight
}

// applyCSS applies a cssStyle to the current document state.
func (r *htmlRenderer) applyCSS(cs cssStyle) {
	doc := r.page.doc
	if cs.hasColor {
		doc.SetTextColor(cs.color[0], cs.color[1], cs.color[2])
	}
	if cs.fontSize > 0 {
		doc.SetFontSize(cs.fontSize)
	}
	if cs.fontFamily != "" {
		doc.SetFont(cs.fontFamily, doc.fontStyle, doc.fontSizePt)
	}
	if cs.fontWeight == "bold" {
		r.addFontStyle("B")
	} else if cs.fontWeight == "normal" {
		r.removeFontStyle("B")
	}
	if cs.fontStyleCSS == "italic" {
		r.addFontStyle("I")
	} else if cs.fontStyleCSS == "normal" {
		r.removeFontStyle("I")
	}
	if cs.textDecoration != "" {
		switch cs.textDecoration {
		case "underline":
			doc.SetUnderline(true)
		case "line-through":
			doc.SetStrikethrough(true)
		case "none":
			doc.SetUnderline(false)
			doc.SetStrikethrough(false)
		}
	}
	if cs.lineHeight > 0 {
		r.lineHeight = cs.lineHeight
	}
}

// addFontStyle merges a style flag ("B" or "I") into the current font style.
func (r *htmlRenderer) addFontStyle(flag string) {
	doc := r.page.doc
	current := doc.fontStyle
	if strings.Contains(current, flag) {
		return
	}
	merged := current + flag
	// Normalize to "BI" order.
	if strings.Contains(merged, "I") && strings.Contains(merged, "B") {
		merged = "BI"
	}
	doc.SetFontStyle(merged)
}

// removeFontStyle removes a style flag ("B" or "I") from the current font style.
func (r *htmlRenderer) removeFontStyle(flag string) {
	doc := r.page.doc
	current := doc.fontStyle
	if !strings.Contains(current, flag) {
		return
	}
	newStyle := strings.ReplaceAll(current, flag, "")
	doc.SetFontStyle(newStyle)
}

// renderElement dispatches rendering based on tag name.
func (r *htmlRenderer) renderElement(n *htmlNode) {
	ss := r.saveStyle()

	// Resolve and apply CSS (stylesheet + inline).
	cs := r.resolveStyle(n)
	r.applyCSS(cs)

	switch n.tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		r.renderHeading(n, cs)
	case "p":
		r.renderParagraph(n, cs)
	case "div":
		r.renderDiv(n, cs)
	case "span":
		r.renderNodes(n.children)
	case "b", "strong":
		r.addFontStyle("B")
		r.renderNodes(n.children)
	case "i", "em":
		r.addFontStyle("I")
		r.renderNodes(n.children)
	case "u":
		r.page.doc.SetUnderline(true)
		r.renderNodes(n.children)
	case "s", "del", "strike":
		r.page.doc.SetStrikethrough(true)
		r.renderNodes(n.children)
	case "small":
		r.renderSmall(n)
	case "mark":
		r.renderMark(n)
	case "code":
		r.renderCode(n)
	case "pre":
		r.renderPre(n)
	case "blockquote":
		r.renderBlockquote(n, cs)
	case "center":
		r.renderCenter(n)
	case "sup":
		r.renderSup(n)
	case "sub":
		r.renderSub(n)
	case "br":
		p := r.page.active()
		p.x = p.doc.lMargin
		p.y += r.lineHeight
	case "hr":
		r.renderHR()
	case "ul":
		r.renderUL(n, cs)
	case "ol":
		r.renderOL(n, cs)
	case "li":
		r.renderNodes(n.children)
	case "table":
		r.renderTable(n)
	case "a":
		r.renderLink(n)
	case "font":
		r.renderFont(n)
	case "style":
		// Already handled before rendering.
	default:
		// Unknown tag: just render children.
		r.renderNodes(n.children)
	}

	r.restoreSnapshot(ss)
}

// --- Tag renderers ---

// htmlHeadingSizes maps heading tag to font size.
var htmlHeadingSizes = map[string]float64{
	"h1": 24, "h2": 20, "h3": 16, "h4": 14, "h5": 12, "h6": 10,
}

func (r *htmlRenderer) renderHeading(n *htmlNode, cs cssStyle) {
	p := r.page.active()
	doc := p.doc

	size := htmlHeadingSizes[n.tag]
	if cs.fontSize > 0 {
		size = cs.fontSize
	}
	lh := size / doc.k * 1.4

	if cs.marginTop > 0 {
		p.y += cs.marginTop
	}

	doc.SetFont(doc.fontFamily, "B", size)

	if cs.textAlign == "center" || cs.textAlign == "right" {
		text := extractText(n)
		if cs.textTransform == "uppercase" {
			text = strings.ToUpper(text)
		}
		align := "C"
		if cs.textAlign == "right" {
			align = "R"
		}
		p = r.page.active()
		p.MultiCell(r.contentWidth, lh, text, "", align, false)
	} else {
		r.lineHeight = lh
		if cs.textTransform == "uppercase" {
			r.renderTextTransformed(n, cs)
		} else {
			r.renderNodes(n.children)
		}
		r.lineHeight = 6
		p = r.page.active()
		p.x = doc.lMargin
		p.y += lh
	}

	if cs.marginBottom > 0 {
		p = r.page.active()
		p.y += cs.marginBottom
	}

	// Render border-bottom if set.
	if cs.borderBottom {
		p = r.page.active()
		savedDraw := doc.drawColor
		doc.SetDrawColor(cs.borderBottomColor[0], cs.borderBottomColor[1], cs.borderBottomColor[2])
		savedLW := doc.lineWidth
		doc.SetLineWidth(cs.borderBottomWidth / doc.k)
		p.Line(doc.lMargin, p.y, p.w-doc.rMargin, p.y)
		doc.drawColor = savedDraw
		doc.SetLineWidth(savedLW)
		p.y += 1
	}
}

func (r *htmlRenderer) renderParagraph(n *htmlNode, cs cssStyle) {
	p := r.page.active()
	doc := p.doc

	if cs.marginTop > 0 {
		p.y += cs.marginTop
	}

	if cs.hasBgColor {
		r.drawBlockBackground(n, cs)
	}

	if cs.textAlign == "center" || cs.textAlign == "right" || cs.textAlign == "justify" {
		r.renderAlignedBlock(n, cs)
	} else if cs.textTransform != "" {
		r.renderTextTransformed(n, cs)
	} else {
		r.renderNodes(n.children)
	}

	p = r.page.active()
	p.x = doc.lMargin
	p.y += r.lineHeight

	if cs.marginBottom > 0 {
		p.y += cs.marginBottom
	}
}

func (r *htmlRenderer) renderDiv(n *htmlNode, cs cssStyle) {
	p := r.page.active()
	doc := p.doc

	// Handle clear property — in our simple model this is a no-op since we
	// don't have true float flow, but it prevents extra blank space.
	if cs.clear_ == "both" || cs.clear_ == "left" || cs.clear_ == "right" {
		// Nothing to clear in our rendering model; just continue.
		return
	}

	if cs.marginTop > 0 {
		p.y += cs.marginTop
	}

	// Render border-top if set.
	if cs.borderTop {
		p = r.page.active()
		savedDraw := doc.drawColor
		doc.SetDrawColor(cs.borderTopColor[0], cs.borderTopColor[1], cs.borderTopColor[2])
		savedLW := doc.lineWidth
		doc.SetLineWidth(cs.borderTopWidth / doc.k)
		p.Line(doc.lMargin, p.y, p.w-doc.rMargin, p.y)
		doc.drawColor = savedDraw
		doc.SetLineWidth(savedLW)
		p.y += 1
	}

	if cs.paddingTop > 0 {
		p.y += cs.paddingTop
	}

	if cs.hasBgColor {
		r.drawBlockBackground(n, cs)
	}

	// Handle float:right with a percentage width.
	if cs.float_ == "right" && cs.widthPct > 0 {
		r.renderFloatRight(n, cs)
		return
	}

	if cs.display == "flex" && cs.justifyContent == "space-between" {
		r.renderFlexSpaceBetween(n, cs)
	} else if (cs.textAlign == "center" || cs.textAlign == "right" || cs.textAlign == "justify") && !hasBlockChildren(n) {
		r.renderAlignedBlock(n, cs)
	} else {
		r.renderNodes(n.children)
	}

	// Advance to the next line after div content.
	p = r.page.active()
	if p.x > doc.lMargin {
		p.x = doc.lMargin
		p.y += r.lineHeight
	}

	if cs.paddingBottom > 0 {
		p.y += cs.paddingBottom
	}

	// Render border-bottom if set.
	if cs.borderBottom {
		p = r.page.active()
		savedDraw := doc.drawColor
		doc.SetDrawColor(cs.borderBottomColor[0], cs.borderBottomColor[1], cs.borderBottomColor[2])
		savedLW := doc.lineWidth
		doc.SetLineWidth(cs.borderBottomWidth / doc.k)
		p.Line(doc.lMargin, p.y, p.w-doc.rMargin, p.y)
		doc.drawColor = savedDraw
		doc.SetLineWidth(savedLW)
		p.y += 1
	}

	if cs.marginBottom > 0 {
		p.y += cs.marginBottom
	}
}

// hasBlockChildren returns true if the node has any block-level element children.
// When true, the div should render children individually rather than flattening
// them into a single aligned text block.
func hasBlockChildren(n *htmlNode) bool {
	for _, child := range n.children {
		switch child.tag {
		case "div", "p", "h1", "h2", "h3", "h4", "h5", "h6",
			"table", "ul", "ol", "blockquote", "pre", "hr", "center":
			return true
		}
	}
	return false
}

// renderFloatRight renders a div positioned at the right side of the page
// with a percentage-based width.
func (r *htmlRenderer) renderFloatRight(n *htmlNode, cs cssStyle) {
	p := r.page.active()
	doc := p.doc

	totalW := r.contentWidth
	divW := totalW * cs.widthPct / 100
	savedLMargin := doc.lMargin
	savedCW := r.contentWidth

	// Position in the right portion.
	doc.lMargin = p.w - doc.rMargin - divW
	r.contentWidth = divW
	p = r.page.active()
	p.x = doc.lMargin

	r.renderNodes(n.children)

	// Restore.
	doc.lMargin = savedLMargin
	r.contentWidth = savedCW
	p = r.page.active()
	p.x = doc.lMargin

	if cs.marginBottom > 0 {
		p.y += cs.marginBottom
	}
}

// renderFlexSpaceBetween renders a flex container with justify-content: space-between.
// With two children it places them in left and right columns on the same row.
// Each child is rendered as a full block (headings, paragraphs, line breaks all work).
func (r *htmlRenderer) renderFlexSpaceBetween(n *htmlNode, cs cssStyle) {
	p := r.page.active()
	doc := p.doc

	// Collect non-text element children.
	var items []*htmlNode
	for _, child := range n.children {
		if child.tag != "" {
			items = append(items, child)
		}
	}

	if len(items) == 0 {
		r.renderNodes(n.children)
		return
	}

	if len(items) < 2 {
		r.renderNodes(items[0].children)
		p = r.page.active()
		p.x = doc.lMargin
		return
	}

	// Check if children are inline (span, b, i, etc) or block-level.
	if !hasBlockChildren(n) {
		// Simple inline case: use Cell for left and right text on one line.
		r.renderFlexInline(items, doc)
		return
	}

	// Block children: render in separate columns.
	startY := p.y
	savedLMargin := doc.lMargin
	savedRMargin := doc.rMargin
	savedCW := r.contentWidth
	halfW := r.contentWidth / 2

	// --- Render left child in left column ---
	doc.rMargin = savedRMargin + halfW
	r.contentWidth = halfW
	p = r.page.active()
	p.x = savedLMargin
	p.y = startY
	r.renderElement(items[0])
	p = r.page.active()
	leftEndY := p.y

	// --- Render right child in right column, right-aligned ---
	doc.lMargin = savedLMargin + halfW
	doc.rMargin = savedRMargin
	r.contentWidth = halfW
	p = r.page.active()
	p.x = doc.lMargin
	p.y = startY
	// Set inherited alignment so all descendants render right-aligned.
	savedAlign := r.inheritAlign
	r.inheritAlign = "right"
	r.renderElement(items[len(items)-1])
	r.inheritAlign = savedAlign
	p = r.page.active()
	rightEndY := p.y

	// Restore margins.
	doc.lMargin = savedLMargin
	doc.rMargin = savedRMargin
	r.contentWidth = savedCW

	// Advance Y to the tallest column, ensuring at least one lineHeight.
	maxY := leftEndY
	if rightEndY > maxY {
		maxY = rightEndY
	}
	if maxY <= startY {
		maxY = startY + r.lineHeight
	}
	p = r.page.active()
	p.x = doc.lMargin
	p.y = maxY
}

// renderFlexInline renders inline flex children with the first left-aligned
// and the last right-aligned on the same line using Cell.
func (r *htmlRenderer) renderFlexInline(items []*htmlNode, doc *Document) {
	p := r.page.active()
	startY := p.y

	leftText := extractText(items[0])
	rightText := extractText(items[len(items)-1])

	// Render left text with its style.
	childCS := r.resolveStyle(items[0])
	ss := r.saveStyle()
	r.applyCSS(childCS)
	p = r.page.active()
	p.x = doc.lMargin
	p.y = startY
	leftW := p.GetStringWidth(leftText)
	p.Cell(leftW, r.lineHeight, leftText, "", "L", false, 0)
	r.restoreSnapshot(ss)

	// Render right text with its style, right-aligned.
	childCS = r.resolveStyle(items[len(items)-1])
	ss = r.saveStyle()
	r.applyCSS(childCS)
	p = r.page.active()
	rightW := p.GetStringWidth(rightText)
	p.x = p.w - doc.rMargin - rightW
	p.y = startY
	p.Cell(rightW, r.lineHeight, rightText, "", "R", false, 0)
	r.restoreSnapshot(ss)

	// Advance past the line.
	p = r.page.active()
	p.x = doc.lMargin
	p.y = startY + r.lineHeight
}



// renderTextTransformed renders children with text-transform applied.
func (r *htmlRenderer) renderTextTransformed(n *htmlNode, cs cssStyle) {
	text := extractText(n)
	switch cs.textTransform {
	case "uppercase":
		text = strings.ToUpper(text)
	case "lowercase":
		text = strings.ToLower(text)
	}
	r.renderText(text)
}

func (r *htmlRenderer) renderAlignedBlock(n *htmlNode, cs cssStyle) {
	p := r.page.active()

	// Extract plain text from the block.
	text := extractText(n)
	if text == "" {
		return
	}

	align := "L"
	switch cs.textAlign {
	case "center":
		align = "C"
	case "right":
		align = "R"
	case "justify":
		align = "J"
	}

	w := r.contentWidth
	p.MultiCell(w, r.lineHeight, text, "", align, false)
}

func (r *htmlRenderer) renderBlockquote(n *htmlNode, cs cssStyle) {
	p := r.page.active()
	doc := p.doc

	indent := 10.0
	if cs.paddingLeft > 0 {
		indent = cs.paddingLeft
	}

	savedLMargin := doc.lMargin
	doc.lMargin += indent
	p = r.page.active()
	p.x = doc.lMargin

	// Draw a left border bar.
	savedDraw := doc.drawColor
	doc.SetDrawColor(180, 180, 180)
	savedLW := doc.lineWidth
	doc.SetLineWidth(0.5)
	startY := p.y

	r.renderNodes(n.children)

	p = r.page.active()
	p.Line(savedLMargin+indent-2, startY, savedLMargin+indent-2, p.y)

	doc.drawColor = savedDraw
	doc.SetLineWidth(savedLW)
	doc.lMargin = savedLMargin
	p.x = doc.lMargin
	p.y += r.lineHeight * 0.5
}

func (r *htmlRenderer) renderPre(n *htmlNode) {
	p := r.page.active()
	doc := p.doc

	savedFamily := doc.fontFamily
	savedStyle := doc.fontStyle
	savedSize := doc.fontSizePt

	doc.SetFont("courier", "", doc.fontSizePt)

	// Render text preserving whitespace and newlines.
	text := extractRawText(n)
	text = strings.TrimRight(text, "\n\r")

	p = r.page.active()
	for _, line := range strings.Split(text, "\n") {
		p = r.page.active()
		p.x = doc.lMargin
		p.Write(r.lineHeight, line)
		p = r.page.active()
		p.x = doc.lMargin
		p.y += r.lineHeight
	}

	doc.SetFont(savedFamily, savedStyle, savedSize)
	p = r.page.active()
	p.y += r.lineHeight * 0.3
}

func (r *htmlRenderer) renderCode(n *htmlNode) {
	doc := r.page.doc

	savedFamily := doc.fontFamily
	savedStyle := doc.fontStyle
	savedSize := doc.fontSizePt

	// Use monospace font, slightly smaller.
	doc.SetFont("courier", "", savedSize*0.9)

	r.renderNodes(n.children)

	doc.SetFont(savedFamily, savedStyle, savedSize)
}

func (r *htmlRenderer) renderSmall(n *htmlNode) {
	doc := r.page.doc
	savedSize := doc.fontSizePt

	doc.SetFontSize(savedSize * 0.83)
	r.renderNodes(n.children)
	doc.SetFontSize(savedSize)
}

func (r *htmlRenderer) renderMark(n *htmlNode) {
	p := r.page.active()
	doc := p.doc

	// Draw highlight background for the text.
	text := extractText(n)
	tw := p.GetStringWidth(text)

	savedFill := doc.fillColor
	doc.SetFillColor(255, 255, 0) // yellow highlight
	p.Rect(p.x, p.y, tw, r.lineHeight, "F")
	doc.fillColor = savedFill

	r.renderNodes(n.children)
}

func (r *htmlRenderer) renderSup(n *htmlNode) {
	doc := r.page.doc
	savedSize := doc.fontSizePt

	doc.SetFontSize(savedSize * 0.7)
	doc.textRise = savedSize * 0.4 / doc.k
	r.renderNodes(n.children)
	doc.textRise = 0
	doc.SetFontSize(savedSize)
}

func (r *htmlRenderer) renderSub(n *htmlNode) {
	doc := r.page.doc
	savedSize := doc.fontSizePt

	doc.SetFontSize(savedSize * 0.7)
	doc.textRise = -(savedSize * 0.2 / doc.k)
	r.renderNodes(n.children)
	doc.textRise = 0
	doc.SetFontSize(savedSize)
}

func (r *htmlRenderer) renderCenter(n *htmlNode) {
	p := r.page.active()
	doc := p.doc

	text := extractText(n)
	if text == "" {
		return
	}

	p.MultiCell(r.contentWidth, r.lineHeight, text, "", "C", false)
	p = r.page.active()
	p.x = doc.lMargin
}

func (r *htmlRenderer) renderHR() {
	p := r.page.active()
	doc := p.doc
	y := p.y + r.lineHeight*0.5
	p.Line(doc.lMargin, y, p.w-doc.rMargin, y)
	p.y = y + r.lineHeight*0.5
}

func (r *htmlRenderer) renderFont(n *htmlNode) {
	doc := r.page.doc

	if color, ok := n.attrs["color"]; ok {
		if cr, cg, cb, cok := parseCSSColor(color); cok {
			doc.SetTextColor(cr, cg, cb)
		}
	}
	if size, ok := n.attrs["size"]; ok {
		if s := parseFontSize(size); s > 0 {
			doc.SetFontSize(s)
		}
	}
	if face, ok := n.attrs["face"]; ok {
		doc.SetFont(face, doc.fontStyle, doc.fontSizePt)
	}

	r.renderNodes(n.children)
}

// parseFontSize converts HTML <font size="N"> (1-7) to point sizes.
func parseFontSize(s string) float64 {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	// HTML font sizes 1-7 mapped to approximate point sizes.
	sizes := [8]float64{0, 8, 10, 12, 14, 18, 24, 36}
	if n >= 1 && n <= 7 {
		return sizes[n]
	}
	return 0
}

// --- Lists ---

func (r *htmlRenderer) renderUL(n *htmlNode, cs cssStyle) {
	p := r.page.active()
	doc := p.doc
	indent := 5.0

	if cs.marginTop > 0 {
		p.y += cs.marginTop
	}

	r.listDepth++
	savedLMargin := doc.lMargin
	doc.lMargin += indent

	for _, child := range n.children {
		if child.tag == "ul" || child.tag == "ol" {
			// Nested list.
			r.renderElement(child)
			continue
		}
		if child.tag != "li" {
			continue
		}
		p = r.page.active()
		p.x = doc.lMargin

		// Bullet style based on depth.
		bullet := "\u2022 " // bullet
		if r.listDepth == 2 {
			bullet = "- "
		} else if r.listDepth >= 3 {
			bullet = "* "
		}
		p.Write(r.lineHeight, bullet)
		r.renderListItem(child)
		p = r.page.active()
		p.x = doc.lMargin
		p.y += r.lineHeight
	}

	doc.lMargin = savedLMargin
	r.listDepth--

	p = r.page.active()
	if cs.marginBottom > 0 {
		p.y += cs.marginBottom
	} else if r.listDepth == 0 {
		p.y += r.lineHeight * 0.3
	}
}

func (r *htmlRenderer) renderOL(n *htmlNode, cs cssStyle) {
	p := r.page.active()
	doc := p.doc
	indent := 5.0

	if cs.marginTop > 0 {
		p.y += cs.marginTop
	}

	r.listDepth++
	savedLMargin := doc.lMargin
	doc.lMargin += indent
	idx := 1

	for _, child := range n.children {
		if child.tag == "ul" || child.tag == "ol" {
			r.renderElement(child)
			continue
		}
		if child.tag != "li" {
			continue
		}
		p = r.page.active()
		p.x = doc.lMargin
		p.Write(r.lineHeight, strconv.Itoa(idx)+". ")
		r.renderListItem(child)
		p = r.page.active()
		p.x = doc.lMargin
		p.y += r.lineHeight
		idx++
	}

	doc.lMargin = savedLMargin
	r.listDepth--

	p = r.page.active()
	if cs.marginBottom > 0 {
		p.y += cs.marginBottom
	} else if r.listDepth == 0 {
		p.y += r.lineHeight * 0.3
	}
}

// renderListItem renders a list item's children, handling nested lists.
func (r *htmlRenderer) renderListItem(n *htmlNode) {
	for _, child := range n.children {
		if child.tag == "ul" || child.tag == "ol" {
			// Move to next line before rendering nested list.
			p := r.page.active()
			p.x = p.doc.lMargin
			p.y += r.lineHeight
			r.renderElement(child)
		} else if child.text != "" {
			r.renderText(child.text)
		} else {
			r.renderElement(child)
		}
	}
}

// --- Links ---

func (r *htmlRenderer) renderLink(n *htmlNode) {
	p := r.page.active()
	doc := p.doc
	href := n.attrs["href"]

	startX := p.x
	startY := p.y
	savedColor := doc.textColor
	doc.SetTextColor(0, 0, 200)

	r.renderNodes(n.children)

	doc.textColor = savedColor
	endX := r.page.active().x

	if href != "" {
		p.LinkURL(startX, startY, endX-startX, r.lineHeight, href)
	}
}

// --- Tables ---

// renderTable renders an HTML <table> using the existing Table helper.
func (r *htmlRenderer) renderTable(n *htmlNode) {
	p := r.page.active()
	doc := p.doc

	// Collect rows.
	var rows [][]string
	var isHeader []bool
	for _, child := range n.children {
		switch child.tag {
		case "tr":
			cells, header := r.collectTableRow(child)
			if len(cells) > 0 {
				rows = append(rows, cells)
				isHeader = append(isHeader, header)
			}
		case "thead", "tbody", "tfoot":
			for _, tr := range child.children {
				if tr.tag == "tr" {
					cells, header := r.collectTableRow(tr)
					if len(cells) > 0 {
						rows = append(rows, cells)
						if child.tag == "thead" {
							header = true
						}
						isHeader = append(isHeader, header)
					}
				}
			}
		}
	}

	if len(rows) == 0 {
		return
	}

	// Determine column count.
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	// Auto-size columns based on content width.
	availW := r.contentWidth
	widths := r.autoSizeColumns(p, rows, maxCols, availW)

	tbl := NewTable(doc, p)
	tbl.SetWidths(widths...)

	for i, row := range rows {
		// Pad row to maxCols.
		for len(row) < maxCols {
			row = append(row, "")
		}
		if isHeader[i] {
			tbl.Header(row...)
		} else {
			tbl.Row(row...)
		}
	}
}

func (r *htmlRenderer) collectTableRow(tr *htmlNode) ([]string, bool) {
	var cells []string
	header := false
	for _, td := range tr.children {
		if td.tag == "td" {
			cells = append(cells, extractText(td))
		} else if td.tag == "th" {
			cells = append(cells, extractText(td))
			header = true
		}
	}
	return cells, header
}

// autoSizeColumns calculates column widths based on content.
// It measures the widest cell in each column, then distributes remaining
// space proportionally to columns that need more room.
func (r *htmlRenderer) autoSizeColumns(p *Page, rows [][]string, maxCols int, availW float64) []float64 {
	// Measure the natural width of each column's widest cell.
	natural := make([]float64, maxCols)
	padding := 4.0 // cell padding in mm
	for _, row := range rows {
		for j := 0; j < maxCols && j < len(row); j++ {
			w := p.GetStringWidth(row[j]) + padding
			if w > natural[j] {
				natural[j] = w
			}
		}
	}

	// Sum of natural widths.
	totalNatural := 0.0
	for _, w := range natural {
		totalNatural += w
	}

	widths := make([]float64, maxCols)
	if totalNatural <= availW {
		// All columns fit: distribute extra space proportionally.
		extra := availW - totalNatural
		for i, w := range natural {
			widths[i] = w + extra*w/totalNatural
		}
	} else {
		// Columns overflow: scale down proportionally.
		for i, w := range natural {
			widths[i] = w * availW / totalNatural
		}
	}
	return widths
}

// --- Block background helper ---

func (r *htmlRenderer) drawBlockBackground(n *htmlNode, cs cssStyle) {
	p := r.page.active()
	doc := p.doc

	text := extractText(n)
	if text == "" {
		return
	}

	// Estimate height: count lines.
	w := r.contentWidth
	fe := p.effectiveFontEntry()
	if fe == nil {
		return
	}
	fontSize := p.effectiveFontSizePt()
	k := doc.k

	lineWidth := w * 1000.0 / fontSize * k
	textWidth := 0.0
	lines := 1
	for _, r := range text {
		var cw int
		if r < 256 {
			cw = fe.Widths[byte(r)]
		}
		if r == ' ' && textWidth+float64(cw) > lineWidth {
			lines++
			textWidth = 0
		} else {
			textWidth += float64(cw)
		}
	}

	bgH := float64(lines) * r.lineHeight
	savedFill := doc.fillColor
	doc.SetFillColor(cs.backgroundColor[0], cs.backgroundColor[1], cs.backgroundColor[2])
	p.Rect(p.x, p.y, w, bgH, "F")
	doc.fillColor = savedFill
}

// --- Text extraction helpers ---

// extractText recursively extracts all text content from an HTML node tree.
func extractText(n *htmlNode) string {
	if n.text != "" {
		return collapseWhitespace(n.text)
	}
	if n.tag == "br" {
		return "\n"
	}
	var sb strings.Builder
	for _, child := range n.children {
		sb.WriteString(extractText(child))
	}
	return strings.TrimSpace(sb.String())
}

// extractRawText extracts text preserving whitespace (for <pre> tags).
func extractRawText(n *htmlNode) string {
	if n.text != "" {
		return n.text
	}
	var sb strings.Builder
	for _, child := range n.children {
		sb.WriteString(extractRawText(child))
	}
	return sb.String()
}

// --- CSS Parsing ---

// cssStyle holds parsed CSS properties.
type cssStyle struct {
	color             [3]int
	hasColor          bool
	fontSize          float64
	fontFamily        string
	fontWeight        string // "bold", "normal"
	fontStyleCSS      string // "italic", "normal" (named to avoid conflict with font style field)
	textDecoration    string // "underline", "line-through", "none"
	textAlign         string // "left", "center", "right", "justify"
	textTransform     string // "uppercase", "lowercase", "capitalize"
	lineHeight        float64
	backgroundColor   [3]int
	hasBgColor        bool
	marginTop         float64
	marginBottom      float64
	paddingTop        float64
	paddingLeft       float64
	paddingBottom     float64
	borderTop         bool
	borderTopWidth    float64 // in points
	borderTopColor    [3]int
	borderBottom      bool
	borderBottomWidth float64 // in points
	borderBottomColor [3]int
	display           string // "flex", "block", "inline-block"
	justifyContent    string // "space-between", "center", etc.
	float_            string // "right", "left", "none"
	widthPct          float64 // width as percentage (0-100)
	clear_            string // "both", "left", "right", "none"
}

// parseInlineStyle parses a CSS style attribute string into a cssStyle.
func parseInlineStyle(s string) cssStyle {
	var cs cssStyle
	if s == "" {
		return cs
	}

	for _, decl := range strings.Split(s, ";") {
		decl = strings.TrimSpace(decl)
		parts := strings.SplitN(decl, ":", 2)
		if len(parts) != 2 {
			continue
		}
		prop := strings.TrimSpace(strings.ToLower(parts[0]))
		val := strings.TrimSpace(parts[1])

		switch prop {
		case "color":
			if r, g, b, ok := parseCSSColor(val); ok {
				cs.color = [3]int{r, g, b}
				cs.hasColor = true
			}
		case "background-color", "background":
			if r, g, b, ok := parseCSSColor(val); ok {
				cs.backgroundColor = [3]int{r, g, b}
				cs.hasBgColor = true
			}
		case "font-size":
			if size := parseCSSFontSize(val); size > 0 {
				cs.fontSize = size
			}
		case "font-family":
			// Take the first family, strip quotes.
			family := strings.Split(val, ",")[0]
			family = strings.TrimSpace(family)
			family = strings.Trim(family, "'\"")
			cs.fontFamily = strings.ToLower(family)
		case "font-weight":
			cs.fontWeight = strings.ToLower(val)
		case "font-style":
			cs.fontStyleCSS = strings.ToLower(val)
		case "text-decoration":
			cs.textDecoration = strings.ToLower(val)
		case "text-align":
			cs.textAlign = strings.ToLower(val)
		case "line-height":
			if lh := parseCSSLength(val); lh > 0 {
				cs.lineHeight = lh
			}
		case "margin-top":
			if mt := parseCSSLength(val); mt > 0 {
				cs.marginTop = mt
			}
		case "margin-bottom":
			if mb := parseCSSLength(val); mb > 0 {
				cs.marginBottom = mb
			}
		case "padding-top":
			if pt := parseCSSLength(val); pt > 0 {
				cs.paddingTop = pt
			}
		case "padding-left":
			if pl := parseCSSLength(val); pl > 0 {
				cs.paddingLeft = pl
			}
		case "margin":
			// Shorthand: margin: top [right [bottom [left]]].
			vals := strings.Fields(val)
			switch len(vals) {
			case 1: // all sides
				v := parseCSSLength(vals[0])
				cs.marginTop = v
				cs.marginBottom = v
			case 2: // top/bottom, right/left
				cs.marginTop = parseCSSLength(vals[0])
				cs.marginBottom = parseCSSLength(vals[0])
			case 3: // top, right/left, bottom
				cs.marginTop = parseCSSLength(vals[0])
				cs.marginBottom = parseCSSLength(vals[2])
			case 4: // top, right, bottom, left
				cs.marginTop = parseCSSLength(vals[0])
				cs.marginBottom = parseCSSLength(vals[2])
				cs.paddingLeft = parseCSSLength(vals[3]) // margin-left used as indent
			}
		case "padding":
			// Shorthand.
			vals := strings.Fields(val)
			if len(vals) >= 4 {
				if v := parseCSSLength(vals[3]); v > 0 {
					cs.paddingLeft = v
				}
			} else if len(vals) >= 2 {
				if v := parseCSSLength(vals[1]); v > 0 {
					cs.paddingLeft = v
				}
			} else if len(vals) >= 1 {
				if v := parseCSSLength(vals[0]); v > 0 {
					cs.paddingLeft = v
				}
			}
		case "padding-bottom":
			if pb := parseCSSLength(val); pb > 0 {
				cs.paddingBottom = pb
			}
		case "text-transform":
			cs.textTransform = strings.ToLower(val)
		case "display":
			cs.display = strings.ToLower(val)
		case "justify-content":
			cs.justifyContent = strings.ToLower(val)
		case "float":
			cs.float_ = strings.ToLower(val)
		case "width":
			v := strings.TrimSpace(strings.ToLower(val))
			if strings.HasSuffix(v, "%") {
				if pct, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64); err == nil {
					cs.widthPct = pct
				}
			}
		case "clear":
			cs.clear_ = strings.ToLower(val)
		case "border-top":
			cs.borderTop = true
			cs.borderTopWidth = 0.5
			cs.borderTopColor = [3]int{0, 0, 0}
			bparts := strings.Fields(val)
			for _, bp := range bparts {
				bp = strings.ToLower(bp)
				if bp == "solid" || bp == "dashed" || bp == "dotted" || bp == "none" {
					if bp == "none" {
						cs.borderTop = false
					}
					continue
				}
				if w := parseCSSLength(bp); w > 0 {
					cs.borderTopWidth = w * 2.83465 // mm to points
					continue
				}
				if r, g, b, ok := parseCSSColor(bp); ok {
					cs.borderTopColor = [3]int{r, g, b}
				}
			}
		case "border-bottom":
			cs.borderBottom = true
			cs.borderBottomWidth = 0.5 // default
			cs.borderBottomColor = [3]int{0, 0, 0}
			// Parse: "2px solid #333"
			bparts := strings.Fields(val)
			for _, bp := range bparts {
				bp = strings.ToLower(bp)
				if bp == "solid" || bp == "dashed" || bp == "dotted" || bp == "none" {
					if bp == "none" {
						cs.borderBottom = false
					}
					continue
				}
				if w := parseCSSLength(bp); w > 0 {
					cs.borderBottomWidth = w * 2.83465 // mm to points
					continue
				}
				if r, g, b, ok := parseCSSColor(bp); ok {
					cs.borderBottomColor = [3]int{r, g, b}
				}
			}
		}
	}
	return cs
}

// parseCSSColor parses CSS color values: #rrggbb, #rgb, rgb(r,g,b), named colors.
func parseCSSColor(s string) (int, int, int, bool) {
	s = strings.TrimSpace(strings.ToLower(s))

	// Named colors.
	if c, ok := cssNamedColors[s]; ok {
		return c[0], c[1], c[2], true
	}

	// rgb(r, g, b)
	if strings.HasPrefix(s, "rgb(") && strings.HasSuffix(s, ")") {
		inner := s[4 : len(s)-1]
		parts := strings.Split(inner, ",")
		if len(parts) == 3 {
			r, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
			g, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
			b, err3 := strconv.Atoi(strings.TrimSpace(parts[2]))
			if err1 == nil && err2 == nil && err3 == nil {
				return clamp255(r), clamp255(g), clamp255(b), true
			}
		}
		return 0, 0, 0, false
	}

	if !strings.HasPrefix(s, "#") {
		return 0, 0, 0, false
	}
	hex := s[1:]

	if len(hex) == 3 {
		r, _ := strconv.ParseUint(string(hex[0])+string(hex[0]), 16, 8)
		g, _ := strconv.ParseUint(string(hex[1])+string(hex[1]), 16, 8)
		b, _ := strconv.ParseUint(string(hex[2])+string(hex[2]), 16, 8)
		return int(r), int(g), int(b), true
	}
	if len(hex) == 6 {
		r, _ := strconv.ParseUint(hex[0:2], 16, 8)
		g, _ := strconv.ParseUint(hex[2:4], 16, 8)
		b, _ := strconv.ParseUint(hex[4:6], 16, 8)
		return int(r), int(g), int(b), true
	}
	return 0, 0, 0, false
}

func clamp255(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// parseCSSFontSize parses "12pt", "16px", or plain numbers.
func parseCSSFontSize(s string) float64 {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimSuffix(s, "pt")
	s = strings.TrimSuffix(s, "px")
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

// parseCSSLength parses a CSS length value: "10px", "5mm", "12pt", or plain numbers.
// Returns the value in user units (mm by default).
func parseCSSLength(s string) float64 {
	s = strings.TrimSpace(strings.ToLower(s))
	if strings.HasSuffix(s, "px") {
		s = strings.TrimSuffix(s, "px")
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return 0
		}
		return v * 0.264583 // px to mm
	}
	if strings.HasSuffix(s, "mm") {
		s = strings.TrimSuffix(s, "mm")
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return 0
		}
		return v
	}
	if strings.HasSuffix(s, "pt") {
		s = strings.TrimSuffix(s, "pt")
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return 0
		}
		return v * 0.352778 // pt to mm
	}
	if strings.HasSuffix(s, "em") {
		s = strings.TrimSuffix(s, "em")
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return 0
		}
		return v * 4.233 // 1em ≈ 12pt ≈ 4.233mm
	}
	// Plain number, treat as mm.
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// cssNamedColors maps CSS color names to RGB values.
var cssNamedColors = map[string][3]int{
	"black":       {0, 0, 0},
	"white":       {255, 255, 255},
	"red":         {255, 0, 0},
	"green":       {0, 128, 0},
	"blue":        {0, 0, 255},
	"yellow":      {255, 255, 0},
	"cyan":        {0, 255, 255},
	"magenta":     {255, 0, 255},
	"gray":        {128, 128, 128},
	"grey":        {128, 128, 128},
	"silver":      {192, 192, 192},
	"maroon":      {128, 0, 0},
	"olive":       {128, 128, 0},
	"lime":        {0, 255, 0},
	"aqua":        {0, 255, 255},
	"teal":        {0, 128, 128},
	"navy":        {0, 0, 128},
	"fuchsia":     {255, 0, 255},
	"purple":      {128, 0, 128},
	"orange":      {255, 165, 0},
	"pink":        {255, 192, 203},
	"brown":       {165, 42, 42},
	"gold":        {255, 215, 0},
	"coral":       {255, 127, 80},
	"crimson":     {220, 20, 60},
	"darkblue":    {0, 0, 139},
	"darkgreen":   {0, 100, 0},
	"darkred":     {139, 0, 0},
	"darkgray":    {169, 169, 169},
	"darkgrey":    {169, 169, 169},
	"lightgray":   {211, 211, 211},
	"lightgrey":   {211, 211, 211},
	"lightblue":   {173, 216, 230},
	"lightgreen":  {144, 238, 144},
	"indianred":   {205, 92, 92},
	"indigo":      {75, 0, 130},
	"ivory":       {255, 255, 240},
	"khaki":       {240, 230, 140},
	"lavender":    {230, 230, 250},
	"salmon":      {250, 128, 114},
	"sienna":      {160, 82, 45},
	"skyblue":     {135, 206, 235},
	"slategray":   {112, 128, 144},
	"slategrey":   {112, 128, 144},
	"steelblue":   {70, 130, 180},
	"tan":         {210, 180, 140},
	"tomato":      {255, 99, 71},
	"turquoise":   {64, 224, 208},
	"violet":      {238, 130, 238},
	"wheat":       {245, 222, 179},
}
