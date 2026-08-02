package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/magicrew/doc7/internal/config"
	"github.com/magicrew/doc7/internal/detect"
	"github.com/magicrew/doc7/internal/discovery"
	"github.com/magicrew/doc7/internal/remoteinput"
	"github.com/magicrew/doc7/internal/vlm"
	"github.com/mattn/go-isatty"
)

func ensureVisionConfig(ctx context.Context, cfg config.AppConfig, inputPath string) (config.AppConfig, error) {
	if !inputNeedsVision(inputPath) {
		return cfg, nil
	}
	return ensureConfiguredModel(ctx, cfg)
}

func ensureChatConfig(ctx context.Context, cfg config.AppConfig) (config.AppConfig, error) {
	return ensureConfiguredModel(ctx, cfg)
}

func ensureConfiguredModel(ctx context.Context, cfg config.AppConfig) (config.AppConfig, error) {
	if strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Model) != "" {
		var err error
		cfg, err = confirmRemoteEndpoint(cfg)
		if err != nil {
			return cfg, err
		}
		announceModel(cfg)
		return cfg, nil
	}
	if cfg.JSONOutput || !stdinIsTerminal() {
		return cfg, vlm.NewError(vlm.ConfigError, translate("model.non_interactive"), false, nil)
	}
	writeText("%s", translate("model.searching"))
	candidates := discovery.LocalModels(ctx)
	if len(candidates) == 0 {
		writeText("%s", translate("model.none"))
		return cfg, vlm.NewError(vlm.ConfigError, translate("model.none_help"), false, nil)
	}
	seenServers := map[string]bool{}
	for _, candidate := range candidates {
		if seenServers[candidate.BaseURL] {
			continue
		}
		seenServers[candidate.BaseURL] = true
		writeText("%s", translate("model.server_found", candidate.ServerName, candidate.BaseURL))
	}
	selected, err := selectModel(candidates)
	if err != nil {
		return cfg, err
	}
	writeText("%s", translate("model.verifying", selected.Model))
	if err := discovery.VerifyVision(ctx, selected); err != nil {
		return cfg, vlm.NewError(vlm.ConfigError, translate("model.verify_failed", err.Error()), false, err)
	}
	writeText("%s", translate("model.verified"))
	if _, err := config.SetUserValue(globals.ConfigPath, "provider", "openai-compatible"); err != nil {
		return cfg, vlm.NewError(vlm.ConfigError, "failed to save discovered provider", false, err)
	}
	if _, err := config.SetUserValue(globals.ConfigPath, "base_url", selected.BaseURL); err != nil {
		return cfg, vlm.NewError(vlm.ConfigError, "failed to save discovered endpoint", false, err)
	}
	if _, err := config.SetUserValue(globals.ConfigPath, "model", selected.Model); err != nil {
		return cfg, vlm.NewError(vlm.ConfigError, "failed to save discovered model", false, err)
	}
	cfg.Provider = "openai-compatible"
	cfg.BaseURL = selected.BaseURL
	cfg.Model = selected.Model
	writeText("%s", translate("model.saved", selected.Model))
	announceModel(cfg)
	return cfg, nil
}

func selectModel(candidates []discovery.Candidate) (discovery.Candidate, error) {
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	writeText("%s", translate("model.select"))
	for index, candidate := range candidates {
		writeText("  %d. %s · %s", index+1, candidate.Model, candidate.ServerName)
	}
	fmt.Fprintf(os.Stdout, "%s", translate("model.select_prompt", len(candidates)))
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return discovery.Candidate{}, vlm.NewError(vlm.ConfigError, translate("model.invalid_choice"), false, err)
	}
	choice, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || choice < 1 || choice > len(candidates) {
		return discovery.Candidate{}, vlm.NewError(vlm.ConfigError, translate("model.invalid_choice"), false, err)
	}
	return candidates[choice-1], nil
}

func inputNeedsVision(inputPath string) bool {
	if inputPath == "-" || remoteinput.IsHTTPURL(inputPath) {
		return true
	}
	input, err := detect.Detect(inputPath)
	if err != nil {
		return true
	}
	return !detect.IsNative(input.Kind)
}

func announceModel(cfg config.AppConfig) {
	if cfg.JSONOutput || cfg.Quiet || strings.TrimSpace(cfg.Model) == "" {
		return
	}
	writeText("%s", translate("model.using", translate("config."+endpointType(cfg.BaseURL)), cfg.Model, vlm.RedactedEndpoint(cfg.BaseURL)))
}

func confirmRemoteEndpoint(cfg config.AppConfig) (config.AppConfig, error) {
	if endpointType(cfg.BaseURL) == "local" || cfg.RemoteConfirmed {
		return cfg, nil
	}
	if globals.Yes {
		return saveRemoteConfirmation(cfg)
	}
	if !stdinIsTerminal() {
		return cfg, vlm.NewError(vlm.ConfigError, translate("remote.non_interactive"), false, nil)
	}
	writeText("%s", translate("remote.warning", vlm.RedactedEndpoint(cfg.BaseURL)))
	fmt.Fprintf(os.Stdout, "%s", translate("remote.confirm"))
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return cfg, vlm.NewError(vlm.ConfigError, translate("remote.denied"), false, err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" && answer != "是" {
		return cfg, vlm.NewError(vlm.ConfigError, translate("remote.denied"), false, nil)
	}
	return saveRemoteConfirmation(cfg)
}

func saveRemoteConfirmation(cfg config.AppConfig) (config.AppConfig, error) {
	if _, err := config.SetUserValue(globals.ConfigPath, "remote_confirmed", "true"); err != nil {
		return cfg, vlm.NewError(vlm.ConfigError, "failed to save remote endpoint confirmation", false, err)
	}
	cfg.RemoteConfirmed = true
	writeText("%s", translate("remote.saved"))
	return cfg, nil
}

func stdinIsTerminal() bool {
	fd := os.Stdin.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
