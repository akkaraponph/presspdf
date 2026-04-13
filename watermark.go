package folio

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/akkaraponph/folio/internal/pdfcore"
)

// WatermarkOption configures PDF watermarking.
type WatermarkOption func(*watermarkConfig)

type watermarkConfig struct {
	// Content (text or image, not both).
	text    string
	imgPath string

	// Text styling.
	fontSize float64
	r, g, b  float64 // color (0-1)

	// General.
	opacity  float64
	rotation float64 // degrees

	// Image scaling.
	scale float64

	// Position (fraction of page 0-1; default 0.5, 0.5 = center).
	cx, cy float64
	posSet bool // true = use absolute points instead of fractions

	// Absolute position in points (used when posSet is true).
	absX, absY float64

	// Pattern mode.
	pattern    bool
	gapX, gapY float64 // spacing in points
}

// WatermarkText sets the watermark text content.
func WatermarkText(text string) WatermarkOption {
	return func(c *watermarkConfig) { c.text = text }
}

// WatermarkImage sets the watermark to an image file (JPEG or PNG).
func WatermarkImage(path string) WatermarkOption {
	return func(c *watermarkConfig) { c.imgPath = path }
}

// WatermarkFontSize sets the font size for text watermarks (default: 72).
func WatermarkFontSize(size float64) WatermarkOption {
	return func(c *watermarkConfig) { c.fontSize = size }
}

// WatermarkColor sets the text color as RGB 0-255 values.
func WatermarkColor(r, g, b int) WatermarkOption {
	return func(c *watermarkConfig) {
		c.r = float64(r) / 255.0
		c.g = float64(g) / 255.0
		c.b = float64(b) / 255.0
	}
}

// WatermarkOpacity sets the transparency (0.0 = invisible, 1.0 = opaque; default: 0.3).
func WatermarkOpacity(alpha float64) WatermarkOption {
	return func(c *watermarkConfig) { c.opacity = alpha }
}

// WatermarkRotation sets the rotation angle in degrees (default: 45).
func WatermarkRotation(degrees float64) WatermarkOption {
	return func(c *watermarkConfig) { c.rotation = degrees }
}

// WatermarkScale sets the image scale factor (default: 1.0).
func WatermarkScale(scale float64) WatermarkOption {
	return func(c *watermarkConfig) { c.scale = scale }
}

// WatermarkCenter positions the watermark at the center of each page (default).
func WatermarkCenter() WatermarkOption {
	return func(c *watermarkConfig) {
		c.cx, c.cy = 0.5, 0.5
		c.posSet = false
	}
}

// WatermarkPosition sets an absolute position in points from the bottom-left.
func WatermarkPosition(x, y float64) WatermarkOption {
	return func(c *watermarkConfig) {
		c.absX, c.absY = x, y
		c.posSet = true
	}
}

// WatermarkPattern repeats the watermark across each page in a grid.
// gapX and gapY set the horizontal and vertical spacing in points.
// Use 0 for automatic spacing.
func WatermarkPattern(gapX, gapY float64) WatermarkOption {
	return func(c *watermarkConfig) {
		c.pattern = true
		c.gapX, c.gapY = gapX, gapY
	}
}

