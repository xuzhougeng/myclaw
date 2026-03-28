// Global navigation function
window.navigateTo = function(viewName) {
  // Update nav items
  document.querySelectorAll('.nav-item').forEach(item => {
    item.classList.remove('active');
    if (item.dataset.view === viewName) {
      item.classList.add('active');
    }
  });

  // Update views
  document.querySelectorAll('.view').forEach(view => {
    view.classList.remove('active');
  });
  const targetView = document.getElementById(`view-${viewName}`);
  if (targetView) {
    targetView.classList.add('active');
  }
};

// Theme management
function initTheme() {
  const saved = localStorage.getItem('myclaw-theme');
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  const theme = saved || (prefersDark ? 'dark' : 'light');
  document.documentElement.setAttribute('data-theme', theme);
  updateThemeIcon(theme);
}

function toggleTheme() {
  const current = document.documentElement.getAttribute('data-theme') || 'dark';
  const next = current === 'dark' ? 'light' : 'dark';
  document.documentElement.setAttribute('data-theme', next);
  localStorage.setItem('myclaw-theme', next);
  updateThemeIcon(next);
}

function updateThemeIcon(theme) {
  const icon = document.querySelector('.theme-icon');
  if (icon) {
    icon.textContent = theme === 'dark' ? '◐' : '◑';
  }
}

const state = {
  backend: null,
  backendMode: "",
  overview: null,
  projectState: defaultProjectState(),
  knowledge: [],
  prompts: [],
  skills: [],
  selectedSkillName: "",
  filter: "",
  promptFilter: "",
  filePath: "",
  fileObject: null,
  appendDrafts: {},
  openAppendId: "",
  model: defaultModelState(),
  weixin: defaultWeixinState(),
  chat: [
    {
      role: "assistant",
      text: "桌面前端已接入。你可以在这里导入图片/PDF、直接管理记忆，也可以直接配置模型和微信扫码登录。",
      time: nowLabel(),
    },
  ],
};

let devPollTimer = 0;

const promptExamples = [
  "记住：Windows 版先把桌面前端做稳",
  "/debug-search macOS 什么时候做？",
  "两小时后提醒我喝水",
  "现在我记了什么？",
];

document.addEventListener("DOMContentLoaded", () => {
  void init();
});

async function init() {
  initTheme();
  bindStaticEvents();
  bindNavigation();
  bindQuickAddModal();
  renderChatShortcuts();
  renderProjectState();
  renderChat();
  renderKnowledge();
  renderPrompts();
  renderSkills();
  renderModel();
  renderWeixin();

  try {
    state.backend = await waitForBackend();
    state.backendMode = state.backend.mode || 'wails';
    bindRuntimeEvents();
    startBackendPolling();
    await refreshAll();
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

function bindNavigation() {
  document.querySelectorAll('.nav-item').forEach(item => {
    item.addEventListener('click', (e) => {
      e.preventDefault();
      const viewName = item.dataset.view;
      if (viewName) {
        window.navigateTo(viewName);
      }
    });
  });
}

function bindQuickAddModal() {
  const modal = document.getElementById('quick-add-modal');
  const openBtn = document.getElementById('quick-add-memory');
  const cancelBtn = document.getElementById('quick-add-cancel');
  const confirmBtn = document.getElementById('quick-add-confirm');
  const input = document.getElementById('quick-memory-input');

  if (openBtn) {
    openBtn.addEventListener('click', () => {
      modal.style.display = 'flex';
      input.focus();
    });
  }

  const closeModal = () => {
    modal.style.display = 'none';
    input.value = '';
  };

  if (cancelBtn) {
    cancelBtn.addEventListener('click', closeModal);
  }

  modal.addEventListener('click', (e) => {
    if (e.target === modal) closeModal();
  });

  if (confirmBtn) {
    confirmBtn.addEventListener('click', async () => {
      const text = input.value.trim();
      if (!text) {
        showBanner('请输入记忆内容', true);
        return;
      }
      try {
        const result = await state.backend.CreateKnowledge(text);
        input.value = '';
        closeModal();
        await refreshAll();
        showBanner(result.message, false);
      } catch (error) {
        showBanner(asMessage(error), true);
      }
    });
  }
}

function bindStaticEvents() {
  // Theme toggle
  const themeToggle = document.getElementById('theme-toggle');
  if (themeToggle) {
    themeToggle.addEventListener('click', toggleTheme);
  }

  const projectSwitch = document.getElementById('project-switch');
  if (projectSwitch) {
    projectSwitch.addEventListener('click', () => void setActiveProject());
  }

  const projectNameInput = document.getElementById('project-name-input');
  if (projectNameInput) {
    projectNameInput.addEventListener('keydown', (event) => {
      if (event.key === 'Enter') {
        event.preventDefault();
        void setActiveProject();
      }
    });
  }

  const projectList = document.getElementById('project-list');
  if (projectList) {
    projectList.addEventListener('click', (event) => {
      const target = event.target.closest('[data-project]');
      if (!target) return;

      const project = target.dataset.project || '';
      void setActiveProject(project);
    });
  }

  // File import events
  const dropZone = document.getElementById('drop-zone');
  if (dropZone) {
    dropZone.addEventListener('click', () => void browseFile());

    dropZone.addEventListener('dragover', (e) => {
      e.preventDefault();
      dropZone.classList.add('dragover');
    });

    dropZone.addEventListener('dragleave', () => {
      dropZone.classList.remove('dragover');
    });

    dropZone.addEventListener('drop', async (e) => {
      e.preventDefault();
      dropZone.classList.remove('dragover');
      const file = e.dataTransfer.files[0];
      if (file) {
        handleFileDrop(file);
      }
    });
  }

  const importConfirm = document.getElementById('import-confirm');
  if (importConfirm) {
    importConfirm.addEventListener('click', () => void importFile());
  }

  const fileInput = document.getElementById('http-file-input');
  if (fileInput) {
    fileInput.addEventListener('change', (event) => {
      const file = event.target.files && event.target.files[0];
      if (file) {
        handleFileDrop(file);
      }
    });
  }

  // Memory filter
  const memoryFilter = document.getElementById('memory-filter');
  if (memoryFilter) {
    memoryFilter.addEventListener('input', (event) => {
      state.filter = event.target.value.trim().toLowerCase();
      renderKnowledge();
    });
  }

  // Clear memory
  const clearMemory = document.getElementById('clear-memory');
  if (clearMemory) {
    clearMemory.addEventListener('click', () => void clearKnowledge());
  }

  const promptFilter = document.getElementById('prompt-filter');
  if (promptFilter) {
    promptFilter.addEventListener('input', (event) => {
      state.promptFilter = event.target.value.trim().toLowerCase();
      renderPrompts();
    });
  }

  const savePrompt = document.getElementById('save-prompt');
  if (savePrompt) {
    savePrompt.addEventListener('click', () => void createPrompt());
  }

  const clearPrompts = document.getElementById('clear-prompts');
  if (clearPrompts) {
    clearPrompts.addEventListener('click', () => void clearPromptsLibrary());
  }

  const skillList = document.getElementById('skill-list');
  if (skillList) {
    skillList.addEventListener('click', (event) => {
      const actionTarget = event.target.closest('[data-skill-action]');
      if (actionTarget) {
        const name = actionTarget.dataset.skillName || '';
        if (actionTarget.dataset.skillAction === 'load') {
          void loadSkill(name);
        } else if (actionTarget.dataset.skillAction === 'unload') {
          void unloadSkill(name);
        }
        return;
      }

      const card = event.target.closest('[data-skill-name]');
      if (!card) return;
      state.selectedSkillName = card.dataset.skillName || '';
      renderSkills();
    });
  }

  const skillDetailActions = document.getElementById('skill-detail-actions');
  if (skillDetailActions) {
    skillDetailActions.addEventListener('click', (event) => {
      const target = event.target.closest('[data-skill-action]');
      if (!target) return;

      const name = target.dataset.skillName || '';
      if (target.dataset.skillAction === 'load') {
        void loadSkill(name);
      } else if (target.dataset.skillAction === 'unload') {
        void unloadSkill(name);
      }
    });
  }

  // Chat events
  const chatSend = document.getElementById('chat-send');
  if (chatSend) {
    chatSend.addEventListener('click', () => void sendMessage());
  }

  const chatInput = document.getElementById('chat-input');
  if (chatInput) {
    chatInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        void sendMessage();
      }
    });
  }

  const weixinStart = document.getElementById('weixin-start-login');
  if (weixinStart) {
    weixinStart.addEventListener('click', () => void startWeixinLogin());
  }

  const weixinStop = document.getElementById('weixin-stop-login');
  if (weixinStop) {
    weixinStop.addEventListener('click', () => void cancelWeixinLogin());
  }

  const weixinLogout = document.getElementById('weixin-logout');
  if (weixinLogout) {
    weixinLogout.addEventListener('click', () => void logoutWeixin());
  }

  const modelSave = document.getElementById('model-save');
  if (modelSave) {
    modelSave.addEventListener('click', () => void saveModelConfig());
  }

  const modelTest = document.getElementById('model-test');
  if (modelTest) {
    modelTest.addEventListener('click', () => void testModelConnection());
  }

  const modelClear = document.getElementById('model-clear');
  if (modelClear) {
    modelClear.addEventListener('click', () => void clearModelConfig());
  }

  // Memory list events
  const memoryList = document.getElementById('memory-list');
  if (memoryList) {
    memoryList.addEventListener('click', (event) => {
      const target = event.target.closest('[data-action]');
      if (!target) return;

      const id = target.dataset.id || '';
      switch (target.dataset.action) {
        case 'toggle-expand':
          toggleMemoryExpand(id);
          break;
        case 'toggle-append':
          state.openAppendId = state.openAppendId === id ? '' : id;
          renderKnowledge();
          break;
        case 'delete':
          void deleteKnowledge(id);
          break;
        case 'save-append':
          void appendKnowledge(id);
          break;
      }
    });

    memoryList.addEventListener('input', (event) => {
      const target = event.target;
      if (!(target instanceof HTMLTextAreaElement) || !target.dataset.id) return;
      state.appendDrafts[target.dataset.id] = target.value;
    });
  }

  const promptList = document.getElementById('prompt-list');
  if (promptList) {
    promptList.addEventListener('click', (event) => {
      const target = event.target.closest('[data-action]');
      if (!target) return;

      const id = target.dataset.id || '';
      switch (target.dataset.action) {
        case 'toggle-expand-prompt':
          togglePromptExpand(id);
          break;
        case 'insert-prompt':
          insertPromptToChat(id);
          break;
        case 'delete-prompt':
          void deletePrompt(id);
          break;
      }
    });
  }
}

