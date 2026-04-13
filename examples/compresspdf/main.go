package main

import (
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"math"
	"os"

	"github.com/akkaraponph/folio"
)

func main() {
	os.MkdirAll("output", 0o755)

	// Create a sample PDF with multiple pages.
	src := createSamplePDF()
	showSize("Original", src)

	// 1. Basic compression (FlateDecode + dedup).
	err := folio.CompressPDF(src, "output/compressed.pdf")
	if err != nil {
		log.Fatal(err)
	}
	showSize("Compressed (default)", "output/compressed.pdf")

	// 2. Compression with JPEG re-encoding at lower quality.
	imgPDF := createImagePDF()
	showSize("Image PDF (original)", imgPDF)

	err = folio.CompressPDF(imgPDF, "output/compressed_images.pdf",
		folio.CompressImageQuality(60),
	)
	if err != nil {
		log.Fatal(err)
	}
	showSize("Image PDF (quality=60)", "output/compressed_images.pdf")

	// 3. Compression without dedup.
	err = folio.CompressPDF(src, "output/compressed_nodedup.pdf",
		folio.CompressDedup(false),
	)
	if err != nil {
		log.Fatal(err)
	}
	showSize("Compressed (no dedup)", "output/compressed_nodedup.pdf")
}

func createSamplePDF() string {
	doc := folio.New()
	for i := 1; i <= 10; i++ {
		doc.AddPage(folio.A4)
		doc.SetFont("helvetica", "B", 24)
		doc.CurrentPage().Cell(0, 15, fmt.Sprintf("Page %d", i), "", "C", false, 1)
		doc.SetFont("helvetica", "", 12)
		for j := 0; j < 20; j++ {
			doc.CurrentPage().Cell(0, 8, "Lorem ipsum dolor sit amet, consectetur adipiscing elit.", "", "", false, 1)
		}
	}
	path := "output/sample_input.pdf"
	if err := doc.Save(path); err != nil {
		log.Fatal(err)
	}
	return path
}

func createImagePDF() string {
	jpegs := createSampleJPEGs()
	path := "output/sample_images.pdf"
	if err := folio.ImagesToPDF(path, jpegs); err != nil {
		log.Fatal(err)
	}
	return path
}

func createSampleJPEGs() []string {
	os.MkdirAll("output/tmp", 0o755)
	var paths []string
	for i := 0; i < 3; i++ {
		path := fmt.Sprintf("output/tmp/sample_%d.jpg", i)
		img := image.NewRGBA(image.Rect(0, 0, 640, 480))
		for y := 0; y < 480; y++ {
			for x := 0; x < 640; x++ {
				r := uint8(float64(x) / 640 * 255)
				g := uint8(float64(y) / 480 * 255)
				b := uint8(128 + 127*math.Sin(float64(x+y+i*100)*0.01))
				img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
			}
		}
		f, err := os.Create(path)
		if err != nil {
			log.Fatal(err)
		}
		jpeg.Encode(f, img, &jpeg.Options{Quality: 95})
		f.Close()
		paths = append(paths, path)
	}
	return paths
}

func showSize(label, path string) {
	info, err := os.Stat(path)
	if err != nil {
		fmt.Printf("  %-30s  (file not found)\n", label)
		return
	}
	fmt.Printf("  %-30s  %d bytes\n", label, info.Size())
}
