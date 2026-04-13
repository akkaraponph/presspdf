package folio

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"sort"

	"github.com/akkaraponph/folio/internal/pdfcore"
)

// CompressOption configures PDF compression.
type CompressOption func(*compressConfig)

type compressConfig struct {
	imageQuality int  // JPEG re-encode quality 1-100 (0 = don't re-encode)
	dedup        bool // deduplicate identical objects
}

// CompressImageQuality re-encodes JPEG images at the given quality
// (1-100, lower = smaller). Set to 0 to skip image re-encoding.
// Default: 0 (images are kept as-is, only streams are compressed).
func CompressImageQuality(quality int) CompressOption {
	return func(c *compressConfig) { c.imageQuality = quality }
}

// CompressDedup enables deduplication of identical objects.
// Default: true.
func CompressDedup(on bool) CompressOption {
	return func(c *compressConfig) { c.dedup = on }
}

// CompressPDF rewrites a PDF with compressed streams and optional
// image quality reduction. Uncompressed streams are compressed with
// FlateDecode, and duplicate objects are merged.
//
// This is a pure Go implementation — no external tools required.
func CompressPDF(inputPath, outputPath string, opts ...CompressOption) error {
	cfg := &compressConfig{dedup: true}
	for _, o := range opts {
		o(cfg)
	}

	if dir := filepath.Dir(outputPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("folio: create output dir: %w", err)
		}
	}

	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("folio: read PDF: %w", err)
	}
	r, err := pdfcore.ReadPDF(data)
	if err != nil {
		return fmt.Errorf("folio: parse PDF: %w", err)
	}
	pageRefs, err := r.PageRefs()
	if err != nil {
		return fmt.Errorf("folio: read pages: %w", err)
	}

	// Collect all page dependencies.
	enhanced := make([]map[string]interface{}, len(pageRefs))
	var rootNums []int
	for i, ref := range pageRefs {
		attrs, err := r.InheritedPageAttrs(ref.Num)
		if err != nil {
			return err
		}
		enhanced[i] = attrs
		rootNums = append(rootNums, ref.Num)
		for _, vref := range pdfcore.CollectValueRefs(attrs) {
			rootNums = append(rootNums, vref.Num)
		}
	}
	deps, err := r.CollectDeps(rootNums...)
	if err != nil {
		return err
	}

	var sorted []int
	for num := range deps {
		sorted = append(sorted, num)
	}
	sort.Ints(sorted)

	// Compress streams and optionally re-encode images.
	compressed := make(map[int]interface{}, len(sorted))
	for _, num := range sorted {
		obj, err := r.Object(num)
		if err != nil {
			continue
		}
		compressed[num] = compressObject(obj, cfg)
	}

	// Deduplicate identical objects.
	remap := make(map[int]int)
	if cfg.dedup {
		remap = deduplicateObjects(compressed, sorted)
	}

	// Build final remap (old → new sequential numbers).
	// First, determine which objects survive dedup.
	survivors := make(map[int]bool)
	for _, num := range sorted {
		target := num
		if t, ok := remap[num]; ok {
			target = t
		}
		survivors[target] = true
	}
	var finalSorted []int
	for num := range survivors {
		finalSorted = append(finalSorted, num)
	}
	sort.Ints(finalSorted)

	// Reserve 1=Catalog, 2=Pages.
	finalRemap := make(map[int]int)
	nextNum := 3
	for _, num := range finalSorted {
		finalRemap[num] = nextNum
		nextNum++
	}
	// Map dedup aliases through to final numbers.
	for old, target := range remap {
		if fn, ok := finalRemap[target]; ok {
			finalRemap[old] = fn
		}
	}

	// Page set for /Parent override.
	pageSet := make(map[int]bool, len(pageRefs))
	enhMap := make(map[int]map[string]interface{}, len(pageRefs))
	for i, ref := range pageRefs {
		pageSet[ref.Num] = true
		enhanced[i]["/Parent"] = pdfcore.Ref{Num: 2}
		enhMap[ref.Num] = enhanced[i]
	}

	// ---- Write output ----
	var buf bytes.Buffer
	offsets := make(map[int]int)

	fmt.Fprintf(&buf, "%%PDF-%s\n", r.Version())

	for _, oldNum := range finalSorted {
		newNum := finalRemap[oldNum]
		offsets[newNum] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", newNum)

		if pageSet[oldNum] {
			pdfcore.WriteValue(&buf, enhMap[oldNum], finalRemap)
		} else {
			pdfcore.WriteValue(&buf, compressed[oldNum], finalRemap)
		}
		buf.WriteByte('\n')
		buf.WriteString("endobj\n")
	}

	// Pages tree.
	offsets[2] = buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [")
	for i, ref := range pageRefs {
		if i > 0 {
			buf.WriteByte(' ')
		}
		fmt.Fprintf(&buf, "%d 0 R", finalRemap[ref.Num])
	}
	fmt.Fprintf(&buf, "] /Count %d >>\nendobj\n", len(pageRefs))

	// Catalog.
	offsets[1] = buf.Len()
	buf.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	totalObj := nextNum - 1

	// Xref.
	xrefOffset := buf.Len()
	buf.WriteString("xref\n")
	fmt.Fprintf(&buf, "0 %d\n", totalObj+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= totalObj; i++ {
		if off, ok := offsets[i]; ok {
			fmt.Fprintf(&buf, "%010d 00000 n \n", off)
		} else {
			buf.WriteString("0000000000 00000 f \n")
		}
	}

	buf.WriteString("trailer\n")
	fmt.Fprintf(&buf, "<< /Size %d /Root 1 0 R >>\n", totalObj+1)
	buf.WriteString("startxref\n")
	fmt.Fprintf(&buf, "%d\n", xrefOffset)
	buf.WriteString("%%EOF\n")

	return os.WriteFile(outputPath, buf.Bytes(), 0o644)
}

// compressObject compresses an uncompressed stream and optionally
// re-encodes JPEG images at lower quality.
func compressObject(obj interface{}, cfg *compressConfig) interface{} {
	stm, ok := obj.(*pdfcore.Stream)
	if !ok {
		return obj
	}

	// Try JPEG image re-encoding.
	if cfg.imageQuality > 0 && cfg.imageQuality < 100 {
		if isJPEGImage(stm.Hdr) {
			reencoded := reencodeJPEG(stm.Raw, cfg.imageQuality)
			if reencoded != nil && len(reencoded) < len(stm.Raw) {
				hdr := copyDictShallow(stm.Hdr)
				hdr["/Length"] = len(reencoded)
				return &pdfcore.Stream{Hdr: hdr, Raw: reencoded}
			}
			return obj // keep original if re-encode didn't help
		}
	}

	// Skip already-compressed streams.
	if stm.Hdr["/Filter"] != nil {
		return obj
	}

	// Compress with FlateDecode.
	deflated := deflateBytes(stm.Raw)
	if len(deflated) >= len(stm.Raw) {
		return obj // compression didn't help
	}

	hdr := copyDictShallow(stm.Hdr)
	hdr["/Filter"] = pdfcore.Name("FlateDecode")
	hdr["/Length"] = len(deflated)
	return &pdfcore.Stream{Hdr: hdr, Raw: deflated}
}

func isJPEGImage(hdr map[string]interface{}) bool {
	subtype, _ := hdr["/Subtype"].(pdfcore.Name)
	filter, _ := hdr["/Filter"].(pdfcore.Name)
	return subtype == "Image" && filter == "DCTDecode"
}

func reencodeJPEG(data []byte, quality int) []byte {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		// Might be CMYK or unsupported JPEG variant — skip.
		return nil
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil
	}
	// Verify dimensions match (sanity check).
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil
	}
	origBounds := img.Bounds()
	if cfg.Width != origBounds.Dx() || cfg.Height != origBounds.Dy() {
		return nil
	}
	return buf.Bytes()
}

func deflateBytes(data []byte) []byte {
	var buf bytes.Buffer
	w, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	if err != nil {
		return data
	}
	w.Write(data)
	w.Close()
	return buf.Bytes()
}

func copyDictShallow(d map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{}, len(d))
	for k, v := range d {
		cp[k] = v
	}
	return cp
}

// deduplicateObjects finds objects with identical content and returns
// a mapping from duplicate → canonical object number.
func deduplicateObjects(objects map[int]interface{}, sorted []int) map[int]int {
	remap := make(map[int]int)
	type hashEntry struct {
		hash [32]byte
		num  int
	}
	seen := make(map[[32]byte]int) // hash → first object number

	for _, num := range sorted {
		obj := objects[num]
		var buf bytes.Buffer
		pdfcore.WriteValue(&buf, obj, nil)
		h := sha256.Sum256(buf.Bytes())

		if canon, ok := seen[h]; ok {
			remap[num] = canon
		} else {
			seen[h] = num
		}
	}
	return remap
}

// ensure image import is used
var _ image.Config
