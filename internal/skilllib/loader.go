package skilllib

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const skillFileName = "SKILL.md"

type Skill struct {
	Name        string
	Description string
	Content     string
	Dir         string
}

type Loader struct {
	dirs []string
}

func NewLoader(dirs ...string) *Loader {
	seen := make(map[string]struct{}, len(dirs))
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		dir = filepath.Clean(dir)
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		out = append(out, dir)
	}
	return &Loader{dirs: out}
}

func DefaultDirs(dataDir string) []string {
	dirs := []string{filepath.Join(dataDir, "skills")}
	for _, dir := range filepath.SplitList(os.Getenv("BAIZE_SKILLS_DIRS")) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		dirs = append(dirs, dir)
	}
	return dirs
}

func (l *Loader) Dirs() []string {
	return slices.Clone(l.dirs)
}

func (l *Loader) List() ([]Skill, error) {
	if len(l.dirs) == 0 {
		return nil, nil
	}

	discovered := make(map[string]Skill)
	order := make([]string, 0)
	for _, dir := range l.dirs {
		skills, err := discoverDir(dir)
		if err != nil {
			return nil, err
		}
		for _, skill := range skills {
			if _, exists := discovered[strings.ToLower(skill.Name)]; exists {
				continue
			}
			discovered[strings.ToLower(skill.Name)] = skill
			order = append(order, skill.Name)
		}
	}
	slices.Sort(order)

	out := make([]Skill, 0, len(order))
	for _, name := range order {
		out = append(out, discovered[strings.ToLower(name)])
	}
	return out, nil
}

func (l *Loader) Load(name string) (Skill, bool, error) {
	name = normalizeSkillName(name)
	if name == "" {
		return Skill{}, false, nil
	}

	skills, err := l.List()
	if err != nil {
		return Skill{}, false, err
	}
	for _, skill := range skills {
		if normalizeSkillName(skill.Name) == name {
			return skill, true, nil
		}
	}
	return Skill{}, false, nil
}

func discoverDir(dir string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skills dir %s: %w", dir, err)
	}

	out := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(dir, entry.Name())
		skillPath := filepath.Join(skillDir, skillFileName)
		data, err := os.ReadFile(skillPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read skill %s: %w", skillPath, err)
		}

		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}

		meta := parseFrontmatter(content)
		name := entry.Name()
		if frontmatterName := strings.TrimSpace(meta["name"]); frontmatterName != "" {
			name = frontmatterName
		}

		out = append(out, Skill{
			Name:        name,
			Description: strings.TrimSpace(meta["description"]),
			Content:     content,
			Dir:         skillDir,
		})
	}
	return out, nil
}

func parseFrontmatter(content string) map[string]string {
	trimmed := strings.TrimSpace(content)
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return nil
	}

	meta := make(map[string]string)
	for _, rawLine := range lines[1:] {
		line := strings.TrimSpace(rawLine)
		if line == "---" {
			return meta
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.ToLower(key))
		value = strings.TrimSpace(strings.Trim(value, `"'`))
		if key == "" || value == "" {
			continue
		}
		meta[key] = value
	}
	return nil
}

func normalizeSkillName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
