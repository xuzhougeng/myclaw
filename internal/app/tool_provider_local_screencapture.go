package app

import (
	"context"
	"fmt"
	"strings"

	"baize/internal/screencapture"
)

var executeScreenCapture = screencapture.Execute

func (p *localAgentToolProvider) executeScreenCapture(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
	if !screencapture.AllowedForInterface(mc.Interface) {
		return "", fmt.Errorf("%s is not available for interface %q", screencapture.ToolName, strings.TrimSpace(mc.Interface))
	}

	var args screencapture.ToolInput
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}

	opts := screencapture.ExecuteOptions{}
	if p.service.aiService != nil {
		opts.Analyzer = func(ctx context.Context, fileName, imageURL string) (string, error) {
			return p.service.aiService.SummarizeImageFile(ctx, fileName, imageURL)
		}
	}

	result, err := executeScreenCapture(ctx, args, opts)
	if err != nil {
		return "", err
	}
	return screencapture.FormatResult(result)
}
