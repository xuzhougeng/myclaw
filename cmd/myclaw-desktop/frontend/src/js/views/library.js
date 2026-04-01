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
    title: item.title || item.shortName || item.name || '',
    description: item.description || item.purpose || '',
    purpose: item.purpose || '',
    provider: item.provider || '',
    providerKind: item.providerKind || '',
    sideEffectLevel: item.sideEffectLevel || '',
    status: item.status || '已就绪',
    statusTone: item.statusTone || 'on',
    configurable: Boolean(item.configurable),
    configValue: item.configValue || '',
  }));
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
        <span>Everything \`es.exe\` 路径</span>
        <input id="tool-everything-path" type="text" placeholder="例如: C:\\Program Files\\Everything\\es.exe 或 es.exe" value="${escapeAttribute(value)}" />
      </label>
      <div class="tool-card-note">
        这个配置同时供 agent 文件检索和 `/find` 使用；如果还没安装 ES，可以去
        <a href="https://www.voidtools.com/zh-cn/downloads" target="_blank" rel="noopener noreferrer">Everything 下载页</a>
        下载。
      </div>
      <div class="tool-card-note">
        `/find` 目前只在 Windows 下生效，desktop / terminal / 微信会复用同一个文件检索模块；微信发送文件仍然使用 `/send &lt;序号&gt;`。
      </div>
      <div class="tool-card-actions">
        <button class="btn btn-primary btn-sm" data-tool-action="save-everything-path">保存路径</button>
      </div>
    </div>
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

  container.innerHTML = tools
    .map((tool) => {
      const purpose = tool.purpose && tool.purpose !== tool.description
        ? `<p class="tool-card-purpose">${escapeHTML(tool.purpose)}</p>`
        : '';
      const providerLabel = [tool.provider, tool.providerKind].filter(Boolean).join(' / ') || 'local';
      return `
        <article class="tool-card">
          <div class="tool-card-header">
            <div>
              <div class="tool-card-title-row">
                <h3>${escapeHTML(tool.title || tool.shortName || tool.name)}</h3>
                <span class="status-pill ${escapeAttribute(tool.statusTone || 'on')}">${escapeHTML(tool.status || '已就绪')}</span>
              </div>
              <div class="tool-card-name">${escapeHTML(tool.name || tool.shortName)}</div>
            </div>
            <div class="tool-card-tags">
              <span class="tool-meta-pill">${escapeHTML(toolSideEffectLabel(tool.sideEffectLevel))}</span>
              <span class="tool-meta-pill provider">${escapeHTML(providerLabel)}</span>
            </div>
          </div>
          <p class="tool-card-desc">${escapeHTML(tool.description || '这个工具还没有填写描述。')}</p>
          ${purpose}
          ${renderToolConfig(tool)}
        </article>
      `;
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
  try {
    const messagesValue = document.getElementById('settings-weixin-history-messages')?.value.trim() || '0';
    const runesValue = document.getElementById('settings-weixin-history-runes')?.value.trim() || '0';
    const everythingPathValue = document.getElementById('tool-everything-path')?.value.trim() || state.settings.weixinEverythingPath || '';
    const payload = {
      weixinHistoryMessages: Number(messagesValue),
      weixinHistoryRunes: Number(runesValue),
      weixinEverythingPath: everythingPathValue,
    };

    if (!Number.isInteger(payload.weixinHistoryMessages) || payload.weixinHistoryMessages < 0) {
      throw new Error('最近消息条数必须是大于等于 0 的整数。');
    }
    if (!Number.isInteger(payload.weixinHistoryRunes) || payload.weixinHistoryRunes < 0) {
      throw new Error('单条消息字符上限必须是大于等于 0 的整数。');
    }

    state.settings = normalizeSettingsState(await state.backend.SaveSettings(payload));
    await refreshTools();
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