// WatermarkTemplate applies a named preset. Subsequent options can override
// individual settings. Available templates:
//
//   - "draft"        — gray "DRAFT", 45°, centered, 30% opacity
//   - "confidential" — red "CONFIDENTIAL", 45°, centered, 20% opacity
//   - "copy"         — gray "COPY", 45°, centered, 30% opacity
//   - "sample"       — gray "SAMPLE", 45°, centered, 30% opacity
//   - "do-not-copy"  — red "DO NOT COPY", 45°, centered, 25% opacity
func WatermarkTemplate(name string) WatermarkOption {
	return func(c *watermarkConfig) {
		switch strings.ToLower(name) {
		case "draft":
			c.text = "DRAFT"
			c.fontSize = 120
			c.r, c.g, c.b = 0.75, 0.75, 0.75
			c.opacity = 0.3
			c.rotation = 45
		case "confidential":
			c.text = "CONFIDENTIAL"
			c.fontSize = 72
			c.r, c.g, c.b = 0.8, 0.0, 0.0
			c.opacity = 0.2
			c.rotation = 45
		case "copy":
			c.text = "COPY"
			c.fontSize = 120
			c.r, c.g, c.b = 0.75, 0.75, 0.75
			c.opacity = 0.3
			c.rotation = 45
		case "sample":
			c.text = "SAMPLE"
			c.fontSize = 100
			c.r, c.g, c.b = 0.75, 0.75, 0.75
			c.opacity = 0.3
			c.rotation = 45
		case "do-not-copy":
			c.text = "DO NOT COPY"
			c.fontSize = 72
			c.r, c.g, c.b = 0.8, 0.0, 0.0
			c.opacity = 0.25
			c.rotation = 45
		}
	}
}

