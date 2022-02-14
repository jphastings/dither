package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/EdlinOrg/prominentcolor"
	"github.com/edwvee/exiffix"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/makeworld-the-better-one/dither/v2"
	"github.com/nfnt/resize"
)

var _ color.Color = (*colorful.Color)(nil)

func main() {
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Printf("Usage:\n  %s <image.ext>\n", os.Args[0])
		os.Exit(1)
	}
	input := os.Args[1]
	output := strings.TrimSuffix(input, filepath.Ext(input)) + ".dither.png"

	forceBlack := false
	forceWhite := false
	hX := 1.0
	_ = hX
	cX := 1.5
	_ = cX
	lX := 1.0
	_ = lX
	dX := uint(16)
	_ = dX
	sX := float32(0.64)
	_ = sX
	k := 8

	f, err := os.Open(input)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	img, _, err := exiffix.Decode(f)
	if err != nil {
		panic(err)
	}

	img = resize.Thumbnail(512, 512, img, resize.Lanczos3)
	bb := img.Bounds()

	if forceBlack {
		k--
	}

	if forceWhite {
		k--
	}

	cols, err := prominentcolor.KmeansWithAll(k, img,
		prominentcolor.ArgumentLAB|prominentcolor.ArgumentNoCropping,
		128, prominentcolor.GetDefaultMasks())
	if err != nil {
		panic(err)
	}

	cPal := make([]colorful.Color, k)
	for i, col := range cols {
		rgb := color.RGBA{
			R: uint8(col.Color.R),
			G: uint8(col.Color.G),
			B: uint8(col.Color.B),
			A: 255,
		}
		col, ok := colorful.MakeColor(rgb)
		if !ok {
			panic("Couldn't transform colour!")
		}

		h, c, l := col.Hcl()
		col = colorful.Hcl(h*hX, c*cX, l*lX).Clamped()

		cPal[i] = col
	}

	if forceBlack {
		b, _ := colorful.MakeColor(color.Black)
		cPal = append(cPal, b)
	}

	if forceWhite {
		w, _ := colorful.MakeColor(color.White)
		cPal = append(cPal, w)
	}

	var pPal color.Palette
	for _, c := range sortColors(cPal) {
		pPal = append(pPal, c)
	}

	d := dither.NewDitherer(pPal)
	d.Mapper = dither.Bayer(dX, dX, sX) // Why not?
	//d.Mapper = dither.PixelMapperFromMatrix(dither.ClusteredDot4x4, sX)
	//d.Matrix = dither.FloydSteinberg
	//d.Matrix = dither.Atkinson

	// Open an image and save it as a dithered GIF

	imgd := d.Dither(img)
	height := bb.Max.Y
	width := bb.Max.X
	landscape := width > height
	if landscape {
		bb.Max.Y = width
	} else {
		bb.Max.X = height
	}

	img2 := image.NewPaletted(bb, pPal)
	draw.Draw(img2, img2.Bounds(), imgd, image.Point{}, draw.Src)

	blockW := width / k
	blockH := height / k
	for i, col := range pPal {
		var bbb image.Rectangle
		if landscape {
			bbb = image.Rect(i*blockW, height, (i+1)*blockW, width)
		} else {
			bbb = image.Rect(width, i*blockH, height, (i+1)*blockH)
		}
		draw.Draw(img2, bbb, &image.Uniform{C: col}, image.Point{}, draw.Src)
	}

	f3, err := os.Create(output)
	if err != nil {
		panic(err)
	}

	err = png.Encode(f3, img2)
	if err != nil {
		panic(err)
	}
}

func sortColors(cs1 []colorful.Color) []colorful.Color {
	cs2 := make([]colorful.Color, len(cs1))
	copy(cs2, cs1)
	sort.Slice(cs2, func(i, j int) bool {
		l1, c1, h1 := cs2[i].LuvLCh()
		l2, c2, h2 := cs2[j].LuvLCh()
		if c1 != c2 {
			return c1 < c2
		}
		if h1 != h2 {
			return h1 < h2
		}
		if l1 != l2 {
			return l1 < l2
		}

		return false
	})
	return colorful.Sorted(cs2)
}
