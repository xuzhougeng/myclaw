package app

import (
	"context"
	"fmt"
	"strings"

	"baize/internal/windowsautomationtool"
)

var executeWindowsAutomationTool = windowsautomationtool.Execute

func (p *localAgentToolProvider) executeWindowsAutomationTool(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
	if !windowsautomationtool.AllowedForInterface(mc.Interface) {
		return "", fmt.Errorf("%s is not available for interface %q", windowsautomationtool.ToolName, strings.TrimSpace(mc.Interface))
	}

	var args windowsautomationtool.ToolInput
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	result, err := executeWindowsAutomationTool(ctx, args)
	if err != nil {
		return "", err
	}
	return windowsautomationtool.FormatResult(result)
}
