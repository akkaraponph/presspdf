package wordcut

import (
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	wc := New()
	if wc == nil {
		t.Fatal("New() returned nil")
	}
	if wc.tree == nil {
		t.Fatal("tree is nil")
	}
}

func TestSegment_ThaiWords(t *testing.T) {
	wc := New()
	// "สวัสดีครับ" = "สวัสดี" + "ครับ"
	tokens := wc.Segment("สวัสดีครับ")
	joined := strings.Join(tokens, "|")
	if joined != "สวัสดี|ครับ" {
		t.Errorf("got %q, want %q", joined, "สวัสดี|ครับ")
	}
}

func TestSegment_ThaiSentence(t *testing.T) {
	wc := New()
	// "ฉันกินข้าว" = "ฉัน" + "กิน" + "ข้าว"
	tokens := wc.Segment("ฉันกินข้าว")
	joined := strings.Join(tokens, "|")
	if joined != "ฉัน|กิน|ข้าว" {
		t.Errorf("got %q, want %q", joined, "ฉัน|กิน|ข้าว")
	}
}

func TestSegment_MixedThaiLatin(t *testing.T) {
	wc := New()
	tokens := wc.Segment("ฉันใช้Go")
	// Should separate Thai words from Latin run.
	found := false
	for _, tok := range tokens {
		if tok == "Go" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Go' as a token, got %v", tokens)
	}
}

func TestSegment_LatinOnly(t *testing.T) {
	wc := New()
	tokens := wc.Segment("hello world")
	joined := strings.Join(tokens, "|")
	if joined != "hello| |world" {
		t.Errorf("got %q, want %q", joined, "hello| |world")
	}
}

func TestSegment_Empty(t *testing.T) {
	wc := New()
	tokens := wc.Segment("")
	if len(tokens) != 0 {
		t.Errorf("expected nil/empty for empty input, got %v", tokens)
	}
}

func TestSegment_SingleThaiWord(t *testing.T) {
	wc := New()
	tokens := wc.Segment("กิน")
	if len(tokens) != 1 || tokens[0] != "กิน" {
		t.Errorf("expected [กิน], got %v", tokens)
	}
}

func TestSegment_Spaces(t *testing.T) {
	wc := New()
	tokens := wc.Segment("สวัสดี ครับ")
	// Space should be its own token.
	hasSpace := false
	for _, tok := range tokens {
		if tok == " " {
			hasSpace = true
		}
	}
	if !hasSpace {
		t.Errorf("expected space token, got %v", tokens)
	}
}

func TestNewFromWords(t *testing.T) {
	wc := NewFromWords([]string{"foo", "bar", "foobar"})
	tokens := wc.Segment("foobar")
	// Should prefer "foobar" over "foo"+"bar" (fewer words).
	joined := strings.Join(tokens, "|")
	if joined != "foobar" {
		t.Errorf("got %q, want %q", joined, "foobar")
	}
}

func TestNewFromWords_UnknownFallback(t *testing.T) {
	wc := NewFromWords([]string{"hello"})
	// "hello" + Thai unknown char that's not in the custom dict.
	tokens := wc.Segment("helloกก")
	if len(tokens) < 2 {
		t.Errorf("expected at least 2 tokens, got %v", tokens)
	}
	if tokens[0] != "hello" {
		t.Errorf("expected first token 'hello', got %q", tokens[0])
	}
}

func BenchmarkSegment(b *testing.B) {
	wc := New()
	text := "สวัสดีครับฉันกินข้าวที่ร้านอาหารใกล้บ้าน"
	b.ResetTimer()
	for range b.N {
		wc.Segment(text)
	}
}

func BenchmarkNew(b *testing.B) {
	for range b.N {
		New()
	}
}
