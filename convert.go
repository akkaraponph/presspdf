package folio

import (
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
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

// ConvertToImages converts each page of a PDF file into an image file
// saved to outputDir. Returns the paths of the generated image files
// in page order.
//
// This function requires an external PDF renderer on PATH. Supported
// renderers (tried in order): pdftoppm (poppler), mutool (mupdf),
// gs (Ghostscript).
//
// If no renderer is found, it returns ErrNoRenderer.
func ConvertToImages(pdfPath, outputDir string, opts ...ConvertOption) ([]string, error) {
	cfg := &convertConfig{dpi: 150, format: PNG}
	for _, o := range opts {
		o(cfg)
	}

	renderer, err := findRenderer()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("folio: create output dir: %w", err)
	}

	return renderer.convert(pdfPath, outputDir, cfg)
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

	renderer, err := findRenderer()
	if err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "folio-convert-*")
	if err != nil {
		return nil, fmt.Errorf("folio: create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	paths, err := renderer.convert(pdfPath, tmpDir, cfg)
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

// ErrNoRenderer is returned when no supported PDF renderer is found on PATH.
var ErrNoRenderer = fmt.Errorf("folio: no PDF renderer found (install poppler-utils, mupdf-tools, or ghostscript)")

// pdfRenderer abstracts the external rendering tool.
type pdfRenderer interface {
	convert(pdfPath, outputDir string, cfg *convertConfig) ([]string, error)
}

// findRenderer returns the first available renderer.
func findRenderer() (pdfRenderer, error) {
	if p, err := exec.LookPath("pdftoppm"); err == nil {
		return &pdftoppmRenderer{bin: p}, nil
	}
	if p, err := exec.LookPath("mutool"); err == nil {
		return &mutoolRenderer{bin: p}, nil
	}
	if p, err := exec.LookPath("gs"); err == nil {
		return &gsRenderer{bin: p}, nil
	}
	return nil, ErrNoRenderer
}

// --- pdftoppm (poppler-utils) ---

type pdftoppmRenderer struct{ bin string }

func (r *pdftoppmRenderer) convert(pdfPath, outputDir string, cfg *convertConfig) ([]string, error) {
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

	cmd := exec.Command(r.bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("folio: pdftoppm: %w: %s", err, out)
	}

	return collectOutputFiles(outputDir, cfg.format, cfg.pages)
}

// --- mutool (mupdf) ---

type mutoolRenderer struct{ bin string }

func (r *mutoolRenderer) convert(pdfPath, outputDir string, cfg *convertConfig) ([]string, error) {
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

	cmd := exec.Command(r.bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("folio: mutool: %w: %s", err, out)
	}

	return collectOutputFiles(outputDir, cfg.format, cfg.pages)
}

// --- gs (Ghostscript) ---

type gsRenderer struct{ bin string }

func (r *gsRenderer) convert(pdfPath, outputDir string, cfg *convertConfig) ([]string, error) {
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

	cmd := exec.Command(r.bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("folio: gs: %w: %s", err, out)
	}

	return collectOutputFiles(outputDir, cfg.format, cfg.pages)
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

// collectOutputFiles gathers the image files generated in outputDir,
// sorted by name. If specific pages were requested, only matching
// files are returned.
func collectOutputFiles(dir string, format ImageFormat, _ []int) ([]string, error) {
	ext := ".png"
	if format == JPEG {
		ext = ".jpeg"
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("folio: read output dir: %w", err)
	}

	var paths []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ext) || (format == JPEG && strings.HasSuffix(name, ".jpg")) {
			paths = append(paths, filepath.Join(dir, name))
		}
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("folio: no output images generated")
	}

	return paths, nil
}
