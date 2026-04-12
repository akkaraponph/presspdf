package folio

import (
	"fmt"
	"sort"
	"time"

	"github.com/akkaraponph/folio/internal/pdfcore"
	"github.com/akkaraponph/folio/internal/resources"
)

// serialize builds the complete PDF byte stream.
func (d *Document) serialize() (*pdfcore.Writer, error) {
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("folio: no pages")
	}

	w := pdfcore.NewWriter()

	// 1. Header
	w.WriteHeader("1.4")

	// 2. Pages: page dicts + content streams, then Pages root at obj 1
	pageObjNums := d.putPages(w)

	// 3. Fonts
	d.putFonts(w)

	// 4. Images
	d.putImages(w)

	// 5. ExtGState (alpha transparency)
	d.putExtGStates(w)

	// 6. Gradients (shading objects and their functions)
	d.putGradients(w)

	// 7. Outlines (bookmarks)
	outlineRootObj := d.putOutlines(w, pageObjNums)

	// 8. Resource dictionary at obj 2
	d.putResourceDict(w)

	// 9. Info dictionary
	infoObjNum := d.putInfo(w)

	// 10. Catalog
	catalogObjNum := d.putCatalog(w, pageObjNums, outlineRootObj)

	// 10. Xref
	xrefOffset := w.WriteXref()

	// 11. Trailer
	w.WriteTrailer(catalogObjNum, infoObjNum)
	w.WriteStartXref(xrefOffset)

	return w, w.Err()
}

// putPages writes page dictionaries, content streams, and link annotations.
// Returns the object numbers of each page dictionary.
func (d *Document) putPages(w *pdfcore.Writer) []int {
	pageObjNums := make([]int, len(d.pages))

	// Pre-compute page object numbers so internal links can reference
	// target pages before they are written. Each page uses:
	//   1 object for the page dict + 1 for the content stream + N for annotations.
	objNum := 3 // first available after reserved objects 1 and 2
	for i, p := range d.pages {
		pageObjNums[i] = objNum
		objNum += 2 + len(p.links) // page dict + content + annotations
	}

	// Build page pointer → object number map for resolving internal link anchors.
	pageObjMap := make(map[*Page]int, len(d.pages))
	for i, p := range d.pages {
		pageObjMap[p] = pageObjNums[i]
	}

	for i, p := range d.pages {
		numAnnots := len(p.links)
		contentObj := pageObjNums[i] + 1

		// Page dictionary
		w.NewObj()
		w.Put("<<")
		w.Put("/Type /Page")
		w.Putf("/Parent 1 0 R")
		w.Putf("/MediaBox [0 0 %.2f %.2f]", p.size.WidthPt, p.size.HeightPt)
		w.Put("/Resources 2 0 R")
		w.Putf("/Contents %d 0 R", contentObj)
		if numAnnots > 0 {
			annots := "/Annots ["
			for j := 0; j < numAnnots; j++ {
				if j > 0 {
					annots += " "
				}
				annots += fmt.Sprintf("%d 0 R", contentObj+1+j)
			}
			annots += "]"
			w.Put(annots)
		}
		w.Put(">>")
		w.EndObj()

		// Content stream
		data := p.stream.Bytes()
		w.NewObj()
		if d.compress && len(data) > 0 {
			w.PutCompressedStream(data)
		} else {
			w.Putf("<</Length %d>>", len(data))
			w.PutStream(data)
		}
		w.EndObj()

		// Annotation objects
		k := d.k
		for _, link := range p.links {
			w.NewObj()

			// Convert user-unit rect to PDF points (bottom-left origin).
			x1 := link.x * k
			y1 := (p.h - (link.y + link.h)) * k // bottom edge
			x2 := (link.x + link.w) * k
			y2 := (p.h - link.y) * k // top edge

			w.Put("<<")
			w.Put("/Type /Annot")
			w.Put("/Subtype /Link")
			w.Putf("/Rect [%.2f %.2f %.2f %.2f]", x1, y1, x2, y2)
			w.Put("/Border [0 0 0]")

			if link.url != "" {
				w.Putf("/A <</S /URI /URI %s>>", pdfString(link.url))
			} else if link.anchor != "" {
				if dest, ok := d.anchors[link.anchor]; ok {
					if targetObj, ok := pageObjMap[dest.page]; ok {
						destY := (dest.page.h - dest.y) * k
						w.Putf("/Dest [%d 0 R /XYZ 0 %.2f 0]", targetObj, destY)
					}
				}
			}

			w.Put(">>")
			w.EndObj()
		}
	}

	// Pages root at object 1
	w.SetOffset(1)
	w.Putf("%d 0 obj", 1)
	w.Put("<<")
	w.Put("/Type /Pages")

	// Kids array
	kids := "/Kids ["
	for i, n := range pageObjNums {
		if i > 0 {
			kids += " "
		}
		kids += fmt.Sprintf("%d 0 R", n)
	}
	kids += "]"
	w.Put(kids)

	w.Putf("/Count %d", len(d.pages))
	w.Put(">>")
	w.Put("endobj")

	return pageObjNums
}