function bindRuntimeEvents() {
  if (!window.runtime || typeof window.runtime.EventsOn !== 'function') return;

  window.runtime.EventsOn('reminder:due', (payload) => {
    const reminder = Array.isArray(payload) ? payload[0] : payload;
    if (!reminder) return;

    const shortId = reminder.shortId || reminder.id || 'notice';
    const message = reminder.message || '提醒触发';
    state.chat.push({
      role: 'system',
      text: `[提醒 #${shortId}] ${message}`,
      time: nowLabel(),
    });
    renderChat();
    showBanner(`提醒 #${shortId}: ${message}`, false);
  });

  window.runtime.EventsOn('weixin:status', (payload) => {
    const next = normalizeWeixinStatus(payload);
    applyWeixinStatus(next, true);
  });
}

function renderChatShortcuts() {
  document.querySelectorAll('.shortcut-chip').forEach(chip => {
    chip.addEventListener('click', () => {
      const input = document.getElementById('chat-input');
      if (input) {
        input.value = chip.dataset.cmd || '';
        input.focus();
      }
    });
  });
}

async function handleFileDrop(file) {
  state.fileObject = file;
  const path = file.path || file.name;
  state.filePath = path;
  updateFilePreview(file);
}

function updateFilePreview(file) {
  const preview = document.getElementById('file-preview');
  const fileName = document.getElementById('file-name');
  const fileSize = document.getElementById('file-size');
  const fileIcon = document.getElementById('file-icon');

  if (preview) preview.classList.add('has-file');
  if (fileName) fileName.textContent = file.name;

  const sizeValue = Number(file.size || 0);
  const sizeMB = (sizeValue / (1024 * 1024)).toFixed(2);
  if (fileSize) fileSize.textContent = sizeValue > 0 ? `${sizeMB} MB` : '本地文件';

  if (fileIcon) {
    if ((file.type || '').includes('image')) {
      fileIcon.textContent = '🖼';
    } else if ((file.type || '').includes('pdf') || /\.pdf$/i.test(file.name || '')) {
      fileIcon.textContent = '📕';
    } else {
      fileIcon.textContent = '📄';
    }
  }
}

async function waitForBackend() {
  for (let index = 0; index < 80; index += 1) {
    const backend = window.go && window.go.main && window.go.main.DesktopApp;
    if (backend) return createWailsBackend(backend);
    await delay(50);
  }
  if (window.location.protocol === 'http:' || window.location.protocol === 'https:') {
    return createHTTPBackend();
  }
  throw new Error('Wails 后端尚未就绪。');
}

