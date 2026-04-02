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
