//go:build cgo

package fileingest

import (
	"errors"
	"strings"

	"github.com/gen2brain/go-fitz"
)

func ExtractPDFText(path string) (string, error) {
	doc, err := fitz.New(path)
	if err != nil {
		return "", err
	}
	defer doc.Close()

	var builder strings.Builder
	for page := 0; page < doc.NumPage(); page++ {
		text, err := doc.Text(page)
		if err != nil {
			return "", err
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(text)
	}

	output := strings.TrimSpace(builder.String())
	if output == "" {
		return "", errors.New("fitz extracted empty text from pdf")
	}
	return output, nil
}
