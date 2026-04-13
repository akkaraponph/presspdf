package main

import (
	"fmt"
	"log"
	"os"

	"github.com/akkaraponph/folio"
)

func main() {
	// Collect image paths from command-line arguments.
	images := os.Args[1:]
	if len(images) == 0 {
		fmt.Println("Usage: jpg2pdf image1.jpg image2.png ...")
		fmt.Println("\nCreates output.pdf with one page per image.")
		os.Exit(1)
	}

	// Auto-fit: each page sized to its image.
	err := folio.ImagesToPDF("output.pdf", images)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created output.pdf (%d page(s))\n", len(images))

	// Example: fixed A4 pages with margins.
	err = folio.ImagesToPDF("output_a4.pdf", images,
		folio.ImagePageSize(folio.A4),
		folio.ImageMargin(36), // 0.5 inch margin
		folio.ImageFit("fit"),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created output_a4.pdf (%d page(s), A4)\n", len(images))
}
