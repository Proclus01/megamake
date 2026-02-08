package httpserver

import "net/http"

const uiIndexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width,initial-scale=1" />
  <title>Megamake Chat</title>
  <link rel="stylesheet" href="/ui/styles.css" />
</head>
<body>
  <header class="topbar">
    <div class="brand">
      <div class="logo">MEGA</div>
      <div>
        <div class="title">Megamake Chat</div>
        <div class="subtitle">Runs: &lt;artifactDir&gt;/MEGACHAT/runs/&lt;run_name&gt;/</div>
      </div>
    </div>

    <div class="actions">
      <button id="btnRefresh" class="btn">Refresh</button>
      <button id="btnNew" class="btn primary">New</button>
    </div>
  </header>

  <main class="grid">
    <aside class="panel">
      <div class="panelHead">
        <div class="panelTitle">Conversations</div>
        <div class="panelMeta" id="runsMeta">—</div>
      </div>
      <div id="runsList" class="list"></div>
    </aside>

    <section class="panel">
      <div class="panelHead">
        <div class="panelTitle" id="convTitle">No conversation selected</div>

        <div class="panelMetaStack">
          <div class="panelMeta" id="convMeta">—</div>
          <div class="panelMeta2" id="convPrompts" title="">system: — • developer: —</div>
        </div>
      </div>

      <div class="content">
        <div id="messages" class="messages"></div>

        <div class="composer">
          <div class="composerRow">
            <textarea id="inputMessage" placeholder="Type a message … (Cmd+Enter to send)" disabled></textarea>
          </div>
          <div class="composerRow right">
            <button id="btnStop" class="btn danger" disabled>Stop</button>
            <button id="btnSend" class="btn primary" disabled>Send</button>
          </div>

          <div class="preview">
            <div class="previewHead">
              <div>Streaming preview</div>
              <div class="previewMeta" id="jobMeta">—</div>
            </div>
            <pre id="streamPreview" class="previewBody"></pre>
          </div>

          <div class="hint" id="status">Ready.</div>
        </div>
      </div>
    </section>

    <aside class="panel">
      <div class="panelHead">
        <div class="panelTitle">New conversation</div>
        <div class="panelMeta">Create + settings</div>
      </div>

      <div class="form">
        <label>Title</label>
        <input id="newTitle" type="text" placeholder="Untitled Conversation" />

        <label>Provider</label>
        <input id="newProvider" type="text" placeholder="openai" />

        <label>Model</label>
        <input id="newModel" type="text" placeholder="gpt-5" />

        <div class="rowActions">
          <button id="btnFetchModels" class="btn">Fetch models</button>
          <button id="btnRefreshModels" class="btn" title="Bypass cache">Refresh models</button>
        </div>

        <div class="rowActions">
          <button id="btnVerifyProvider" class="btn">Verify provider</button>
        </div>

        <label>Models (from provider)</label>
        <select id="modelsSelect">
          <option value="">(fetch models)</option>
        </select>
        <div class="hint small" id="modelsMeta">—</div>

        <label>System</label>
        <textarea id="newSystem" rows="3" placeholder="You are …"></textarea>

        <label>Developer</label>
        <textarea id="newDeveloper" rows="3" placeholder="Follow these constraints …"></textarea>

        <button id="btnCreate" class="btn primary">Create</button>

        <div class="divider"></div>

        <div class="panelTitleMini">Selected run settings (Playground-like)</div>
        <div class="hint small" id="runSettingsMeta">Select a conversation to load settings.</div>
        <div class="hint small">
          Tip: Leaving provider/model blank means “inherit current run settings”.
        </div>

        <div class="rowActions">
          <button id="btnCopyMetaToOverrides" class="btn" disabled>Use run meta → overrides</button>
          <button id="btnClearOverrides" class="btn" disabled>Clear overrides</button>
        </div>

        <label>provider override (optional)</label>
        <input id="rsProvider" type="text" placeholder="inherit" disabled />

        <label>model override (optional)</label>
        <input id="rsModel" type="text" placeholder="inherit" disabled />

        <label>systemText</label>
        <textarea id="rsSystem" rows="3" placeholder="(optional) system message" disabled></textarea>

        <label>developerText</label>
        <textarea id="rsDeveloper" rows="3" placeholder="(optional) developer message" disabled></textarea>

        <label>textFormat</label>
        <select id="rsTextFormat" disabled>
          <option value="text">text</option>
          <option value="markdown">markdown</option>
          <option value="json">json</option>
        </select>

        <label>verbosity</label>
        <select id="rsVerbosity" disabled>
          <option value="low">low</option>
          <option value="medium">medium</option>
          <option value="high">high</option>
        </select>

        <label>effort</label>
        <select id="rsEffort" disabled>
          <option value="minimal">minimal</option>
          <option value="low">low</option>
          <option value="medium">medium</option>
          <option value="high">high</option>
        </select>

        <label class="rowCheck">
          <input id="rsSummaryAuto" type="checkbox" disabled />
          <span>summaryAuto</span>
        </label>

        <label>maxOutputTokens</label>
        <input id="rsMaxOutputTokens" type="number" min="1" step="1" value="999999" disabled />

        <div class="panelTitleMini" style="margin-top:8px;">Tools</div>
        <label class="rowCheck"><input id="rsToolWeb" type="checkbox" disabled /> <span>web_search</span></label>
        <label class="rowCheck"><input id="rsToolCI" type="checkbox" disabled /> <span>code_interpreter</span></label>
        <label class="rowCheck"><input id="rsToolFS" type="checkbox" disabled /> <span>file_search</span></label>
        <label class="rowCheck"><input id="rsToolImg" type="checkbox" disabled /> <span>image_generation</span></label>

        <button id="btnSaveRunSettings" class="btn primary" disabled>Save run settings</button>

        <div class="hint small">
          Stored at <code>runs/&lt;run_name&gt;/settings.json</code> and applied to subsequent turns.
        </div>
      </div>
    </aside>
  </main>

  <script src="/ui/app.js"></script>
