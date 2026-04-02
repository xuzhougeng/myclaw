package app

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"baize/internal/ai"
	"baize/internal/toolcontract"
)

type AgentToolProviderKind string

const (
	AgentToolProviderLocal AgentToolProviderKind = "local"
	AgentToolProviderMCP   AgentToolProviderKind = "mcp"
	AgentToolProviderNCP   AgentToolProviderKind = "ncp"
	AgentToolProviderACP   AgentToolProviderKind = "acp"
)

// ToolSideEffectLevel describes the potential impact of executing a tool.
type ToolSideEffectLevel string

const (
	// ToolSideEffectReadOnly tools never modify state.
	ToolSideEffectReadOnly ToolSideEffectLevel = "read_only"
	// ToolSideEffectSoftWrite tools modify state but the change is reversible or low-risk.
	ToolSideEffectSoftWrite ToolSideEffectLevel = "soft_write"
	// ToolSideEffectDestructive tools delete or irreversibly modify state.
	ToolSideEffectDestructive ToolSideEffectLevel = "destructive"
)

// AgentToolSpec describes a single tool exposed by an AgentToolProvider.
type AgentToolSpec struct {
	FamilyKey            string
	FamilyTitle          string
	DisplayTitle         string
	DisplayOrder         int
	Purpose              string
	Name                 string
	Description          string
	InputContract        string
	OutputContract       string
	Usage                string
	InputJSONExample     string
	OutputJSONExample    string
	SideEffectLevel      ToolSideEffectLevel
	RequiresConfirmation bool
	IsIdempotent         bool
}

type AgentToolProvider interface {
	ProviderKind() AgentToolProviderKind
	ProviderKey() string
	ListAgentTools(ctx context.Context, mc MessageContext) ([]AgentToolSpec, error)
	ExecuteAgentTool(ctx context.Context, mc MessageContext, toolName, rawInput string) (string, error)
}

type agentToolProviders struct {
	mu        sync.RWMutex
	providers map[string]AgentToolProvider
	order     []string
}

func newAgentToolProviders() *agentToolProviders {
	return &agentToolProviders{
		providers: make(map[string]AgentToolProvider),
	}
}

func (r *agentToolProviders) Register(provider AgentToolProvider) {
	if provider == nil {
		return
	}

	key := normalizeProviderKey(provider.ProviderKey())
	if key == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.providers[key]; !ok {
		r.order = append(r.order, key)
	}
	r.providers[key] = provider
}

func (r *agentToolProviders) Definitions(ctx context.Context, mc MessageContext) ([]ai.AgentToolDefinition, error) {
	r.mu.RLock()
	order := append([]string(nil), r.order...)
	providers := make(map[string]AgentToolProvider, len(r.providers))
	for key, provider := range r.providers {
		providers[key] = provider
	}
	r.mu.RUnlock()

	out := make([]ai.AgentToolDefinition, 0)
	for _, key := range order {
		provider := providers[key]
		if provider == nil {
			continue
		}
		tools, err := provider.ListAgentTools(ctx, mc)
		if err != nil {
			return nil, err
		}
		for _, tool := range tools {
			name := strings.TrimSpace(tool.Name)
			if name == "" {
				continue
			}
			out = append(out, ai.AgentToolDefinition{
				Name:              joinProviderToolName(key, name),
				FamilyKey:         strings.TrimSpace(tool.FamilyKey),
				FamilyTitle:       strings.TrimSpace(tool.FamilyTitle),
				DisplayTitle:      strings.TrimSpace(tool.DisplayTitle),
				DisplayOrder:      tool.DisplayOrder,
				Provider:          key,
				ProviderKind:      string(provider.ProviderKind()),
				Purpose:           strings.TrimSpace(tool.Purpose),
				Description:       strings.TrimSpace(tool.Description),
				InputContract:     strings.TrimSpace(tool.InputContract),
				OutputContract:    strings.TrimSpace(tool.OutputContract),
				Usage:             strings.TrimSpace(tool.Usage),
				InputJSONExample:  strings.TrimSpace(tool.InputJSONExample),
				OutputJSONExample: strings.TrimSpace(tool.OutputJSONExample),
				SideEffectLevel:   string(tool.SideEffectLevel),
			})
		}
	}
	return out, nil
}

func (r *agentToolProviders) Execute(ctx context.Context, mc MessageContext, fullToolName, rawInput string) (string, error) {
	providerKey, toolName, err := splitProviderToolName(fullToolName)
	if err != nil {
		return "", err
	}

	r.mu.RLock()
	provider := r.providers[providerKey]
	r.mu.RUnlock()
	if provider == nil {
		return "", fmt.Errorf("unknown tool provider %q", providerKey)
	}
	return provider.ExecuteAgentTool(ctx, mc, toolName, rawInput)
}

func joinProviderToolName(providerKey, toolName string) string {
	return normalizeProviderKey(providerKey) + "::" + strings.TrimSpace(toolName)
}

func splitProviderToolName(value string) (string, string, error) {
	providerKey, toolName, ok := strings.Cut(strings.TrimSpace(value), "::")
	if !ok {
		return "", "", fmt.Errorf("tool name %q is missing provider prefix", value)
	}
	providerKey = normalizeProviderKey(providerKey)
	toolName = strings.TrimSpace(toolName)
	if providerKey == "" || toolName == "" {
		return "", "", fmt.Errorf("invalid tool name %q", value)
	}
	return providerKey, toolName, nil
}

func normalizeProviderKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func agentToolSpecFromContract(spec toolcontract.Spec) AgentToolSpec {
	spec = spec.Normalized()
	return AgentToolSpec{
		FamilyKey:         spec.FamilyKey,
		FamilyTitle:       spec.FamilyTitle,
		DisplayTitle:      spec.DisplayTitle,
		DisplayOrder:      spec.DisplayOrder,
		Name:              spec.Name,
		Purpose:           spec.Purpose,
		Description:       spec.Description,
		InputContract:     spec.InputContract,
		OutputContract:    spec.OutputContract,
		Usage:             spec.Usage,
		InputJSONExample:  spec.InputJSONExample,
		OutputJSONExample: spec.OutputJSONExample,
	}
}
