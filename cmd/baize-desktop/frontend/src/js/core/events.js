function bindStaticEvents() {
  document.addEventListener('click', (event) => {
    const anchor = event.target.closest('a[href]');
    if (!anchor) return;

    const targetURL = sanitizeHTTPURL(anchor.getAttribute('href') || anchor.href || '');
    if (!targetURL) return;
    if (state.backendMode !== 'wails' || !state.backend || typeof state.backend.OpenExternalURL !== 'function') {
      return;
    }

    event.preventDefault();
    void openExternalLink(targetURL);
  });

  // Theme toggle
  const themeToggle = document.getElementById('theme-toggle');
  if (themeToggle) {
    themeToggle.addEventListener('click', toggleTheme);
  }

  const versionCheck = document.getElementById('version-check');
  if (versionCheck) {
    versionCheck.addEventListener('click', () => void checkLatestVersion());
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

  const screenTraceSave = document.getElementById('settings-screentrace-save');
  if (screenTraceSave) {
    screenTraceSave.addEventListener('click', () => void saveSettings());
  }

  const screenTraceAutoSaveIds = [
    'settings-screentrace-enabled',
    'settings-screentrace-interval-seconds',
    'settings-screentrace-retention-days',
    'settings-screentrace-vision-profile',
    'settings-screentrace-write-digests-kb',
  ];
  screenTraceAutoSaveIds.forEach((id) => {
    const field = document.getElementById(id);
    if (!field) return;
    field.addEventListener('change', () => void saveSettings());
  });

  const screenTraceRefresh = document.getElementById('screentrace-refresh');
  if (screenTraceRefresh) {
    screenTraceRefresh.addEventListener('click', () => void refreshScreenTraceManually());
  }

  const screenTraceCaptureNow = document.getElementById('screentrace-capture-now');
  if (screenTraceCaptureNow) {
    screenTraceCaptureNow.addEventListener('click', () => void captureScreenTraceNow());
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

  const toolList = document.getElementById('tool-list');
  if (toolList) {
    toolList.addEventListener('click', (event) => {
      const actionTarget = event.target.closest('[data-tool-action]');
      if (!actionTarget) return;

      if (actionTarget.dataset.toolAction === 'save-everything-path') {
        void saveSettings();
      } else if (actionTarget.dataset.toolAction === 'toggle-enabled') {
        void setToolEnabled(actionTarget.dataset.toolName || '', actionTarget.dataset.toolEnabled === 'true');
      } else if (actionTarget.dataset.toolAction === 'toggle-group') {
        toggleToolGroupCollapse(actionTarget.dataset.toolGroupKey || '');
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
      if (Date.now() < (state.chatSessionDrag.suppressClickUntil || 0)) return;
      closeChatSessionContextMenu();
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
      const refreshButton = event.target.closest('[data-chat-refresh-index]');
      if (refreshButton) {
        const messageIndex = Number(refreshButton.dataset.chatRefreshIndex || '-1');
        if (!Number.isInteger(messageIndex) || messageIndex < 0) return;
        void confirmRefreshChatMessage(messageIndex);
        return;
      }

      const copyButton = event.target.closest('[data-chat-copy-index]');
      if (copyButton) {
        const messageIndex = Number(copyButton.dataset.chatCopyIndex || '-1');
        if (!Number.isInteger(messageIndex) || messageIndex < 0) return;
        void copyChatMessage(messageIndex);
        return;
      }

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

  const modelSection = document.getElementById('settings-section-model');
  if (modelSection) {
    const markModelFormDirty = (event) => {
      const target = event.target;
      if (!(target instanceof HTMLInputElement || target instanceof HTMLSelectElement || target instanceof HTMLTextAreaElement)) return;
      if (!target.id || !target.id.startsWith('model-')) return;
      if (target.id === 'model-profile-select') return;
      state.modelFormDirty = true;
    };
    modelSection.addEventListener('input', markModelFormDirty);
    modelSection.addEventListener('change', markModelFormDirty);
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
    if (!event.target.closest('#chat-session-context-menu')) {
      closeChatSessionContextMenu();
    }
    if (event.target.closest('.chat-input-area')) return;
    closeChatAutocomplete();
  });
}

function bindRuntimeEvents() {
  if (!window.runtime || typeof window.runtime.EventsOn !== 'function') return;

  const bindEvent = (name, handler) => {
    try {
      window.runtime.EventsOn(name, handler);
    } catch (error) {
      reportDesktopDiagnostics('runtime-event-bind-failed', {
        eventName: name,
        error,
      });
      throw error;
    }
  };

  bindEvent('reminder:due', (payload) => {
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

  bindEvent('weixin:status', (payload) => {
    const next = normalizeWeixinStatus(payload);
    applyWeixinStatus(next, true);
  });

  bindEvent('chat:changed', () => {
    if (state.chatStreaming) return;
    void refreshChatState().catch(() => {});
  });

  bindEvent('chat:stream', (payload) => {
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
