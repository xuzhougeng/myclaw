document.addEventListener("DOMContentLoaded", () => {
  void init();
});

async function init() {
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