function createWailsBackend(backend) {
  return {
    mode: 'wails',
    GetOverview: () => backend.GetOverview(),
    GetProjectState: () => backend.GetProjectState(),
    SetActiveProject: (name) => backend.SetActiveProject(name),
    ListKnowledge: () => backend.ListKnowledge(),
    CreateKnowledge: (text) => backend.CreateKnowledge(text),
    AppendKnowledge: (idOrPrefix, addition) => backend.AppendKnowledge(idOrPrefix, addition),
    DeleteKnowledge: (idOrPrefix) => backend.DeleteKnowledge(idOrPrefix),
    ClearKnowledge: () => backend.ClearKnowledge(),
    ListPrompts: () => backend.ListPrompts(),
    CreatePrompt: (title, content) => backend.CreatePrompt(title, content),
    DeletePrompt: (idOrPrefix) => backend.DeletePrompt(idOrPrefix),
    ClearPrompts: () => backend.ClearPrompts(),
    ListSkills: () => backend.ListSkills(),
    LoadSkill: (name) => backend.LoadSkill(name),
    UnloadSkill: (name) => backend.UnloadSkill(name),
    ConfirmAction: (title, message) => backend.ConfirmAction(title, message),
    OpenImportDialog: () => backend.OpenImportDialog(),
    ImportFile: (path) => backend.ImportFile(path),
    UploadImportFile: () => Promise.reject(new Error('Wails 模式不使用浏览器上传。')),
    SendMessage: (input) => backend.SendMessage(input),
    GetModelSettings: () => backend.GetModelSettings(),
    SaveModelConfig: (payload) => backend.SaveModelConfig(payload),
    TestModelConnection: () => backend.TestModelConnection(),
    ClearModelConfig: () => backend.ClearModelConfig(),
    GetWeixinStatus: () => backend.GetWeixinStatus(),
    StartWeixinLogin: () => backend.StartWeixinLogin(),
    CancelWeixinLogin: () => backend.CancelWeixinLogin(),
    LogoutWeixin: () => backend.LogoutWeixin(),
  };
}

function createHTTPBackend() {
  return {
    mode: 'http',
    GetOverview: () => requestJSON('GET', '/api/overview'),
    GetProjectState: () => requestJSON('GET', '/api/projects'),
    SetActiveProject: (name) => requestJSON('POST', '/api/projects/active', { name }),
    ListKnowledge: () => requestJSON('GET', '/api/knowledge'),
    CreateKnowledge: (text) => requestJSON('POST', '/api/knowledge', { text }),
    AppendKnowledge: (idOrPrefix, addition) => requestJSON('POST', '/api/knowledge/append', { idOrPrefix, addition }),
    DeleteKnowledge: (idOrPrefix) => requestJSON('POST', '/api/knowledge/delete', { idOrPrefix }),
    ClearKnowledge: () => requestJSON('POST', '/api/knowledge/clear'),
    ListPrompts: () => requestJSON('GET', '/api/prompts'),
    CreatePrompt: (title, content) => requestJSON('POST', '/api/prompts', { title, content }),
    DeletePrompt: (idOrPrefix) => requestJSON('POST', '/api/prompts/delete', { idOrPrefix }),
    ClearPrompts: () => requestJSON('POST', '/api/prompts/clear'),
    ListSkills: () => requestJSON('GET', '/api/skills'),
    LoadSkill: (name) => requestJSON('POST', '/api/skills/load', { name }),
    UnloadSkill: (name) => requestJSON('POST', '/api/skills/unload', { name }),
    ConfirmAction: async (title, message) => window.confirm(`${title}\n\n${message}`),
    OpenImportDialog: async () => '',
    ImportFile: async () => {
      throw new Error('HTTP 模式请直接选择本地文件上传。');
    },
    UploadImportFile: (file) => uploadFile('/api/import/upload', file),
    SendMessage: (input) => requestJSON('POST', '/api/chat', { input }),
    GetModelSettings: () => requestJSON('GET', '/api/model'),
    SaveModelConfig: (payload) => requestJSON('POST', '/api/model/save', payload),
    TestModelConnection: () => requestJSON('POST', '/api/model/test'),
    ClearModelConfig: () => requestJSON('POST', '/api/model/clear'),
    GetWeixinStatus: () => requestJSON('GET', '/api/weixin/status'),
    StartWeixinLogin: () => requestJSON('POST', '/api/weixin/login'),
    CancelWeixinLogin: () => requestJSON('POST', '/api/weixin/cancel'),
    LogoutWeixin: () => requestJSON('POST', '/api/weixin/logout'),
  };
}

async function requestJSON(method, url, body) {
  const options = { method, headers: {} };
  if (body !== undefined) {
    options.headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(body);
  }

  const response = await fetch(url, options);
  const text = await response.text();
  const payload = text ? JSON.parse(text) : null;
  if (!response.ok) {
    throw new Error((payload && payload.error) || `HTTP ${response.status}`);
  }
  return payload;
}

async function uploadFile(url, file) {
  const formData = new FormData();
  formData.set('file', file, file.name || 'upload.bin');

  const response = await fetch(url, {
    method: 'POST',
    body: formData,
  });
  const text = await response.text();
  const payload = text ? JSON.parse(text) : null;
  if (!response.ok) {
    throw new Error((payload && payload.error) || `HTTP ${response.status}`);
  }
  return payload;
}

function startBackendPolling() {
  window.clearInterval(devPollTimer);
  if (state.backendMode !== 'http') return;

  devPollTimer = window.setInterval(() => {
    void Promise.all([refreshProjectState(), refreshOverview(), refreshModel(), refreshWeixin()]).catch(() => {});
  }, 2000);
}

async function refreshAll() {
  await Promise.all([refreshProjectState(), refreshOverview(), refreshKnowledge(), refreshPrompts(), refreshSkills(), refreshModel(), refreshWeixin()]);
}

async function refreshProjectState() {
  state.projectState = normalizeProjectState(await state.backend.GetProjectState());
  renderProjectState();
}

