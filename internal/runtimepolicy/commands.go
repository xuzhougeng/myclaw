package runtimepolicy

import "strings"

type CommandExecutionKind string

const (
	CommandExecutionService       CommandExecutionKind = "service"
	CommandExecutionTransportTool CommandExecutionKind = "transport_tool"
	CommandExecutionControl       CommandExecutionKind = "control"
)

type CommandPolicy struct {
	Command              string
	Aliases              []string
	Execution            CommandExecutionKind
	PersistHistory       bool
	ActivateConversation bool
}

type InputPolicy struct {
	RawInput              string
	NormalizedInput       string
	Command               string
	Execution             CommandExecutionKind
	PersistHistory        bool
	ActivateConversation  bool
	IsKnownCommand        bool
	IsConversationControl bool
}

var commandPolicies = []CommandPolicy{
	{Command: "/new", Execution: CommandExecutionControl, PersistHistory: true, ActivateConversation: true},
	{Command: "/help", Aliases: []string{"/h"}, Execution: CommandExecutionService, PersistHistory: false, ActivateConversation: false},
	{Command: "/remember", Aliases: []string{"/r"}, Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/remember-file", Aliases: []string{"/ingest"}, Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/find", Execution: CommandExecutionTransportTool, PersistHistory: false, ActivateConversation: false},
	{Command: "/send", Execution: CommandExecutionTransportTool, PersistHistory: false, ActivateConversation: false},
	{Command: "/append", Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/skills", Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/show-skill", Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/load-skill", Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/unload-skill", Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/page-skills", Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/prompt", Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/translate", Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/debug-search", Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/mode", Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/forget", Aliases: []string{"/delete"}, Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/list", Aliases: []string{"/ls"}, Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/stats", Execution: CommandExecutionService, PersistHistory: false, ActivateConversation: false},
	{Command: "/clear", Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
	{Command: "/notice", Aliases: []string{"/cron"}, Execution: CommandExecutionService, PersistHistory: true, ActivateConversation: true},
}

var routeDecisionCommands = []string{
	"remember",
	"append",
	"append_last",
	"forget",
	"notice_add",
	"notice_list",
	"notice_remove",
	"list",
	"stats",
	"help",
	"answer",
}

var commandPolicyByAlias = buildCommandPolicyByAlias()

func buildCommandPolicyByAlias() map[string]CommandPolicy {
	index := make(map[string]CommandPolicy, len(commandPolicies)*2)
	for _, policy := range commandPolicies {
		keys := append([]string{policy.Command}, policy.Aliases...)
		for _, key := range keys {
			index[strings.ToLower(strings.TrimSpace(key))] = policy
		}
	}
	return index
}

func NormalizeSlash(text string) string {
	if strings.HasPrefix(text, "／") {
		return "/" + strings.TrimPrefix(text, "／")
	}
	return text
}

func InspectInputPolicy(input string) InputPolicy {
	normalized := strings.TrimSpace(NormalizeSlash(input))
	policy := InputPolicy{
		RawInput:        input,
		NormalizedInput: normalized,
	}
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return policy
	}

	command, ok := LookupCommandPolicy(fields[0])
	if !ok {
		return policy
	}

	policy.Command = command.Command
	policy.Execution = command.Execution
	policy.PersistHistory = command.PersistHistory
	policy.ActivateConversation = command.ActivateConversation
	policy.IsKnownCommand = true
	policy.IsConversationControl = command.Execution == CommandExecutionControl
	return policy
}

func IsNewConversationCommand(input string) bool {
	policy := InspectInputPolicy(input)
	return policy.IsConversationControl && policy.Command == "/new"
}

func CanonicalizeCommandInput(input string) string {
	policy := InspectInputPolicy(input)
	if !policy.IsKnownCommand || policy.IsConversationControl {
		return strings.TrimSpace(NormalizeSlash(input))
	}
	fields := strings.Fields(policy.NormalizedInput)
	if len(fields) == 0 || fields[0] == policy.Command {
		return policy.NormalizedInput
	}
	return strings.TrimSpace(policy.Command + strings.TrimPrefix(policy.NormalizedInput, fields[0]))
}

func LookupCommandPolicy(command string) (CommandPolicy, bool) {
	policy, ok := commandPolicyByAlias[strings.ToLower(strings.TrimSpace(command))]
	return policy, ok
}

func IsSlashCommand(text string) bool {
	policy := InspectInputPolicy(text)
	return policy.IsKnownCommand && !policy.IsConversationControl
}

func RouteDecisionCommands() []string {
	return append([]string(nil), routeDecisionCommands...)
}
