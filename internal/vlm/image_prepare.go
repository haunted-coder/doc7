package vlm

import (
	"fmt"
	"image"
	"image/color"
	stdDraw "image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	xdraw "golang.org/x/image/draw"
)

const defaultMaxImageBytes int64 = 9 * 1024 * 1024

func prepareOversizeImage(path string, maxBytes int64) (string, string, func(), error) {
	magick, err := exec.LookPath("magick")
	if err != nil {
		return prepareOversizeImageNative(path, maxBytes)
	}
	return prepareOversizeImageWithImageMagick(path, maxBytes, magick)
}

func prepareImageForDimension(path string, maxDimension int) (string, string, func(), error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", nil, err
	}
	img, _, err := image.Decode(file)
	_ = file.Close()
	if err != nil {
		return "", "", nil, NewError(RenderError, "failed to decode image for context fallback", false, err)
	}
	if maxDimension <= 0 || max(img.Bounds().Dx(), img.Bounds().Dy()) <= maxDimension {
		return path, "", func() {}, nil
	}
	tmp, err := os.MkdirTemp("", "doc7-vlm-context-image-*")
	if err != nil {
		return "", "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	output := filepath.Join(tmp, "request.jpg")
	out, err := os.Create(output)
	if err != nil {
		cleanup()
		return "", "", nil, err
	}
	err = jpeg.Encode(out, resizeToMax(img, maxDimension), &jpeg.Options{Quality: 92})
	closeErr := out.Close()
	if err != nil {
		cleanup()
		return "", "", nil, NewError(RenderError, "failed to encode context fallback image", false, err)
	}
	if closeErr != nil {
		cleanup()
		return "", "", nil, closeErr
	}
	return output, "image/jpeg", cleanup, nil
}

func prepareOversizeImageWithImageMagick(path string, maxBytes int64, magick string) (string, string, func(), error) {
	tmp, err := os.MkdirTemp("", "doc7-vlm-image-*")
	if err != nil {
		return "", "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	widths := []int{3600, 3200, 2800, 2400, 2000, 1600}
	qualities := []int{90, 85, 80, 75, 70}
	var lastSize int64
	for _, width := range widths {
		for _, quality := range qualities {
			output := filepath.Join(tmp, "request_"+strconv.Itoa(width)+"_"+strconv.Itoa(quality)+".jpg")
			cmd := exec.Command(
				magick,
				path,
				"-auto-orient",
				"-resize", strconv.Itoa(width)+"x"+strconv.Itoa(width)+">",
				"-background", "white",
				"-alpha", "remove",
				"-strip",
				"-quality", strconv.Itoa(quality),
				output,
			)
			if outputBytes, err := cmd.CombinedOutput(); err != nil {
				cleanup()
				return "", "", nil, NewError(RenderError, "failed to compress oversize image: "+string(outputBytes), false, err)
			}
			info, err := os.Stat(output)
			if err != nil {
				cleanup()
				return "", "", nil, err
			}
			lastSize = info.Size()
			if info.Size() <= maxBytes {
				return output, "image/jpeg", cleanup, nil
			}
		}
	}
	cleanup()
	return "", "", nil, NewError(ModelError, fmt.Sprintf("compressed image still exceeds provider limit: %d bytes > %d bytes", lastSize, maxBytes), false, nil)
}

func prepareOversizeImageNative(path string, maxBytes int64) (string, string, func(), error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", nil, err
	}
	img, _, err := image.Decode(file)
	_ = file.Close()
	if err != nil {
		return "", "", nil, NewError(RenderError, "failed to decode oversize image for native compression", false, err)
	}
	tmp, err := os.MkdirTemp("", "doc7-vlm-image-*")
	if err != nil {
		return "", "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	widths := []int{3600, 3200, 2800, 2400, 2000, 1600, 1200}
	qualities := []int{90, 85, 80, 75, 70, 65}
	var lastSize int64
	for _, width := range widths {
		resized := resizeToMax(img, width)
		for _, quality := range qualities {
			output := filepath.Join(tmp, "request_"+strconv.Itoa(width)+"_"+strconv.Itoa(quality)+".jpg")
			out, err := os.Create(output)
			if err != nil {
				cleanup()
				return "", "", nil, err
			}
			err = jpeg.Encode(out, resized, &jpeg.Options{Quality: quality})
			closeErr := out.Close()
			if err != nil {
				cleanup()
				return "", "", nil, NewError(RenderError, "failed to encode native compressed image", false, err)
			}
			if closeErr != nil {
				cleanup()
				return "", "", nil, closeErr
			}
			info, err := os.Stat(output)
			if err != nil {
				cleanup()
				return "", "", nil, err
			}
			lastSize = info.Size()
			if info.Size() <= maxBytes {
				return output, "image/jpeg", cleanup, nil
			}
		}
	}
	cleanup()
	return "", "", nil, NewError(ModelError, fmt.Sprintf("native compressed image still exceeds provider limit: %d bytes > %d bytes", lastSize, maxBytes), false, nil)
}

func resizeToMax(src image.Image, maxDim int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	scaleWidth := width
	scaleHeight := height
	if width > maxDim || height > maxDim {
		scaleWidth = maxDim
		scaleHeight = height * maxDim / width
		if height > width {
			scaleHeight = maxDim
			scaleWidth = width * maxDim / height
		}
	}
	if scaleWidth <= 0 {
		scaleWidth = 1
	}
	if scaleHeight <= 0 {
		scaleHeight = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, scaleWidth, scaleHeight))
	stdDraw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, stdDraw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, stdDraw.Over, nil)
	return dst
}
