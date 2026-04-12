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

	// 5. Resource dictionary at obj 2
	d.putResourceDict(w)

	// 6. Info dictionary
	infoObjNum := d.putInfo(w)

	// 7. Catalog
	catalogObjNum := d.putCatalog(w, pageObjNums)

	// 8. Xref
	xrefOffset := w.WriteXref()

	// 9. Trailer
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
func (d *Document) putCatalog(w *pdfcore.Writer, pageObjNums []int) int {
	n := w.NewObj()
	w.Put("<<")
	w.Put("/Type /Catalog")
	w.Put("/Pages 1 0 R")
	w.Put(">>")
	w.EndObj()
	return n
}
