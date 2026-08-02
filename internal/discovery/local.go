package discovery

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/magicrew/doc7/internal/vlm"
)

type Server struct {
	Name    string
	BaseURL string
}

type Candidate struct {
	ServerName string
	BaseURL    string
	Model      string
}

var localServers = []Server{
	{Name: "LM Studio", BaseURL: "http://127.0.0.1:1234/v1"},
	{Name: "Ollama", BaseURL: "http://127.0.0.1:11434/v1"},
}

func LocalModels(ctx context.Context) []Candidate {
	return LocalModelsFiltered(ctx, "")
}

func LocalModelsFiltered(ctx context.Context, runtimeName string) []Candidate {
	var wait sync.WaitGroup
	var mutex sync.Mutex
	candidates := make([]Candidate, 0)
	for _, server := range localServers {
		if !matchesRuntime(server.Name, runtimeName) {
			continue
		}
		server := server
		wait.Add(1)
		go func() {
			defer wait.Done()
			probeContext, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer cancel()
			models, err := vlm.ListModelsOpenAICompatible(probeContext, vlm.Config{
				BaseURL: server.BaseURL,
				Timeout: 1500 * time.Millisecond,
			}, nil)
			if err != nil {
				return
			}
			mutex.Lock()
			defer mutex.Unlock()
			for _, model := range models {
				if strings.TrimSpace(model.ID) == "" {
					continue
				}
				candidates = append(candidates, Candidate{
					ServerName: server.Name,
					BaseURL:    server.BaseURL,
					Model:      model.ID,
				})
			}
		}()
	}
	wait.Wait()
	sort.Slice(candidates, func(left int, right int) bool {
		if candidates[left].ServerName == candidates[right].ServerName {
			return candidates[left].Model < candidates[right].Model
		}
		return candidates[left].ServerName < candidates[right].ServerName
	})
	return candidates
}

func matchesRuntime(serverName string, runtimeName string) bool {
	switch strings.ToLower(strings.TrimSpace(runtimeName)) {
	case "", "any":
		return true
	case "lm_studio":
		return serverName == "LM Studio"
	case "ollama":
		return serverName == "Ollama"
	default:
		return false
	}
}

func VerifyVision(ctx context.Context, candidate Candidate) error {
	return VerifyVisionWithAPIKey(ctx, candidate, "")
}

func VerifyVisionWithAPIKey(ctx context.Context, candidate Candidate, apiKey string) error {
	path, cleanup, err := probeImage()
	if err != nil {
		return err
	}
	defer cleanup()
	client, err := vlm.NewOpenAICompatible(vlm.Config{
		Provider:      "openai-compatible",
		BaseURL:       candidate.BaseURL,
		Model:         candidate.Model,
		APIKey:        apiKey,
		ImageDetail:   "low",
		MaxImageBytes: 1024 * 1024,
		MaxTokens:     512,
		Timeout:       30 * time.Second,
	}, nil)
	if err != nil {
		return err
	}
	response, err := client.Complete(ctx, vlm.Request{
		Prompt:      "Reply only DOC7_VISION_OK if the image contains a black square centered on a white background.",
		ImagePath:   path,
		ImageMIME:   "image/png",
		ImageDetail: "low",
	})
	if err != nil {
		return err
	}
	if !strings.Contains(strings.ToUpper(response.Content), "DOC7_VISION_OK") {
		return vlm.NewError(vlm.ConfigError, "the selected model did not confirm image understanding", false, nil)
	}
	return nil
}

func probeImage() (string, func(), error) {
	file, err := os.CreateTemp("", "doc7-vision-probe-*.png")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.Remove(file.Name()) }
	canvas := image.NewRGBA(image.Rect(0, 0, 96, 96))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(24, 24, 72, 72), &image.Uniform{C: color.Black}, image.Point{}, draw.Src)
	if err := png.Encode(file, canvas); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return file.Name(), cleanup, nil
}
