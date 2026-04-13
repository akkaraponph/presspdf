package main

import (
	"fmt"
	"log"

	"github.com/akkaraponph/folio"
)

func main() {
	input := "../pdf/14_markdown.pdf"

	// 1. Template watermark — centered "DRAFT".
	err := folio.WatermarkPDF(input, "output/draft.pdf",
		folio.WatermarkTemplate("draft"),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Created output/draft.pdf")

	// 2. Confidential with custom opacity.
	err = folio.WatermarkPDF(input, "output/confidential.pdf",
		folio.WatermarkTemplate("confidential"),
		folio.WatermarkOpacity(0.15),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Created output/confidential.pdf")

	// 3. Repeating pattern watermark.
	err = folio.WatermarkPDF(input, "output/pattern.pdf",
		folio.WatermarkText("COPY"),
		folio.WatermarkPattern(180, 180),
		folio.WatermarkFontSize(36),
		folio.WatermarkOpacity(0.1),
		folio.WatermarkColor(200, 0, 0),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Created output/pattern.pdf")

	// 4. Custom position, no rotation.
	err = folio.WatermarkPDF(input, "output/custom.pdf",
		folio.WatermarkText("Company Inc."),
		folio.WatermarkPosition(400, 30),
		folio.WatermarkRotation(0),
		folio.WatermarkFontSize(14),
		folio.WatermarkOpacity(0.4),
		folio.WatermarkColor(100, 100, 100),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Created output/custom.pdf")
}