async function refreshOverview() {
  state.overview = await state.backend.GetOverview();

  // Update dashboard stats
  const dataDirStat = document.getElementById('data-dir-stat');
  const dataDirPath = document.getElementById('data-dir-path');
  const memoryCountStat = document.getElementById('memory-count-stat');
  const promptCountStat = document.getElementById('prompt-count-stat');
  const aiStatusStat = document.getElementById('ai-status-stat');
  const aiMessageStat = document.getElementById('ai-message-stat');
  const weixinStatusStat = document.getElementById('weixin-status-stat');
  const weixinMessageStat = document.getElementById('weixin-message-stat');

  if (dataDirStat) dataDirStat.textContent = '已配置';
  if (dataDirPath) dataDirPath.textContent = state.overview.dataDir;
  if (memoryCountStat) memoryCountStat.textContent = String(state.overview.knowledgeCount);
  if (promptCountStat) promptCountStat.textContent = String(state.overview.promptCount || 0);
  if (aiStatusStat) aiStatusStat.textContent = state.overview.aiAvailable ? '已配置' : '未配置';
  if (aiMessageStat) aiMessageStat.textContent = state.overview.aiMessage;
  if (weixinStatusStat) weixinStatusStat.textContent = state.overview.weixinConnected ? '已连接' : '未连接';
  if (weixinMessageStat) weixinMessageStat.textContent = state.overview.weixinMessage || '未连接微信';

  // Update sidebar compact stats
  const aiStatusCompact = document.getElementById('ai-status-compact');
  const memoryCountCompact = document.getElementById('memory-count-compact');
  const promptCountCompact = document.getElementById('prompt-count-compact');

  if (aiStatusCompact) aiStatusCompact.textContent = state.overview.aiAvailable ? 'OK' : '—';
  if (memoryCountCompact) memoryCountCompact.textContent = String(state.overview.knowledgeCount);
  if (promptCountCompact) promptCountCompact.textContent = String(state.overview.promptCount || 0);
}

async function refreshKnowledge() {
  state.knowledge = await state.backend.ListKnowledge();
  renderKnowledge();
}

async function refreshPrompts() {
  state.prompts = await state.backend.ListPrompts();
  renderPrompts();
}

async function refreshSkills() {
  state.skills = normalizeSkills(await state.backend.ListSkills());
  ensureSelectedSkill();
  renderSkills();
}

async function refreshModel() {
  state.model = normalizeModelSettings(await state.backend.GetModelSettings());
  renderModel();
}

async function refreshWeixin() {
  const next = await state.backend.GetWeixinStatus();
  applyWeixinStatus(next, false);
}

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
    renderChat();
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
  try {
    const ok = await state.backend.ConfirmAction('删除记忆', `确认删除 #${id.slice(0, 8)} 吗？`);
    if (!ok) return;

    const result = await state.backend.DeleteKnowledge(id);
    await refreshAll();
    showBanner(result.message, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
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
    if (state.projectState.activeProject !== previousProject) {
      state.chat.push({
        role: 'system',
        text: `已切换到项目 [${state.projectState.activeProject}]，后续导入、记忆和对话检索都会只使用这个项目。`,
        time: nowLabel(),
      });
      renderChat();
    }
    showBanner(`已切换到项目 ${state.projectState.activeProject}。`, false);
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
    input.focus();
  }

  window.navigateTo('chat');
  showBanner(`已将 Prompt #${prompt.shortId} 放入对话输入框。`, false);
}

