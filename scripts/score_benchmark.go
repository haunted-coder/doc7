package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/magicrew/doc7/internal/vlm"
)

type specification struct {
	Case         string       `json:"case"`
	Input        artifact     `json:"input"`
	Systems      []system     `json:"systems"`
	Capabilities []capability `json:"capabilities"`
}

type artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type system struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Output string `json:"output"`
	SHA256 string `json:"sha256"`
	Mode   string `json:"mode,omitempty"`
	Note   string `json:"note,omitempty"`
}

type capability struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Patterns []string `json:"patterns"`
}

type score struct {
	System       string          `json:"system"`
	Label        string          `json:"label"`
	Output       string          `json:"output"`
	OutputSHA256 string          `json:"output_sha256"`
	OutputBytes  int             `json:"output_bytes"`
	Scored       bool            `json:"scored"`
	Note         string          `json:"note,omitempty"`
	Passed       int             `json:"passed"`
	Total        int             `json:"total"`
	Capabilities map[string]bool `json:"capabilities"`
}

func main() {
	format := flag.String("format", "markdown", "output format: markdown or json")
	flag.Parse()
	if flag.NArg() != 1 {
		fatalf("usage: go run ./scripts/score_benchmark.go [-format markdown|json] <ground-truth.json>")
	}
	specPath, err := filepath.Abs(flag.Arg(0))
	if err != nil {
		fatalf("failed to resolve specification path: %v", err)
	}
	spec := readSpecification(specPath)
	root := filepath.Dir(specPath)
	readVerifiedArtifact(root, spec.Input, "benchmark input")
	scores := evaluate(spec, root)
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "markdown":
		writeMarkdown(spec, scores)
	case "json":
		writeJSON(spec, scores)
	default:
		fatalf("format must be markdown or json")
	}
}

func readSpecification(path string) specification {
	data, err := os.ReadFile(path)
	if err != nil {
		fatalf("failed to read %s: %v", path, err)
	}
	var spec specification
	if err := json.Unmarshal(data, &spec); err != nil {
		fatalf("failed to parse %s: %v", path, err)
	}
	if strings.TrimSpace(spec.Case) == "" || strings.TrimSpace(spec.Input.Path) == "" || len(spec.Systems) == 0 || len(spec.Capabilities) == 0 {
		fatalf("specification must define case, input, systems, and capabilities")
	}
	if !validSHA256(spec.Input.SHA256) {
		fatalf("benchmark input must define a valid SHA-256")
	}
	for _, system := range spec.Systems {
		if strings.TrimSpace(system.ID) == "" || strings.TrimSpace(system.Label) == "" || strings.TrimSpace(system.Output) == "" {
			fatalf("every system must define id, label, and output")
		}
		if !validSHA256(system.SHA256) {
			fatalf("system %s must define a valid SHA-256", system.ID)
		}
		mode := strings.ToLower(strings.TrimSpace(system.Mode))
		if mode != "" && mode != "score" && mode != "diagnostic" {
			fatalf("system %s mode must be score or diagnostic", system.ID)
		}
		if mode == "diagnostic" && strings.TrimSpace(system.Note) == "" {
			fatalf("diagnostic system %s must define a note", system.ID)
		}
	}
	return spec
}

func evaluate(spec specification, root string) []score {
	compiled := make(map[string][]*regexp.Regexp, len(spec.Capabilities))
	for _, capability := range spec.Capabilities {
		if capability.ID == "" || len(capability.Patterns) == 0 {
			fatalf("every capability must define an id and at least one pattern")
		}
		for _, pattern := range capability.Patterns {
			expression, err := regexp.Compile("(?is)" + pattern)
			if err != nil {
				fatalf("invalid pattern for %s: %v", capability.ID, err)
			}
			compiled[capability.ID] = append(compiled[capability.ID], expression)
		}
	}

	results := make([]score, 0, len(spec.Systems))
	for _, system := range spec.Systems {
		data, outputSHA256 := readVerifiedArtifact(root, artifact{Path: system.Output, SHA256: system.SHA256}, "output for "+system.ID)
		content, _ := vlm.StripLeadingReasoningBlocks(string(data))
		scored := !strings.EqualFold(strings.TrimSpace(system.Mode), "diagnostic")
		result := score{
			System:       system.ID,
			Label:        system.Label,
			Output:       system.Output,
			OutputSHA256: outputSHA256,
			OutputBytes:  len(data),
			Scored:       scored,
			Note:         strings.TrimSpace(system.Note),
			Total:        len(spec.Capabilities),
			Capabilities: make(map[string]bool, len(spec.Capabilities)),
		}
		if !scored {
			results = append(results, result)
			continue
		}
		for _, capability := range spec.Capabilities {
			passed := true
			for _, expression := range compiled[capability.ID] {
				if !expression.MatchString(content) {
					passed = false
					break
				}
			}
			result.Capabilities[capability.ID] = passed
			if passed {
				result.Passed++
			}
		}
		results = append(results, result)
	}
	return results
}

func readVerifiedArtifact(root string, value artifact, label string) ([]byte, string) {
	path := filepath.Join(root, filepath.FromSlash(value.Path))
	data, err := os.ReadFile(path)
	if err != nil {
		fatalf("failed to read %s at %s: %v", label, value.Path, err)
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	expected := strings.ToLower(strings.TrimSpace(value.SHA256))
	if actual != expected {
		fatalf("%s SHA-256 mismatch: expected %s, got %s", label, expected, actual)
	}
	return data, actual
}

func validSHA256(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func writeMarkdown(spec specification, scores []score) {
	fmt.Print("| System | Bytes |")
	for _, capability := range spec.Capabilities {
		fmt.Printf(" %s |", capability.Label)
	}
	fmt.Println(" Score |")
	fmt.Print("| --- | ---: |")
	for range spec.Capabilities {
		fmt.Print(" --- |")
	}
	fmt.Println(" ---: |")
	for _, result := range scores {
		fmt.Printf("| %s | %d |", result.Label, result.OutputBytes)
		if !result.Scored {
			for range spec.Capabilities {
				fmt.Print(" N/A |")
			}
			fmt.Println(" N/A |")
			continue
		}
		for _, capability := range spec.Capabilities {
			value := "no"
			if result.Capabilities[capability.ID] {
				value = "yes"
			}
			fmt.Printf(" %s |", value)
		}
		fmt.Printf(" %d/%d |\n", result.Passed, result.Total)
	}
}

func writeJSON(spec specification, scores []score) {
	value := struct {
		Case   string   `json:"case"`
		Input  artifact `json:"input"`
		Scores []score  `json:"scores"`
	}{Case: spec.Case, Input: spec.Input, Scores: scores}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fatalf("failed to encode scores: %v", err)
	}
	fmt.Println(string(data))
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
