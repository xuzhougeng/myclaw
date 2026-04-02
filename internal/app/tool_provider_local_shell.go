package app

import (
	"context"
	"fmt"
	"strings"

	"baize/internal/bashtool"
	"baize/internal/powershelltool"
)

func (p *localAgentToolProvider) executeBashTool(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
	if !bashtool.AllowedForInterface(mc.Interface) {
		return "", fmt.Errorf("%s is not available for interface %q", bashtool.ToolName, strings.TrimSpace(mc.Interface))
	}

	var args bashtool.ToolInput
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	result, err := bashtool.Execute(ctx, args)
	if err != nil {
		return "", err
	}
	return bashtool.FormatResult(result)
}

func (p *localAgentToolProvider) executePowerShellTool(ctx context.Context, mc MessageContext, rawInput string) (string, error) {
	if !powershelltool.AllowedForInterface(mc.Interface) {
		return "", fmt.Errorf("%s is not available for interface %q", powershelltool.ToolName, strings.TrimSpace(mc.Interface))
	}

	var args powershelltool.ToolInput
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	result, err := powershelltool.Execute(ctx, args)
	if err != nil {
		return "", err
	}
	return powershelltool.FormatResult(result)
}