// WatermarkPDF adds a watermark to every page of an existing PDF.
//
// This is a pure Go implementation — no external tools required.
func WatermarkPDF(inputPath, outputPath string, opts ...WatermarkOption) error {
	cfg := &watermarkConfig{
		fontSize: 72,
		r:        0.75, g: 0.75, b: 0.75,
		opacity:  0.3,
		rotation: 45,
		scale:    1.0,
		cx:       0.5, cy: 0.5,
	}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.text == "" && cfg.imgPath == "" {
		return fmt.Errorf("folio: watermark requires text or image")
	}

	// Ensure output directory exists.
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

	// Load image if needed.
	var wmImg *wmImage
	if cfg.imgPath != "" {
		wmImg, err = loadWmImage(cfg.imgPath)
		if err != nil {
			return fmt.Errorf("folio: load watermark image: %w", err)
		}
	}

	// Resolve inherited page attributes and collect deps.
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

	// Build remap (1=Catalog, 2=Pages).
	remap := make(map[int]int)
	var sorted []int
	for num := range deps {
		sorted = append(sorted, num)
	}
	sort.Ints(sorted)
	nextNum := 3
	for _, num := range sorted {
		remap[num] = nextNum
		nextNum++
	}

	// Allocate new watermark objects.
	wmFontNum := nextNum
	nextNum++
	wmGSNum := nextNum
	nextNum++
	qStreamNum := nextNum
	nextNum++
	qEndStreamNum := nextNum
	nextNum++

	var wmImgNum, wmSmaskNum int
	if wmImg != nil {
		wmImgNum = nextNum
		nextNum++
		if wmImg.smask != nil {
			wmSmaskNum = nextNum
			nextNum++
		}
	}

	// Per-page watermark content streams.
	wmStreamNums := make([]int, len(pageRefs))
	for i := range pageRefs {
		wmStreamNums[i] = nextNum
		nextNum++
	}

	// Generate per-page watermark streams and modify page dicts.
	wmStreams := make([][]byte, len(pageRefs))
	for i := range pageRefs {
		mb := getMediaBox(enhanced[i])
		wmStreams[i] = buildWmStream(cfg, mb, wmImg)

		dict := enhanced[i]

		// Modify /Contents: [q, ...original, Q, watermark].
		var contents []interface{}
		contents = append(contents, pdfcore.Ref{Num: qStreamNum})
		if existing, ok := dict["/Contents"]; ok {
			switch v := existing.(type) {
			case pdfcore.Ref:
				contents = append(contents, v)
			case []interface{}:
				contents = append(contents, v...)
			}
		}
		contents = append(contents, pdfcore.Ref{Num: qEndStreamNum})
		contents = append(contents, pdfcore.Ref{Num: wmStreamNums[i]})
		dict["/Contents"] = contents

		// Modify /Resources.
		res := resolveSubDict(dict, "/Resources", r)

		if cfg.text != "" {
			fonts := resolveSubDict(res, "/Font", r)
			fonts["/FolioWmF"] = pdfcore.Ref{Num: wmFontNum}
			res["/Font"] = fonts
		}

		gs := resolveSubDict(res, "/ExtGState", r)
		gs["/FolioWmGS"] = pdfcore.Ref{Num: wmGSNum}
		res["/ExtGState"] = gs

		if wmImg != nil {
			xo := resolveSubDict(res, "/XObject", r)
			xo["/FolioWmIm"] = pdfcore.Ref{Num: wmImgNum}
			res["/XObject"] = xo
		}

		dict["/Resources"] = res
		dict["/Parent"] = pdfcore.Ref{Num: 2}
		enhanced[i] = dict
	}

	// ---- Write output PDF ----
	var buf bytes.Buffer
	offsets := make(map[int]int)

	fmt.Fprintf(&buf, "%%PDF-%s\n", r.Version())

	// Existing objects.
	pageSet := make(map[int]bool, len(pageRefs))
	enhMap := make(map[int]map[string]interface{}, len(pageRefs))
	for i, ref := range pageRefs {
		pageSet[ref.Num] = true
		enhMap[ref.Num] = enhanced[i]
	}
	for _, oldNum := range sorted {
		newNum := remap[oldNum]
		offsets[newNum] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", newNum)
		if pageSet[oldNum] {
			pdfcore.WriteValue(&buf, enhMap[oldNum], remap)
		} else {
			obj, err := r.Object(oldNum)
			if err != nil {
				return err
			}
			pdfcore.WriteValue(&buf, obj, remap)
		}
		buf.WriteByte('\n')
		buf.WriteString("endobj\n")
	}

	// Watermark font (Helvetica — standard, no embedding needed).
	writeRawObj(&buf, offsets, wmFontNum,
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")

	// ExtGState for transparency.
	writeRawObj(&buf, offsets, wmGSNum,
		fmt.Sprintf("<< /Type /ExtGState /ca %.4f >>", cfg.opacity))

	// q / Q wrapper streams.
	writeStreamObj(&buf, offsets, qStreamNum, []byte("q\n"))
	writeStreamObj(&buf, offsets, qEndStreamNum, []byte("Q\n"))

	// Image XObject.
	if wmImg != nil {
		writeWmImageObj(&buf, offsets, wmImgNum, wmSmaskNum, wmImg)
		if wmImg.smask != nil {
			writeWmSmaskObj(&buf, offsets, wmSmaskNum, wmImg)
		}
	}

	// Per-page watermark streams.
	for i, num := range wmStreamNums {
		writeStreamObj(&buf, offsets, num, wmStreams[i])
	}

	// Pages tree.
	offsets[2] = buf.Len()
	buf.WriteString("2 0 obj\n<< /Type /Pages /Kids [")
	for i, ref := range pageRefs {
		if i > 0 {
			buf.WriteByte(' ')
		}
		fmt.Fprintf(&buf, "%d 0 R", remap[ref.Num])
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

	// Trailer.
	buf.WriteString("trailer\n")
	fmt.Fprintf(&buf, "<< /Size %d /Root 1 0 R >>\n", totalObj+1)
	buf.WriteString("startxref\n")
	fmt.Fprintf(&buf, "%d\n", xrefOffset)
	buf.WriteString("%%EOF\n")

	return os.WriteFile(outputPath, buf.Bytes(), 0o644)
}

// ---- content stream generation ----

func buildWmStream(cfg *watermarkConfig, mb [4]float64, img *wmImage) []byte {
	pageW := mb[2] - mb[0]
	pageH := mb[3] - mb[1]

	var buf bytes.Buffer

	if cfg.pattern {
		gx, gy := cfg.gapX, cfg.gapY
		if gx <= 0 {
			gx = 200
		}
		if gy <= 0 {
			gy = 200
		}
		for y := gy / 2; y < pageH+gy; y += gy {
			for x := gx / 2; x < pageW+gx; x += gx {
				writeWmAt(&buf, cfg, mb[0]+x, mb[1]+y, img)
			}
		}
	} else {
		cx, cy := cfg.cx*pageW+mb[0], cfg.cy*pageH+mb[1]
		if cfg.posSet {
			cx, cy = cfg.absX, cfg.absY
		}
		writeWmAt(&buf, cfg, cx, cy, img)
	}

	return buf.Bytes()
}

func writeWmAt(buf *bytes.Buffer, cfg *watermarkConfig, cx, cy float64, img *wmImage) {
	rad := cfg.rotation * math.Pi / 180
	cos := math.Cos(rad)
	sin := math.Sin(rad)

	buf.WriteString("q\n")
	buf.WriteString("/FolioWmGS gs\n")
	fmt.Fprintf(buf, "%.4f %.4f %.4f %.4f %.4f %.4f cm\n", cos, sin, -sin, cos, cx, cy)

	if cfg.text != "" {
		tw := float64(len(cfg.text)) * cfg.fontSize * 0.5
		buf.WriteString("BT\n")
		fmt.Fprintf(buf, "/FolioWmF %.1f Tf\n", cfg.fontSize)
		fmt.Fprintf(buf, "%.4f %.4f %.4f rg\n", cfg.r, cfg.g, cfg.b)
		fmt.Fprintf(buf, "%.4f %.4f Td\n", -tw/2, -cfg.fontSize/3)
		fmt.Fprintf(buf, "(%s) Tj\n", pdfEscape(cfg.text))
		buf.WriteString("ET\n")
	}

	if img != nil {
		iw := float64(img.width) * cfg.scale
		ih := float64(img.height) * cfg.scale
		fmt.Fprintf(buf, "%.4f 0 0 %.4f %.4f %.4f cm\n", iw, ih, -iw/2, -ih/2)
		buf.WriteString("/FolioWmIm Do\n")
	}

	buf.WriteString("Q\n")
}

// ---- helpers ----

func getMediaBox(dict map[string]interface{}) [4]float64 {
	var mb [4]float64
	if arr, ok := dict["/MediaBox"].([]interface{}); ok && len(arr) == 4 {
		for i, v := range arr {
			switch n := v.(type) {
			case int:
				mb[i] = float64(n)
			case float64:
				mb[i] = n
			}
		}
	}
	if mb[2] == 0 && mb[3] == 0 {
		// Default A4 in points.
		mb[2], mb[3] = 595, 842
	}
	return mb
}

func resolveSubDict(dict map[string]interface{}, key string, r *pdfcore.Reader) map[string]interface{} {
	v, ok := dict[key]
	if !ok {
		sub := make(map[string]interface{})
		dict[key] = sub
		return sub
	}
	if ref, ok := v.(pdfcore.Ref); ok {
		obj, err := r.Object(ref.Num)
		if err == nil {
			if d, ok := obj.(map[string]interface{}); ok {
				cp := copyDict(d)
				dict[key] = cp
				return cp
			}
		}
	}
	if d, ok := v.(map[string]interface{}); ok {
		cp := copyDict(d)
		dict[key] = cp
		return cp
	}
	sub := make(map[string]interface{})
	dict[key] = sub
	return sub
}

func copyDict(d map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{}, len(d))
	for k, v := range d {
		cp[k] = v
	}
	return cp
}

func writeRawObj(buf *bytes.Buffer, offsets map[int]int, num int, content string) {
	offsets[num] = buf.Len()
	fmt.Fprintf(buf, "%d 0 obj\n%s\nendobj\n", num, content)
}

func writeStreamObj(buf *bytes.Buffer, offsets map[int]int, num int, data []byte) {
	offsets[num] = buf.Len()
	fmt.Fprintf(buf, "%d 0 obj\n<< /Length %d >>\nstream\n", num, len(data))
	buf.Write(data)
	buf.WriteString("\nendstream\nendobj\n")
}

// ---- image handling ----

type wmImage struct {
	data   []byte
	width  int
	height int
	filter string // "DCTDecode" or "FlateDecode"
	cs     string // "DeviceRGB" or "DeviceGray"
	bpc    int
	smask  []byte // compressed alpha channel (nil if opaque)
	smaskW int
	smaskH int
}

func loadWmImage(path string) (*wmImage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return loadWmJPEG(data)
	case ".png":
		return loadWmPNG(data)
	default:
		return nil, fmt.Errorf("unsupported format: %s (use JPEG or PNG)", ext)
	}
}

