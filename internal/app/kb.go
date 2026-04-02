package app

import (
	"context"
	"fmt"
	"strings"

	"baize/internal/knowledge"
)

func (s *Service) activeProject(ctx context.Context, mc MessageContext) (string, error) {
	if project := strings.TrimSpace(mc.Project); project != "" {
		return knowledge.CanonicalProjectName(project), nil
	}
	if s.projectStore == nil {
		return knowledge.DefaultProjectName, nil
	}
	snapshot, err := s.projectStore.LoadScope(ctx, knowledgeBaseScopeID(mc))
	if err != nil {
		return "", err
	}
	return knowledge.CanonicalProjectName(snapshot.ActiveProject), nil
}

func (s *Service) switchKnowledgeBase(ctx context.Context, mc MessageContext, project string) (knowledge.ProjectInfo, error) {
	project = knowledge.CanonicalProjectName(project)
	info, err := s.store.EnsureProject(ctx, project)
	if err != nil {
		return knowledge.ProjectInfo{}, err
	}
	if s.projectStore != nil {
		if _, err := s.projectStore.SaveScope(ctx, knowledgeBaseScopeID(mc), project); err != nil {
			return knowledge.ProjectInfo{}, err
		}
	}
	return info, nil
}

func (s *Service) currentKnowledgeBaseReply(ctx context.Context, mc MessageContext) (string, error) {
	activeProject, err := s.activeProject(ctx, mc)
	if err != nil {
		return "", err
	}

	infos, err := s.store.ListProjects(ctx)
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	builder.WriteString("当前知识库：")
	builder.WriteString(activeProject)

	if len(infos) == 0 {
		builder.WriteString("\n可用知识库：")
		builder.WriteString(activeProject)
		return builder.String(), nil
	}

	builder.WriteString(fmt.Sprintf("\n可用知识库（共 %d 个）:", len(infos)))
	for index, info := range infos {
		builder.WriteString("\n")
		builder.WriteString(fmt.Sprintf("%d. %s", index+1, info.Name))
		if strings.EqualFold(info.Name, activeProject) {
			builder.WriteString(" [当前]")
		}
		builder.WriteString(fmt.Sprintf(" (%d 条)", info.KnowledgeCount))
	}
	return builder.String(), nil
}

func knowledgeBaseScopeID(mc MessageContext) string {
	interfaceName := strings.ToLower(strings.TrimSpace(mc.Interface))
	userID := strings.ToLower(strings.TrimSpace(mc.UserID))

	switch {
	case interfaceName == "" && userID == "":
		return "knowledge:primary"
	case interfaceName == "":
		return "knowledge:user:" + userID
	case userID == "":
		return "knowledge:" + interfaceName
	default:
		return "knowledge:" + interfaceName + ":" + userID
	}
}
