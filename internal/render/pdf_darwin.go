//go:build darwin

package render

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/magicrew/doc7/internal/vlm"
)

func renderPDFWithPDFKit(ctx context.Context, path string, outputDir string, dpi int) error {
	swift, err := exec.LookPath("swift")
	if err != nil {
		return vlm.NewError(vlm.DependencyError, "macOS PDFKit fallback requires swift", false, err)
	}
	tmp, err := os.MkdirTemp("", "doc7-pdfkit-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	scriptPath := filepath.Join(tmp, "render_pdf.swift")
	if err := os.WriteFile(scriptPath, []byte(pdfKitSwiftRenderer), 0o644); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, swift, scriptPath, path, outputDir, fmt.Sprint(dpi))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return vlm.NewError(vlm.RenderError, "PDFKit renderer failed: "+strings.TrimSpace(string(output)), false, err)
	}
	return nil
}

const pdfKitSwiftRenderer = `
import AppKit
import Foundation
import ImageIO
import PDFKit

func fail(_ message: String) -> Never {
    FileHandle.standardError.write((message + "\n").data(using: .utf8)!)
    exit(1)
}

guard CommandLine.arguments.count == 4 else {
    fail("usage: render_pdf.swift <pdf> <output_dir> <dpi>")
}

let pdfPath = CommandLine.arguments[1]
let outputDir = CommandLine.arguments[2]
let dpi = Double(CommandLine.arguments[3]) ?? 220.0
let scale = CGFloat(dpi / 72.0)
let pdfURL = URL(fileURLWithPath: pdfPath)

guard let document = PDFDocument(url: pdfURL) else {
    fail("failed to open PDF: \(pdfPath)")
}

try FileManager.default.createDirectory(
    at: URL(fileURLWithPath: outputDir),
    withIntermediateDirectories: true
)

for index in 0..<document.pageCount {
    guard let page = document.page(at: index) else {
        fail("failed to read page \(index + 1)")
    }
    let bounds = page.bounds(for: .mediaBox)
    let pixelWidth = max(1, Int(ceil(bounds.width * scale)))
    let pixelHeight = max(1, Int(ceil(bounds.height * scale)))
    let targetSize = NSSize(width: pixelWidth, height: pixelHeight)
    let pageImage = page.thumbnail(of: targetSize, for: .mediaBox)
    guard let cgImage = pageImage.cgImage(forProposedRect: nil, context: nil, hints: nil) else {
        fail("failed to create image for page \(index + 1)")
    }

    let filename = String(format: "page_%03d.png", index + 1)
    let outputURL = URL(fileURLWithPath: outputDir).appendingPathComponent(filename)
    guard let destination = CGImageDestinationCreateWithURL(outputURL as CFURL, "public.png" as CFString, 1, nil) else {
        fail("failed to create PNG destination for page \(index + 1)")
    }
    CGImageDestinationAddImage(destination, cgImage, nil)
    if !CGImageDestinationFinalize(destination) {
        fail("failed to encode page \(index + 1) as PNG")
    }
}
`
