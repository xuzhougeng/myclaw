function renderChat() {
  const container = document.getElementById('chat-list');
  if (!container) {
    renderChatContentActions();
    renderChatComposerState();
    return;
  }

  if (state.chat.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">○</div>
        <h3>开始新对话</h3>
        <p>输入问题或使用命令如 /kb remember、/notice、/skill list</p>
      </div>
    `;
    renderChatContentActions();
    renderChatComposerState();
    return;
  }

  container.innerHTML = state.chat
    .map(
      (message, index) => `
        <div class="chat-message ${escapeAttribute(message.role)}">
          <div class="chat-avatar">${message.role === 'user' ? '◐' : message.role === 'system' ? '◇' : '○'}</div>
          <div class="chat-bubble">
            ${renderChatProcess(message)}
            ${renderChatMessageContent(message)}
            ${renderChatMessageFooter(message, index)}
          </div>
        </div>
      `,
    )
    .join('');
  container.scrollTop = container.scrollHeight;
  renderChatContentActions();
  renderChatComposerState();
}

function renderChatProcess(message) {
  const steps = normalizeChatProcess(message?.process);
  if (message?.role === 'user' || steps.length === 0) return '';
  const open = message?.streaming || message?.role === 'system';
  return `
    <details class="chat-process"${open ? ' open' : ''}>
      <summary>调试过程</summary>
      <ol class="chat-process-list">
        ${steps.map((step) => `
          <li class="chat-process-step">
            ${step.title ? `<div class="chat-process-title">${escapeHTML(step.title)}</div>` : ''}
            ${step.detail ? `<pre class="chat-process-detail">${escapeHTML(step.detail)}</pre>` : ''}
          </li>
        `).join('')}
      </ol>
    </details>
  `;
}

function renderChatContentActions() {
  const exportButton = document.getElementById('chat-export-markdown');
  if (exportButton) {
    const disabled = state.chatStreaming || state.chat.length === 0;
    exportButton.disabled = disabled;
    exportButton.title = state.chatStreaming
      ? '当前回复尚未完成。'
      : state.chat.length === 0
        ? '当前对话还没有消息可导出。'
        : '导出当前对话';
  }

  const newButton = document.getElementById('chat-new-session');
  if (newButton) {
    newButton.disabled = Boolean(state.chatStreaming);
    newButton.title = state.chatStreaming ? '当前回复尚未完成。' : '开启新对话';
  }
}

function renderChatMeta(message) {
  const parts = [];
  if (message.time) {
    parts.push(`<span class="chat-time">${escapeHTML(message.time)}</span>`);
  }
  if (message.role === 'assistant') {
    const usageText = formatTokenUsage(message.usage);
    if (usageText) {
      parts.push(`<span class="chat-usage">${escapeHTML(usageText)}</span>`);
    }
  }
  if (parts.length === 0) return '';
  return `<div class="chat-meta">${parts.join('')}</div>`;
}

function renderChatMessageFooter(message, index) {
  const meta = renderChatMeta(message);
  return `
    <div class="chat-bubble-footer${meta ? '' : ' copy-only'}">
      ${meta || ''}
      ${renderChatMessageActions(message, index)}
    </div>
  `;
}

function renderChatMessageActions(message, index) {
  return `
    <div class="chat-message-actions">
      ${renderChatRefreshButton(message, index)}
      ${renderChatCopyButton(index)}
    </div>
  `;
}

function renderChatRefreshButton(message, index) {
  if (currentChatConversation()?.readOnly || message.role !== 'assistant' || index !== findRefreshableChatMessageIndex()) {
    return '';
  }

  return `
    <button
      type="button"
      class="chat-action-button"
      data-chat-refresh-index="${escapeAttribute(index)}"
      aria-label="刷新当前回复"
      title="刷新当前回复"
    >
      <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false">
        <path d="M8 2.5a5.5 5.5 0 1 0 5.19 7.32.5.5 0 0 1 .94.34A6.5 6.5 0 1 1 8 1.5V0l3 2.5-3 2.5z"></path>
      </svg>
    </button>
  `;
}

function renderChatCopyButton(index) {
  return `
    <button
      type="button"
      class="chat-action-button"
      data-chat-copy-index="${escapeAttribute(index)}"
      aria-label="复制此条对话"
      title="复制此条对话"
    >
      <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false">
        <path d="M5.5 2.25A1.25 1.25 0 0 0 4.25 3.5v7A1.25 1.25 0 0 0 5.5 11.75h6A1.25 1.25 0 0 0 12.75 10.5v-7A1.25 1.25 0 0 0 11.5 2.25z"></path>
        <path d="M2.75 5.5A1.25 1.25 0 0 1 4 4.25h.75v1H4A.25.25 0 0 0 3.75 5.5v6A1.25 1.25 0 0 0 5 12.75h5.25a.25.25 0 0 0 .25-.25v-.75h1v.75A1.25 1.25 0 0 1 10.25 13.75H5A2.25 2.25 0 0 1 2.75 11.5z"></path>
      </svg>
    </button>
  `;
}

function renderChatMessageContent(message) {
  const optionContent = message.role === 'assistant' ? extractChatOptionContent(message.text) : null;
  if (optionContent) {
    return renderChatOptionMessage(optionContent);
  }
  return `<div class="chat-markdown">${renderMarkdown(message.text || (message.streaming ? '思考中…' : ''))}</div>`;
}

function renderChatOptionMessage(content) {
  const blocks = [];
  if (content.beforeText) {
    blocks.push(`<div class="chat-markdown">${renderMarkdown(content.beforeText)}</div>`);
  }
  blocks.push(renderChatOptions(content.payload));
  if (content.afterText) {
    blocks.push(`<div class="chat-markdown">${renderMarkdown(content.afterText)}</div>`);
  }
  return `<div class="chat-option-message">${blocks.join('')}</div>`;
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
  const optionContent = extractChatOptionContent(source);
  return optionContent ? optionContent.payload : null;
}

function extractChatOptionContent(source) {
  const text = String(source ?? '').replace(/\r\n?/g, '\n');
  const trimmed = text.trim();
  if (!trimmed) return null;

  const directPayload = parseChatOptionsPayloadCandidate(trimmed);
  if (directPayload) {
    return { payload: directPayload, beforeText: '', afterText: '' };
  }

  const fencedPayload = extractChatOptionContentFromFencedBlocks(text);
  if (fencedPayload) return fencedPayload;

  return extractChatOptionContentFromEmbeddedObject(text);
}

function parseChatOptionsPayloadCandidate(text) {
  const candidate = String(text ?? '').trim();
  if (!candidate.startsWith('{') || !candidate.endsWith('}')) return null;

  const jsonPayload = parseJSONChatOptionsPayload(candidate);
  if (jsonPayload) return jsonPayload;

  const ednPayload = parseEDNChatOptionsPayload(candidate);
  if (ednPayload) return ednPayload;

  return parseAskUserInputChatOptionsPayload(candidate);
}

function extractChatOptionContentFromFencedBlocks(text) {
  const fencePattern = /```(?:[\w+-]+)?\s*\n([\s\S]*?)\n```/g;
  for (const match of text.matchAll(fencePattern)) {
    const candidate = parseChatOptionsPayloadCandidate(match[1]);
    if (!candidate) continue;
    return {
      payload: candidate,
      beforeText: normalizeChatOptionContextText(text.slice(0, match.index)),
      afterText: normalizeChatOptionContextText(text.slice((match.index || 0) + match[0].length)),
    };
  }
  return null;
}

function extractChatOptionContentFromEmbeddedObject(text) {
  const segments = findBraceDelimitedSegments(text);
  for (const segment of segments) {
    const candidate = parseChatOptionsPayloadCandidate(segment.text);
    if (!candidate) continue;
    return {
      payload: candidate,
      beforeText: normalizeChatOptionContextText(text.slice(0, segment.start)),
      afterText: normalizeChatOptionContextText(text.slice(segment.end)),
    };
  }
  return null;
}

function normalizeChatOptionContextText(text) {
  let normalized = String(text ?? '').replace(/\r\n?/g, '\n');
  if (!normalized.trim()) return '';

  normalized = normalized.replace(
    /<details[^>]*>\s*<summary>([\s\S]*?)<\/summary>/gi,
    (_match, summary) => `**${stripChatOptionHTML(summary).trim()}**\n\n`,
  );
  normalized = normalized.replace(/<\/details>/gi, '\n');
  normalized = normalized.replace(/<br\s*\/?>/gi, '\n');
  normalized = normalized.replace(/<\/(p|div|section|article|li|ul|ol)>/gi, '\n');
  normalized = normalized.replace(/<(p|div|section|article|li|ul|ol)[^>]*>/gi, '');
  normalized = stripChatOptionHTML(normalized);
  normalized = normalized.replace(/[ \t]+\n/g, '\n');
  normalized = normalized.replace(/\n{3,}/g, '\n\n');
  return normalized.trim();
}

function stripChatOptionHTML(text) {
  return String(text ?? '').replace(/<[^>]+>/g, '');
}

function findBraceDelimitedSegments(text) {
  const segments = [];
  let depth = 0;
  let start = -1;
  let inString = false;
  let escape = false;

  for (let index = 0; index < text.length; index += 1) {
    const char = text[index];
    if (escape) {
      escape = false;
      continue;
    }
    if (char === '\\') {
      escape = true;
      continue;
    }
    if (char === '"') {
      inString = !inString;
      continue;
    }
    if (inString) continue;
    if (char === '{') {
      if (depth === 0) start = index;
      depth += 1;
      continue;
    }
    if (char === '}') {
      if (depth === 0) continue;
      depth -= 1;
      if (depth === 0 && start >= 0) {
        segments.push({
          start,
          end: index + 1,
          text: text.slice(start, index + 1),
        });
        start = -1;
      }
    }
  }

  return segments;
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

function parseAskUserInputChatOptionsPayload(text) {
  const inputTypeMatch = text.match(/\bask_user_input\s*:\s*([A-Za-z_][\w-]*)/i);
  if (!inputTypeMatch) return null;

  const inputType = normalizeChatOptionScalar(inputTypeMatch[1]).toLowerCase();
  if (inputType && inputType !== 'single_select' && inputType !== 'singleselect') {
    return null;
  }

  const questionMatch = text.match(/\bquestion\s*:\s*"((?:\\.|[^"])*)"/s);
  const optionsMatch = text.match(/\boptions\s*:\s*\[((?:.|\n)*)\]/s);
  if (!questionMatch || !optionsMatch) return null;

  const question = unescapeChatOptionText(questionMatch[1]).trim();
  const options = Array.from(optionsMatch[1].matchAll(/"((?:\\.|[^"])*)"/g))
    .map((item) => unescapeChatOptionText(item[1]).trim())
    .filter(Boolean);

  return normalizeChatOptionsPayload({
    question,
    questiontype: 'singleselect',
    options,
  });
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

function buildChatCopyText(message) {
  const optionContent = message.role === 'assistant' ? extractChatOptionContent(message.text) : null;
  if (!optionContent) {
    return String(message.text || (message.streaming ? '思考中…' : '')).trim();
  }

  const parts = [];
  if (optionContent.beforeText) {
    parts.push(optionContent.beforeText.trim());
  }
  parts.push(optionContent.payload.question.trim());
  parts.push(optionContent.payload.options.map((option) => `- ${option.label}`).join('\n'));
  if (optionContent.afterText) {
    parts.push(optionContent.afterText.trim());
  }
  return parts.filter(Boolean).join('\n\n').trim();
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
        <p>点击上方新建对话，或输入 <code>/new</code></p>
      </div>
    `;
    return;
  }

  container.innerHTML = conversations
    .map((conversation) => `
      <div
        class="chat-session-row ${conversation.active ? 'active' : ''} ${state.chatSessionContextMenu.open && state.chatSessionContextMenu.sessionId === conversation.sessionId ? 'context-open' : ''}"
        data-chat-session-row="${escapeAttribute(conversation.sessionId)}"
        draggable="true"
      >
        <button
          type="button"
          class="chat-session-item ${conversation.active ? 'active' : ''}"
          data-chat-session="${escapeAttribute(conversation.sessionId)}"
          title="${escapeAttribute([
            conversation.sourceLabel || '',
            conversation.preview || '',
            conversation.readOnly ? '只读会话' : '可继续对话',
          ].filter(Boolean).join('\n'))}"
        >
          <span class="chat-session-title">${conversation.sourceLabel ? `[${escapeHTML(conversation.sourceLabel)}] ` : ''}${escapeHTML(conversation.title || '新对话')}</span>
        </button>
      </div>
    `)
    .join('');
}

