// Package thai provides built-in Thai language support for folio.
//
// It bundles a Thai word segmenter (dictionary-based, ~15K words) that
// enables proper line wrapping at word boundaries in Thai text. No
// external dependencies are required.
//
// Usage:
//
//	import "github.com/akkaraponph/folio/thai"
//
//	doc := folio.New()
//	thai.Setup(doc) // installs Thai word breaker
//
// This replaces the previous approach of importing github.com/veer66/mapkha.
package thai

import (
	"github.com/akkaraponph/folio"
	"github.com/akkaraponph/folio/internal/wordcut"
)

// wc is the package-level wordcut instance (initialized once).
var wc *wordcut.Wordcut

func init() {
	wc = wordcut.New()
}

// Setup installs the Thai word breaker on the document so that
// MultiCell, Write, and other text-wrapping functions break lines
// at Thai word boundaries instead of mid-word.
func Setup(doc *folio.Document) {
	doc.SetWordBreaker(func(paragraph string) []string {
		return wc.Segment(paragraph)
	})
}

// Segment splits Thai text into word-boundary tokens. This can be
// used directly if you need the tokens outside of folio's text layout.
func Segment(text string) []string {
	return wc.Segment(text)
}