func loadWmJPEG(data []byte) (*wmImage, error) {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return &wmImage{
		data: data, width: cfg.Width, height: cfg.Height,
		filter: "DCTDecode", cs: "DeviceRGB", bpc: 8,
	}, nil
}

func loadWmPNG(data []byte) (*wmImage, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	var rgb, alpha bytes.Buffer
	hasAlpha := false
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			rgb.WriteByte(byte(r >> 8))
			rgb.WriteByte(byte(g >> 8))
			rgb.WriteByte(byte(b >> 8))
			alpha.WriteByte(byte(a >> 8))
			if a < 0xFFFF {
				hasAlpha = true
			}
		}
	}

	compressed := deflate(rgb.Bytes())
	result := &wmImage{
		data: compressed, width: w, height: h,
		filter: "FlateDecode", cs: "DeviceRGB", bpc: 8,
	}
	if hasAlpha {
		result.smask = deflate(alpha.Bytes())
		result.smaskW = w
		result.smaskH = h
	}
	return result, nil
}

func deflate(data []byte) []byte {
	var buf bytes.Buffer
	w, _ := zlib.NewWriterLevel(&buf, zlib.DefaultCompression)
	w.Write(data)
	w.Close()
	return buf.Bytes()
}

func writeWmImageObj(buf *bytes.Buffer, offsets map[int]int, num, smaskNum int, img *wmImage) {
	offsets[num] = buf.Len()
	fmt.Fprintf(buf, "%d 0 obj\n", num)
	fmt.Fprintf(buf, "<< /Type /XObject /Subtype /Image /Width %d /Height %d", img.width, img.height)
	fmt.Fprintf(buf, " /ColorSpace /%s /BitsPerComponent %d", img.cs, img.bpc)
	fmt.Fprintf(buf, " /Filter /%s /Length %d", img.filter, len(img.data))
	if img.smask != nil {
		fmt.Fprintf(buf, " /SMask %d 0 R", smaskNum)
	}
	buf.WriteString(" >>\nstream\n")
	buf.Write(img.data)
	buf.WriteString("\nendstream\nendobj\n")
}