function applyChatState(nextState) {
  const previousSessionId = state.chatState.sessionId || '';
  state.chatState = normalizeChatState(nextState);
  state.chatState = {
    ...state.chatState,
    conversations: reconcileChatSessionOrder(state.chatState.conversations),
  };
  if (
    state.chatSessionContextMenu.open
    && !state.chatState.conversations.some((item) => item.sessionId === state.chatSessionContextMenu.sessionId)
  ) {
    state.chatSessionContextMenu = defaultChatSessionContextMenuState();
    renderChatSessionContextMenu();
  }
  const incomingMessages = (state.chatState.messages || []).map((message) => ({
    role: message.role,
    text: message.text,
    time: message.time || '',
    usage: normalizeTokenUsage(message.usage),
    process: normalizeChatProcess(message.process),
  }));
  state.chat = previousSessionId && previousSessionId === state.chatState.sessionId
    ? mergeTransientChatMessages(incomingMessages)
    : incomingMessages;
  renderChatSessions();
  renderChatContext();
  renderChat();
}

function mergeTransientChatMessages(incomingMessages) {
  const localMessages = Array.isArray(state.chat) ? state.chat : [];
  if (!localMessages.some((message) => message?.transient)) {
    return incomingMessages;
  }

  const merged = [];
  let incomingIndex = 0;
  for (const localMessage of localMessages) {
    if (localMessage?.transient) {
      merged.push({
        role: localMessage.role,
        text: localMessage.text,
        time: localMessage.time || '',
        usage: normalizeTokenUsage(localMessage.usage),
        process: normalizeChatProcess(localMessage.process),
        transient: true,
      });
      continue;
    }
    if (incomingIndex < incomingMessages.length) {
      merged.push({ ...incomingMessages[incomingIndex] });
      incomingIndex += 1;
    }
  }
  while (incomingIndex < incomingMessages.length) {
    merged.push({ ...incomingMessages[incomingIndex] });
    incomingIndex += 1;
  }
  return merged;
}