async function deletePrompt(id) {
  try {
    const ok = await state.backend.ConfirmAction('删除 Prompt', `确认删除 Prompt #${id.slice(0, 8)} 吗？`);
    if (!ok) return;

    const result = await state.backend.DeletePrompt(id);
    await refreshAll();
    showBanner(result.message, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
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

async function sendMessage() {
  const input = document.getElementById('chat-input');
  const text = input?.value.trim();
  if (!text) return;

  state.chat.push({ role: 'user', text, time: nowLabel() });
  renderChat();
  if (input) input.value = '';

  try {
    const result = await state.backend.SendMessage(text);
    state.chat.push({
      role: 'assistant',
      text: result.reply,
      time: result.timestamp || nowLabel(),
    });
    renderChat();
    await refreshAll();
  } catch (error) {
    state.chat.push({
      role: 'system',
      text: asMessage(error),
      time: nowLabel(),
    });
    renderChat();
    showBanner(asMessage(error), true);
  }
}

function renderKnowledge() {
  const container = document.getElementById('memory-list');
  if (!container) return;

  const filtered = state.knowledge.filter((item) => {
    if (!state.filter) return true;
    const haystack = [item.id, item.shortId, item.source, item.text, ...(item.keywords || [])]
      .join(' ')
      .toLowerCase();
    return haystack.includes(state.filter);
  });

  if (filtered.length === 0) {
    const activeProject = state.projectState.activeProject || 'default';
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">◈</div>
        <h3>${state.filter ? '没有找到匹配的记忆' : `项目 ${escapeHTML(activeProject)} 为空`}</h3>
        <p>${state.filter ? '尝试其他关键词' : '切换项目或导入文件、直接添加记忆来开始使用'}</p>
      </div>
    `;
    return;
  }

  container.innerHTML = filtered
    .map((item) => {
      const isOpen = state.openAppendId === item.id;
      const isExpanded = state.expandedIds?.has(item.id);

      return `
        <article class="memory-card">
          <div class="memory-card-header">
            <div class="memory-meta">
              <span class="memory-badge id">#${escapeHTML(item.shortId)}</span>
              ${item.isFile ? '<span class="memory-badge source">文件</span>' : ''}
              ${item.source ? `<span class="memory-badge source">${escapeHTML(item.source)}</span>` : ''}
            </div>
          </div>
          <div class="memory-content ${isExpanded ? 'expanded' : 'collapsed'}" data-content-id="${escapeAttribute(item.id)}">
            ${escapeHTML(item.preview)}
          </div>
          <div class="memory-card-footer">
            <span class="memory-date">${escapeHTML(item.recordedAt)}</span>
            <div class="memory-actions">
              <button class="btn btn-ghost btn-sm" data-action="toggle-expand" data-id="${escapeAttribute(item.id)}">
                ${isExpanded ? '收起' : '展开'}
              </button>
              <button class="btn btn-ghost btn-sm" data-action="toggle-append" data-id="${escapeAttribute(item.id)}">
                ${isOpen ? '收起' : '补充'}
              </button>
              <button class="btn btn-ghost btn-sm" data-action="delete" data-id="${escapeAttribute(item.id)}">
                删除
              </button>
            </div>
          </div>
          ${
            isOpen
              ? `
                <div style="margin-top: 12px;">
                  <textarea
                    style="width: 100%; min-height: 60px;"
                    data-id="${escapeAttribute(item.id)}"
                    placeholder="补充这一条记忆的新增事实。"
                  >${escapeHTML(state.appendDrafts[item.id] || '')}</textarea>
                  <button class="btn btn-primary btn-sm" style="margin-top: 8px;" data-action="save-append" data-id="${escapeAttribute(item.id)}">
                    保存补充
                  </button>
                </div>
              `
              : ''
          }
        </article>
      `;
    })
    .join('');
}

function renderProjectState() {
  const activeProject = state.projectState.activeProject || 'default';
  const activeSummary = (state.projectState.projects || []).find((item) => item.active) || {
    name: activeProject,
    knowledgeCount: 0,
  };

  const compact = document.getElementById('project-name-compact');
  const display = document.getElementById('project-name-display');
  const summary = document.getElementById('project-summary-display');
  const input = document.getElementById('project-name-input');
  const list = document.getElementById('project-list');

  if (compact) compact.textContent = activeProject;
  if (display) display.textContent = activeProject;
  if (summary) summary.textContent = `${activeSummary.knowledgeCount || 0} 条记忆会用于当前导入与对话检索`;
  if (input && document.activeElement !== input) input.value = activeProject;

  if (!list) return;

  const projects = state.projectState.projects || [];
  if (projects.length === 0) {
    list.innerHTML = '';
    return;
  }

  list.innerHTML = projects
    .map((item) => `
      <button class="project-chip ${item.active ? 'active' : ''}" data-project="${escapeAttribute(item.name)}">
        <span>${escapeHTML(item.name)}</span>
        <span class="project-chip-meta">${escapeHTML(String(item.knowledgeCount || 0))} 条</span>
      </button>
    `)
    .join('');
}

function renderPrompts() {
  const container = document.getElementById('prompt-list');
  if (!container) return;

  const filtered = state.prompts.filter((item) => {
    if (!state.promptFilter) return true;
    const haystack = [item.id, item.shortId, item.title, item.content]
      .join(' ')
      .toLowerCase();
    return haystack.includes(state.promptFilter);
  });

  if (filtered.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">✦</div>
        <h3>${state.promptFilter ? '没有找到匹配的 Prompt' : 'Prompt 库为空'}</h3>
        <p>${state.promptFilter ? '尝试其他关键词' : '把常用提示词模板沉淀在这里。'}</p>
      </div>
    `;
    return;
  }

  container.innerHTML = filtered
    .map((item) => `
      <article class="memory-card">
        <div class="memory-card-header">
          <div>
            <div class="memory-meta">
              <span class="memory-badge id">#${escapeHTML(item.shortId)}</span>
            </div>
            <h3 class="prompt-card-title">${escapeHTML(item.title)}</h3>
          </div>
        </div>
        <div class="memory-content collapsed" data-prompt-content-id="${escapeAttribute(item.id)}">
          ${escapeHTML(item.content)}
        </div>
        <div class="memory-card-footer">
          <span class="memory-date">${escapeHTML(item.recordedAt)}</span>
          <div class="memory-actions">
            <button class="btn btn-ghost btn-sm" data-action="toggle-expand-prompt" data-id="${escapeAttribute(item.id)}">
              展开
            </button>
            <button class="btn btn-ghost btn-sm" data-action="insert-prompt" data-id="${escapeAttribute(item.id)}">
              插入对话
            </button>
            <button class="btn btn-ghost btn-sm" data-action="delete-prompt" data-id="${escapeAttribute(item.id)}">
              删除
            </button>
          </div>
        </div>
      </article>
    `)
    .join('');
}

function normalizeSkills(payload) {
  if (!Array.isArray(payload)) return [];
  return payload.map((item) => ({
    name: item.name || '',
    description: item.description || '',
    content: item.content || '',
    dir: item.dir || '',
    loaded: Boolean(item.loaded),
  }));
}

function ensureSelectedSkill() {
  if (state.skills.length === 0) {
    state.selectedSkillName = '';
    return;
  }

  const existing = state.skills.find((item) => item.name === state.selectedSkillName);
  if (existing) return;

  const preferred = state.skills.find((item) => item.loaded) || state.skills[0];
  state.selectedSkillName = preferred ? preferred.name : '';
}

async function loadSkill(name) {
  if (!name) return;

  try {
    const result = await state.backend.LoadSkill(name);
    await refreshSkills();
    showBanner(result.message, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function unloadSkill(name) {
  if (!name) return;

  try {
    const result = await state.backend.UnloadSkill(name);
    await refreshSkills();
    showBanner(result.message, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

function renderSkills() {
  const list = document.getElementById('skill-list');
  const loadedCount = document.getElementById('skill-loaded-count');
  const detailName = document.getElementById('skill-detail-name');
  const detailDescription = document.getElementById('skill-detail-description');
  const detailPath = document.getElementById('skill-detail-path');
  const detailContent = document.getElementById('skill-detail-content');
  const detailActions = document.getElementById('skill-detail-actions');
  if (!list || !detailName || !detailDescription || !detailPath || !detailContent || !detailActions) return;

  const skills = [...state.skills].sort((left, right) => {
    if (left.loaded !== right.loaded) return left.loaded ? -1 : 1;
    return left.name.localeCompare(right.name, 'zh-CN');
  });
  const loadedTotal = skills.filter((item) => item.loaded).length;
  if (loadedCount) loadedCount.textContent = `${loadedTotal} 个已加载`;

  if (skills.length === 0) {
    list.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">⌬</div>
        <h3>还没有技能</h3>
        <p>把技能放进本地 skills 目录后，这里会自动显示。</p>
      </div>
    `;
    detailName.textContent = '暂无技能';
    detailDescription.textContent = '当前还没有发现可用 skill。';
    detailPath.textContent = '—';
    detailContent.innerHTML = '<p>把技能放进本地 skills 目录后，这里会显示内容。</p>';
    detailActions.innerHTML = '';
    return;
  }

  ensureSelectedSkill();
  const selected = skills.find((item) => item.name === state.selectedSkillName) || skills[0];
  if (selected) {
    state.selectedSkillName = selected.name;
  }

  list.innerHTML = skills
    .map((item) => `
      <article class="skill-item ${item.loaded ? 'loaded' : ''} ${selected && selected.name === item.name ? 'active' : ''}" data-skill-name="${escapeAttribute(item.name)}">
        <div class="skill-item-top">
          <div class="skill-item-name">${escapeHTML(item.name)}</div>
          <span class="skill-item-pill ${item.loaded ? 'loaded' : ''}">${item.loaded ? '已加载' : '未加载'}</span>
        </div>
        <p class="skill-item-desc">${escapeHTML(item.description || '这个技能没有填写描述。')}</p>
        <div class="skill-item-meta">${escapeHTML(item.dir || '未提供目录')}</div>
        <div class="skill-actions">
          <button class="btn ${item.loaded ? 'btn-ghost' : 'btn-primary'} btn-sm" data-skill-action="${item.loaded ? 'unload' : 'load'}" data-skill-name="${escapeAttribute(item.name)}">
            ${item.loaded ? '卸载' : '加载'}
          </button>
        </div>
      </article>
    `)
    .join('');

  detailName.textContent = selected.name;
  detailDescription.textContent = selected.description || '这个技能没有填写描述。';
  detailPath.textContent = selected.dir || '未提供目录';
  detailActions.innerHTML = `
    <button class="btn ${selected.loaded ? 'btn-ghost' : 'btn-primary'} btn-sm" data-skill-action="${selected.loaded ? 'unload' : 'load'}" data-skill-name="${escapeAttribute(selected.name)}">
      ${selected.loaded ? '卸载技能' : '加载技能'}
    </button>
  `;

  const visibleContent = stripFrontmatter(selected.content).trim();
  detailContent.innerHTML = visibleContent
    ? renderMarkdown(visibleContent)
    : '<p>这个技能没有可显示的正文内容。</p>';
}

async function saveModelConfig() {
  try {
    const result = await state.backend.SaveModelConfig(readModelForm());
    state.model = normalizeModelSettings(result);
    renderModel();
    await refreshOverview();
    showBanner('模型配置已保存。', false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function testModelConnection() {
  try {
    const result = await state.backend.TestModelConnection();
    await refreshAll();
    showBanner(result.message, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function clearModelConfig() {
  try {
    const ok = await state.backend.ConfirmAction('清空模型配置', '确认清空本地保存的模型配置吗？');
    if (!ok) return;

    const result = await state.backend.ClearModelConfig();
    await refreshAll();
    showBanner(result.message, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

function readModelForm() {
  return {
    provider: document.getElementById('model-provider')?.value.trim() || '',
    baseUrl: document.getElementById('model-base-url')?.value.trim() || '',
    apiKey: document.getElementById('model-api-key')?.value.trim() || '',
    model: document.getElementById('model-name')?.value.trim() || '',
  };
}

function renderModel() {
  const provider = document.getElementById('model-provider');
  const baseUrl = document.getElementById('model-base-url');
  const apiKey = document.getElementById('model-api-key');
  const model = document.getElementById('model-name');
  const pill = document.getElementById('model-status-pill');
  const message = document.getElementById('model-message');
  const effectiveProvider = document.getElementById('effective-provider');
  const effectiveBaseUrl = document.getElementById('effective-base-url');
  const effectiveModel = document.getElementById('effective-model');
  const effectiveApiKey = document.getElementById('effective-api-key');
  const envOverrideBox = document.getElementById('model-env-override-box');
  const envOverrideText = document.getElementById('model-env-override-text');

  if (provider && document.activeElement !== provider) provider.value = state.model.provider || '';
  if (baseUrl && document.activeElement !== baseUrl) baseUrl.value = state.model.baseUrl || '';
  if (apiKey && document.activeElement !== apiKey) apiKey.value = state.model.apiKey || '';
  if (model && document.activeElement !== model) model.value = state.model.model || '';

  if (pill) {
    pill.className = `status-pill ${state.model.configured ? 'on' : 'off'}`;
    pill.textContent = state.model.configured ? '已配置' : '未配置';
  }
  if (message) {
    const missing = (state.model.missingFields || []).length > 0
      ? ` 缺少：${state.model.missingFields.join('、')}。`
      : '';
    message.textContent = `${state.model.message || '尚未保存本地模型配置。'}${missing}`;
  }

  if (effectiveProvider) effectiveProvider.textContent = state.model.effectiveProvider || '—';
  if (effectiveBaseUrl) effectiveBaseUrl.textContent = state.model.effectiveBaseUrl || '—';
  if (effectiveModel) effectiveModel.textContent = state.model.effectiveModel || '—';
  if (effectiveApiKey) effectiveApiKey.textContent = state.model.effectiveApiKeyMasked || '—';

  if (envOverrideBox) {
    const overrides = state.model.envOverrides || [];
    envOverrideBox.hidden = overrides.length === 0;
    if (!envOverrideBox.hidden && envOverrideText) {
      envOverrideText.textContent = `以下字段当前由环境变量覆盖：${overrides.join('、')}。`;
    }
  }
}

async function startWeixinLogin() {
  try {
    const status = await state.backend.StartWeixinLogin();
    applyWeixinStatus(status, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function cancelWeixinLogin() {
  try {
    const result = await state.backend.CancelWeixinLogin();
    showBanner(result.message, false);
    await refreshWeixin();
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function logoutWeixin() {
  try {
    if (state.weixin.connected) {
      const ok = await state.backend.ConfirmAction('退出微信', '确认让桌面端退出当前微信登录吗？');
      if (!ok) return;
    }

    const result = await state.backend.LogoutWeixin();
    showBanner(result.message, false);
    await refreshAll();
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

function applyWeixinStatus(nextStatus, fromEvent) {
  const normalized = normalizeWeixinStatus(nextStatus);
  const previous = state.weixin;
  state.weixin = normalized;
  renderWeixin();
  if (state.overview) {
    state.overview.weixinConnected = normalized.connected;
    state.overview.weixinMessage = normalized.message;
    const weixinStatusStat = document.getElementById('weixin-status-stat');
    const weixinMessageStat = document.getElementById('weixin-message-stat');
    if (weixinStatusStat) weixinStatusStat.textContent = normalized.connected ? '已连接' : '未连接';
    if (weixinMessageStat) weixinMessageStat.textContent = normalized.message || '未连接微信';
  }

  if (normalized.connected && !previous.connected) {
    state.chat.push({
      role: 'system',
      text: '微信已连接，desktop 会直接接收微信消息。',
      time: nowLabel(),
    });
    renderChat();
  }

  if (fromEvent && normalized.message && normalized.message !== previous.message) {
    const isError = !normalized.connected && !normalized.loggingIn && /(失败|超时|失效|中断)/.test(normalized.message);
    showBanner(normalized.message, isError);
  }
}

function renderWeixin() {
  const pill = document.getElementById('weixin-status-pill');
  const copy = document.getElementById('weixin-status-copy');
  const qrImage = document.getElementById('weixin-qr-image');
  const qrEmpty = document.getElementById('weixin-qr-empty');
  const qrCaption = document.getElementById('weixin-qr-caption');
  const account = document.getElementById('weixin-account');
  const accountId = document.getElementById('weixin-account-id');
  const userId = document.getElementById('weixin-user-id');
  const startButton = document.getElementById('weixin-start-login');
  const stopButton = document.getElementById('weixin-stop-login');
  const logoutButton = document.getElementById('weixin-logout');

  if (pill) {
    pill.className = `status-pill ${state.weixin.connected ? 'on' : state.weixin.loggingIn ? 'pending' : 'off'}`;
    pill.textContent = state.weixin.connected ? '已连接' : state.weixin.loggingIn ? '等待扫码' : '未连接';
  }
  if (copy) {
    copy.textContent = state.weixin.message || '未连接微信，可在桌面端直接扫码登录。';
  }
  if (account) {
    account.hidden = !state.weixin.connected;
  }
  if (accountId) {
    accountId.textContent = state.weixin.accountId || '—';
  }
  if (userId) {
    userId.textContent = state.weixin.userId || '—';
  }

  if (startButton) {
    startButton.disabled = state.weixin.connected || state.weixin.loggingIn;
    startButton.textContent = state.weixin.loggingIn ? '等待扫码' : '生成二维码';
  }
  if (stopButton) {
    stopButton.disabled = !state.weixin.loggingIn;
  }
  if (logoutButton) {
    logoutButton.disabled = !state.weixin.connected;
  }

  if (qrImage && state.weixin.qrCodeDataUrl) {
    qrImage.hidden = false;
    qrImage.src = state.weixin.qrCodeDataUrl;
  } else if (qrImage) {
    qrImage.hidden = true;
    qrImage.removeAttribute('src');
  }

  if (qrEmpty) {
    qrEmpty.hidden = Boolean(state.weixin.qrCodeDataUrl);
    const title = qrEmpty.querySelector('h3');
    const desc = qrEmpty.querySelector('p');
    if (title) {
      title.textContent = state.weixin.connected ? '微信已连接' : state.weixin.loggingIn ? '等待扫码确认' : '等待生成二维码';
    }
    if (desc) {
      desc.textContent = state.weixin.connected
        ? '当前 desktop 已绑定微信，会直接接收消息。'
        : state.weixin.loggingIn
          ? '请在手机上完成扫码并确认登录。'
          : '点击左侧按钮后，在这里直接扫码即可。';
    }
  }

  if (qrCaption) {
    qrCaption.textContent = state.weixin.connected
      ? '当前登录已生效，微信消息会直接进入 desktop 后台。'
      : state.weixin.loggingIn
        ? '二维码有效期 8 分钟，扫码后状态会自动刷新。'
        : '二维码会在本窗口内展示。';
  }
}

function renderChat() {
  const container = document.getElementById('chat-list');
  if (!container) return;

  if (state.chat.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">○</div>
        <h3>开始新对话</h3>
        <p>输入问题或使用命令如 /remember、/notice、/forget</p>
      </div>
    `;
    return;
  }

  container.innerHTML = state.chat
    .map(
      (message) => `
        <div class="chat-message ${escapeAttribute(message.role)}">
          <div class="chat-avatar">${message.role === 'user' ? '◐' : message.role === 'system' ? '◇' : '○'}</div>
          <div class="chat-bubble">
            <div class="chat-markdown">${renderMarkdown(message.text)}</div>
            <span class="chat-time">${escapeHTML(message.time)}</span>
          </div>
        </div>
      `,
    )
    .join('');
  container.scrollTop = container.scrollHeight;
}

let bannerTimer = 0;

function defaultProjectState() {
  return {
    activeProject: 'default',
    projects: [
      {
        name: 'default',
        knowledgeCount: 0,
        latestRecordedAt: '',
        latestRecordedAtUnix: 0,
        active: true,
      },
    ],
  };
}

function normalizeProjectState(payload) {
  const source = Array.isArray(payload) ? payload[0] : payload;
  const stateValue = {
    ...defaultProjectState(),
    ...(source || {}),
  };
  const projects = Array.isArray(stateValue.projects)
    ? stateValue.projects.map((item) => ({
        name: item.name || 'default',
        knowledgeCount: Number(item.knowledgeCount || 0),
        latestRecordedAt: item.latestRecordedAt || '',
        latestRecordedAtUnix: Number(item.latestRecordedAtUnix || 0),
        active: Boolean(item.active),
      }))
    : [];

  if (projects.length === 0) {
    return defaultProjectState();
  }

  return {
    activeProject: stateValue.activeProject || projects.find((item) => item.active)?.name || 'default',
    projects,
  };
}

function defaultModelState() {
  return {
    provider: 'openai',
    baseUrl: 'https://api.openai.com/v1',
    apiKey: '',
    model: '',
    configured: false,
    saved: false,
    missingFields: [],
    envOverrides: [],
    effectiveProvider: 'openai',
    effectiveBaseUrl: 'https://api.openai.com/v1',
    effectiveApiKeyMasked: '(empty)',
    effectiveModel: '',
    message: '尚未保存本地模型配置。',
  };
}

function normalizeModelSettings(payload) {
  const source = Array.isArray(payload) ? payload[0] : payload;
  return {
    ...defaultModelState(),
    ...(source || {}),
  };
}

function defaultWeixinState() {
  return {
    connected: false,
    loggingIn: false,
    hasAccount: false,
    accountId: '',
    userId: '',
    qrCode: '',
    qrCodeDataUrl: '',
    message: '未连接微信，可在桌面端直接生成二维码扫码登录。',
  };
}

function normalizeWeixinStatus(payload) {
  const source = Array.isArray(payload) ? payload[0] : payload;
  return {
    ...defaultWeixinState(),
    ...(source || {}),
  };
}

function showBanner(message, isError) {
  const banner = document.getElementById('banner');
  if (!banner) return;

  banner.hidden = false;
  banner.textContent = message;
  banner.style.borderColor = isError ? 'var(--danger)' : 'var(--accent-primary)';

  window.clearTimeout(bannerTimer);
  bannerTimer = window.setTimeout(() => {
    banner.hidden = true;
  }, 3200);
}

function delay(ms) {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms);
  });
}

function nowLabel() {
  return new Date().toLocaleString('zh-CN', { hour12: false });
}

function asMessage(error) {
  if (!error) return '发生未知错误。';
  if (typeof error === 'string') return error;
  if (error.message) return error.message;
  return String(error);
}

function renderMarkdown(source) {
  const normalized = String(source ?? '').replace(/\r\n?/g, '\n');
  const lines = normalized.split('\n');
  const blocks = [];

  for (let index = 0; index < lines.length;) {
    const line = lines[index];

    if (!line.trim()) {
      index += 1;
      continue;
    }

    const fence = line.match(/^```([\w+-]*)\s*$/);
    if (fence) {
      const codeLines = [];
      index += 1;
      while (index < lines.length && !/^```/.test(lines[index])) {
        codeLines.push(lines[index]);
        index += 1;
      }
      if (index < lines.length) index += 1;
      const language = fence[1] ? ` class="language-${escapeAttribute(fence[1])}"` : '';
      blocks.push(`<pre><code${language}>${escapeHTML(codeLines.join('\n'))}</code></pre>`);
      continue;
    }

    const heading = line.match(/^(#{1,6})\s+(.*)$/);
    if (heading) {
      const level = heading[1].length;
      blocks.push(`<h${level}>${renderInlineMarkdown(heading[2].trim())}</h${level}>`);
      index += 1;
      continue;
    }

    if (/^([-*_])(?:\s*\1){2,}\s*$/.test(line)) {
      blocks.push('<hr>');
      index += 1;
      continue;
    }

    if (/^\s*>/.test(line)) {
      const quoteLines = [];
      while (index < lines.length && /^\s*>/.test(lines[index])) {
        quoteLines.push(lines[index].replace(/^\s*>\s?/, ''));
        index += 1;
      }
      blocks.push(`<blockquote>${renderMarkdown(quoteLines.join('\n'))}</blockquote>`);
      continue;
    }

    const table = parseMarkdownTable(lines, index);
    if (table) {
      blocks.push(table.html);
      index = table.nextIndex;
      continue;
    }

    const list = parseMarkdownList(lines, index);
    if (list) {
      blocks.push(list.html);
      index = list.nextIndex;
      continue;
    }

    const paragraphLines = [];
    while (index < lines.length) {
      const current = lines[index];
      if (!current.trim()) break;
      if (
        /^```/.test(current) ||
        /^(#{1,6})\s+/.test(current) ||
        /^([-*_])(?:\s*\1){2,}\s*$/.test(current) ||
        /^\s*>/.test(current) ||
        parseMarkdownTable(lines, index) ||
        parseMarkdownList(lines, index)
      ) {
        break;
      }
      paragraphLines.push(current.trim());
      index += 1;
    }
    blocks.push(`<p>${renderInlineMarkdown(paragraphLines.join('\n'))}</p>`);
  }

  return blocks.join('');
}

function parseMarkdownList(lines, startIndex) {
  const firstLine = lines[startIndex];
  const firstMatch = firstLine.match(/^(\s*)([-*+]|\d+\.)\s+(.*)$/);
  if (!firstMatch) return null;

  const ordered = /\d+\./.test(firstMatch[2]);
  const itemPattern = ordered ? /^(\s*)\d+\.\s+(.*)$/ : /^(\s*)[-*+]\s+(.*)$/;
  const items = [];
  let index = startIndex;

  while (index < lines.length) {
    const line = lines[index];
    if (!line.trim()) break;

    const match = line.match(itemPattern);
    if (!match) break;

    const itemLines = [match[2]];
    index += 1;

    while (index < lines.length) {
      const continuation = lines[index];
      if (!continuation.trim()) break;
      if (itemPattern.test(continuation)) break;
      if (
        /^```/.test(continuation) ||
        /^(#{1,6})\s+/.test(continuation) ||
        /^([-*_])(?:\s*\1){2,}\s*$/.test(continuation) ||
        /^\s*>/.test(continuation)
      ) {
        break;
      }
      itemLines.push(continuation.trim());
      index += 1;
    }

    items.push(`<li>${renderInlineMarkdown(itemLines.join('\n'))}</li>`);
  }

  if (items.length === 0) return null;
  const tag = ordered ? 'ol' : 'ul';
  return {
    html: `<${tag}>${items.join('')}</${tag}>`,
    nextIndex: index,
  };
}

function parseMarkdownTable(lines, startIndex) {
  const headerLine = lines[startIndex];
  const dividerLine = lines[startIndex + 1];
  if (!headerLine || !dividerLine) return null;
  if (!/\|/.test(headerLine)) return null;
  if (!/^\s*\|?(?:\s*:?-{3,}:?\s*\|)+\s*:?-{3,}:?\s*\|?\s*$/.test(dividerLine)) return null;

  const parseRow = (line) =>
    line
      .trim()
      .replace(/^\|/, '')
      .replace(/\|$/, '')
      .split('|')
      .map((cell) => cell.trim());

  const headers = parseRow(headerLine);
  if (headers.length === 0) return null;

  const bodyRows = [];
  let index = startIndex + 2;
  while (index < lines.length && /\|/.test(lines[index]) && lines[index].trim()) {
    bodyRows.push(parseRow(lines[index]));
    index += 1;
  }

  const headHTML = `<tr>${headers.map((cell) => `<th>${renderInlineMarkdown(cell)}</th>`).join('')}</tr>`;
  const bodyHTML = bodyRows
    .map((row) => `<tr>${row.map((cell) => `<td>${renderInlineMarkdown(cell)}</td>`).join('')}</tr>`)
    .join('');

  return {
    html: `<table><thead>${headHTML}</thead><tbody>${bodyHTML}</tbody></table>`,
    nextIndex: index,
  };
}

function renderInlineMarkdown(source) {
  const tokens = [];
  const stash = (html) => {
    const key = `%%MDTOKEN${tokens.length}%%`;
    tokens.push(html);
    return key;
  };

  let text = String(source ?? '');
  text = text.replace(/`([^`\n]+)`/g, (_, code) => stash(`<code>${escapeHTML(code)}</code>`));
  text = text.replace(/\[([^\]]+)\]\(([^)\s]+)\)/g, (match, label, url) => {
    const safeURL = sanitizeURL(url);
    if (!safeURL) return match;
    return stash(
      `<a href="${escapeAttribute(safeURL)}" target="_blank" rel="noreferrer noopener">${escapeHTML(label)}</a>`,
    );
  });

  let html = escapeHTML(text);
  html = html.replace(/\*\*([^*\n]+)\*\*/g, '<strong>$1</strong>');
  html = html.replace(/__([^_\n]+)__/g, '<strong>$1</strong>');
  html = html.replace(/\*([^*\n]+)\*/g, '<em>$1</em>');
  html = html.replace(/_([^_\n]+)_/g, '<em>$1</em>');
  html = html.replace(/~~([^~\n]+)~~/g, '<del>$1</del>');
  html = html.replace(/\n/g, '<br>');

  return html.replace(/%%MDTOKEN(\d+)%%/g, (_, tokenIndex) => tokens[Number(tokenIndex)] || '');
}

function sanitizeURL(value) {
  const url = String(value ?? '').trim();
  if (!url) return '';

  try {
    const parsed = new URL(url, window.location.href);
    const protocol = parsed.protocol.toLowerCase();
    if (protocol === 'http:' || protocol === 'https:' || protocol === 'mailto:') {
      return parsed.href;
    }
  } catch (error) {
    return '';
  }

  return '';
}

function stripFrontmatter(source) {
  const text = String(source ?? '').replace(/\r\n?/g, '\n').trim();
  if (!text.startsWith('---\n')) return text;

  const boundary = text.indexOf('\n---\n', 4);
  if (boundary === -1) return text;
  return text.slice(boundary + 5).trim();
}

function escapeHTML(value) {
  return String(value ?? '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function escapeAttribute(value) {
  return escapeHTML(value).replaceAll('`', '&#96;');
}