// putFonts writes font objects for all registered fonts.
func (d *Document) putFonts(w *pdfcore.Writer) {
	for _, fe := range d.fonts.All() {
		if fe.Type == "TTF" {
			d.putTTFFont(w, fe)
		} else {
			d.putCoreFont(w, fe)
		}
	}
}

func (d *Document) putCoreFont(w *pdfcore.Writer, fe *resources.FontEntry) {
	n := w.NewObj()
	fe.ObjNum = n
	w.Put("<<")
	w.Put("/Type /Font")
	w.Put("/Subtype /Type1")
	w.Putf("/BaseFont /%s", fe.Name)
	if fe.Name != "Symbol" && fe.Name != "ZapfDingbats" {
		w.Put("/Encoding /WinAnsiEncoding")
	}
	w.Put(">>")
	w.EndObj()
}

// putTTFFont writes CIDFont Type2 objects for an embedded TrueType font.
// Uses the gofpdf approach: Unicode code points as CIDs with a custom
// CIDToGIDMap stream that maps Unicode→GlyphID.
//
// PDF structure:
//
//	Type0 font (/Encoding /Identity-H) → DescendantFonts → CIDFont
//	                                    → ToUnicode CMap
//	CIDFont → FontDescriptor → FontFile2 (subset TTF)
//	        → CIDToGIDMap stream (Unicode → GlyphID)
func (d *Document) putTTFFont(w *pdfcore.Writer, fe *resources.FontEntry) {
	ttf := fe.TTF

	// Generate subset font
	subsetData, runeToGlyph := ttf.Subset(fe.UsedRunes)
	fe.SubsetData = subsetData
	fe.RuneToGlyph = runeToGlyph

	// 1. Font file stream (subset TTF)
	fileFontObj := w.NewObj()
	fe.FileFontObjNum = fileFontObj
	if d.compress {
		w.PutCompressedStream(subsetData)
	} else {
		w.Putf("<</Length %d /Length1 %d>>", len(subsetData), len(subsetData))
		w.PutStream(subsetData)
	}
	w.EndObj()

	// 2. FontDescriptor
	descObj := w.NewObj()
	fe.DescObjNum = descObj
	w.Put("<<")
	w.Put("/Type /FontDescriptor")
	w.Putf("/FontName /%s", fe.Name)
	w.Putf("/Ascent %d", ttf.Ascent)
	w.Putf("/Descent %d", ttf.Descent)
	w.Putf("/CapHeight %d", ttf.CapHeight)
	w.Putf("/Flags %d", ttf.Flags)
	w.Putf("/FontBBox [%d %d %d %d]", ttf.Bbox[0], ttf.Bbox[1], ttf.Bbox[2], ttf.Bbox[3])
	w.Putf("/ItalicAngle %d", ttf.ItalicAngle)
	w.Putf("/StemV %d", ttf.StemV)
	w.Putf("/MissingWidth %.0f", ttf.DefaultWidth)
	w.Putf("/FontFile2 %d 0 R", fileFontObj)
	w.Put(">>")
	w.EndObj()

	// 3. Build /W (widths) array keyed by Unicode code points (CID = Unicode)
	wArray := buildCIDWidthArray(fe)

	// 4. CIDFont (DescendantFont) — references CIDToGIDMap as next+2 object
	cidFontObj := w.NewObj()
	fe.CIDFontObjNum = cidFontObj
	cidToGidObjNum := cidFontObj + 3 // ToUnicode, Type0, then CIDToGIDMap
	w.Put("<<")
	w.Put("/Type /Font")
	w.Put("/Subtype /CIDFontType2")
	w.Putf("/BaseFont /%s", fe.Name)
	w.Put("/CIDSystemInfo <</Registry (Adobe) /Ordering (Identity) /Supplement 0>>")
	w.Putf("/FontDescriptor %d 0 R", descObj)
	w.Putf("/DW %.0f", ttf.DefaultWidth)
	w.Putf("/W %s", wArray)
	w.Putf("/CIDToGIDMap %d 0 R", cidToGidObjNum)
	w.Put(">>")
	w.EndObj()

	// 5. ToUnicode CMap (identity: CID = Unicode code point)
	toUnicodeObj := w.NewObj()
	fe.ToUnicodeObjNum = toUnicodeObj
	cmapData := []byte(resources.ToUnicodeCMap)
	if d.compress {
		w.PutCompressedStream(cmapData)
	} else {
		w.Putf("<</Length %d>>", len(cmapData))
		w.PutStream(cmapData)
	}
	w.EndObj()

	// 6. Type0 font (top-level reference)
	fontObj := w.NewObj()
	fe.ObjNum = fontObj
	w.Put("<<")
	w.Put("/Type /Font")
	w.Put("/Subtype /Type0")
	w.Putf("/BaseFont /%s", fe.Name)
	w.Put("/Encoding /Identity-H")
	w.Putf("/DescendantFonts [%d 0 R]", cidFontObj)
	w.Putf("/ToUnicode %d 0 R", toUnicodeObj)
	w.Put(">>")
	w.EndObj()

	// 7. CIDToGIDMap stream (maps Unicode code points → glyph IDs)
	cidToGidMap := buildCIDToGIDMap(fe)
	w.NewObj()
	if d.compress {
		w.PutCompressedStream(cidToGidMap)
	} else {
		w.Putf("<</Length %d>>", len(cidToGidMap))
		w.PutStream(cidToGidMap)
	}
	w.EndObj()
}

