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
