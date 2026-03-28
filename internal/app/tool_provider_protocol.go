package app

import (
	"context"
	"fmt"
	"strings"

	"myclaw/internal/ai"
)

type ProtocolToolSpec struct {
	Name             string
	Description      string
	InputJSONExample string
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
			Name:             name,
			Description:      strings.TrimSpace(tool.Description),
			InputJSONExample: strings.TrimSpace(tool.InputJSONExample),
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

func (s *Service) ListAgentToolDefinitions(ctx context.Context, mc MessageContext) ([]ai.AgentToolDefinition, error) {
	if s.toolProviders == nil {
		return nil, nil
	}
	return s.toolProviders.Definitions(ctx, mc)
}
