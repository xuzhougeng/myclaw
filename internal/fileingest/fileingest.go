package fileingest

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Kind string

const (
	KindPDF   Kind = "pdf"
	KindImage Kind = "image"
)

type Input struct {
	Path     string
	Name     string
	Kind     Kind
	MIMEType string
	DataURL  string
}

var ErrPDFExtractorUnavailable = errors.New("pdf extraction is unavailable in this build")

func NormalizePath(raw string) string {
	path := strings.TrimSpace(raw)
	path = strings.Trim(path, `"'`)
	return strings.TrimSpace(path)
}

func Resolve(raw string) (Input, bool, error) {
	path := NormalizePath(raw)
	if path == "" {
		return Input{}, false, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Input{}, false, nil
		}
		return Input{}, false, err
	}
	if info.IsDir() {
		return Input{}, false, nil
	}

	kind, mimeType, ok := detectKind(path)
	if !ok {
		return Input{}, false, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return Input{}, false, err
	}
	dataURL := ""
	if kind == KindImage {
		data, err := os.ReadFile(absPath)
		if err != nil {
			return Input{}, false, err
		}
		dataURL = "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
	}

	return Input{
		Path:     absPath,
		Name:     filepath.Base(absPath),
		Kind:     kind,
		MIMEType: mimeType,
		DataURL:  dataURL,
	}, true, nil
}

func detectKind(path string) (Kind, string, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return KindPDF, "application/pdf", true
	case ".png":
		return KindImage, "image/png", true
	case ".jpg", ".jpeg":
		return KindImage, "image/jpeg", true
	case ".webp":
		return KindImage, "image/webp", true
	case ".gif":
		return KindImage, "image/gif", true
	default:
		return "", "", false
	}
}

func FormatKnowledgeText(input Input, summary string) string {
	return strings.TrimSpace(fmt.Sprintf("## 文件摘要\n- 文件: %s\n- 类型: %s\n- 路径: %s\n\n%s",
		input.Name,
		input.Kind,
		input.Path,
		strings.TrimSpace(summary),
	))
}
