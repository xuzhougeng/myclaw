function bindNavigation() {
  document.querySelectorAll('.nav-item, [data-nav-view]').forEach(item => {
    item.addEventListener('click', (e) => {
      e.preventDefault();
      const viewName = item.dataset.navView || item.dataset.view;
      const sectionId = item.dataset.navSection || '';
      if (viewName) {
        window.navigateTo(viewName, sectionId);
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

function bindChatSessionUI() {
  state.chatSidebarCollapsed = localStorage.getItem('baize-chat-sidebar-collapsed') === '1';
  applyChatSidebarState();

  const toggle = document.getElementById('chat-sidebar-toggle');
  if (toggle) {
    toggle.addEventListener('click', () => {
      setChatSidebarCollapsed(!state.chatSidebarCollapsed);
    });
  }

  const dialog = document.getElementById('chat-session-dialog');
  if (dialog) {
    dialog.addEventListener('click', (event) => {
      if (event.target === dialog) {
        closeChatSessionDialog();
      }
    });
  }

  const cancel = document.getElementById('chat-session-dialog-cancel');
  if (cancel) {
    cancel.addEventListener('click', closeChatSessionDialog);
  }

  const confirm = document.getElementById('chat-session-dialog-confirm');
  if (confirm) {
    confirm.addEventListener('click', () => void submitChatSessionDialog());
  }

  const modeOptions = document.getElementById('chat-session-dialog-mode-options');
  if (modeOptions) {
    modeOptions.addEventListener('click', (event) => {
      const target = event.target instanceof Element
        ? event.target.closest('[data-chat-session-mode-option]')
        : null;
      if (!target) return;
      setChatSessionDialogMode(target.dataset.chatSessionModeOption || '');
    });
  }

  const contextMenu = document.getElementById('chat-session-context-menu');
  if (contextMenu) {
    contextMenu.addEventListener('click', (event) => {
      const target = event.target instanceof Element
        ? event.target.closest('[data-chat-session-menu-action]')
        : null;
      if (!target) return;

      const action = target.dataset.chatSessionMenuAction || '';
      const sessionId = state.chatSessionContextMenu.sessionId || '';
      closeChatSessionContextMenu();
      if (!sessionId) return;
      if (action === 'rename') {
        void renameChatSession(sessionId);
      } else if (action === 'delete') {
        void deleteChatSession(sessionId);
      }
    });
  }

  const sessionList = document.getElementById('chat-session-list');
  if (sessionList) {
    sessionList.addEventListener('contextmenu', (event) => {
      const target = event.target instanceof Element
        ? event.target.closest('[data-chat-session]')
        : null;
      if (!target) return;

      const sessionId = target.dataset.chatSession || '';
      if (!sessionId) return;

      event.preventDefault();
      openChatSessionContextMenu(sessionId, event.clientX, event.clientY);
    });

    sessionList.addEventListener('dragstart', (event) => {
      const row = event.target instanceof Element
        ? event.target.closest('[data-chat-session-row]')
        : null;
      if (!row) return;

      const sessionId = row.dataset.chatSessionRow || '';
      if (!sessionId) {
        event.preventDefault();
        return;
      }

      closeChatSessionContextMenu();
      clearChatSessionDropIndicators();
      row.classList.add('dragging');
      state.chatSessionDrag = {
        ...state.chatSessionDrag,
        sessionId,
        targetSessionId: '',
        placeBefore: true,
      };

      if (event.dataTransfer) {
        event.dataTransfer.effectAllowed = 'move';
        event.dataTransfer.setData('text/plain', sessionId);
      }
    });

    sessionList.addEventListener('dragover', (event) => {
      const draggingSessionId = state.chatSessionDrag.sessionId || '';
      const row = event.target instanceof Element
        ? event.target.closest('[data-chat-session-row]')
        : null;
      const targetSessionId = row?.dataset.chatSessionRow || '';
      if (!draggingSessionId || !row || !targetSessionId || draggingSessionId === targetSessionId) return;

      event.preventDefault();
      const rect = row.getBoundingClientRect();
      const placeBefore = event.clientY < rect.top + rect.height / 2;
      clearChatSessionDropIndicators();
      row.classList.add(placeBefore ? 'drop-before' : 'drop-after');
      state.chatSessionDrag = {
        ...state.chatSessionDrag,
        targetSessionId,
        placeBefore,
      };
      if (event.dataTransfer) {
        event.dataTransfer.dropEffect = 'move';
      }
    });

    sessionList.addEventListener('drop', (event) => {
      const draggingSessionId = state.chatSessionDrag.sessionId || '';
      const row = event.target instanceof Element
        ? event.target.closest('[data-chat-session-row]')
        : null;
      const targetSessionId = row?.dataset.chatSessionRow || state.chatSessionDrag.targetSessionId || '';
      if (!draggingSessionId || !targetSessionId || draggingSessionId === targetSessionId) {
        clearChatSessionDropIndicators();
        state.chatSessionDrag = {
          ...defaultChatSessionDragState(),
          suppressClickUntil: Date.now() + 200,
        };
        return;
      }

      event.preventDefault();
      reorderChatSessions(draggingSessionId, targetSessionId, state.chatSessionDrag.placeBefore);
      clearChatSessionDropIndicators();
      state.chatSessionDrag = {
        ...defaultChatSessionDragState(),
        suppressClickUntil: Date.now() + 200,
      };
    });

    sessionList.addEventListener('dragend', () => {
      clearChatSessionDropIndicators();
      state.chatSessionDrag = {
        ...defaultChatSessionDragState(),
        suppressClickUntil: state.chatSessionDrag.suppressClickUntil || 0,
      };
    });
  }

  const input = document.getElementById('chat-session-dialog-input');
  if (input) {
    input.addEventListener('keydown', (event) => {
      if (event.isComposing) return;
      if (event.key === 'Enter') {
        event.preventDefault();
        void submitChatSessionDialog();
        return;
      }
      if (event.key === 'Escape') {
        event.preventDefault();
        closeChatSessionDialog();
      }
    });
  }

  document.addEventListener('keydown', (event) => {
    if (event.key !== 'Escape') return;
    if (state.chatSessionDialog.open) {
      event.preventDefault();
      closeChatSessionDialog();
      return;
    }
    if (state.chatSessionContextMenu.open) {
      event.preventDefault();
      closeChatSessionContextMenu();
    }
  });

  window.addEventListener('blur', closeChatSessionContextMenu);
}

function setChatSidebarCollapsed(collapsed) {
  state.chatSidebarCollapsed = Boolean(collapsed);
  localStorage.setItem('baize-chat-sidebar-collapsed', state.chatSidebarCollapsed ? '1' : '0');
  applyChatSidebarState();
}

function applyChatSidebarState() {
  const container = document.getElementById('chat-container');
  const toggle = document.getElementById('chat-sidebar-toggle');
  const icon = document.getElementById('chat-sidebar-toggle-icon');
  if (container) {
    container.classList.toggle('chat-sidebar-collapsed', state.chatSidebarCollapsed);
  }
  if (toggle) {
    const label = state.chatSidebarCollapsed ? '展开对话列表' : '收起对话列表';
    toggle.setAttribute('aria-expanded', String(!state.chatSidebarCollapsed));
    toggle.setAttribute('aria-label', label);
    toggle.title = label;
  }
  if (icon) {
    icon.textContent = state.chatSidebarCollapsed ? '>' : '<';
  }
}

function setChatSessionDialogMode(mode) {
  const nextMode = String(mode || '').trim().toLowerCase() === 'ask' ? 'ask' : 'agent';
  state.chatSessionDialog = {
    ...state.chatSessionDialog,
    selectedMode: nextMode,
  };

  document.querySelectorAll('[data-chat-session-mode-option]').forEach((option) => {
    option.classList.toggle('active', option.dataset.chatSessionModeOption === nextMode);
  });

  const modeHint = document.getElementById('chat-session-dialog-mode-hint');
  if (modeHint) {
    modeHint.textContent = nextMode === 'ask'
      ? 'Ask 只走传统一问一答；需要时可在单条消息前加 @kb 临时附加知识库。'
      : 'Agent 会自主判断是否调用知识库、提醒和本地只读工具。';
  }
}

function openChatSessionDialog(mode, conversation) {
  const dialog = document.getElementById('chat-session-dialog');
  const card = dialog?.querySelector('.dialog-card');
  const eyebrow = document.getElementById('chat-session-dialog-eyebrow');
  const title = document.getElementById('chat-session-dialog-title');
  const description = document.getElementById('chat-session-dialog-desc');
  const targetLabel = document.getElementById('chat-session-dialog-target-label');
  const targetValue = document.getElementById('chat-session-dialog-target-value');
  const field = document.getElementById('chat-session-dialog-field');
  const input = document.getElementById('chat-session-dialog-input');
  const modeGroup = document.getElementById('chat-session-dialog-mode-group');
  const modeHint = document.getElementById('chat-session-dialog-mode-hint');
  const confirm = document.getElementById('chat-session-dialog-confirm');
  if (!dialog || !card || !eyebrow || !title || !description || !targetLabel || !targetValue || !field || !input || !modeGroup || !modeHint || !confirm) {
    return;
  }

  state.chatSessionDialog = {
    ...defaultChatSessionDialogState(),
    open: true,
    mode,
    sessionId: conversation?.sessionId || '',
    itemId: conversation?.id || '',
    restoreFocus: document.activeElement instanceof HTMLElement ? document.activeElement : null,
    selectedMode: conversation?.mode === 'ask' ? 'ask' : 'agent',
  };

  if (mode === 'knowledge-delete') {
    const shortId = (conversation?.shortId || String(conversation?.id || '').slice(0, 8)).trim();
    const previewText = truncateText(String(conversation?.preview || conversation?.text || '').replace(/\s+/g, ' ').trim(), 48);
    const displayTitle = previewText ? `#${shortId} · ${previewText}` : `#${shortId}`;
    state.chatSessionDialog.initialTitle = displayTitle;
    targetLabel.textContent = '目标记忆';
    targetValue.textContent = displayTitle;
    eyebrow.textContent = '危险操作';
    title.textContent = '删除记忆';
    description.textContent = '删除后这条记忆会立即从当前记忆库移除，这个动作不能撤销。';
    field.hidden = true;
    input.value = displayTitle;
    input.placeholder = '';
    confirm.textContent = '删除';
    confirm.classList.remove('btn-primary');
    confirm.classList.add('btn-danger');
    card.classList.add('danger');
    modeGroup.hidden = true;
  } else if (mode === 'prompt-delete') {
    const shortId = (conversation?.shortId || String(conversation?.id || '').slice(0, 8)).trim();
    const promptTitle = String(conversation?.title || '').trim();
    const previewText = truncateText(String(conversation?.content || '').replace(/\s+/g, ' ').trim(), 48);
    const displayTitle = promptTitle ? `#${shortId} · ${promptTitle}` : `#${shortId}`;
    state.chatSessionDialog.initialTitle = displayTitle;
    targetLabel.textContent = '目标 Prompt';
    targetValue.textContent = displayTitle;
    eyebrow.textContent = '危险操作';
    title.textContent = '删除 Prompt';
    description.textContent = previewText
      ? `删除后这个 Prompt 会立即从 Prompt 库移除，这个动作不能撤销。\n\n内容预览：${previewText}`
      : '删除后这个 Prompt 会立即从 Prompt 库移除，这个动作不能撤销。';
    field.hidden = true;
    input.value = displayTitle;
    input.placeholder = '';
    confirm.textContent = '删除';
    confirm.classList.remove('btn-primary');
    confirm.classList.add('btn-danger');
    card.classList.add('danger');
    modeGroup.hidden = true;
  } else if (mode === 'refresh') {
    const previewText = truncateText(String(conversation?.preview || '').replace(/\s+/g, ' ').trim(), 48);
    const displayTitle = previewText || '当前回复';
    state.chatSessionDialog.initialTitle = displayTitle;
    targetLabel.textContent = '当前回复';
    targetValue.textContent = displayTitle;
    eyebrow.textContent = '确认操作';
    title.textContent = '刷新当前回复';
    description.textContent = '是不是一定要刷新？刷新后会丢弃当前这条 AI 回复，并重新生成。';
    field.hidden = true;
    input.value = displayTitle;
    input.placeholder = '';
    confirm.textContent = '刷新';
    confirm.classList.remove('btn-danger');
    confirm.classList.add('btn-primary');
    card.classList.remove('danger');
    modeGroup.hidden = true;
  } else if (mode === 'new') {
    targetLabel.textContent = '新对话';
    targetValue.textContent = '创建后会固定当前会话模式';
    eyebrow.textContent = '对话模式';
    title.textContent = '新建对话';
    description.textContent = '默认推荐 agent。ask 适合传统一问一答；agent 会自主判断是否调用知识库、提醒和本地只读工具。';
    field.hidden = true;
    modeGroup.hidden = false;
    input.value = '';
    input.placeholder = '';
    confirm.textContent = '创建';
    confirm.classList.remove('btn-danger');
    confirm.classList.add('btn-primary');
    card.classList.remove('danger');
    setChatSessionDialogMode(state.chatSessionDialog.selectedMode);
  } else {
    const displayTitle = (conversation?.title || '新对话').trim() || '新对话';
    state.chatSessionDialog.initialTitle = displayTitle;
    targetLabel.textContent = '当前对话';
    targetValue.textContent = displayTitle;

    if (mode === 'delete') {
      eyebrow.textContent = '危险操作';
      title.textContent = '删除对话';
      description.textContent = '删除后当前会话消息会一并移除，这个动作不能撤销。';
      field.hidden = true;
      input.value = displayTitle;
      input.placeholder = '';
      confirm.textContent = '删除';
      confirm.classList.remove('btn-primary');
      confirm.classList.add('btn-danger');
      card.classList.add('danger');
      modeGroup.hidden = true;
    } else {
      eyebrow.textContent = '对话标题';
      title.textContent = '重命名对话';
      description.textContent = '只修改列表显示，不影响当前上下文和消息内容。';
      field.hidden = false;
      modeGroup.hidden = true;
      input.value = displayTitle;
      input.placeholder = '输入对话标题';
      confirm.textContent = '保存';
      confirm.classList.remove('btn-danger');
      confirm.classList.add('btn-primary');
      card.classList.remove('danger');
    }
  }

  dialog.hidden = false;
  requestAnimationFrame(() => {
    if (mode === 'delete' || mode === 'knowledge-delete' || mode === 'prompt-delete' || mode === 'refresh') {
      confirm.focus();
    } else if (mode === 'new') {
      const activeMode = document.querySelector('.dialog-choice-option.active');
      if (activeMode instanceof HTMLElement) {
        activeMode.focus();
      } else {
        confirm.focus();
      }
    } else {
      input.focus();
      input.select();
    }
  });
}

function closeChatSessionDialog() {
  const dialog = document.getElementById('chat-session-dialog');
  const card = dialog?.querySelector('.dialog-card');
  const targetLabel = document.getElementById('chat-session-dialog-target-label');
  const targetValue = document.getElementById('chat-session-dialog-target-value');
  const field = document.getElementById('chat-session-dialog-field');
  const input = document.getElementById('chat-session-dialog-input');
  const modeGroup = document.getElementById('chat-session-dialog-mode-group');
  const modeHint = document.getElementById('chat-session-dialog-mode-hint');
  const confirm = document.getElementById('chat-session-dialog-confirm');
  const restoreFocus = state.chatSessionDialog.restoreFocus;

  state.chatSessionDialog = defaultChatSessionDialogState();

  if (dialog) {
    dialog.hidden = true;
  }
  if (card) {
    card.classList.remove('danger');
  }
  if (targetLabel) {
    targetLabel.textContent = '当前对话';
  }
  if (targetValue) {
    targetValue.textContent = '新对话';
  }
  if (field) {
    field.hidden = false;
  }
  if (input) {
    input.value = '';
    input.placeholder = '输入对话标题';
  }
  if (modeGroup) {
    modeGroup.hidden = true;
  }
  if (modeHint) {
    modeHint.textContent = 'Agent 会自主判断是否调用知识库、提醒和本地只读工具。';
  }
  document.querySelectorAll('[data-chat-session-mode-option]').forEach((option) => {
    option.classList.remove('active');
  });
  if (confirm) {
    confirm.disabled = false;
    confirm.textContent = '保存';
    confirm.classList.remove('btn-danger');
    confirm.classList.add('btn-primary');
  }
  if (restoreFocus && typeof restoreFocus.focus === 'function') {
    restoreFocus.focus();
  }
}

async function submitChatSessionDialog() {
  if (!state.chatSessionDialog.open) return;

  const { mode, sessionId, itemId, initialTitle } = state.chatSessionDialog;
  const input = document.getElementById('chat-session-dialog-input');
  const confirm = document.getElementById('chat-session-dialog-confirm');
  if (!confirm) return;
  if ((mode === 'rename' || mode === 'delete') && !sessionId) return;
  if ((mode === 'knowledge-delete' || mode === 'prompt-delete') && !itemId) return;

  if (mode === 'rename') {
    const nextTitle = (input?.value || '').trim();
    if (!nextTitle) {
      showBanner('对话标题不能为空。', true);
      if (input) input.focus();
      return;
    }
    if (nextTitle === (initialTitle || '').trim()) {
      closeChatSessionDialog();
      return;
    }
  }

  confirm.disabled = true;

  try {
    if (mode === 'refresh') {
      closeChatSessionDialog();
      await refreshCurrentChatResponse();
      return;
    }

    if (mode === 'new') {
      const selectedMode = state.chatSessionDialog.selectedMode === 'ask' ? 'ask' : 'agent';
      applyChatState(normalizeChatState(await state.backend.NewChatSession(selectedMode)));
      closeChatSessionDialog();
      await Promise.all([refreshSkills(), refreshChatPrompt(), refreshProjectState()]);
      const chatInput = document.getElementById('chat-input');
      if (chatInput) chatInput.focus();
      showBanner(`已开启 ${selectedMode} 对话。`, false);
      return;
    }

    if (mode === 'delete') {
      applyChatState(normalizeChatState(await state.backend.DeleteChatSession(sessionId)));
      closeChatSessionDialog();
      await Promise.all([refreshSkills(), refreshChatPrompt()]);
      showBanner('对话已删除。', false);
      return;
    }

    if (mode === 'knowledge-delete') {
      const result = await state.backend.DeleteKnowledge(itemId);
      closeChatSessionDialog();
      await refreshAll();
      showBanner(result.message, false);
      return;
    }

    if (mode === 'prompt-delete') {
      const result = await state.backend.DeletePrompt(itemId);
      closeChatSessionDialog();
      await refreshAll();
      showBanner(result.message, false);
      return;
    }

    applyChatState(normalizeChatState(await state.backend.RenameChatSession(sessionId, (input?.value || '').trim())));
    closeChatSessionDialog();
    showBanner('对话已重命名。', false);
  } catch (error) {
    showBanner(asMessage(error), true);
  } finally {
    if (state.chatSessionDialog.open) {
      confirm.disabled = false;
    }
  }
}
