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
  chatState: defaultChatState(),
  reminders: [],
  knowledge: [],
  prompts: [],
  skills: [],
  chatPrompt: defaultChatPromptState(),
  autocomplete: defaultAutocompleteState(),
  selectedSkillName: "",
  filter: "",
  promptFilter: "",
  filePath: "",
  fileObject: null,
  appendDrafts: {},
  openAppendId: "",
  model: defaultModelState(),
  weixin: defaultWeixinState(),
  settings: defaultSettingsState(),
  chat: [],
  chatStreaming: false,
  chatStreamHandlers: {},
};

let devPollTimer = 0;

const promptExamples = [
  "记住：Windows 版先把桌面前端做稳",
  "/debug-search macOS 什么时候做？",
  "两小时后提醒我喝水",
  "现在我记了什么？",
];

const CHAT_SLASH_COMMANDS = [
  { label: '/help', insert: '/help', description: '查看可用命令' },
  { label: '/new', insert: '/new', description: '开启一个新的对话' },
  { label: '/remember', insert: '/remember ', description: '保存一条知识' },
  { label: '/remember-file', insert: '/remember-file ', description: '总结图片或 PDF 并写入知识库' },
  { label: '/append', insert: '/append ', description: '追加到已有知识' },
  { label: '/skills', insert: '/skills', description: '查看可用技能和当前会话已加载技能' },
  { label: '/show-skill', insert: '/show-skill ', description: '查看某个技能内容' },
  { label: '/load-skill', insert: '/load-skill ', description: '为当前会话加载一个技能' },
  { label: '/unload-skill', insert: '/unload-skill ', description: '从当前会话卸载一个技能' },
  { label: '/page-skills', insert: '/page-skills', description: '查看当前会话已加载技能' },
  { label: '/prompt', insert: '/prompt', description: '查看当前 Prompt profile' },
  { label: '/prompt list', insert: '/prompt list', description: '查看可用 Prompt profiles' },
  { label: '/prompt use', insert: '/prompt use ', description: '为当前会话启用 Prompt profile' },
  { label: '/prompt clear', insert: '/prompt clear', description: '清除当前会话 Prompt profile' },
  { label: '/translate', insert: '/translate ', description: '翻译成中文' },
  { label: '/debug-search', insert: '/debug-search ', description: '查看关键词检索和候选复核过程' },
  { label: '/mode', insert: '/mode', description: '查看当前对话模式' },
  { label: '/mode direct', insert: '/mode direct', description: '切换到 direct 模式' },
  { label: '/mode knowledge', insert: '/mode knowledge', description: '切换到 knowledge 模式' },
  { label: '/mode agent', insert: '/mode agent', description: '切换到 agent 模式' },
  { label: '/forget', insert: '/forget ', description: '删除一条知识' },
  { label: '/list', insert: '/list', description: '查看全部知识' },
  { label: '/stats', insert: '/stats', description: '查看知识库状态' },
  { label: '/notice', insert: '/notice ', description: '创建提醒' },
  { label: '/notice list', insert: '/notice list', description: '查看提醒列表' },
  { label: '/notice remove', insert: '/notice remove ', description: '删除提醒' },
  { label: '/cron', insert: '/cron ', description: '与 /notice 等价' },
  { label: '/clear', insert: '/clear', description: '清空知识库' },
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
  renderChatContext();
  renderChatAutocomplete();
  renderProjectState();
  renderChatSessions();
  renderChat();
  renderKnowledge();
  renderReminders();
  renderPrompts();
  renderSkills();
  renderModel();
  renderWeixin();
  renderSettings();

  try {
    state.backend = await waitForBackend();
    state.backendMode = state.backend.mode || 'wails';
    bindRuntimeEvents();
    startBackendPolling();
    await refreshAll();
    await refreshChatState();
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
  const openButtons = document.querySelectorAll('[data-open-quick-memory]');
  const cancelBtn = document.getElementById('quick-add-cancel');
  const confirmBtn = document.getElementById('quick-add-confirm');
  const input = document.getElementById('quick-memory-input');

  openButtons.forEach((button) => {
    button.addEventListener('click', () => {
      modal.style.display = 'flex';
      input.focus();
    });
  });

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

  const reminderRefresh = document.getElementById('reminder-refresh');
  if (reminderRefresh) {
    reminderRefresh.addEventListener('click', () => void refreshReminders());
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

  const settingsSave = document.getElementById('settings-save');
  if (settingsSave) {
    settingsSave.addEventListener('click', () => void saveSettings());
  }

  const skillImportTrigger = document.getElementById('skill-import-trigger');
  if (skillImportTrigger) {
    skillImportTrigger.addEventListener('click', () => void browseSkillArchive());
  }

  const skillZipInput = document.getElementById('http-skill-zip-input');
  if (skillZipInput) {
    skillZipInput.addEventListener('change', (event) => {
      const file = event.target.files && event.target.files[0];
      if (file) {
        void importSkillArchiveFromFile(file);
      }
    });
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

  const chatNewSession = document.getElementById('chat-new-session');
  if (chatNewSession) {
    chatNewSession.addEventListener('click', () => void startNewConversation());
  }

  const chatExportMarkdown = document.getElementById('chat-export-markdown');
  if (chatExportMarkdown) {
    chatExportMarkdown.addEventListener('click', () => void exportChatMarkdown());
  }

  const chatSessionList = document.getElementById('chat-session-list');
  if (chatSessionList) {
    chatSessionList.addEventListener('click', (event) => {
      const target = event.target.closest('[data-chat-session]');
      if (!target) return;
      const sessionId = target.dataset.chatSession || '';
      if (!sessionId) return;
      void switchChatSession(sessionId);
    });
  }

  const chatList = document.getElementById('chat-list');
  if (chatList) {
    chatList.addEventListener('click', (event) => {
      const target = event.target.closest('[data-chat-option]');
      if (!target) return;
      const value = target.dataset.chatOptionValue || target.dataset.chatOption || '';
      const label = target.dataset.chatOptionLabel || value;
      const question = target.dataset.chatOptionQuestion || '';
      if (!value || !question) return;
      void sendChatOption(question, value, label);
    });
  }

  const chatInput = document.getElementById('chat-input');
  if (chatInput) {
    autoResizeChatInput();
    chatInput.addEventListener('keydown', (e) => {
      if (state.autocomplete.open) {
        if (e.key === 'ArrowDown') {
          e.preventDefault();
          moveAutocompleteSelection(1);
          return;
        }
        if (e.key === 'ArrowUp') {
          e.preventDefault();
          moveAutocompleteSelection(-1);
          return;
        }
        if (e.key === 'Enter' || e.key === 'Tab') {
          const selected = (state.autocomplete.items || [])[state.autocomplete.selectedIndex];
          if (selected && !selected.disabled) {
            e.preventDefault();
            void applySelectedAutocompleteItem();
            return;
          }
          if (e.key === 'Tab') {
            e.preventDefault();
            closeChatAutocomplete();
            return;
          }
        }
        if (e.key === 'Escape') {
          e.preventDefault();
          closeChatAutocomplete();
          return;
        }
      }

      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        void sendMessage();
      }
    });

    chatInput.addEventListener('input', () => {
      autoResizeChatInput();
      updateChatAutocomplete();
    });
    chatInput.addEventListener('click', updateChatAutocomplete);
    chatInput.addEventListener('focus', updateChatAutocomplete);
  }

  const chatContextBar = document.getElementById('chat-context-bar');
  if (chatContextBar) {
    chatContextBar.addEventListener('click', (event) => {
      const target = event.target.closest('[data-chat-context-action]');
      if (!target) return;

      const action = target.dataset.chatContextAction || '';
      const value = target.dataset.value || '';
      if (action === 'clear-prompt') {
        void clearChatPromptSelection();
      } else if (action === 'unload-skill') {
        void unloadSkill(value);
      }
    });
  }

  const chatAutocomplete = document.getElementById('chat-autocomplete');
  if (chatAutocomplete) {
    chatAutocomplete.addEventListener('mousedown', (event) => {
      event.preventDefault();
    });
    chatAutocomplete.addEventListener('click', (event) => {
      const target = event.target.closest('[data-autocomplete-index]');
      if (!target) return;
      const index = Number(target.dataset.autocompleteIndex || '-1');
      if (Number.isNaN(index) || index < 0) return;
      state.autocomplete.selectedIndex = index;
      renderChatAutocomplete();
      void applySelectedAutocompleteItem();
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

  const modelNew = document.getElementById('model-new');
  if (modelNew) {
    modelNew.addEventListener('click', () => createNewModelProfileDraft());
  }

  const modelSetActive = document.getElementById('model-set-active');
  if (modelSetActive) {
    modelSetActive.addEventListener('click', () => void setActiveModelProfile());
  }

  const modelTest = document.getElementById('model-test');
  if (modelTest) {
    modelTest.addEventListener('click', () => void testModelConnection());
  }

  const modelDelete = document.getElementById('model-delete');
  if (modelDelete) {
    modelDelete.addEventListener('click', () => void deleteModelProfile());
  }

  const modelProfileSelect = document.getElementById('model-profile-select');
  if (modelProfileSelect) {
    modelProfileSelect.addEventListener('change', () => syncModelFormFromSelection(true));
  }

  const modelProvider = document.getElementById('model-provider');
  if (modelProvider) {
    modelProvider.addEventListener('change', () => syncModelProviderFields(true));
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

  document.addEventListener('click', (event) => {
    if (event.target.closest('.chat-input-area')) return;
    closeChatAutocomplete();
  });
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
    syncCurrentChatConversationFromMessages();
    renderChat();
    showBanner(`提醒 #${shortId}: ${message}`, false);
    void refreshReminders().catch(() => {});
  });

  window.runtime.EventsOn('weixin:status', (payload) => {
    const next = normalizeWeixinStatus(payload);
    applyWeixinStatus(next, true);
  });

  window.runtime.EventsOn('chat:stream', (payload) => {
    const event = Array.isArray(payload) ? payload[0] : payload;
    dispatchChatStreamEvent(event);
  });
}

function dispatchChatStreamEvent(event) {
  if (!event || !event.requestId) return;
  const handler = state.chatStreamHandlers[event.requestId];
  if (typeof handler === 'function') {
    handler(event);
  }
}

function renderChatShortcuts() {
  document.querySelectorAll('.shortcut-chip').forEach(chip => {
    chip.addEventListener('click', () => {
      const input = document.getElementById('chat-input');
      if (input) {
        input.value = chip.dataset.cmd || '';
        const cursor = input.value.length;
        input.setSelectionRange(cursor, cursor);
        autoResizeChatInput();
        updateChatAutocomplete();
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
    ListReminders: () => backend.ListReminders(),
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
    OpenSkillImportDialog: () => backend.OpenSkillImportDialog(),
    ImportSkillArchive: (path) => backend.ImportSkillArchive(path),
    UploadSkillArchive: () => Promise.reject(new Error('Wails 模式不使用浏览器上传。')),
    ConfirmAction: (title, message) => backend.ConfirmAction(title, message),
    OpenImportDialog: () => backend.OpenImportDialog(),
    ImportFile: (path) => backend.ImportFile(path),
    UploadImportFile: () => Promise.reject(new Error('Wails 模式不使用浏览器上传。')),
    SendMessage: (input) => backend.SendMessage(input),
    SendMessageStream: async (input, handlers = {}) => {
      if (typeof backend.SendMessageStream !== 'function' || !window.runtime || typeof window.runtime.EventsOn !== 'function') {
        const result = await backend.SendMessage(input);
        if (typeof handlers.onDelta === 'function' && result?.reply) {
          handlers.onDelta(result.reply);
        }
        return result;
      }

      const requestId = newChatStreamRequestID();
      state.chatStreamHandlers[requestId] = (event) => {
        if ((event?.delta || '') && typeof handlers.onDelta === 'function') {
          handlers.onDelta(event.delta);
        }
      };
      try {
        return await backend.SendMessageStream(requestId, input);
      } finally {
        delete state.chatStreamHandlers[requestId];
      }
    },
    GetChatState: () => backend.GetChatState(),
    ExportChatMarkdown: () => backend.ExportChatMarkdown(),
    NewChatSession: () => backend.NewChatSession(),
    SwitchChatSession: (sessionId) => backend.SwitchChatSession(sessionId),
    GetChatPrompt: () => backend.GetChatPrompt(),
    SetChatPrompt: (idOrPrefix) => backend.SetChatPrompt(idOrPrefix),
    ClearChatPrompt: () => backend.ClearChatPrompt(),
    GetModelSettings: () => backend.GetModelSettings(),
    SaveModelConfig: (payload) => backend.SaveModelConfig(payload),
    TestModelConnection: (id) => backend.TestModelConnection(id),
    DeleteModelConfig: (id) => backend.DeleteModelConfig(id),
    SetActiveModel: (id) => backend.SetActiveModel(id),
    GetWeixinStatus: () => backend.GetWeixinStatus(),
    StartWeixinLogin: () => backend.StartWeixinLogin(),
    CancelWeixinLogin: () => backend.CancelWeixinLogin(),
    LogoutWeixin: () => backend.LogoutWeixin(),
    GetSettings: () => backend.GetSettings(),
    SaveSettings: (payload) => backend.SaveSettings(payload),
  };
}

function createHTTPBackend() {
  return {
    mode: 'http',
    GetOverview: () => requestJSON('GET', '/api/overview'),
    GetProjectState: () => requestJSON('GET', '/api/projects'),
    SetActiveProject: (name) => requestJSON('POST', '/api/projects/active', { name }),
    ListReminders: () => requestJSON('GET', '/api/reminders'),
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
    OpenSkillImportDialog: async () => '',
    ImportSkillArchive: async () => {
      throw new Error('HTTP 模式请直接选择 zip 文件上传。');
    },
    UploadSkillArchive: (file) => uploadFile('/api/skills/upload', file),
    ConfirmAction: async (title, message) => window.confirm(`${title}\n\n${message}`),
    OpenImportDialog: async () => '',
    ImportFile: async () => {
      throw new Error('HTTP 模式请直接选择本地文件上传。');
    },
    UploadImportFile: (file) => uploadFile('/api/import/upload', file),
    SendMessage: (input) => requestJSON('POST', '/api/chat', { input }),
    SendMessageStream: (input, handlers = {}) => streamJSON('POST', '/api/chat/stream', { input }, handlers),
    GetChatState: () => requestJSON('GET', '/api/chat/state'),
    ExportChatMarkdown: async () => {
      const payload = await requestJSON('GET', '/api/chat/export-markdown');
      downloadTextFile(payload.filename || 'myclaw-chat.md', payload.markdown || '', 'text/markdown;charset=utf-8');
      return { message: `已导出 Markdown：${payload.filename || 'myclaw-chat.md'}` };
    },
    NewChatSession: () => requestJSON('POST', '/api/chat/session/new'),
    SwitchChatSession: (sessionId) => requestJSON('POST', '/api/chat/session/switch', { sessionId }),
    GetChatPrompt: () => requestJSON('GET', '/api/chat/prompt'),
    SetChatPrompt: (idOrPrefix) => requestJSON('POST', '/api/chat/prompt', { idOrPrefix }),
    ClearChatPrompt: () => requestJSON('DELETE', '/api/chat/prompt'),
    GetModelSettings: () => requestJSON('GET', '/api/model'),
    SaveModelConfig: (payload) => requestJSON('POST', '/api/model/save', payload),
    TestModelConnection: (id) => requestJSON('POST', '/api/model/test', { id }),
    DeleteModelConfig: (id) => requestJSON('POST', '/api/model/delete', { id }),
    SetActiveModel: (id) => requestJSON('POST', '/api/model/active', { id }),
    GetWeixinStatus: () => requestJSON('GET', '/api/weixin/status'),
    StartWeixinLogin: () => requestJSON('POST', '/api/weixin/login'),
    CancelWeixinLogin: () => requestJSON('POST', '/api/weixin/cancel'),
    LogoutWeixin: () => requestJSON('POST', '/api/weixin/logout'),
    GetSettings: () => requestJSON('GET', '/api/settings'),
    SaveSettings: (payload) => requestJSON('POST', '/api/settings', payload),
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

async function streamJSON(method, url, body, handlers = {}) {
  const options = { method, headers: {} };
  if (body !== undefined) {
    options.headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(body);
  }

  const response = await fetch(url, options);
  if (!response.ok) {
    const text = await response.text();
    const payload = text ? JSON.parse(text) : null;
    throw new Error((payload && payload.error) || `HTTP ${response.status}`);
  }
  if (!response.body) {
    throw new Error('浏览器不支持流式响应。');
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let result = null;

  while (true) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value || new Uint8Array(), { stream: !done });

    let newlineIndex = buffer.indexOf('\n');
    while (newlineIndex >= 0) {
      const line = buffer.slice(0, newlineIndex).trim();
      buffer = buffer.slice(newlineIndex + 1);
      if (line) {
        const event = JSON.parse(line);
        if (event.type === 'delta') {
          if (typeof handlers.onDelta === 'function' && event.delta) {
            handlers.onDelta(event.delta);
          }
        } else if (event.type === 'done') {
          result = event;
        } else if (event.type === 'error') {
          throw new Error(event.message || '流式请求失败');
        }
      }
      newlineIndex = buffer.indexOf('\n');
    }

    if (done) break;
  }

  const tail = buffer.trim();
  if (tail) {
    const event = JSON.parse(tail);
    if (event.type === 'done') {
      result = event;
    } else if (event.type === 'error') {
      throw new Error(event.message || '流式请求失败');
    } else if (event.type === 'delta' && typeof handlers.onDelta === 'function' && event.delta) {
      handlers.onDelta(event.delta);
    }
  }

  if (!result) {
    throw new Error('流式响应提前结束。');
  }
  return result;
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

function downloadTextFile(filename, content, type = 'text/plain;charset=utf-8') {
  const blob = new Blob([String(content ?? '')], { type });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = String(filename || 'download.txt').trim() || 'download.txt';
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function startBackendPolling() {
  window.clearInterval(devPollTimer);
  if (state.backendMode !== 'http') return;

  devPollTimer = window.setInterval(() => {
    void Promise.all([refreshProjectState(), refreshOverview(), refreshReminders(), refreshModel(), refreshWeixin()]).catch(() => {});
  }, 2000);
}

async function refreshAll() {
  await Promise.all([refreshProjectState(), refreshOverview(), refreshReminders(), refreshKnowledge(), refreshPrompts(), refreshSkills(), refreshChatPrompt(), refreshModel(), refreshWeixin(), refreshSettings()]);
}

async function refreshProjectState() {
  state.projectState = normalizeProjectState(await state.backend.GetProjectState());
  renderProjectState();
}

async function refreshChatState() {
  applyChatState(normalizeChatState(await state.backend.GetChatState()));
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

async function refreshReminders() {
  state.reminders = normalizeReminders(await state.backend.ListReminders());
  renderReminders();
}

async function refreshKnowledge() {
  state.knowledge = await state.backend.ListKnowledge();
  renderKnowledge();
}

async function refreshPrompts() {
  state.prompts = await state.backend.ListPrompts();
  renderPrompts();
  updateChatAutocomplete();
}

async function refreshSkills() {
  state.skills = normalizeSkills(await state.backend.ListSkills());
  ensureSelectedSkill();
  renderSkills();
  renderChatContext();
  updateChatAutocomplete();
}

async function refreshChatPrompt() {
  state.chatPrompt = normalizeChatPromptState(await state.backend.GetChatPrompt());
  renderChatContext();
  updateChatAutocomplete();
}

async function refreshModel() {
  state.model = normalizeModelSettings(await state.backend.GetModelSettings());
  renderModel();
}

async function refreshWeixin() {
  const next = await state.backend.GetWeixinStatus();
  applyWeixinStatus(next, false);
}

async function refreshSettings() {
  state.settings = normalizeSettingsState(await state.backend.GetSettings());
  renderSettings();
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

async function sendMessage(rawText = null, displayText = null) {
  if (state.chatStreaming) return;

  const input = document.getElementById('chat-input');
  const text = String(rawText ?? input?.value ?? '').trim();
  if (!text) return;
  const visibleText = String(displayText ?? text).trim() || text;
  if (text === '/new') {
    closeChatAutocomplete();
    if (input && rawText == null) {
      input.value = '';
      autoResizeChatInput();
    }
    await startNewConversation();
    return;
  }

  state.chat.push({ role: 'user', text: visibleText, time: nowLabel() });
  syncCurrentChatConversationFromMessages();
  renderChat();
  closeChatAutocomplete();
  if (input && rawText == null) {
    input.value = '';
    autoResizeChatInput();
  }

  const placeholder = {
    role: 'assistant',
    text: '',
    time: '',
    streaming: true,
  };
  state.chat.push(placeholder);
  renderChat();

  state.chatStreaming = true;
  try {
    const send = typeof state.backend.SendMessageStream === 'function'
      ? state.backend.SendMessageStream(text, {
          onDelta: (delta) => {
            if (!delta) return;
            placeholder.text += delta;
            syncCurrentChatConversationFromMessages();
            renderChat();
          },
        })
      : state.backend.SendMessage(text);
    const result = await send;
    if (result.sessionChanged) {
      state.chat.pop();
      await refreshChatState();
      await Promise.all([refreshSkills(), refreshChatPrompt()]);
      showBanner(result.reply || '已开启新对话。', false);
      return;
    }
    placeholder.text = result.reply || placeholder.text;
    placeholder.time = result.timestamp || nowLabel();
    placeholder.streaming = false;
    syncCurrentChatConversationFromMessages();
    renderChat();
    await refreshAll();
  } catch (error) {
    if ((placeholder.text || '').trim()) {
      placeholder.time = nowLabel();
      placeholder.streaming = false;
      state.chat.push({
        role: 'system',
        text: asMessage(error),
        time: nowLabel(),
      });
    } else {
      placeholder.role = 'system';
      placeholder.text = asMessage(error);
      placeholder.time = nowLabel();
      placeholder.streaming = false;
    }
    syncCurrentChatConversationFromMessages();
    renderChat();
    showBanner(asMessage(error), true);
  } finally {
    state.chatStreaming = false;
  }
}

async function startNewConversation() {
  if (state.chatStreaming) {
    showBanner('当前回复尚未完成。', true);
    return;
  }
  try {
    applyChatState(normalizeChatState(await state.backend.NewChatSession()));
    await Promise.all([refreshSkills(), refreshChatPrompt()]);
    const input = document.getElementById('chat-input');
    if (input) input.focus();
    showBanner('已开启新对话。', false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function exportChatMarkdown() {
  if (state.chatStreaming) {
    showBanner('当前回复尚未完成。', true);
    return;
  }
  try {
    const result = await state.backend.ExportChatMarkdown();
    if (result?.message) {
      showBanner(result.message, false);
    }
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function sendChatOption(question, value, label = value) {
  if (state.chatStreaming) {
    showBanner('当前回复尚未完成。', true);
    return;
  }
  await sendMessage(buildChatOptionSubmission(question, value, label), label);
}

async function switchChatSession(sessionId) {
  if (state.chatStreaming) {
    showBanner('当前回复尚未完成。', true);
    return;
  }
  const nextSessionId = (sessionId || '').trim();
  if (!nextSessionId || nextSessionId === state.chatState.sessionId) return;

  try {
    applyChatState(normalizeChatState(await state.backend.SwitchChatSession(nextSessionId)));
    await Promise.all([refreshSkills(), refreshChatPrompt()]);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

function renderChatContext() {
  const container = document.getElementById('chat-context-bar');
  if (!container) return;

  const chips = [];
  if (state.chatPrompt.promptId) {
    chips.push(`
      <span class="chat-context-chip prompt">
        <span>Prompt</span>
        <strong>${escapeHTML(state.chatPrompt.title || `#${state.chatPrompt.shortId}`)}</strong>
        <button type="button" data-chat-context-action="clear-prompt" title="清除当前 Prompt">×</button>
      </span>
    `);
  }

  const loadedSkills = [...state.skills]
    .filter((item) => item.loaded)
    .sort((left, right) => left.name.localeCompare(right.name, 'zh-CN'));
  for (const skill of loadedSkills) {
    chips.push(`
      <span class="chat-context-chip skill">
        <span>Skill</span>
        <strong>${escapeHTML(skill.name)}</strong>
        <button type="button" data-chat-context-action="unload-skill" data-value="${escapeAttribute(skill.name)}" title="卸载技能">×</button>
      </span>
    `);
  }

  container.innerHTML = chips.join('');
}

async function clearChatPromptSelection() {
  try {
    await state.backend.ClearChatPrompt();
    state.chatPrompt = defaultChatPromptState();
    renderChatContext();
    updateChatAutocomplete();
    showBanner('已清除当前对话 Prompt。', false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

function autoResizeChatInput() {
  const input = document.getElementById('chat-input');
  if (!(input instanceof HTMLTextAreaElement)) return;
  input.style.height = 'auto';
  input.style.height = `${Math.min(input.scrollHeight, 120)}px`;
}

function updateChatAutocomplete() {
  const input = document.getElementById('chat-input');
  if (!(input instanceof HTMLTextAreaElement)) {
    closeChatAutocomplete();
    return;
  }

  const active = getActiveChatTrigger(input);
  if (!active) {
    closeChatAutocomplete();
    return;
  }

  const items = buildAutocompleteItems(active.trigger, active.query);
  state.autocomplete = {
    open: true,
    trigger: active.trigger,
    query: active.query,
    tokenStart: active.start,
    tokenEnd: active.end,
    selectedIndex: firstSelectableAutocompleteIndex(items),
    items,
  };
  renderChatAutocomplete();
}

function getActiveChatTrigger(input) {
  const value = input.value || '';
  const caret = input.selectionStart ?? value.length;
  let start = caret;
  while (start > 0 && !/\s/.test(value[start - 1])) {
    start -= 1;
  }

  if (start >= value.length) return null;
  const trigger = value[start];
  if (!['/', '$', '@'].includes(trigger)) return null;

  let end = caret;
  while (end < value.length && !/\s/.test(value[end])) {
    end += 1;
  }

  return {
    trigger,
    query: value.slice(start + 1, caret),
    start,
    end,
  };
}

function buildAutocompleteItems(trigger, query) {
  switch (trigger) {
    case '/':
      return buildCommandAutocompleteItems(query);
    case '$':
      return buildSkillAutocompleteItems(query);
    case '@':
      return buildPromptAutocompleteItems(query);
    default:
      return [];
  }
}

function buildCommandAutocompleteItems(query) {
  const filtered = CHAT_SLASH_COMMANDS.filter((item) =>
    autocompleteMatches(query, [item.label, item.insert, item.description]),
  );
  const items = filtered.map((item) => ({
    kind: 'command',
    title: item.label,
    description: item.description,
    meta: 'slash',
    insertText: item.insert,
    disabled: false,
  }));
  if (items.length > 0) return items;
  return [
    {
      kind: 'empty',
      title: '没有匹配的 slash command',
      description: '继续输入，或直接发送普通消息。',
      meta: '',
      disabled: true,
    },
  ];
}

function buildSkillAutocompleteItems(query) {
  const sorted = [...state.skills].sort((left, right) => {
    if (left.loaded !== right.loaded) return left.loaded ? -1 : 1;
    return left.name.localeCompare(right.name, 'zh-CN');
  });
  const filtered = sorted.filter((item) =>
    autocompleteMatches(query, [item.name, item.description, item.dir]),
  );
  if (filtered.length === 0) {
    return [
      {
        kind: 'empty',
        title: state.skills.length === 0 ? '当前没有可用 skill' : '没有匹配的 skill',
        description: state.skills.length === 0 ? '先在 Skill 库导入或添加本地技能。' : '尝试按技能名或描述搜索。',
        meta: '',
        disabled: true,
      },
    ];
  }

  return filtered.map((item) => ({
    kind: 'skill',
    title: `$${item.name}`,
    description: item.description || '加载到当前对话会话',
    meta: item.loaded ? '已加载' : '可加载',
    name: item.name,
    disabled: false,
  }));
}

function buildPromptAutocompleteItems(query) {
  const filtered = state.prompts.filter((item) =>
    autocompleteMatches(query, [item.title, item.shortId, item.content]),
  );
  const items = filtered.map((item) => ({
    kind: 'prompt',
    title: `@${item.title}`,
    description: preview(item.content, 80),
    meta: state.chatPrompt.promptId === item.id ? `当前 · #${item.shortId}` : `Prompt · #${item.shortId}`,
    promptId: item.id,
    shortId: item.shortId,
    disabled: false,
  }));

  if (state.chatPrompt.promptId) {
    items.unshift({
      kind: 'prompt-clear',
      title: '@清除当前 Prompt',
      description: `当前使用：${state.chatPrompt.title || `#${state.chatPrompt.shortId}`}`,
      meta: 'Prompt',
      disabled: false,
    });
  }

  if (items.length > 0) return items;
  return [
    {
      kind: 'empty',
      title: state.prompts.length === 0 ? '当前没有可用 Prompt' : '没有匹配的 Prompt',
      description: state.prompts.length === 0 ? '先在 Prompt 库保存常用模板。' : '尝试按标题、ID 或内容搜索。',
      meta: '',
      disabled: true,
    },
  ];
}

function autocompleteMatches(query, values) {
  const normalized = String(query || '').trim().toLowerCase();
  if (!normalized) return true;
  return values.some((value) => String(value || '').toLowerCase().includes(normalized));
}

function firstSelectableAutocompleteIndex(items) {
  const index = items.findIndex((item) => !item.disabled);
  return index >= 0 ? index : 0;
}

function moveAutocompleteSelection(direction) {
  const items = state.autocomplete.items || [];
  if (!state.autocomplete.open || items.length === 0) return;

  let nextIndex = state.autocomplete.selectedIndex;
  for (let step = 0; step < items.length; step += 1) {
    nextIndex = (nextIndex + direction + items.length) % items.length;
    if (!items[nextIndex]?.disabled) {
      state.autocomplete.selectedIndex = nextIndex;
      renderChatAutocomplete();
      return;
    }
  }
}

function renderChatAutocomplete() {
  const container = document.getElementById('chat-autocomplete');
  if (!container) return;

  if (!state.autocomplete.open || (state.autocomplete.items || []).length === 0) {
    container.hidden = true;
    container.innerHTML = '';
    return;
  }

  container.hidden = false;
  container.innerHTML = `
    <div class="chat-autocomplete-list">
      ${(state.autocomplete.items || [])
        .map((item, index) => `
          <button
            type="button"
            class="chat-autocomplete-item ${index === state.autocomplete.selectedIndex ? 'active' : ''} ${item.disabled ? 'disabled' : ''}"
            data-autocomplete-index="${index}"
            ${item.disabled ? 'disabled' : ''}
          >
            <div class="chat-autocomplete-head">
              <span class="chat-autocomplete-title">${escapeHTML(item.title || '')}</span>
              ${item.meta ? `<span class="chat-autocomplete-meta">${escapeHTML(item.meta)}</span>` : ''}
            </div>
            <div class="chat-autocomplete-desc">${escapeHTML(item.description || '')}</div>
          </button>
        `)
        .join('')}
    </div>
  `;
}

function closeChatAutocomplete() {
  state.autocomplete = defaultAutocompleteState();
  renderChatAutocomplete();
}

async function applySelectedAutocompleteItem() {
  const items = state.autocomplete.items || [];
  const item = items[state.autocomplete.selectedIndex];
  if (!item || item.disabled) return;
  await applyAutocompleteItem(item);
}

async function applyAutocompleteItem(item) {
  switch (item.kind) {
    case 'command':
      replaceCurrentChatToken(item.insertText || '');
      closeChatAutocomplete();
      break;
    case 'skill':
      replaceCurrentChatToken('');
      closeChatAutocomplete();
      await loadSkill(item.name || '');
      focusChatInput();
      break;
    case 'prompt':
      replaceCurrentChatToken('');
      closeChatAutocomplete();
      try {
        state.chatPrompt = normalizeChatPromptState(await state.backend.SetChatPrompt(item.promptId || ''));
        renderChatContext();
        showBanner(`已为当前对话启用 Prompt ${item.title.replace(/^@/, '')}。`, false);
      } catch (error) {
        showBanner(asMessage(error), true);
      }
      focusChatInput();
      break;
    case 'prompt-clear':
      replaceCurrentChatToken('');
      closeChatAutocomplete();
      await clearChatPromptSelection();
      focusChatInput();
      break;
    default:
      break;
  }
}

function replaceCurrentChatToken(replacement) {
  const input = document.getElementById('chat-input');
  if (!(input instanceof HTMLTextAreaElement)) return;

  const value = input.value || '';
  let start = state.autocomplete.tokenStart;
  let end = state.autocomplete.tokenEnd;
  if (start < 0 || end < start) return;

  if (!replacement) {
    if (start === 0) {
      while (end < value.length && /\s/.test(value[end])) {
        end += 1;
      }
    } else if (/\s/.test(value[start - 1] || '') && /\s/.test(value[end] || '')) {
      while (end < value.length && /\s/.test(value[end])) {
        end += 1;
      }
    }
  }

  input.value = value.slice(0, start) + replacement + value.slice(end);
  const cursor = start + replacement.length;
  input.setSelectionRange(cursor, cursor);
  autoResizeChatInput();
}

function focusChatInput() {
  const input = document.getElementById('chat-input');
  if (!(input instanceof HTMLTextAreaElement)) return;
  input.focus();
  updateChatAutocomplete();
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
        <h3>${state.filter ? '没有找到匹配的记忆' : `记忆库项目 ${escapeHTML(activeProject)} 为空`}</h3>
        <p>${state.filter ? '尝试其他关键词' : '切换记忆库项目、导入文件或直接添加记忆来开始使用'}</p>
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

function renderReminders() {
  const container = document.getElementById('reminder-list');
  const count = document.getElementById('reminder-count');
  if (!container) return;

  const reminders = [...state.reminders].sort((left, right) => {
    if (left.nextRunAtUnix !== right.nextRunAtUnix) {
      return left.nextRunAtUnix - right.nextRunAtUnix;
    }
    return left.createdAtUnix - right.createdAtUnix;
  });

  if (count) {
    count.textContent = `${reminders.length} 个任务`;
  }

  if (reminders.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">◷</div>
        <h3>当前没有提醒任务</h3>
        <p>在对话里创建提醒后，这里会显示你的定时列表。</p>
      </div>
    `;
    return;
  }

  container.innerHTML = reminders
    .map((item) => `
      <article class="memory-card reminder-card">
        <div class="memory-card-header">
          <div>
            <div class="memory-meta">
              <span class="memory-badge id">#${escapeHTML(item.shortId)}</span>
              <span class="memory-badge source">${escapeHTML(item.frequencyLabel)}</span>
              <span class="memory-badge source">${escapeHTML(item.scheduleLabel)}</span>
            </div>
            <h3 class="reminder-card-title">${escapeHTML(item.message)}</h3>
          </div>
          <div class="reminder-card-side">
            <div class="reminder-card-label">下次执行</div>
            <div class="reminder-card-next">${escapeHTML(item.nextRunAt || '—')}</div>
            <div class="reminder-card-relative">${escapeHTML(relativeTimeLabel(item.nextRunAtUnix))}</div>
          </div>
        </div>
        <div class="reminder-card-footer">
          <span class="memory-date">创建于 ${escapeHTML(item.createdAt || '—')}</span>
          <span class="reminder-card-kind">${escapeHTML(item.frequency === 'daily' ? '每日循环' : '执行一次')}</span>
        </div>
      </article>
    `)
    .join('');
}

function renderProjectState() {
  const activeProject = state.projectState.activeProject || 'default';
  const activeSummary = (state.projectState.projects || []).find((item) => item.active) || {
    name: activeProject,
    knowledgeCount: 0,
  };

  const display = document.getElementById('project-name-display');
  const summary = document.getElementById('project-summary-display');
  const input = document.getElementById('project-name-input');
  const list = document.getElementById('project-list');

  if (display) display.textContent = activeProject;
  if (summary) summary.textContent = `${activeSummary.knowledgeCount || 0} 条记忆属于当前记忆库项目`;
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

function selectedModelProfileId() {
  return document.getElementById('model-profile-select')?.value || '';
}

function selectedModelProfile() {
  const selectedId = selectedModelProfileId();
  return (state.model.profiles || []).find((item) => item.id === selectedId) || null;
}

function modelProviderDefaults(provider) {
  return MODEL_PROVIDER_DEFAULTS[provider] || MODEL_PROVIDER_DEFAULTS.openai;
}

function syncModelProviderFields(forceBaseUrl) {
  const provider = document.getElementById('model-provider');
  const apiType = document.getElementById('model-api-type');
  const baseUrl = document.getElementById('model-base-url');
  if (!provider || !apiType || !baseUrl) return;

  const providerValue = provider.value || 'openai';
  const options = MODEL_API_TYPE_OPTIONS[providerValue] || MODEL_API_TYPE_OPTIONS.openai;
  const previousAPIType = apiType.value;
  const previousBaseURL = baseUrl.value.trim();

  apiType.innerHTML = options
    .map((item) => `<option value="${escapeAttribute(item.value)}">${escapeHTML(item.label)}</option>`)
    .join('');

  if (options.some((item) => item.value === previousAPIType)) {
    apiType.value = previousAPIType;
  } else {
    apiType.value = options[0]?.value || '';
  }

  const defaults = modelProviderDefaults(providerValue);
  const knownDefaults = Object.values(MODEL_PROVIDER_DEFAULTS).map((item) => item.baseUrl);
  if (forceBaseUrl || !previousBaseURL || knownDefaults.includes(previousBaseURL)) {
    baseUrl.value = defaults.baseUrl;
  }
}

function populateModelForm(profile) {
  const name = document.getElementById('model-profile-name');
  const provider = document.getElementById('model-provider');
  const apiType = document.getElementById('model-api-type');
  const baseUrl = document.getElementById('model-base-url');
  const model = document.getElementById('model-name');
  const apiKey = document.getElementById('model-api-key');
  const apiKeyHint = document.getElementById('model-api-key-hint');

  const source = profile || {
    name: '',
    provider: 'openai',
    apiType: 'responses',
    baseUrl: MODEL_PROVIDER_DEFAULTS.openai.baseUrl,
    model: '',
    hasApiKey: false,
    apiKeyMasked: '(empty)',
  };

  if (name) name.value = source.name || '';
  if (provider) provider.value = source.provider || 'openai';
  syncModelProviderFields(!profile);
  if (apiType) apiType.value = source.apiType || modelProviderDefaults(source.provider || 'openai').apiType;
  if (baseUrl) baseUrl.value = source.baseUrl || modelProviderDefaults(source.provider || 'openai').baseUrl;
  if (model) model.value = source.model || '';
  if (apiKey) apiKey.value = '';

  const setOptional = (id, val) => {
    const el = document.getElementById(id);
    if (el) el.value = val != null ? val : '';
  };
  setOptional('model-max-output-tokens', source.maxOutputTokens);
  setOptional('model-temperature', source.temperature);
  setOptional('model-top-p', source.topP);
  setOptional('model-frequency-penalty', source.frequencyPenalty);
  setOptional('model-presence-penalty', source.presencePenalty);

  if (apiKeyHint) {
    apiKeyHint.textContent = profile && profile.hasApiKey
      ? `已保存 API Key：${profile.apiKeyMasked || '********'}。输入新值会覆盖；留空会保留原 key。`
      : '新建 profile 时请输入 API Key；编辑已保存 profile 时留空会保留原 key。';
  }
}

function syncModelFormFromSelection(fromUser) {
  const profile = selectedModelProfile();
  populateModelForm(profile);
  if (fromUser && profile) {
    showBanner(`已切换到 profile：${profile.name || profile.model || profile.id}`, false);
  }
}

function createNewModelProfileDraft() {
  const profileSelect = document.getElementById('model-profile-select');
  if (profileSelect) {
    profileSelect.value = '';
  }
  populateModelForm(null);
}

function readOptionalNumber(id) {
  const el = document.getElementById(id);
  if (!el) return null;
  const v = el.value.trim();
  if (v === '') return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

function readModelForm() {
  const selected = selectedModelProfile();
  const profileSelect = document.getElementById('model-profile-select');
  return {
    id: profileSelect?.value || '',
    name: document.getElementById('model-profile-name')?.value.trim() || '',
    provider: document.getElementById('model-provider')?.value.trim() || '',
    apiType: document.getElementById('model-api-type')?.value.trim() || '',
    baseUrl: document.getElementById('model-base-url')?.value.trim() || '',
    apiKey: document.getElementById('model-api-key')?.value.trim() || '',
    model: document.getElementById('model-name')?.value.trim() || '',
    maxOutputTokens: readOptionalNumber('model-max-output-tokens'),
    temperature: readOptionalNumber('model-temperature'),
    topP: readOptionalNumber('model-top-p'),
    frequencyPenalty: readOptionalNumber('model-frequency-penalty'),
    presencePenalty: readOptionalNumber('model-presence-penalty'),
    setActive: false,
    preserveApiKey: Boolean(selected?.hasApiKey) && !(document.getElementById('model-api-key')?.value.trim()),
  };
}

async function saveModelConfig() {
  try {
    const result = await state.backend.SaveModelConfig(readModelForm());
    state.model = normalizeModelSettings(result);
    renderModel();
    await refreshOverview();
    showBanner('模型 profile 已保存。', false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function setActiveModelProfile() {
  const selected = selectedModelProfile();
  if (!selected) {
    showBanner('请先选择已保存的 profile。', true);
    return;
  }

  try {
    state.model = normalizeModelSettings(await state.backend.SetActiveModel(selected.id));
    renderModel();
    await refreshOverview();
    showBanner(`已切换当前模型到 ${selected.name || selected.model || selected.id}。`, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function testModelConnection() {
  const selected = selectedModelProfile();
  if (!selected && (state.model.profiles || []).length > 0) {
    showBanner('请先选择要测试的 profile。', true);
    return;
  }

  try {
    const result = await state.backend.TestModelConnection(selected?.id || state.model.activeProfileId || '');
    await refreshAll();
    showBanner(result.message, false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function deleteModelProfile() {
  const selected = selectedModelProfile();
  if (!selected) {
    showBanner('请先选择要删除的 profile。', true);
    return;
  }

  try {
    const ok = await state.backend.ConfirmAction('删除模型 Profile', `确认删除 ${selected.name || selected.model || selected.id} 吗？`);
    if (!ok) return;

    state.model = normalizeModelSettings(await state.backend.DeleteModelConfig(selected.id));
    renderModel();
    await refreshOverview();
    showBanner('模型 profile 已删除。', false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

function renderModel() {
  const profileSelect = document.getElementById('model-profile-select');
  const pill = document.getElementById('model-status-pill');
  const message = document.getElementById('model-message');
  const effectiveProfileName = document.getElementById('effective-profile-name');
  const effectiveProvider = document.getElementById('effective-provider');
  const effectiveAPIType = document.getElementById('effective-api-type');
  const effectiveBaseUrl = document.getElementById('effective-base-url');
  const effectiveModel = document.getElementById('effective-model');
  const effectiveApiKey = document.getElementById('effective-api-key');

  const profiles = state.model.profiles || [];
  const previousSelectedId = profileSelect?.value || '';
  const nextSelectedId = profiles.some((item) => item.id === previousSelectedId)
    ? previousSelectedId
    : state.model.activeProfileId || '';

  if (profileSelect) {
    profileSelect.innerHTML = ['<option value="">新建 Profile</option>']
      .concat(
        profiles.map((item) => `
          <option value="${escapeAttribute(item.id)}">${escapeHTML(item.name || item.model || item.id)}${item.active ? ' · 当前' : ''}</option>
        `),
      )
      .join('');
    profileSelect.value = nextSelectedId;
  }

  if (pill) {
    pill.className = `status-pill ${state.model.configured ? 'on' : 'off'}`;
    pill.textContent = state.model.configured ? '已配置' : '未配置';
  }
  if (message) {
    const missing = (state.model.missingFields || []).length > 0
      ? ` 缺少：${state.model.missingFields.join('、')}。`
      : '';
    message.textContent = `${state.model.message || '尚未保存任何模型 profile。'}${missing}`;
  }

  if (effectiveProfileName) effectiveProfileName.textContent = state.model.effectiveProfileName || '—';
  if (effectiveProvider) effectiveProvider.textContent = state.model.effectiveProvider || '—';
  if (effectiveAPIType) effectiveAPIType.textContent = state.model.effectiveApiType || '—';
  if (effectiveBaseUrl) effectiveBaseUrl.textContent = state.model.effectiveBaseUrl || '—';
  if (effectiveModel) effectiveModel.textContent = state.model.effectiveModel || '—';
  if (effectiveApiKey) effectiveApiKey.textContent = state.model.effectiveApiKeyMasked || '—';

  populateModelForm((profiles || []).find((item) => item.id === nextSelectedId) || null);
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
    syncCurrentChatConversationFromMessages();
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

async function saveSettings() {
  try {
    const messagesValue = document.getElementById('settings-weixin-history-messages')?.value.trim() || '0';
    const runesValue = document.getElementById('settings-weixin-history-runes')?.value.trim() || '0';
    const payload = {
      weixinHistoryMessages: Number(messagesValue),
      weixinHistoryRunes: Number(runesValue),
    };

    if (!Number.isInteger(payload.weixinHistoryMessages) || payload.weixinHistoryMessages < 0) {
      throw new Error('最近消息条数必须是大于等于 0 的整数。');
    }
    if (!Number.isInteger(payload.weixinHistoryRunes) || payload.weixinHistoryRunes < 0) {
      throw new Error('单条消息字符上限必须是大于等于 0 的整数。');
    }

    state.settings = normalizeSettingsState(await state.backend.SaveSettings(payload));
    renderSettings();
    showBanner('设置已保存。', false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

function renderSettings() {
  const messages = document.getElementById('settings-weixin-history-messages');
  const runes = document.getElementById('settings-weixin-history-runes');

  if (messages) {
    messages.value = String(state.settings.weixinHistoryMessages ?? 12);
  }
  if (runes) {
    runes.value = String(state.settings.weixinHistoryRunes ?? 360);
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
            ${renderChatMessageContent(message)}
            ${message.time ? `<span class="chat-time">${escapeHTML(message.time)}</span>` : ''}
          </div>
        </div>
      `,
    )
    .join('');
  container.scrollTop = container.scrollHeight;
}

function renderChatMessageContent(message) {
  const payload = message.role === 'assistant' ? parseChatOptionsPayload(message.text) : null;
  if (payload) {
    return renderChatOptions(payload);
  }
  return `<div class="chat-markdown">${renderMarkdown(message.text || (message.streaming ? '思考中…' : ''))}</div>`;
}

function renderChatOptions(payload) {
  return `
    <div class="chat-option-card">
      <div class="chat-option-question">${escapeHTML(payload.question)}</div>
      <div class="chat-option-list">
        ${payload.options
          .map((option) => `
            <button
              type="button"
              class="chat-option-button"
              data-chat-option="${escapeAttribute(option.value)}"
              data-chat-option-value="${escapeAttribute(option.value)}"
              data-chat-option-label="${escapeAttribute(option.label)}"
              data-chat-option-question="${escapeAttribute(payload.question)}"
            >
              ${escapeHTML(option.label)}
            </button>
          `)
          .join('')}
      </div>
    </div>
  `;
}

function parseChatOptionsPayload(source) {
  const text = String(source ?? '').trim();
  if (!text.startsWith('{') || !text.endsWith('}')) return null;

  const jsonPayload = parseJSONChatOptionsPayload(text);
  if (jsonPayload) return jsonPayload;

  return parseEDNChatOptionsPayload(text);
}

function parseJSONChatOptionsPayload(text) {
  try {
    return normalizeChatOptionsPayload(JSON.parse(text));
  } catch (_error) {
    return null;
  }
}

function parseEDNChatOptionsPayload(text) {
  const questionMatch = text.match(/:question\s+"((?:\\.|[^"])*)"/s);
  const optionsMatch = text.match(/:options\s+\[((?:.|\n)*)\]/s);
  if (!questionMatch || !optionsMatch) return null;

  const question = unescapeChatOptionText(questionMatch[1]).trim();
  const options = Array.from(optionsMatch[1].matchAll(/"((?:\\.|[^"])*)"/g))
    .map((item) => unescapeChatOptionText(item[1]).trim())
    .filter(Boolean);
  return normalizeChatOptionsPayload({ question, options });
}

function normalizeChatOptionsPayload(payload) {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return null;

  const questionType = normalizeChatOptionScalar(payload.questiontype ?? payload.questionType).toLowerCase();
  if (questionType && questionType !== 'singleselect') return null;

  const question = normalizeChatOptionScalar(payload.question);
  const options = normalizeChatOptionList(payload.options);
  if (!question || options.length === 0) return null;

  return { question, options };
}

function normalizeChatOptionList(value) {
  if (!Array.isArray(value)) return [];
  return value
    .map((item) => normalizeChatOption(item))
    .filter(Boolean);
}

function normalizeChatOption(option) {
  if (typeof option === 'string' || typeof option === 'number' || typeof option === 'boolean') {
    const text = normalizeChatOptionScalar(option);
    return text ? { label: text, value: text } : null;
  }
  if (!option || typeof option !== 'object' || Array.isArray(option)) return null;

  const value = normalizeChatOptionScalar(option.value);
  const label = normalizeChatOptionScalar(option.label) || value;
  const nextValue = value || label;
  if (!label || !nextValue) return null;

  return { label, value: nextValue };
}

function normalizeChatOptionScalar(value) {
  if (typeof value === 'string') return value.trim();
  if (typeof value === 'number' || typeof value === 'boolean') return String(value).trim();
  return '';
}

function unescapeChatOptionText(value) {
  try {
    return JSON.parse(`"${value}"`);
  } catch (_error) {
    return value;
  }
}

function buildChatOptionSubmission(question, optionValue, optionLabel = optionValue) {
  const label = String(optionLabel ?? optionValue ?? '').trim();
  const value = String(optionValue ?? optionLabel ?? '').trim();
  const selection = label && value && label !== value
    ? `我选择“${label}”（选项值：${value}）。`
    : `我选择“${label || value}”。`;
  return [
    `对于你刚才给出的选项题“${question}”，${selection}`,
    '请严格基于上一轮上下文执行这个选择，不要把它当成一个脱离上下文的新话题。',
  ].join('\n');
}

function renderChatSessions() {
  const container = document.getElementById('chat-session-list');
  if (!container) return;

  const conversations = state.chatState.conversations || [];
  if (conversations.length === 0) {
    container.innerHTML = `
      <div class="empty-state compact">
        <div class="empty-state-icon">◌</div>
        <h3>还没有对话</h3>
        <p>点击右上角新建，或输入 <code>/new</code></p>
      </div>
    `;
    return;
  }

  container.innerHTML = conversations
    .map((conversation) => `
      <button
        type="button"
        class="chat-session-item ${conversation.active ? 'active' : ''}"
        data-chat-session="${escapeAttribute(conversation.sessionId)}"
      >
        <div class="chat-session-title-row">
          <strong>${escapeHTML(conversation.title || '新对话')}</strong>
          <span>${escapeHTML(conversation.updatedAt || '')}</span>
        </div>
        <div class="chat-session-preview">${escapeHTML(conversation.preview || '还没有消息')}</div>
      </button>
    `)
    .join('');
}

function applyChatState(nextState) {
  state.chatState = normalizeChatState(nextState);
  state.chat = (state.chatState.messages || []).map((message) => ({
    role: message.role,
    text: message.text,
    time: message.time || '',
  }));
  renderChatSessions();
  renderChat();
}

function syncCurrentChatConversationFromMessages() {
  const sessionId = state.chatState.sessionId || '';
  if (!sessionId) return;

  const conversations = Array.isArray(state.chatState.conversations) ? [...state.chatState.conversations] : [];
  const currentIndex = conversations.findIndex((item) => item.sessionId === sessionId);
  const nextConversation = {
    sessionId,
    title: summarizeChatTitle(state.chat),
    preview: summarizeChatPreview(state.chat),
    updatedAt: nowLabel(),
    updatedAtUnix: Date.now(),
    messageCount: state.chat.length,
    hasMessages: state.chat.length > 0,
    active: true,
  };

  const nextConversations = conversations.map((item, index) => ({
    ...item,
    active: index === currentIndex ? true : false,
  }));
  if (currentIndex >= 0) {
    nextConversations[currentIndex] = nextConversation;
  } else {
    nextConversations.unshift(nextConversation);
  }

  state.chatState = {
    ...state.chatState,
    sessionId,
    conversations: nextConversations,
    messages: state.chat.map((message) => ({
      role: message.role,
      text: message.text,
      time: message.time || '',
    })),
  };
  renderChatSessions();
}

function summarizeChatTitle(messages) {
  const firstUser = (messages || []).find((item) => item.role === 'user' && (item.text || '').trim());
  return truncateText(firstUser?.text || '新对话', 28);
}

function summarizeChatPreview(messages) {
  for (let index = (messages || []).length - 1; index >= 0; index -= 1) {
    const message = messages[index] || {};
    const payload = parseChatOptionsPayload(message.text);
    const text = (payload?.question || message.text || '').trim();
    if (text) return truncateText(text, 72);
  }
  return '还没有消息';
}

function truncateText(value, maxRunes) {
  const text = (value || '').trim();
  if (!text) return '';
  const chars = Array.from(text);
  if (chars.length <= maxRunes) return text;
  return `${chars.slice(0, Math.max(1, maxRunes - 1)).join('')}…`;
}

function newChatStreamRequestID() {
  if (window.crypto && typeof window.crypto.randomUUID === 'function') {
    return window.crypto.randomUUID();
  }
  return `chat-${Date.now()}-${Math.random().toString(16).slice(2)}`;
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

function defaultChatState() {
  return {
    sessionId: '',
    conversations: [],
    messages: [],
  };
}

function normalizeChatState(payload) {
  const source = Array.isArray(payload) ? payload[0] : payload;
  const next = {
    ...defaultChatState(),
    ...(source || {}),
  };
  next.conversations = Array.isArray(next.conversations)
    ? next.conversations.map((item) => ({
      sessionId: item?.sessionId || '',
      title: item?.title || '',
      preview: item?.preview || '',
      updatedAt: item?.updatedAt || '',
      updatedAtUnix: Number(item?.updatedAtUnix || 0),
      messageCount: Number(item?.messageCount || 0),
      hasMessages: Boolean(item?.hasMessages),
      active: Boolean(item?.active),
    }))
    : [];
  next.messages = Array.isArray(next.messages)
    ? next.messages.map((item) => ({
      role: item?.role || 'assistant',
      text: item?.text || '',
      time: item?.time || '',
    }))
    : [];
  return next;
}

function defaultChatPromptState() {
  return {
    promptId: '',
    shortId: '',
    title: '',
  };
}

function normalizeChatPromptState(payload) {
  const source = Array.isArray(payload) ? payload[0] : payload;
  return {
    ...defaultChatPromptState(),
    ...(source || {}),
  };
}

function defaultAutocompleteState() {
  return {
    open: false,
    trigger: '',
    query: '',
    tokenStart: -1,
    tokenEnd: -1,
    selectedIndex: 0,
    items: [],
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

function normalizeReminders(payload) {
  if (!Array.isArray(payload)) return [];
  return payload.map((item) => ({
    id: item.id || '',
    shortId: item.shortId || (item.id || '').slice(0, 8),
    message: item.message || '',
    frequency: item.frequency || 'once',
    frequencyLabel: item.frequencyLabel || (item.frequency === 'daily' ? '每天' : '单次'),
    scheduleLabel: item.scheduleLabel || (item.frequency === 'daily' ? '每天' : '单次'),
    nextRunAt: item.nextRunAt || '',
    nextRunAtUnix: Number(item.nextRunAtUnix || 0),
    createdAt: item.createdAt || '',
    createdAtUnix: Number(item.createdAtUnix || 0),
  }));
}

const MODEL_PROVIDER_DEFAULTS = {
  openai: {
    apiType: 'responses',
    baseUrl: 'https://api.openai.com/v1',
  },
  anthropic: {
    apiType: 'messages',
    baseUrl: 'https://api.anthropic.com/v1',
  },
};

const MODEL_API_TYPE_OPTIONS = {
  openai: [
    { value: 'responses', label: 'Responses (new)' },
    { value: 'chat_completions', label: 'Chat Completions (legacy)' },
  ],
  anthropic: [
    { value: 'messages', label: 'Messages' },
  ],
};

function defaultModelState() {
  return {
    profiles: [],
    activeProfileId: '',
    configured: false,
    missingFields: [],
    effectiveProfileName: '—',
    effectiveProvider: 'openai',
    effectiveApiType: 'responses',
    effectiveBaseUrl: 'https://api.openai.com/v1',
    effectiveApiKeyMasked: '(empty)',
    effectiveModel: '',
    message: '尚未保存任何模型 profile。',
  };
}

function normalizeModelSettings(payload) {
  const source = Array.isArray(payload) ? payload[0] : payload;
  const stateValue = {
    ...defaultModelState(),
    ...(source || {}),
  };
  stateValue.profiles = Array.isArray(stateValue.profiles)
    ? stateValue.profiles.map((item) => ({
        id: item.id || '',
        name: item.name || '',
        provider: item.provider || 'openai',
        apiType: item.apiType || 'responses',
        baseUrl: item.baseUrl || MODEL_PROVIDER_DEFAULTS[item.provider || 'openai']?.baseUrl || '',
        model: item.model || '',
        hasApiKey: Boolean(item.hasApiKey),
        apiKeyMasked: item.apiKeyMasked || (item.hasApiKey ? '********' : '(empty)'),
        active: Boolean(item.active),
      }))
    : [];
  return stateValue;
}

function defaultSettingsState() {
  return {
    weixinHistoryMessages: 12,
    weixinHistoryRunes: 360,
  };
}

function normalizeSettingsState(payload) {
  const source = Array.isArray(payload) ? payload[0] : payload;
  return {
    ...defaultSettingsState(),
    ...(source || {}),
    weixinHistoryMessages: Number(source?.weixinHistoryMessages ?? 12),
    weixinHistoryRunes: Number(source?.weixinHistoryRunes ?? 360),
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

function relativeTimeLabel(timestamp) {
  const value = Number(timestamp || 0);
  if (!value) return '未安排';

  const diffSeconds = value - Math.floor(Date.now() / 1000);
  const absSeconds = Math.abs(diffSeconds);
  const future = diffSeconds >= 0;

  if (absSeconds < 60) {
    return future ? '1 分钟内' : '刚刚';
  }

  const units = [
    { size: 24 * 60 * 60, label: '天' },
    { size: 60 * 60, label: '小时' },
    { size: 60, label: '分钟' },
  ];

  for (const unit of units) {
    if (absSeconds >= unit.size) {
      const amount = Math.floor(absSeconds / unit.size);
      return future ? `${amount} ${unit.label}后` : `${amount} ${unit.label}前`;
    }
  }

  return future ? '即将执行' : '已执行';
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

function preview(value, maxLength) {
  const text = String(value ?? '').replace(/\s+/g, ' ').trim();
  if (!text) return '';
  if (text.length <= maxLength) return text;
  return `${text.slice(0, Math.max(maxLength - 1, 1))}…`;
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
