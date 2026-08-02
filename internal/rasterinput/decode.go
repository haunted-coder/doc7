package rasterinput

import (
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"path/filepath"
	"strings"

	"github.com/hhrutter/tiff"
	"golang.org/x/image/bmp"
)

const maxTIFFPages = 4096

type Reader interface {
	io.Reader
	io.ReaderAt
}

func DecodePages(reader Reader, size int64, filename string, mediaType string) ([]image.Image, bool, error) {
	switch rasterFormat(filename, mediaType) {
	case "bmp":
		decoded, err := bmp.Decode(reader)
		if err != nil {
			return nil, true, fmt.Errorf("failed to decode BMP image: %w", err)
		}
		return []image.Image{decoded}, true, nil
	case "tiff":
		pages, err := decodeTIFF(reader, size)
		if err != nil {
			return nil, true, err
		}
		return pages, true, nil
	default:
		return nil, false, nil
	}
}

func rasterFormat(filename string, mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.SplitN(mediaType, ";", 2)[0])) {
	case "image/bmp", "image/x-ms-bmp":
		return "bmp"
	case "image/tiff":
		return "tiff"
	}
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".bmp":
		return "bmp"
	case ".tif", ".tiff":
		return "tiff"
	default:
		return ""
	}
}

func decodeTIFF(reader Reader, size int64) ([]image.Image, error) {
	offsets, err := tiffPageOffsets(reader, size)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect TIFF pages: %w", err)
	}
	pages := make([]image.Image, 0, len(offsets))
	for index, offset := range offsets {
		decoded, err := tiff.DecodeAt(reader, offset)
		if err != nil {
			return nil, fmt.Errorf("failed to decode TIFF page %d: %w", index+1, err)
		}
		pages = append(pages, decoded)
	}
	return pages, nil
}

func tiffPageOffsets(reader io.ReaderAt, size int64) ([]int64, error) {
	var header [8]byte
	if _, err := reader.ReadAt(header[:], 0); err != nil {
		return nil, err
	}
	var order binary.ByteOrder
	switch string(header[:4]) {
	case "II*\x00":
		order = binary.LittleEndian
	case "MM\x00*":
		order = binary.BigEndian
	case "II+\x00", "MM\x00+":
		return nil, fmt.Errorf("BigTIFF is not supported")
	default:
		return nil, fmt.Errorf("invalid TIFF header")
	}

	offset := int64(order.Uint32(header[4:8]))
	seen := map[int64]struct{}{}
	offsets := make([]int64, 0, 1)
	for offset != 0 {
		if len(offsets) >= maxTIFFPages {
			return nil, fmt.Errorf("TIFF page count exceeds %d", maxTIFFPages)
		}
		if offset < 8 || offset > size-2 {
			return nil, fmt.Errorf("TIFF IFD offset is outside the file: %d", offset)
		}
		if _, exists := seen[offset]; exists {
			return nil, fmt.Errorf("TIFF IFD chain contains a cycle")
		}
		seen[offset] = struct{}{}

		var countBytes [2]byte
		if _, err := reader.ReadAt(countBytes[:], offset); err != nil {
			return nil, err
		}
		count := int64(order.Uint16(countBytes[:]))
		nextPosition := offset + 2 + count*12
		if nextPosition < offset || nextPosition > size-4 {
			return nil, fmt.Errorf("TIFF IFD entries are outside the file")
		}
		var nextBytes [4]byte
		if _, err := reader.ReadAt(nextBytes[:], nextPosition); err != nil {
			return nil, err
		}
		offsets = append(offsets, offset)
		offset = int64(order.Uint32(nextBytes[:]))
	}
	if len(offsets) == 0 {
		return nil, fmt.Errorf("TIFF contains no pages")
	}
	return offsets, nil
}
