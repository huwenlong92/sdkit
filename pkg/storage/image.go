package storage

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"github.com/huwenlong92/sdkit/pkg/storage/core"
)

type ImageInfo struct {
	Width  int
	Height int
	Format string
}

type ImageLimit struct {
	MaxWidth  int
	MaxHeight int
	MinWidth  int
	MinHeight int
}

type CropRect struct {
	X      int
	Y      int
	Width  int
	Height int
}

func DecodeImageInfo(reader io.Reader) (ImageInfo, error) {
	cfg, format, err := image.DecodeConfig(reader)
	if err != nil {
		return ImageInfo{}, err
	}
	return ImageInfo{Width: cfg.Width, Height: cfg.Height, Format: format}, nil
}

func ValidateImageInfo(info ImageInfo, limit ImageLimit) error {
	if limit.MinWidth > 0 && info.Width < limit.MinWidth {
		return fmt.Errorf("image width too small")
	}
	if limit.MinHeight > 0 && info.Height < limit.MinHeight {
		return fmt.Errorf("image height too small")
	}
	if limit.MaxWidth > 0 && info.Width > limit.MaxWidth {
		return fmt.Errorf("image width too large")
	}
	if limit.MaxHeight > 0 && info.Height > limit.MaxHeight {
		return fmt.Errorf("image height too large")
	}
	return nil
}

func CropImage(reader io.Reader, writer io.Writer, rect CropRect, format string, quality int) error {
	img, sourceFormat, err := image.Decode(reader)
	if err != nil {
		return err
	}
	bounds := img.Bounds()
	cropBounds := image.Rect(rect.X, rect.Y, rect.X+rect.Width, rect.Y+rect.Height)
	if rect.Width <= 0 || rect.Height <= 0 || !cropBounds.In(bounds) {
		return fmt.Errorf("crop rect out of bounds")
	}
	subImage, ok := img.(interface {
		SubImage(r image.Rectangle) image.Image
	})
	if !ok {
		return fmt.Errorf("image format does not support crop")
	}
	if format == "" {
		format = sourceFormat
	}
	return encodeImage(writer, subImage.SubImage(cropBounds), format, quality)
}

func (fs *FileSystem) UploadCroppedImage(ctx context.Context, reader io.Reader, info core.FileInfo, rect CropRect, format string, quality int) UploadResult {
	return fs.UploadCroppedImageWithHook(ctx, reader, info, rect, format, quality)
}

func (fs *FileSystem) UploadCroppedImageWithHook(ctx context.Context, reader io.Reader, info core.FileInfo, rect CropRect, format string, quality int, opts ...HookOption) UploadResult {
	return fs.uploadCroppedImage(ctx, reader, info, rect, format, quality, collectHookOptions(opts...))
}

func (fs *FileSystem) uploadCroppedImage(ctx context.Context, reader io.Reader, info core.FileInfo, rect CropRect, format string, quality int, hooks operationHooks) UploadResult {
	var buf bytes.Buffer
	if err := CropImage(reader, &buf, rect, format, quality); err != nil {
		return UploadResult{Error: err}
	}
	if format != "" {
		info.Name = replaceImageExt(info.Name, format)
		info.MIMEType = mime.TypeByExtension(filepath.Ext(info.Name))
	}
	info.Size = int64(buf.Len())
	return fs.uploadStream(ctx, &buf, info, hooks)
}

func encodeImage(writer io.Writer, img image.Image, format string, quality int) error {
	switch strings.ToLower(format) {
	case "jpg", "jpeg":
		if quality <= 0 || quality > 100 {
			quality = 85
		}
		return jpeg.Encode(writer, img, &jpeg.Options{Quality: quality})
	case "png":
		return png.Encode(writer, img)
	default:
		return fmt.Errorf("unsupported image format: %s", format)
	}
}

func replaceImageExt(name, format string) string {
	if name == "" {
		return name
	}
	ext := ".png"
	switch strings.ToLower(format) {
	case "jpg", "jpeg":
		ext = ".jpg"
	}
	old := filepath.Ext(name)
	return strings.TrimSuffix(name, old) + ext
}
