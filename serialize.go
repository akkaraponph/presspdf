package presspdf

import (
	"fmt"
	"sort"
	"strings"
	"time"

	pdfcrypto "github.com/akkaraponph/presspdf/internal/crypto"
	"github.com/akkaraponph/presspdf/internal/pdfcore"
	"github.com/akkaraponph/presspdf/internal/resources"
)

// serialize builds the complete PDF byte stream.
func (d *Document) serialize() (*pdfcore.Writer, error) {
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("presspdf: no pages")
	}

	w := pdfcore.NewWriter()

	// 0. PDF/A validation
	if err := d.validatePDFA(); err != nil {
		return nil, err
	}

	// 1. Header
	w.WriteHeader(d.pdfaVersion())

	// 2. Encryption setup (before any content so streams are encrypted)
	var encryptObjNum int
	var fileID []byte
	var ownerHash [32]byte
	var encKey []byte
	// AES-256 specific values
	var uValue, oValue [48]byte
	var ueEncrypted, oeEncrypted []byte
	if d.encrypted {
		if d.encryptAES {
			// AES-256 (V=5, R=6)
			fileID = pdfcrypto.FileIDAES(d.title, d.producer)
			var err error
			encKey, err = pdfcrypto.GenerateFileEncryptionKey()
			if err != nil {
				return nil, fmt.Errorf("presspdf: generate encryption key: %w", err)
			}
			// Compute U value and UE (encrypted file key).
			var ueKey [32]byte
			uValue, ueKey = pdfcrypto.ComputeUserHashV5(d.userPw)
			ueEncrypted, err = pdfcrypto.EncryptAESCBC(ueKey[:], encKey)
			if err != nil {
				return nil, fmt.Errorf("presspdf: encrypt UE: %w", err)
			}
			// Compute O value and OE (encrypted file key).
			var oeKey [32]byte
			oValue, oeKey = pdfcrypto.ComputeOwnerHashV5(d.ownerPw, uValue[:])
			oeEncrypted, err = pdfcrypto.EncryptAESCBC(oeKey[:], encKey)
			if err != nil {
				return nil, fmt.Errorf("presspdf: encrypt OE: %w", err)
			}
			w.SetEncryption(encKey, pdfcrypto.EncryptDataAES256)
		} else {
			// RC4-40 (V=1, R=2)
			fileID = pdfcrypto.FileID(d.title, d.producer)
			ownerHash = pdfcrypto.ComputeOwnerHash(d.ownerPw, d.userPw)
			encKey = pdfcrypto.ComputeEncryptionKey(d.userPw, ownerHash, d.permissions, fileID)
			w.SetEncryption(encKey, pdfcrypto.EncryptData)
		}
	}

	// 2.5. Alias replacement (before page streams are written)
	d.replaceAliases()

	// 3. Pages: page dicts + content streams, then Pages root at obj 1
	pageObjNums := d.putPages(w)

	// 4. Fonts
	d.putFonts(w)

	// 5. Images
	d.putImages(w)

	// 6. ExtGState (alpha transparency)
	d.putExtGStates(w)

	// 6.5. Layers (OCG)
	d.putLayers(w)

	// 6.6. Spot colors (Separation)
	d.putSpotColors(w)

	// 6.7. Attachments (embedded files)
	attachFilespecObjs := d.putAttachments(w)

	// 7. Gradients (shading objects and their functions)
	d.putGradients(w)

	// 8. Templates (Form XObjects)
	d.putTemplates(w)

	// 9. Outlines (bookmarks)
	outlineRootObj := d.putOutlines(w, pageObjNums)

	// 10. Resource dictionary at obj 2
	d.putResourceDict(w)

	// 11. AcroForm fields
	fieldObjNums := d.putAcroForm(w, pageObjNums)

	// 12. Digital signature
	if sigFieldObj := d.putSignature(w, pageObjNums); sigFieldObj > 0 {
		fieldObjNums = append(fieldObjNums, sigFieldObj)
	}

	// 13. Encrypt dictionary (not encrypted itself)
	if d.encrypted {
		// Temporarily disable encryption for the encrypt dict.
		w.SetEncryption(nil, nil)
		if d.encryptAES {
			encryptObjNum = d.putEncryptAES256(w, uValue, oValue, ueEncrypted, oeEncrypted)
			w.SetEncryption(encKey, pdfcrypto.EncryptDataAES256)
		} else {
			encryptObjNum = d.putEncrypt(w, ownerHash, encKey, fileID)
			w.SetEncryption(encKey, pdfcrypto.EncryptData)
		}
	}

	// 13.5 Structure tree (tagged PDF)
	structTreeRootObj := d.putStructTree(w, pageObjNums)

	// 14. JavaScript
	jsObjNum := d.putJavascript(w)

	// 15. XMP metadata (user-supplied, distinct from PDF/A metadata)
	xmpObjNum := d.putXmpMetadata(w)

	// 16. PDF/A metadata and output intent
	metadataObjNum := d.putMetadata(w)
	outputIntentObjNum := d.putOutputIntent(w)

	// 17. Info dictionary
	infoObjNum := d.putInfo(w)

	// 18. Catalog
	catalogObjNum := d.putCatalog(w, pageObjNums, outlineRootObj, fieldObjNums, metadataObjNum, outputIntentObjNum, structTreeRootObj, jsObjNum, xmpObjNum, attachFilespecObjs)

	// 14. Xref
	xrefOffset := w.WriteXref()

	// 15. Trailer
	w.WriteTrailerEncrypt(catalogObjNum, infoObjNum, encryptObjNum, fileID)
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
		// Each attachment annotation uses 2 objects (filespec + annot),
		// plus the embedded file stream is handled in putAttachments.
		objNum += 2 + len(p.links) + len(p.attachAnnotations)
	}

	// Build page pointer → object number map for resolving internal link anchors.
	pageObjMap := make(map[*Page]int, len(d.pages))
	for i, p := range d.pages {
		pageObjMap[p] = pageObjNums[i]
	}

	for i, p := range d.pages {
		numLinkAnnots := len(p.links)
		numAttachAnnots := len(p.attachAnnotations)
		numAnnots := numLinkAnnots + numAttachAnnots
		contentObj := pageObjNums[i] + 1

		// Page dictionary
		w.NewObj()
		w.Put("<<")
		w.Put("/Type /Page")
		w.Putf("/Parent 1 0 R")
		w.Putf("/MediaBox [0 0 %.2f %.2f]", p.size.WidthPt, p.size.HeightPt)
		w.Put("/Resources 2 0 R")
		w.Putf("/Contents %d 0 R", contentObj)
		// Optional page boxes (TrimBox, CropBox, BleedBox, ArtBox)
		for _, boxName := range []string{"TrimBox", "CropBox", "BleedBox", "ArtBox"} {
			if box, ok := p.pageBoxes[boxName]; ok {
				w.Putf("/%s [%.2f %.2f %.2f %.2f]", boxName, box[0], box[1], box[2], box[3])
			}
		}
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
		// Tagged PDF: StructParents index for this page.
		if d.tagged && len(p.structElements) > 0 {
			w.Putf("/StructParents %d", i)
			w.Put("/Tabs /S") // tab order follows structure
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

		// File attachment annotation objects
		for _, aa := range p.attachAnnotations {
			w.NewObj()

			x1 := aa.x * k
			y1 := (p.h - (aa.y + aa.h)) * k
			x2 := (aa.x + aa.w) * k
			y2 := (p.h - aa.y) * k

			w.Put("<<")
			w.Put("/Type /Annot")
			w.Put("/Subtype /FileAttachment")
			w.Putf("/Rect [%.2f %.2f %.2f %.2f]", x1, y1, x2, y2)
			w.Put("/Border [0 0 0]")
			if aa.attachment.Description != "" {
				w.Putf("/Contents %s", pdfString(aa.attachment.Description))
			}
			if aa.attachment.Filename != "" {
				w.Putf("/T %s", pdfString(aa.attachment.Filename))
			}
			if aa.attachment.objNum > 0 {
				w.Putf("/FS %d 0 R", aa.attachment.objNum)
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
		if ae.blendMode != "" && ae.blendMode != "Normal" {
			w.Putf("/BM /%s", ae.blendMode)
		}
		w.Put(">>")
		w.EndObj()
	}
}

// putLayers writes OCG (Optional Content Group) dictionaries for layers.
func (d *Document) putLayers(w *pdfcore.Writer) {
	for i := range d.layers {
		n := w.NewObj()
		d.layers[i].objNum = n
		w.Put("<<")
		w.Put("/Type /OCG")
		w.Putf("/Name %s", pdfString(d.layers[i].name))
		w.Put(">>")
		w.EndObj()
	}
}

// putSpotColors writes Separation color space objects for spot colors.
func (d *Document) putSpotColors(w *pdfcore.Writer) {
	for i := range d.spotColors {
		sc := &d.spotColors[i]
		n := w.NewObj()
		sc.objNum = n
		w.Putf("[/Separation /%s", strings.ReplaceAll(sc.name, " ", "#20"))
		w.Put("/DeviceCMYK <<")
		w.Put("/Range [0 1 0 1 0 1 0 1] /C0 [0 0 0 0]")
		w.Putf("/C1 [%.3f %.3f %.3f %.3f]", sc.c, sc.m, sc.y, sc.k)
		w.Put("/FunctionType 2 /Domain [0 1] /N 1>>]")
		w.EndObj()
	}
}

// putAttachments writes embedded file objects and filespec dictionaries for
// both document-level and annotation-level attachments. Returns object
// numbers of document-level filespec objects (for the catalog name tree).
func (d *Document) putAttachments(w *pdfcore.Writer) []int {
	var docFilespecObjs []int

	// Helper to write a single attachment and return its filespec objNum.
	writeAttachment := func(a *Attachment) int {
		// 1. Embedded file stream (compressed)
		streamObj := w.NewObj()
		w.Putf("<</Type /EmbeddedFile /Length %d>>", len(a.Content))
		w.PutStream(a.Content)
		w.EndObj()

		// 2. Filespec dictionary
		fsObj := w.NewObj()
		w.Put("<<")
		w.Put("/Type /Filespec")
		w.Putf("/F %s", pdfString(a.Filename))
		w.Putf("/UF %s", pdfString(a.Filename))
		w.Putf("/EF <</F %d 0 R>>", streamObj)
		if a.Description != "" {
			w.Putf("/Desc %s", pdfString(a.Description))
		}
		w.Put(">>")
		w.EndObj()
		a.objNum = fsObj
		return fsObj
	}

	// Document-level attachments
	for i := range d.attachments {
		obj := writeAttachment(&d.attachments[i])
		docFilespecObjs = append(docFilespecObjs, obj)
	}

	// Page-level attachment annotations
	for _, p := range d.pages {
		for j := range p.attachAnnotations {
			writeAttachment(&p.attachAnnotations[j].attachment)
		}
	}

	return docFilespecObjs
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

// putTemplates writes Form XObject entries for all registered templates.
func (d *Document) putTemplates(w *pdfcore.Writer) {
	for _, t := range d.templates {
		n := w.NewObj()
		t.objNum = n
		data := t.stream.Bytes()
		w.Put("<<")
		w.Put("/Type /XObject")
		w.Put("/Subtype /Form")
		w.Putf("/BBox [0 0 %.2f %.2f]", t.size.WidthPt, t.size.HeightPt)
		w.Put("/Resources 2 0 R")
		w.Putf("/Length %d", len(data))
		w.Put(">>")
		w.PutStream(data)
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

	// XObject references (images + templates)
	images := d.images.All()
	if len(images) > 0 || len(d.templates) > 0 {
		s := "/XObject <<"
		for _, ie := range images {
			s += fmt.Sprintf(" /Im%s %d 0 R", ie.Name, ie.ObjNum)
		}
		for _, t := range d.templates {
			s += fmt.Sprintf(" /%s %d 0 R", t.name, t.objNum)
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

	// Layer (OCG) properties
	if len(d.layers) > 0 {
		s := "/Properties <<"
		for i, l := range d.layers {
			s += fmt.Sprintf(" /OC%d %d 0 R", i, l.objNum)
		}
		s += " >>"
		w.Put(s)
	}

	// Spot color space references
	if len(d.spotColors) > 0 {
		s := "/ColorSpace <<"
		for i, sc := range d.spotColors {
			s += fmt.Sprintf(" /CS%d %d 0 R", i+1, sc.objNum)
		}
		s += " >>"
		w.Put(s)
	}

	w.Put(">>")
	w.Put("endobj")
}

// putJavascript writes the JavaScript name tree and action objects.
// Returns the object number of the name tree, or 0 if no JavaScript is set.
func (d *Document) putJavascript(w *pdfcore.Writer) int {
	if d.javascript == nil {
		return 0
	}
	nameTreeObj := w.NewObj()
	w.Put("<<")
	w.Putf("/Names [(EmbeddedJS) %d 0 R]", nameTreeObj+1)
	w.Put(">>")
	w.EndObj()

	w.NewObj()
	w.Put("<<")
	w.Put("/S /JavaScript")
	w.Putf("/JS %s", pdfString(*d.javascript))
	w.Put(">>")
	w.EndObj()

	return nameTreeObj
}

// putXmpMetadata writes user-provided XMP metadata as a metadata stream.
// Returns the object number, or 0 if no XMP data is set.
func (d *Document) putXmpMetadata(w *pdfcore.Writer) int {
	if len(d.xmpMetadata) == 0 {
		return 0
	}
	n := w.NewObj()
	w.Putf("<< /Type /Metadata /Subtype /XML /Length %d >>", len(d.xmpMetadata))
	w.PutStream(d.xmpMetadata)
	w.EndObj()
	return n
}

// replaceAliases performs string replacement on all page content streams.
// This is called during serialization to resolve {nb} and custom aliases.
func (d *Document) replaceAliases() {
	nb := len(d.pages)
	// Register the page-count alias if set.
	if d.aliasNbPages != "" {
		d.aliases[d.aliasNbPages] = fmt.Sprintf("%d", nb)
	}
	if len(d.aliases) == 0 {
		return
	}
	for _, p := range d.pages {
		original := string(p.stream.Bytes())
		replaced := original
		for alias, replacement := range d.aliases {
			replaced = strings.ReplaceAll(replaced, alias, replacement)
		}
		if replaced != original {
			p.stream.Reset()
			// Write replaced content back without adding an extra newline.
			p.stream.Raw(strings.TrimSuffix(replaced, "\n"))
		}
	}
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
	if d.keywords != "" {
		w.Putf("/Keywords %s", pdfString(d.keywords))
	}
	if d.creator != "" {
		w.Putf("/Creator %s", pdfString(d.creator))
	}
	creation := d.creationDate
	if creation.IsZero() {
		creation = time.Now()
	}
	w.Putf("/CreationDate %s", pdfString(pdfDate(creation)))
	mod := d.modDate
	if !mod.IsZero() {
		w.Putf("/ModDate %s", pdfString(pdfDate(mod)))
	}
	w.Put(">>")
	w.EndObj()
	return n
}

// putEncrypt writes the RC4 encryption dictionary (V=1, R=2).
// Caller must disable encryption before calling and restore after.
func (d *Document) putEncrypt(w *pdfcore.Writer, ownerHash [32]byte, encKey, fileID []byte) int {
	userHash := pdfcrypto.ComputeUserHash(encKey)

	n := w.NewObj()
	w.Put("<<")
	w.Put("/Filter /Standard")
	w.Put("/V 1")
	w.Put("/R 2")
	w.Putf("/O <%x>", ownerHash)
	w.Putf("/U <%x>", userHash)
	w.Putf("/P %d", d.permissions)
	w.Put(">>")
	w.EndObj()
	return n
}

// putEncryptAES256 writes the AES-256 encrypt dictionary (V=5, R=6).
// Caller must disable encryption before calling and restore after.
func (d *Document) putEncryptAES256(w *pdfcore.Writer, uValue, oValue [48]byte, ueEncrypted, oeEncrypted []byte) int {
	n := w.NewObj()
	w.Put("<<")
	w.Put("/Filter /Standard")
	w.Put("/V 5")
	w.Put("/R 6")
	w.Put("/Length 256")
	w.Putf("/O <%x>", oValue)
	w.Putf("/U <%x>", uValue)
	w.Putf("/OE <%x>", oeEncrypted)
	w.Putf("/UE <%x>", ueEncrypted)
	w.Putf("/P %d", d.permissions)
	w.Put("/EncryptMetadata true")
	// Crypt filter for AES-256.
	w.Put("/CF <</StdCF <</AuthEvent /DocOpen /CFM /AESV3 /Length 32>>>>")
	w.Put("/StmF /StdCF")
	w.Put("/StrF /StdCF")
	w.Put(">>")
	w.EndObj()
	return n
}

// putStructTree writes the structure tree for tagged PDFs.
// Returns 0 if tagging is not enabled.
func (d *Document) putStructTree(w *pdfcore.Writer, pageObjNums []int) int {
	if !d.tagged || d.structRoot == nil || len(d.structRoot.children) == 0 {
		return 0
	}

	// Build page pointer → index map.
	pageIdx := make(map[*Page]int, len(d.pages))
	for i, p := range d.pages {
		pageIdx[p] = i
	}

	// Collect all elements in tree order (pre-order traversal).
	var allElems []*structElement
	var collect func(e *structElement)
	collect = func(e *structElement) {
		allElems = append(allElems, e)
		for _, c := range e.children {
			collect(c)
		}
	}
	for _, child := range d.structRoot.children {
		collect(child)
	}

	// Pre-allocate object numbers for all elements + parent tree + root.
	// Next obj will be w.ObjCount()+1.
	baseObj := w.ObjCount() + 1
	for i, e := range allElems {
		e.objNum = baseObj + i
	}
	parentTreeObjNum := baseObj + len(allElems)
	rootObjNum := parentTreeObjNum + 1

	// Write structure elements in pre-order. Parent /P refs are known
	// because we pre-allocated all obj numbers above.
	for _, elem := range allElems {
		parentObj := rootObjNum // default: parent is the StructTreeRoot
		if elem.parent != nil && elem.parent != d.structRoot {
			parentObj = elem.parent.objNum
		}

		w.NewObj() // allocates elem.objNum as expected
		w.Put("<<")
		w.Put("/Type /StructElem")
		w.Putf("/S /%s", elem.tag)
		w.Putf("/P %d 0 R", parentObj)

		if len(elem.children) > 0 {
			kids := "/K ["
			for _, child := range elem.children {
				kids += fmt.Sprintf(" %d 0 R", child.objNum)
			}
			kids += "]"
			w.Put(kids)
		} else if elem.page != nil {
			if pi, ok := pageIdx[elem.page]; ok {
				w.Putf("/Pg %d 0 R", pageObjNums[pi])
			}
			w.Putf("/K %d", elem.mcid)
		}

		if elem.altText != "" {
			w.Putf("/Alt (%s)", pdfEscape(elem.altText))
		}
		w.Put(">>")
		w.EndObj()
	}

	// Write parent tree (maps StructParents index + MCID → struct elem).
	w.NewObj() // parentTreeObjNum
	w.Put("<<")
	w.Put("/Nums [")
	for i, p := range d.pages {
		if len(p.structElements) == 0 {
			continue
		}
		w.Putf("%d [", i)
		for _, se := range p.structElements {
			w.Putf("%d 0 R ", se.objNum)
		}
		w.Put("]")
	}
	w.Put("]")
	w.Put(">>")
	w.EndObj()

	// Write StructTreeRoot.
	w.NewObj() // rootObjNum
	w.Put("<<")
	w.Put("/Type /StructTreeRoot")
	kids := "/K ["
	for _, child := range d.structRoot.children {
		kids += fmt.Sprintf(" %d 0 R", child.objNum)
	}
	kids += "]"
	w.Put(kids)
	w.Putf("/ParentTree %d 0 R", parentTreeObjNum)
	w.Put(">>")
	w.EndObj()

	return rootObjNum
}

func (d *Document) putCatalog(w *pdfcore.Writer, pageObjNums []int, outlineRootObj int, fieldObjNums []int, metadataObjNum, outputIntentObjNum, structTreeRootObj, jsObjNum, xmpObjNum int, attachFilespecObjs []int) int {
	n := w.NewObj()
	w.Put("<<")
	w.Put("/Type /Catalog")
	w.Put("/Pages 1 0 R")

	// Display mode: zoom
	switch d.zoomMode {
	case "fullpage":
		w.Put("/OpenAction [3 0 R /Fit]")
	case "fullwidth":
		w.Put("/OpenAction [3 0 R /FitH null]")
	case "real":
		w.Put("/OpenAction [3 0 R /XYZ null null 1]")
	}
	// Display mode: layout
	switch d.layoutMode {
	case "single", "SinglePage":
		w.Put("/PageLayout /SinglePage")
	case "continuous", "OneColumn":
		w.Put("/PageLayout /OneColumn")
	case "two", "TwoColumnLeft":
		w.Put("/PageLayout /TwoColumnLeft")
	case "TwoColumnRight":
		w.Put("/PageLayout /TwoColumnRight")
	case "TwoPageLeft":
		w.Put("/PageLayout /TwoPageLeft")
	case "TwoPageRight":
		w.Put("/PageLayout /TwoPageRight")
	}

	if outlineRootObj > 0 {
		w.Putf("/Outlines %d 0 R", outlineRootObj)
		w.Put("/PageMode /UseOutlines")
	}
	if len(fieldObjNums) > 0 {
		fields := "/AcroForm << /Fields ["
		for _, fn := range fieldObjNums {
			fields += fmt.Sprintf(" %d 0 R", fn)
		}
		fields += "] /DA (/Helv 12 Tf 0 g) /DR << /Font << /Helv 2 0 R >> >> >>"
		w.Put(fields)
	}

	// Names dictionary (JavaScript + embedded files)
	hasJS := jsObjNum > 0
	hasAttach := len(attachFilespecObjs) > 0
	if hasJS || hasAttach {
		names := "/Names <<"
		if hasJS {
			names += fmt.Sprintf("/JavaScript %d 0 R", jsObjNum)
		}
		if hasAttach {
			names += " /EmbeddedFiles <</Names ["
			for i, obj := range attachFilespecObjs {
				names += fmt.Sprintf(" (Attachment%d) %d 0 R", i+1, obj)
			}
			names += "]>>"
		}
		names += ">>"
		w.Put(names)
	}

	// Layer (OCG) properties
	if len(d.layers) > 0 {
		all := ""
		off := ""
		for i, l := range d.layers {
			if i > 0 {
				all += " "
			}
			all += fmt.Sprintf("%d 0 R", l.objNum)
			if !l.visible {
				if off != "" {
					off += " "
				}
				off += fmt.Sprintf("%d 0 R", l.objNum)
			}
		}
		w.Putf("/OCProperties <</OCGs [%s] /D <</OFF [%s] /Order [%s]>>>>", all, off, all)
		if d.openLayerPane {
			w.Put("/PageMode /UseOC")
		}
	}

	// PDF/A: metadata, output intents, and mark info.
	if metadataObjNum > 0 {
		w.Putf("/Metadata %d 0 R", metadataObjNum)
	}
	// XMP metadata (non-PDF/A)
	if xmpObjNum > 0 && metadataObjNum == 0 {
		w.Putf("/Metadata %d 0 R", xmpObjNum)
	}
	if outputIntentObjNum > 0 {
		w.Putf("/OutputIntents [%d 0 R]", outputIntentObjNum)
	}
	if d.pdfaLevel != nil || d.tagged {
		w.Put("/MarkInfo <</Marked true>>")
	}
	if structTreeRootObj > 0 {
		w.Putf("/StructTreeRoot %d 0 R", structTreeRootObj)
	}
	w.Put(">>")
	w.EndObj()
	return n
}
