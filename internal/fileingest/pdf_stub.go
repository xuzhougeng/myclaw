//go:build !cgo

package fileingest

import "fmt"

func ExtractPDFText(string) (string, error) {
	return "", fmt.Errorf("%w: rebuild with CGO_ENABLED=1 to enable go-fitz PDF extraction", ErrPDFExtractorUnavailable)
}
