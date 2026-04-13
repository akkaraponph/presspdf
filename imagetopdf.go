package folio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/akkaraponph/folio/internal/state"
)

// ImageToPDFOption configures image-to-PDF conversion.
type ImageToPDFOption func(*imgPDFConfig)

type imgPDFConfig struct {
	pageSize *PageSize // nil = auto-fit to image
	dpi      float64   // resolution for auto-fit sizing (default: 96)
	margin   float64   // uniform margin in points (default: 0)
	fit      string    // "fit", "fill", "stretch" (default: "fit")
}

// ImagePageSize sets a fixed page size. Images are scaled to fit within
// the page. Default: each page is auto-sized to match the image.
func ImagePageSize(size PageSize) ImageToPDFOption {
	return func(c *imgPDFConfig) { ps := size; c.pageSize = &ps }
}

// ImageDPI sets the image resolution for calculating page dimensions
// in auto-fit mode. Higher DPI = smaller pages. Default: 96.
func ImageDPI(dpi float64) ImageToPDFOption {
	return func(c *imgPDFConfig) { c.dpi = dpi }
}

// ImageMargin sets a uniform margin around each image in points. Default: 0.
func ImageMargin(margin float64) ImageToPDFOption {
	return func(c *imgPDFConfig) { c.margin = margin }
}

// ImageFit controls how images are placed on fixed-size pages:
//   - "fit"     — scale to fit within page, preserving aspect ratio (default)
//   - "fill"    — scale to cover page, preserving aspect ratio (may crop)
//   - "stretch" — stretch to fill page exactly (may distort)
func ImageFit(mode string) ImageToPDFOption {
	return func(c *imgPDFConfig) { c.fit = mode }
}

// ImagesToPDF converts image files (JPEG or PNG) into a single PDF
// where each image becomes one page.
//
// This is a pure Go implementation — no external tools required.
func ImagesToPDF(outputPath string, imagePaths []string, opts ...ImageToPDFOption) error {
	if len(imagePaths) == 0 {
		return fmt.Errorf("folio: no images provided")
	}

	cfg := &imgPDFConfig{dpi: 96, fit: "fit"}
	for _, o := range opts {
		o(cfg)
	}

	// Ensure output directory exists.
	if dir := filepath.Dir(outputPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("folio: create output dir: %w", err)
		}
	}

	// Validate image paths and extensions.
	for _, p := range imagePaths {
		ext := strings.ToLower(filepath.Ext(p))
		if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
			return fmt.Errorf("folio: unsupported format %s (use JPEG or PNG)", ext)
		}
	}

	doc := New(WithUnit(state.UnitPt))

	for i, imgPath := range imagePaths {
		name := fmt.Sprintf("img%d", i)

		// Register image.
		f, err := os.Open(imgPath)
		if err != nil {
			return fmt.Errorf("folio: open %s: %w", imgPath, err)
		}
		err = doc.RegisterImage(name, f)
		f.Close()
		if err != nil {
			return fmt.Errorf("folio: register %s: %w", imgPath, err)
		}

		// Get pixel dimensions from the registry.
		entry, _ := doc.images.Get(name)
		imgWPt := float64(entry.Width) * 72.0 / cfg.dpi
		imgHPt := float64(entry.Height) * 72.0 / cfg.dpi

		var pageSize PageSize
		var drawX, drawY, drawW, drawH float64

		if cfg.pageSize != nil {
			pageSize = *cfg.pageSize
			availW := pageSize.WidthPt - 2*cfg.margin
			availH := pageSize.HeightPt - 2*cfg.margin

			switch cfg.fit {
			case "stretch":
				drawW, drawH = availW, availH
				drawX, drawY = cfg.margin, cfg.margin
			case "fill":
				scale := max(availW/imgWPt, availH/imgHPt)
				drawW = imgWPt * scale
				drawH = imgHPt * scale
				drawX = cfg.margin + (availW-drawW)/2
				drawY = cfg.margin + (availH-drawH)/2
			default: // "fit"
				scale := min(availW/imgWPt, availH/imgHPt)
				drawW = imgWPt * scale
				drawH = imgHPt * scale
				drawX = cfg.margin + (availW-drawW)/2
				drawY = cfg.margin + (availH-drawH)/2
			}
		} else {
			// Auto-fit: page sized to image + margins.
			pageSize = PageSize{
				WidthPt:  imgWPt + 2*cfg.margin,
				HeightPt: imgHPt + 2*cfg.margin,
			}
			drawX = cfg.margin
			drawY = cfg.margin
			drawW = imgWPt
			drawH = imgHPt
		}

		page := doc.AddPage(pageSize)
		page.DrawImageRect(name, drawX, drawY, drawW, drawH)
	}

	if err := doc.Save(outputPath); err != nil {
		return fmt.Errorf("folio: save PDF: %w", err)
	}
	return nil
}
