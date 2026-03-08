package compress

import (
	"bytes"
	"fmt"
	"image"
	"io"

	"github.com/disintegration/imaging"
)

const (
	maxInputBytes  int64 = 64 * 1024 * 1024 // 64MB
	maxImagePixels int64 = 50 * 1000 * 1000 // 5000万像素
)

// Options 压缩选项
type Options struct {
	MaxWidth  int
	MaxHeight int
	Quality   int
	Preserve  bool
}

// Result 压缩结果
type Result struct {
	Reader io.Reader
	Width  int
	Height int
	Format string
}

// CompressFile 基于尺寸与质量压缩（保持格式）
func CompressFile(input io.Reader, options *Options) (*Result, error) {
	data, err := readLimited(input, maxInputBytes)
	if err != nil {
		return nil, err
	}

	if err := validateDecodedConfig(data); err != nil {
		return nil, err
	}

	file, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	ow, oh := file.Bounds().Dx(), file.Bounds().Dy()
	tw, th := ow, oh
	if options != nil {
		if options.MaxWidth > 0 || options.MaxHeight > 0 {
			if options.Preserve || true {
				if options.MaxWidth > 0 && (ow > options.MaxWidth) {
					r := float64(options.MaxWidth) / float64(ow)
					tw = options.MaxWidth
					th = int(float64(oh) * r)
				}
				if options.MaxHeight > 0 && th > options.MaxHeight {
					r := float64(options.MaxHeight) / float64(th)
					th = options.MaxHeight
					tw = int(float64(tw) * r)
				}
			}
		}
	}
	out := imaging.Resize(file, tw, th, imaging.Lanczos)
	var buf bytes.Buffer
	switch format {
	case "jpeg", "jpg":
		if options != nil && options.Quality > 0 {
			if err := imaging.Encode(&buf, out, imaging.JPEG, imaging.JPEGQuality(options.Quality)); err != nil {
				return nil, err
			}
		} else {
			if err := imaging.Encode(&buf, out, imaging.JPEG, imaging.JPEGQuality(80)); err != nil {
				return nil, err
			}
		}
	case "png":
		if err := imaging.Encode(&buf, out, imaging.PNG); err != nil {
			return nil, err
		}
	default:
		if err := imaging.Encode(&buf, out, imaging.JPEG, imaging.JPEGQuality(80)); err != nil {
			return nil, err
		}
		format = "jpeg"
	}
	b := buf.Bytes()
	return &Result{Reader: bytes.NewReader(b), Width: tw, Height: th, Format: format}, nil
}

// CompressToTargetSize 目标大小压缩（简单迭代质量）
func CompressToTargetSize(reader io.Reader, targetSizeMB float64, options *Options) (*Result, error) {
	if targetSizeMB <= 0 {
		return nil, fmt.Errorf("invalid target size")
	}
	data, err := readLimited(reader, maxInputBytes)
	if err != nil {
		return nil, err
	}
	if err := validateDecodedConfig(data); err != nil {
		return nil, err
	}

	maxWidth, maxHeight := 0, 0
	if options != nil {
		maxWidth = options.MaxWidth
		maxHeight = options.MaxHeight
	}

	quality := 85
	if options != nil && options.Quality > 0 {
		quality = options.Quality
	}
	for q := quality; q >= 40; q -= 5 {
		res, err := CompressFile(bytes.NewReader(data), &Options{MaxWidth: maxWidth, MaxHeight: maxHeight, Quality: q, Preserve: true})
		if err != nil {
			return nil, err
		}
		buf, _ := io.ReadAll(res.Reader)
		if float64(len(buf)) <= targetSizeMB*1024*1024 {
			return &Result{Reader: bytes.NewReader(buf), Width: res.Width, Height: res.Height, Format: res.Format}, nil
		}
	}
	res, err := CompressFile(bytes.NewReader(data), &Options{MaxWidth: maxWidth, MaxHeight: maxHeight, Quality: 60, Preserve: true})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func readLimited(input io.Reader, limit int64) ([]byte, error) {
	lr := &io.LimitedReader{R: input, N: limit + 1}
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("input too large: max %d bytes", limit)
	}
	return data, nil
}

func validateDecodedConfig(data []byte) error {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode image config failed: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("invalid image dimension")
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return fmt.Errorf("image too large: pixel count exceeds limit")
	}
	return nil
}