// buildCIDWidthArray builds the /W array for a CIDFont.
// Format: [cid1 [w1 w2 ...] cid2 [w3 w4 ...] ...]
// CID = Unicode code point (custom CIDToGIDMap maps Unicode→GlyphID).
func buildCIDWidthArray(fe *resources.FontEntry) string {
	if fe.TTF == nil {
		return "[]"
	}

	// Collect all used Unicode code points and their widths
	type cidWidth struct {
		cid   int
		width int
	}
	var cws []cidWidth
	for _, ch := range fe.UsedRunes {
		w := fe.TTF.CharWidths[ch]
		cws = append(cws, cidWidth{cid: ch, width: w})
	}
	sort.Slice(cws, func(i, j int) bool { return cws[i].cid < cws[j].cid })

	if len(cws) == 0 {
		return "[]"
	}

	// Group consecutive CIDs
	s := "["
	i := 0
	for i < len(cws) {
		start := i
		// Find consecutive run
		for i+1 < len(cws) && cws[i+1].cid == cws[i].cid+1 {
			i++
		}
		s += fmt.Sprintf(" %d [", cws[start].cid)
		for j := start; j <= i; j++ {
			if j > start {
				s += " "
			}
			s += fmt.Sprintf("%d", cws[j].width)
		}
		s += "]"
		i++
	}
	s += "]"
	return s
}

// buildCIDToGIDMap builds a binary CIDToGIDMap stream that maps Unicode code
// points (used as CIDs) to glyph IDs in the subset font. This is a 128KB
// array where entry[cid*2 : cid*2+2] contains the big-endian glyph ID.
func buildCIDToGIDMap(fe *resources.FontEntry) []byte {
	cidToGid := make([]byte, 256*256*2)
	if fe.TTF != nil {
		for ch, gid := range fe.TTF.CharToGlyph {
			if ch > 0 && ch < 256*256 {
				cidToGid[ch*2] = byte(gid >> 8)
				cidToGid[ch*2+1] = byte(gid & 0xFF)
			}
		}
	}
	return cidToGid
}