</body>
</html>
`

const uiStylesCSS = `
:root{
  --bg: #0b0f16;
  --panel: #0f1520;
  --border: rgba(255,255,255,0.10);
  --text: rgba(255,255,255,0.92);
  --muted: rgba(255,255,255,0.65);
  --muted2: rgba(255,255,255,0.45);
  --accent: #4cc9f0;

  --btn: rgba(255,255,255,0.08);
  --btnHover: rgba(255,255,255,0.12);
  --btnPrimary: rgba(76,201,240,0.16);
  --btnPrimaryHover: rgba(76,201,240,0.22);

  --btnDanger: rgba(255, 70, 70, 0.12);
  --btnDangerHover: rgba(255, 70, 70, 0.18);

  --user: rgba(76,201,240,0.10);
  --assistant: rgba(255,255,255,0.06);
}
@media (prefers-color-scheme: light) {
  :root{
    --bg: #f6f7fb;
    --panel: #ffffff;
    --border: rgba(20,30,50,0.12);
    --text: rgba(10,14,20,0.92);
    --muted: rgba(10,14,20,0.65);
    --muted2: rgba(10,14,20,0.48);

    --btn: rgba(10,14,20,0.06);
    --btnHover: rgba(10,14,20,0.10);
    --btnPrimary: rgba(76,201,240,0.20);
    --btnPrimaryHover: rgba(76,201,240,0.28);

    --btnDanger: rgba(255, 70, 70, 0.14);
    --btnDangerHover: rgba(255, 70, 70, 0.22);

    --user: rgba(76,201,240,0.18);
    --assistant: rgba(10,14,20,0.05);
  }
}

*{ box-sizing:border-box; }
html,body{ height:100%; }
body{
  margin:0;
  font-family: ui-sans-serif, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial;
  color: var(--text);
  background: var(--bg);
}

.topbar{
  display:flex;
  align-items:center;
  justify-content:space-between;
  gap:16px;
  padding:12px 14px;
  border-bottom: 1px solid var(--border);
  background: color-mix(in srgb, var(--panel) 96%, transparent);
}

.brand{ display:flex; gap:12px; align-items:center; }
.logo{
  width:44px; height:44px;
  border-radius:10px;
  background: var(--btnPrimary);
  display:flex;
  align-items:center;
  justify-content:center;
  font-weight:700;
}
.title{ font-weight:700; }
.subtitle{ font-size:12px; color: var(--muted2); margin-top:2px; }

.actions{ display:flex; gap:8px; align-items:center; }

.grid{
  display:grid;
  grid-template-columns: 300px 1fr 420px;
  gap:12px;
  padding:12px;
  height: calc(100vh - 70px);
}

.panel{
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: 12px;
  overflow:hidden;
  display:flex;
  flex-direction:column;
  min-height:0;
}

