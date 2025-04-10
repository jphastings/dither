package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "golang.org/x/image/webp"

	"github.com/EdlinOrg/prominentcolor"
	"github.com/edwvee/exiffix"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/makeworld-the-better-one/dither/v2"
	"github.com/nfnt/resize"
)

var _ color.Color = (*colorful.Color)(nil)

const (
	canvasSize        = 512
	colorCount        = 8
	fixedWidthPalette = true
)

var (
	forceBlack = os.Getenv("FORCE_BLACK") == "t"
	forceWhite = os.Getenv("FORCE_WHITE") == "t"
)

func check(in string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not process %s: %v\n", in, err)
		os.Exit(1)
	}
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Printf("Usage:\n  %s <image.ext>\n", os.Args[0])
		os.Exit(1)
	}
	input := os.Args[1]
	output := strings.TrimSuffix(input, filepath.Ext(input)) + ".dither.png"
	if _, err := os.Stat(output); err == nil {
		fmt.Fprintln(os.Stderr, output, "already exists.")
		os.Exit(0)
	}

	var pPal color.Palette
	if len(os.Args) >= 3 && os.Args[2][0] == '#' {
		pPal = parsePalette(os.Args[2])
	}

	dX := uint(16)
	_ = dX
	sX := float32(0.25)
	_ = sX
	k := colorCount

	f, err := os.Open(input)
	check(input, err)
	defer f.Close()

	img, _, err := exiffix.Decode(f)
	check(input, err)

	img = resize.Thumbnail(canvasSize, canvasSize, img, resize.Lanczos3)
	bb := img.Bounds()

	if pPal == nil {
		pPal, err = makePalette(img, k, forceBlack, forceWhite)
		check(input, err)
	}

	d := dither.NewDitherer(pPal)
	d.Mapper = dither.Bayer(dX, dX, sX) // Why not?
	// d.Mapper = dither.RandomNoiseGrayscale(-0.5, 0.5)
	// d.Mapper = dither.PixelMapperFromMatrix(dither.ClusteredDot4x4, sX)
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

	sizes := make(map[color.Color]int, len(pPal))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			sizes[img2.At(x, y)]++
		}
	}

	pos := 0
	for _, col := range pPal {
		size := float64(sizes[col]) / float64(width*height)

		var bbb image.Rectangle
		if landscape {
			blockW := width / colorCount
			if !fixedWidthPalette {
				blockW = int(size * float64(width))
			}

			// Add colorCount to hieght to ensure that even if all divisions round down we still cover the full side with palette
			bbb = image.Rect(pos, height, pos+blockW+colorCount, width)
			pos += blockW
		} else {
			blockH := height / colorCount
			if !fixedWidthPalette {
				blockH = int(size * float64(height))
			}

			// Add colorCount to width to ensure that even if all divisions round down we still cover the full side with palette
			bbb = image.Rect(width, pos, height, pos+blockH+colorCount)
			pos += blockH
		}
		draw.Draw(img2, bbb, &image.Uniform{C: col}, image.Point{}, draw.Src)
	}

	f3, err := os.Create(output)
	check(input, err)

	err = png.Encode(f3, img2)
	check(input, err)
}

func parsePalette(str string) (pPal color.Palette) {
	for _, hex := range strings.Split(str[1:], "#") {
		var c color.RGBA
		c.A = 0xff
		if _, err := fmt.Sscanf(hex, "%02x%02x%02x", &c.R, &c.G, &c.B); err != nil {
			panic(err)
		}
		pPal = append(pPal, c)
	}
	return pPal
}

func makePalette(img image.Image, k int, forceBlack, forceWhite bool) (color.Palette, error) {
	hX := 1.0
	cX := 1.5
	lX := 1.0

	if forceBlack {
		k--
	}

	if forceWhite {
		k--
	}

	cols, err := prominentcolor.KmeansWithAll(k, img,
		prominentcolor.ArgumentLAB|prominentcolor.ArgumentNoCropping,
		512, nil)
	if err != nil {
		return nil, err
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
			return nil, fmt.Errorf("couldn't transform colour")
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
		fmt.Printf("%s ", c.Hex())
	}
	fmt.Println()

	return pPal, nil
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
