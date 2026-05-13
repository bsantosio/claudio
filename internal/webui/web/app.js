/**
 * claudio web UI — vanilla JS SPA
 * Hash routing: #agents (default) | #agents/new | #sessions | #chat/{sid}
 * SSE streaming: fetch() + response.body.getReader() — EventSource is NOT used.
 */

/* ── state ── */

const state = {
  apiKey: '',
  agents: [],
  sessions: [],
};

/* ── API client ── */

function apiHeaders(auth = true) {
  const h = { 'Content-Type': 'application/json' };
  if (auth && state.apiKey) {
    h['Authorization'] = 'Bearer ' + state.apiKey;
  }
  return h;
}

async function apiFetch(method, path, body) {
  const opts = { method, headers: apiHeaders() };
  if (body !== undefined) {
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  if (res.status === 401) {
    showKeyModal();
    throw new Error('unauthorized');
  }
  return res;
}

async function apiJSON(method, path, body) {
  const res = await apiFetch(method, path, body);
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }
  return res.json();
}

/* ── hash router ── */

function navigate(hash) {
  window.location.hash = hash;
}

function parseHash() {
  const raw = window.location.hash.replace(/^#/, '') || 'agents';
  const parts = raw.split('/');
  return parts;
}

function router() {
  const parts = parseHash();
  const app = document.getElementById('app');
  app.innerHTML = '';

  const [screen, ...rest] = parts;

  if (screen === 'agents' && rest.length === 0) {
    renderAgentsScreen(app);
  } else if (screen === 'agents' && rest[0] === 'new') {
    renderNewAgentScreen(app);
  } else if (screen === 'sessions') {
    renderSessionsScreen(app, null);
  } else if (screen === 'chat' && rest[0]) {
    renderChatScreen(app, rest[0]);
  } else {
    renderAgentsScreen(app);
  }
}

/* ── key modal ── */

function showKeyModal() {
  document.getElementById('key-modal').classList.remove('hidden');
  document.getElementById('key-input').focus();
}

function hideKeyModal() {
  document.getElementById('key-modal').classList.add('hidden');
}

/* ── agents screen ── */

async function renderAgentsScreen(container) {
  container.innerHTML = `
    <div class="screen-header">
      <h1>Agents</h1>
      <button class="primary" id="btn-new-agent">+ New agent</button>
    </div>
    <div id="agent-list"><div class="empty">Loading…</div></div>
  `;
  document.getElementById('btn-new-agent').onclick = () => navigate('agents/new');

  let agents;
  try {
    agents = await apiJSON('GET', '/agents');
  } catch (e) {
    document.getElementById('agent-list').innerHTML =
      `<div class="empty">Error: ${esc(e.message)}</div>`;
    return;
  }
  state.agents = agents;

  const list = document.getElementById('agent-list');
  if (!agents.length) {
    list.innerHTML = '<div class="empty">No agents yet — create one to get started.</div>';
    return;
  }
  list.innerHTML = '';
  agents.forEach(a => {
    const card = document.createElement('div');
    card.className = 'card';
    card.innerHTML = `
      <div class="card-body">
        <div class="card-title">${esc(a.name)}</div>
        <div class="card-meta">id: ${esc(a.id)} &nbsp;·&nbsp; model: ${esc(a.model)}</div>
      </div>
      <div class="card-actions">
        <button class="small" data-action="sessions" data-id="${esc(a.id)}">Sessions</button>
        <button class="small" data-action="install" data-id="${esc(a.id)}" data-name="${esc(a.name)}">Install</button>
        <button class="small" data-action="uninstall" data-id="${esc(a.id)}" data-name="${esc(a.name)}">Uninstall</button>
        <button class="small danger" data-action="delete" data-id="${esc(a.id)}">Delete</button>
      </div>
    `;
    list.appendChild(card);
  });

  list.addEventListener('click', async e => {
    const btn = e.target.closest('button[data-action]');
    if (!btn) return;
    const { action, id, name } = btn.dataset;

    if (action === 'sessions') {
      renderSessionsScreen(container, id);
    } else if (action === 'install') {
      btn.disabled = true;
      btn.textContent = '…';
      try {
        await apiJSON('POST', `/agents/${id}/install`);
        btn.textContent = 'Installed';
        btn.className = 'small badge green';
      } catch (err) {
        btn.textContent = 'Error';
        btn.disabled = false;
      }
    } else if (action === 'uninstall') {
      btn.disabled = true;
      btn.textContent = '…';
      try {
        await apiFetch('DELETE', `/agents/${id}/install`);
        btn.textContent = 'Uninstalled';
        btn.className = 'small';
        btn.disabled = false;
      } catch (err) {
        btn.textContent = 'Error';
        btn.disabled = false;
      }
    } else if (action === 'delete') {
      if (!confirm(`Delete agent "${name || id}"?`)) return;
      try {
        await apiFetch('DELETE', `/agents/${id}`);
        renderAgentsScreen(container);
      } catch (err) {
        alert('Delete failed: ' + err.message);
      }
    }
  });
}

/* ── new agent screen ── */

async function renderNewAgentScreen(container) {
  container.innerHTML = `
    <a class="back" href="#agents">← Back to agents</a>
    <div class="screen-header"><h1>New Agent</h1></div>
    <div class="form-group">
      <label>Name</label>
      <input id="ag-name" type="text" placeholder="my-agent" />
    </div>
    <div class="form-group">
      <label>System Prompt</label>
      <textarea id="ag-prompt" rows="6" placeholder="You are a helpful assistant."></textarea>
    </div>
    <div class="form-group">
      <label>Model</label>
      <select id="ag-model">
        <option value="sonnet" selected>sonnet</option>
        <option value="opus">opus</option>
        <option value="haiku">haiku</option>
        <option value="claude-opus-4-6[1m]">claude-opus-4-6[1m]</option>
      </select>
    </div>
    <div class="form-actions">
      <button class="primary" id="btn-create-agent">Create</button>
      <button id="btn-cancel-agent">Cancel</button>
    </div>
    <div id="ag-error" style="color:var(--danger);margin-top:10px;display:none"></div>
  `;

  document.getElementById('btn-cancel-agent').onclick = () => navigate('agents');
  document.getElementById('btn-create-agent').onclick = async () => {
    const name = document.getElementById('ag-name').value.trim();
    const prompt = document.getElementById('ag-prompt').value.trim();
    const model = document.getElementById('ag-model').value.trim() || 'sonnet';
    const errEl = document.getElementById('ag-error');
    errEl.style.display = 'none';
    if (!name) { showFieldError(errEl, 'Name is required.'); return; }
    try {
      await apiJSON('POST', '/agents', { name, system_prompt: prompt, model });
      navigate('agents');
    } catch (e) {
      showFieldError(errEl, e.message);
    }
  };
}

function showFieldError(el, msg) {
  el.textContent = msg;
  el.style.display = 'block';
}

/* ── sessions screen ── */

async function renderSessionsScreen(container, agentId) {
  const titleSuffix = agentId ? ` for ${agentId.slice(0, 8)}…` : '';
  container.innerHTML = `
    <a class="back" href="#agents">← Back to agents</a>
    <div class="screen-header">
      <h1>Sessions${esc(titleSuffix)}</h1>
      <button class="primary" id="btn-new-session">+ New session</button>
    </div>
    <div id="session-list"><div class="empty">Loading…</div></div>
  `;

  document.getElementById('btn-new-session').onclick = async () => {
    if (!agentId) {
      alert('Select an agent first.');
      return;
    }
    try {
      const sess = await apiJSON('POST', `/agents/${agentId}/sessions`, { name: '' });
      navigate(`chat/${sess.id}`);
    } catch (e) {
      alert('Failed to create session: ' + e.message);
    }
  };

  let sessions;
  try {
    const url = agentId ? `/agents/${agentId}/sessions` : '/sessions';
    sessions = await apiJSON('GET', url);
  } catch (e) {
    document.getElementById('session-list').innerHTML =
      `<div class="empty">Error: ${esc(e.message)}</div>`;
    return;
  }
  state.sessions = sessions;

  const list = document.getElementById('session-list');
  if (!sessions.length) {
    list.innerHTML = '<div class="empty">No sessions yet.</div>';
    return;
  }
  list.innerHTML = '';
  sessions.forEach(s => {
    const card = document.createElement('div');
    card.className = 'card';
    card.innerHTML = `
      <div class="card-body">
        <div class="card-title">${esc(s.name || s.id)}</div>
        <div class="card-meta">turns: ${s.turn_count} &nbsp;·&nbsp; last active: ${esc(s.last_active || '—')}</div>
      </div>
      <div class="card-actions">
        <button class="small" data-action="open" data-id="${esc(s.id)}">Open chat</button>
        <button class="small danger" data-action="delete" data-id="${esc(s.id)}">Delete</button>
      </div>
    `;
    list.appendChild(card);
  });

  list.addEventListener('click', async e => {
    const btn = e.target.closest('button[data-action]');
    if (!btn) return;
    const { action, id } = btn.dataset;
    if (action === 'open') {
      navigate(`chat/${id}`);
    } else if (action === 'delete') {
      if (!confirm('Delete session?')) return;
      try {
        await apiFetch('DELETE', `/sessions/${id}`);
        renderSessionsScreen(container, agentId);
      } catch (e2) {
        alert('Delete failed: ' + e2.message);
      }
    }
  });
}

/* ── chat screen ── */

async function renderChatScreen(container, sessionId) {
  container.innerHTML = `
    <a class="back" href="#sessions">← Back to sessions</a>
    <div class="chat-container">
      <div id="chat-messages" class="chat-messages"></div>
      <div class="chat-input-row">
        <textarea id="chat-input" placeholder="Type a message… (Shift+Enter for newline, Enter to send)"></textarea>
        <button class="primary" id="chat-send">Send</button>
      </div>
    </div>
  `;

  const messagesEl = document.getElementById('chat-messages');
  const inputEl = document.getElementById('chat-input');
  const sendBtn = document.getElementById('chat-send');

  // Load message history.
  try {
    const history = await apiJSON('GET', `/sessions/${sessionId}/messages`);
    history.forEach(m => appendBubble(messagesEl, m.role, m.content));
  } catch (e) {
    appendBubble(messagesEl, 'error', 'Could not load history: ' + e.message);
  }
  scrollBottom(messagesEl);

  async function sendMessage() {
    const content = inputEl.value.trim();
    if (!content) return;

    inputEl.value = '';
    inputEl.disabled = true;
    sendBtn.disabled = true;

    appendBubble(messagesEl, 'user', content);
    const assistantBubble = appendBubble(messagesEl, 'assistant', '');
    assistantBubble.classList.add('streaming');
    scrollBottom(messagesEl);

    try {
      await streamMessage(sessionId, content, chunk => {
        assistantBubble.textContent += chunk;
        scrollBottom(messagesEl);
      });
    } catch (e) {
      assistantBubble.classList.remove('streaming');
      assistantBubble.classList.add('error');
      assistantBubble.textContent = 'Stream error: ' + e.message;
    } finally {
      assistantBubble.classList.remove('streaming');
      inputEl.disabled = false;
      sendBtn.disabled = false;
      inputEl.focus();
    }
  }

  sendBtn.onclick = sendMessage;
  inputEl.addEventListener('keydown', e => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  });
}

