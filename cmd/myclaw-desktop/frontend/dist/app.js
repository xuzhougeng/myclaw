window.__BAIZE_DESKTOP_BUILD_MODE__ = "release";
window.__BAIZE_DESKTOP_DEBUG_DIAGNOSTICS__ = false;

/* Source: js/core/navigation.js */
const NAV_VIEW_ALIASES = {
  model: { view: 'settings', sectionId: 'settings-section-model' },
  weixin: { view: 'settings', sectionId: 'settings-section-weixin' },
};

// Global navigation function
window.navigateTo = function(viewName, sectionId) {
  const alias = NAV_VIEW_ALIASES[viewName] || null;
  const normalizedView = alias?.view || viewName;
  const normalizedSectionId = sectionId || alias?.sectionId || '';

  // Update nav items
  document.querySelectorAll('.nav-item').forEach(item => {
    item.classList.remove('active');
    if (item.dataset.view === normalizedView) {
      item.classList.add('active');
    }
  });

  // Update views
  document.querySelectorAll('.view').forEach(view => {
    view.classList.remove('active');
  });
  const targetView = document.getElementById(`view-${normalizedView}`);
  if (targetView) {
    targetView.classList.add('active');
  }

  if (normalizedView === 'reminders' && state.backend) {
    void refreshReminders().catch((error) => {
      showBanner(asMessage(error), true);
    });
  }

  if (normalizedView === 'memory' && state.backend) {
    void Promise.all([refreshKnowledge(), refreshProjectState(), refreshOverview()]).catch((error) => {
      showBanner(asMessage(error), true);
    });
  }

  if (normalizedView === 'tools' && state.backend) {
    void refreshTools().catch((error) => {
      showBanner(asMessage(error), true);
    });
  }

  if (targetView && normalizedSectionId) {
    const targetSection = document.getElementById(normalizedSectionId);
    if (targetSection && targetView.contains(targetSection)) {
      requestAnimationFrame(() => {
        targetSection.scrollIntoView({ block: 'start', behavior: 'smooth' });
      });
    }
  }
};

// Theme management
function initTheme() {
  const saved = localStorage.getItem('baize-theme');
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
  const theme = saved || (prefersDark ? 'dark' : 'light');
  document.documentElement.setAttribute('data-theme', theme);
  updateThemeIcon(theme);
}

function toggleTheme() {
  const current = document.documentElement.getAttribute('data-theme') || 'dark';
  const next = current === 'dark' ? 'light' : 'dark';
  document.documentElement.setAttribute('data-theme', next);
  localStorage.setItem('baize-theme', next);
  updateThemeIcon(next);
}

function updateThemeIcon(theme) {
  const icon = document.querySelector('.theme-icon');
  if (icon) {
    icon.textContent = theme === 'dark' ? '◐' : '◑';
  }
}


/* Source: js/shared/state-models.js */
function defaultChatSessionContextMenuState() {
  return {
    open: false,
    sessionId: '',
    x: 0,
    y: 0,
  };
}

function openChatSessionContextMenu(sessionId, x, y) {
  const conversation = (state.chatState.conversations || []).find((item) => item.sessionId === String(sessionId || '').trim());
  if (conversation?.readOnly) return;
  state.chatSessionContextMenu = {
    open: true,
    sessionId: String(sessionId || '').trim(),
    x: Number(x || 0),
    y: Number(y || 0),
  };
  renderChatSessionContextMenu();
  renderChatSessions();
}

function closeChatSessionContextMenu() {
  if (!state.chatSessionContextMenu.open) return;
  state.chatSessionContextMenu = defaultChatSessionContextMenuState();
  renderChatSessionContextMenu();
  renderChatSessions();
}

function renderChatSessionContextMenu() {
  const menu = document.getElementById('chat-session-context-menu');
  if (!menu) return;

  if (!state.chatSessionContextMenu.open) {
    menu.hidden = true;
    menu.style.removeProperty('left');
    menu.style.removeProperty('top');
    return;
  }

  menu.hidden = false;
  menu.style.left = `${state.chatSessionContextMenu.x}px`;
  menu.style.top = `${state.chatSessionContextMenu.y}px`;

  requestAnimationFrame(() => {
    if (menu.hidden) return;
    const padding = 12;
    const maxLeft = Math.max(padding, window.innerWidth - menu.offsetWidth - padding);
    const maxTop = Math.max(padding, window.innerHeight - menu.offsetHeight - padding);
    menu.style.left = `${Math.min(Math.max(state.chatSessionContextMenu.x, padding), maxLeft)}px`;
    menu.style.top = `${Math.min(Math.max(state.chatSessionContextMenu.y, padding), maxTop)}px`;
  });
}

function currentChatConversation() {
  const sessionId = state.chatState.sessionId || '';
  return (state.chatState.conversations || []).find((item) => item.sessionId === sessionId) || null;
}

function defaultChatSessionDragState() {
  return {
    sessionId: '',
    targetSessionId: '',
    placeBefore: true,
    suppressClickUntil: 0,
  };
}

function clearChatSessionDropIndicators() {
  document.querySelectorAll('.chat-session-row.dragging, .chat-session-row.drop-before, .chat-session-row.drop-after')
    .forEach((row) => {
      row.classList.remove('dragging', 'drop-before', 'drop-after');
    });
}

function summarizeChatTitle(messages) {
  const firstUser = (messages || []).find((item) => item.role === 'user' && (item.text || '').trim());
  return truncateText(firstUser?.text || '新对话', 28);
}

