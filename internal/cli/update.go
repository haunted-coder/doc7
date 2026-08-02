package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/magicrew/doc7/internal/vlm"
	"github.com/spf13/cobra"
)

const updateRepository = "magicrew/doc7"

type releaseInfo struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func newUpdateCommand() *cobra.Command {
	var check bool
	command := &cobra.Command{
		Use:   "update",
		Short: translate("update.short"),
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return updateCLI(cmd.Context(), check, globals.Yes)
		},
		Annotations: map[string]string{"doc7.command": "update"},
	}
	command.Flags().BoolVar(&check, "check", false, translate("update.check_flag"))
	return command
}

func newInternalUpdateCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "__update-apply <source> <target>",
		Hidden: true,
		Args:   cobra.ExactArgs(3),
		RunE: func(_ *cobra.Command, args []string) error {
			time.Sleep(time.Second)
			if err := replaceExecutable(args[0], args[1]); err != nil {
				return err
			}
			return os.RemoveAll(args[2])
		},
	}
}

func updateCLI(ctx context.Context, check bool, confirmed bool) error {
	release, err := fetchLatestRelease(ctx)
	if err != nil {
		return vlm.NewError(vlm.ConfigError, translate("update.fetch_failed"), false, err)
	}
	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(buildVersion, "v")
	if release.Draft || release.Prerelease {
		return vlm.NewError(vlm.ConfigError, translate("update.invalid_release"), false, nil)
	}
	if compareVersions(current, latest) >= 0 {
		writeText("%s", translate("update.current", buildVersion))
		return nil
	}
	if check {
		writeText("%s", translate("update.available", buildVersion, release.TagName))
		return nil
	}
	if !confirmed {
		if !stdinIsTerminal() {
			return vlm.NewError(vlm.ConfigError, translate("update.non_interactive"), false, nil)
		}
		fmt.Fprintf(os.Stdout, "%s [y/N]: ", translate("update.confirm", release.TagName))
		var answer string
		if _, err := fmt.Fscanln(os.Stdin, &answer); err != nil || (strings.ToLower(answer) != "y" && strings.ToLower(answer) != "yes") {
			return vlm.NewError(vlm.ConfigError, translate("update.cancelled"), false, nil)
		}
	}
	return downloadAndInstall(ctx, release.TagName)
}

func compareVersions(current string, latest string) int {
	currentParts, currentOK := versionParts(current)
	latestParts, latestOK := versionParts(latest)
	if !currentOK {
		return -1
	}
	if !latestOK {
		return 1
	}
	for index := 0; index < len(currentParts); index++ {
		switch {
		case currentParts[index] > latestParts[index]:
			return 1
		case currentParts[index] < latestParts[index]:
			return -1
		}
	}
	return 0
}

func versionParts(value string) ([3]int, bool) {
	var parts [3]int
	segments := strings.SplitN(value, ".", 4)
	if len(segments) != 3 {
		return parts, false
	}
	for index, segment := range segments {
		if segment == "" {
			return parts, false
		}
		for _, character := range segment {
			if character < '0' || character > '9' {
				return parts, false
			}
		}
		number, err := strconv.Atoi(segment)
		if err != nil {
			return parts, false
		}
		parts[index] = number
	}
	return parts, true
}

func fetchLatestRelease(ctx context.Context) (releaseInfo, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+updateRepository+"/releases/latest", nil)
	if err != nil {
		return releaseInfo{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "doc7-updater")
	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return releaseInfo{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return releaseInfo{}, fmt.Errorf("GitHub returned HTTP %d", response.StatusCode)
	}
	var release releaseInfo
	if err := json.NewDecoder(response.Body).Decode(&release); err != nil {
		return releaseInfo{}, err
	}
	if release.TagName == "" {
		return releaseInfo{}, errors.New("latest release has no tag")
	}
	return release, nil
}

