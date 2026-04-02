package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"baize/internal/ai"
)

type ProtocolToolSpec struct {
	Purpose           string
	Name              string
	Description       string
	InputContract     string
	OutputContract    string
	Usage             string
	InputJSONExample  string
	OutputJSONExample string
}

type ProtocolToolClient interface {
	ListProtocolTools(ctx context.Context, mc MessageContext) ([]ProtocolToolSpec, error)
	ExecuteProtocolTool(ctx context.Context, mc MessageContext, toolName, rawInput string) (string, error)
}

type protocolAgentToolProvider struct {
	kind   AgentToolProviderKind
	key    string
	client ProtocolToolClient
}

func newProtocolAgentToolProvider(kind AgentToolProviderKind, name string, client ProtocolToolClient) AgentToolProvider {
	if client == nil {
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	return &protocolAgentToolProvider{
		kind:   kind,
		key:    string(kind) + "." + strings.ToLower(name),
		client: client,
	}
}

func (p *protocolAgentToolProvider) ProviderKind() AgentToolProviderKind {
	return p.kind
}

func (p *protocolAgentToolProvider) ProviderKey() string {
	return p.key
}

func (p *protocolAgentToolProvider) ListAgentTools(ctx context.Context, mc MessageContext) ([]AgentToolSpec, error) {
	tools, err := p.client.ListProtocolTools(ctx, mc)
	if err != nil {
		return nil, err
	}
	out := make([]AgentToolSpec, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		out = append(out, AgentToolSpec{
			Name:              name,
			Purpose:           strings.TrimSpace(tool.Purpose),
			Description:       strings.TrimSpace(tool.Description),
			InputContract:     strings.TrimSpace(tool.InputContract),
			OutputContract:    strings.TrimSpace(tool.OutputContract),
			Usage:             strings.TrimSpace(tool.Usage),
			InputJSONExample:  strings.TrimSpace(tool.InputJSONExample),
			OutputJSONExample: strings.TrimSpace(tool.OutputJSONExample),
		})
	}
	return out, nil
}

func (p *protocolAgentToolProvider) ExecuteAgentTool(ctx context.Context, mc MessageContext, toolName, rawInput string) (string, error) {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return "", fmt.Errorf("%s provider requires a tool name", p.key)
	}
	return p.client.ExecuteProtocolTool(ctx, mc, toolName, rawInput)
}

func (s *Service) RegisterAgentToolProvider(provider AgentToolProvider) {
	if s.toolProviders == nil {
		s.toolProviders = newAgentToolProviders()
	}
	s.toolProviders.Register(provider)
}

func (s *Service) RegisterMCPToolProvider(name string, client ProtocolToolClient) {
	s.RegisterAgentToolProvider(newProtocolAgentToolProvider(AgentToolProviderMCP, name, client))
}

func (s *Service) RegisterNCPToolProvider(name string, client ProtocolToolClient) {
	s.RegisterAgentToolProvider(newProtocolAgentToolProvider(AgentToolProviderNCP, name, client))
}

func (s *Service) RegisterACPToolProvider(name string, client ProtocolToolClient) {
	s.RegisterAgentToolProvider(newProtocolAgentToolProvider(AgentToolProviderACP, name, client))
}

func (s *Service) DisabledAgentTools() []string {
	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()

	if len(s.disabledTools) == 0 {
		return nil
	}
	out := make([]string, 0, len(s.disabledTools))
	for name := range s.disabledTools {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func (s *Service) SetDisabledAgentTools(names []string) {
	normalized := NormalizeAgentToolNames(names)
	next := make(map[string]struct{}, len(normalized))
	for _, name := range normalized {
		next[name] = struct{}{}
	}

	s.settingsMu.Lock()
	s.disabledTools = next
	s.settingsMu.Unlock()
}

func (s *Service) ListAllAgentToolDefinitions(ctx context.Context, mc MessageContext) ([]ai.AgentToolDefinition, error) {
	if s.toolProviders == nil {
		return nil, nil
	}
	return s.toolProviders.Definitions(ctx, mc)
}

func (s *Service) ListAgentToolDefinitions(ctx context.Context, mc MessageContext) ([]ai.AgentToolDefinition, error) {
	definitions, err := s.ListAllAgentToolDefinitions(ctx, mc)
	if err != nil {
		return nil, err
	}
	if len(definitions) == 0 {
		return definitions, nil
	}

	filtered := make([]ai.AgentToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		if s.isAgentToolEnabled(definition.Name) {
			filtered = append(filtered, definition)
		}
	}
	return filtered, nil
}

func (s *Service) ExecuteAgentTool(ctx context.Context, mc MessageContext, fullToolName, rawInput string) (string, error) {
	if !s.isAgentToolEnabled(fullToolName) {
		return "", fmt.Errorf("tool %q is disabled", strings.TrimSpace(fullToolName))
	}
	if s.toolProviders == nil {
		return "", fmt.Errorf("tool providers are not configured")
	}
	return s.toolProviders.Execute(ctx, mc, fullToolName, rawInput)
}

func (s *Service) isAgentToolEnabled(name string) bool {
	name = normalizeAgentToolName(name)
	if name == "" {
		return false
	}

	s.settingsMu.RLock()
	defer s.settingsMu.RUnlock()
	_, disabled := s.disabledTools[name]
	return !disabled
}

func NormalizeAgentToolNames(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		name := normalizeAgentToolName(item)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	slices.Sort(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeAgentToolName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
