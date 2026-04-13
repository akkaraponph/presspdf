package main

import (
	"fmt"
	"log"

	"github.com/akkaraponph/folio"
)

func main() {
	// Merge multiple PDFs into one (pure Go, no external tools).
	err := folio.MergePDF("output/combined.pdf",
		"../pdf/14_markdown.pdf",
		"../pdf/05_toc.pdf",
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Merged into output/combined.pdf")
}
