package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appsvc "baize/internal/app"
	"baize/internal/knowledge"
	"baize/internal/reminder"
	"baize/internal/skilllib"
)

func TestImportSkillArchiveRejectsDuplicateSkillName(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "writer")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(strings.TrimSpace(`
---
name: writer
description: 帮助输出更清晰的中文写作
---
# Writer
给出简洁、结构清晰、少废话的中文输出。
`)), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	service := appsvc.NewServiceWithSkills(
		knowledge.NewStore(filepath.Join(root, "app.db")),
		nil,
		reminder.NewManager(reminder.NewStore(filepath.Join(root, "app.db"))),
		skilllib.NewLoader(filepath.Join(root, "skills")),
	)
	app := &DesktopApp{
		dataDir: root,
		service: service,
	}

	archivePath := filepath.Join(root, "writer-upload.zip")
	writeDesktopSkillArchive(t, archivePath, map[string]string{
		"SKILL.md": strings.TrimSpace(`
---
name: writer
description: 新版 writer
---
# Writer
新版 skill 内容。
`),
	})

	_, err := app.ImportSkillArchive(archivePath)
	if err == nil {
		t.Fatal("expected duplicate skill import to fail")
	}
	if !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeDesktopSkillArchive(t *testing.T, archivePath string, files map[string]string) {
	t.Helper()

	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}

	writer := zip.NewWriter(file)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create archive entry %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("write archive entry %s: %v", name, err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close archive writer: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close archive file: %v", err)
	}
}
