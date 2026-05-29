package captcha

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"image"
	"image/color"
	stddraw "image/draw"
	"image/png"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"

	_ "image/jpeg"
	_ "image/png"
)

type BackgroundSource interface {
	Pick(ctx context.Context, width int, height int) (image.Image, error)
}

type FileBackgroundSource struct {
	dir string
}

func NewFileBackgroundSource(dir string) *FileBackgroundSource {
	return &FileBackgroundSource{dir: strings.TrimSpace(dir)}
}

func (s *FileBackgroundSource) Pick(ctx context.Context, width int, height int) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.dir == "" {
		return nil, ErrInvalidToken
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		switch strings.ToLower(filepath.Ext(name)) {
		case ".jpg", ".jpeg", ".png":
			files = append(files, filepath.Join(s.dir, name))
		}
	}
	if len(files) == 0 {
		return nil, ErrInvalidToken
	}
	file, err := os.Open(files[randomInt(len(files))])
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	return resizeImage(img, width, height), nil
}

type generatedBackgroundSource struct{}

func (generatedBackgroundSource) Pick(ctx context.Context, width int, height int) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return generatedBackground(width, height), nil
}

func pickBackground(ctx context.Context, source BackgroundSource, width int, height int) (*image.RGBA, error) {
	if source == nil {
		source = generatedBackgroundSource{}
	}
	img, err := source.Pick(ctx, width, height)
	if err != nil {
		return nil, err
	}
	return resizeImage(img, width, height), nil
}

func resizeImage(src image.Image, width int, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

func generatedBackground(width int, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	c1 := color.RGBA{R: uint8(90 + randomInt(80)), G: uint8(110 + randomInt(90)), B: uint8(130 + randomInt(80)), A: 255}
	c2 := color.RGBA{R: uint8(150 + randomInt(70)), G: uint8(120 + randomInt(70)), B: uint8(90 + randomInt(90)), A: 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			t := float64(x+y) / float64(width+height)
			n := uint8(randomInt(18))
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(float64(c1.R)*(1-t) + float64(c2.R)*t + float64(n)),
				G: uint8(float64(c1.G)*(1-t) + float64(c2.G)*t + float64(n)),
				B: uint8(float64(c1.B)*(1-t) + float64(c2.B)*t + float64(n)),
				A: 255,
			})
		}
	}
	for i := 0; i < 10; i++ {
		drawCircle(img, randomInt(width), randomInt(height), 10+randomInt(24), color.RGBA{
			R: uint8(180 + randomInt(60)),
			G: uint8(180 + randomInt(60)),
			B: uint8(180 + randomInt(60)),
			A: uint8(35 + randomInt(45)),
		})
	}
	return img
}

func encodePNGDataURL(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func cloneRGBA(src image.Image) *image.RGBA {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	stddraw.Draw(dst, dst.Bounds(), src, bounds.Min, stddraw.Src)
	return dst
}

func drawRect(img *image.RGBA, rect image.Rectangle, c color.RGBA) {
	rect = rect.Intersect(img.Bounds())
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.SetRGBA(x, y, blend(img.RGBAAt(x, y), c))
		}
	}
}

func drawCircle(img *image.RGBA, cx int, cy int, r int, c color.RGBA) {
	r2 := r * r
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
				continue
			}
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r2 {
				img.SetRGBA(x, y, blend(img.RGBAAt(x, y), c))
			}
		}
	}
}

func blend(dst color.RGBA, src color.RGBA) color.RGBA {
	a := uint32(src.A)
	ia := uint32(255 - src.A)
	return color.RGBA{
		R: uint8((uint32(src.R)*a + uint32(dst.R)*ia) / 255),
		G: uint8((uint32(src.G)*a + uint32(dst.G)*ia) / 255),
		B: uint8((uint32(src.B)*a + uint32(dst.B)*ia) / 255),
		A: 255,
	}
}

func randomInt(max int) int {
	if max <= 0 {
		return 0
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}
