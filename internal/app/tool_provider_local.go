package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"baize/internal/bashtool"
	"baize/internal/dirlist"
	"baize/internal/filesearch"
	"baize/internal/osascripttool"
	"baize/internal/powershelltool"
	"baize/internal/screencapture"
	"baize/internal/toolcontract"
	"baize/internal/windowsautomationtool"
)

type localAgentToolProvider struct {
	service *Service
}

type localToolHandler func(context.Context, MessageContext, string) (string, error)

type localToolSet struct {
	contracts        []toolcontract.Spec
	sideEffectByName map[string]ToolSideEffectLevel
	handlers         map[string]localToolHandler
	listable         func(MessageContext) bool
}

func newLocalAgentToolProvider(service *Service) AgentToolProvider {
	return &localAgentToolProvider{service: service}
}

func (p *localAgentToolProvider) ProviderKind() AgentToolProviderKind {
	return AgentToolProviderLocal
}

func (p *localAgentToolProvider) ProviderKey() string {
	return string(AgentToolProviderLocal)
}

func newLocalToolSet(contracts []toolcontract.Spec, sideEffects map[string]ToolSideEffectLevel, handlers map[string]localToolHandler, listable func(MessageContext) bool) localToolSet {
	normalizedSideEffects := make(map[string]ToolSideEffectLevel, len(sideEffects))
	for name, level := range sideEffects {
		name = normalizeLocalToolName(name)
		if name == "" {
			continue
		}
		normalizedSideEffects[name] = level
	}

	normalizedHandlers := make(map[string]localToolHandler, len(handlers))
	for name, handler := range handlers {
		name = normalizeLocalToolName(name)
		if name == "" || handler == nil {
			continue
		}
		normalizedHandlers[name] = handler
	}

	return localToolSet{
		contracts:        append([]toolcontract.Spec(nil), contracts...),
		sideEffectByName: normalizedSideEffects,
		handlers:         normalizedHandlers,
		listable:         listable,
	}
}

func singletonLocalToolSet(contract toolcontract.Spec, level ToolSideEffectLevel, handler localToolHandler, listable func(MessageContext) bool) localToolSet {
	return newLocalToolSet(
		[]toolcontract.Spec{contract},
		map[string]ToolSideEffectLevel{contract.Name: level},
		map[string]localToolHandler{contract.Name: handler},
		listable,
	)
}

func normalizeLocalToolName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (s localToolSet) listableFor(mc MessageContext) bool {
	if s.listable == nil {
		return true
	}
	return s.listable(mc)
}

func (s localToolSet) specs() []AgentToolSpec {
	out := make([]AgentToolSpec, 0, len(s.contracts))
	for _, contract := range s.contracts {
		spec := agentToolSpecFromContract(contract)
		if level, ok := s.sideEffectByName[normalizeLocalToolName(spec.Name)]; ok {
			spec.SideEffectLevel = level
		}
		out = append(out, spec)
	}
	return out
}

func (s localToolSet) handler(toolName string) (localToolHandler, bool) {
	handler, ok := s.handlers[normalizeLocalToolName(toolName)]
	return handler, ok
}

func (p *localAgentToolProvider) localToolSets() []localToolSet {
	return []localToolSet{
		p.knowledgeToolSet(),
		singletonLocalToolSet(filesearch.Definition(), ToolSideEffectReadOnly, p.executeFileSearch, nil),
		p.reminderToolSet(),
		singletonLocalToolSet(
			bashtool.Definition(),
			ToolSideEffectReadOnly,
			p.executeBashTool,
			func(mc MessageContext) bool {
				return bashtool.AllowedForInterface(mc.Interface) && bashtool.SupportedForCurrentPlatform()
			},
		),
		singletonLocalToolSet(
			powershelltool.Definition(),
			ToolSideEffectReadOnly,
			p.executePowerShellTool,
			func(mc MessageContext) bool {
				return powershelltool.AllowedForInterface(mc.Interface) && powershelltool.SupportedForCurrentPlatform()
			},
		),
		singletonLocalToolSet(
			dirlist.Definition(),
			ToolSideEffectReadOnly,
			p.executeListDirectory,
			func(mc MessageContext) bool {
				return dirlist.AllowedForInterface(mc.Interface)
			},
		),
		singletonLocalToolSet(
			screencapture.Definition(),
			ToolSideEffectReadOnly,
			p.executeScreenCapture,
			func(mc MessageContext) bool {
				return screencapture.AllowedForInterface(mc.Interface) && screencapture.SupportedForCurrentPlatform()
			},
		),
		singletonLocalToolSet(
			osascripttool.Definition(),
			ToolSideEffectSoftWrite,
			p.executeOsaScriptTool,
			func(mc MessageContext) bool {
				return osascripttool.AllowedForInterface(mc.Interface) && osascripttool.SupportedForCurrentPlatform()
			},
		),
		singletonLocalToolSet(
			windowsautomationtool.Definition(),
			ToolSideEffectSoftWrite,
			p.executeWindowsAutomationTool,
			func(mc MessageContext) bool {
				return windowsautomationtool.AllowedForInterface(mc.Interface) && windowsautomationtool.SupportedForCurrentPlatform()
			},
		),
	}
}

func (p *localAgentToolProvider) ListAgentTools(_ context.Context, mc MessageContext) ([]AgentToolSpec, error) {
	sets := p.localToolSets()
	tools := make([]AgentToolSpec, 0, len(sets)*2)
	for _, set := range sets {
		if !set.listableFor(mc) {
			continue
		}
		tools = append(tools, set.specs()...)
	}
	return tools, nil
}

func (p *localAgentToolProvider) ExecuteAgentTool(ctx context.Context, mc MessageContext, toolName, rawInput string) (string, error) {
	for _, set := range p.localToolSets() {
		handler, ok := set.handler(toolName)
		if !ok {
			continue
		}
		return handler(ctx, mc, rawInput)
	}
	return "", fmt.Errorf("unknown local tool %q", toolName)
}

func (p *localAgentToolProvider) executeFileSearch(ctx context.Context, _ MessageContext, rawInput string) (string, error) {
	var args filesearch.ToolInput
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	result, reply, err := p.service.performFileSearch(ctx, args)
	if err != nil {
		return "", err
	}
	if reply != "" {
		return reply, nil
	}
	return filesearch.FormatSearchResult(result), nil
}

func (p *localAgentToolProvider) executeListDirectory(_ context.Context, mc MessageContext, rawInput string) (string, error) {
	if !dirlist.AllowedForInterface(mc.Interface) {
		return "", fmt.Errorf("%s is not available for interface %q", dirlist.ToolName, strings.TrimSpace(mc.Interface))
	}

	var args dirlist.ToolInput
	if err := decodeAgentToolInput(rawInput, &args); err != nil {
		return "", err
	}
	result, err := dirlist.Execute(args)
	if err != nil {
		return "", err
	}
	return dirlist.FormatResult(result)
}

func decodeAgentToolInput(raw string, out any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "{}"
	}
	if !strings.HasPrefix(raw, "{") {
		return fmt.Errorf("tool input must be a JSON object")
	}
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		return fmt.Errorf("decode tool input: %w", err)
	}
	return nil
}
