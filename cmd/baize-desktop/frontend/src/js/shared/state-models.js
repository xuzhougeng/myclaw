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