func writeWmSmaskObj(buf *bytes.Buffer, offsets map[int]int, num int, img *wmImage) {
	offsets[num] = buf.Len()
	fmt.Fprintf(buf, "%d 0 obj\n", num)
	fmt.Fprintf(buf, "<< /Type /XObject /Subtype /Image /Width %d /Height %d", img.smaskW, img.smaskH)
	fmt.Fprintf(buf, " /ColorSpace /DeviceGray /BitsPerComponent 8")
	fmt.Fprintf(buf, " /Filter /FlateDecode /Length %d >>\nstream\n", len(img.smask))
	buf.Write(img.smask)
	buf.WriteString("\nendstream\nendobj\n")
}

// imageForWm converts a Go image.Image to a wmImage for use in watermarking.
// This is an internal helper; users provide a file path via WatermarkImage.
func imageToWmImage(img image.Image) *wmImage {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	var rgb, alpha bytes.Buffer
	hasAlpha := false
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			rgb.WriteByte(byte(r >> 8))
			rgb.WriteByte(byte(g >> 8))
			rgb.WriteByte(byte(b >> 8))
			alpha.WriteByte(byte(a >> 8))
			if a < 0xFFFF {
				hasAlpha = true
			}
		}
	}

	result := &wmImage{
		data: deflate(rgb.Bytes()), width: w, height: h,
		filter: "FlateDecode", cs: "DeviceRGB", bpc: 8,
	}
	if hasAlpha {
		result.smask = deflate(alpha.Bytes())
		result.smaskW = w
		result.smaskH = h
	}
	return result
}