function syncCurrentChatConversationFromMessages() {
  const sessionId = state.chatState.sessionId || '';
  if (!sessionId) return;

  const conversations = Array.isArray(state.chatState.conversations) ? [...state.chatState.conversations] : [];
  const currentIndex = conversations.findIndex((item) => item.sessionId === sessionId);
  const nextConversation = {
    sessionId,
    title: summarizeChatTitle(state.chat),
    customTitle: Boolean(currentIndex >= 0 && conversations[currentIndex]?.customTitle),
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
    if (conversations[currentIndex]?.customTitle) {
      nextConversation.title = conversations[currentIndex].title || nextConversation.title;
    }
    nextConversations[currentIndex] = nextConversation;
  } else {
    nextConversations.push(nextConversation);
  }

  state.chatState = {
    ...state.chatState,
    sessionId,
    conversations: reconcileChatSessionOrder(nextConversations),
    messages: state.chat.map((message) => ({
      role: message.role,
      text: message.text,
      time: message.time || '',
      usage: normalizeTokenUsage(message.usage),
      process: normalizeChatProcess(message.process),
    })),
  };
  renderChatSessions();
}

function normalizeProjectStorageKey(project) {
  const value = String(project || 'default').trim().toLowerCase();
  return value || 'default';
}

function chatSessionOrderStorageKey(project = state.projectState.activeProject) {
  return `baize-chat-session-order:${normalizeProjectStorageKey(project)}`;
}

