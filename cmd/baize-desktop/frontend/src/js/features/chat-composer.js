async function sendMessage(rawText = null, displayText = null) {
  if (state.chatStreaming) return;

  const conversation = currentChatConversation();
  if (conversation?.readOnly) {
    showBanner('当前为微信会话，只支持查看历史；请新建本地对话后继续。', true);
    return;
  }

  const input = document.getElementById('chat-input');
  const shouldRestoreFocus = rawText == null;
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
    process: [],
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
          onProcess: (step) => {
            if (!appendChatProcessStep(placeholder, step)) return;
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
    placeholder.usage = normalizeTokenUsage(result.usage);
    placeholder.process = normalizeChatProcess(result.process);
    placeholder.streaming = false;
    if (result.historyPersisted === false) {
      markLatestChatExchangeTransient();
    }
    syncCurrentChatConversationFromMessages();
    renderChat();
    await Promise.all([refreshAll(), refreshChatState()]);
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
    renderChatContentActions();
    renderChatComposerState();
    if (shouldRestoreFocus) {
      focusChatInput();
    }
  }
}

function markLatestChatExchangeTransient() {
  if (state.chat.length < 2) return;
  const assistantIndex = state.chat.length - 1;
  const userIndex = assistantIndex - 1;
  if (userIndex < 0) return;

  if (state.chat[userIndex]) {
    state.chat[userIndex].transient = true;
  }
  if (state.chat[assistantIndex]) {
    state.chat[assistantIndex].transient = true;
  }
}

