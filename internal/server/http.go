package server

import (
	"bytes"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/magicrew/doc7/internal/detect"
	"github.com/magicrew/doc7/internal/extract"
	"github.com/magicrew/doc7/internal/render"
)

const (
	maxMultipartOverhead = int64(8 * 1024 * 1024)
	maxFormFieldBytes    = int64(64 * 1024)
	maxUploadNameRunes   = 180
)

type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/v1/jobs", s.authorize(s.handleJobs))
	mux.HandleFunc("/v1/jobs/", s.authorize(s.handleJob))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		expected := strings.TrimSpace(s.config.AuthToken)
		if expected == "" {
			next(w, r)
			return
		}
		scheme, provided, ok := strings.Cut(strings.TrimSpace(r.Header.Get("Authorization")), " ")
		provided = strings.TrimSpace(provided)
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			provided = ""
		}
		if len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="doc7"`)
			writeAPIError(w, http.StatusUnauthorized, "unauthorized", "a valid bearer token is required")
			return
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	queued, running := s.manager.Stats()
	writeJSON(w, http.StatusOK, HealthResponse{
		OK:      true,
		Service: "doc7",
		Version: s.config.ServiceVersion,
		Queued:  queued,
		Running: running,
	})
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handleCreateJob(w, r)
	case http.MethodGet:
		s.handleListJobs(w, r)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		writeAPIError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be multipart/form-data")
		return
	}
	requestLimit := s.config.MaxUploadBytes + maxMultipartOverhead
	if requestLimit < s.config.MaxUploadBytes {
		requestLimit = s.config.MaxUploadBytes
	}
	r.Body = http.MaxBytesReader(w, r.Body, requestLimit)
	reader, err := r.MultipartReader()
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_multipart", err.Error())
		return
	}

	merge := true
	promptName := "auto"
	pages := ""
	inputName := ""
	temporaryPath := ""
	var uploadedBytes int64
	defer func() {
		if temporaryPath != "" {
			_ = os.Remove(temporaryPath)
		}
	}()

	for {
		part, partErr := reader.NextPart()
		if errors.Is(partErr, io.EOF) {
			break
		}
		if partErr != nil {
			var maxErr *http.MaxBytesError
			if errors.As(partErr, &maxErr) {
				writeAPIError(w, http.StatusRequestEntityTooLarge, "upload_too_large", fmt.Sprintf("request exceeds %d bytes", s.config.MaxUploadBytes))
			} else {
				writeAPIError(w, http.StatusBadRequest, "invalid_multipart", partErr.Error())
			}
			return
		}
		switch part.FormName() {
		case "file":
			if temporaryPath != "" {
				_ = part.Close()
				writeAPIError(w, http.StatusBadRequest, "multiple_files", "exactly one file is allowed")
				return
			}
			inputName, err = normalizeUploadName(part.FileName())
			if err != nil {
				_ = part.Close()
				writeAPIError(w, http.StatusBadRequest, "invalid_filename", err.Error())
				return
			}
			temporary, createErr := os.CreateTemp(s.manager.TempDir(), ".doc7-upload-*")
			if createErr != nil {
				_ = part.Close()
				writeAPIError(w, http.StatusInternalServerError, "upload_failed", createErr.Error())
				return
			}
			temporaryPath = temporary.Name()
			uploadedBytes, err = io.Copy(temporary, io.LimitReader(part, s.config.MaxUploadBytes+1))
			closeErr := temporary.Close()
			_ = part.Close()
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, "upload_failed", err.Error())
				return
			}
			if closeErr != nil {
				writeAPIError(w, http.StatusInternalServerError, "upload_failed", closeErr.Error())
				return
			}
			if uploadedBytes > s.config.MaxUploadBytes {
				writeAPIError(w, http.StatusRequestEntityTooLarge, "upload_too_large", fmt.Sprintf("file exceeds %d bytes", s.config.MaxUploadBytes))
				return
			}
		case "prompt":
			value, readErr := readField(part)
			_ = part.Close()
			if readErr != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_prompt", readErr.Error())
				return
			}
			promptName = strings.TrimSpace(value)
		case "pages":
			value, readErr := readField(part)
			_ = part.Close()
			if readErr != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_pages", readErr.Error())
				return
			}
			pages, err = render.NormalizePageSelection(value)
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_pages", err.Error())
				return
			}
		case "merge":
			value, readErr := readField(part)
			_ = part.Close()
			if readErr != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_merge", readErr.Error())
				return
			}
			merge, err = strconv.ParseBool(strings.TrimSpace(value))
			if err != nil {
				writeAPIError(w, http.StatusBadRequest, "invalid_merge", "merge must be true or false")
				return
			}
		default:
			_, _ = io.Copy(io.Discard, io.LimitReader(part, maxFormFieldBytes+1))
			_ = part.Close()
		}
	}

	if temporaryPath == "" {
		writeAPIError(w, http.StatusBadRequest, "file_required", "multipart field file is required")
		return
	}
	if promptName == "" {
		promptName = "auto"
	}
	if promptName != "auto" {
		if _, err := extract.Prompt(promptName); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_prompt", err.Error())
			return
		}
	}
	job, err := s.manager.CreateJob(inputName, promptName, pages, merge, temporaryPath, uploadedBytes)
	if err != nil {
		if errors.Is(err, ErrQueueFull) {
			writeAPIError(w, http.StatusServiceUnavailable, "queue_full", err.Error())
		} else {
			writeAPIError(w, http.StatusInternalServerError, "job_create_failed", err.Error())
		}
		return
	}
	temporaryPath = ""
	writeJSON(w, http.StatusAccepted, jobResponse(job))
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			writeAPIError(w, http.StatusBadRequest, "invalid_limit", "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}
	jobs := s.manager.List(limit)
	responses := make([]JobResponse, 0, len(jobs))
	for _, job := range jobs {
		responses = append(responses, jobResponse(job))
	}
	writeJSON(w, http.StatusOK, ListResponse{Jobs: responses})
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/jobs/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || !validJobID(parts[0]) {
		writeAPIError(w, http.StatusNotFound, "job_not_found", "job not found")
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		job, err := s.manager.Snapshot(id)
		if err != nil {
			writeManagerError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, jobResponse(job))
		return
	}
	if len(parts) != 2 {
		writeAPIError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if parts[1] == "resume" {
		if r.Method != http.MethodPost {
			methodNotAllowed(w, http.MethodPost)
			return
		}
		s.handleResume(w, r, id)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	switch parts[1] {
	case "markdown":
		s.handleMarkdown(w, r, id)
	case "artifacts":
		s.handleArtifacts(w, r, id)
	default:
		writeAPIError(w, http.StatusNotFound, "not_found", "endpoint not found")
	}
}

func (s *Server) handleResume(w http.ResponseWriter, r *http.Request, id string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormFieldBytes)
	request := ResumeRequest{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "invalid_resume", err.Error())
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeAPIError(w, http.StatusBadRequest, "invalid_resume", "request body must contain one JSON object")
		return
	}
	pages, err := render.NormalizePageSelection(request.Pages)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_pages", err.Error())
		return
	}
	job, err := s.manager.ResumeJob(id, pages)
	if err != nil {
		if errors.Is(err, ErrQueueFull) {
			writeAPIError(w, http.StatusServiceUnavailable, "queue_full", err.Error())
		} else {
			writeManagerError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, jobResponse(job))
}

func (s *Server) handleMarkdown(w http.ResponseWriter, r *http.Request, id string) {
	data, err := s.manager.ReadMarkdown(id)
	if err != nil {
		writeManagerError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="document.md"`)
	http.ServeContent(w, r, "document.md", time.Time{}, bytes.NewReader(data))
}