.panelHead{
  padding:12px;
  border-bottom: 1px solid var(--border);
  display:flex;
  align-items:baseline;
  justify-content:space-between;
  gap:12px;
}
.panelTitle{ font-weight:700; }
.panelTitleMini{ font-weight:700; font-size:12px; color: var(--muted); }

.panelMeta{ font-size:12px; color: var(--muted2); white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
.panelMetaStack{ display:flex; flex-direction:column; align-items:flex-end; gap:2px; min-width: 220px; }
.panelMeta2{
  font-size:11px;
  color: var(--muted2);
  max-width: 360px;
  white-space:nowrap;
  overflow:hidden;
  text-overflow:ellipsis;
}

.list{ overflow:auto; padding:6px; }
.row{
  padding:10px;
  border-radius:10px;
  cursor:pointer;
  border: 1px solid transparent;
}
.row:hover{ background: var(--btn); }
.row.active{
  background: var(--btnPrimary);
  border-color: color-mix(in srgb, var(--accent) 40%, transparent);
}
.rowTitle{ font-weight:650; }
.rowSub{ font-size:12px; color: var(--muted2); margin-top:3px; }

.content{ display:flex; flex-direction:column; min-height:0; height:100%; }
.messages{
  flex: 1 1 auto;
  overflow:auto;
  padding: 12px;
  display:flex;
  flex-direction:column;
  gap:10px;
}
.msg{
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 10px;
  background: var(--assistant);
}
.msg.user{ background: var(--user); }
.msgHead{
  display:flex;
  justify-content:space-between;
  gap:10px;
  font-size:12px;
  color: var(--muted2);
  margin-bottom:6px;
}
.msgText{ white-space:pre-wrap; }

.composer{
  border-top: 1px solid var(--border);
  padding: 10px;
  display:flex;
  flex-direction:column;
  gap:8px;
}
.composerRow{ display:flex; gap:8px; }
.composerRow.right{ justify-content:flex-end; }

textarea, input, select{
  width:100%;
  padding:10px;
  border-radius:10px;
  border:1px solid var(--border);
  background: color-mix(in srgb, var(--panel) 88%, transparent);
  color: var(--text);
}
textarea{ resize: vertical; min-height: 56px; }

.btn{
  padding:9px 12px;
  border-radius:10px;
  border: 1px solid var(--border);
  background: var(--btn);
  color: var(--text);
  cursor:pointer;
}
.btn:hover{ background: var(--btnHover); }
.btn.primary{
  background: var(--btnPrimary);
  border-color: color-mix(in srgb, var(--accent) 40%, transparent);
}
.btn.primary:hover{ background: var(--btnPrimaryHover); }
.btn.danger{
  background: var(--btnDanger);
  border-color: color-mix(in srgb, #ff3b3b 35%, transparent);
}
.btn.danger:hover{ background: var(--btnDangerHover); }
.btn:disabled{ opacity:0.55; cursor:not-allowed; }

.form{
  padding:12px;
  overflow:auto;
  display:flex;
  flex-direction:column;
  gap:8px;
}
label{ font-size:12px; color: var(--muted2); }
.hint{ font-size:12px; color: var(--muted); }
.hint.small{ color: var(--muted2); }

.rowActions{ display:flex; gap:8px; align-items:center; }
.rowActions .btn{ flex: 1 1 0; }

.rowCheck{
  display:flex;
  align-items:center;
  gap:8px;
  font-size:12px;
  color: var(--muted2);
}
.rowCheck input{ width:auto; }

.divider{
  height:1px;
  background: var(--border);
  margin: 10px 0;
}

/* Streaming preview */
.preview{
  border: 1px solid var(--border);
  border-radius: 12px;
  overflow:hidden;
  background: color-mix(in srgb, var(--panel) 92%, transparent);
}
.previewHead{
  display:flex;
  justify-content:space-between;
  gap:10px;
  padding:8px 10px;
  border-bottom: 1px solid var(--border);
  font-size:12px;
  color: var(--muted2);
}
.previewMeta{ white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
.previewBody{
  margin:0;
  padding:10px;
  max-height: 180px;
  overflow:auto;
  white-space: pre-wrap;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Courier New", monospace;
  font-size: 12px;
}
`

const uiAppJS = `
(function(){
  const elRunsList = document.getElementById('runsList');
  const elRunsMeta = document.getElementById('runsMeta');
  const elConvTitle = document.getElementById('convTitle');
  const elConvMeta = document.getElementById('convMeta');
  const elConvPrompts = document.getElementById('convPrompts');

  const elMessages = document.getElementById('messages');
  const elStatus = document.getElementById('status');

  const elInputMessage = document.getElementById('inputMessage');
  const btnSend = document.getElementById('btnSend');
  const btnStop = document.getElementById('btnStop');

  const elStreamPreview = document.getElementById('streamPreview');
  const elJobMeta = document.getElementById('jobMeta');

  const btnRefresh = document.getElementById('btnRefresh');
  const btnNew = document.getElementById('btnNew');
  const btnCreate = document.getElementById('btnCreate');

  const newTitle = document.getElementById('newTitle');
  const newProvider = document.getElementById('newProvider');
  const newModel = document.getElementById('newModel');
  const newSystem = document.getElementById('newSystem');
  const newDeveloper = document.getElementById('newDeveloper');

  const btnFetchModels = document.getElementById('btnFetchModels');
  const btnRefreshModels = document.getElementById('btnRefreshModels');
  const btnVerifyProvider = document.getElementById('btnVerifyProvider');

  const modelsSelect = document.getElementById('modelsSelect');
  const modelsMeta = document.getElementById('modelsMeta');

  const runSettingsMeta = document.getElementById('runSettingsMeta');
  const rsProvider = document.getElementById('rsProvider');
  const rsModel = document.getElementById('rsModel');
  const rsSystem = document.getElementById('rsSystem');
  const rsDeveloper = document.getElementById('rsDeveloper');

  const rsTextFormat = document.getElementById('rsTextFormat');
  const rsVerbosity = document.getElementById('rsVerbosity');
  const rsEffort = document.getElementById('rsEffort');
  const rsSummaryAuto = document.getElementById('rsSummaryAuto');
  const rsMaxOutputTokens = document.getElementById('rsMaxOutputTokens');
  const rsToolWeb = document.getElementById('rsToolWeb');
  const rsToolCI = document.getElementById('rsToolCI');
  const rsToolFS = document.getElementById('rsToolFS');
  const rsToolImg = document.getElementById('rsToolImg');
  const btnSaveRunSettings = document.getElementById('btnSaveRunSettings');

  const btnCopyMetaToOverrides = document.getElementById('btnCopyMetaToOverrides');
  const btnClearOverrides = document.getElementById('btnClearOverrides');

  let currentRun = '';
  let currentMeta = null;

  let pollTimer = null;
  let jobStartedAt = 0;
  let currentJobID = '';

  function setStatus(s){ elStatus.textContent = s || ''; }

  async function apiGET(url){
    const r = await fetch(url, { cache: 'no-store' });
    const txt = await r.text();
    let j = null;
    try { j = txt ? JSON.parse(txt) : null; } catch { j = { ok:false, error: txt }; }
    return { status: r.status, json: j };
  }
  async function apiPOST(url, body){
    const r = await fetch(url, {
      method: 'POST',
      headers: { 'Content-Type':'application/json', 'Accept':'application/json' },
      body: JSON.stringify(body || {}),
      cache: 'no-store'
    });
    const txt = await r.text();
    let j = null;
    try { j = txt ? JSON.parse(txt) : null; } catch { j = { ok:false, error: txt }; }
    return { status: r.status, json: j };
  }
  async function apiGETText(url){
    const r = await fetch(url, { cache:'no-store' });
    const txt = await r.text();
    return { status: r.status, text: txt };
  }

  function esc(s){
    return String(s || '').replace(/[&<>"']/g, (c) => ({
      '&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'
    }[c]));
  }

  function snip(s, n){
    s = String(s || '').trim().replace(/\\s+/g, ' ');
    if (!s) return '—';
    if (s.length <= n) return s;
    return s.slice(0, n) + '…';
  }

  function setComposerEnabled(enabled){
    elInputMessage.disabled = !enabled;
    btnSend.disabled = !enabled;
    if (!enabled) btnStop.disabled = true;
    elInputMessage.placeholder = enabled
      ? "Type a message … (Cmd+Enter to send)"
      : "Select a conversation to enable messaging";
  }

  function setRunSettingsEnabled(enabled){
    rsProvider.disabled = !enabled;
    rsModel.disabled = !enabled;
    rsSystem.disabled = !enabled;
    rsDeveloper.disabled = !enabled;

    rsTextFormat.disabled = !enabled;
    rsVerbosity.disabled = !enabled;
    rsEffort.disabled = !enabled;
    rsSummaryAuto.disabled = !enabled;
    rsMaxOutputTokens.disabled = !enabled;
    rsToolWeb.disabled = !enabled;
    rsToolCI.disabled = !enabled;
    rsToolFS.disabled = !enabled;
    rsToolImg.disabled = !enabled;
    btnSaveRunSettings.disabled = !enabled;

    btnCopyMetaToOverrides.disabled = !enabled;
    btnClearOverrides.disabled = !enabled;
  }

  function clearPolling(){
    if (pollTimer){
      clearInterval(pollTimer);
      pollTimer = null;
    }
    jobStartedAt = 0;
    currentJobID = '';
    btnStop.disabled = true;
  }

  function setJobMeta(s){ elJobMeta.textContent = s || '—'; }

  function renderHeaderFromMeta(meta){
    if (!meta){
      elConvTitle.textContent = 'No conversation selected';
      elConvMeta.textContent = '—';
      elConvPrompts.textContent = 'system: — • developer: —';
      elConvPrompts.title = '';
      return;
    }
    elConvTitle.textContent = meta.title || '(untitled)';

    const model = meta.model || '—';
    const provider = meta.provider || '—';
    const turns = meta.turns_n ?? 0;
    const msgs = meta.messages_n ?? 0;
    elConvMeta.textContent = provider + ' / ' + model + ' • turns ' + turns + ' • messages ' + msgs;

    const sys = snip(meta.systemText || meta.system_text || '', 70);
    const dev = snip(meta.developerText || meta.developer_text || '', 70);
    const line = 'system: ' + sys + ' • developer: ' + dev;
    elConvPrompts.textContent = line;

    const fullSys = String(meta.systemText || meta.system_text || '').trim();
    const fullDev = String(meta.developerText || meta.developer_text || '').trim();
    elConvPrompts.title = 'system:\\n' + (fullSys || '—') + '\\n\\n' + 'developer:\\n' + (fullDev || '—');
  }

  function renderRuns(items){
    elRunsList.innerHTML = '';
    if (!items || items.length === 0){
      elRunsList.innerHTML = '<div class="hint" style="padding:10px;">No conversations.</div>';
      elRunsMeta.textContent = '0';
      return;
    }
    elRunsMeta.textContent = String(items.length);

    for (const m of items){
      const run = m.run_name || '';
      const title = m.title || '(untitled)';
      const model = m.model || '—';
      const upd = m.updated_ts || '';
      const turns = (m.turns_n ?? 0);

      const row = document.createElement('div');
      row.className = 'row' + (run === currentRun ? ' active' : '');
      row.innerHTML = '<div class="rowTitle">' + esc(title) + '</div>' +
        '<div class="rowSub">' + esc(model) + ' • turns ' + esc(turns) + ' • ' + esc(upd) + '</div>';

      row.addEventListener('click', () => { selectRun(run); });
      elRunsList.appendChild(row);
    }
  }

  function renderTranscript(meta, events){
    renderHeaderFromMeta(meta);

    elMessages.innerHTML = '';
    if (!meta) return;

    if (!events || events.length === 0){
      elMessages.innerHTML = '<div class="hint">No transcript yet.</div>';
      return;
    }

    for (const ev of events){
      const role = (ev.role || '').toLowerCase();
      const ts = ev.ts || '';
      const turn = ev.turn || 0;
      const text = ev.text || '';
      const div = document.createElement('div');
      div.className = 'msg' + (role === 'user' ? ' user' : '');
      div.innerHTML =
        '<div class="msgHead"><div>' + esc(role) + ' • turn ' + esc(turn) + '</div><div>' + esc(ts) + '</div></div>' +
        '<div class="msgText">' + esc(text) + '</div>';
      elMessages.appendChild(div);
    }
    elMessages.scrollTop = elMessages.scrollHeight;
  }

  async function refreshList(){
    const { json } = await apiGET('/api/chat/list?limit=200');
    if (!json || json.ok !== true){
      setStatus('List error: ' + (json && json.error ? json.error : 'unknown'));
      renderRuns([]);
      return;
    }
    renderRuns(json.items || []);
  }

  async function loadRun(runName){
    const { json } = await apiGET('/api/chat/get?run_name=' + encodeURIComponent(runName) + '&tail=800');
    if (!json || json.ok !== true){
      setStatus('Get error: ' + (json && json.error ? json.error : 'unknown'));
      return false;
    }
    currentMeta = json.meta || null;
    renderTranscript(json.meta, json.events);
    return true;
  }

  function providerForVerify(){
    const rs = (rsProvider.value || '').trim();
    if (rs) return rs;
    if (currentMeta && (currentMeta.provider || '').trim()) return (currentMeta.provider || '').trim();
    const np = (newProvider.value || '').trim();
    return np || 'openai';
  }

  async function verifyProvider(){
    const p = providerForVerify();
    setStatus('Verifying provider: ' + p + ' …');
    const { json } = await apiPOST('/api/chat/providers/verify', { provider: p, timeout_seconds: 20 });
    if (!json || json.ok !== true){
      setStatus('Verify failed: ' + (json && json.error ? json.error : 'unknown'));
      return;
    }
    const res = json.result || {};
    if (res.ok){
      setStatus('Verify OK: ' + (res.provider || p) + (res.message ? (' • ' + res.message) : ''));
    } else {
      setStatus('Verify NOT OK: ' + (res.provider || p) + (res.message ? (' • ' + res.message) : ''));
    }
  }

  function readRunSettingsFromUI(){
    return {
      provider: (rsProvider.value || '').trim(),
      model: (rsModel.value || '').trim(),
      systemText: (rsSystem.value || ''),
      developerText: (rsDeveloper.value || ''),

      textFormat: (rsTextFormat.value || 'text'),
      verbosity: (rsVerbosity.value || 'high'),
      effort: (rsEffort.value || 'high'),
      summaryAuto: !!rsSummaryAuto.checked,
      maxOutputTokens: Math.max(1, parseInt(rsMaxOutputTokens.value || '4096', 10) || 4096),
      tools: {
        web_search: !!rsToolWeb.checked,
        code_interpreter: !!rsToolCI.checked,
        file_search: !!rsToolFS.checked,
        image_generation: !!rsToolImg.checked
      }
    };
  }

  function applyRunSettingsToUI(settings, source, found){
    settings = settings || {};

    const curProv = (currentMeta && currentMeta.provider) ? currentMeta.provider : '';
    const curModel = (currentMeta && currentMeta.model) ? currentMeta.model : '';
    rsProvider.placeholder = curProv ? ('inherit (current: ' + curProv + ')') : 'inherit';
    rsModel.placeholder = curModel ? ('inherit (current: ' + curModel + ')') : 'inherit';

    rsProvider.value = settings.provider || '';
    rsModel.value = settings.model || '';
    rsSystem.value = settings.systemText || '';
    rsDeveloper.value = settings.developerText || '';

    rsTextFormat.value = settings.textFormat || 'text';
    rsVerbosity.value = settings.verbosity || 'high';
    rsEffort.value = settings.effort || 'high';
    rsSummaryAuto.checked = !!settings.summaryAuto;
    rsMaxOutputTokens.value = String(settings.maxOutputTokens || 4096);

    const tools = settings.tools || {};
    rsToolWeb.checked = !!tools.web_search;
    rsToolCI.checked = !!tools.code_interpreter;
    rsToolFS.checked = !!tools.file_search;
    rsToolImg.checked = !!tools.image_generation;

    runSettingsMeta.textContent = (found ? 'run override' : 'no run override') + ' • source: ' + (source || '—');
  }

  async function loadRunSettings(runName){
    setRunSettingsEnabled(false);
    runSettingsMeta.textContent = 'Loading…';

    const { json } = await apiGET('/api/chat/run/settings?run_name=' + encodeURIComponent(runName));
    if (!json || json.ok !== true){
      runSettingsMeta.textContent = 'Error: ' + (json && json.error ? json.error : 'unknown');
      setRunSettingsEnabled(false);
      return;
    }
    const res = json.result || {};
    applyRunSettingsToUI(res.settings, res.source, !!res.found);
    setRunSettingsEnabled(true);
  }

  async function saveRunSettings(){
    if (!currentRun){
      setStatus('Select a run first.');
      return;
    }
    setStatus('Saving run settings…');
    const settings = readRunSettingsFromUI();

    const { json } = await apiPOST('/api/chat/run/settings', { run_name: currentRun, settings });
    if (!json || json.ok !== true){
      setStatus('Save settings error: ' + (json && json.error ? json.error : 'unknown'));
      return;
    }
    setStatus('Run settings saved.');
    await loadRunSettings(currentRun);
    await refreshList();
    await loadRun(currentRun);
  }

  function copyMetaToOverrides(){
    if (!currentMeta){
      setStatus('No run meta loaded.');
      return;
    }
    rsProvider.value = (currentMeta.provider || '').trim();
    rsModel.value = (currentMeta.model || '').trim();
    rsSystem.value = currentMeta.systemText || currentMeta.system_text || '';
    rsDeveloper.value = currentMeta.developerText || currentMeta.developer_text || '';
    setStatus('Copied run meta into overrides (not saved yet).');
  }

  function clearOverrides(){
    rsProvider.value = '';
    rsModel.value = '';
    rsSystem.value = '';
    rsDeveloper.value = '';
    setStatus('Cleared overrides (not saved yet).');
  }

  async function selectRun(runName){
    if (!runName) return;
    currentRun = runName;

    clearPolling();
    elStreamPreview.textContent = '';
    setJobMeta('—');

    setComposerEnabled(true);
    await refreshList();
    setStatus('Loading run…');
    await loadRun(runName);
    await loadRunSettings(runName);
    setStatus('Ready.');
    elInputMessage.focus();
  }

  async function createRun(){
    const body = {
      title: (newTitle.value || 'Untitled Conversation').trim(),
      provider: (newProvider.value || '').trim(),
      model: (newModel.value || '').trim(),
      systemText: newSystem.value || '',
      developerText: newDeveloper.value || ''
    };
    setStatus('Creating…');
    const { json } = await apiPOST('/api/chat/new', body);
    if (!json || json.ok !== true){
      setStatus('New error: ' + (json && json.error ? json.error : 'unknown'));
      return;
    }
    const run = json.run_name || '';
    await refreshList();
    if (run){
      await selectRun(run);
    } else {
      setStatus('Created (no run_name returned?)');
    }
  }

  function fmtElapsed(ms){ return (ms/1000).toFixed(1) + 's'; }

  async function stopAndCancel(){
    if (!pollTimer || !currentJobID){
      setStatus('No active job.');
      return;
    }
    const jobID = currentJobID;
    btnStop.disabled = true;
    setStatus('Canceling…');

    try {
      const { json } = await apiPOST('/api/chat/jobs/cancel', { job_id: jobID });
      if (!json || json.ok !== true){
        setStatus('Cancel request failed; stopped polling locally. Error: ' + (json && json.error ? json.error : 'unknown'));
      } else {
        setStatus('Cancel requested.');
      }
    } catch (e) {
      setStatus('Cancel request failed; stopped polling locally. Error: ' + String(e));
    }

    clearPolling();
    btnSend.disabled = false;
    elInputMessage.disabled = false;
    setJobMeta('canceled (requested) • job ' + jobID);
    if (currentRun){
      await loadRun(currentRun);
    }
    elInputMessage.focus();
  }

  async function sendMessage(){
    if (!currentRun){
      setStatus('Select a conversation first.');
      return;
    }
    const msg = (elInputMessage.value || '').trim();
    if (!msg){
      setStatus('Enter a message.');
      return;
    }

    clearPolling();
    elStreamPreview.textContent = '';
    setJobMeta('Starting…');
    setStatus('Starting job…');

    btnSend.disabled = true;
    elInputMessage.disabled = true;

    const { json } = await apiPOST('/api/chat/run_async', { run_name: currentRun, message: msg });
    if (!json || json.ok !== true){
      setStatus('Run error: ' + (json && json.error ? json.error : 'unknown'));
      btnSend.disabled = false;
      elInputMessage.disabled = false;
      return;
    }

    const jobID = json.job_id || '';
    const turn = json.turn || 0;
    currentJobID = jobID;
    jobStartedAt = Date.now();
    elInputMessage.value = '';

    btnStop.disabled = false;

    pollTimer = setInterval(async () => {
      try {
        const elapsed = fmtElapsed(Date.now() - jobStartedAt);

        const st = await apiGET('/api/chat/jobs/status?job_id=' + encodeURIComponent(jobID));
        if (!st.json || st.json.ok !== true){
          setJobMeta('job ' + jobID + ' • ' + elapsed + ' • status error');
          setStatus('Job status error: ' + (st.json && st.json.error ? st.json.error : 'unknown'));
          return;
        }

        const job = st.json.job || {};
        const status = (job.status || '').toLowerCase();
        const pct = job.percent ?? 0;
        const jmsg = job.message || '';
        const err = job.error || '';

        setJobMeta('job ' + jobID + ' • turn ' + turn + ' • ' + status + ' • ' + pct + '% • ' + elapsed + (jmsg ? ' • ' + jmsg : ''));

        const tail = await apiGETText('/api/chat/jobs/tail?job_id=' + encodeURIComponent(jobID) + '&limit=16384');
        if (tail && typeof tail.text === 'string'){
          elStreamPreview.textContent = tail.text;
          elStreamPreview.scrollTop = elStreamPreview.scrollHeight;
        }

        if (status === 'done'){
          clearPolling();
          setStatus('Done.');
          await loadRun(currentRun);
          btnSend.disabled = false;
          elInputMessage.disabled = false;
          elInputMessage.focus();
          return;
        }

        if (status === 'canceled'){
          clearPolling();
          setStatus('Canceled.');
          await loadRun(currentRun);
          btnSend.disabled = false;
          elInputMessage.disabled = false;
          elInputMessage.focus();
          return;
        }

        if (status === 'error'){
          clearPolling();
          setStatus('Job error: ' + (err || 'unknown'));
          btnSend.disabled = false;
          elInputMessage.disabled = false;
          return;
        }
      } catch (e) {
        setStatus('Polling error: ' + String(e));
      }
    }, 650);
  }

  function clearModelsUI(){
    modelsSelect.innerHTML = '<option value="">(fetch models)</option>';
    modelsMeta.textContent = '—';
  }

  function modelsProviderName(){
    const p = (newProvider.value || '').trim();
    return p || 'openai';
  }

  async function fetchModels(noCache){
    const prov = modelsProviderName();
    setStatus((noCache ? 'Refreshing' : 'Fetching') + ' models for ' + prov + ' …');
    modelsMeta.textContent = 'loading…';

    const url = '/api/chat/providers/models?provider=' + encodeURIComponent(prov) +
      '&limit=500&timeout_seconds=25&cache_ttl_seconds=300' +
      (noCache ? '&no_cache=true' : '');

    const { json } = await apiGET(url);
    if (!json || json.ok !== true){
      const err = (json && json.error) ? json.error : 'unknown';
      modelsMeta.textContent = 'error: ' + err;
      setStatus('Models error: ' + err);
      return;
    }

    const res = json.result || {};
    const models = res.models || [];
    const cached = !!res.cached;
    const age = res.cache_age_s ?? 0;

    modelsSelect.innerHTML = '<option value="">(select a model)</option>';
    for (const m of models){
      const id = m.id || '';
      if (!id) continue;
      const opt = document.createElement('option');
      opt.value = id;
      opt.textContent = id;
      modelsSelect.appendChild(opt);
    }

    modelsMeta.textContent = (cached ? ('cached (age ' + age + 's)') : 'fresh') + ' • ' + models.length + ' model(s)';
    setStatus('Models loaded: ' + models.length + ' (' + (cached ? 'cached' : 'fresh') + ')');

    if (!newModel.value && models.length > 0 && models[0].id){
      newModel.value = models[0].id;
      modelsSelect.value = models[0].id;
    }
  }

  modelsSelect.addEventListener('change', () => {
    const v = (modelsSelect.value || '').trim();
    if (v){
      newModel.value = v;
      setStatus('Selected model: ' + v);
    }
  });

  btnFetchModels.addEventListener('click', () => fetchModels(false));
  btnRefreshModels.addEventListener('click', () => fetchModels(true));
  btnVerifyProvider.addEventListener('click', () => verifyProvider());

  btnRefresh.addEventListener('click', async () => {
    setStatus('Refreshing…');
    await refreshList();
    setStatus('Ready.');
  });

  btnNew.addEventListener('click', () => {
    newTitle.focus();
    setStatus('Fill the form and click Create.');
  });

  btnCreate.addEventListener('click', () => createRun());

  btnSend.addEventListener('click', () => sendMessage());
  btnStop.addEventListener('click', () => stopAndCancel());
  btnSaveRunSettings.addEventListener('click', () => saveRunSettings());
  btnCopyMetaToOverrides.addEventListener('click', () => copyMetaToOverrides());
  btnClearOverrides.addEventListener('click', () => clearOverrides());

  elInputMessage.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter'){
      sendMessage();
    }
  });

  newProvider.addEventListener('input', () => clearModelsUI());

  // initial state
  setComposerEnabled(false);
  setRunSettingsEnabled(false);
  clearModelsUI();
  setStatus('Loading conversations…');
  refreshList().then(() => setStatus('Ready.'));
})();
`

func registerUI(mux *http.ServeMux) {
	if mux == nil {
		return
	}

	mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(uiIndexHTML))
	})

	mux.HandleFunc("GET /ui/styles.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(uiStylesCSS))
	})

	mux.HandleFunc("GET /ui/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(uiAppJS))
	})
}
