package screencapture

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"baize/internal/toolcontract"

	"github.com/kbinani/screenshot"
)

const (
	ToolName           = "screen_capture"
	ToolFamilyKey      = "screen"
	ToolFamilyTitle    = "屏幕"
	defaultMaxDim      = 1600
	defaultJPEGQuality = 72
	minJPEGQuality     = 30
	maxJPEGQuality     = 95
	minMaxDim          = 320
	maxMaxDim          = 4096
)

var (
	nowFunc = time.Now
)

type ToolInput struct {
	Analyze      *bool `json:"analyze,omitempty"`
	MaxDimension int   `json:"max_dimension,omitempty"`
	JPEGQuality  int   `json:"jpeg_quality,omitempty"`
}

type ToolResult struct {
	Tool           string `json:"tool"`
	Path           string `json:"path"`
	MIMEType       string `json:"mime_type"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	DisplayIndex   int    `json:"display_index"`
	CapturedAt     string `json:"captured_at"`
	Analyze        bool   `json:"analyze"`
	AnalysisStatus string `json:"analysis_status,omitempty"`
	Summary        string `json:"summary,omitempty"`
}

type Analyzer func(context.Context, string, string) (string, error)

type ExecuteOptions struct {
	BaseDir   string
	Analyzer  Analyzer
	CaptureFn func(context.Context, ToolInput) (capturedImage, error)
	Now       func() time.Time
}

type capturedImage struct {
	DisplayIndex int
	Width        int
	Height       int
	JPEGBytes    []byte
}

func Definition() toolcontract.Spec {
	return toolcontract.Spec{
		Name:              ToolName,
		FamilyKey:         ToolFamilyKey,
		FamilyTitle:       ToolFamilyTitle,
		DisplayTitle:      "当前屏幕截图",
		DisplayOrder:      35,
		Purpose:           "Capture the current host screen as an image file, optionally with a short visual summary.",
		Description:       "Takes a screenshot of the current machine's primary display and stores it in a temporary local file. When AI image analysis is available, it can also return a concise summary of the visible screen.",
		InputContract:     `Provide {} or optionally {"analyze":true,"max_dimension":1600,"jpeg_quality":72}.`,
		OutputContract:    "Returns JSON with the saved image path, mime type, dimensions, display index, capture time, whether analysis was requested, the analysis status, and an optional summary.",
		InputJSONExample:  `{"analyze":true,"max_dimension":1600}`,
		OutputJSONExample: `{"tool":"screen_capture","path":"C:\\Users\\me\\AppData\\Local\\Temp\\baize-screen-capture\\screen-20260402-103000.jpg","mime_type":"image/jpeg","width":1600,"height":900,"display_index":0,"captured_at":"2026-04-02T10:30:00Z","analyze":true,"analysis_status":"summarized","summary":"current_user 正在阅读文档并修改代码。"}`,
		Usage:             UsageText(),
	}
}

func UsageText() string {
	return strings.TrimSpace(`
Tool: screen_capture
Purpose: capture the current machine screen as a local JPEG file, with optional visual summarization.

Input:
- analyze: optional boolean. Default true. When true and image analysis is available, return a concise summary.
- max_dimension: optional integer. Resizes the captured image to fit within this size. Default 1600.
- jpeg_quality: optional integer. JPEG quality from 30 to 95. Default 72.

Output:
- tool: always screen_capture.
- path: absolute path of the saved JPEG file.
- mime_type: always image/jpeg.
- width, height: saved image dimensions.
- display_index: captured display index. Currently always 0.
- captured_at: RFC3339 timestamp.
- analyze: whether analysis was requested.
- analysis_status: summarized, unavailable, or skipped.
- summary: optional concise screen summary.

Notes:
- This tool captures the current host screen and is read-only from the runtime perspective.
- It is intended for one-shot inspection, not long-running background recording.
`)
}

func AllowedForInterface(name string) bool {
	return true
}

func SupportedForCurrentPlatform() bool {
	switch runtime.GOOS {
	case "windows", "darwin", "linux":
		return true
	default:
		return false
	}
}

func NormalizeInput(raw ToolInput) ToolInput {
	analyze := true
	if raw.Analyze != nil {
		analyze = *raw.Analyze
	}
	maxDim := raw.MaxDimension
	switch {
	case maxDim <= 0:
		maxDim = defaultMaxDim
	case maxDim < minMaxDim:
		maxDim = minMaxDim
	case maxDim > maxMaxDim:
		maxDim = maxMaxDim
	}
	quality := raw.JPEGQuality
	switch {
	case quality <= 0:
		quality = defaultJPEGQuality
	case quality < minJPEGQuality:
		quality = minJPEGQuality
	case quality > maxJPEGQuality:
		quality = maxJPEGQuality
	}
	return ToolInput{
		Analyze:      &analyze,
		MaxDimension: maxDim,
		JPEGQuality:  quality,
	}
}

func Execute(ctx context.Context, raw ToolInput, opts ExecuteOptions) (ToolResult, error) {
	input := NormalizeInput(raw)
	if !SupportedForCurrentPlatform() {
		return ToolResult{}, fmt.Errorf("%s is not supported on %s", ToolName, runtime.GOOS)
	}

	captureFn := opts.CaptureFn
	if captureFn == nil {
		captureFn = capturePrimaryDisplay
	}
	now := opts.Now
	if now == nil {
		now = nowFunc
	}

	capturedAt := now().UTC()
	imageData, err := captureFn(ctx, input)
	if err != nil {
		return ToolResult{}, err
	}

	baseDir := strings.TrimSpace(opts.BaseDir)
	if baseDir == "" {
		baseDir = filepath.Join(os.TempDir(), "baize-screen-capture")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return ToolResult{}, err
	}

	fileName := fmt.Sprintf("screen-%s-%09d.jpg", capturedAt.Format("20060102-150405"), capturedAt.Nanosecond())
	filePath := filepath.Join(baseDir, fileName)
	if err := os.WriteFile(filePath, imageData.JPEGBytes, 0o644); err != nil {
		return ToolResult{}, err
	}

	result := ToolResult{
		Tool:         ToolName,
		Path:         filePath,
		MIMEType:     "image/jpeg",
		Width:        imageData.Width,
		Height:       imageData.Height,
		DisplayIndex: imageData.DisplayIndex,
		CapturedAt:   capturedAt.Format(time.RFC3339),
		Analyze:      input.Analyze != nil && *input.Analyze,
	}

	if !result.Analyze {
		result.AnalysisStatus = "skipped"
		return result, nil
	}
	if opts.Analyzer == nil {
		result.AnalysisStatus = "unavailable"
		return result, nil
	}

	summary, err := opts.Analyzer(ctx, fileName, "data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString(imageData.JPEGBytes))
	if err != nil {
		result.AnalysisStatus = "unavailable"
		return result, nil
	}
	result.AnalysisStatus = "summarized"
	result.Summary = strings.TrimSpace(summary)
	return result, nil
}

func FormatResult(result ToolResult) (string, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func capturePrimaryDisplay(_ context.Context, input ToolInput) (capturedImage, error) {
	if screenshot.NumActiveDisplays() < 1 {
		return capturedImage{}, fmt.Errorf("no active display available")
	}
	bounds := screenshot.GetDisplayBounds(0)
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return capturedImage{}, err
	}
	return encodeResizedJPEG(0, img, input)
}

func encodeResizedJPEG(displayIndex int, src image.Image, input ToolInput) (capturedImage, error) {
	scaled := resizeToFit(src, input.MaxDimension)
	bounds := scaled.Bounds()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, scaled, &jpeg.Options{Quality: input.JPEGQuality}); err != nil {
		return capturedImage{}, err
	}
	return capturedImage{
		DisplayIndex: displayIndex,
		Width:        bounds.Dx(),
		Height:       bounds.Dy(),
		JPEGBytes:    buf.Bytes(),
	}, nil
}

func resizeToFit(src image.Image, maxDimension int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return src
	}
	if maxDimension <= 0 || (width <= maxDimension && height <= maxDimension) {
		return src
	}
	var dstWidth, dstHeight int
	if width >= height {
		dstWidth = maxDimension
		dstHeight = int(float64(height) * (float64(maxDimension) / float64(width)))
	} else {
		dstHeight = maxDimension
		dstWidth = int(float64(width) * (float64(maxDimension) / float64(height)))
	}
	if dstWidth < 1 {
		dstWidth = 1
	}
	if dstHeight < 1 {
		dstHeight = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))
	for y := 0; y < dstHeight; y++ {
		srcY := bounds.Min.Y + y*height/dstHeight
		for x := 0; x < dstWidth; x++ {
			srcX := bounds.Min.X + x*width/dstWidth
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}
