// Package wordcut provides dictionary-based Thai word segmentation.
//
// It uses a shortest-path word graph algorithm over a prefix-tree
// dictionary to split Thai text into segments suitable for line wrapping.
// Non-Thai runs (Latin, whitespace, digits) are returned as single tokens.
package wordcut

import (
	"bufio"
	_ "embed"
	"strings"
)

//go:embed tdict-std.txt
var defaultDict string

// Wordcut segments Thai text into word-boundary tokens.
type Wordcut struct {
	tree *prefixTree
}

// New creates a Wordcut using the built-in Thai dictionary (~15K words).
func New() *Wordcut {
	words := loadEmbeddedDict()
	return &Wordcut{tree: buildPrefixTree(words)}
}

// NewFromWords creates a Wordcut from a custom word list.
func NewFromWords(words []string) *Wordcut {
	return &Wordcut{tree: buildPrefixTree(words)}
}

// Segment splits text into tokens at word boundaries.
// Thai words are segmented using the dictionary. Latin characters,
// digits, and whitespace are grouped into their own tokens.
func (w *Wordcut) Segment(text string) []string {
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	path := w.buildPath(runes)
	return pathToTokens(runes, path)
}

// --- edge types ---

type edgeType int

const (
	etInit  edgeType = iota
	etDict           // matched a dictionary word
	etLatin          // run of ASCII Latin letters
	etSpace          // run of whitespace / punctuation
	etUnk            // unknown (fallback single-char)
)

// --- edge (node in shortest-path graph) ---

type edge struct {
	src       int      // start index of this segment
	etype     edgeType // how this segment was matched
	wordCount int      // total segments on this path
	unkCount  int      // total unknown segments on this path
}

func (e *edge) betterThan(other *edge) bool {
	if e == nil {
		return false
	}
	if other == nil {
		return true
	}
	// Prefer fewer unknowns, then fewer total words.
	if e.unkCount != other.unkCount {
		return e.unkCount < other.unkCount
	}
	return e.wordCount < other.wordCount
}

// --- prefix tree ---

type treeKey struct {
	nodeID int
	offset int
	ch     rune
}

type treeVal struct {
	childID int
	isFinal bool
}

type prefixTree struct {
	tab map[treeKey]*treeVal
}

func buildPrefixTree(words []string) *prefixTree {
	tab := make(map[treeKey]*treeVal, len(words)*6)
	for i, word := range words {
		nodeID := 0
		runes := []rune(word)
		for j, ch := range runes {
			isFinal := j+1 == len(runes)
			key := treeKey{nodeID, j, ch}
			if v, ok := tab[key]; ok {
				nodeID = v.childID
				if isFinal {
					v.isFinal = true
				}
			} else {
				tab[key] = &treeVal{childID: i, isFinal: isFinal}
				nodeID = i
			}
		}
	}
	return &prefixTree{tab: tab}
}

func (t *prefixTree) lookup(nodeID, offset int, ch rune) (*treeVal, bool) {
	v, ok := t.tab[treeKey{nodeID, offset, ch}]
	return v, ok
}

// --- dict pointer (active prefix-tree traversal) ---

type dictPointer struct {
	nodeID int
	src    int // start position in rune slice
	offset int // how deep into the tree
	final  bool
}

// --- path builder (shortest-path word graph) ---

func (w *Wordcut) buildPath(runes []rune) []edge {
	n := len(runes)
	path := make([]edge, n+1)
	path[0] = edge{src: 0, etype: etInit}

	// Active dict pointers.
	pointers := make([]dictPointer, 0, 32)

	// Pattern matchers for Latin and space runs.
	var latinStart, spaceStart int
	latinActive, spaceActive := false, false
	leftBoundary := 0

	for i, ch := range runes {
		var best *edge

		// --- dictionary matching ---
		// Add a new pointer starting at position i.
		pointers = append(pointers, dictPointer{src: i})

		// Advance all pointers with the current character.
		alive := 0
		for k := range pointers {
			p := &pointers[k]
			v, ok := w.tree.lookup(p.nodeID, p.offset, ch)
			if !ok {
				continue
			}
			p.nodeID = v.childID
			p.offset++
			p.final = v.isFinal
			pointers[alive] = *p
			alive++

			if v.isFinal {
				s := 1 + i - p.offset
				src := &path[s]
				e := &edge{
					src:       s,
					etype:     etDict,
					wordCount: src.wordCount + 1,
					unkCount:  src.unkCount,
				}
				if e.betterThan(best) {
					best = e
				}
			}
		}
		pointers = pointers[:alive]

		// --- Latin run ---
		if isLatin(ch) {
			if !latinActive {
				latinStart = i
				latinActive = true
			}
			// Check if this is the end of the Latin run.
			if i+1 >= n || !isLatin(runes[i+1]) {
				src := &path[latinStart]
				e := &edge{
					src:       latinStart,
					etype:     etLatin,
					wordCount: src.wordCount + 1,
					unkCount:  src.unkCount,
				}
				if e.betterThan(best) {
					best = e
				}
				latinActive = false
			}
		} else {
			latinActive = false
		}

		// --- Space/punctuation run ---
		if isSpace(ch) {
			if !spaceActive {
				spaceStart = i
				spaceActive = true
			}
			if i+1 >= n || !isSpace(runes[i+1]) {
				src := &path[spaceStart]
				e := &edge{
					src:       spaceStart,
					etype:     etSpace,
					wordCount: src.wordCount + 1,
					unkCount:  src.unkCount,
				}
				if e.betterThan(best) {
					best = e
				}
				spaceActive = false
			}
		} else {
			spaceActive = false
		}

		// --- Unknown fallback ---
		if best == nil {
			src := &path[leftBoundary]
			best = &edge{
				src:       leftBoundary,
				etype:     etUnk,
				wordCount: src.wordCount + 1,
				unkCount:  src.unkCount + 1,
			}
		}

		if best.etype != etUnk {
			leftBoundary = i + 1
		}

		path[i+1] = *best
	}

	return path
}

func pathToTokens(runes []rune, path []edge) []string {
	// Walk backwards to collect ranges.
	n := len(path) - 1
	var ranges [][2]int
	for e := n; e > 0; {
		s := path[e].src
		ranges = append(ranges, [2]int{s, e})
		e = s
	}
	// Reverse to get forward order.
	tokens := make([]string, len(ranges))
	for i, r := range ranges {
		tokens[len(ranges)-1-i] = string(runes[r[0]:r[1]])
	}
	return tokens
}

// --- character classifiers ---

func isLatin(ch rune) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') ||
		(ch >= '0' && ch <= '9')
}

func isSpace(ch rune) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '"', '(', ')', '\u201C', '\u201D':
		return true
	}
	return false
}

// --- dictionary loader ---

func loadEmbeddedDict() []string {
	var words []string
	scanner := bufio.NewScanner(strings.NewReader(defaultDict))
	for scanner.Scan() {
		if line := scanner.Text(); len(line) > 0 {
			words = append(words, line)
		}
	}
	return words
}
