// Package captcha generates local animated GIF challenges without external services.
package captcha

import (
	"bytes"
	"crypto/rand"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"math/big"
)

const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ"

const Width = 120
const Height = 44
const glyphScale = 3
const glyphStart = 10
const glyphStep = 28
const glyphTop = 11

var glyphs = map[byte][]string{
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "10001"}, 'B': {"11110", "10001", "10001", "11110", "10001", "10001", "11110"}, 'C': {"01111", "10000", "10000", "10000", "10000", "10000", "01111"}, 'D': {"11110", "10001", "10001", "10001", "10001", "10001", "11110"}, 'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"}, 'F': {"11111", "10000", "10000", "11110", "10000", "10000", "10000"}, 'G': {"01111", "10000", "10000", "10111", "10001", "10001", "01111"}, 'H': {"10001", "10001", "10001", "11111", "10001", "10001", "10001"}, 'J': {"00111", "00010", "00010", "00010", "10010", "10010", "01100"}, 'K': {"10001", "10010", "10100", "11000", "10100", "10010", "10001"}, 'L': {"10000", "10000", "10000", "10000", "10000", "10000", "11111"}, 'M': {"10001", "11011", "10101", "10101", "10001", "10001", "10001"}, 'N': {"10001", "11001", "11001", "10101", "10011", "10011", "10001"}, 'P': {"11110", "10001", "10001", "11110", "10000", "10000", "10000"}, 'Q': {"01110", "10001", "10001", "10001", "10101", "10010", "01101"}, 'R': {"11110", "10001", "10001", "11110", "10100", "10010", "10001"}, 'S': {"01111", "10000", "10000", "01110", "00001", "00001", "11110"}, 'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"}, 'U': {"10001", "10001", "10001", "10001", "10001", "10001", "01110"}, 'V': {"10001", "10001", "10001", "10001", "10001", "01010", "00100"}, 'W': {"10001", "10001", "10001", "10101", "10101", "10101", "01010"}, 'X': {"10001", "10001", "01010", "00100", "01010", "10001", "10001"}, 'Y': {"10001", "10001", "01010", "00100", "00100", "00100", "00100"}, 'Z': {"11111", "00001", "00010", "00100", "01000", "10000", "11111"},
}

func Generate(answer string) (string, []byte, error) {
	if answer == "" {
		for i := 0; i < 4; i++ {
			n, e := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
			if e != nil {
				return "", nil, errors.New("captcha random generation failed")
			}
			answer += string(alphabet[n.Int64()])
		}
	}
	if len(answer) != 4 {
		return "", nil, errors.New("captcha requires four letters")
	}
	for i := 0; i < 4; i++ {
		if _, ok := glyphs[answer[i]]; !ok {
			return "", nil, errors.New("unsupported captcha letter")
		}
	}
	palette := color.Palette{color.RGBA{250, 251, 253, 255}, color.RGBA{30, 45, 68, 255}}
	anim := &gif.GIF{LoopCount: 0, BackgroundIndex: 0, Config: image.Config{ColorModel: palette, Width: Width, Height: Height}}
	for frame := 0; frame < 4; frame++ {
		img := image.NewPaletted(image.Rect(0, 0, Width, Height), palette)
		for index := 0; index < 4; index++ {
			if index == frame {
				continue
			}
			x0 := glyphStart + index*glyphStep
			y0 := glyphTop
			for y, row := range glyphs[answer[index]] {
				for x, b := range row {
					if b != '1' {
						continue
					}
					for dy := 0; dy < glyphScale; dy++ {
						for dx := 0; dx < glyphScale; dx++ {
							img.SetColorIndex(x0+x*glyphScale+dx, y0+y*glyphScale+dy, 1)
						}
					}
				}
			}
		}
		anim.Image = append(anim.Image, img)
		anim.Delay = append(anim.Delay, 45)
		anim.Disposal = append(anim.Disposal, gif.DisposalBackground)
	}
	var buf bytes.Buffer
	if e := gif.EncodeAll(&buf, anim); e != nil {
		return "", nil, errors.New("captcha image encoding failed")
	}
	return answer, buf.Bytes(), nil
}