function summarizeChatPreview(messages) {
  for (let index = (messages || []).length - 1; index >= 0; index -= 1) {
    const message = messages[index] || {};
    const optionContent = extractChatOptionContent(message.text);
    const text = (
      optionContent?.beforeText
      || optionContent?.payload?.question
      || optionContent?.afterText
      || message.text
      || ''
    ).trim();
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

function defaultChatSessionDialogState() {
  return {
    open: false,
    mode: '',
    sessionId: '',
    itemId: '',
    initialTitle: '',
    selectedMode: 'agent',
    restoreFocus: null,
  };
}

function normalizeTokenUsage(payload) {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return null;
  const inputTokens = Number(payload.inputTokens || 0);
  const outputTokens = Number(payload.outputTokens || 0);
  const cachedTokens = Number(payload.cachedTokens || 0);
  const totalTokens = Number(payload.totalTokens || inputTokens + outputTokens);
  if (inputTokens <= 0 && outputTokens <= 0 && cachedTokens <= 0 && totalTokens <= 0) {
    return null;
  }
  return {
    inputTokens,
    outputTokens,
    cachedTokens,
    totalTokens,
  };
}

function formatTokenUsage(usage) {
  const value = normalizeTokenUsage(usage);
  if (!value) return '';
  return `input ${value.inputTokens} · output ${value.outputTokens} · cached ${value.cachedTokens} · total ${value.totalTokens}`;
}

function normalizeChatProcess(payload) {
  if (!Array.isArray(payload)) return [];
  return payload
    .map((item) => ({
      title: String(item?.title || '').trim(),
      detail: String(item?.detail || '').trim(),
    }))
    .filter((item) => item.title || item.detail);
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
      customTitle: Boolean(item?.customTitle),
      preview: item?.preview || '',
      source: item?.source || '',
      sourceLabel: item?.sourceLabel || '',
      mode: item?.mode === 'ask' ? 'ask' : item?.mode === 'agent' ? 'agent' : '',
      readOnly: Boolean(item?.readOnly),
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
      usage: normalizeTokenUsage(item?.usage),
      process: normalizeChatProcess(item?.process),
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
    source: item.source || '',
    sourceLabel: item.sourceLabel || '',
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

const DEFAULT_MODEL_REQUEST_TIMEOUT_SECONDS = 90;

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
    effectiveRequestTimeoutSeconds: null,
    effectiveMaxOutputTokensText: null,
    effectiveMaxOutputTokensJSON: null,
    effectiveTemperature: null,
    effectiveTopP: null,
    effectiveFrequencyPenalty: null,
    effectivePresencePenalty: null,
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
    ? stateValue.profiles.map((item) => {
        const legacyMaxOutputTokens = normalizeOptionalNumber(item.maxOutputTokens);
        return {
          id: item.id || '',
          name: item.name || '',
          provider: item.provider || 'openai',
          apiType: item.apiType || 'responses',
          baseUrl: item.baseUrl || MODEL_PROVIDER_DEFAULTS[item.provider || 'openai']?.baseUrl || '',
          model: item.model || '',
          requestTimeoutSeconds: normalizeOptionalNumber(item.requestTimeoutSeconds),
          hasApiKey: Boolean(item.hasApiKey),
          apiKeyMasked: item.apiKeyMasked || (item.hasApiKey ? '********' : '(empty)'),
          active: Boolean(item.active),
          maxOutputTokensText: coalesceOptionalNumber(item.maxOutputTokensText, legacyMaxOutputTokens),
          maxOutputTokensJSON: coalesceOptionalNumber(item.maxOutputTokensJSON, legacyMaxOutputTokens),
          maxOutputTokens: legacyMaxOutputTokens,
          temperature: normalizeOptionalNumber(item.temperature),
          topP: normalizeOptionalNumber(item.topP),
          frequencyPenalty: normalizeOptionalNumber(item.frequencyPenalty),
          presencePenalty: normalizeOptionalNumber(item.presencePenalty),
        };
      })
    : [];
  stateValue.effectiveMaxOutputTokensText = coalesceOptionalNumber(stateValue.effectiveMaxOutputTokensText, stateValue.effectiveMaxOutputTokens);
  stateValue.effectiveMaxOutputTokensJSON = coalesceOptionalNumber(stateValue.effectiveMaxOutputTokensJSON, stateValue.effectiveMaxOutputTokens);
  stateValue.effectiveRequestTimeoutSeconds = normalizeOptionalNumber(stateValue.effectiveRequestTimeoutSeconds);
  stateValue.effectiveTemperature = normalizeOptionalNumber(stateValue.effectiveTemperature);
  stateValue.effectiveTopP = normalizeOptionalNumber(stateValue.effectiveTopP);
  stateValue.effectiveFrequencyPenalty = normalizeOptionalNumber(stateValue.effectiveFrequencyPenalty);
  stateValue.effectivePresencePenalty = normalizeOptionalNumber(stateValue.effectivePresencePenalty);
  return stateValue;
}

function defaultSettingsState() {
  return {
    weixinHistoryMessages: 12,
    weixinHistoryRunes: 360,
    weixinEverythingPath: '',
    disabledToolNames: [],
    screenTraceEnabled: false,
    screenTraceIntervalSeconds: 15,
    screenTraceRetentionDays: 7,
    screenTraceVisionProfileId: '',
    screenTraceWriteDigestsToKb: false,
  };
}

function normalizeSettingsState(payload) {
  const source = Array.isArray(payload) ? payload[0] : payload;
  const disabledToolNames = Array.isArray(source?.disabledToolNames)
    ? source.disabledToolNames
      .map((item) => String(item || '').trim().toLowerCase())
      .filter(Boolean)
      .filter((item, index, items) => items.indexOf(item) === index)
      .sort((left, right) => left.localeCompare(right, 'en'))
    : [];
  return {
    ...defaultSettingsState(),
    ...(source || {}),
    weixinHistoryMessages: Number(source?.weixinHistoryMessages ?? 12),
    weixinHistoryRunes: Number(source?.weixinHistoryRunes ?? 360),
    weixinEverythingPath: String(source?.weixinEverythingPath ?? ''),
    screenTraceEnabled: Boolean(source?.screenTraceEnabled),
    screenTraceIntervalSeconds: Number(source?.screenTraceIntervalSeconds ?? 15),
    screenTraceRetentionDays: Number(source?.screenTraceRetentionDays ?? 7),
    screenTraceVisionProfileId: String(source?.screenTraceVisionProfileId ?? ''),
    screenTraceWriteDigestsToKb: Boolean(source?.screenTraceWriteDigestsToKb),
    disabledToolNames,
  };
}

function defaultScreenTraceStatus() {
  return {
    enabled: false,
    running: false,
    intervalSeconds: 15,
    retentionDays: 7,
    visionProfileId: '',
    writeDigestsToKb: false,
    lastCaptureAt: '',
    lastCaptureAtUnix: 0,
    lastAnalysisAt: '',
    lastAnalysisAtUnix: 0,
    lastDigestAt: '',
    lastDigestAtUnix: 0,
    lastError: '',
    lastImagePath: '',
    totalRecords: 0,
    skippedDuplicates: 0,
  };
}

function normalizeScreenTraceStatus(payload) {
  const source = Array.isArray(payload) ? payload[0] : payload;
  return {
    ...defaultScreenTraceStatus(),
    ...(source || {}),
    enabled: Boolean(source?.enabled),
    running: Boolean(source?.running),
    intervalSeconds: Number(source?.intervalSeconds ?? 15),
    retentionDays: Number(source?.retentionDays ?? 7),
    visionProfileId: String(source?.visionProfileId ?? ''),
    writeDigestsToKb: Boolean(source?.writeDigestsToKb),
    lastCaptureAtUnix: Number(source?.lastCaptureAtUnix || 0),
    lastAnalysisAtUnix: Number(source?.lastAnalysisAtUnix || 0),
    lastDigestAtUnix: Number(source?.lastDigestAtUnix || 0),
    totalRecords: Number(source?.totalRecords || 0),
    skippedDuplicates: Number(source?.skippedDuplicates || 0),
  };
}

function normalizeScreenTraceRecords(payload) {
  if (!Array.isArray(payload)) return [];
  return payload.map((item) => ({
    id: item?.id || '',
    shortId: item?.shortId || '',
    capturedAt: item?.capturedAt || '',
    capturedAtUnix: Number(item?.capturedAtUnix || 0),
    imagePath: item?.imagePath || '',
    sceneSummary: item?.sceneSummary || '',
    visibleText: Array.isArray(item?.visibleText) ? item.visibleText.map((v) => String(v || '').trim()).filter(Boolean) : [],
    apps: Array.isArray(item?.apps) ? item.apps.map((v) => String(v || '').trim()).filter(Boolean) : [],
    taskGuess: item?.taskGuess || '',
    keywords: Array.isArray(item?.keywords) ? item.keywords.map((v) => String(v || '').trim()).filter(Boolean) : [],
    sensitiveLevel: item?.sensitiveLevel || '',
    confidence: Number(item?.confidence || 0),
    displayLabel: item?.displayLabel || '',
    dimensionsLabel: item?.dimensionsLabel || '',
  }));
}

function normalizeScreenTraceDigests(payload) {
  if (!Array.isArray(payload)) return [];
  return payload.map((item) => ({
    id: item?.id || '',
    shortId: item?.shortId || '',
    bucketStart: item?.bucketStart || '',
    bucketStartUnix: Number(item?.bucketStartUnix || 0),
    bucketEnd: item?.bucketEnd || '',
    bucketEndUnix: Number(item?.bucketEndUnix || 0),
    recordCount: Number(item?.recordCount || 0),
    summary: item?.summary || '',
    keywords: Array.isArray(item?.keywords) ? item.keywords.map((v) => String(v || '').trim()).filter(Boolean) : [],
    dominantApps: Array.isArray(item?.dominantApps) ? item.dominantApps.map((v) => String(v || '').trim()).filter(Boolean) : [],
    dominantTasks: Array.isArray(item?.dominantTasks) ? item.dominantTasks.map((v) => String(v || '').trim()).filter(Boolean) : [],
    writtenToKb: Boolean(item?.writtenToKb),
    knowledgeEntryId: item?.knowledgeEntryId || '',
  }));
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


/* Source: js/shared/utils.js */
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

async function openExternalLink(targetURL) {
  try {
    await state.backend.OpenExternalURL(targetURL);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
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

function sanitizeHTTPURL(value) {
  const url = String(value ?? '').trim();
  if (!url) return '';

  try {
    const parsed = new URL(url, window.location.href);
    const protocol = parsed.protocol.toLowerCase();
    if (protocol === 'http:' || protocol === 'https:') {
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


/* Source: js/core/state.js */
const TOOL_GROUP_COLLAPSE_STORAGE_KEY = 'baize-tool-group-collapsed';

function loadToolGroupCollapseState() {
  try {
    const raw = localStorage.getItem(TOOL_GROUP_COLLAPSE_STORAGE_KEY);
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {};
    }
    return parsed;
  } catch (_error) {
    return {};
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
  tools: [],
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
  modelFormDirty: false,
  weixin: defaultWeixinState(),
  settings: defaultSettingsState(),
  screenTraceStatus: defaultScreenTraceStatus(),
  screenTraceRecords: [],
  screenTraceDigests: [],
  screenTraceCapturePending: false,
  chat: [],
  chatSidebarCollapsed: false,
  chatSessionDialog: defaultChatSessionDialogState(),
  chatSessionContextMenu: defaultChatSessionContextMenuState(),
  chatSessionDrag: defaultChatSessionDragState(),
  chatStreaming: false,
  chatStreamHandlers: {},
  toolGroupCollapsed: loadToolGroupCollapseState(),
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
  { label: '/kb', insert: '/kb', description: '查看当前知识库和可用知识库' },
  { label: '/kb new', insert: '/kb new ', description: '新建并切换到一个知识库' },
  { label: '/kb switch', insert: '/kb switch ', description: '切换当前知识库' },
  { label: '/kb remember', insert: '/kb remember ', description: '保存一条知识' },
  { label: '/kb remember-file', insert: '/kb remember-file ', description: '总结图片或 PDF 并写入知识库' },
  { label: '/kb append', insert: '/kb append ', description: '追加到已有知识' },
  { label: '/kb forget', insert: '/kb forget ', description: '删除一条知识' },
  { label: '/kb list', insert: '/kb list', description: '查看全部知识' },
  { label: '/kb stats', insert: '/kb stats', description: '查看知识库状态' },
  { label: '/kb clear', insert: '/kb clear', description: '清空知识库' },
  { label: '/skill', insert: '/skill', description: '查看当前会话已加载技能' },
  { label: '/skill list', insert: '/skill list', description: '查看可用技能和加载状态' },
  { label: '/skill show', insert: '/skill show ', description: '查看某个技能内容' },
  { label: '/skill load', insert: '/skill load ', description: '为当前会话加载一个技能' },
  { label: '/skill unload', insert: '/skill unload ', description: '从当前会话卸载一个技能' },
  { label: '/skill clear', insert: '/skill clear', description: '清空当前会话已加载技能' },
  { label: '/prompt', insert: '/prompt', description: '查看当前 Prompt profile' },
  { label: '/prompt list', insert: '/prompt list', description: '查看可用 Prompt profiles' },
  { label: '/prompt use', insert: '/prompt use ', description: '为当前会话启用 Prompt profile' },
  { label: '/prompt clear', insert: '/prompt clear', description: '清除当前会话 Prompt profile' },
  { label: '/translate', insert: '/translate ', description: '翻译成中文' },
  { label: '/debug-search', insert: '/debug-search ', description: '查看关键词检索和候选复核过程' },
  { label: '/notice', insert: '/notice ', description: '创建提醒' },
  { label: '/notice list', insert: '/notice list', description: '查看提醒列表' },
  { label: '/notice remove', insert: '/notice remove ', description: '删除提醒' },
  { label: '/cron', insert: '/cron ', description: '与 /notice 等价' },
];


/* Source: js/views/chat.js */
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


/* Source: js/views/library.js */
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
              ${item.sourceLabel ? `<span class="memory-badge source">${escapeHTML(item.sourceLabel)}</span>` : ''}
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

function normalizeTools(payload) {
  if (!Array.isArray(payload)) return [];
  return payload.map((item) => ({
    name: item.name || '',
    shortName: item.shortName || '',
    familyKey: item.familyKey || '',
    familyTitle: item.familyTitle || '',
    title: item.title || item.shortName || item.name || '',
    description: item.description || item.purpose || '',
    purpose: item.purpose || '',
    provider: item.provider || '',
    providerKind: item.providerKind || '',
    sideEffectLevel: item.sideEffectLevel || '',
    status: item.status || '已就绪',
    statusTone: item.statusTone || 'on',
    enabled: item.enabled !== false,
    toggleable: item.toggleable !== false,
    configurable: Boolean(item.configurable),
    configValue: item.configValue || '',
  }));
}

function formatProviderLabel(provider, providerKind) {
  const parts = [provider, providerKind]
    .map((item) => String(item || '').trim())
    .filter(Boolean);
  if (parts.length === 0) return 'local';
  if (parts.length === 2 && parts[0].toLowerCase() === parts[1].toLowerCase()) {
    return parts[0];
  }
  return parts.join(' / ');
}

function toolGroupKey(tool) {
  const provider = String(tool.provider || '').trim().toLowerCase();
  const familyKey = String(tool.familyKey || '').trim().toLowerCase();
  if (familyKey) {
    return `family:${provider}:${familyKey}`;
  }
  return `tool:${String(tool.name || '').trim().toLowerCase()}`;
}

function groupTools(tools) {
  const groups = [];
  const index = new Map();

  (tools || []).forEach((tool) => {
    const key = toolGroupKey(tool);
    const existing = index.get(key);
    if (existing) {
      existing.items.push(tool);
      return;
    }
    const next = {
      key,
      familyKey: String(tool.familyKey || '').trim(),
      familyTitle: String(tool.familyTitle || '').trim(),
      provider: String(tool.provider || '').trim(),
      providerKind: String(tool.providerKind || '').trim(),
      items: [tool],
    };
    index.set(key, next);
    groups.push(next);
  });

  return groups;
}

function toolSideEffectLabel(level) {
  switch ((level || '').toLowerCase()) {
    case 'read_only':
      return '只读';
    case 'soft_write':
      return '软写入';
    case 'destructive':
      return '删除/变更';
    default:
      return level || '未标注';
  }
}

function persistToolGroupCollapseState() {
  try {
    localStorage.setItem(TOOL_GROUP_COLLAPSE_STORAGE_KEY, JSON.stringify(state.toolGroupCollapsed || {}));
  } catch (_error) {
    // Ignore storage failures and keep collapse state in memory.
  }
}

function isToolGroupCollapsed(groupKey) {
  return Boolean(state.toolGroupCollapsed?.[groupKey]);
}

function toggleToolGroupCollapse(groupKey) {
  const key = String(groupKey || '').trim();
  if (!key) return;
  state.toolGroupCollapsed = {
    ...(state.toolGroupCollapsed || {}),
    [key]: !isToolGroupCollapsed(key),
  };
  persistToolGroupCollapseState();
  renderTools();
}

function renderToolConfig(tool) {
  if (!tool.configurable || tool.shortName !== 'everything_file_search') {
    return '';
  }

  if (tool.statusTone === 'off') {
    return `
      <div class="tool-config">
        <div class="tool-card-note">
          这个工具当前只在 Windows 下可用，所以这里不显示 Everything 路径输入。
        </div>
      </div>
    `;
  }

  const value = tool.configValue || state.settings.weixinEverythingPath || '';
  return `
    <div class="tool-config">
      <label class="form-field form-field-wide">
        <span>Everything <code>es.exe</code> 路径</span>
        <input id="tool-everything-path" type="text" placeholder="例如: C:\\Program Files\\Everything\\es.exe 或 es.exe" value="${escapeAttribute(value)}" />
      </label>
      <div class="tool-card-note">
        这个配置同时供 agent 文件检索和 <code>/find</code> 使用；如果还没安装 ES，可以去
        <a href="https://www.voidtools.com/zh-cn/downloads" target="_blank" rel="noopener noreferrer">Everything 下载页</a>
        下载。
      </div>
      <div class="tool-card-note">
        <code>/find</code> 目前只在 Windows 下生效，desktop / terminal / 微信会复用同一个文件检索模块；微信发送文件仍然使用 <code>/send &lt;序号&gt;</code>。
      </div>
      <div class="tool-card-actions">
        <button class="btn btn-primary btn-sm" data-tool-action="save-everything-path">保存路径</button>
      </div>
    </div>
  `;
}

function renderToolToggle(tool) {
  if (!tool.toggleable) return '';
  const nextEnabled = !tool.enabled;
  return `
    <button
      class="btn btn-ghost btn-sm"
      data-tool-action="toggle-enabled"
      data-tool-name="${escapeAttribute(tool.name)}"
      data-tool-enabled="${escapeAttribute(nextEnabled ? 'true' : 'false')}"
    >
      ${tool.enabled ? '关闭工具' : '启用工具'}
    </button>
  `;
}

function renderStandaloneTool(tool) {
  const purpose = tool.purpose && tool.purpose !== tool.description
    ? `<p class="tool-card-purpose">${escapeHTML(tool.purpose)}</p>`
    : '';
  const providerLabel = formatProviderLabel(tool.provider, tool.providerKind);
  return `
    <article class="tool-card">
      <div class="tool-card-header">
        <div>
          <div class="tool-card-title-row">
            <h3>${escapeHTML(tool.title || tool.shortName || tool.name)}</h3>
            <span class="status-pill ${escapeAttribute(tool.statusTone || 'on')}">${escapeHTML(tool.status || '已就绪')}</span>
          </div>
          <div class="tool-card-name">${escapeHTML(tool.shortName || tool.name)}</div>
        </div>
        <div class="tool-card-tags">
          <span class="tool-meta-pill">${escapeHTML(toolSideEffectLabel(tool.sideEffectLevel))}</span>
          <span class="tool-meta-pill provider">${escapeHTML(providerLabel)}</span>
        </div>
      </div>
      <p class="tool-card-desc">${escapeHTML(tool.description || '这个工具还没有填写描述。')}</p>
      ${purpose}
      ${renderToolConfig(tool)}
      <div class="tool-card-actions">
        ${renderToolToggle(tool)}
      </div>
    </article>
  `;
}

function renderToolGroupItem(tool) {
  const purpose = tool.purpose && tool.purpose !== tool.description
    ? `<p class="tool-card-purpose">${escapeHTML(tool.purpose)}</p>`
    : '';
  return `
    <section class="tool-group-item">
      <div class="tool-group-item-header">
        <div>
          <div class="tool-group-item-title-row">
            <h4>${escapeHTML(tool.title || tool.shortName || tool.name)}</h4>
            <span class="status-pill ${escapeAttribute(tool.statusTone || 'on')}">${escapeHTML(tool.status || '已就绪')}</span>
          </div>
          <div class="tool-group-item-name">${escapeHTML(tool.shortName || tool.name)}</div>
        </div>
        <div class="tool-group-item-tags">
          <span class="tool-meta-pill">${escapeHTML(toolSideEffectLabel(tool.sideEffectLevel))}</span>
        </div>
      </div>
      <p class="tool-card-desc">${escapeHTML(tool.description || '这个工具还没有填写描述。')}</p>
      ${purpose}
      ${renderToolConfig(tool)}
      <div class="tool-card-actions">
        ${renderToolToggle(tool)}
      </div>
    </section>
  `;
}

function renderToolGroup(group) {
  const providerLabel = formatProviderLabel(group.provider, group.providerKind);
  const countLabel = `${group.items.length} 个工具`;
  const collapsed = isToolGroupCollapsed(group.key);
  return `
    <article class="tool-card tool-group-card ${collapsed ? 'is-collapsed' : ''}">
      <button
        type="button"
        class="tool-group-toggle"
        data-tool-action="toggle-group"
        data-tool-group-key="${escapeAttribute(group.key)}"
        aria-expanded="${escapeAttribute(collapsed ? 'false' : 'true')}"
      >
        <div class="tool-card-header">
          <div>
            <div class="tool-card-title-row">
              <h3>${escapeHTML(group.familyTitle || '工具组')}</h3>
            </div>
            <div class="tool-card-name">${escapeHTML(countLabel)}</div>
          </div>
          <div class="tool-card-tags tool-group-toggle-side">
            <span class="tool-meta-pill provider">${escapeHTML(providerLabel)}</span>
            <span class="tool-group-toggle-label">${collapsed ? '展开' : '折叠'}</span>
            <span class="tool-group-chevron" aria-hidden="true">▾</span>
          </div>
        </div>
      </button>
      <div class="tool-group-list">
        ${group.items.map((tool) => renderToolGroupItem(tool)).join('')}
      </div>
    </article>
  `;
}

function renderTools() {
  const container = document.getElementById('tool-list');
  if (!container) return;

  const tools = [...state.tools];
  if (tools.length === 0) {
    container.innerHTML = `
      <div class="empty-state">
        <div class="empty-state-icon">⌘</div>
        <h3>还没有可用工具</h3>
        <p>后端加载完成后，这里会展示当前 claw 的可调用能力。</p>
      </div>
    `;
    return;
  }

  container.innerHTML = groupTools(tools)
    .map((group) => {
      if (group.familyTitle) {
        return renderToolGroup(group);
      }
      return renderStandaloneTool(group.items[0]);
    })
    .join('');
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
  setOptional('model-max-output-tokens-text', coalesceOptionalNumber(source.maxOutputTokensText, source.maxOutputTokens));
  setOptional('model-max-output-tokens-json', coalesceOptionalNumber(source.maxOutputTokensJSON, source.maxOutputTokens));
  setOptional('model-request-timeout-seconds', source.requestTimeoutSeconds);
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
  state.modelFormDirty = !profile;
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
  state.modelFormDirty = true;
}

function readOptionalNumber(id) {
  const el = document.getElementById(id);
  if (!el) return null;
  const v = el.value.trim();
  if (v === '') return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

function normalizeOptionalNumber(value) {
  if (value == null || value === '') return null;
  const n = Number(value);
  return Number.isFinite(n) ? n : null;
}

function coalesceOptionalNumber(primary, fallback) {
  const normalizedPrimary = normalizeOptionalNumber(primary);
  if (normalizedPrimary != null) {
    return normalizedPrimary;
  }
  return normalizeOptionalNumber(fallback);
}

function formatEffectiveMaxOutputTokens(value, defaultValue) {
  const n = normalizeOptionalNumber(value);
  if (n != null && n > 0) {
    return String(n);
  }
  return `默认（${defaultValue}）`;
}

function formatEffectiveRequestTimeoutSeconds(value) {
  const n = normalizeOptionalNumber(value);
  if (n != null && n > 0) {
    return `${n} 秒`;
  }
  return `默认（${DEFAULT_MODEL_REQUEST_TIMEOUT_SECONDS} 秒）`;
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
    requestTimeoutSeconds: readOptionalNumber('model-request-timeout-seconds'),
    maxOutputTokensText: readOptionalNumber('model-max-output-tokens-text'),
    maxOutputTokensJSON: readOptionalNumber('model-max-output-tokens-json'),
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
    state.modelFormDirty = false;
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
    state.modelFormDirty = false;
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
    state.modelFormDirty = false;
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
  const effectiveRequestTimeoutSeconds = document.getElementById('effective-request-timeout-seconds');
  const effectiveMaxOutputTokensText = document.getElementById('effective-max-output-tokens-text');
  const effectiveMaxOutputTokensJSON = document.getElementById('effective-max-output-tokens-json');
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
  if (effectiveRequestTimeoutSeconds) effectiveRequestTimeoutSeconds.textContent = formatEffectiveRequestTimeoutSeconds(state.model.effectiveRequestTimeoutSeconds);
  if (effectiveMaxOutputTokensText) effectiveMaxOutputTokensText.textContent = formatEffectiveMaxOutputTokens(state.model.effectiveMaxOutputTokensText, 1500);
  if (effectiveMaxOutputTokensJSON) effectiveMaxOutputTokensJSON.textContent = formatEffectiveMaxOutputTokens(state.model.effectiveMaxOutputTokensJSON, 800);
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
  return saveSettingsWithOverrides({});
}

async function saveSettingsWithOverrides(overrides = {}) {
  try {
    const messagesValue = document.getElementById('settings-weixin-history-messages')?.value.trim() || String(state.settings.weixinHistoryMessages ?? 12);
    const runesValue = document.getElementById('settings-weixin-history-runes')?.value.trim() || String(state.settings.weixinHistoryRunes ?? 360);
    const everythingPathValue = document.getElementById('tool-everything-path')?.value.trim() || state.settings.weixinEverythingPath || '';
    const screenTraceEnabled = Boolean(document.getElementById('settings-screentrace-enabled')?.checked ?? state.settings.screenTraceEnabled);
    const screenTraceIntervalValue = document.getElementById('settings-screentrace-interval-seconds')?.value.trim() || String(state.settings.screenTraceIntervalSeconds ?? 15);
    const screenTraceRetentionValue = document.getElementById('settings-screentrace-retention-days')?.value.trim() || String(state.settings.screenTraceRetentionDays ?? 7);
    const screenTraceVisionProfileId = document.getElementById('settings-screentrace-vision-profile')?.value.trim() || state.settings.screenTraceVisionProfileId || '';
    const screenTraceWriteDigestsToKb = Boolean(document.getElementById('settings-screentrace-write-digests-kb')?.checked ?? state.settings.screenTraceWriteDigestsToKb);
    const payload = {
      weixinHistoryMessages: Number(messagesValue),
      weixinHistoryRunes: Number(runesValue),
      weixinEverythingPath: everythingPathValue,
      disabledToolNames: Array.isArray(state.settings.disabledToolNames) ? [...state.settings.disabledToolNames] : [],
      screenTraceEnabled,
      screenTraceIntervalSeconds: Number(screenTraceIntervalValue),
      screenTraceRetentionDays: Number(screenTraceRetentionValue),
      screenTraceVisionProfileId,
      screenTraceWriteDigestsToKb,
      ...overrides,
    };

    if (!Number.isInteger(payload.weixinHistoryMessages) || payload.weixinHistoryMessages < 0) {
      throw new Error('最近消息条数必须是大于等于 0 的整数。');
    }
    if (!Number.isInteger(payload.weixinHistoryRunes) || payload.weixinHistoryRunes < 0) {
      throw new Error('单条消息字符上限必须是大于等于 0 的整数。');
    }
    if (!Number.isInteger(payload.screenTraceIntervalSeconds) || payload.screenTraceIntervalSeconds < 0) {
      throw new Error('活动记录截图间隔必须是大于等于 0 的整数。');
    }
    if (!Number.isInteger(payload.screenTraceRetentionDays) || payload.screenTraceRetentionDays < 0) {
      throw new Error('活动记录保留天数必须是大于等于 0 的整数。');
    }

    state.settings = normalizeSettingsState(await state.backend.SaveSettings(payload));
    await refreshTools();
    await refreshScreenTraceData();
    renderSettings();
    showBanner('设置已保存。', false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function setToolEnabled(name, enabled) {
  const toolName = String(name || '').trim().toLowerCase();
  if (!toolName) return;

  const disabled = new Set(Array.isArray(state.settings.disabledToolNames) ? state.settings.disabledToolNames : []);
  if (enabled) {
    disabled.delete(toolName);
  } else {
    disabled.add(toolName);
  }
  await saveSettingsWithOverrides({
    disabledToolNames: Array.from(disabled).sort((left, right) => left.localeCompare(right, 'en')),
  });
}

function renderSettings() {
  const messages = document.getElementById('settings-weixin-history-messages');
  const runes = document.getElementById('settings-weixin-history-runes');
  const screenTraceEnabled = document.getElementById('settings-screentrace-enabled');
  const screenTraceInterval = document.getElementById('settings-screentrace-interval-seconds');
  const screenTraceRetention = document.getElementById('settings-screentrace-retention-days');
  const screenTraceProfile = document.getElementById('settings-screentrace-vision-profile');
  const screenTraceWriteDigests = document.getElementById('settings-screentrace-write-digests-kb');

  if (messages) {
    messages.value = String(state.settings.weixinHistoryMessages ?? 12);
  }
  if (runes) {
    runes.value = String(state.settings.weixinHistoryRunes ?? 360);
  }
  if (screenTraceEnabled) {
    screenTraceEnabled.checked = Boolean(state.settings.screenTraceEnabled);
  }
  if (screenTraceInterval) {
    screenTraceInterval.value = String(state.settings.screenTraceIntervalSeconds ?? 15);
  }
  if (screenTraceRetention) {
    screenTraceRetention.value = String(state.settings.screenTraceRetentionDays ?? 7);
  }
  if (screenTraceWriteDigests) {
    screenTraceWriteDigests.checked = Boolean(state.settings.screenTraceWriteDigestsToKb);
  }
  if (screenTraceProfile) {
    const profiles = Array.isArray(state.model.profiles) ? state.model.profiles : [];
    const selected = state.settings.screenTraceVisionProfileId || '';
    const options = ['<option value="">请选择独立视觉模型</option>']
      .concat(profiles.map((item) => `<option value="${escapeAttribute(item.id)}">${escapeHTML(item.name || item.model || item.id)}</option>`));
    screenTraceProfile.innerHTML = options.join('');
    screenTraceProfile.value = selected;
  }
}

function renderScreenTrace() {
  const status = state.screenTraceStatus || defaultScreenTraceStatus();
  const captureNowButton = document.getElementById('screentrace-capture-now');
  const pill = document.getElementById('screentrace-status-pill');
  const copy = document.getElementById('screentrace-status-copy');
  const total = document.getElementById('screentrace-total-records');
  const skipped = document.getElementById('screentrace-skipped-duplicates');
  const lastCapture = document.getElementById('screentrace-last-capture');
  const lastAnalysis = document.getElementById('screentrace-last-analysis');
  const lastDigest = document.getElementById('screentrace-last-digest');
  const lastError = document.getElementById('screentrace-last-error');
  const recordList = document.getElementById('screentrace-record-list');
  const digestList = document.getElementById('screentrace-digest-list');

  if (pill) {
    pill.className = `status-pill ${status.enabled && status.running ? 'on' : 'off'}`;
    pill.textContent = status.enabled ? (status.running ? '记录中' : '已启用') : '未启用';
  }
  if (copy) {
    copy.textContent = status.lastError
      ? `最近错误：${status.lastError}`
      : status.enabled
        ? `每 ${status.intervalSeconds || 15} 秒截图一次，当前已保存 ${status.totalRecords || 0} 条记录。`
        : '尚未启用活动记录。启用后会周期性截图并生成轻量活动摘要。';
  }
  if (total) total.textContent = String(status.totalRecords || 0);
  if (skipped) skipped.textContent = String(status.skippedDuplicates || 0);
  if (lastCapture) lastCapture.textContent = status.lastCaptureAt || '—';
  if (lastAnalysis) lastAnalysis.textContent = status.lastAnalysisAt || '—';
  if (lastDigest) lastDigest.textContent = status.lastDigestAt || '—';
  if (lastError) lastError.textContent = status.lastError || '无';
  if (captureNowButton) {
    captureNowButton.disabled = Boolean(state.screenTraceCapturePending);
    captureNowButton.textContent = state.screenTraceCapturePending ? '分析中...' : '立即分析一次';
  }

  if (recordList) {
    const items = Array.isArray(state.screenTraceRecords) ? state.screenTraceRecords.slice(0, 10) : [];
    if (items.length === 0) {
      recordList.innerHTML = `
        <div class="empty-state">
          <div class="empty-state-icon">▣</div>
          <h3>还没有活动记录</h3>
          <p>启用 ScreenTrace 后，这里会显示最近的桌面截图摘要。</p>
        </div>
      `;
    } else {
      recordList.innerHTML = items.map((item) => `
        <article class="memory-card">
          <div class="memory-card-header">
            <div>
              <div class="memory-meta">
                <span class="memory-badge id">#${escapeHTML(item.shortId)}</span>
                ${item.displayLabel ? `<span class="memory-badge source">${escapeHTML(item.displayLabel)}</span>` : ''}
                ${item.sensitiveLevel ? `<span class="memory-badge source">${escapeHTML(item.sensitiveLevel)}</span>` : ''}
              </div>
              <h3 class="prompt-card-title">${escapeHTML(item.sceneSummary || '未生成摘要')}</h3>
            </div>
          </div>
          <div class="memory-content expanded">${escapeHTML(item.taskGuess || '—')}</div>
          <div class="memory-card-footer">
            <span class="memory-date">${escapeHTML(item.capturedAt || '—')}</span>
            <div class="memory-actions">
              ${item.apps?.length ? `<span class="memory-badge source">${escapeHTML(item.apps.join(' / '))}</span>` : ''}
              ${item.dimensionsLabel ? `<span class="memory-badge source">${escapeHTML(item.dimensionsLabel)}</span>` : ''}
            </div>
          </div>
        </article>
      `).join('');
    }
  }

  if (digestList) {
    const items = Array.isArray(state.screenTraceDigests) ? state.screenTraceDigests : [];
    if (items.length === 0) {
      digestList.innerHTML = `
        <div class="empty-state">
          <div class="empty-state-icon">◫</div>
          <h3>还没有时间段摘要</h3>
          <p>累计活动记录后，这里会生成按时间段整理的摘要。</p>
        </div>
      `;
    } else {
      digestList.innerHTML = items.map((item) => `
        <article class="memory-card">
          <div class="memory-card-header">
            <div>
              <div class="memory-meta">
                <span class="memory-badge id">#${escapeHTML(item.shortId)}</span>
                <span class="memory-badge source">${escapeHTML(String(item.recordCount || 0))} 条</span>
                ${item.writtenToKb ? '<span class="memory-badge source">已写入 KB</span>' : ''}
              </div>
              <h3 class="prompt-card-title">${escapeHTML(item.bucketStart || '')} - ${escapeHTML(item.bucketEnd || '')}</h3>
            </div>
          </div>
          <div class="memory-content expanded">${escapeHTML(item.summary || '—')}</div>
          <div class="memory-card-footer">
            <span class="memory-date">${escapeHTML((item.dominantApps || []).join(' / ') || '—')}</span>
            <div class="memory-actions">
              ${item.dominantTasks?.length ? `<span class="memory-badge source">${escapeHTML(item.dominantTasks.join(' / '))}</span>` : ''}
            </div>
          </div>
        </article>
      `).join('');
    }
  }
}

async function refreshScreenTraceManually() {
  try {
    await refreshScreenTraceData();
    showBanner('活动记录已刷新。', false);
  } catch (error) {
    showBanner(asMessage(error), true);
  }
}

async function captureScreenTraceNow() {
  if (state.screenTraceCapturePending) return;
  const trigger = document.getElementById('screentrace-capture-now');
  const originalLabel = trigger?.textContent || '立即分析一次';
  try {
    state.screenTraceCapturePending = true;
    if (trigger) {
      trigger.disabled = true;
      trigger.textContent = '分析中...';
    }
    renderScreenTrace();
    const result = await state.backend.CaptureScreenTraceNow();
    await refreshScreenTraceData();
    showBanner(result?.message || '已执行一次即时截图分析。', false);
  } catch (error) {
    showBanner(asMessage(error), true);
  } finally {
    state.screenTraceCapturePending = false;
    if (trigger) {
      trigger.disabled = false;
      trigger.textContent = originalLabel;
    }
    renderScreenTrace();
  }
}


/* Source: js/ui/chat-session-ui.js */
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


/* Source: js/core/events.js */
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


/* Source: js/core/backend.js */
function desktopDiagnosticsEnabled() {
  return window.__BAIZE_DESKTOP_DEBUG_DIAGNOSTICS__ === true;
}

const desktopDiagnosticsState = {
  installed: false,
  entries: [],
  bannerShown: false,
  outboundMessages: [],
};

function describeDiagnosticFunction(value) {
  if (typeof value !== 'function') return '';
  try {
    return String(value).replace(/\s+/g, ' ').slice(0, 240);
  } catch (error) {
    return `[uninspectable function: ${asMessage(error)}]`;
  }
}

function summarizeDiagnosticValue(value) {
  if (value instanceof Error) {
    return {
      name: value.name || 'Error',
      message: value.message || '',
      stack: value.stack || '',
    };
  }
  if (value === null || value === undefined) return value;
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') {
    return value;
  }
  if (Array.isArray(value)) {
    return value.slice(0, 8).map((item) => summarizeDiagnosticValue(item));
  }
  if (typeof value === 'function') {
    return describeDiagnosticFunction(value);
  }
  if (typeof value === 'object') {
    const summary = {};
    Object.entries(value).slice(0, 16).forEach(([key, item]) => {
      summary[key] = summarizeDiagnosticValue(item);
    });
    return summary;
  }
  return String(value);
}

function buildDesktopDiagnosticSnapshot(reason, detail = {}) {
  return {
    timestamp: new Date().toISOString(),
    reason,
    detail: summarizeDiagnosticValue(detail),
    buildMode: window.__BAIZE_DESKTOP_BUILD_MODE__ || 'release',
    location: window.location.href,
    readyState: document.readyState,
    userAgent: navigator.userAgent,
    runtime: {
      WailsInvoke: {
        type: typeof window.WailsInvoke,
        preview: describeDiagnosticFunction(window.WailsInvoke),
      },
      chrome: {
        type: typeof window.chrome,
        hasWebview: Boolean(window.chrome?.webview),
        postMessageType: typeof window.chrome?.webview?.postMessage,
        postMessagePreview: describeDiagnosticFunction(window.chrome?.webview?.postMessage),
      },
      webkit: {
        hasExternalHandler: Boolean(window.webkit?.messageHandlers?.external),
        postMessageType: typeof window.webkit?.messageHandlers?.external?.postMessage,
        postMessagePreview: describeDiagnosticFunction(window.webkit?.messageHandlers?.external?.postMessage),
      },
      external: {
        type: typeof window.external,
        invokeType: typeof window.external?.invoke,
        invokePreview: describeDiagnosticFunction(window.external?.invoke),
      },
      go: {
        hasDesktopApp: Boolean(window.go?.main?.DesktopApp),
        desktopMethods: Object.keys(window.go?.main?.DesktopApp || {}).slice(0, 20),
      },
      wails: {
        present: Boolean(window.wails),
        callbackCount: Object.keys(window.wails?.callbacks || {}).length,
        callbackType: typeof window.wails?.Callback,
      },
      runtime: {
        present: Boolean(window.runtime),
        eventsOnType: typeof window.runtime?.EventsOn,
      },
    },
    outboundMessages: desktopDiagnosticsState.outboundMessages.slice(-12),
  };
}

function formatDesktopDiagnosticEntries() {
  return desktopDiagnosticsState.entries
    .map((entry, index) => `#${desktopDiagnosticsState.entries.length - index}\n${JSON.stringify(entry, null, 2)}`)
    .join('\n\n');
}

function ensureDesktopDiagnosticsPanel() {
  if (!desktopDiagnosticsEnabled() || !document.body) return null;

  let panel = document.getElementById('desktop-debug-panel');
  if (!panel) {
    panel = document.createElement('details');
    panel.id = 'desktop-debug-panel';
    panel.open = true;
    panel.style.position = 'fixed';
    panel.style.right = '16px';
    panel.style.bottom = '16px';
    panel.style.zIndex = '99999';
    panel.style.width = 'min(680px, calc(100vw - 24px))';
    panel.style.maxHeight = '52vh';
    panel.style.padding = '10px 12px 12px';
    panel.style.border = '1px solid rgba(255, 107, 107, 0.65)';
    panel.style.borderRadius = '14px';
    panel.style.background = 'rgba(6, 11, 18, 0.96)';
    panel.style.color = '#dbe7f3';
    panel.style.boxShadow = '0 14px 36px rgba(0, 0, 0, 0.35)';
    panel.style.backdropFilter = 'blur(12px)';

    const summary = document.createElement('summary');
    summary.textContent = 'Desktop Diagnostics';
    summary.style.cursor = 'pointer';
    summary.style.fontWeight = '700';
    summary.style.marginBottom = '8px';
    panel.appendChild(summary);

    const meta = document.createElement('div');
    meta.id = 'desktop-debug-meta';
    meta.style.fontSize = '12px';
    meta.style.color = '#9db2c8';
    meta.style.marginBottom = '8px';
    panel.appendChild(meta);

    const output = document.createElement('pre');
    output.id = 'desktop-debug-output';
    output.style.margin = '0';
    output.style.maxHeight = 'calc(52vh - 52px)';
    output.style.overflow = 'auto';
    output.style.whiteSpace = 'pre-wrap';
    output.style.wordBreak = 'break-word';
    output.style.fontSize = '11px';
    output.style.lineHeight = '1.45';
    output.style.fontFamily = 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, Liberation Mono, monospace';
    panel.appendChild(output);

    document.body.appendChild(panel);
  }

  const meta = document.getElementById('desktop-debug-meta');
  if (meta) {
    meta.textContent = `build=${window.__BAIZE_DESKTOP_BUILD_MODE__ || 'release'}  entries=${desktopDiagnosticsState.entries.length}`;
  }

  const output = document.getElementById('desktop-debug-output');
  if (output) {
    output.textContent = formatDesktopDiagnosticEntries();
  }

  return panel;
}

function reportDesktopDiagnostics(reason, detail = {}) {
  if (!desktopDiagnosticsEnabled()) return;

  const snapshot = buildDesktopDiagnosticSnapshot(reason, detail);
  desktopDiagnosticsState.entries.push(snapshot);
  if (desktopDiagnosticsState.entries.length > 12) {
    desktopDiagnosticsState.entries = desktopDiagnosticsState.entries.slice(-12);
  }

  try {
    ensureDesktopDiagnosticsPanel();
  } catch (error) {
    console.error('[desktop-debug] render diagnostics panel failed', error);
  }

  if (reason !== 'diagnostics-enabled' && !desktopDiagnosticsState.bannerShown && typeof showBanner === 'function') {
    desktopDiagnosticsState.bannerShown = true;
    showBanner('已捕获桌面运行时诊断，请展开右下角 Desktop Diagnostics 面板。', true);
  }

  console.error(`[desktop-debug] ${reason}`, snapshot);
}

function recordDesktopOutboundMessage(rawMessage) {
  if (!desktopDiagnosticsEnabled()) return;

  const message = String(rawMessage || '');
  const entry = {
    timestamp: new Date().toISOString(),
    prefix: message.slice(0, 2),
    preview: message.slice(0, 240),
  };
  if (message.startsWith('C') || message.startsWith('c') || message.startsWith('EE')) {
    try {
      entry.payload = summarizeDiagnosticValue(JSON.parse(message.slice(message.startsWith('EE') ? 2 : 1)));
    } catch (error) {
      entry.parseError = asMessage(error);
    }
  }
  desktopDiagnosticsState.outboundMessages.push(entry);
  if (desktopDiagnosticsState.outboundMessages.length > 20) {
    desktopDiagnosticsState.outboundMessages = desktopDiagnosticsState.outboundMessages.slice(-20);
  }
}

function installDesktopDebugDiagnostics() {
  if (!desktopDiagnosticsEnabled() || desktopDiagnosticsState.installed) return;
  desktopDiagnosticsState.installed = true;

  window.addEventListener('error', (event) => {
    reportDesktopDiagnostics('window-error', {
      message: event.message || '',
      filename: event.filename || '',
      lineno: event.lineno || 0,
      colno: event.colno || 0,
      error: event.error || null,
    });
  });

  window.addEventListener('unhandledrejection', (event) => {
    reportDesktopDiagnostics('unhandled-rejection', {
      reason: event.reason || null,
    });
  });

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => {
      ensureDesktopDiagnosticsPanel();
      reportDesktopDiagnostics('diagnostics-enabled', {
        protocol: window.location.protocol,
      });
    }, { once: true });
    return;
  }

  ensureDesktopDiagnosticsPanel();
  reportDesktopDiagnostics('diagnostics-enabled', {
    protocol: window.location.protocol,
  });
}

async function waitForBackend() {
  for (let index = 0; index < 80; index += 1) {
    if (hasWailsRuntime()) return createWailsBackend();
    await delay(50);
  }
  reportDesktopDiagnostics('backend-unavailable-after-wait', {
    protocol: window.location.protocol,
  });
  if (window.location.protocol === 'http:' || window.location.protocol === 'https:') {
    reportDesktopDiagnostics('falling-back-to-http-backend', {
      protocol: window.location.protocol,
    });
    return createHTTPBackend();
  }
  throw new Error('Wails 后端尚未就绪。');
}

function selectNativeWailsSender() {
  if (window.chrome?.webview && typeof window.chrome.webview.postMessage === 'function') {
    return (message) => window.chrome.webview.postMessage(message);
  }
  if (window.webkit?.messageHandlers?.external && typeof window.webkit.messageHandlers.external.postMessage === 'function') {
    return (message) => window.webkit.messageHandlers.external.postMessage(message);
  }
  if (window.external && typeof window.external.invoke === 'function') {
    return (message) => window.external.invoke(message);
  }
  return null;
}

function installWailsBridgeShim() {
  const nativeSender = selectNativeWailsSender();
  const currentInvoke = typeof window.WailsInvoke === 'function'
    ? window.WailsInvoke.bind(window)
    : null;
  const sender = nativeSender || currentInvoke;
  if (!sender) {
    reportDesktopDiagnostics('wails-bridge-sender-missing', {});
    return false;
  }

  window.WailsInvoke = (message) => {
    recordDesktopOutboundMessage(message);
    try {
      return sender(message);
    } catch (error) {
      reportDesktopDiagnostics('wailsinvoke-throw', {
        message,
        error,
      });
      throw error;
    }
  };

  if (nativeSender) {
    try {
      if (!window.external || typeof window.external !== 'object') {
        window.external = {};
      }
      window.external.invoke = (message) => {
        recordDesktopOutboundMessage(message);
        try {
          return nativeSender(message);
        } catch (error) {
          reportDesktopDiagnostics('external-invoke-throw', {
            message,
            error,
          });
          throw error;
        }
      };
    } catch (error) {
      // Ignore readonly host objects; direct WailsInvoke override is enough.
    }
  }

  return true;
}

function hasWailsRuntime() {
  return installWailsBridgeShim()
    && Boolean(window.wails)
    && Boolean(window.wails.callbacks)
    && typeof window.wails.Callback === 'function';
}

function newWailsCallbackID(methodName) {
  const prefix = String(methodName || 'wails').replaceAll('.', '_');
  if (window.crypto && typeof window.crypto.getRandomValues === 'function') {
    const parts = new Uint32Array(2);
    window.crypto.getRandomValues(parts);
    return `${prefix}-${parts[0]}-${parts[1]}`;
  }
  return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function invokeWailsMethod(methodName, args = [], timeout = 0) {
  return new Promise((resolve, reject) => {
    if (!hasWailsRuntime()) {
      const error = new Error('Wails runtime 尚未就绪。');
      reportDesktopDiagnostics('invoke-without-runtime', {
        methodName,
        args,
        error,
      });
      reject(error);
      return;
    }

    const callbackID = newWailsCallbackID(methodName);
    const payload = {
      name: methodName,
      args: Array.isArray(args) ? args : [args],
      callbackID,
    };

    let timeoutHandle = 0;
    if (timeout > 0) {
      timeoutHandle = window.setTimeout(() => {
        delete window.wails.callbacks[callbackID];
        const error = new Error(`调用 ${methodName} 超时。`);
        reportDesktopDiagnostics('invoke-timeout', {
          methodName,
          callbackID,
          error,
        });
        reject(error);
      }, timeout);
    }

    window.wails.callbacks[callbackID] = {
      timeoutHandle,
      reject: (error) => {
        reportDesktopDiagnostics('invoke-callback-error', {
          methodName,
          callbackID,
          args,
          error,
        });
        reject(error);
      },
      resolve: (result) => {
        resolve(result);
      },
    };

    try {
      window.WailsInvoke(`C${JSON.stringify(payload)}`);
    } catch (error) {
      window.clearTimeout(timeoutHandle);
      delete window.wails.callbacks[callbackID];
      reportDesktopDiagnostics('invoke-throw', {
        methodName,
        callbackID,
        payload,
        error,
      });
      reject(error);
    }
  });
}

function createWailsBackend() {
  const call = (methodName, ...args) => invokeWailsMethod(`main.DesktopApp.${methodName}`, args);

  return {
    mode: 'wails',
    GetOverview: () => call('GetOverview'),
    GetProjectState: () => call('GetProjectState'),
    SetActiveProject: (name) => call('SetActiveProject', name),
    ListReminders: () => call('ListReminders'),
    ListKnowledge: () => call('ListKnowledge'),
    CreateKnowledge: (text) => call('CreateKnowledge', text),
    AppendKnowledge: (idOrPrefix, addition) => call('AppendKnowledge', idOrPrefix, addition),
    DeleteKnowledge: (idOrPrefix) => call('DeleteKnowledge', idOrPrefix),
    ClearKnowledge: () => call('ClearKnowledge'),
    ListPrompts: () => call('ListPrompts'),
    CreatePrompt: (title, content) => call('CreatePrompt', title, content),
    DeletePrompt: (idOrPrefix) => call('DeletePrompt', idOrPrefix),
    ClearPrompts: () => call('ClearPrompts'),
    ListSkills: () => call('ListSkills'),
    ListTools: () => call('ListTools'),
    LoadSkill: (name) => call('LoadSkill', name),
    UnloadSkill: (name) => call('UnloadSkill', name),
    OpenSkillImportDialog: () => call('OpenSkillImportDialog'),
    ImportSkillArchive: (path) => call('ImportSkillArchive', path),
    UploadSkillArchive: () => Promise.reject(new Error('Wails 模式不使用浏览器上传。')),
    ConfirmAction: (title, message) => call('ConfirmAction', title, message),
    OpenImportDialog: () => call('OpenImportDialog'),
    ImportFile: (path) => call('ImportFile', path),
    UploadImportFile: () => Promise.reject(new Error('Wails 模式不使用浏览器上传。')),
    SendMessage: (input) => call('SendMessage', input),
    SendMessageStream: async (input, handlers = {}) => {
      if (!window.runtime || typeof window.runtime.EventsOn !== 'function') {
        const result = await call('SendMessage', input);
        if (typeof handlers.onDelta === 'function' && result?.reply) {
          handlers.onDelta(result.reply);
        }
        return result;
      }

      const requestId = newChatStreamRequestID();
      state.chatStreamHandlers[requestId] = (event) => {
        if ((event?.type === 'process' || event?.step) && typeof handlers.onProcess === 'function') {
          handlers.onProcess(event.step || null);
          return;
        }
        if ((event?.delta || '') && typeof handlers.onDelta === 'function') {
          handlers.onDelta(event.delta);
        }
      };
      try {
        return await call('SendMessageStream', requestId, input);
      } finally {
        delete state.chatStreamHandlers[requestId];
      }
    },
    GetChatState: () => call('GetChatState'),
    GetVersionInfo: () => call('GetVersionInfo'),
    OpenExternalURL: (url) => call('OpenExternalURL', url),
    RefreshChatResponse: () => call('RefreshChatResponse'),
    ExportChatMarkdown: () => call('ExportChatMarkdown'),
    NewChatSession: (mode = 'agent') => call('NewChatSession', mode),
    SwitchChatSession: (sessionId) => call('SwitchChatSession', sessionId),
    RenameChatSession: (sessionId, title) => call('RenameChatSession', sessionId, title),
    DeleteChatSession: (sessionId) => call('DeleteChatSession', sessionId),
    GetChatPrompt: () => call('GetChatPrompt'),
    SetChatPrompt: (idOrPrefix) => call('SetChatPrompt', idOrPrefix),
    ClearChatPrompt: () => call('ClearChatPrompt'),
    GetModelSettings: () => call('GetModelSettings'),
    SaveModelConfig: (payload) => call('SaveModelConfig', payload),
    TestModelConnection: (id) => call('TestModelConnection', id),
    DeleteModelConfig: (id) => call('DeleteModelConfig', id),
    SetActiveModel: (id) => call('SetActiveModel', id),
    GetWeixinStatus: () => call('GetWeixinStatus'),
    StartWeixinLogin: () => call('StartWeixinLogin'),
    CancelWeixinLogin: () => call('CancelWeixinLogin'),
    LogoutWeixin: () => call('LogoutWeixin'),
    GetSettings: () => call('GetSettings'),
    SaveSettings: (payload) => call('SaveSettings', payload),
    GetScreenTraceStatus: () => call('GetScreenTraceStatus'),
    ListScreenTraceRecords: (limit = 60) => call('ListScreenTraceRecords', limit),
    ListScreenTraceDigests: (limit = 20) => call('ListScreenTraceDigests', limit),
    CaptureScreenTraceNow: () => call('CaptureScreenTraceNow'),
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
    ListTools: () => requestJSON('GET', '/api/tools'),
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
    GetVersionInfo: () => requestJSON('GET', '/api/version'),
    OpenExternalURL: (url) => requestJSON('POST', '/api/open-external', { url }),
    RefreshChatResponse: () => requestJSON('POST', '/api/chat/refresh'),
    ExportChatMarkdown: async () => {
      const payload = await requestJSON('GET', '/api/chat/export-markdown');
      downloadTextFile(payload.filename || 'baize-chat.md', payload.markdown || '', 'text/markdown;charset=utf-8');
      return { message: `已导出 Markdown：${payload.filename || 'baize-chat.md'}` };
    },
    NewChatSession: (mode = 'agent') => requestJSON('POST', '/api/chat/session/new', { mode }),
    SwitchChatSession: (sessionId) => requestJSON('POST', '/api/chat/session/switch', { sessionId }),
    RenameChatSession: (sessionId, title) => requestJSON('POST', '/api/chat/session/rename', { sessionId, title }),
    DeleteChatSession: (sessionId) => requestJSON('POST', '/api/chat/session/delete', { sessionId }),
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
    GetScreenTraceStatus: () => requestJSON('GET', '/api/screentrace/status'),
    ListScreenTraceRecords: (limit = 60) => requestJSON('GET', `/api/screentrace/records?limit=${encodeURIComponent(limit)}`),
    ListScreenTraceDigests: (limit = 20) => requestJSON('GET', `/api/screentrace/digests?limit=${encodeURIComponent(limit)}`),
    CaptureScreenTraceNow: () => requestJSON('POST', '/api/screentrace/capture'),
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
        } else if (event.type === 'process') {
          if (typeof handlers.onProcess === 'function' && event.step) {
            handlers.onProcess(event.step);
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
    } else if (event.type === 'process') {
      if (typeof handlers.onProcess === 'function' && event.step) {
        handlers.onProcess(event.step);
      }
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
    void Promise.all([refreshProjectState(), refreshOverview(), refreshReminders(), refreshModel(), refreshWeixin(), refreshScreenTraceStatus()]).catch(() => {});
  }, 2000);
}

async function refreshAll() {
  await Promise.all([refreshProjectState(), refreshOverview(), refreshReminders(), refreshKnowledge(), refreshPrompts(), refreshSkills(), refreshTools(), refreshChatPrompt(), refreshModel(), refreshWeixin(), refreshSettings(), refreshScreenTraceData()]);
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
  const versionCompact = document.getElementById('version-compact');
  const versionCheck = document.getElementById('version-check');

  if (aiStatusCompact) aiStatusCompact.textContent = state.overview.aiAvailable ? 'OK' : '—';
  if (memoryCountCompact) memoryCountCompact.textContent = String(state.overview.knowledgeCount);
  if (promptCountCompact) promptCountCompact.textContent = String(state.overview.promptCount || 0);
  if (versionCompact) versionCompact.textContent = state.overview.currentVersion || 'dev';
  if (versionCheck) {
    versionCheck.title = `当前版本 ${state.overview.currentVersion || 'dev'}，点击查看最新版本`;
  }
}

async function checkLatestVersion() {
  const trigger = document.getElementById('version-check');
  if (trigger) {
    trigger.disabled = true;
  }

  try {
    const info = await state.backend.GetVersionInfo();
    if (state.overview && info?.currentVersion) {
      state.overview.currentVersion = info.currentVersion;
      const versionCompact = document.getElementById('version-compact');
      if (versionCompact) {
        versionCompact.textContent = info.currentVersion;
      }
    }
    if (info?.hasUpdate && info?.releaseUrl) {
      const ok = await state.backend.ConfirmAction(
        '发现新版本',
        `${info.message}\n\n是否打开发布页？`,
      );
      if (ok) {
        await state.backend.OpenExternalURL(info.releaseUrl);
        showBanner(`已打开发布页：${info.latestVersion || info.releaseUrl}`, false);
        return;
      }
    }
    showBanner(info?.message || '暂时无法获取版本信息。', false);
  } catch (error) {
    showBanner(asMessage(error), true);
  } finally {
    if (trigger) {
      trigger.disabled = false;
    }
  }
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

async function refreshTools() {
  state.tools = normalizeTools(await state.backend.ListTools());
  renderTools();
}

async function refreshChatPrompt() {
  state.chatPrompt = normalizeChatPromptState(await state.backend.GetChatPrompt());
  renderChatContext();
  updateChatAutocomplete();
}

async function refreshModel() {
  state.model = normalizeModelSettings(await state.backend.GetModelSettings());
  if (state.modelFormDirty) {
    renderSettings();
    return;
  }
  renderModel();
  renderSettings();
}

async function refreshWeixin() {
  const next = await state.backend.GetWeixinStatus();
  applyWeixinStatus(next, false);
}

async function refreshSettings() {
  state.settings = normalizeSettingsState(await state.backend.GetSettings());
  renderSettings();
}

async function refreshScreenTraceStatus() {
  state.screenTraceStatus = normalizeScreenTraceStatus(await state.backend.GetScreenTraceStatus());
  renderScreenTrace();
}

async function refreshScreenTraceData() {
  const [status, records, digests] = await Promise.all([
    state.backend.GetScreenTraceStatus(),
    state.backend.ListScreenTraceRecords(10),
    state.backend.ListScreenTraceDigests(20),
  ]);
  state.screenTraceStatus = normalizeScreenTraceStatus(status);
  state.screenTraceRecords = normalizeScreenTraceRecords(records);
  state.screenTraceDigests = normalizeScreenTraceDigests(digests);
  renderScreenTrace();
}


/* Source: js/features/library-actions.js */
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


/* Source: js/features/chat-composer.js */
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


/* Source: js/core/init.js */
document.addEventListener("DOMContentLoaded", () => {
  void init();
});

async function init() {
  installDesktopDebugDiagnostics();
  initTheme();
  bindStaticEvents();
  bindNavigation();
  bindQuickAddModal();
  bindChatSessionUI();
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
  renderTools();
  renderModel();
  renderWeixin();
  renderSettings();
  renderScreenTrace();

  try {
    state.backend = await waitForBackend();
    state.backendMode = state.backend.mode || 'wails';
    bindRuntimeEvents();
    startBackendPolling();
    await refreshAll();
    await refreshChatState();
  } catch (error) {
    reportDesktopDiagnostics('init-failed', {
      error,
    });
    showBanner(asMessage(error), true);
  }
}
