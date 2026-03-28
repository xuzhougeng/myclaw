package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type desktopHTTPDevServer struct {
	app         *DesktopApp
	frontendDir string
}

func runHTTPDevServer(listenAddr string, app *DesktopApp) error {
	server := desktopHTTPDevServer{
		app:         app,
		frontendDir: filepath.Join("cmd", "myclaw-desktop", "frontend", "dist"),
	}

	app.startBackgroundServices()
	defer app.stopBackgroundServices()

	mux := http.NewServeMux()
	server.registerAPI(mux)
	server.registerFrontend(mux)

	log.Printf("desktop http dev server listening at http://%s", listenAddr)
	return http.ListenAndServe(listenAddr, mux)
}

func (s desktopHTTPDevServer) registerAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/api/overview", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			s.writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		result, err := s.app.GetOverview()
		s.writeResult(w, result, err)
	})

	mux.HandleFunc("/api/knowledge", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			result, err := s.app.ListKnowledge()
			s.writeResult(w, result, err)
		case http.MethodPost:
			var body struct {
				Text string `json:"text"`
			}
			if err := decodeJSONBody(r, &body); err != nil {
				s.writeError(w, http.StatusBadRequest, err)
				return
			}
			result, err := s.app.CreateKnowledge(body.Text)
			s.writeResult(w, result, err)
		default:
			s.writeMethodNotAllowed(w, http.MethodGet, http.MethodPost)
		}
	})

	mux.HandleFunc("/api/knowledge/append", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		var body struct {
			IDOrPrefix string `json:"idOrPrefix"`
			Addition   string `json:"addition"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := s.app.AppendKnowledge(body.IDOrPrefix, body.Addition)
		s.writeResult(w, result, err)
	})

	mux.HandleFunc("/api/knowledge/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		var body struct {
			IDOrPrefix string `json:"idOrPrefix"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := s.app.DeleteKnowledge(body.IDOrPrefix)
		s.writeResult(w, result, err)
	})

	mux.HandleFunc("/api/knowledge/clear", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		result, err := s.app.ClearKnowledge()
		s.writeResult(w, result, err)
	})

	mux.HandleFunc("/api/prompts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			result, err := s.app.ListPrompts()
			s.writeResult(w, result, err)
		case http.MethodPost:
			var body struct {
				Title   string `json:"title"`
				Content string `json:"content"`
			}
			if err := decodeJSONBody(r, &body); err != nil {
				s.writeError(w, http.StatusBadRequest, err)
				return
			}
			result, err := s.app.CreatePrompt(body.Title, body.Content)
			s.writeResult(w, result, err)
		default:
			s.writeMethodNotAllowed(w, http.MethodGet, http.MethodPost)
		}
	})

	mux.HandleFunc("/api/prompts/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		var body struct {
			IDOrPrefix string `json:"idOrPrefix"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := s.app.DeletePrompt(body.IDOrPrefix)
		s.writeResult(w, result, err)
	})

	mux.HandleFunc("/api/prompts/clear", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		result, err := s.app.ClearPrompts()
		s.writeResult(w, result, err)
	})

	mux.HandleFunc("/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		var body struct {
			Input string `json:"input"`
		}
		if err := decodeJSONBody(r, &body); err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := s.app.SendMessage(body.Input)
		s.writeResult(w, result, err)
	})

	mux.HandleFunc("/api/model", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			s.writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		result, err := s.app.GetModelSettings()
		s.writeResult(w, result, err)
	})

	mux.HandleFunc("/api/model/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		var body ModelConfigInput
		if err := decodeJSONBody(r, &body); err != nil {
			s.writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := s.app.SaveModelConfig(body)
		s.writeResult(w, result, err)
	})

	mux.HandleFunc("/api/model/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		result, err := s.app.TestModelConnection()
		s.writeResult(w, result, err)
	})

	mux.HandleFunc("/api/model/clear", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		result, err := s.app.ClearModelConfig()
		s.writeResult(w, result, err)
	})

	mux.HandleFunc("/api/import/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		result, err := s.handleImportUpload(r)
		s.writeResult(w, result, err)
	})

	mux.HandleFunc("/api/weixin/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			s.writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		s.writeJSON(w, http.StatusOK, s.app.GetWeixinStatus())
	})

	mux.HandleFunc("/api/weixin/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		result, err := s.app.StartWeixinLogin()
		s.writeResult(w, result, err)
	})

	mux.HandleFunc("/api/weixin/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		result, err := s.app.CancelWeixinLogin()
		s.writeResult(w, result, err)
	})

	mux.HandleFunc("/api/weixin/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			s.writeMethodNotAllowed(w, http.MethodPost)
			return
		}
		result, err := s.app.LogoutWeixin()
		s.writeResult(w, result, err)
	})
}

func (s desktopHTTPDevServer) registerFrontend(mux *http.ServeMux) {
	fileServer := http.FileServer(http.Dir(s.frontendDir))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		if r.URL.Path == "/" {
			http.ServeFile(w, r, filepath.Join(s.frontendDir, "index.html"))
			return
		}

		relativePath := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if strings.HasPrefix(relativePath, "..") {
			http.NotFound(w, r)
			return
		}

		target := filepath.Join(s.frontendDir, relativePath)
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		http.ServeFile(w, r, filepath.Join(s.frontendDir, "index.html"))
	})
}

func (s desktopHTTPDevServer) handleImportUpload(r *http.Request) (KnowledgeMutation, error) {
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		return KnowledgeMutation{}, err
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return KnowledgeMutation{}, err
	}
	defer file.Close()

	tempPath, err := writeUploadedTempFile(file, header)
	if err != nil {
		return KnowledgeMutation{}, err
	}
	defer os.RemoveAll(filepath.Dir(tempPath))

	return s.app.ImportFile(tempPath)
}

func (s desktopHTTPDevServer) writeResult(w http.ResponseWriter, payload any, err error) {
	if err == nil {
		s.writeJSON(w, http.StatusOK, payload)
		return
	}

	status := http.StatusInternalServerError
	if errors.Is(err, os.ErrNotExist) {
		status = http.StatusNotFound
	} else {
		status = http.StatusBadRequest
	}
	s.writeError(w, status, err)
}

func (s desktopHTTPDevServer) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("encode http response: %v", err)
	}
}

func (s desktopHTTPDevServer) writeError(w http.ResponseWriter, status int, err error) {
	s.writeJSON(w, status, map[string]string{
		"error": err.Error(),
	})
}

func (s desktopHTTPDevServer) writeMethodNotAllowed(w http.ResponseWriter, allowed ...string) {
	if len(allowed) > 0 {
		w.Header().Set("Allow", strings.Join(allowed, ", "))
	}
	s.writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
}

func decodeJSONBody(r *http.Request, out any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(out)
}

func writeUploadedTempFile(file multipart.File, header *multipart.FileHeader) (string, error) {
	tempDir, err := os.MkdirTemp("", "myclaw-upload-*")
	if err != nil {
		return "", err
	}

	name := filepath.Base(header.Filename)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "upload.bin"
	}
	targetPath := filepath.Join(tempDir, name)

	targetFile, err := os.Create(targetPath)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return "", err
	}

	_, copyErr := io.Copy(targetFile, file)
	closeErr := targetFile.Close()
	if copyErr != nil {
		_ = os.RemoveAll(tempDir)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.RemoveAll(tempDir)
		return "", closeErr
	}
	return targetPath, nil
}
