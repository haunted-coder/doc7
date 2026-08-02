package render

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/magicrew/doc7/internal/rasterinput"
	"github.com/magicrew/doc7/internal/vlm"
	_ "golang.org/x/image/webp"
)

func renderSingleImage(path string, imagesDir string) ([]PageImage, error) {
	return renderImageFile(path, imagesDir, 1)
}

func renderImageDir(files []string, imagesDir string) ([]PageImage, error) {
	pages := make([]PageImage, 0, len(files))
	for _, path := range files {
		filePages, err := renderImageFile(path, imagesDir, len(pages)+1)
		if err != nil {
			return nil, err
		}
		pages = append(pages, filePages...)
	}
	return pages, nil
}

func renderImageFile(path string, imagesDir string, firstPage int) ([]PageImage, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".bmp", ".tif", ".tiff":
		return renderNormalizedImage(path, imagesDir, firstPage)
	default:
		page, err := copyImage(path, imagesDir, firstPage)
		if err != nil {
			return nil, err
		}
		return []PageImage{page}, nil
	}
}

func renderNormalizedImage(path string, imagesDir string, firstPage int) ([]PageImage, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, vlm.NewError(vlm.RenderError, "failed to open raster image", false, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, vlm.NewError(vlm.RenderError, "failed to inspect raster image", false, err)
	}
	decodedPages, normalized, err := rasterinput.DecodePages(file, info.Size(), path, "")
	if err != nil {
		return nil, vlm.NewError(vlm.RenderError, "failed to normalize raster image", false, err)
	}
	if !normalized {
		return nil, vlm.NewError(vlm.RenderError, "unsupported raster normalization format", false, nil)
	}
	pages := make([]PageImage, 0, len(decodedPages))
	for index, decoded := range decodedPages {
		page, err := writePNGPage(decoded, path, imagesDir, firstPage+index)
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
	}
	return pages, nil
}

func writePNGPage(decoded image.Image, sourcePath string, imagesDir string, pageNumber int) (PageImage, error) {
	destination := filepath.Join(imagesDir, pageName(pageNumber, ".png"))
	file, err := os.Create(destination)
	if err != nil {
		return PageImage{}, vlm.NewError(vlm.RenderError, "failed to create normalized image", false, err)
	}
	encodeErr := png.Encode(file, decoded)
	closeErr := file.Close()
	if encodeErr != nil {
		return PageImage{}, vlm.NewError(vlm.RenderError, "failed to encode normalized image", false, encodeErr)
	}
	if closeErr != nil {
		return PageImage{}, vlm.NewError(vlm.RenderError, "failed to close normalized image", false, closeErr)
	}
	hash, err := hashFile(destination)
	if err != nil {
		return PageImage{}, err
	}
	width, height := imageSize(destination)
	return PageImage{Page: pageNumber, SourcePath: sourcePath, ImagePath: destination, Width: width, Height: height, SHA256: hash}, nil
}

func copyImage(src string, imagesDir string, pageNumber int) (PageImage, error) {
	ext := strings.ToLower(filepath.Ext(src))
	if ext == "" {
		ext = ".png"
	}
	dst := filepath.Join(imagesDir, pageName(pageNumber, ext))
	if err := copyFile(src, dst); err != nil {
		return PageImage{}, vlm.NewError(vlm.RenderError, "failed to copy image", false, err)
	}
	hash, err := hashFile(dst)
	if err != nil {
		return PageImage{}, err
	}
	width, height := imageSize(dst)
	return PageImage{Page: pageNumber, SourcePath: src, ImagePath: dst, Width: width, Height: height, SHA256: hash}, nil
}

func copyFile(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func pageName(page int, ext string) string {
	return strings.TrimSuffix("page_"+leftPad3(page), ".") + ext
}

func leftPad3(value int) string {
	if value < 10 {
		return "00" + itoa(value)
	}
	if value < 100 {
		return "0" + itoa(value)
	}
	return itoa(value)
}

func itoa(value int) string {
	return strconvItoa(value)
}
