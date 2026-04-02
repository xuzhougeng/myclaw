package app

import "baize/internal/runtimepolicy"

type CommandExecutionKind = runtimepolicy.CommandExecutionKind

const (
	CommandExecutionService       = runtimepolicy.CommandExecutionService
	CommandExecutionTransportTool = runtimepolicy.CommandExecutionTransportTool
	CommandExecutionControl       = runtimepolicy.CommandExecutionControl
)

type CommandPolicy = runtimepolicy.CommandPolicy
type InputPolicy = runtimepolicy.InputPolicy

func InspectInputPolicy(input string) InputPolicy {
	return runtimepolicy.InspectInputPolicy(input)
}

func IsNewConversationCommand(input string) bool {
	return runtimepolicy.IsNewConversationCommand(input)
}

func CanonicalizeCommandInput(input string) string {
	return runtimepolicy.CanonicalizeCommandInput(input)
}

func normalizeSlash(text string) string {
	return runtimepolicy.NormalizeSlash(text)
}

func isSlashCommand(text string) bool {
	return runtimepolicy.IsSlashCommand(text)
}
