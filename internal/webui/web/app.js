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

  if (screen === 'quickchat') {
    renderQuickChatScreen(app);
  } else if (screen === 'agents' && rest.length === 0) {
    renderAgentsScreen(app);
  } else if (screen === 'agents' && rest[0] === 'new') {
    renderNewAgentScreen(app);
  } else if (screen === 'agents' && rest.length >= 2 && rest[1] === 'sessions') {
    renderSessionsScreen(app, rest[0]);
  } else if (screen === 'chat' && rest[0]) {
    renderChatScreen(app, rest[0]);
  } else if (screen === 'templates') {
    renderTemplatesScreen(app);
  } else if (screen === 'prompt') {
    renderPromptScreen(app);
  } else if (screen === 'connect') {
    renderConnectScreen(app);
  } else if (screen === 'docs') {
    renderDocsScreen(app);
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
    <div id="global-stats" class="global-stats" aria-label="Global usage statistics"></div>
    <div id="agent-list" role="list"><div class="empty">Loading…</div></div>
  `;
  document.getElementById('btn-new-agent').onclick = () => navigate('agents/new');

  // Load global usage stats (non-blocking — failure hides the bar)
  (async () => {
    try {
      const usage = await apiJSON('GET', '/usage');
      const statsEl = document.getElementById('global-stats');
      if (!statsEl) return;
      const totalTokens = (usage.total_input_tokens || 0) + (usage.total_output_tokens || 0)
        + (usage.total_cache_creation_input_tokens || 0) + (usage.total_cache_read_input_tokens || 0);
      const cost = usage.total_cost_usd || 0;
      const turns = usage.turn_count || 0;
      if (turns === 0 && cost === 0) return; // nothing to show on empty db
      statsEl.innerHTML = `<span aria-label="Global usage">Global:</span>
        <span>$${cost.toFixed(4)}</span>
        <span>·</span>
        <span>${turns} turns</span>
        <span>·</span>
        <span>${formatTokens(totalTokens)} tokens</span>`;
    } catch (_) {
      // endpoint not yet available — stay hidden
    }
  })();

  let agents;
  try {
    agents = await apiJSON('GET', '/agents');
  } catch (e) {
    document.getElementById('agent-list').innerHTML =
      `<div class="empty">Error: ${esc(e.message)}</div>`;
    return;
  }
  agents = agents.filter(a => a.name !== 'Quick Chat');
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
    card.setAttribute('role', 'listitem');
    const subAgentsBadges = (a.sub_agents && a.sub_agents.length)
      ? a.sub_agents.map(s => `<span class="badge">${esc(s)}</span>`).join(' ')
      : '';
    card.innerHTML = `
      <div class="card-body">
        <div class="card-title">${esc(a.name)}</div>
        <div class="card-meta">id: ${esc(a.id)} &nbsp;·&nbsp; model: ${esc(a.model)}${subAgentsBadges ? ' &nbsp;·&nbsp; sub-agents: ' + subAgentsBadges : ''}</div>
      </div>
      <div class="card-actions">
        <button class="small" data-action="sessions" data-id="${esc(a.id)}" aria-label="View sessions for ${esc(a.name)}">Sessions</button>
        <button class="small" data-action="install" data-id="${esc(a.id)}" data-name="${esc(a.name)}" aria-label="Install ${esc(a.name)}">Install</button>
        <button class="small" data-action="uninstall" data-id="${esc(a.id)}" data-name="${esc(a.name)}" aria-label="Uninstall ${esc(a.name)}">Uninstall</button>
        <button class="small danger" data-action="delete" data-id="${esc(a.id)}" aria-label="Delete ${esc(a.name)}">Delete</button>
      </div>
    `;
    list.appendChild(card);
  });

  list.addEventListener('click', async e => {
    const btn = e.target.closest('button[data-action]');
    if (!btn) return;
    const { action, id, name } = btn.dataset;

    if (action === 'sessions') {
      navigate(`agents/${id}/sessions`);
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

    <div class="generate-section">
      <h2 class="docs-section">Generate with AI</h2>
      <p class="section-desc">Describe what you want the agent to do. Claude will generate the optimal configuration.</p>
      <div class="form-group">
        <label for="gen-desc" class="sr-only">Agent description</label>
        <textarea id="gen-desc" rows="3" placeholder="I want an agent that reviews Go code for security vulnerabilities and suggests fixes..." aria-label="Describe the agent you want to create"></textarea>
      </div>
      <div class="form-actions">
        <button class="primary" id="btn-generate">Generate with AI</button>
      </div>
      <div id="gen-status" role="status" aria-live="polite" style="margin-top:8px"></div>
    </div>

    <div class="divider"></div>

    <h2 class="docs-section">Or configure manually</h2>
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
    <div class="form-group">
      <label for="ag-sub-agents">Sub-Agents</label>
      <input type="text" id="ag-sub-agents" placeholder="code-reviewer, test-engineer, doc-writer" aria-label="Sub-agent names, comma separated">
      <small class="form-hint">Names of installed Claude Code agents this agent can delegate to. Comma-separated.</small>
    </div>
    <div class="form-actions">
      <button class="primary" id="btn-create-agent">Create</button>
      <button id="btn-cancel-agent">Cancel</button>
    </div>
    <div id="ag-error" role="alert" style="color:var(--danger);margin-top:10px;display:none"></div>
  `;

  // AI generation
  document.getElementById('btn-generate').onclick = async () => {
    const desc = document.getElementById('gen-desc').value.trim();
    if (!desc) return;
    const btn = document.getElementById('btn-generate');
    const statusEl = document.getElementById('gen-status');
    btn.disabled = true;
    btn.textContent = 'Generating…';
    statusEl.innerHTML = '';
    try {
      const generated = await apiJSON('POST', '/agents/generate', { description: desc });
      // Pre-fill the manual form with generated values
      document.getElementById('ag-name').value = generated.name || '';
      document.getElementById('ag-prompt').value = generated.system_prompt || '';
      if (generated.model) {
        const sel = document.getElementById('ag-model');
        for (const opt of sel.options) {
          if (opt.value === generated.model) { opt.selected = true; break; }
        }
      }
      if (generated.sub_agents && generated.sub_agents.length) {
        document.getElementById('ag-sub-agents').value = generated.sub_agents.join(', ');
      }
      statusEl.innerHTML = '<span style="color:var(--accent2)">✓ Generated! Review and edit below, then click Create.</span>';
    } catch (e) {
      statusEl.innerHTML = '<span style="color:var(--danger)">Error: ' + esc(e.message) + '</span>';
    } finally {
      btn.disabled = false;
      btn.textContent = 'Generate with AI';
    }
  };

  // Manual create
  document.getElementById('btn-cancel-agent').onclick = () => navigate('agents');
  document.getElementById('btn-create-agent').onclick = async () => {
    const name = document.getElementById('ag-name').value.trim();
    const prompt = document.getElementById('ag-prompt').value.trim();
    const model = document.getElementById('ag-model').value.trim() || 'sonnet';
    const subAgentsStr = document.getElementById('ag-sub-agents').value.trim();
    const subAgents = subAgentsStr ? subAgentsStr.split(',').map(s => s.trim()).filter(Boolean) : [];
    const errEl = document.getElementById('ag-error');
    errEl.style.display = 'none';
    if (!name) { showFieldError(errEl, 'Name is required.'); return; }
    try {
      await apiJSON('POST', '/agents', { name, system_prompt: prompt, model, sub_agents: subAgents });
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
  const agent = state.agents.find(a => a.id === agentId);
  const agentLabel = agent ? agent.name : (agentId ? agentId.slice(0, 8) + '…' : '');
  const titleSuffix = agentLabel ? ` — ${agentLabel}` : '';
  container.innerHTML = `
    <a class="back" href="#agents">← Back to agents</a>
    <div class="screen-header">
      <h1>Sessions${esc(titleSuffix)}</h1>
      <button class="primary" id="btn-new-session">+ New session</button>
    </div>
    <div id="session-list" role="list"><div class="empty">Loading…</div></div>
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
    card.setAttribute('role', 'listitem');
    const sessionLabel = esc(s.name || s.id);
    card.innerHTML = `
      <div class="card-body">
        <div class="card-title">${sessionLabel}</div>
        <div class="card-meta">turns: ${s.turn_count} &nbsp;·&nbsp; last active: ${esc(s.last_active || '—')}</div>
        <div class="session-usage" id="usage-${esc(s.id)}" aria-label="Usage for session ${sessionLabel}"></div>
      </div>
      <div class="card-actions">
        <button class="small" data-action="open" data-id="${esc(s.id)}" aria-label="Open chat for ${sessionLabel}">Open chat</button>
        <button class="small danger" data-action="delete" data-id="${esc(s.id)}" aria-label="Delete session ${sessionLabel}">Delete</button>
      </div>
    `;
    list.appendChild(card);

    // Fetch usage for this session asynchronously — failure hides badge
    (async (sid) => {
      try {
        const usage = await apiJSON('GET', `/sessions/${sid}/usage`);
        const el = document.getElementById(`usage-${sid}`);
        if (!el) return;
        const totalTokens = (usage.total_input_tokens || 0) + (usage.total_output_tokens || 0)
          + (usage.total_cache_creation_input_tokens || 0) + (usage.total_cache_read_input_tokens || 0);
        const cost = usage.total_cost_usd || 0;
        const turns = usage.turn_count || 0;
        if (turns === 0 && cost === 0) return;
        el.innerHTML = `<span class="badge" aria-label="Session cost and tokens">$${cost.toFixed(4)} · ${turns} turns · ${formatTokens(totalTokens)} tokens</span>`;
      } catch (_) {
        // endpoint unavailable or session has no usage — stay hidden
      }
    })(s.id);
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

/* ── quick chat screen ── */

async function renderQuickChatScreen(container) {
  container.innerHTML = `
    <div class="screen-header"><h1>Quick Chat</h1></div>
    <p class="section-desc">Pick a model and start chatting — no agent setup needed.</p>
    <div class="model-grid" id="model-grid" role="list" aria-label="Available models"></div>
    <div id="quickchat-status" role="status" aria-live="polite" style="margin-top:12px"></div>
  `;

  const models = [
    { id: 'sonnet', label: 'Sonnet', desc: 'Fast, balanced — best for most tasks' },
    { id: 'opus', label: 'Opus', desc: 'Most capable — complex reasoning' },
    { id: 'haiku', label: 'Haiku', desc: 'Fastest — quick tasks' },
    { id: 'claude-opus-4-6[1m]', label: 'Opus 4.6 [1M]', desc: 'Opus with 1M context window' },
  ];

  const grid = document.getElementById('model-grid');
  models.forEach(m => {
    const card = document.createElement('button');
    card.className = 'model-card';
    card.setAttribute('role', 'listitem');
    card.setAttribute('aria-label', `${esc(m.label)}: ${esc(m.desc)}`);
    card.innerHTML = `<div class="model-card-title">${esc(m.label)}</div><div class="model-card-desc">${esc(m.desc)}</div>`;
    card.onclick = () => startQuickChat(container, m.id);
    grid.appendChild(card);
  });
}

async function startQuickChat(container, model) {
  const statusEl = document.getElementById('quickchat-status');
  statusEl.innerHTML = '<div class="empty">Starting session…</div>';

  // Disable all model cards
  document.querySelectorAll('.model-card').forEach(c => { c.disabled = true; });

  try {
    const agent = await apiJSON('POST', '/agents', {
      name: 'Quick Chat',
      system_prompt: 'You are a helpful assistant.',
      model: model,
    });
    const session = await apiJSON('POST', `/agents/${agent.id}/sessions`, { name: 'quick' });
    navigate(`chat/${session.id}`);
  } catch (e) {
    statusEl.innerHTML = `<div class="empty" style="color:var(--danger)">Error: ${esc(e.message)}</div>`;
    document.querySelectorAll('.model-card').forEach(c => { c.disabled = false; });
  }
}

/* ── chat screen ── */

async function renderChatScreen(container, sessionId) {
  container.innerHTML = `
    <a class="back" href="#agents">← Back</a>
    <div class="chat-header" aria-live="polite"></div>
    <div class="chat-container">
      <div id="chat-messages" class="chat-messages" aria-live="polite" aria-label="Chat messages" role="log"></div>
      <div class="chat-input-row">
        <label for="chat-input" class="sr-only">Message</label>
        <textarea id="chat-input" placeholder="Type a message… (Shift+Enter for newline, Enter to send)" aria-label="Message input"></textarea>
        <button class="primary" id="chat-send" aria-label="Send message">Send</button>
      </div>
    </div>
  `;

  const messagesEl = document.getElementById('chat-messages');
  const inputEl = document.getElementById('chat-input');
  const sendBtn = document.getElementById('chat-send');

  // Load session details to show agent name and model in header.
  try {
    const sess = await apiJSON('GET', `/sessions/${sessionId}`);
    const agent = await apiJSON('GET', `/agents/${sess.agent_id}`);
    const header = container.querySelector('.chat-header');
    if (header) {
      header.textContent = `${agent.name} — ${agent.model}`;
    }
  } catch (_) {}

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
      const result = await streamMessage(sessionId, content, chunk => {
        assistantBubble.textContent += chunk;
        scrollBottom(messagesEl);
      });
      if (result && result.usage) {
        appendUsageBadge(messagesEl, result);
        scrollBottom(messagesEl);
      }
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
  let resultData = null;

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });

    const events = buffer.split('\n\n');
    buffer = events.pop();

    for (const event of events) {
      const lines = event.split('\n');
      const eventLine = lines.find(l => l.startsWith('event:'));
      const dataLine = lines.find(l => l.startsWith('data:'));
      if (!dataLine) continue;

      const eventType = eventLine ? eventLine.slice('event:'.length).trim() : '';
      const raw = dataLine.slice('data:'.length).trimStart();

      try {
        const payload = JSON.parse(raw);
        if (eventType === 'text_delta' && payload.text) {
          onChunk(payload.text);
        } else if (eventType === 'result') {
          resultData = payload;
        }
      } catch (_) {}
    }
  }

  return resultData;
}

/* ── helpers ── */

function appendUsageBadge(container, result) {
  const u = result.usage;
  if (!u) return;

  const input = u.input_tokens || 0;
  const output = u.output_tokens || 0;
  const cacheRead = u.cache_read_input_tokens || 0;
  const cacheCreate = u.cache_creation_input_tokens || 0;
  const total = input + output + cacheRead + cacheCreate;
  const contextWindow = u.context_window || 0;
  const cost = result.cost_usd || 0;

  let pctText = '';
  if (contextWindow > 0) {
    const pct = ((total / contextWindow) * 100).toFixed(1);
    pctText = ` · ${pct}% of ${formatTokens(contextWindow)}`;
  }

  const div = document.createElement('div');
  div.className = 'usage-badge';
  div.innerHTML = `
    <span>in: ${formatTokens(input)}</span>
    <span>out: ${formatTokens(output)}</span>
    <span>cache: ${formatTokens(cacheRead + cacheCreate)}</span>
    <span>total: ${formatTokens(total)}${pctText}</span>
    <span>cost: $${cost.toFixed(4)}</span>
  `;
  container.appendChild(div);
}

function formatTokens(n) {
  if (n >= 1e6) return (n / 1e6).toFixed(1) + 'M';
  if (n >= 1e3) return (n / 1e3).toFixed(1) + 'k';
  return String(n);
}

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

/* ── templates screen ── */

async function renderTemplatesScreen(container) {
  container.innerHTML = `
    <div class="screen-header"><h1>Agent Templates</h1></div>
    <p class="section-desc">Pre-built agent configurations with optimized prompts. Click to install.</p>
    <div id="template-list" role="list"><div class="empty">Loading…</div></div>
  `;

  let templates;
  try {
    templates = await apiJSON('GET', '/api/templates');
  } catch (e) {
    document.getElementById('template-list').innerHTML = '<div class="empty">Error: ' + esc(e.message) + '</div>';
    return;
  }

  const list = document.getElementById('template-list');
  if (!templates.length) {
    list.innerHTML = '<div class="empty">No templates available.</div>';
    return;
  }
  list.innerHTML = '';
  templates.forEach(t => {
    const card = document.createElement('div');
    card.className = 'card';
    card.setAttribute('role', 'listitem');
    card.innerHTML = `
      <div class="card-body">
        <div class="card-title">${esc(t.name)}</div>
        <div class="card-meta">${esc(t.description)} · model: ${esc(t.model)}</div>
      </div>
      <div class="card-actions">
        <button class="primary small" data-action="install-template" aria-label="Create agent from template: ${esc(t.name)}">Create Agent</button>
      </div>
    `;
    card.querySelector('button').onclick = async () => {
      const btn = card.querySelector('button');
      btn.disabled = true;
      btn.textContent = 'Creating…';
      try {
        await apiJSON('POST', '/agents', {
          name: t.name,
          system_prompt: t.system_prompt,
          model: t.model,
          allowed_tools: t.allowed_tools,
          max_turns: t.max_turns,
        });
        btn.textContent = '✓ Created';
        btn.className = 'small badge green';
      } catch (e) {
        btn.textContent = 'Error';
        btn.disabled = false;
      }
    };
    list.appendChild(card);
  });
}

/* ── prompt screen ── */

async function renderPromptScreen(container) {
  container.innerHTML = `
    <div class="screen-header">
      <h1>AI Prompt</h1>
      <button class="primary" id="btn-copy-prompt">Copy to clipboard</button>
    </div>
    <p class="section-desc">Paste this prompt into any AI to give it full context on how to use claudio.</p>
    <pre id="prompt-text" class="code-block">Loading…</pre>
  `;

  try {
    const res = await fetch('/api/prompt');
    const text = await res.text();
    document.getElementById('prompt-text').textContent = text;

    document.getElementById('btn-copy-prompt').onclick = async () => {
      try {
        await navigator.clipboard.writeText(text);
        const btn = document.getElementById('btn-copy-prompt');
        btn.textContent = '✓ Copied!';
        setTimeout(() => { btn.textContent = 'Copy to clipboard'; }, 2000);
      } catch (_) {
        alert('Copy failed — select the text manually.');
      }
    };
  } catch (e) {
    document.getElementById('prompt-text').textContent = 'Error loading prompt: ' + e.message;
  }
}

/* ── docs screen ── */

function renderDocsScreen(container) {
  container.innerHTML = `
    <div class="screen-header"><h1>API Reference</h1></div>

    <h2 class="docs-section">Models</h2>
    <table class="docs-table">
      <tr><td><strong>sonnet</strong></td><td>claude-sonnet-4-6</td><td>200k context</td><td>Fast, balanced — default</td></tr>
      <tr><td><strong>opus</strong></td><td>claude-opus-4-7</td><td>200k context</td><td>Most capable</td></tr>
      <tr><td><strong>haiku</strong></td><td>claude-haiku-4-5</td><td>200k context</td><td>Fastest, cheapest</td></tr>
      <tr><td><strong>claude-opus-4-6[1m]</strong></td><td>claude-opus-4-6[1m]</td><td>1M context</td><td>Opus 4.6 extended context</td></tr>
    </table>
    <p class="section-desc">Use the alias (sonnet, opus, haiku) or the full model ID. OpenAI aliases: gpt-4 → opus, gpt-4o → sonnet, gpt-3.5-turbo → haiku.</p>

    <h2 class="docs-section">Agents</h2>

    <h3 class="docs-sub">POST /agents — Create agent</h3>
    <pre class="code-block">curl -X POST http://localhost:18080/agents \\
  -H "Content-Type: application/json" \\
  -d '{
    "name": "code-reviewer",
    "system_prompt": "You are an expert code reviewer.",
    "model": "sonnet",
    "allowed_tools": ["Bash", "Read", "Grep"],
    "max_turns": 10,
    "permission_mode": "auto-accept"
  }'</pre>
    <p class="section-desc">Response (201): Agent object with id, name, model, created_at, updated_at.</p>

    <h3 class="docs-sub">GET /agents — List agents</h3>
    <pre class="code-block">curl http://localhost:18080/agents</pre>

    <h3 class="docs-sub">GET /agents/{id} — Get agent</h3>
    <pre class="code-block">curl http://localhost:18080/agents/{id}</pre>

    <h3 class="docs-sub">PUT /agents/{id} — Update agent</h3>
    <pre class="code-block">curl -X PUT http://localhost:18080/agents/{id} \\
  -H "Content-Type: application/json" \\
  -d '{"name": "new-name", "model": "opus"}'</pre>

    <h3 class="docs-sub">DELETE /agents/{id} — Delete agent</h3>
    <pre class="code-block">curl -X DELETE http://localhost:18080/agents/{id}</pre>
    <p class="section-desc">Returns 409 if agent has active sessions.</p>

    <h3 class="docs-sub">POST /agents/{id}/install — Install to Claude Code</h3>
    <pre class="code-block">curl -X POST http://localhost:18080/agents/{id}/install</pre>
    <p class="section-desc">Creates .claude/agents/{name}.md — agent becomes available as @agent-name in Claude Code.</p>

    <h3 class="docs-sub">DELETE /agents/{id}/install — Uninstall from Claude Code</h3>
    <pre class="code-block">curl -X DELETE http://localhost:18080/agents/{id}/install</pre>

    <h2 class="docs-section">Sessions</h2>

    <h3 class="docs-sub">POST /agents/{aid}/sessions — Create session</h3>
    <pre class="code-block">curl -X POST http://localhost:18080/agents/{agent_id}/sessions \\
  -H "Content-Type: application/json" \\
  -d '{"name": "my-session"}'</pre>
    <p class="section-desc">Response (201): Session object with id, agent_id, turn_count, created_at.</p>

    <h3 class="docs-sub">GET /agents/{aid}/sessions — List sessions</h3>
    <pre class="code-block">curl http://localhost:18080/agents/{agent_id}/sessions</pre>

    <h3 class="docs-sub">GET /sessions/{sid} — Get session</h3>
    <pre class="code-block">curl http://localhost:18080/sessions/{session_id}</pre>

    <h3 class="docs-sub">DELETE /sessions/{sid} — Delete session</h3>
    <pre class="code-block">curl -X DELETE http://localhost:18080/sessions/{session_id}</pre>
    <p class="section-desc">Returns 409 if session has an in-flight request.</p>

    <h2 class="docs-section">Messages</h2>

    <h3 class="docs-sub">POST /sessions/{sid}/message — Send message (SSE stream)</h3>
    <pre class="code-block">curl -N -X POST http://localhost:18080/sessions/{session_id}/message \\
  -H "Content-Type: application/json" \\
  -d '{"content": "Review this function for bugs"}'</pre>
    <p class="section-desc">Response is a Server-Sent Events stream:</p>
    <pre class="code-block">event: session_init
data: {"session_id": "s1e2s3s4-..."}

event: text_delta
data: {"text": "Looking at the function..."}

event: text_delta
data: {"text": " I found a potential null pointer issue."}

event: result
data: {"text": "full response", "cost_usd": 0.0756, "usage": {
  "input_tokens": 6, "output_tokens": 42,
  "cache_read_input_tokens": 32000,
  "cache_creation_input_tokens": 3000,
  "context_window": 200000
}}</pre>

    <h3 class="docs-sub">GET /sessions/{sid}/messages — Message history</h3>
    <pre class="code-block">curl http://localhost:18080/sessions/{session_id}/messages</pre>
    <p class="section-desc">Response: array of {role, content} objects from Claude CLI's session JSONL files.</p>

    <h2 class="docs-section">OpenAI-Compatible</h2>

    <h3 class="docs-sub">GET /v1/models — List models</h3>
    <pre class="code-block">curl http://localhost:18080/v1/models</pre>

    <h3 class="docs-sub">POST /v1/chat/completions — Chat (non-streaming)</h3>
    <pre class="code-block">curl -X POST http://localhost:18080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "sonnet",
    "messages": [
      {"role": "system", "content": "You are helpful."},
      {"role": "user", "content": "Hello!"}
    ],
    "stream": false
  }'</pre>
    <p class="section-desc">Response includes usage: {prompt_tokens, completion_tokens, total_tokens}.</p>

    <h3 class="docs-sub">POST /v1/chat/completions — Chat (streaming)</h3>
    <pre class="code-block">curl -N -X POST http://localhost:18080/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -d '{"model": "sonnet", "messages": [{"role": "user", "content": "Hello"}], "stream": true}'</pre>
    <p class="section-desc">Returns SSE chunks in OpenAI format, ending with data: [DONE].</p>

    <h3 class="docs-sub">X-Agent-Id header</h3>
    <pre class="code-block">curl -X POST http://localhost:18080/v1/chat/completions \\
  -H "X-Agent-Id: {agent_id}" \\
  -H "Content-Type: application/json" \\
  -d '{"model": "sonnet", "messages": [{"role": "user", "content": "Review this"}], "stream": false}'</pre>
    <p class="section-desc">Routes the request through a specific agent's system prompt and config.</p>

    <h2 class="docs-section">System</h2>

    <h3 class="docs-sub">GET /health</h3>
    <pre class="code-block">curl http://localhost:18080/health</pre>
    <pre class="code-block">{"status": "ok", "claude_auth": "authenticated", "version": "0.3.0", "claude_version": "2.1.140"}</pre>

    <h3 class="docs-sub">GET /api/prompt</h3>
    <pre class="code-block">curl http://localhost:18080/api/prompt</pre>
    <p class="section-desc">Returns the full AI-ready usage prompt as plain text. Paste into any AI for instant context.</p>

    <h2 class="docs-section">Authentication</h2>
    <p class="section-desc">Set ADAPTER_API_KEY to require Bearer token on all API routes (except /health, / and /web/*):</p>
    <pre class="code-block">ADAPTER_API_KEY=my-secret claudio web
curl -H "Authorization: Bearer my-secret" http://localhost:18080/agents</pre>

    <h2 class="docs-section">How to Write Good Agent Prompts</h2>
    <p class="section-desc">The system prompt is the most important part of an agent. A vague prompt creates a generic agent. A structured prompt creates a specialist.</p>

    <h3 class="docs-sub">Recommended structure</h3>
    <pre class="code-block">## Purpose
One or two sentences: what this agent does and its primary goal.

## Constraints
- What the agent MUST always do
- What the agent MUST NEVER do
- Quality standards and boundaries
- Tools it should prefer or avoid

## Workflow
1. First, understand the input/context
2. Then, analyze/process/execute the main task
3. Finally, format and deliver the output

## Output Format
How the agent should structure its responses:
- Summaries first, details after
- Use headers/sections for long outputs
- Include code examples when relevant</pre>

    <h3 class="docs-sub">Good vs bad prompts</h3>
    <table class="docs-table">
      <tr><td style="color:var(--danger)">Bad</td><td>"You are a code reviewer"</td></tr>
      <tr><td style="color:var(--accent2)">Good</td><td>"You are a security-focused code reviewer. Check every change for: injection vulnerabilities, auth bypass, data exposure, and unsafe deserialization. For each finding: show the file:line, explain the attack vector, rate severity (critical/high/medium/low), and provide a fix."</td></tr>
    </table>
    <table class="docs-table">
      <tr><td style="color:var(--danger)">Bad</td><td>"Help me write tests"</td></tr>
      <tr><td style="color:var(--accent2)">Good</td><td>"You are a TDD enforcer. For every feature: 1) Write a failing test first (RED), 2) Write minimum code to pass (GREEN), 3) Refactor. Use table-driven tests. Test edge cases: nil, empty, zero, max, concurrent access. Never mock the system under test."</td></tr>
    </table>

    <h3 class="docs-sub">Tips</h3>
    <table class="docs-table">
      <tr><td><strong>Be specific</strong></td><td>Name the exact checks, formats, and steps you want</td></tr>
      <tr><td><strong>Set boundaries</strong></td><td>Tell the agent what NOT to do — it prevents scope creep</td></tr>
      <tr><td><strong>Define output</strong></td><td>If you want bullet points, say so. If you want JSON, show the schema</td></tr>
      <tr><td><strong>Give examples</strong></td><td>One input/output example is worth 100 words of description</td></tr>
      <tr><td><strong>Choose the right model</strong></td><td>haiku for quick tasks, sonnet for balanced work, opus for complex reasoning</td></tr>
      <tr><td><strong>Limit tools</strong></td><td>Only give the tools the agent needs — fewer tools = more focused behavior</td></tr>
    </table>
  `;
}

/* ── connect screen ── */

async function renderConnectScreen(container) {
  container.innerHTML = `
    <div class="screen-header"><h1>MCP Connections</h1></div>
    <p class="section-desc">Connect claudio to Claude Code and Claude Desktop as an MCP server.</p>
    <div id="connect-targets"><div class="empty">Loading…</div></div>
  `;

  try {
    const status = await apiJSON('GET', '/api/connect/status');
    const targets = document.getElementById('connect-targets');
    if (!targets) return;
    targets.innerHTML =
      renderConnectTarget('claude-code', 'Claude Code', status['claude-code']) +
      renderConnectTarget('claude-desktop', 'Claude Desktop', status['claude-desktop']);

    targets.addEventListener('click', async e => {
      const btn = e.target.closest('button[data-action]');
      if (!btn) return;
      const { action, target: tgt } = btn.dataset;
      await toggleConnect(tgt, action === 'connect', container);
    });
  } catch (err) {
    const targets = document.getElementById('connect-targets');
    if (targets) {
      targets.innerHTML = `<div class="empty" style="color:var(--danger)">Error: ${esc(err.message)}</div>`;
    }
  }
}

function renderConnectTarget(target, label, connected) {
  const badge = connected
    ? `<span class="badge badge-ok">Connected</span>`
    : `<span class="badge">Not Connected</span>`;
  const btn = connected
    ? `<button data-action="disconnect" data-target="${esc(target)}" aria-label="Disconnect ${esc(label)}">Disconnect</button>`
    : `<button data-action="connect" data-target="${esc(target)}" aria-label="Connect ${esc(label)}">Connect</button>`;
  return `
    <div class="card">
      <div class="card-body">
        <div class="card-title">${esc(label)}</div>
        <div class="card-meta">${badge}</div>
      </div>
      <div class="card-actions">${btn}</div>
    </div>
  `;
}

async function toggleConnect(target, doConnect, container) {
  const method = doConnect ? 'POST' : 'DELETE';
  try {
    await apiJSON(method, `/api/connect/${target}`);
    renderConnectScreen(container);
  } catch (err) {
    const targets = document.getElementById('connect-targets');
    if (targets) {
      targets.insertAdjacentHTML('beforeend', `<div class="empty" style="color:var(--danger)">Error: ${esc(err.message)}</div>`);
    }
  }
}

/* ── health check ── */

async function checkHealth() {
  try {
    const res = await fetch('/health');
    const data = await res.json();
    const banner = document.getElementById('health-banner');
    if (data.status === 'degraded' || data.claude_auth !== 'authenticated') {
      banner.textContent = '⚠ Claude CLI not authenticated — run: claude auth login';
      banner.classList.remove('hidden');
    } else {
      banner.classList.add('hidden');
    }
  } catch (_) {}
}

/* ── bootstrap ── */

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('key-save').onclick = () => {
    state.apiKey = document.getElementById('key-input').value.trim();
    hideKeyModal();
    router();
  };
  document.getElementById('key-btn').onclick = showKeyModal;

  window.addEventListener('hashchange', router);
  checkHealth();
  router();
});
