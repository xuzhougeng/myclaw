package app

import (
	"context"
	"fmt"
	"strings"

	"baize/internal/osascripttool"
)

var executeOsaScriptTool = osascripttool.Execute

func (p *localAgentToolProvider) executeOsaScriptTool(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
	if !osascripttool.AllowedForInterface(mc.Interface) {
		return "", fmt.Errorf("%s is not available for interface %q", osascripttool.ToolName, strings.TrimSpace(mc.Interface))
	}

	var args osascripttool.ToolInput
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	result, err := executeOsaScriptTool(ctx, args)
	if err != nil {
		return "", err
	}
	return osascripttool.FormatResult(result)
}