async function startNewConversation() {
  if (state.chatStreaming) {
    showBanner('当前回复尚未完成。', true);
    return;
  }
  openChatSessionDialog('new', { mode: 'agent' });
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

async function copyChatMessage(messageIndex) {
  const message = state.chat[messageIndex];
  if (!message) {
    showBanner('没有找到要复制的对话内容。', true);
    return;
  }

  const text = buildChatCopyText(message);
  if (!text) {
    showBanner('当前这条对话没有可复制的内容。', true);
    return;
  }

  try {
    await copyTextToClipboard(text);
    showBanner('已复制当前对话。', false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

function findRefreshableChatMessageIndex() {
  if (state.chatStreaming || state.chat.length === 0) return -1;
  const messageIndex = state.chat.length - 1;
  const message = state.chat[messageIndex];
  if (!message || message.role !== 'assistant' || message.streaming) {
    return -1;
  }
  return String(message.text || '').trim() ? messageIndex : -1;
}

async function confirmRefreshChatMessage(messageIndex) {
  if (state.chatStreaming) {
    showBanner('当前回复尚未完成。', true);
    return;
  }

  const refreshableIndex = findRefreshableChatMessageIndex();
  if (messageIndex !== refreshableIndex) {
    showBanner('目前只能刷新当前最后一条回复。', true);
    return;
  }

  const message = state.chat[messageIndex];
  if (!message) {
    showBanner('没有找到要刷新的回复。', true);
    return;
  }

  openChatSessionDialog('refresh', {
    sessionId: state.chatState.sessionId,
    preview: buildChatCopyText(message),
  });
}

async function refreshCurrentChatResponse() {
  if (state.chatStreaming) {
    showBanner('当前回复尚未完成。', true);
    return;
  }

  const messageIndex = findRefreshableChatMessageIndex();
  if (messageIndex < 0) {
    showBanner('当前没有可刷新的回复。', true);
    return;
  }

  const previousMessage = state.chat[messageIndex];
  const placeholder = {
    role: 'assistant',
    text: '',
    time: '',
    process: [],
    streaming: true,
  };

  state.chat = [...state.chat.slice(0, messageIndex), placeholder];
  syncCurrentChatConversationFromMessages();
  renderChat();

  state.chatStreaming = true;
  try {
    const result = await state.backend.RefreshChatResponse();
    placeholder.text = result.reply || placeholder.text;
    placeholder.time = result.timestamp || nowLabel();
    placeholder.usage = normalizeTokenUsage(result.usage);
    placeholder.process = normalizeChatProcess(result.process);
    placeholder.streaming = false;
    syncCurrentChatConversationFromMessages();
    renderChat();
    await refreshAll();
    showBanner('已刷新当前回复。', false);
  } catch (error) {
    state.chat = [...state.chat.slice(0, messageIndex), previousMessage];
    syncCurrentChatConversationFromMessages();
    renderChat();
    await refreshChatState().catch(() => {});
    showBanner(asMessage(error), true);
  } finally {
    state.chatStreaming = false;
    renderChatContentActions();
    renderChatComposerState();
  }
}

function appendChatProcessStep(message, step) {
  const next = normalizeChatProcess([step]);
  if (next.length === 0 || !message) return false;
  message.process = [...normalizeChatProcess(message.process), next[0]];
  return true;
}

async function copyTextToClipboard(text) {
  const value = String(text ?? '');
  if (!value) {
    throw new Error('当前没有可复制的内容。');
  }

  if (navigator.clipboard && typeof navigator.clipboard.writeText === 'function') {
    try {
      await navigator.clipboard.writeText(value);
      return;
    } catch (_error) {
      // Fall back to execCommand for desktop shells that do not expose clipboard permissions.
    }
  }

  const textarea = document.createElement('textarea');
  textarea.value = value;
  textarea.setAttribute('readonly', 'readonly');
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  textarea.style.pointerEvents = 'none';
  textarea.style.left = '-9999px';
  document.body.append(textarea);
  textarea.focus();
  textarea.select();
  textarea.setSelectionRange(0, textarea.value.length);

  const copied = typeof document.execCommand === 'function' && document.execCommand('copy');
  textarea.remove();

  if (!copied) {
    throw new Error('复制失败，请手动选择内容后复制。');
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

async function renameChatSession(sessionId) {
  if (state.chatStreaming) {
    showBanner('当前回复尚未完成。', true);
    return;
  }
  const conversation = (state.chatState.conversations || []).find((item) => item.sessionId === sessionId);
  if (!conversation) return;
  openChatSessionDialog('rename', conversation);
}

async function deleteChatSession(sessionId) {
  if (state.chatStreaming) {
    showBanner('当前回复尚未完成。', true);
    return;
  }
  const conversation = (state.chatState.conversations || []).find((item) => item.sessionId === sessionId);
  if (!conversation) return;
  openChatSessionDialog('delete', conversation);
}

function renderChatContext() {
  const container = document.getElementById('chat-context-bar');
  if (!container) return;

  const chips = [];
  const conversation = currentChatConversation();
  if (conversation?.sourceLabel) {
    const modeLabel = conversation.mode === 'ask'
      ? 'Ask Mode'
      : conversation.mode === 'agent'
        ? 'Agent Mode'
        : '';
    chips.push(`
      <span class="chat-context-chip ${conversation.readOnly ? 'skill' : 'prompt'}">
        <span>来源</span>
        <strong>${escapeHTML(conversation.sourceLabel)}</strong>
        ${modeLabel ? `<span class="chat-context-mode">${escapeHTML(modeLabel)}</span>` : ''}
        ${conversation.readOnly ? '<span>只读</span>' : ''}
      </span>
    `);
  }
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
  renderChatComposerState();
}

function renderChatComposerState() {
  const input = document.getElementById('chat-input');
  const sendButton = document.getElementById('chat-send');
  const conversation = currentChatConversation();
  const readOnly = Boolean(conversation?.readOnly);

  if (input instanceof HTMLTextAreaElement) {
    input.disabled = readOnly || state.chatStreaming;
    input.placeholder = readOnly
      ? '当前为微信会话，只读查看；如需继续对话，请新建本地对话。'
      : '输入消息，或使用 / 命令、$ 技能、@ Prompt...';
  }
  if (sendButton instanceof HTMLButtonElement) {
    sendButton.disabled = readOnly || state.chatStreaming;
    sendButton.title = readOnly ? '微信会话只支持查看历史' : '发送消息';
  }
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
