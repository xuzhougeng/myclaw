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
