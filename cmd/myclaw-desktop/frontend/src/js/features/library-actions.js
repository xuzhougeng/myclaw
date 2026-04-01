async function browseFile() {
  if (state.backendMode === 'http') {
    const fileInput = document.getElementById('http-file-input');
    if (fileInput) {
      fileInput.value = '';
      fileInput.click();
    }
    return;
  }

  try {
    const selected = await state.backend.OpenImportDialog();
    if (!selected) return;

    state.fileObject = null;
    state.filePath = selected;

    // Simulate file preview
    const preview = document.getElementById('file-preview');
    const fileName = document.getElementById('file-name');

    if (preview) preview.classList.add('has-file');
    if (fileName) fileName.textContent = selected.split(/[/\\]/).pop() || selected;
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function importFile() {
  if (!state.filePath.trim() && !state.fileObject) {
    showBanner('请先选择文件。', true);
    return;
  }

  try {
    const result = state.backendMode === 'http'
      ? await state.backend.UploadImportFile(state.fileObject)
      : await state.backend.ImportFile(state.filePath);

    // Reset file preview
    const preview = document.getElementById('file-preview');
    if (preview) preview.classList.remove('has-file');
    state.filePath = '';
    state.fileObject = null;
    const fileInput = document.getElementById('http-file-input');
    if (fileInput) fileInput.value = '';

    await refreshAll();
    showBanner(result.message, false);

    state.chat.push({
      role: 'system',
      text: `${result.message}\n${result.item.preview}`,
      time: nowLabel(),
    });
    syncCurrentChatConversationFromMessages();
    renderChat();
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function browseSkillArchive() {
  if (state.backendMode === 'http') {
    const input = document.getElementById('http-skill-zip-input');
    if (input) {
      input.value = '';
      input.click();
    }
    return;
  }

  try {
    const selected = await state.backend.OpenSkillImportDialog();
    if (!selected) return;
    await importSkillArchiveFromPath(selected);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function importSkillArchiveFromPath(path) {
  if (!path) return;

  try {
    const result = await state.backend.ImportSkillArchive(path);
    if (result && result.item && result.item.name) {
      state.selectedSkillName = result.item.name;
    }
    await refreshSkills();
    showBanner(result.message || 'skill 已导入。', false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function importSkillArchiveFromFile(file) {
  if (!file) return;

  try {
    const result = await state.backend.UploadSkillArchive(file);
    if (result && result.item && result.item.name) {
      state.selectedSkillName = result.item.name;
    }
    const input = document.getElementById('http-skill-zip-input');
    if (input) input.value = '';
    await refreshSkills();
    showBanner(result.message || 'skill 已导入。', false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function createKnowledge() {
  const input = document.getElementById('memory-input');
  const text = input?.value.trim();
  if (!text) {
    showBanner('请输入要保存的记忆内容。', true);
    return;
  }

  try {
    const result = await state.backend.CreateKnowledge(text);
    if (input) input.value = '';
    await refreshAll();
    showBanner(result.message, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

function toggleMemoryExpand(id) {
  const content = document.querySelector(`[data-content-id="${id}"]`);
  const btn = document.querySelector(`[data-action="toggle-expand"][data-id="${id}"]`);
  if (content && btn) {
    content.classList.toggle('expanded');
    btn.textContent = content.classList.contains('expanded') ? '收起' : '展开';
  }
}

async function appendKnowledge(id) {
  const draft = (state.appendDrafts[id] || '').trim();
  if (!draft) {
    showBanner('请输入补充内容。', true);
    return;
  }

  try {
    const result = await state.backend.AppendKnowledge(id, draft);
    state.appendDrafts[id] = '';
    state.openAppendId = '';
    await refreshAll();
    showBanner(result.message, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function deleteKnowledge(id) {
  const targetId = String(id || '').trim();
  if (!targetId) return;
  const knowledge = state.knowledge.find((item) => item.id === targetId);
  if (!knowledge) {
    showBanner('没有找到要删除的记忆。', true);
    return;
  }
  openChatSessionDialog('knowledge-delete', knowledge);
}

async function clearKnowledge() {
  try {
    const ok = await state.backend.ConfirmAction('清空知识库', '确认清空全部记忆吗？这个动作不可撤销。');
    if (!ok) return;

    const result = await state.backend.ClearKnowledge();
    await refreshAll();
    showBanner(result.message, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function setActiveProject(nextProject) {
  const input = document.getElementById('project-name-input');
  const project = (nextProject ?? input?.value ?? '').trim() || 'default';

  try {
    const previousProject = state.projectState.activeProject || 'default';
    state.projectState = normalizeProjectState(await state.backend.SetActiveProject(project));
    renderProjectState();
    await refreshAll();
    await refreshChatState();
    if (state.projectState.activeProject !== previousProject) {
      state.chat.push({
        role: 'system',
        text: `已切换记忆库项目 [${state.projectState.activeProject}]，后续导入和新增记忆会写入这里，对话也会优先检索这个项目。`,
        time: nowLabel(),
      });
      syncCurrentChatConversationFromMessages();
      renderChat();
    }
    showBanner(`已切换记忆库项目 ${state.projectState.activeProject}。`, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function createPrompt() {
  const titleInput = document.getElementById('prompt-title-input');
  const contentInput = document.getElementById('prompt-content-input');
  const title = titleInput?.value.trim() || '';
  const content = contentInput?.value.trim() || '';

  if (!title) {
    showBanner('请输入 Prompt 标题。', true);
    return;
  }
  if (!content) {
    showBanner('请输入 Prompt 内容。', true);
    return;
  }

  try {
    const result = await state.backend.CreatePrompt(title, content);
    if (titleInput) titleInput.value = '';
    if (contentInput) contentInput.value = '';
    await refreshAll();
    showBanner(result.message, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

function togglePromptExpand(id) {
  const content = document.querySelector(`[data-prompt-content-id="${id}"]`);
  const btn = document.querySelector(`[data-action="toggle-expand-prompt"][data-id="${id}"]`);
  if (content && btn) {
    content.classList.toggle('expanded');
    content.classList.toggle('collapsed');
    btn.textContent = content.classList.contains('expanded') ? '收起' : '展开';
  }
}

function insertPromptToChat(id) {
  const prompt = state.prompts.find((item) => item.id === id);
  if (!prompt) {
    showBanner('没有找到对应的 Prompt。', true);
    return;
  }

  const input = document.getElementById('chat-input');
  if (input) {
    input.value = prompt.content || '';
    const cursor = input.value.length;
    input.setSelectionRange(cursor, cursor);
    autoResizeChatInput();
    updateChatAutocomplete();
    input.focus();
  }

  window.navigateTo('chat');
  showBanner(`已将 Prompt #${prompt.shortId} 放入对话输入框。`, false);
}

async function deletePrompt(id) {
  const targetId = String(id || '').trim();
  if (!targetId) return;
  const prompt = state.prompts.find((item) => item.id === targetId);
  if (!prompt) {
    showBanner('没有找到要删除的 Prompt。', true);
    return;
  }
  openChatSessionDialog('prompt-delete', prompt);
}

async function clearPromptsLibrary() {
  try {
    const ok = await state.backend.ConfirmAction('清空 Prompt 库', '确认清空全部 Prompt 吗？这个动作不可撤销。');
    if (!ok) return;

    const result = await state.backend.ClearPrompts();
    await refreshAll();
    showBanner(result.message, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}
