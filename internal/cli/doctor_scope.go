package cli

import (
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/magicrew/doc7/internal/detect"
)

type doctorScope struct {
	PDF          bool
	Office       bool
	Presentation bool
	HTML         bool
	VLM          bool
}

func scopeForDoctorTarget(target string) (doctorScope, error) {
	if strings.TrimSpace(target) == "" {
		return doctorScope{VLM: true}, nil
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(target)), "http://") || strings.HasPrefix(strings.ToLower(strings.TrimSpace(target)), "https://") {
		parsed, err := url.Parse(strings.TrimSpace(target))
		if err != nil {
			return doctorScope{}, err
		}
		return scopeForExtension(filepath.Ext(parsed.Path)), nil
	}
	info, err := os.Stat(target)
	if err != nil {
		return doctorScope{}, err
	}
	if info.IsDir() {
		input, detectErr := detect.Detect(target)
		if detectErr != nil {
			return scopeForDirectory(target), nil
		}
		return scopeForKind(input.Kind), nil
	}
	return scopeForExtension(filepath.Ext(target)), nil
}

func scopeForDirectory(path string) doctorScope {
	scope := doctorScope{}
	found := false
	err := filepath.WalkDir(path, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		extension := strings.ToLower(filepath.Ext(current))
		if extension != ".zip" && !detect.IsSupportedFile(current) {
			return nil
		}
		found = true
		scope = mergeDoctorScopes(scope, scopeForExtension(extension))
		return nil
	})
	if err != nil || !found {
		return allDoctorScope()
	}
	return scope
}

func mergeDoctorScopes(left doctorScope, right doctorScope) doctorScope {
	return doctorScope{
		PDF:          left.PDF || right.PDF,
		Office:       left.Office || right.Office,
		Presentation: left.Presentation || right.Presentation,
		HTML:         left.HTML || right.HTML,
		VLM:          left.VLM || right.VLM,
	}
}

func scopeForExtension(extension string) doctorScope {
	if strings.EqualFold(strings.TrimSpace(extension), ".zip") {
		return allDoctorScope()
	}
	kind, ok := detect.KindForExtension(extension)
	if !ok {
		return allDoctorScope()
	}
	return scopeForKind(kind)
}

func scopeForKind(kind detect.Kind) doctorScope {
	switch {
	case kind == detect.KindPDF:
		return doctorScope{PDF: true, VLM: true}
	case detect.IsOffice(kind):
		return doctorScope{PDF: true, Office: true, Presentation: detect.IsPresentation(kind), VLM: true}
	case kind == detect.KindHTML || kind == detect.KindSVG:
		return doctorScope{PDF: true, HTML: true, VLM: true}
	case kind == detect.KindEPUB || kind == detect.KindEML || kind == detect.KindMHTML || kind == detect.KindMSG || kind == detect.KindIPYNB:
		return doctorScope{PDF: true, HTML: true, VLM: true}
	case kind == detect.KindHTMLSlides:
		return doctorScope{HTML: true, VLM: true}
	case kind == detect.KindImage || kind == detect.KindImageDir:
		return doctorScope{VLM: true}
	case detect.IsNative(kind):
		return doctorScope{}
	default:
		return allDoctorScope()
	}
}

func allDoctorScope() doctorScope {
	return doctorScope{PDF: true, Office: true, HTML: true, VLM: true}
}