func downloadAndInstall(ctx context.Context, tag string) error {
	archiveName := releaseArchiveName(tag)
	tempDir, err := os.MkdirTemp("", "doc7-update-")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tempDir)
		}
	}()
	archivePath := filepath.Join(tempDir, archiveName)
	checksumsPath := filepath.Join(tempDir, "checksums.txt")
	baseURL := "https://github.com/" + updateRepository + "/releases/download/" + tag + "/"
	if err := downloadFile(ctx, baseURL+archiveName, archivePath); err != nil {
		return vlm.NewError(vlm.ConfigError, translate("update.download_failed"), false, err)
	}
	if err := downloadFile(ctx, baseURL+"checksums.txt", checksumsPath); err != nil {
		return vlm.NewError(vlm.ConfigError, translate("update.download_failed"), false, err)
	}
	expected, err := checksumFor(checksumsPath, archiveName)
	if err != nil {
		return err
	}
	actual, err := checksumFile(archivePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expected, actual) {
		return vlm.NewError(vlm.ConfigError, translate("update.checksum_failed"), false, nil)
	}
	newBinary, err := extractBinary(archivePath, tempDir)
	if err != nil {
		return err
	}
	if err := os.Chmod(newBinary, 0o755); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		if err := scheduleWindowsReplacement(newBinary, executable, tempDir); err != nil {
			return err
		}
		cleanup = false
		writeText("%s", translate("update.scheduled"))
		return nil
	}
	if err := replaceExecutable(newBinary, executable); err != nil {
		return err
	}
	writeText("%s", translate("update.complete", tag))
	return nil
}

func releaseArchiveName(tag string) string {
	extension := ".tar.gz"
	if runtime.GOOS == "windows" {
		extension = ".zip"
	}
	return fmt.Sprintf("doc7_%s_%s_%s%s", tag, runtime.GOOS, runtime.GOARCH, extension)
}

func downloadFile(ctx context.Context, source string, target string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", "doc7-updater")
	response, err := (&http.Client{Timeout: 10 * time.Minute}).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned HTTP %d", response.StatusCode)
	}
	file, err := os.Create(target)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func checksumFor(path string, name string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		candidate := strings.TrimPrefix(strings.TrimPrefix(fields[1], "*"), "./")
		if candidate == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum missing for %s", name)
}

func checksumFile(path string) (string, error) {
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

func extractBinary(archivePath string, directory string) (string, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZipBinary(archivePath, directory)
	}
	return extractTarBinary(archivePath, directory)
}

func extractZipBinary(path string, directory string) (string, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	for _, entry := range archive.File {
		if filepath.Base(entry.Name) != binaryName() {
			continue
		}
		file, err := entry.Open()
		if err != nil {
			return "", err
		}
		target := filepath.Join(directory, binaryName()+".new")
		output, err := os.Create(target)
		if err == nil {
			_, err = io.Copy(output, file)
			closeErr := output.Close()
			if err == nil {
				err = closeErr
			}
		}
		file.Close()
		return target, err
	}
	return "", errors.New("release archive does not contain doc7 binary")
}

func extractTarBinary(path string, directory string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if filepath.Base(header.Name) != binaryName() || header.Typeflag != tar.TypeReg {
			continue
		}
		target := filepath.Join(directory, binaryName()+".new")
		output, err := os.Create(target)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(output, reader)
		closeErr := output.Close()
		if copyErr != nil {
			return "", copyErr
		}
		return target, closeErr
	}
	return "", errors.New("release archive does not contain doc7 binary")
}

func binaryName() string {
	if runtime.GOOS == "windows" {
		return "doc7.exe"
	}
	return "doc7"
}

func scheduleWindowsReplacement(source string, target string, temporaryDirectory string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	command := exec.Command(executable, "__update-apply", source, target, temporaryDirectory)
	command.Stdin = nil
	command.Stdout = nil
	command.Stderr = nil
	return command.Start()
}

func replaceExecutable(source string, target string) error {
	if runtime.GOOS == "windows" {
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Rename(source, target)
}
