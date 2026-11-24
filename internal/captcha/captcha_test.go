package captcha

import (
	"bytes"
	"image/gif"
	"testing"
)

func TestFourFramesHideOneLetter(t *testing.T) {
	a, b, e := Generate("TEST")
	if e != nil || a != "TEST" {
		t.Fatal(e)
	}
	g, e := gif.DecodeAll(bytes.NewReader(b))
	if e != nil || len(g.Image) != 4 || g.Config.Width != Width || g.Config.Height != Height {
		t.Fatalf("GIF: %v", e)
	}
	for frame, img := range g.Image {
		if g.Delay[frame] != 45 || g.Disposal[frame] != gif.DisposalBackground {
			t.Fatal("animation timing or disposal")
		}
		for index := 0; index < 4; index++ {
			ink := 0
			for y := 0; y < Height; y++ {
				for x := glyphStart + index*glyphStep; x < glyphStart+index*glyphStep+5*glyphScale; x++ {
					if img.ColorIndexAt(x, y) != 0 {
						ink++
					}
				}
			}
			if index == frame && ink != 0 {
				t.Fatal("hidden glyph left pixels")
			}
			if index != frame && ink < 60 {
				t.Fatal("visible glyph missing")
			}
		}
	}
	for index := 0; index < 4; index++ {
		var previous []byte
		for frame, img := range g.Image {
			if frame == index {
				continue
			}
			var patch []byte
			for y := glyphTop; y < glyphTop+7*glyphScale; y++ {
				for x := glyphStart + index*glyphStep; x < glyphStart+index*glyphStep+5*glyphScale; x++ {
					patch = append(patch, img.ColorIndexAt(x, y))
				}
			}
			if previous != nil && !bytes.Equal(previous, patch) {
				t.Fatal("letter shifted between frames")
			}
			previous = patch
		}
	}
}
func TestRandomLettersAndValidation(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 15; i++ {
		a, _, e := Generate("")
		if e != nil || len(a) != 4 {
			t.Fatal(e)
		}
		for _, c := range a {
			if c < 'A' || c > 'Z' || c == 'I' || c == 'O' {
				t.Fatal("ambiguous alphabet")
			}
		}
		seen[a] = true
	}
	if len(seen) < 2 {
		t.Fatal("not random")
	}
	for _, s := range []string{"1234", "ABC", "ABCDE", "IOIO"} {
		if _, _, e := Generate(s); e == nil {
			t.Fatal("invalid text accepted")
		}
	}
}