// putImages writes image XObjects for all registered images.
// For images with transparency (SMaskData), a separate SMask XObject is
// written first, then referenced from the main image.
func (d *Document) putImages(w *pdfcore.Writer) {
	for _, ie := range d.images.All() {
		// Write SMask XObject first (if present) so we know its obj number.
		if ie.SMaskData != nil {
			smaskObj := w.NewObj()
			ie.SMaskObjNum = smaskObj
			w.Put("<<")
			w.Put("/Type /XObject")
			w.Put("/Subtype /Image")
			w.Putf("/Width %d", ie.Width)
			w.Putf("/Height %d", ie.Height)
			w.Put("/ColorSpace /DeviceGray")
			w.Putf("/BitsPerComponent %d", ie.BPC)
			w.Put("/Filter /FlateDecode")
			w.Putf("/Length %d", len(ie.SMaskData))
			w.Put(">>")
			w.PutStream(ie.SMaskData)
			w.EndObj()
		}

		// Main image XObject.
		n := w.NewObj()
		ie.ObjNum = n
		w.Put("<<")
		w.Put("/Type /XObject")
		w.Put("/Subtype /Image")
		w.Putf("/Width %d", ie.Width)
		w.Putf("/Height %d", ie.Height)
		w.Putf("/ColorSpace /%s", ie.ColorSpace)
		w.Putf("/BitsPerComponent %d", ie.BPC)
		w.Putf("/Filter /%s", ie.Filter)
		if ie.SMaskData != nil {
			w.Putf("/SMask %d 0 R", ie.SMaskObjNum)
		}
		w.Putf("/Length %d", len(ie.Data))
		w.Put(">>")
		w.PutStream(ie.Data)
		w.EndObj()
	}
}

// putExtGStates writes ExtGState objects for alpha transparency.
func (d *Document) putExtGStates(w *pdfcore.Writer) {
	for _, ae := range d.alphaStates {
		n := w.NewObj()
		ae.objNum = n
		w.Put("<<")
		w.Put("/Type /ExtGState")
		w.Putf("/ca %.3f", ae.alpha) // fill opacity
		w.Putf("/CA %.3f", ae.alpha) // stroke opacity
		w.Put(">>")
		w.EndObj()
	}
}

// putGradients writes shading objects for all registered gradients.
// Each gradient consists of a function object (Type 3 stitching function
// that chains Type 2 exponential interpolation functions for each stop
// pair) and a shading object (Type 2 axial or Type 3 radial).
func (d *Document) putGradients(w *pdfcore.Writer) {
	for _, g := range d.gradients {
		stops := g.colors
		if len(stops) < 2 {
			continue
		}

		// Sort stops by position.
		sort.Slice(stops, func(i, j int) bool { return stops[i].pos < stops[j].pos })

		// Write Type 2 (exponential interpolation) functions for each stop pair.
		nFuncs := len(stops) - 1
		funcObjNums := make([]int, nFuncs)
		for i := 0; i < nFuncs; i++ {
			n := w.NewObj()
			funcObjNums[i] = n
			w.Put("<<")
			w.Put("/FunctionType 2")
			w.Put("/Domain [0 1]")
			w.Putf("/C0 [%.4f %.4f %.4f]", stops[i].r, stops[i].g, stops[i].b)
			w.Putf("/C1 [%.4f %.4f %.4f]", stops[i+1].r, stops[i+1].g, stops[i+1].b)
			w.Put("/N 1")
			w.Put(">>")
			w.EndObj()
		}

		// Write stitching function (Type 3) if multiple stop pairs.
		var funcRef string
		if nFuncs == 1 {
			funcRef = fmt.Sprintf("%d 0 R", funcObjNums[0])
		} else {
			n := w.NewObj()
			w.Put("<<")
			w.Put("/FunctionType 3")
			w.Put("/Domain [0 1]")
			funcs := "/Functions ["
			for i, fn := range funcObjNums {
				if i > 0 {
					funcs += " "
				}
				funcs += fmt.Sprintf("%d 0 R", fn)
			}
			funcs += "]"
			w.Put(funcs)

			// Bounds: interior breakpoints between stop pairs.
			bounds := "/Bounds ["
			for i := 1; i < len(stops)-1; i++ {
				if i > 1 {
					bounds += " "
				}
				bounds += fmt.Sprintf("%.4f", stops[i].pos)
			}
			bounds += "]"
			w.Put(bounds)

			// Encode: map each sub-function's domain to [0 1].
			encode := "/Encode ["
			for i := 0; i < nFuncs; i++ {
				if i > 0 {
					encode += " "
				}
				encode += "0 1"
			}
			encode += "]"
			w.Put(encode)

			w.Put(">>")
			w.EndObj()
			funcRef = fmt.Sprintf("%d 0 R", n)
		}

		// Write shading object.
		n := w.NewObj()
		g.objNum = n
		w.Put("<<")
		w.Putf("/ShadingType %d", g.gtype)
		w.Put("/ColorSpace /DeviceRGB")
		if g.gtype == 2 {
			// Axial (linear)
			w.Putf("/Coords [%.4f %.4f %.4f %.4f]", g.x0, g.y0, g.x1, g.y1)
		} else {
			// Radial
			w.Putf("/Coords [%.4f %.4f %.4f %.4f %.4f %.4f]", g.x0, g.y0, g.r0, g.x1, g.y1, g.r1)
		}
		w.Putf("/Function %s", funcRef)
		w.Put("/Extend [true true]")
		w.Put(">>")
		w.EndObj()
	}
}