function loadChatSessionOrder(project = state.projectState.activeProject) {
  try {
    const raw = localStorage.getItem(chatSessionOrderStorageKey(project));
    const parsed = JSON.parse(raw || '[]');
    return Array.isArray(parsed)
      ? parsed.map((item) => String(item || '').trim()).filter(Boolean)
      : [];
  } catch (_error) {
    return [];
  }
}

function saveChatSessionOrder(sessionIds, project = state.projectState.activeProject) {
  const next = [];
  const seen = new Set();
  for (const sessionId of sessionIds || []) {
    const value = String(sessionId || '').trim();
    if (!value || seen.has(value)) continue;
    seen.add(value);
    next.push(value);
  }

  try {
    localStorage.setItem(chatSessionOrderStorageKey(project), JSON.stringify(next));
  } catch (_error) {
    // Ignore local persistence failures and keep the in-memory order.
  }
  return next;
}

function sameStringArray(left, right) {
  if (left.length !== right.length) return false;
  for (let index = 0; index < left.length; index += 1) {
    if (left[index] !== right[index]) return false;
  }
  return true;
}

function reconcileChatSessionOrder(conversations, project = state.projectState.activeProject) {
  const list = Array.isArray(conversations)
    ? conversations.filter((item) => item?.sessionId)
    : [];
  if (list.length === 0) {
    saveChatSessionOrder([], project);
    return [];
  }

  const stored = loadChatSessionOrder(project);
  if (stored.length === 0) {
    saveChatSessionOrder(list.map((item) => item.sessionId), project);
    return list;
  }

  const byID = new Map(list.map((item) => [item.sessionId, item]));
  const ordered = [];
  for (const sessionId of stored) {
    const item = byID.get(sessionId);
    if (!item) continue;
    ordered.push(item);
    byID.delete(sessionId);
  }
  for (const item of list) {
    if (!byID.has(item.sessionId)) continue;
    ordered.push(item);
    byID.delete(item.sessionId);
  }

  const mergedOrder = ordered.map((item) => item.sessionId);
  if (!sameStringArray(stored, mergedOrder)) {
    saveChatSessionOrder(mergedOrder, project);
  }
  return ordered;
}

function reorderChatSessions(sourceSessionId, targetSessionId, placeBefore) {
  const conversations = Array.isArray(state.chatState.conversations) ? [...state.chatState.conversations] : [];
  const sourceIndex = conversations.findIndex((item) => item.sessionId === sourceSessionId);
  const targetIndex = conversations.findIndex((item) => item.sessionId === targetSessionId);
  if (sourceIndex < 0 || targetIndex < 0 || sourceIndex === targetIndex) return;

  const [source] = conversations.splice(sourceIndex, 1);
  let insertIndex = conversations.findIndex((item) => item.sessionId === targetSessionId);
  if (insertIndex < 0) {
    conversations.push(source);
  } else {
    if (!placeBefore) insertIndex += 1;
    conversations.splice(insertIndex, 0, source);
  }

  state.chatState = {
    ...state.chatState,
    conversations,
  };
  saveChatSessionOrder(conversations.map((item) => item.sessionId));
  renderChatSessions();
}
