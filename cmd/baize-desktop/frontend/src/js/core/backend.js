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