/* ── SSE streaming via fetch + ReadableStream ── */
/* NOTE: EventSource is NOT used — it does not support POST. */

async function streamMessage(sessionId, content, onChunk) {
  const res = await fetch(`/sessions/${sessionId}/message`, {
    method: 'POST',
    headers: apiHeaders(),
    body: JSON.stringify({ content }),
  });

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || res.statusText);
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });

    // Split on SSE event boundaries (\n\n).
    const events = buffer.split('\n\n');
    // Keep the last (possibly incomplete) chunk in the buffer.
    buffer = events.pop();

    for (const event of events) {
      const dataLine = event.split('\n').find(l => l.startsWith('data:'));
      if (!dataLine) continue;
      const raw = dataLine.slice('data:'.length).trimStart();

      // Parse the inner JSON payload from the SSE data field.
      try {
        const payload = JSON.parse(raw);
        // assistant events carry text content.
        if (payload.type === 'assistant' && payload.message?.content) {
          for (const part of payload.message.content) {
            if (part.type === 'text' && part.text) {
              onChunk(part.text);
            }
          }
        }
        // result event signals end of stream — no action needed (loop ends naturally).
      } catch (_) {
        // Ignore non-JSON lines (keep-alives, comments, etc.)
      }
    }
  }
}

/* ── helpers ── */

function appendBubble(container, role, text) {
  const div = document.createElement('div');
  div.className = `bubble ${role}`;
  div.textContent = text;
  container.appendChild(div);
  return div;
}

function scrollBottom(el) {
  el.scrollTop = el.scrollHeight;
}

function esc(str) {
  if (str == null) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

/* ── bootstrap ── */

document.addEventListener('DOMContentLoaded', () => {
  // API key modal controls.
  document.getElementById('key-save').onclick = () => {
    state.apiKey = document.getElementById('key-input').value.trim();
    hideKeyModal();
    router();
  };
  document.getElementById('key-btn').onclick = showKeyModal;

  // Hash router.
  window.addEventListener('hashchange', router);
  router();
});
