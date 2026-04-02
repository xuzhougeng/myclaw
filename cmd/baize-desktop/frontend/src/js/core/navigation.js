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
