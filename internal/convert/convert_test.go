package convert

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareLibreOfficeInputCopiesToShortASCIIName(t *testing.T) {
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "（V3） 20260122 东莞市情（汇总）改14（封面加长）-最后改2（改格式）.docx")
	sourceData := []byte("docx payload")
	if err := os.WriteFile(sourcePath, sourceData, 0o644); err != nil {
		t.Fatal(err)
	}

	workDir := t.TempDir()
	prepared, err := prepareLibreOfficeInput(sourcePath, workDir)
	if err != nil {
		t.Fatal(err)
	}

	wantPath := filepath.Join(workDir, "input.docx")
	if prepared != wantPath {
		t.Fatalf("prepared path = %q, want %q", prepared, wantPath)
	}
	gotData, err := os.ReadFile(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotData) != string(sourceData) {
		t.Fatalf("prepared file data = %q, want %q", gotData, sourceData)
	}
}

func TestFindProducedPDFUsesActualLibreOfficeOutputName(t *testing.T) {
	outputDir := t.TempDir()
	actualPDF := filepath.Join(outputDir, "input.pdf")
	if err := os.WriteFile(actualPDF, []byte("%PDF-1.7"), 0o644); err != nil {
		t.Fatal(err)
	}

	produced, err := findProducedPDF(outputDir)
	if err != nil {
		t.Fatal(err)
	}
	if produced != actualPDF {
		t.Fatalf("produced PDF = %q, want %q", produced, actualPDF)
	}
}