func (s *Server) handleArtifacts(w http.ResponseWriter, r *http.Request, id string) {
	file, info, err := s.manager.Artifact(id)
	if err != nil {
		writeManagerError(w, err)
		return
	}
	defer file.Close()
	name := "doc7-" + id + ".zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func jobResponse(job Job) JobResponse {
	base := "/v1/jobs/" + job.ID
	links := JobLinks{Status: base}
	if job.Status == StatusFailed || job.Status == StatusSucceeded {
		links.Resume = base + "/resume"
	}
	if job.Status == StatusSucceeded {
		links.Artifacts = base + "/artifacts"
		if job.Merge && job.Result != nil && job.Result.Mode == "document" {
			links.Markdown = base + "/markdown"
		}
	}
	return JobResponse{Job: job, Links: links}
}

func normalizeUploadName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("uploaded file must have a filename")
	}
	if filepath.Base(value) != value || strings.ContainsAny(value, `/\`) {
		return "", errors.New("uploaded filename must not contain a path")
	}
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return "", errors.New("uploaded filename contains unsupported characters")
		}
		builder.WriteRune(r)
	}
	name := strings.Trim(builder.String(), " .")
	if name == "" || len([]rune(name)) > maxUploadNameRunes {
		return "", errors.New("uploaded filename is empty or too long")
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if isWindowsReservedName(base) {
		return "", errors.New("uploaded filename is reserved on Windows")
	}
	if !strings.EqualFold(filepath.Ext(name), ".zip") && !detect.IsSupportedFile(name) {
		return "", errors.New("unsupported uploaded file type: " + filepath.Ext(name))
	}
	return name, nil
}

func isWindowsReservedName(value string) bool {
	upper := strings.ToUpper(strings.TrimSpace(value))
	switch upper {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}

func readField(reader io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxFormFieldBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > maxFormFieldBytes {
		return "", errors.New("form field is too large")
	}
	return string(data), nil
}

func validJobID(value string) bool {
	if len(value) != jobIDBytes*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func writeManagerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrJobNotFound):
		writeAPIError(w, http.StatusNotFound, "job_not_found", err.Error())
	case errors.Is(err, ErrJobNotReady):
		writeAPIError(w, http.StatusConflict, "job_not_ready", err.Error())
	case errors.Is(err, ErrJobActive):
		writeAPIError(w, http.StatusConflict, "job_active", err.Error())
	case errors.Is(err, ErrResumeRejected):
		writeAPIError(w, http.StatusConflict, "resume_rejected", err.Error())
	default:
		writeAPIError(w, http.StatusInternalServerError, "server_error", err.Error())
	}
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
}

func writeAPIError(w http.ResponseWriter, status int, code string, message string) {
	value := apiError{}
	value.Error.Code = code
	value.Error.Message = message
	writeJSON(w, status, value)
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
