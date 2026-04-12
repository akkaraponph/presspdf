package folio

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ImageFormat specifies the output image format for PDF conversion.
type ImageFormat string

const (
	PNG  ImageFormat = "png"
	JPEG ImageFormat = "jpeg"
)

// ConvertOption configures PDF-to-image conversion.
type ConvertOption func(*convertConfig)

type convertConfig struct {
	dpi    int
	format ImageFormat
	pages  []int // 1-indexed; empty = all pages
}

// WithDPI sets the rendering resolution in dots per inch (default: 150).
func WithDPI(dpi int) ConvertOption {
	return func(c *convertConfig) { c.dpi = dpi }
}

// WithFormat sets the output image format (PNG or JPEG, default: PNG).
func WithFormat(f ImageFormat) ConvertOption {
	return func(c *convertConfig) { c.format = f }
}

// WithPages sets specific pages to convert (1-indexed). If not set,
// all pages are converted.
func WithPages(pages ...int) ConvertOption {
	return func(c *convertConfig) { c.pages = pages }
}

// ErrNoRenderer is returned when no supported PDF renderer is found on PATH.
// Deprecated: check for *ToolNotFoundError instead.
var ErrNoRenderer = fmt.Errorf("folio: no PDF renderer found (install poppler-utils, mupdf-tools, or ghostscript)")

// ConvertToImages converts each page of a PDF file into an image file
// saved to outputDir. Returns the paths of the generated image files
// in page order.
//
// This function requires an external PDF renderer on PATH. Supported
// renderers (tried in order): pdftoppm (poppler), mutool (mupdf),
// gs (Ghostscript).
func ConvertToImages(pdfPath, outputDir string, opts ...ConvertOption) ([]string, error) {
	cfg := &convertConfig{dpi: 150, format: PNG}
	for _, o := range opts {
		o(cfg)
	}

	backend, err := findConvertBackend()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("folio: create output dir: %w", err)
	}

	return backend.convert(pdfPath, outputDir, cfg)
}

// ConvertPage converts a single page of a PDF to an in-memory image.
// Page numbering is 1-indexed.
//
// Requires an external PDF renderer on PATH (same as ConvertToImages).
func ConvertPage(pdfPath string, page int, opts ...ConvertOption) (image.Image, error) {
	cfg := &convertConfig{dpi: 150, format: PNG}
	for _, o := range opts {
		o(cfg)
	}
	cfg.pages = []int{page}

	backend, err := findConvertBackend()
	if err != nil {
		return nil, err
	}

	tmpDir, cleanup, err := TempDir("folio-convert-*")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	paths, err := backend.convert(pdfPath, tmpDir, cfg)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("folio: page %d not found in PDF", page)
	}

	f, err := os.Open(paths[0])
	if err != nil {
		return nil, fmt.Errorf("folio: open rendered image: %w", err)
	}
	defer f.Close()

	switch cfg.format {
	case JPEG:
		return jpeg.Decode(f)
	default:
		return png.Decode(f)
	}
}

// convertBackend abstracts the external rendering tool.
type convertBackend interface {
	convert(pdfPath, outputDir string, cfg *convertConfig) ([]string, error)
}

// findConvertBackend returns the first available renderer.
func findConvertBackend() (convertBackend, error) {
	tool, err := FindTool("pdftoppm", "mutool", "gs")
	if err != nil {
		return nil, ErrNoRenderer
	}
	switch tool.Name {
	case "pdftoppm":
		return &pdftoppmBackend{tool: tool}, nil
	case "mutool":
		return &mutoolBackend{tool: tool}, nil
	case "gs":
		return &gsBackend{tool: tool}, nil
	}
	return nil, ErrNoRenderer
}

// --- pdftoppm (poppler-utils) ---

type pdftoppmBackend struct{ tool *ExternalTool }

func (b *pdftoppmBackend) convert(pdfPath, outputDir string, cfg *convertConfig) ([]string, error) {
	base := filepath.Join(outputDir, "page")
	args := []string{"-r", strconv.Itoa(cfg.dpi)}

	switch cfg.format {
	case JPEG:
		args = append(args, "-jpeg")
	default:
		args = append(args, "-png")
	}

	if len(cfg.pages) > 0 {
		first, last := pageRange(cfg.pages)
		args = append(args, "-f", strconv.Itoa(first), "-l", strconv.Itoa(last))
	}

	args = append(args, pdfPath, base)

	if _, err := b.tool.Run(args...); err != nil {
		return nil, err
	}

	return collectConvertOutput(outputDir, cfg.format)
}

// --- mutool (mupdf) ---

type mutoolBackend struct{ tool *ExternalTool }

func (b *mutoolBackend) convert(pdfPath, outputDir string, cfg *convertConfig) ([]string, error) {
	ext := "png"
	if cfg.format == JPEG {
		ext = "jpeg"
	}

	outPattern := filepath.Join(outputDir, "page-%d."+ext)
	args := []string{"convert", "-o", outPattern, "-O", "resolution=" + strconv.Itoa(cfg.dpi)}

	if len(cfg.pages) > 0 {
		args = append(args, pdfPath, pageList(cfg.pages))
	} else {
		args = append(args, pdfPath)
	}

	if _, err := b.tool.Run(args...); err != nil {
		return nil, err
	}

	return collectConvertOutput(outputDir, cfg.format)
}

// --- gs (Ghostscript) ---

type gsBackend struct{ tool *ExternalTool }

func (b *gsBackend) convert(pdfPath, outputDir string, cfg *convertConfig) ([]string, error) {
	device := "png16m"
	ext := "png"
	if cfg.format == JPEG {
		device = "jpeg"
		ext = "jpeg"
	}

	outPattern := filepath.Join(outputDir, "page-%03d."+ext)
	args := []string{
		"-dNOPAUSE", "-dBATCH", "-dSAFER", "-dQUIET",
		"-sDEVICE=" + device,
		"-r" + strconv.Itoa(cfg.dpi),
		"-sOutputFile=" + outPattern,
	}

	if len(cfg.pages) > 0 {
		first, last := pageRange(cfg.pages)
		args = append(args, "-dFirstPage="+strconv.Itoa(first), "-dLastPage="+strconv.Itoa(last))
	}

	args = append(args, pdfPath)

	if _, err := b.tool.Run(args...); err != nil {
		return nil, err
	}

	return collectConvertOutput(outputDir, cfg.format)
}

// --- helpers ---

// pageRange returns the min and max from a page list.
func pageRange(pages []int) (int, int) {
	first, last := pages[0], pages[0]
	for _, p := range pages[1:] {
		if p < first {
			first = p
		}
		if p > last {
			last = p
		}
	}
	return first, last
}

// pageList formats pages as a comma-separated string for mutool.
func pageList(pages []int) string {
	ss := make([]string, len(pages))
	for i, p := range pages {
		ss[i] = strconv.Itoa(p)
	}
	return strings.Join(ss, ",")
}

// collectConvertOutput gathers the rendered image files from outputDir.
func collectConvertOutput(dir string, format ImageFormat) ([]string, error) {
	ext := ".png"
	if format == JPEG {
		ext = ".jpeg"
	}

	paths, err := CollectFiles(dir, ext)
	if err != nil {
		return nil, err
	}

	// Also check .jpg for JPEG (some tools use this extension).
	if format == JPEG && len(paths) == 0 {
		paths, err = CollectFiles(dir, ".jpg")
		if err != nil {
			return nil, err
		}
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("folio: no output images generated")
	}

	return paths, nil
}