// outlineNode is used during serialization to build the outline tree.
type outlineNode struct {
	entry    *outlineEntry
	objNum   int
	parent   *outlineNode
	children []*outlineNode
}

// putOutlines writes the PDF outline (bookmark) tree.
// Returns the object number of the outline root, or 0 if no outlines.
func (d *Document) putOutlines(w *pdfcore.Writer, pageObjNums []int) int {
	if len(d.outlines) == 0 {
		return 0
	}

	// Build page → object number map.
	pageObjMap := make(map[*Page]int, len(d.pages))
	for i, p := range d.pages {
		pageObjMap[p] = pageObjNums[i]
	}

	// Build tree from flat level-based list.
	root := &outlineNode{} // virtual root
	stack := []*outlineNode{root}

	for _, entry := range d.outlines {
		node := &outlineNode{entry: entry}

		// Pop stack until we find the correct parent level.
		// Parent's level should be entry.level - 1, or root for level 0.
		for len(stack) > entry.level+1 {
			stack = stack[:len(stack)-1]
		}

		parent := stack[len(stack)-1]
		node.parent = parent
		parent.children = append(parent.children, node)

		// Push this node so it can be a parent for deeper levels.
		if len(stack) == entry.level+1 {
			stack = append(stack, node)
		} else {
			stack[entry.level+1] = node
		}
	}

	// Collect all nodes in DFS pre-order for object allocation.
	var allNodes []*outlineNode
	var collect func(n *outlineNode)
	collect = func(n *outlineNode) {
		if n.entry != nil { // skip virtual root
			allNodes = append(allNodes, n)
		}
		for _, c := range n.children {
			collect(c)
		}
	}
	collect(root)

	// Allocate object numbers: root + all items.
	rootObjNum := w.ObjCount() + 1
	for i, n := range allNodes {
		n.objNum = rootObjNum + 1 + i
	}
	root.objNum = rootObjNum

	// Count visible descendants for a node.
	var countDesc func(n *outlineNode) int
	countDesc = func(n *outlineNode) int {
		c := len(n.children)
		for _, ch := range n.children {
			c += countDesc(ch)
		}
		return c
	}

	// Write outline root object.
	w.NewObj()
	w.Put("<<")
	w.Put("/Type /Outlines")
	if len(root.children) > 0 {
		w.Putf("/First %d 0 R", root.children[0].objNum)
		w.Putf("/Last %d 0 R", root.children[len(root.children)-1].objNum)
		w.Putf("/Count %d", countDesc(root))
	}
	w.Put(">>")
	w.EndObj()

	// Write each outline item.
	for _, node := range allNodes {
		w.NewObj()
		w.Put("<<")
		w.Putf("/Title %s", pdfString(node.entry.title))
		w.Putf("/Parent %d 0 R", node.parent.objNum)

		// Destination
		if pageObj, ok := pageObjMap[node.entry.page]; ok {
			destY := (node.entry.page.h - node.entry.y) * d.k
			w.Putf("/Dest [%d 0 R /XYZ 0 %.2f 0]", pageObj, destY)
		}

		// Sibling links
		siblings := node.parent.children
		for idx, sib := range siblings {
			if sib == node {
				if idx > 0 {
					w.Putf("/Prev %d 0 R", siblings[idx-1].objNum)
				}
				if idx < len(siblings)-1 {
					w.Putf("/Next %d 0 R", siblings[idx+1].objNum)
				}
				break
			}
		}

		// Children
		if len(node.children) > 0 {
			w.Putf("/First %d 0 R", node.children[0].objNum)
			w.Putf("/Last %d 0 R", node.children[len(node.children)-1].objNum)
			w.Putf("/Count %d", countDesc(node))
		}

		w.Put(">>")
		w.EndObj()
	}

	return rootObjNum
}

// putResourceDict writes the shared resource dictionary at object 2.
func (d *Document) putResourceDict(w *pdfcore.Writer) {
	w.SetOffset(2)
	w.Putf("%d 0 obj", 2)
	w.Put("<<")
	w.Put("/ProcSet [/PDF /Text /ImageB /ImageC /ImageI]")

	// Font references
	fonts := d.fonts.All()
	if len(fonts) > 0 {
		s := "/Font <<"
		for _, fe := range fonts {
			s += fmt.Sprintf(" /F%s %d 0 R", fe.Index, fe.ObjNum)
		}
		s += " >>"
		w.Put(s)
	}

	// Image references
	images := d.images.All()
	if len(images) > 0 {
		s := "/XObject <<"
		for _, ie := range images {
			s += fmt.Sprintf(" /Im%s %d 0 R", ie.Name, ie.ObjNum)
		}
		s += " >>"
		w.Put(s)
	}

	// ExtGState references (alpha transparency)
	if len(d.alphaStates) > 0 {
		s := "/ExtGState <<"
		for _, ae := range d.alphaStates {
			s += fmt.Sprintf(" /%s %d 0 R", ae.name, ae.objNum)
		}
		s += " >>"
		w.Put(s)
	}

	// Shading references (gradients)
	if len(d.gradients) > 0 {
		s := "/Shading <<"
		for _, g := range d.gradients {
			if g.objNum > 0 {
				s += fmt.Sprintf(" /%s %d 0 R", g.name, g.objNum)
			}
		}
		s += " >>"
		w.Put(s)
	}

	w.Put(">>")
	w.Put("endobj")
}

// putInfo writes the document info dictionary.
func (d *Document) putInfo(w *pdfcore.Writer) int {
	n := w.NewObj()
	w.Put("<<")
	w.Putf("/Producer %s", pdfString(d.producer))
	if d.title != "" {
		w.Putf("/Title %s", pdfString(d.title))
	}
	if d.author != "" {
		w.Putf("/Author %s", pdfString(d.author))
	}
	if d.subject != "" {
		w.Putf("/Subject %s", pdfString(d.subject))
	}
	if d.creator != "" {
		w.Putf("/Creator %s", pdfString(d.creator))
	}
	w.Putf("/CreationDate %s", pdfString(pdfDate(time.Now())))
	w.Put(">>")
	w.EndObj()
	return n
}

// putCatalog writes the document catalog.
func (d *Document) putCatalog(w *pdfcore.Writer, pageObjNums []int, outlineRootObj int) int {
	n := w.NewObj()
	w.Put("<<")
	w.Put("/Type /Catalog")
	w.Put("/Pages 1 0 R")
	if outlineRootObj > 0 {
		w.Putf("/Outlines %d 0 R", outlineRootObj)
		w.Put("/PageMode /UseOutlines")
	}
	w.Put(">>")
	w.EndObj()
	return n
}
