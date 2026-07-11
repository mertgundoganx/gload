// ============================================
// gload — Frontend Application
// ============================================

const state = {
    services: [],
    testResults: null,
    eventSource: null,
    currentRoute: null,
    compareMode: false,
    compareSelected: [],
    settings: {},
    queue: null,
    filterTag: '',
    filterGroup: '',
    filterStatus: '',
    searchQuery: '',
    viewMode: localStorage.getItem('gload_view_mode') || 'grid',
    sortBy: localStorage.getItem('gload_sort_by') || 'manual',
    pinned: JSON.parse(localStorage.getItem('gload_pinned') || '[]'),
    collapsedGroups: {},
    liveTimeSeries: [],
    schedules: [],
    patterns: null,
    widgetOrder: JSON.parse(localStorage.getItem('gload_widget_order') || 'null') || ['stats', 'health', 'queue', 'recent', 'services'],
    hiddenWidgets: JSON.parse(localStorage.getItem('gload_hidden_widgets') || '[]'),
    historySort: { column: 'created_at', direction: 'desc' },
    dashboardPage: 1,
    historyPage: 1,
    theme: localStorage.getItem('gload_theme') || 'dark',
    formTab: 'basic',
    loadMode: 'simple',
    bulkMode: false,
    bulkSelected: [],
};

// ============================================
// Unsaved Changes Tracking
// ============================================

let formDirty = false;
function markFormDirty() { formDirty = true; }
function clearFormDirty() { formDirty = false; }

function setupFormTracking() {
    const form = document.getElementById('service-form');
    if (!form) return;
    form.addEventListener('input', markFormDirty);
    form.addEventListener('change', markFormDirty);
}

window.addEventListener('beforeunload', function(e) {
    if (formDirty) {
        e.preventDefault();
        e.returnValue = '';
    }
});

const SERVICES_PER_PAGE = 12;
const HISTORY_PER_PAGE = 10;

const METHOD_COLORS = {
    GET: '#10b981', POST: '#3b82f6', PUT: '#f59e0b',
    DELETE: '#ef4444', PATCH: '#8b5cf6',
};

// ============================================
// Toast Notifications
// ============================================

function toast(message, type = 'info', action = null) {
    const container = document.getElementById('toast-container');
    const icons = {
        success: '<svg class="w-4 h-4 shrink-0" style="color:#10b981" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>',
        error: '<svg class="w-4 h-4 shrink-0" style="color:#ef4444" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>',
        info: '<svg class="w-4 h-4 shrink-0" style="color:#06b6d4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>',
    };
    const el = document.createElement('div');
    el.className = `toast toast-${type}`;
    el.innerHTML = `${icons[type] || icons.info}<span>${esc(message)}</span>`;
    const dismiss = () => { el.classList.add('toast-out'); setTimeout(() => el.remove(), 250); };
    if (action && action.label && action.href) {
        const a = document.createElement('a');
        a.href = action.href;
        a.textContent = action.label;
        a.style.cssText = 'margin-left:auto;padding-left:14px;color:var(--accent-text);font-weight:600;white-space:nowrap;cursor:pointer;';
        a.addEventListener('click', dismiss);
        el.appendChild(a);
    }
    container.appendChild(el);
    // Actionable toasts linger longer so the user can reach the button.
    setTimeout(dismiss, action ? 7000 : 3000);
}

// ============================================
// Confirm Modal
// ============================================

function confirmModal(title, message, opts = {}) {
    const confirmLabel = opts.confirmLabel || 'Delete';
    const cancelLabel = opts.cancelLabel || 'Cancel';
    const confirmClass = opts.confirmClass || 'btn-danger';
    return new Promise(resolve => {
        const container = document.getElementById('modal-container');
        container.innerHTML = `
        <div class="modal-overlay" id="modal-overlay">
            <div class="modal-box">
                <h3 class="text-lg font-semibold mb-2" style="color:var(--text);">${esc(title)}</h3>
                <p class="text-sm mb-6" style="color:var(--text-muted);">${esc(message)}</p>
                <div class="flex items-center justify-end gap-3">
                    <button class="btn btn-ghost btn-sm" id="modal-cancel">${esc(cancelLabel)}</button>
                    <button class="btn ${confirmClass} btn-sm" id="modal-confirm">${esc(confirmLabel)}</button>
                </div>
            </div>
        </div>`;
        const cleanup = (result) => { container.innerHTML = ''; resolve(result); };
        document.getElementById('modal-cancel').onclick = () => cleanup(false);
        document.getElementById('modal-confirm').onclick = () => cleanup(true);
        document.getElementById('modal-overlay').onclick = (e) => { if (e.target.id === 'modal-overlay') cleanup(false); };
    });
}

// ============================================
// Validation
// ============================================

function validateServiceForm(form) {
    let valid = true;
    clearErrors(form);

    const name = form.querySelector('[name="name"]');
    const url = form.querySelector('[name="url"]');
    const concurrency = form.querySelector('[name="concurrency"]');
    const duration = form.querySelector('[name="duration"]');
    const timeout = form.querySelector('[name="timeout"]');

    if (!name.value.trim()) {
        showFieldError(name, 'Service name is required.'); valid = false;
    }

    const urlVal = url.value.trim();
    if (!urlVal) {
        showFieldError(url, 'URL is required.'); valid = false;
    } else if (!/^https?:\/\/.+/.test(urlVal)) {
        showFieldError(url, 'URL must start with http:// or https://'); valid = false;
    }

    const conc = parseInt(concurrency.value, 10);
    if (!conc || conc < 1) {
        showFieldError(concurrency, 'Must be at least 1.'); valid = false;
    } else if (conc > 10000) {
        showFieldError(concurrency, 'Maximum 10,000 workers.'); valid = false;
    }

    const dur = parseInt(duration.value, 10);
    if (!dur || dur < 1) {
        showFieldError(duration, 'Must be at least 1 second.'); valid = false;
    } else if (dur > 3600) {
        showFieldError(duration, 'Maximum 3600 seconds (1 hour).'); valid = false;
    }

    const tout = parseInt(timeout.value, 10);
    if (!tout || tout < 1) {
        showFieldError(timeout, 'Must be at least 1 second.'); valid = false;
    } else if (tout > 300) {
        showFieldError(timeout, 'Maximum 300 seconds (5 minutes).'); valid = false;
    }

    // Advanced field validations — pick visible input when duplicates exist
    function getVisibleVal(name) {
        let v = NaN;
        form.querySelectorAll(`[name="${name}"]`).forEach(inp => {
            if (inp.offsetParent !== null) v = parseFloat(inp.value);
        });
        return v;
    }

    const arrRate = getVisibleVal('arrival_rate');
    if (!isNaN(arrRate) && arrRate < 0) { toast('Arrival rate cannot be negative.', 'error'); valid = false; }

    const thinkMin = getVisibleVal('think_time_ms');
    const thinkMax = getVisibleVal('think_time_max_ms');
    if (!isNaN(thinkMin) && thinkMin < 0) { toast('Think time cannot be negative.', 'error'); valid = false; }
    if (!isNaN(thinkMax) && thinkMax < 0) { toast('Think time max cannot be negative.', 'error'); valid = false; }
    if (!isNaN(thinkMin) && !isNaN(thinkMax) && thinkMax > 0 && thinkMax < thinkMin) {
        toast('Think time max must be greater than think time min.', 'error'); valid = false;
    }

    const warmSec = getVisibleVal('warmup_seconds');
    if (!isNaN(warmSec) && warmSec < 0) { toast('Warm-up seconds cannot be negative.', 'error'); valid = false; }

    const warmConns = parseInt(form.querySelector('[name="warmup_conns"]')?.value, 10);
    if (!isNaN(warmConns) && warmConns < 0) { toast('Warm-up connections cannot be negative.', 'error'); valid = false; }

    const reqIter = parseInt(form.querySelector('[name="requests_per_iteration"]')?.value, 10);
    if (!isNaN(reqIter) && reqIter < 1) { toast('Requests per iteration must be at least 1.', 'error'); valid = false; }

    const adaptTarget = parseFloat(form.querySelector('[name="adaptive_target_ms"]')?.value);
    if (!isNaN(adaptTarget) && adaptTarget < 0) { toast('Adaptive target cannot be negative.', 'error'); valid = false; }

    const maxIdle = parseInt(form.querySelector('[name="max_idle_conns"]')?.value, 10);
    if (!isNaN(maxIdle) && maxIdle < 0) { toast('Max idle connections cannot be negative.', 'error'); valid = false; }

    // JSON field validations
    const protocolConfig = form.querySelector('[name="protocol_config"]')?.value?.trim();
    if (protocolConfig && protocolConfig !== '{}') {
        try { JSON.parse(protocolConfig); } catch (_) { toast('Protocol config must be valid JSON.', 'error'); valid = false; }
    }

    const dataSource = form.querySelector('[name="data_source"]')?.value?.trim();
    if (dataSource && dataSource !== '[]') {
        try { JSON.parse(dataSource); } catch (_) { toast('Data source must be valid JSON array.', 'error'); valid = false; }
    }

    return valid;
}

function isValidDuration(val) {
    if (!val) return false;
    return /^(\d+h)?(\d+m)?(\d+s)?$/.test(val) && /\d/.test(val);
}

// After a failed validation, jump to the tab holding the first errored field
// so the user always sees why the form won't submit.
function jumpToFirstError(form) {
    const firstErr = form.querySelector('.input-error');
    if (firstErr) {
        const tab = tabOfField(firstErr);
        if (tab && tab !== state.formTab) switchFormTab(tab);
        firstErr.focus();
        firstErr.scrollIntoView({ behavior: 'smooth', block: 'center' });
        toast('Please fix the highlighted field.', 'error');
    } else {
        toast('Please fix the errors before saving.', 'error');
    }
}

function showFieldError(input, msg) {
    input.classList.add('input-error');
    let errEl = input.parentElement.querySelector('.field-error');
    if (!errEl) {
        errEl = document.createElement('div');
        errEl.className = 'field-error';
        input.parentElement.appendChild(errEl);
    }
    errEl.textContent = msg;
    errEl.classList.add('visible');
    markTabForField(input); // flag the tab this field lives on
}

// Returns the form-tab id ('basic'|'load'|...) that contains the given field.
function tabOfField(input) {
    const tabEl = input.closest('[id^="tab-"]');
    return tabEl ? tabEl.id.replace('tab-', '') : null;
}

// Add a red dot to the tab button whose panel contains an errored field.
function markTabForField(input) {
    const tab = tabOfField(input);
    if (!tab) return;
    const btn = document.querySelector(`[onclick="switchFormTab('${tab}')"]`);
    if (btn && !btn.querySelector('.tab-error-dot')) {
        const dot = document.createElement('span');
        dot.className = 'tab-error-dot';
        dot.style.cssText = 'display:inline-block;width:6px;height:6px;border-radius:50%;background:#ef4444;margin-left:5px;vertical-align:middle;';
        btn.appendChild(dot);
    }
}

function clearErrors(form) {
    form.querySelectorAll('.input-error').forEach(el => el.classList.remove('input-error'));
    form.querySelectorAll('.field-error').forEach(el => el.classList.remove('visible'));
    document.querySelectorAll('.tab-error-dot').forEach(el => el.remove());
}

// ============================================
// Router
// ============================================

function navigate(hash) { window.location.hash = hash; }

function getRoute() {
    const hash = window.location.hash || '#/';
    const p = hash.replace('#', '').split('/').filter(Boolean);
    if (p.length === 0) return { page: 'dashboard' };
    if (p[0] === 'services' && p[1] === 'new') return { page: 'new-service' };
    if (p[0] === 'services' && p[2] === 'run') return { page: 'running', id: p[1] };
    if (p[0] === 'services' && p[2] === 'capacity') return { page: 'capacity', id: p[1] };
    if (p[0] === 'services' && p[2] === 'edit') return { page: 'edit-service', id: p[1] };
    if (p[0] === 'services' && p[1]) return { page: 'service', id: p[1] };
    if (p[0] === 'settings') return { page: 'settings' };
    if (p[0] === 'queue') return { page: 'queue' };
    if (p[0] === 'schedules') return { page: 'schedules' };
    if (p[0] === 'compare') return { page: 'compare' };
    if (p[0] === 'plugins') return { page: 'plugins' };
    if (p[0] === 'workspaces') return { page: 'workspaces' };
    return { page: 'dashboard' };
}

async function router() {
    // Unsaved changes guard (uses the app's modal instead of native confirm).
    if (formDirty) {
        const leave = await confirmModal(
            'Unsaved changes',
            'You have unsaved changes on this form. Leave without saving?',
            { confirmLabel: 'Leave', cancelLabel: 'Stay', confirmClass: 'btn-danger' }
        );
        if (!leave) {
            const cr = state.currentRoute;
            const restoreHash = cr?.page === 'new-service' ? '/services/new' :
                cr?.page === 'edit-service' ? `/services/${cr.id}/edit` : '/';
            window.history.pushState(null, '', '#' + restoreHash);
            return;
        }
        formDirty = false;
    }

    const route = getRoute();
    state.currentRoute = route;

    // Reset pagination and bulk mode when navigating
    state.dashboardPage = 1;
    state.historyPage = 1;
    state.bulkMode = false;
    state.bulkSelected = [];

    if (route.page !== 'running' && state.eventSource) {
        state.eventSource.close();
        state.eventSource = null;
    }

    // Update nav active state
    const dashLink = document.getElementById('nav-dashboard');
    const settingsLink = document.getElementById('nav-settings');
    const queueLink = document.getElementById('nav-queue');
    const schedulesLink = document.getElementById('nav-schedules');
    const compareLink = document.getElementById('nav-compare');
    const pluginsLink = document.getElementById('nav-plugins');
    const workspacesLink = document.getElementById('nav-workspaces');
    if (dashLink) dashLink.classList.toggle('active', route.page === 'dashboard');
    if (settingsLink) settingsLink.classList.toggle('active', route.page === 'settings');
    if (queueLink) queueLink.classList.toggle('active', route.page === 'queue');
    if (schedulesLink) schedulesLink.classList.toggle('active', route.page === 'schedules');
    if (compareLink) compareLink.classList.toggle('active', route.page === 'compare');
    if (pluginsLink) pluginsLink.classList.toggle('active', route.page === 'plugins');
    if (workspacesLink) workspacesLink.classList.toggle('active', route.page === 'workspaces');

    await refreshServices();
    const app = document.getElementById('app');

    switch (route.page) {
        case 'dashboard': {
            app.innerHTML = wrap(renderDashboard());
            break;
        }
        case 'new-service':
            // Open on Advanced when arriving from the Plugins page with a protocol.
            state.formTab = state.pendingProtocol ? 'advanced' : 'basic';
            state.loadMode = 'simple';
            if (!state.settings || !state.settings.default_concurrency) {
                try { state.settings = (await api('/api/settings')) || {}; } catch(_) {}
            }
            app.innerHTML = wrap(renderForm(null)); setupFormTracking(); populateFormWorkspaces();
            state.pendingProtocol = null; // consume after the form has rendered
            break;
        case 'edit-service': {
            state.formTab = 'basic';
            const editSvc = state.services.find(s => s.id == route.id);
            // Auto-detect load mode from existing config
            if (editSvc && editSvc.arrival_rate > 0) state.loadMode = 'realistic';
            else if (editSvc && (editSvc.warmup_conns > 0 || editSvc.adaptive_concurrency || editSvc.requests_per_iteration > 1)) state.loadMode = 'expert';
            else state.loadMode = 'simple';
            app.innerHTML = wrap(renderForm(editSvc || null));
            switchLoadMode(state.loadMode);
            setupFormTracking();
            populateFormWorkspaces();
            break;
        }
        case 'service': {
            const svc = state.services.find(s => s.id == route.id);
            if (svc && svc.is_running) { navigate(`/services/${route.id}/run`); return; }
            app.innerHTML = wrap(detailSkeleton(svc)); // show immediately while data loads
            const html = await renderServiceDetail(route.id);
            if (state.currentRoute && state.currentRoute.page === 'service' && state.currentRoute.id == route.id) {
                app.innerHTML = wrap(html);
            }
            break;
        }
        case 'running':
            app.innerHTML = wrap(renderRunningView(route.id));
            startStream(route.id);
            break;
        case 'capacity':
            app.innerHTML = wrap(renderCapacityView(route.id));
            initCapacityView(route.id);
            break;
        case 'settings':
            app.innerHTML = wrap(await renderSettingsPage());
            break;
        case 'queue':
            app.innerHTML = wrap(await renderQueuePage());
            break;
        case 'schedules':
            app.innerHTML = wrap(await renderSchedulesPage());
            break;
        case 'compare':
            app.innerHTML = wrap(await renderComparePage());
            break;
        case 'plugins':
            app.innerHTML = wrap(await renderPluginsPage());
            break;
        case 'workspaces':
            app.innerHTML = wrap(await renderWorkspacesPage());
            break;
        default: {
            app.innerHTML = wrap(renderDashboard());
        }
    }

    // Update workspace selector
    await updateWorkspaceSelector();
}

function wrap(html) { return `<div class="view-enter">${html}</div>`; }

// ============================================
// API
// ============================================

async function api(path, opts = {}) {
    const res = await fetch(path, {
        headers: { 'Content-Type': 'application/json', ...opts.headers }, ...opts,
    });
    if (!res.ok) throw new Error((await res.text()) || res.statusText);
    if (res.status === 204) return null;
    const text = await res.text();
    if (!text) return null;
    try { return JSON.parse(text); }
    catch (_) { throw new Error('Invalid JSON response from server'); }
}

async function refreshServices() {
    try { state.services = (await api('/api/services')) || []; }
    catch (_) { state.services = []; }
    updateGlobalRunning();
}

// Renders the header pill showing how many tests are running right now, so the
// user stays aware of live activity from any page. Clicking jumps to the live
// view (single test) or the dashboard (multiple).
function updateGlobalRunning() {
    const el = document.getElementById('global-running-indicator');
    if (!el) return;
    const running = (state.services || []).filter(s => s.is_running);
    if (!running.length) { el.style.display = 'none'; el.innerHTML = ''; return; }
    const one = running.length === 1;
    const oneLink = running[0] && running[0].running_kind === 'capacity' ? `#/services/${running[0].id}/capacity` : `#/services/${running[0] ? running[0].id : ''}/run`;
    el.href = one ? oneLink : '#/';
    el.title = one ? `Live: ${running[0].name}` : `${running.length} tests running`;
    el.style.display = 'inline-flex';
    const label = one ? esc(running[0].name) : `${running.length} tests`;
    el.innerHTML = `<span class="pulse-dot" style="width:7px;height:7px;"></span><span>${label} running</span>`;
}

// ============================================
// Formatting
// ============================================

function fmt(n) { return n == null ? '0' : Number(n).toLocaleString('en-US', { maximumFractionDigits: 0 }); }
function fmtDec(n, d = 1) { return n == null ? '0' : Number(n).toLocaleString('en-US', { minimumFractionDigits: d, maximumFractionDigits: d }); }

function fmtLatency(ms) {
    if (ms == null || ms === 0) return '-';
    if (ms < 1) return `${(ms * 1000).toFixed(0)} \u00b5s`;
    if (ms >= 1000) return `${(ms / 1000).toFixed(2)} s`;
    return `${ms.toFixed(1)} ms`;
}

function fmtDuration(ms) {
    if (ms == null) return '0s';
    const s = Math.round(ms / 1000);
    if (s < 60) return `${s}s`;
    return `${Math.floor(s / 60)}m ${s % 60}s`;
}

function methodBadge(method, size) {
    const c = METHOD_COLORS[method] || 'var(--text-muted)';
    const cls = size === 'lg' ? 'px-2.5 py-0.5 text-xs' : 'px-1.5 py-0.5 text-[10px]';
    return `<span class="badge ${cls}" style="background:${c}18;color:${c};">${method}</span>`;
}

function statusCodeColor(code) {
    const c = parseInt(code, 10);
    if (c === 0) return '#ef4444';
    if (c < 300) return '#10b981';
    if (c < 400) return '#06b6d4';
    if (c < 500) return '#f59e0b';
    return '#ef4444';
}

function statusCodeLabel(code) {
    const c = parseInt(code, 10);
    if (c === 0) return 'Connection Failed';
    return String(c);
}

function esc(str) { if (str == null) return ''; const d = document.createElement('div'); d.textContent = String(str); return d.innerHTML; }

// ---- cURL Parser ----

function parseCurl(input) {
    // Normalize: remove line continuations, collapse whitespace
    let cmd = input.replace(/\\\n/g, ' ').replace(/\\\r\n/g, ' ').trim();

    // Remove leading 'curl' keyword
    if (cmd.startsWith('curl ')) cmd = cmd.slice(5).trim();

    const result = { url: '', method: 'GET', headers: {}, body: '', cookies: {} };

    // Tokenize respecting single and double quotes
    const tokens = [];
    let i = 0;
    while (i < cmd.length) {
        // Skip whitespace
        while (i < cmd.length && /\s/.test(cmd[i])) i++;
        if (i >= cmd.length) break;

        let token = '';
        if (cmd[i] === "'" || cmd[i] === '"') {
            const q = cmd[i]; i++;
            while (i < cmd.length && cmd[i] !== q) {
                if (cmd[i] === '\\' && i + 1 < cmd.length) { token += cmd[i + 1]; i += 2; }
                else { token += cmd[i]; i++; }
            }
            i++; // skip closing quote
        } else {
            while (i < cmd.length && !/\s/.test(cmd[i])) { token += cmd[i]; i++; }
        }
        tokens.push(token);
    }

    // Parse tokens
    let ti = 0;
    while (ti < tokens.length) {
        const t = tokens[ti];

        if (t === '-X' || t === '--request') {
            ti++; if (ti < tokens.length) result.method = tokens[ti].toUpperCase();
        } else if (t === '-H' || t === '--header') {
            ti++;
            if (ti < tokens.length) {
                const sep = tokens[ti].indexOf(':');
                if (sep > 0) {
                    const key = tokens[ti].slice(0, sep).trim();
                    const val = tokens[ti].slice(sep + 1).trim();
                    if (val) result.headers[key] = val; // skip empty-value headers like "authorization;"
                }
            }
        } else if (t === '-b' || t === '--cookie') {
            ti++;
            if (ti < tokens.length) {
                // Parse cookie string into individual cookies
                tokens[ti].split(';').forEach(c => {
                    const eq = c.indexOf('=');
                    if (eq > 0) result.cookies[c.slice(0, eq).trim()] = c.slice(eq + 1).trim();
                });
            }
        } else if (t === '-d' || t === '--data' || t === '--data-raw' || t === '--data-binary') {
            ti++;
            if (ti < tokens.length) {
                result.body = tokens[ti];
                if (result.method === 'GET') result.method = 'POST';
            }
        } else if (!t.startsWith('-')) {
            // Probably the URL
            if (!result.url && (t.startsWith('http://') || t.startsWith('https://'))) {
                result.url = t;
            }
        }
        ti++;
    }

    // Derive a name from the URL
    try {
        const u = new URL(result.url);
        const path = u.pathname.split('/').filter(Boolean).pop() || u.hostname;
        result.name = path.charAt(0).toUpperCase() + path.slice(1);
    } catch (_) {
        result.name = 'Imported Service';
    }

    // Set cookie header if -b cookies were found
    if (Object.keys(result.cookies).length > 0 && !result.headers['Cookie'] && !result.headers['cookie']) {
        result.headers['Cookie'] = Object.entries(result.cookies).map(([k, v]) => `${k}=${v}`).join('; ');
    }

    return result;
}

function handleCurlImport() {
    const input = document.getElementById('curl-input');
    if (!input || !input.value.trim()) {
        toast('Paste a cURL command first.', 'error');
        return;
    }

    const parsed = parseCurl(input.value);

    if (!parsed.url) {
        toast('Could not find a URL in the cURL command.', 'error');
        return;
    }

    const form = document.getElementById('service-form');
    if (!form) return;

    // Fill form fields
    const nameField = form.querySelector('[name="name"]');
    if (nameField && !nameField.value) nameField.value = parsed.name;

    const urlField = form.querySelector('[name="url"]');
    if (urlField) urlField.value = parsed.url;

    const methodField = form.querySelector('[name="method"]');
    if (methodField) {
        methodField.value = parsed.method;
        toggleBody(parsed.method);
    }

    const bodyField = form.querySelector('[name="body"]');
    if (bodyField && parsed.body) bodyField.value = parsed.body;

    // Fill headers
    const headerContainer = document.getElementById('headers-container');
    if (headerContainer) {
        headerContainer.innerHTML = '';
        const hdrs = Object.entries(parsed.headers);
        if (hdrs.length > 0) {
            hdrs.forEach(([k, v]) => {
                headerContainer.insertAdjacentHTML('beforeend', headerRow(k, v));
            });
        } else {
            headerContainer.innerHTML = '<p class="text-xs py-2" style="color:var(--text-faint);">No headers configured.</p>';
        }
    }

    // Enable cookie jar if cookies were found
    if (Object.keys(parsed.cookies).length > 0) {
        const cookieToggle = form.querySelector('[name="cookie_jar"]');
        if (cookieToggle) cookieToggle.checked = true;
    }

    const headerCount = Object.keys(parsed.headers).length;
    toast(`Imported: ${parsed.method} ${parsed.url.slice(0, 50)}... (${headerCount} headers)`, 'success');
}

function parseDurationToSeconds(val) {
    if (!val) return 10;
    if (typeof val === 'number') return val;
    val = String(val).trim();
    // Pure number
    if (/^\d+$/.test(val)) return parseInt(val, 10);
    // Go duration: 2m30s, 1m, 30s, 1h
    let secs = 0;
    const h = val.match(/(\d+)h/); if (h) secs += parseInt(h[1], 10) * 3600;
    const m = val.match(/(\d+)m/); if (m) secs += parseInt(m[1], 10) * 60;
    const s = val.match(/(\d+)s/); if (s) secs += parseInt(s[1], 10);
    return secs || 10;
}

function parseTags(tags) {
    if (Array.isArray(tags)) return tags.filter(Boolean);
    if (typeof tags === 'string' && tags.trim()) return tags.split(',').map(t => t.trim()).filter(Boolean);
    return [];
}

function parseJSON(val, fallback) {
    if (!val) return fallback;
    if (typeof val === 'object') return val;
    try { return JSON.parse(val); } catch (_) { return fallback; }
}

// ============================================
// Tabbed Form
// ============================================

// Plain-language, task-oriented names + one-line descriptions for each tab.
const FORM_TABS = {
    basic:      { label: 'Request',  desc: 'The endpoint and payload to test — URL, method, headers, and body.' },
    load:       { label: 'Load',     desc: 'How much load to generate, for how long, plus reusable presets.' },
    validation: { label: 'Checks',   desc: 'Rules that decide whether a test run passes or fails.' },
    advanced:   { label: 'Advanced', desc: 'Protocol, connection tuning, and dynamic data — all optional.' },
    scenarios:  { label: 'Steps',    desc: 'Chain several requests into one multi-step user flow.' },
};

function switchFormTab(tab) {
    state.formTab = tab;
    ['basic','load','validation','advanced','scenarios'].forEach(t => {
        const el = document.getElementById('tab-' + t);
        if (el) el.style.display = t === tab ? 'block' : 'none';
    });
    document.querySelectorAll('[onclick^="switchFormTab"]').forEach(btn => {
        const isActive = btn.getAttribute('onclick').includes("'" + tab + "'");
        btn.style.color = isActive ? 'var(--accent-text)' : 'var(--text-subtle)';
        btn.style.borderColor = isActive ? '#7c3aed' : 'transparent';
    });
    const desc = document.getElementById('tab-desc');
    if (desc && FORM_TABS[tab]) desc.textContent = FORM_TABS[tab].desc;
}

function switchLoadMode(mode) {
    state.loadMode = mode;
    const hints = {simple:'Basic concurrency test — just set workers and duration.',realistic:'Simulates real users with arrival rate and think time.',expert:'Full control over all parameters.'};

    // Update mode buttons
    ['simple','realistic','expert'].forEach(m => {
        const btn = document.getElementById('load-mode-' + m);
        if (btn) {
            btn.style.background = m === mode ? 'rgba(124,58,237,0.15)' : 'transparent';
            btn.style.color = m === mode ? 'var(--accent-text)' : 'var(--text-subtle)';
            btn.style.borderColor = m === mode ? 'rgba(124,58,237,0.3)' : 'rgba(71,85,105,0.3)';
        }
    });
    const hint = document.getElementById('load-mode-hint');
    if (hint) hint.textContent = hints[mode] || '';

    // Show/hide sections
    const concWrap = document.getElementById('concurrency-wrap');
    const realistic = document.getElementById('load-realistic');
    const expert = document.getElementById('load-expert');

    if (concWrap) concWrap.style.display = mode === 'realistic' ? 'none' : 'block';
    if (realistic) realistic.style.display = mode === 'realistic' ? 'block' : 'none';
    if (expert) expert.style.display = mode === 'expert' ? 'block' : 'none';

    // In realistic mode, show think-max-wrap since think time defaults to 500ms
    if (mode === 'realistic') {
        const tmw = document.getElementById('think-max-wrap');
        if (tmw) tmw.style.display = 'block';
    }

    // In realistic mode, disable concurrency
    const concInput = document.querySelector('[name="concurrency"]');
    if (concInput) {
        concInput.disabled = mode === 'realistic';
        concInput.style.opacity = mode === 'realistic' ? '0.4' : '1';
    }

    // Sync arrival_rate values between realistic and expert inputs
    const realisticAR = document.querySelector('#load-realistic [name="arrival_rate"]');
    const expertAR = document.getElementById('expert-arrival-rate');
    if (realisticAR && expertAR) {
        if (mode === 'expert') expertAR.value = realisticAR.value;
        if (mode === 'realistic') realisticAR.value = expertAR.value;
    }

    // Sync think time values between modes
    const realisticTT = document.querySelector('#load-realistic [name="think_time_ms"]');
    const expertTT = document.getElementById('expert-think');
    if (realisticTT && expertTT) {
        if (mode === 'expert') expertTT.value = realisticTT.value;
        if (mode === 'realistic') realisticTT.value = expertTT.value;
    }
}

function toggleThinkMax() {
    const thinkInputs = document.querySelectorAll('[name="think_time_ms"]');
    let val = 0;
    thinkInputs.forEach(inp => { if (inp.offsetParent !== null) val = parseInt(inp.value, 10) || 0; });
    ['think-max-wrap', 'expert-think-max-wrap'].forEach(id => {
        const el = document.getElementById(id);
        if (el) el.style.display = val > 0 ? 'block' : 'none';
    });
}

function toggleAdaptiveTarget() {
    const cb = document.querySelector('[name="adaptive_concurrency"]');
    const wrap = document.getElementById('adaptive-target-wrap');
    if (wrap) wrap.style.display = cb && cb.checked ? 'block' : 'none';
}

// ============================================
// Test Templates
// ============================================

async function showTemplates() {
    const panel = document.getElementById('templates-panel');
    if (!panel) return;

    if (panel.style.display === 'block') {
        panel.style.display = 'none'; return;
    }

    let templates = [];
    try { templates = await api('/api/templates'); } catch(_) {}

    if (!templates || !templates.length) {
        panel.innerHTML = '<div class="card p-4 text-sm" style="color:var(--text-subtle);">No templates available.</div>';
        panel.style.display = 'block';
        return;
    }

    const groups = {};
    templates.forEach(t => {
        if (!groups[t.category]) groups[t.category] = [];
        groups[t.category].push(t);
    });

    panel.innerHTML = `
    <div class="card p-5 mb-4" style="border-color:rgba(6,182,212,0.3);">
        <div class="flex items-center justify-between mb-4">
            <h3 class="text-sm font-semibold" style="color:#06b6d4;">Test Templates</h3>
            <button onclick="document.getElementById('templates-panel').style.display='none'" class="btn btn-icon btn-ghost btn-sm" style="padding:0.3rem;">${svgIcon('x')}</button>
        </div>
        <div class="grid grid-cols-2 gap-3">
            ${templates.map(t => `
            <div class="p-3 rounded-lg cursor-pointer hover:bg-slate-700/30 transition-colors" style="border:1px solid rgba(71,85,105,0.2);" onclick="applyTemplate('${esc(t.id)}')">
                <div class="flex items-center gap-2 mb-1">
                    <span class="text-xs font-bold px-1.5 py-0.5 rounded" style="background:rgba(6,182,212,0.15);color:#06b6d4;">${esc(t.category)}</span>
                    <span class="text-sm font-semibold" style="color:var(--text);">${esc(t.name)}</span>
                </div>
                <p class="text-[11px]" style="color:var(--text-subtle);">${esc(t.description)}</p>
            </div>`).join('')}
        </div>
    </div>`;
    panel.style.display = 'block';
}

async function applyTemplate(templateId) {
    let templates = [];
    try { templates = await api('/api/templates'); } catch(_) { return; }

    const t = templates.find(t => t.id === templateId);
    if (!t) return;

    const form = document.getElementById('service-form');
    if (!form) return;

    const s = t.service;

    const set = (name, val) => { const el = form.querySelector('[name="' + name + '"]'); if (el) el.value = val || ''; };
    set('name', t.name);
    set('url', s.url);
    set('method', s.method);
    set('body', s.body);
    set('concurrency', s.concurrency);
    set('duration', parseDurationToSeconds(s.duration));
    set('timeout', parseDurationToSeconds(s.timeout));
    set('think_time_ms', s.think_time_ms);
    set('arrival_rate', s.arrival_rate);

    if (s.headers) {
        const container = document.getElementById('headers-container');
        if (container) {
            container.innerHTML = '';
            Object.entries(s.headers).forEach(([k, v]) => {
                container.insertAdjacentHTML('beforeend', headerRow(k, v));
            });
        }
    }

    document.getElementById('templates-panel').style.display = 'none';
    markFormDirty();
    toast('Template "' + t.name + '" applied. Update the URLs to match your API.', 'success');
}

// ============================================
// Bulk Operations
// ============================================

function toggleBulkMode() {
    state.bulkMode = !state.bulkMode;
    state.bulkSelected = [];
    renderDashboardContent();
}

function toggleBulkSelect(id, checked) {
    if (checked) {
        if (!state.bulkSelected.includes(String(id))) state.bulkSelected.push(String(id));
    } else {
        state.bulkSelected = state.bulkSelected.filter(s => s !== String(id));
    }
    renderDashboardContent();
}

async function bulkDelete() {
    const count = state.bulkSelected.length;
    const confirmed = await confirmModal('Delete Services', 'Delete ' + count + ' selected service(s)? This cannot be undone.');
    if (!confirmed) return;

    try {
        await api('/api/services/bulk-delete', {
            method: 'POST',
            body: JSON.stringify({ ids: state.bulkSelected.map(Number) })
        });
        toast(count + ' service(s) deleted.', 'success');
        state.bulkMode = false;
        state.bulkSelected = [];
        router();
    } catch (err) { toast('Failed: ' + err.message, 'error'); }
}

async function bulkQueue() {
    try {
        await api('/api/queue/bulk-add', {
            method: 'POST',
            body: JSON.stringify({ service_ids: state.bulkSelected.map(Number) })
        });
        toast(state.bulkSelected.length + ' service(s) added to queue.', 'success');
        state.bulkMode = false;
        state.bulkSelected = [];
        router();
        startLivePolling();
    } catch (err) { toast('Failed: ' + err.message, 'error'); }
}

function bulkExport() {
    const selected = state.bulkSelected.map(Number);
    const services = state.services.filter(s => selected.includes(s.id));
    const blob = new Blob([JSON.stringify(services, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url; a.download = 'gload-services-export.json'; a.click();
    URL.revokeObjectURL(url);
    toast(selected.length + ' service(s) exported.', 'success');
}

// ============================================
// Real-Time Collaboration (WebSocket Broadcast)
// ============================================

let broadcastWs = null;

function connectBroadcast() {
    if (broadcastWs) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = protocol + '//' + window.location.host + '/api/ws/events';

    broadcastWs = new WebSocket(url);

    broadcastWs.onmessage = function(e) {
        try {
            const msg = JSON.parse(e.data);
            handleBroadcastEvent(msg.event, msg.data);
        } catch(_) {}
    };

    broadcastWs.onclose = function() {
        broadcastWs = null;
        setTimeout(connectBroadcast, 5000);
    };

    broadcastWs.onerror = function() {
        broadcastWs.close();
    };
}

// Clean up WebSocket and SSE on page unload to prevent resource leaks.
window.addEventListener('beforeunload', () => {
    if (broadcastWs) { broadcastWs.close(); broadcastWs = null; }
    if (state.eventSource) { state.eventSource.close(); state.eventSource = null; }
});

function handleBroadcastEvent(event, data) {
    switch(event) {
        case 'service_created':
        case 'service_updated':
        case 'service_deleted':
            if (state.currentRoute && state.currentRoute.page === 'dashboard') {
                refreshServices().then(function() { renderDashboardContent(); });
            }
            break;
        case 'test_started':
            toast('Test started: ' + (data.service_name || 'Unknown'), 'info');
            // Refresh services on any page so the global running indicator stays
            // current; only re-render the page-specific content where relevant.
            refreshServices().then(function() {
                if (state.currentRoute && state.currentRoute.page === 'dashboard') renderDashboardContent();
            });
            if (state.currentRoute && state.currentRoute.page === 'queue') {
                refreshQueue().then(function() { renderQueueContent(); });
            }
            break;
        case 'test_completed':
            var status = data.status === 'fail' ? 'FAILED' : 'PASSED';
            var action = data.service_id ? { label: 'View results →', href: '#/services/' + data.service_id } : null;
            toast('Test completed: ' + (data.service_name || 'Unknown') + ' — ' + status, data.status === 'fail' ? 'error' : 'success', action);
            refreshServices().then(function() {
                if (state.currentRoute && state.currentRoute.page === 'dashboard') renderDashboardContent();
            });
            if (state.currentRoute && state.currentRoute.page === 'queue') {
                refreshQueue().then(function() { renderQueueContent(); });
            }
            break;
    }
}

function svgIcon(name) {
    const icons = {
        chevron: '<svg class="w-4 h-4 sep" style="color:var(--text-faint)" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/></svg>',
        edit: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"/></svg>',
        play: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>',
        stop: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/><rect x="9" y="9" width="6" height="6" rx="1" stroke="currentColor" stroke-width="2" fill="none"/></svg>',
        report: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 17v-2m3 2v-4m3 4v-6m2 10H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>',
        plus: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/></svg>',
        minus: '<svg class="w-4 h-4" style="color:#ef4444" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12H4"/></svg>',
        x: '<svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg>',
        bolt: '<svg class="w-10 h-10" style="color:#7c3aed" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>',
        refresh: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/></svg>',
        trash: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>',
        clone: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"/></svg>',
        download: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/></svg>',
        upload: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12"/></svg>',
        settings: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/></svg>',
        queue: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16"/></svg>',
        pin: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z"/></svg>',
        pinFilled: '<svg class="w-4 h-4" fill="currentColor" viewBox="0 0 24 24"><path d="M5 5a2 2 0 012-2h10a2 2 0 012 2v16l-7-3.5L5 21V5z"/></svg>',
        compare: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"/></svg>',
        collapseDown: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/></svg>',
        collapseRight: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/></svg>',
        schedule: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>',
        share: '<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8.684 13.342C8.886 12.938 9 12.482 9 12c0-.482-.114-.938-.316-1.342m0 2.684a3 3 0 110-2.684m0 2.684l6.632 3.316m-6.632-6l6.632-3.316m0 0a3 3 0 105.367-2.684 3 3 0 00-5.367 2.684zm0 9.316a3 3 0 105.368 2.684 3 3 0 00-5.368-2.684z"/></svg>',
    };
    return icons[name] || '';
}

function breadcrumb(...parts) {
    return `<nav class="breadcrumb">${parts.map((p, i) => {
        if (i === parts.length - 1) return `<span class="current">${esc(p.label)}</span>`;
        return `<a href="${p.href}">${esc(p.label)}</a>${svgIcon('chevron')}`;
    }).join('')}</nav>`;
}

// ============================================
// Cron Helpers
// ============================================

function cronToHuman(expr) {
    if (!expr) return '';
    const f = expr.trim().split(/\s+/);
    if (f.length !== 5) return expr;
    const [min, hour, dom, mon, dow] = f;

    const DAYS = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
    const time12 = (h, m) => {
        h = parseInt(h, 10); m = parseInt(m, 10);
        if (isNaN(h) || isNaN(m)) return null;
        const ap = h < 12 ? 'AM' : 'PM';
        const hr = h % 12 === 0 ? 12 : h % 12;
        return `${hr}:${String(m).padStart(2, '0')} ${ap}`;
    };
    const dowName = (d) => {
        if (d === '1-5') return 'weekdays';
        if (d === '0,6' || d === '6,0') return 'weekends';
        return d.split(',').map(x => {
            if (x.includes('-')) { const [a, b] = x.split('-'); return `${DAYS[+a % 7] || a}–${DAYS[+b % 7] || b}`; }
            const n = parseInt(x, 10); return DAYS[n % 7] || x;
        }).join(', ');
    };

    // Every N minutes
    let m = min.match(/^\*\/(\d+)$/);
    if (m && hour === '*' && dom === '*' && mon === '*' && dow === '*') return `Every ${m[1]} minutes`;
    // Every N hours
    m = hour.match(/^\*\/(\d+)$/);
    if (min === '0' && m && dom === '*' && mon === '*' && dow === '*') return `Every ${m[1]} hours`;
    if (min === '*' && hour === '*' && dom === '*' && mon === '*' && dow === '*') return 'Every minute';

    const t = time12(hour, min);
    if (t && dom === '*' && mon === '*') {
        if (dow === '*') return `Daily at ${t}`;
        return `${dowName(dow)} at ${t}`;
    }
    if (t && dow === '*' && mon === '*' && /^\d/.test(dom)) return `Monthly on day ${dom} at ${t}`;
    if (min !== '*' && hour === '*' && dom === '*' && mon === '*' && dow === '*') return `Hourly at :${String(min).padStart(2, '0')}`;
    return expr;
}

// ============================================
// VIEWS
// ============================================

// ---- Dashboard ----

function renderDashboardContent() {
    const app = document.getElementById('app');
    const searchInput = app.querySelector('input[placeholder="Search services..."]');
    const hadFocus = searchInput && document.activeElement === searchInput;
    const cursorPos = hadFocus ? searchInput.selectionStart : 0;

    // No view-enter wrapper here: this runs on every keystroke/toggle, and
    // replaying the slide-in animation each time causes a visible flash.
    app.innerHTML = renderDashboard();

    if (hadFocus) {
        const newInput = app.querySelector('input[placeholder="Search services..."]');
        if (newInput) { newInput.focus(); newInput.setSelectionRange(cursorPos, cursorPos); }
    }
}

function renderDashboard() {
    const svcs = getFilteredServices();
    const total = svcs.length;
    const tested = svcs.filter(s => s.last_result).length;
    const running = svcs.filter(s => s.is_running).length;
    if (running > 0) startLivePolling();

    // Collect all tags and groups for filtering
    const allTags = [...new Set(svcs.flatMap(s => parseTags(s.tags)))].sort();
    const allGroups = [...new Set(svcs.map(s => s.group_name).filter(Boolean))].sort();
    const hasGroups = allGroups.length > 0;
    const groupKeys = allGroups.concat(svcs.some(s => !s.group_name) ? ['__ungrouped'] : []);
    const allGroupsCollapsed = groupKeys.length > 0 && groupKeys.every(g => state.collapsedGroups[g]);

    // Filter services
    let filtered = svcs;
    if (state.searchQuery) {
        const q = state.searchQuery.toLowerCase();
        filtered = filtered.filter(s =>
            (s.name || '').toLowerCase().includes(q) ||
            (s.url || '').toLowerCase().includes(q) ||
            parseTags(s.tags).some(t => t.toLowerCase().includes(q)) ||
            (s.group_name || '').toLowerCase().includes(q) ||
            (s.method || '').toLowerCase().includes(q) ||     // search by HTTP method
            serviceStatusLabel(s).toLowerCase().includes(q)   // search by status word
        );
    }
    if (state.filterTag) filtered = filtered.filter(s => parseTags(s.tags).includes(state.filterTag));
    if (state.filterGroup) filtered = filtered.filter(s => (s.group_name || '') === state.filterGroup || (state.filterGroup === '__ungrouped' && !s.group_name));
    if (state.filterStatus === 'tested') filtered = filtered.filter(s => s.last_result);
    else if (state.filterStatus === 'running') filtered = filtered.filter(s => s.is_running);
    else if (state.filterStatus === 'issues') filtered = filtered.filter(serviceHasIssues);

    // Sort: pinned services always float to the top, then by the chosen mode.
    const order = state.sortBy === 'manual' ? getServiceOrder() : [];
    const rate = s => (s.last_result && s.last_result.total_reqs > 0) ? s.last_result.errors / s.last_result.total_reqs * 100 : 0;
    filtered.sort((a, b) => {
        const pa = state.pinned.includes(String(a.id)), pb = state.pinned.includes(String(b.id));
        if (pa !== pb) return pa ? -1 : 1;
        switch (state.sortBy) {
            case 'name':   return (a.name || '').localeCompare(b.name || '');
            case 'rps':    return ((b.last_result && b.last_result.rps) || 0) - ((a.last_result && a.last_result.rps) || 0);
            case 'errors': return rate(b) - rate(a);
            case 'recent': return new Date((b.last_result && b.last_result.created_at) || 0) - new Date((a.last_result && a.last_result.created_at) || 0);
            default: {
                const ai = order.indexOf(String(a.id)), bi = order.indexOf(String(b.id));
                if (ai === -1 && bi === -1) return 0;
                if (ai === -1) return 1;
                if (bi === -1) return -1;
                return ai - bi;
            }
        }
    });

    // Pagination
    const totalPages = Math.ceil(filtered.length / SERVICES_PER_PAGE);
    if (state.dashboardPage > totalPages && totalPages > 0) state.dashboardPage = totalPages;
    const paged = filtered.slice((state.dashboardPage - 1) * SERVICES_PER_PAGE, state.dashboardPage * SERVICES_PER_PAGE);

    const hasFilters = allTags.length > 0 || allGroups.length > 0 || total > 0;

    // Build filter bar
    const filterBar = hasFilters ? `
    <div class="card p-3 mb-6 flex items-center gap-3 flex-wrap">
        <div class="relative flex-1 min-w-[200px] max-w-sm">
            <svg class="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" style="color:var(--text-subtle);" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>
            <input type="text" class="input-dark input-sm" style="padding-left:2.25rem;" placeholder="Search services..." aria-label="Search services" value="${esc(state.searchQuery)}" oninput="state.searchQuery=this.value;state.dashboardPage=1;renderDashboardContent();">
        </div>
        ${allGroups.length > 0 ? `<select class="input-dark input-sm" style="width:auto;min-width:140px;" aria-label="Filter by group" onchange="state.filterGroup=this.value;state.dashboardPage=1;renderDashboardContent();">
            <option value="">All Groups</option>
            ${allGroups.map(g => `<option value="${esc(g)}" ${state.filterGroup===g?'selected':''}>${esc(g)}</option>`).join('')}
            <option value="__ungrouped" ${state.filterGroup==='__ungrouped'?'selected':''}>Ungrouped</option>
        </select>` : ''}
        ${allTags.length > 0 ? `<select class="input-dark input-sm" style="width:auto;min-width:140px;" aria-label="Filter by tag" onchange="state.filterTag=this.value;state.dashboardPage=1;renderDashboardContent();">
            <option value="">All Tags</option>
            ${allTags.map(t => `<option value="${esc(t)}" ${state.filterTag===t?'selected':''}>${esc(t)}</option>`).join('')}
        </select>` : ''}
        <select class="input-dark input-sm" style="width:auto;min-width:130px;" aria-label="Sort services" onchange="setSort(this.value)">
            <option value="manual" ${state.sortBy==='manual'?'selected':''}>Sort: Manual</option>
            <option value="name" ${state.sortBy==='name'?'selected':''}>Sort: Name</option>
            <option value="rps" ${state.sortBy==='rps'?'selected':''}>Sort: RPS</option>
            <option value="errors" ${state.sortBy==='errors'?'selected':''}>Sort: Error rate</option>
            <option value="recent" ${state.sortBy==='recent'?'selected':''}>Sort: Recent</option>
        </select>
        ${(state.filterTag || state.filterGroup || state.searchQuery || state.filterStatus) ? `<button onclick="state.filterTag='';state.filterGroup='';state.searchQuery='';state.filterStatus='';state.dashboardPage=1;renderDashboardContent();" class="btn btn-ghost btn-sm" style="padding:0.3rem 0.6rem;font-size:0.75rem;">Clear</button>` : ''}
        <span class="text-xs" style="color:var(--text-subtle);">${filtered.length} of ${total}</span>
        ${hasGroups && !state.filterGroup ? `<button onclick="setAllGroupsCollapsed(${!allGroupsCollapsed})" class="btn btn-ghost btn-sm ml-auto" style="padding:0.3rem 0.6rem;font-size:0.75rem;" aria-label="${allGroupsCollapsed ? 'Expand all groups' : 'Collapse all groups'}">${allGroupsCollapsed ? 'Expand all' : 'Collapse all'}</button>` : ''}
        <div class="flex items-center gap-0.5 ${hasGroups && !state.filterGroup ? '' : 'ml-auto'} p-0.5 rounded-lg" style="background:var(--surface-2);border:1px solid var(--border);" role="group" aria-label="View mode">
            <button onclick="setViewMode('grid')" title="Grid view" aria-label="Grid view" aria-pressed="${state.viewMode === 'grid'}"
                    class="p-1.5 rounded-md transition-colors" style="${state.viewMode === 'grid' ? 'background:var(--accent-weak);color:var(--accent-text);' : 'color:var(--text-subtle);'}">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 5a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1H5a1 1 0 01-1-1V5zM14 5a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1V5zM4 15a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1H5a1 1 0 01-1-1v-4zM14 15a1 1 0 011-1h4a1 1 0 011 1v4a1 1 0 01-1 1h-4a1 1 0 01-1-1v-4z"/></svg>
            </button>
            <button onclick="setViewMode('list')" title="List view" aria-label="List view" aria-pressed="${state.viewMode === 'list'}"
                    class="p-1.5 rounded-md transition-colors" style="${state.viewMode === 'list' ? 'background:var(--accent-weak);color:var(--accent-text);' : 'color:var(--text-subtle);'}">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"/></svg>
            </button>
        </div>
    </div>` : '';

    // Queue panel
    let queuePanel = '';
    if (state.queue && (state.queue.running || (state.queue.items && state.queue.items.length > 0))) {
        const q = state.queue;
        queuePanel = `
        <div class="card p-4 mb-6 border" style="border-color:rgba(124,58,237,0.3);">
            <div class="flex items-center justify-between mb-2">
                <div class="flex items-center gap-2">
                    ${svgIcon('bolt')}
                    <h3 class="text-sm font-semibold" style="color:var(--text);">Test Queue</h3>
                </div>
                <div class="flex items-center gap-2">
                    <a href="#/queue" class="btn btn-ghost btn-sm" style="padding:0.3rem 0.6rem;font-size:0.75rem;">View All</a>
                    <button onclick="handleClearQueue()" class="btn btn-ghost btn-sm" style="padding:0.3rem 0.6rem;font-size:0.75rem;">Clear</button>
                </div>
            </div>
            ${q.current ? `<div class="text-xs mb-1" style="color:#10b981;">Running: <a href="#/services/${q.current.service_id}/run" style="color:#10b981;text-decoration:underline;">${esc(q.current.name)}</a></div>` : ''}
            ${q.items && q.items.length > 0 ? `<div class="text-xs" style="color:var(--text-muted);">Pending: ${q.items.length} test(s)</div>` : ''}
        </div>`;
    }

    // Group services if groups exist and no specific group filter
    let serviceGrid = '';
    const paginationHtml = renderPagination(state.dashboardPage, totalPages, 'setDashboardPage');
    if (hasGroups && !state.filterGroup) {
        // Group only the current page's slice so pagination still limits how
        // many cards render, while group headers show the full per-group total.
        // Pinned services collect into a "Pinned" section that renders first,
        // so a pin lifts a service above all groups (not just within its own).
        const groupKeyFor = s => state.pinned.includes(String(s.id)) ? '__pinned' : (s.group_name || '__ungrouped');
        const groupedPage = {};
        paged.forEach(s => {
            const g = groupKeyFor(s);
            (groupedPage[g] = groupedPage[g] || []).push(s);
        });
        const groupTotals = {};
        filtered.forEach(s => {
            const g = groupKeyFor(s);
            groupTotals[g] = (groupTotals[g] || 0) + 1;
        });
        const groupNames = Object.keys(groupedPage).sort((a, b) => {
            if (a === '__pinned') return -1;
            if (b === '__pinned') return 1;
            if (a === '__ungrouped') return 1;
            if (b === '__ungrouped') return -1;
            return a.localeCompare(b);
        });
        serviceGrid = groupNames.map(g => {
            const label = g === '__pinned' ? '📌 Pinned' : g === '__ungrouped' ? 'Ungrouped' : g;
            const collapsed = state.collapsedGroups[g] || false;
            return `
            <div class="mb-6">
                <button onclick="state.collapsedGroups['${esc(g)}']=!state.collapsedGroups['${esc(g)}'];router();"
                        aria-label="${collapsed ? 'Expand' : 'Collapse'} ${esc(label)} group" aria-expanded="${!collapsed}"
                        class="flex items-center gap-2 mb-3 cursor-pointer" style="background:none;border:none;padding:0;">
                    ${collapsed ? svgIcon('collapseRight') : svgIcon('collapseDown')}
                    <h3 class="text-base font-semibold" style="color:var(--accent-text);">${esc(label)}</h3>
                    <span class="text-xs" style="color:var(--text-subtle);">(${groupTotals[g]})</span>
                </button>
                ${collapsed ? '' : serviceListWrap(groupedPage[g].map((svc, idx) => renderServiceCard(svc, idx)).join(''))}
            </div>`;
        }).join('') + paginationHtml;
    } else {
        serviceGrid = filtered.length === 0 ? emptyState('No services found', 'No services match your current filter.', null, null) :
            `${serviceListWrap(paged.map((svc, idx) => renderServiceCard(svc, idx)).join(''))}${paginationHtml}`;
    }

    // Load queue state in background
    refreshQueue();

    // Widget renderers
    const widgetRenderers = {
        stats: () => {
            const issues = svcs.filter(serviceHasIssues).length;
            return `<div class="grid grid-cols-2 md:grid-cols-4 gap-4">${summaryCard('Total Services', total, 'var(--text)', svgStatIcon('server'), '')}${summaryCard('Tested', tested, '#10b981', svgStatIcon('check'), 'tested')}${summaryCard('Issues', issues, '#ef4444', svgStatIcon('warning'), 'issues')}${summaryCard('Running', running, '#f59e0b', svgStatIcon('activity'), 'running')}</div>`;
        },
        health: () => renderHealthOverview(svcs),
        queue: () => queuePanel,
        recent: () => renderRecentWidget(svcs),
        services: () => `${filterBar}${total === 0 ? renderOnboardingGuide() : serviceGrid}`,
    };

    // Effective order: user order minus hidden widgets, with 'services'
    // always present (the main content can't be hidden).
    let effectiveOrder = state.widgetOrder.filter(w => !state.hiddenWidgets.includes(w));
    if (!effectiveOrder.includes('services')) effectiveOrder.push('services');
    const widgets = effectiveOrder.map(wid => {
        const content = widgetRenderers[wid] ? widgetRenderers[wid]() : '';
        if (!content) return '';
        return `<div class="mb-6">${content}</div>`;
    }).join('');

    return `
    <div class="flex items-end justify-between mb-8">
        <div>
            <h2 class="text-2xl font-bold" style="color:var(--text);">Dashboard</h2>
            <p class="text-sm mt-1" style="color:var(--text-muted);">Overview of all services and test results.</p>
        </div>
        <div class="flex items-center gap-2">
            <button onclick="toggleBulkMode()" class="btn btn-ghost btn-sm">${state.bulkMode ? 'Cancel' : 'Select'}</button>
            ${state.bulkMode && state.bulkSelected.length > 0 ? `
                <span class="text-xs" style="color:var(--text-muted);">${state.bulkSelected.length} selected</span>
                <button onclick="bulkQueue()" class="btn btn-success btn-sm" title="Queue and run selected services sequentially">${svgIcon('play')}<span>Run selected</span></button>
                <button onclick="bulkDelete()" class="btn btn-danger btn-sm">Delete</button>
                <button onclick="bulkExport()" class="btn btn-ghost btn-sm">Export</button>
            ` : ''}
            <button onclick="showCustomizeDashboard()" class="btn btn-ghost btn-sm" aria-label="Customize dashboard layout">${svgIcon('settings')}<span class="hide-mobile">Customize</span></button>
            <button onclick="handleExport()" class="btn btn-ghost btn-sm" aria-label="Export all services">${svgIcon('download')}<span>Export</span></button>
            <button onclick="document.getElementById('import-file').click()" class="btn btn-ghost btn-sm" aria-label="Import services from file">${svgIcon('upload')}<span>Import</span></button>
            <input type="file" id="import-file" accept=".json" style="display:none;" onchange="handleImport(event)">
        </div>
    </div>
    ${widgets}`;
}

// ---- Service Card Drag & Drop ----

let draggedServiceId = null;
let isDraggingService = false;

function handleServiceDragStart(e, id) {
    draggedServiceId = id;
    isDraggingService = true;
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', id);
    const card = e.target.closest('.card');
    if (card) card.style.opacity = '0.4';
}

function handleServiceDragOver(e) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    const card = e.target.closest('[data-service-id]');
    document.querySelectorAll('.drag-over-indicator').forEach(el => el.classList.remove('drag-over-indicator'));
    if (card && card.dataset.serviceId != draggedServiceId) {
        card.classList.add('drag-over-indicator');
    }
}

async function handleServiceDrop(e, targetId) {
    e.preventDefault();
    e.stopPropagation();
    if (!draggedServiceId || draggedServiceId == targetId) return;

    const dragged = state.services.find(s => s.id == draggedServiceId);
    const target = state.services.find(s => s.id == targetId);
    const draggedIdCopy = draggedServiceId;
    isDraggingService = false;
    draggedServiceId = null;

    // If dropped onto a card in a different group, move it to that group.
    const movedGroup = dragged && target && (dragged.group_name || '') !== (target.group_name || '');
    if (movedGroup) {
        const newGroup = target.group_name || '';
        try {
            const payload = { ...dragged, group_name: newGroup };
            delete payload.is_running; delete payload.last_result;
            await api(`/api/services/${dragged.id}`, { method: 'PUT', body: JSON.stringify(payload) });
            dragged.group_name = newGroup; // reflect locally
            toast(`Moved "${dragged.name}" to ${newGroup || 'Ungrouped'}.`, 'success');
        } catch (err) {
            toast('Move failed: ' + err.message, 'error');
        }
    }

    // Reorder against the FULL ordered id list so positions are deterministic
    // (a partial order containing only moved ids sorts unstably).
    let order = fullOrderedServiceIds();
    order = order.filter(x => x !== String(draggedIdCopy));
    const ti = order.indexOf(String(targetId));
    if (ti === -1) order.push(String(draggedIdCopy));
    else order.splice(ti, 0, String(draggedIdCopy));
    localStorage.setItem('gload_service_order', JSON.stringify(order));

    if (movedGroup) { await refreshServices(); }
    renderDashboardContent();
}

// Full list of every current service id in the current display order:
// saved order first (filtered to existing ids), then any new ids in natural order.
function fullOrderedServiceIds() {
    const saved = getServiceOrder();
    const all = state.services.map(s => String(s.id));
    const known = saved.filter(id => all.includes(id));
    const unknown = all.filter(id => !known.includes(id));
    return [...known, ...unknown];
}

function handleServiceDragEnd(e) {
    isDraggingService = false;
    draggedServiceId = null;
    const card = e.target.closest('.card');
    if (card) card.style.opacity = '1';
    document.querySelectorAll('.drag-over-indicator').forEach(el => el.classList.remove('drag-over-indicator'));
}

function handleServiceCardClick(e, id) {
    if (isDraggingService) { e.preventDefault(); return; }
    // In bulk mode, clicking anywhere on the card toggles its selection
    // instead of navigating.
    if (state.bulkMode && id !== undefined) {
        e.preventDefault();
        const selected = state.bulkSelected.includes(String(id));
        toggleBulkSelect(id, !selected);
    }
}

function getServiceOrder() {
    try { return JSON.parse(localStorage.getItem('gload_service_order') || '[]').map(String); }
    catch(_) { return []; }
}

// ---- Recent Tests Widget ----

function renderRecentWidget(svcs) {
    const withResults = svcs.filter(s => s.last_result && s.last_result.created_at);
    if (withResults.length === 0) return '';

    const sorted = withResults.slice().sort((a, b) =>
        new Date(b.last_result.created_at).getTime() - new Date(a.last_result.created_at).getTime()
    );
    const recent = sorted.slice(0, 5);

    const rows = recent.map(s => {
        const r = s.last_result;
        const errRate = r.total_reqs > 0 ? (r.errors / r.total_reqs * 100) : 0;
        const statusColor = errRate > 5 ? '#ef4444' : '#10b981';
        const statusLabel = errRate > 5 ? 'Issues' : 'Healthy';
        return `
        <a href="#/services/${s.id}" class="flex items-center gap-3 py-2.5 px-1 rounded-lg hover:bg-slate-700/20 transition-colors" style="text-decoration:none;color:inherit;">
            <div class="flex-1 min-w-0">
                <div class="flex items-center gap-2">
                    ${methodBadge(s.method)}
                    <span class="text-sm font-medium truncate" style="color:var(--text);">${esc(s.name)}</span>
                </div>
                <div class="text-[10px] mt-0.5" style="color:var(--text-subtle);">${fmtRelativeTime(r.created_at)}</div>
            </div>
            <div class="text-xs font-mono" style="color:#06b6d4;">${fmtDec(r.rps)} /s</div>
            <span class="status-badge" style="background:${statusColor}15;color:${statusColor};">
                <span class="w-1.5 h-1.5 rounded-full" style="background:${statusColor};"></span>
                ${statusLabel}
            </span>
        </a>`;
    }).join('');

    return `
    <div class="card p-5">
        <h3 class="text-sm font-semibold mb-3" style="color:var(--text);">Recent Tests</h3>
        <div class="divide-y divide-slate-700/30">${rows}</div>
    </div>`;
}

function summaryCard(label, value, color, icon, status) {
    // When a status is given, the card becomes a toggle that filters the list.
    // The empty status ("Total") is clickable to clear but never shows active.
    const clickable = status !== undefined;
    const active = clickable && status !== '' && state.filterStatus === status;
    const attrs = clickable
        ? `role="button" tabindex="0" aria-pressed="${active}" aria-label="Filter by ${label}"
           style="cursor:pointer;${active ? 'border-color:var(--accent);box-shadow:0 0 0 1px var(--accent-ring);' : ''}"
           onclick="setStatusFilter('${status}')"
           onkeydown="if(event.key==='Enter'||event.key===' '){event.preventDefault();setStatusFilter('${status}');}"`
        : '';
    return `<div class="stat-card flex items-center gap-4" ${attrs}>
        <div class="w-10 h-10 rounded-xl flex items-center justify-center shrink-0" style="background:${color}12;">${icon}</div>
        <div>
            <div class="text-xs font-medium" style="color:var(--text-muted);">${label}${active ? ' <span style="color:var(--accent-text);">• filtered</span>' : ''}</div>
            <div class="text-2xl font-bold" style="color:${color};">${value}</div>
        </div>
    </div>`;
}

// Toggle the dashboard status filter (empty status clears it).
function setStatusFilter(status) {
    state.filterStatus = (state.filterStatus === status) ? '' : status;
    state.dashboardPage = 1;
    renderDashboardContent();
}

// ---- Dashboard Widget Customization ----

const DASHBOARD_WIDGETS = [
    { id: 'stats',  label: 'Summary stats' },
    { id: 'health', label: 'Service health' },    { id: 'recent', label: 'Recent tests' },
    { id: 'queue',  label: 'Test queue' },
];

function persistWidgets() {
    localStorage.setItem('gload_widget_order', JSON.stringify(state.widgetOrder));
    localStorage.setItem('gload_hidden_widgets', JSON.stringify(state.hiddenWidgets));
}

function toggleWidget(id) {
    const i = state.hiddenWidgets.indexOf(id);
    if (i === -1) state.hiddenWidgets.push(id);
    else state.hiddenWidgets.splice(i, 1);
    persistWidgets();
    renderDashboardContent();
    renderCustomizeBody();
}

function moveWidget(id, dir) {
    // Reorder within the toggleable widgets, keeping 'services' pinned last.
    const order = state.widgetOrder.filter(w => w !== 'services');
    const i = order.indexOf(id);
    if (i === -1) return;
    const j = i + dir;
    if (j < 0 || j >= order.length) return;
    [order[i], order[j]] = [order[j], order[i]];
    order.push('services');
    state.widgetOrder = order;
    persistWidgets();
    renderDashboardContent();
    renderCustomizeBody();
}

function resetWidgets() {
    state.widgetOrder = ['stats', 'health', 'queue', 'recent', 'services'];
    state.hiddenWidgets = [];
    persistWidgets();
    renderDashboardContent();
    renderCustomizeBody();
}

function renderCustomizeBody() {
    const body = document.getElementById('customize-body');
    if (!body) return;
    // List in current order (toggleable widgets only).
    const ordered = state.widgetOrder.filter(w => w !== 'services' && DASHBOARD_WIDGETS.some(d => d.id === w));
    // Include any toggleable widget missing from the order (safety).
    DASHBOARD_WIDGETS.forEach(d => { if (!ordered.includes(d.id)) ordered.push(d.id); });

    body.innerHTML = ordered.map((id, idx) => {
        const meta = DASHBOARD_WIDGETS.find(d => d.id === id);
        const visible = !state.hiddenWidgets.includes(id);
        return `
        <div class="flex items-center justify-between py-2 px-1 rounded-lg" style="border-bottom:1px solid var(--divider);">
            <label class="flex items-center gap-2.5 cursor-pointer flex-1">
                <input type="checkbox" ${visible ? 'checked' : ''} onchange="toggleWidget('${id}')"
                       aria-label="Show ${meta.label}"
                       style="accent-color:var(--accent);width:16px;height:16px;cursor:pointer;">
                <span class="text-sm" style="color:${visible ? 'var(--text)' : 'var(--text-subtle)'};">${meta.label}</span>
            </label>
            <div class="flex items-center gap-1">
                <button onclick="moveWidget('${id}', -1)" ${idx === 0 ? 'disabled' : ''} aria-label="Move ${meta.label} up"
                        class="btn btn-icon btn-ghost btn-sm" style="padding:0.3rem;${idx === 0 ? 'opacity:0.3;' : ''}">
                    <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 15l7-7 7 7"/></svg>
                </button>
                <button onclick="moveWidget('${id}', 1)" ${idx === ordered.length - 1 ? 'disabled' : ''} aria-label="Move ${meta.label} down"
                        class="btn btn-icon btn-ghost btn-sm" style="padding:0.3rem;${idx === ordered.length - 1 ? 'opacity:0.3;' : ''}">
                    <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/></svg>
                </button>
            </div>
        </div>`;
    }).join('');
}

function showCustomizeDashboard() {
    const existing = document.getElementById('customize-modal');
    if (existing) { existing.remove(); return; }

    const overlay = document.createElement('div');
    overlay.id = 'customize-modal';
    overlay.className = 'modal-overlay';
    overlay.onclick = (e) => { if (e.target === overlay) overlay.remove(); };
    overlay.innerHTML = `
        <div class="modal-box" style="max-width:420px;">
            <div class="flex items-center justify-between mb-1">
                <h3 class="text-base font-semibold" style="color:var(--text);">Customize dashboard</h3>
                <button onclick="document.getElementById('customize-modal').remove()" aria-label="Close"
                        class="p-1 rounded hover:bg-slate-700/30" style="color:var(--text-muted);">${svgIcon('x')}</button>
            </div>
            <p class="text-xs mb-3" style="color:var(--text-subtle);">Show, hide, and reorder dashboard sections. The service list always stays at the bottom.</p>
            <div id="customize-body"></div>
            <div class="mt-4 pt-3 flex items-center justify-between" style="border-top:1px solid var(--divider);">
                <button onclick="resetWidgets()" class="btn btn-ghost btn-sm">Reset to default</button>
                <button onclick="document.getElementById('customize-modal').remove()" class="btn btn-primary btn-sm">Done</button>
            </div>
        </div>`;
    document.getElementById('modal-container').appendChild(overlay);
    renderCustomizeBody();
}

function svgStatIcon(name) {
    if (name === 'server') return '<svg class="w-5 h-5" style="color:var(--text-muted)" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-7-4h.01M12 16h.01"/></svg>';
    if (name === 'check') return '<svg class="w-5 h-5" style="color:#10b981" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>';
    if (name === 'activity') return '<svg class="w-5 h-5" style="color:#f59e0b" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>';
    if (name === 'warning') return '<svg class="w-5 h-5" style="color:#ef4444" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 9v2m0 4h.01M5.07 19h13.86c1.54 0 2.5-1.67 1.73-3L13.73 4a2 2 0 00-3.46 0L3.34 16c-.77 1.33.19 3 1.73 3z"/></svg>';
    return '';
}

function emptyState(title, desc, href, btnLabel) {
    return `<div class="card p-12 text-center">
        <div class="empty-icon">${svgIcon('bolt')}</div>
        <h3 class="text-lg font-semibold mb-2" style="color:var(--text);">${esc(title)}</h3>
        <p class="text-sm mb-6" style="color:var(--text-muted);">${esc(desc)}</p>
        ${href ? `<a href="${href}" class="btn btn-primary">${svgIcon('plus')}<span>${esc(btnLabel)}</span></a>` : ''}
    </div>`;
}

// Tiny inline RPS sparkline from a result's timeline points.
function sparkline(timeline, color) {
    if (!Array.isArray(timeline) || timeline.length < 2) return '';
    const vals = timeline.map(p => p.rps || 0);
    const max = Math.max(...vals, 1), min = Math.min(...vals);
    const range = (max - min) || 1;
    const W = 100, H = 22;
    const pts = vals.map((v, i) => {
        const x = (i / (vals.length - 1)) * W;
        const y = H - ((v - min) / range) * (H - 2) - 1;
        return `${x.toFixed(1)},${y.toFixed(1)}`;
    }).join(' ');
    return `<svg viewBox="0 0 ${W} ${H}" preserveAspectRatio="none" style="width:100%;height:22px;display:block;">
        <polyline points="${pts}" fill="none" stroke="${color}" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round" vector-effect="non-scaling-stroke"/>
    </svg>`;
}

// A service "has issues" when its last run failed assertions or errored >5%.
function serviceHasIssues(s) {
    const r = s.last_result;
    if (!r) return false;
    const errPct = r.total_reqs > 0 ? r.errors / r.total_reqs * 100 : 0;
    return r.status === 'fail' || errPct > 5;
}

// Human status word used for search matching.
function serviceStatusLabel(s) {
    if (s.is_running) return 'running';
    if (!s.last_result) return 'not tested untested';
    return serviceHasIssues(s) ? 'issues failing' : 'healthy';
}

function setSort(mode) {
    state.sortBy = mode;
    localStorage.setItem('gload_sort_by', mode);
    state.dashboardPage = 1;
    renderDashboardContent();
}

function togglePin(id) {
    id = String(id);
    const i = state.pinned.indexOf(id);
    if (i === -1) state.pinned.push(id); else state.pinned.splice(i, 1);
    localStorage.setItem('gload_pinned', JSON.stringify(state.pinned));
    renderDashboardContent();
}

function setAllGroupsCollapsed(collapsed) {
    const groups = [...new Set(getFilteredServices().map(s => s.group_name || '__ungrouped'))];
    groups.forEach(g => { state.collapsedGroups[g] = collapsed; });
    renderDashboardContent();
}

// Wraps rendered service cards in a grid or a vertical list per view mode.
function serviceListWrap(inner) {
    return state.viewMode === 'list'
        ? `<div class="flex flex-col gap-2">${inner}</div>`
        : `<div class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">${inner}</div>`;
}

function setViewMode(mode) {
    state.viewMode = mode;
    localStorage.setItem('gload_view_mode', mode);
    renderDashboardContent();
}

function renderServiceCard(svc, index) {
    const r = svc.last_result;
    const running = svc.is_running;
    const idx = typeof index === 'number' ? index : 0;

    let badge;
    if (running)    badge = statusBadge('Running', '#f59e0b', true);
    else if (r) {
        const errPct = r.total_reqs > 0 ? r.errors / r.total_reqs * 100 : 0;
        badge = errPct > 5 ? statusBadge('Issues', '#ef4444') : statusBadge('Healthy', '#10b981');
    } else          badge = statusBadge('Not tested', 'var(--text-subtle)');

    const runningLink = svc.running_kind === 'capacity' ? `#/services/${svc.id}/capacity` : `#/services/${svc.id}/run`;
    const link = running ? runningLink : `#/services/${svc.id}`;

    const lastTested = r && r.created_at ? `<div class="text-[10px] mt-3" style="color:var(--text-subtle);">Last tested: ${fmtRelativeTime(r.created_at)}</div>` : '';

    const tagsArr = parseTags(svc.tags);
    const cap = svc.capacity;
    const capPill = (cap && cap.max_rps > 0)
        ? `<span onclick="event.preventDefault();event.stopPropagation();navigate('/services/${svc.id}/capacity')" class="text-[10px] px-1.5 py-0.5 rounded-full font-medium inline-flex items-center gap-1" style="background:rgba(6,182,212,0.12);color:#06b6d4;cursor:pointer;" title="View capacity result (saturates at ~${fmt(cap.knee_concurrency)} concurrent)"><svg style="width:10px;height:10px;flex-shrink:0;" fill="currentColor" viewBox="0 0 24 24"><path d="M13 2L4.09 12.63a1 1 0 00.76 1.63H11v5.74a1 1 0 001.76.65L21.67 9.37A1 1 0 0020.91 7.74H15V2.26A1 1 0 0013 2z"/></svg><span>cap ~${fmtDec(cap.max_rps)}/s</span></span>` : '';
    const tagPills = (tagsArr.length > 0 || capPill) ?
        `<div class="flex flex-wrap items-center gap-1 mt-2">${capPill}${tagsArr.map(t => `<span class="text-[10px] px-1.5 py-0.5 rounded-full font-medium" style="background:rgba(124,58,237,0.15);color:var(--accent-text);">${esc(t)}</span>`).join('')}</div>` : '';

    // Live progress block shown while the test is running.
    const livePct = svc.live ? Math.round((svc.live.progress || 0) * 100) : 0;
    const liveSpark = (running && svc.live && svc.live.timeline && svc.live.timeline.length >= 2)
        ? `<div class="mt-2" title="Requests/sec (live)">${renderSparkline(svc.live.timeline, 'rps', '#06b6d4')}</div>` : '';
    const isCapacityRun = svc.running_kind === 'capacity';
    const liveBlock = (running && svc.live) ? `
    <div class="mt-4 pt-4 border-t border-slate-700/30">
        <div class="flex items-center justify-between mb-1.5">
            <span class="text-[10px] uppercase tracking-wider" style="color:#f59e0b;">${isCapacityRun ? 'Finding capacity…' : `Running · ${livePct}%`}</span>
            <span class="text-xs font-bold font-mono" style="color:#06b6d4;">${fmtDec(svc.live.rps)} rps</span>
        </div>
        ${isCapacityRun ? '' : `<div class="progress-track"><div class="progress-fill" style="width:${livePct}%;"></div></div>`}
        ${liveSpark}
    </div>` : '';

    const stats = liveBlock || (r ? `
    <div class="grid grid-cols-3 gap-3 mt-4 pt-4 border-t border-slate-700/30">
        <div><div class="text-[10px] uppercase tracking-wider mb-0.5" style="color:var(--text-subtle);">RPS</div><div class="text-sm font-bold font-mono" style="color:#06b6d4;">${fmtDec(r.rps)}</div></div>
        <div><div class="text-[10px] uppercase tracking-wider mb-0.5" style="color:var(--text-subtle);">Latency</div><div class="text-sm font-bold font-mono" style="color:var(--text);">${fmtLatency(r.avg_latency_ms)}</div></div>
        <div><div class="text-[10px] uppercase tracking-wider mb-0.5" style="color:var(--text-subtle);">Errors</div><div class="text-sm font-bold font-mono" style="color:${(r.errors||0)>0?'#ef4444':'#10b981'};">${fmtDec(r.total_reqs>0?r.errors/r.total_reqs*100:0)}%</div></div>
    </div>${lastTested}` : '');

    const selected = state.bulkSelected.includes(String(svc.id));
    // In bulk mode the whole card toggles selection; the checkbox is a visual
    // indicator only (pointer-events:none) so clicks reach the card handler.
    const selectedStyle = state.bulkMode && selected ? 'border-color:var(--accent);box-shadow:0 0 0 1px var(--accent-ring);' : '';
    const checkbox = state.bulkMode ? `
        <div class="absolute top-3 left-3 z-10" style="pointer-events:none;">
            <input type="checkbox" ${selected ? 'checked' : ''} tabindex="-1" aria-hidden="true"
                   style="accent-color:var(--accent);width:18px;height:18px;">
        </div>` : '';
    const runBtn = running
        ? `<button onclick="event.preventDefault();event.stopPropagation();handleStopService('${svc.id}')"
                   class="btn btn-icon btn-ghost btn-sm" style="padding:0.35rem;" title="Stop test" aria-label="Stop ${esc(svc.name)}">${svgIcon('stop')}</button>`
        : `<button onclick="event.preventDefault();event.stopPropagation();handleRunService('${svc.id}')"
                   class="btn btn-icon btn-ghost btn-sm" style="padding:0.35rem;color:#10b981;" title="Run test" aria-label="Run ${esc(svc.name)}">${svgIcon('play')}</button>`;
    const pinned = state.pinned.includes(String(svc.id));
    const pinBtn = `<button onclick="event.preventDefault();event.stopPropagation();togglePin('${svc.id}')"
            class="btn btn-icon btn-ghost btn-sm" style="padding:0.35rem;${pinned ? 'color:var(--accent-text);' : ''}" title="${pinned ? 'Unpin' : 'Pin to top'}" aria-label="${pinned ? 'Unpin' : 'Pin'} ${esc(svc.name)}">${svgIcon(pinned ? 'pinFilled' : 'pin')}</button>`;
    const cardActions = state.bulkMode ? '' : `
        <div class="card-actions absolute top-3 right-3 opacity-0 group-hover:opacity-100 flex items-center gap-1">
            ${runBtn}
            ${pinBtn}
            <button onclick="event.preventDefault();event.stopPropagation();handleAddToQueue('${svc.id}')"
                    class="btn btn-icon btn-ghost btn-sm" style="padding:0.35rem;" title="Add to Queue" aria-label="Add ${esc(svc.name)} to queue">
                ${svgIcon('queue')}
            </button>
            <button onclick="event.preventDefault();event.stopPropagation();handleDeleteService('${svc.id}')"
                    class="btn btn-icon btn-ghost btn-sm" style="padding:0.35rem;" title="Delete service" aria-label="Delete ${esc(svc.name)}">
                ${svgIcon('x')}
            </button>
        </div>`;

    const dragAttrs = `draggable="${state.bulkMode ? 'false' : 'true'}" data-service-id="${svc.id}"
       ondragstart="handleServiceDragStart(event, '${svc.id}')"
       ondragover="handleServiceDragOver(event)"
       ondrop="handleServiceDrop(event, '${svc.id}')"
       ondragend="handleServiceDragEnd(event)"
       onclick="handleServiceCardClick(event, '${svc.id}')"`;

    // ----- List (row) layout -----
    if (state.viewMode === 'list') {
        const errPct = r && r.total_reqs > 0 ? r.errors / r.total_reqs * 100 : 0;
        // Trailing area: status badge by default, swapped for action buttons on
        // hover (same slot) so the icons never overlap the badge.
        const listActions = state.bulkMode ? `<div class="shrink-0">${badge}</div>` : `
            <div class="relative shrink-0 flex items-center justify-end" style="min-width:104px;">
                <div class="group-hover:opacity-0 transition-opacity">${badge}</div>
                <div class="absolute right-0 top-1/2 -translate-y-1/2 opacity-0 group-hover:opacity-100 transition-opacity flex items-center gap-1" style="background:var(--surface);padding-left:0.5rem;">
                    ${runBtn}
                    <button onclick="event.preventDefault();event.stopPropagation();handleAddToQueue('${svc.id}')"
                            class="btn btn-icon btn-ghost btn-sm" style="padding:0.35rem;" title="Add to Queue" aria-label="Add ${esc(svc.name)} to queue">${svgIcon('queue')}</button>
                    <button onclick="event.preventDefault();event.stopPropagation();handleDeleteService('${svc.id}')"
                            class="btn btn-icon btn-ghost btn-sm" style="padding:0.35rem;" title="Delete service" aria-label="Delete ${esc(svc.name)}">${svgIcon('x')}</button>
                </div>
            </div>`;
        return `
        <a href="${link}" class="card card-link group relative service-card-enter gap-4 px-4 py-3" style="display:flex;align-items:center;animation-delay:${idx * 0.02}s;${state.bulkMode ? 'padding-left:2.75rem;' : ''}${selectedStyle}"
           ${dragAttrs}>
            ${checkbox}
            <div class="flex items-center gap-2 shrink-0" style="width:64px;">${methodBadge(svc.method)}</div>
            <div class="flex-1 min-w-0">
                <div class="text-sm font-semibold truncate group-hover:text-violet-300 transition-colors" style="color:var(--text);">${esc(svc.name)}</div>
                <div class="text-[11px] font-mono truncate" style="color:var(--text-subtle);">${esc(svc.url)}</div>
            </div>
            ${r ? `
            <div class="hidden md:block text-right shrink-0" style="width:90px;"><div class="text-[10px] uppercase tracking-wider" style="color:var(--text-subtle);">RPS</div><div class="text-sm font-bold font-mono" style="color:#06b6d4;">${fmtDec(r.rps)}</div></div>
            <div class="hidden md:block text-right shrink-0" style="width:90px;"><div class="text-[10px] uppercase tracking-wider" style="color:var(--text-subtle);">Latency</div><div class="text-sm font-bold font-mono" style="color:var(--text);">${fmtLatency(r.avg_latency_ms)}</div></div>
            <div class="hidden md:block text-right shrink-0" style="width:80px;"><div class="text-[10px] uppercase tracking-wider" style="color:var(--text-subtle);">Errors</div><div class="text-sm font-bold font-mono" style="color:${(r.errors||0)>0?'#ef4444':'#10b981'};">${fmtDec(errPct)}%</div></div>
            ` : '<div class="hidden md:block shrink-0" style="width:260px;"></div>'}
            ${listActions}
        </a>`;
    }

    // Sparkline of the last run's RPS shape (grid, tested & not running).
    const spark = (!running && r && Array.isArray(r.timeline) && r.timeline.length > 1)
        ? `<div class="mt-3" title="RPS over the last run">${sparkline(r.timeline, '#06b6d4')}</div>` : '';
    // Persistent pin marker (top-left) so pinned cards are recognizable at rest.
    const pinMarker = (pinned && !state.bulkMode) ? `<div class="absolute top-3 left-3" style="color:var(--accent-text);" title="Pinned">${svgIcon('pinFilled')}</div>` : '';

    // ----- Grid (card) layout -----
    return `
    <a href="${link}" class="card card-link p-5 group relative service-card-enter" style="animation-delay: ${idx * 0.03}s;${state.bulkMode || pinned ? 'padding-left:2.75rem;' : ''}${selectedStyle}"
       ${dragAttrs}>
        ${checkbox}
        ${pinMarker}
        ${cardActions}
        <div class="flex items-center gap-2 mb-3">${methodBadge(svc.method)} ${badge}</div>
        <h3 class="text-base font-semibold mb-1 group-hover:text-violet-300 transition-colors" style="color:var(--text);">${esc(svc.name)}</h3>
        <p class="text-xs font-mono truncate" style="color:var(--text-subtle);">${esc(svc.url)}</p>
        ${tagPills}
        ${stats}
        ${spark}
    </a>`;
}

function statusBadge(label, color, pulse) {
    return `<span class="status-badge" style="background:${color}15;color:${color};">
        ${pulse ? '<span class="w-1.5 h-1.5 rounded-full animate-pulse" style="background:'+color+'"></span>' :
                  '<span class="w-1.5 h-1.5 rounded-full" style="background:'+color+'"></span>'}
        ${label}</span>`;
}

// ---- Analytics Panels ----

async function renderInsightsPanel(id) {
    let insights = [];
    try { insights = await api(`/api/services/${id}/insights`); } catch(_) {}
    if (!insights || insights.length === 0) return '';

    const typeConfig = {
        critical: { color: '#ef4444', icon: '!', bg: 'rgba(239,68,68,0.08)' },
        warning:  { color: '#f59e0b', icon: '!', bg: 'rgba(245,158,11,0.08)' },
        info:     { color: '#06b6d4', icon: 'i', bg: 'rgba(6,182,212,0.08)' },
        success:  { color: '#10b981', icon: '✓', bg: 'rgba(16,185,129,0.08)' },
    };

    return `
    <div class="card p-5 mb-6">
        <div class="flex items-center gap-2 mb-4">
            <svg class="w-4 h-4" style="color:var(--accent-text);" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z"/>
            </svg>
            <h3 class="text-sm font-semibold" style="color:var(--accent-text);">Performance Insights</h3>
        </div>
        <div class="space-y-3">
            ${insights.map(i => {
                const cfg = typeConfig[i.type] || typeConfig.info;
                const changeText = i.change ? ` (${i.change > 0 ? '+' : ''}${i.change.toFixed(1)}%)` : '';
                return `
                <div class="p-3 rounded-lg" style="background:${cfg.bg};border-left:3px solid ${cfg.color};">
                    <div class="text-xs font-semibold mb-1" style="color:${cfg.color};">${esc(i.title)}${changeText}</div>
                    <div class="text-xs" style="color:var(--text-muted);">${esc(i.detail)}</div>
                </div>`;
            }).join('')}
        </div>
    </div>`;
}

async function renderCapacityCard(id) {
    let cap = null;
    try { cap = await api(`/api/services/${id}/capacity`); } catch(_) {}
    if (!cap || !cap.current_rps) return '';

    // Verdict from MEASURED data (P95 + error rate at the tested load), not a
    // model headroom %. Plain language, honest.
    const measRps = cap.current_rps || 0;
    const measP95 = cap.current_p95_ms || 0;
    const measErr = cap.current_error_pct || 0;
    const maxRps = (cap.est_max_rps || 0);

    let vColor, vText;
    if (measErr > 5) {
        vColor = '#ef4444';
        vText = `Errors reached ${fmtDec(measErr)}% at ~${measRps.toFixed(0)} req/s — the service looks at or past its limit here.`;
    } else if (measP95 > 1000) {
        vColor = '#f59e0b';
        vText = `P95 latency was ${fmtLatency(measP95)} at ~${measRps.toFixed(0)} req/s — response times are climbing under this load.`;
    } else {
        vColor = '#10b981';
        vText = `Handled ~${measRps.toFixed(0)} req/s comfortably — P95 stayed at ${fmtLatency(measP95)} with ${fmtDec(measErr)}% errors.`;
    }

    // Projection bars
    const projections = cap.projections || [];
    const projMaxRps = Math.max(1, ...projections.map(p => p.rps));
    // Estimated latency-vs-load curve. The fabricated error% / Safe-At-Risk
    // verdicts were removed — they were pure model output presented as fact.
    const projHtml = projections.map(p => {
        const barPct = (p.rps / projMaxRps) * 100;
        const latColor = p.est_latency_ms > 500 ? '#ef4444' : p.est_latency_ms > 200 ? '#f59e0b' : '#10b981';
        return `
        <div class="flex items-center gap-3 py-2 border-b border-slate-700/10">
            <div class="w-16 text-right text-xs font-mono font-medium" style="color:var(--text);">${p.rps.toFixed(0)}/s</div>
            <div class="flex-1 h-5 rounded-full relative" style="background:rgba(71,85,105,0.15);">
                <div class="h-full rounded-full transition-all" style="width:${barPct}%;background:#06b6d4;opacity:0.6;"></div>
            </div>
            <div class="w-20 text-right text-xs font-mono" style="color:${latColor};">~${p.est_latency_ms.toFixed(0)}ms</div>
        </div>`;
    }).join('');

    const capCollapsed = isPanelCollapsed('capacity');
    return `
    <div class="card p-5 mb-6" style="border-color:rgba(6,182,212,0.15);">
        <div class="flex items-center gap-2 mb-4">
            <svg class="w-4 h-4" style="color:#06b6d4;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6"/></svg>
            <h3 class="text-sm font-semibold flex-1" style="color:#06b6d4;">Capacity Planning</h3>
            ${panelChevron('capacity')}
        </div>
        <div id="panel-body-capacity" style="display:${capCollapsed ? 'none' : ''};">
        <!-- Plain-language verdict from measured data -->
        <div class="flex items-start gap-2 mb-4 p-3 rounded-lg" style="background:${vColor}0f;border-left:3px solid ${vColor};">
            <span class="text-sm" style="color:var(--text);">${vText}</span>
        </div>

        <div class="flex items-center justify-between mb-4">
            <div>
                <div class="text-[10px] uppercase tracking-wider" style="color:var(--text-subtle);">Estimated max throughput</div>
                <div class="text-2xl font-bold font-mono" style="color:var(--accent-text);">~${maxRps.toFixed(0)}<span class="text-sm font-normal" style="color:var(--text-subtle);"> req/s</span></div>
            </div>
            <span class="text-[10px] px-1.5 py-0.5 rounded-full" style="background:var(--surface-2);color:var(--text-subtle);" title="Extrapolated from this run using Little's Law — a rough estimate.">estimate</span>
        </div>

        ${projections.length > 0 ? `
        <div class="mb-1">
            <div class="flex items-center justify-between mb-2">
                <h4 class="text-xs font-semibold" style="color:var(--text-muted);">Estimated latency at higher load</h4>
                <div class="flex items-center gap-3 text-[9px]" style="color:var(--text-faint);">
                    <span>Target req/s</span><span>Est. latency</span>
                </div>
            </div>
            ${projHtml}
        </div>` : ''}
        </div>
    </div>`;
}

// ---- Service Detail ----

// ---- Collapsible detail panels (persisted) ----
function isPanelCollapsed(key) {
    try { return JSON.parse(localStorage.getItem('gload_collapsed_panels') || '{}')[key] === true; }
    catch (_) { return false; }
}
function togglePanel(key, btn) {
    const collapsed = !isPanelCollapsed(key);
    let m = {};
    try { m = JSON.parse(localStorage.getItem('gload_collapsed_panels') || '{}'); } catch (_) {}
    m[key] = collapsed;
    localStorage.setItem('gload_collapsed_panels', JSON.stringify(m));
    const body = document.getElementById('panel-body-' + key);
    if (body) body.style.display = collapsed ? 'none' : '';
    if (btn) btn.style.transform = collapsed ? 'rotate(-90deg)' : 'none';
}
function panelChevron(key) {
    const collapsed = isPanelCollapsed(key);
    return `<button onclick="event.stopPropagation();togglePanel('${key}', this)" aria-label="Collapse section" class="btn btn-icon btn-ghost btn-sm" style="padding:0.25rem;transform:${collapsed ? 'rotate(-90deg)' : 'none'};transition:transform .15s;">
        <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/></svg>
    </button>`;
}

// Sensitive header keys whose values are masked (with click-to-reveal) so
// tokens aren't exposed on screen.
function isSensitiveHeader(key) {
    return /^(authorization|cookie|proxy-authorization|x-api-key|x-auth-token|api-key|token|secret)$/i.test(key.trim());
}

function headerValueHtml(key, val, idx) {
    if (!isSensitiveHeader(key)) {
        return `<span style="color:var(--text);">${esc(val)}</span>`;
    }
    const hid = `hdr-val-${idx}`;
    return `<span id="${hid}" data-real="${esc(val)}" style="color:var(--text);">••••••••</span>
        <button onclick="toggleHeaderValue('${hid}', this)" class="text-[10px] ml-1" style="color:var(--accent-text);background:none;border:none;cursor:pointer;">show</button>`;
}

function toggleHeaderValue(hid, btn) {
    const el = document.getElementById(hid);
    if (!el) return;
    if (btn.textContent === 'show') {
        el.textContent = el.getAttribute('data-real');
        btn.textContent = 'hide';
    } else {
        el.textContent = '••••••••';
        btn.textContent = 'show';
    }
}

// A labeled divider that groups the detail page into readable sections.
function sectionHeader(title, subtitle) {
    return `<div class="flex items-baseline gap-3 mt-8 mb-4">
        <h3 class="text-lg font-semibold" style="color:var(--text);">${esc(title)}</h3>
        ${subtitle ? `<span class="text-xs" style="color:var(--text-subtle);">${esc(subtitle)}</span>` : ''}
    </div>`;
}

// Top-of-page verdict + the four numbers a user actually cares about, all from
// the measured result (no estimates).
// launchReadiness renders a verdict card for launch-spike (open-model) runs:
// did the system survive the surge? Only shown when the run was a launch sim.
function launchReadiness(results, id) {
    let rc = results.run_config;
    if (typeof rc === 'string') { try { rc = JSON.parse(rc); } catch (_) { rc = null; } }
    if (!rc || !rc.open_model) return '';

    let tl = results.timeline;
    if (typeof tl === 'string') { try { tl = JSON.parse(tl); } catch (_) { tl = []; } }
    if (!Array.isArray(tl)) tl = [];

    let stages = rc.stages;
    if (typeof stages === 'string') { try { stages = JSON.parse(stages); } catch (_) { stages = []; } }
    const targetPeak = Array.isArray(stages) && stages.length ? Math.max(0, ...stages.map(s => s.target || 0)) : 0;

    const total = results.total_reqs || 0;
    const errRate = total > 0 ? (results.errors || 0) / total * 100 : 0;
    const peakRps = tl.length ? Math.max(0, ...tl.map(p => p.rps || 0)) : (results.rps || 0);

    const hi = tl.filter(p => (p.rps || 0) >= peakRps * 0.8);
    const lo = tl.filter(p => (p.rps || 0) > 0 && (p.rps || 0) <= peakRps * 0.3);
    const peakLat = hi.length ? Math.max(...hi.map(p => p.lat_ms || 0)) : (results.p95_latency_ms || 0);
    const baseLatVals = lo.map(p => p.lat_ms || 0).filter(v => v > 0);
    const baseLat = baseLatVals.length ? Math.min(...baseLatVals) : (results.avg_latency_ms || 0);

    const reachedTarget = targetPeak === 0 || peakRps >= targetPeak * 0.85;
    const latDegraded = baseLat > 0 && peakLat > baseLat * 3;

    let verdict, color, bg, msg;
    if (errRate > 5 || !reachedTarget) {
        verdict = 'Not ready'; color = '#ef4444'; bg = 'rgba(239,68,68,0.10)';
        const why = [];
        if (!reachedTarget && targetPeak > 0) why.push(`throughput topped out at ~${fmtDec(peakRps)} of the ${fmt(targetPeak)} req/sec target — arrivals piled up faster than they could be served`);
        if (errRate > 5) why.push(`errors reached ${fmtDec(errRate)}%`);
        msg = `The system would buckle under this launch: ${why.join(', ')}. Scale out and/or cache the entry page before going live, then re-run.`;
    } else if (errRate > 1 || latDegraded) {
        verdict = 'At risk'; color = '#f59e0b'; bg = 'rgba(245,158,11,0.10)';
        const why = [];
        if (errRate > 1) why.push(`errors climbed to ${fmtDec(errRate)}%`);
        if (latDegraded) why.push(`latency rose from ~${fmtLatency(baseLat)} to ~${fmtLatency(peakLat)} at the peak`);
        msg = `It reached the peak but ${why.join(' and ')}. It may hold, but users will feel it — add headroom (cache or more instances) before launch.`;
    } else {
        verdict = 'Ready'; color = '#10b981'; bg = 'rgba(16,185,129,0.10)';
        msg = `Held the ~${fmtDec(peakRps)} req/sec peak with ${fmtDec(errRate)}% errors and latency staying around ${fmtLatency(peakLat)}. Your system can take this launch.`;
    }

    return `
    <div class="card p-6 mb-6" style="border-color:${color}55;background:${bg};">
        <div class="flex items-center gap-3 mb-2">
            <span class="text-xs uppercase tracking-wider" style="color:var(--text-subtle);">Launch readiness</span>
            <span class="text-xs font-bold px-2 py-0.5 rounded-full" style="background:${color}22;color:${color};border:1px solid ${color}55;">${verdict.toUpperCase()}</span>
        </div>
        <p class="text-sm" style="color:var(--text-muted);">${msg}</p>
        <div class="grid grid-cols-3 gap-4 mt-5 max-w-2xl">
            ${metricCard('Peak reached', fmtDec(peakRps) + '/s', 'var(--text)', 'lr-peak', targetPeak > 0 ? `<div class="text-[10px] mt-1" style="color:var(--text-subtle);">target ${fmt(targetPeak)}/s</div>` : '')}
            ${metricCard('Error rate', fmtDec(errRate) + '%', errRate > 5 ? '#ef4444' : errRate > 1 ? '#f59e0b' : '#10b981', 'lr-err')}
            ${metricCard('Latency at peak', fmtLatency(peakLat), '#7c3aed', 'lr-lat', `<div class="text-[10px] mt-1" style="color:var(--text-subtle);">at rest ${fmtLatency(baseLat)}</div>`)}
        </div>
        <div class="mt-4"><a href="#/services/${id}/capacity" class="text-xs" style="color:var(--accent-text);">Not sure how many instances you need? Run Find Capacity →</a></div>
    </div>`;
}

function renderSummaryHeader(results) {
    if (!results) return '';
    const errorRate = results.total_reqs > 0 ? ((results.errors || 0) / results.total_reqs * 100) : 0;
    const errColor = errorRate > 10 ? '#ef4444' : errorRate > 2 ? '#f59e0b' : '#10b981';
    const assertions = parseJSON(results.assertion_results, []);
    let verdict, vColor, vBg;
    if (assertions.length > 0) {
        const pass = (results.status || (assertions.every(a => a.passed) ? 'pass' : 'fail')) === 'pass';
        verdict = pass ? 'PASSED' : 'FAILED';
        vColor = pass ? '#10b981' : '#ef4444';
    } else {
        verdict = 'COMPLETED'; vColor = '#06b6d4';
    }
    vBg = vColor + '1f';
    const when = results.created_at ? fmtRelativeTime(results.created_at) : '';

    const kpi = (label, value, color, sub) => `
        <div class="stat-card">
            <div class="text-xs font-medium mb-1" style="color:var(--text-muted);">${label}</div>
            <div class="text-2xl font-bold stat-number" style="color:${color};">${value}</div>
            ${sub ? `<div class="text-[11px] mt-0.5" style="color:var(--text-subtle);">${sub}</div>` : ''}
        </div>`;

    return `
    <div class="card p-5 mb-6">
        <div class="flex items-center gap-3 mb-4">
            <span class="px-3 py-1 rounded-full text-sm font-bold" style="background:${vBg};color:${vColor};">${verdict}</span>
            <span class="text-sm" style="color:var(--text-muted);">Last test result${when ? ' · ' + when : ''}</span>
        </div>
        <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
            ${kpi('Total Requests', fmt(results.total_reqs), 'var(--text)')}
            ${kpi('Requests/sec', fmtDec(results.rps), '#06b6d4')}
            ${kpi('Error Rate', fmtDec(errorRate) + '%', errColor, `${fmt(results.errors || 0)} errors`)}
            ${kpi('P95 Latency', fmtLatency(results.p95_latency_ms), 'var(--text)', `avg ${fmtLatency(results.avg_latency_ms)}`)}
        </div>
    </div>`;
}

// Honest latency picture: real percentile bars (no synthesized distribution).
function renderLatencyBars(m) {
    const rows = [['P50', m.p50_latency_ms], ['P95', m.p95_latency_ms], ['P99', m.p99_latency_ms], ['Max', m.max_latency_ms]];
    const max = Math.max(1, m.max_latency_ms || 0);
    return `<div class="space-y-3">${rows.map(([label, v]) => {
        const pct = Math.min(100, ((v || 0) / max) * 100);
        const c = (v || 0) > 500 ? '#ef4444' : (v || 0) > 200 ? '#f59e0b' : '#06b6d4';
        return `<div>
            <div class="flex justify-between mb-1">
                <span class="text-xs font-mono font-medium" style="color:var(--text-muted);">${label}</span>
                <span class="text-xs font-mono font-semibold" style="color:var(--text);">${fmtLatency(v)}</span>
            </div>
            <div class="w-full h-2 rounded-full" style="background:${c}18;"><div class="h-full rounded-full" style="width:${pct}%;background:${c};transition:width .2s;"></div></div>
        </div>`;
    }).join('')}</div>`;
}

// Lightweight placeholder shown while the detail page's data loads.
function detailSkeleton(svc) {
    const name = svc ? esc(svc.name) : 'Loading…';
    return `
    ${breadcrumb({ label: 'Dashboard', href: '#/' }, { label: svc ? svc.name : 'Service' })}
    <div class="flex items-center gap-3 mb-8">
        <h2 class="text-2xl font-bold" style="color:var(--text);">${name}</h2>
        <div class="spinner" style="border-top-color:var(--accent);border-color:var(--border-strong);border-top-color:var(--accent);"></div>
    </div>
    <div class="skeleton mb-6" style="height:120px;"></div>
    <div class="skeleton mb-6" style="height:88px;"></div>
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <div class="skeleton" style="height:88px;"></div><div class="skeleton" style="height:88px;"></div>
        <div class="skeleton" style="height:88px;"></div><div class="skeleton" style="height:88px;"></div>
    </div>
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div class="skeleton" style="height:220px;"></div><div class="skeleton" style="height:220px;"></div>
    </div>`;
}

async function renderServiceDetail(id) {
    const svc = state.services.find(s => s.id == id);
    if (!svc) return notFound();

    // Fetch everything in parallel; the analytics panels tolerate no-data.
    const val = (r) => r.status === 'fulfilled' ? r.value : null;
    const [resultsR, historyR, insightsR, capacityR] = await Promise.allSettled([
        api(`/api/services/${id}/results`),
        api(`/api/services/${id}/history`),
        renderInsightsPanel(id),
        renderCapacityCard(id),
    ]);
    const results = val(resultsR);
    const history = val(historyR) || [];
    // Analytics panels only shown when a result exists.
    const insightsHtml = results ? (val(insightsR) || '') : '';
    const capacityHtml = results ? (val(capacityR) || '') : '';

    const hdrs = Object.entries(svc.headers || {});
    const createdAt = svc.created_at ? `<span class="text-xs" style="color:var(--text-subtle);">Created ${fmtDateShort(svc.created_at)}</span>` : '';

    // Parse steps for chain visualization
    const svcSteps = parseJSON(svc.steps, []);

    // Timeline charts
    const timelineCharts = (results && results.timeline && results.timeline.length > 0) ? renderTimelineCharts(results.timeline) : '';

    // Compare mode
    const compareSection = (history.length >= 2) ? `
        <button onclick="toggleCompareMode('${id}')" class="btn btn-ghost btn-sm" id="compare-toggle-btn">
            ${svgIcon('compare')}<span>${state.compareMode ? 'Cancel Compare' : 'Compare'}</span>
        </button>` : '';

    // Assertion results from last test
    const assertionResultsHtml = renderAssertionResults(results);

    // Test profiles dropdown
    const profiles = parseJSON(svc.profiles, []);
    const profileDropdown = profiles.length > 0 ? `
        <div class="relative inline-block" id="profile-dropdown-wrap">
            <button onclick="toggleProfileDropdown()" class="btn btn-ghost btn-sm" style="padding-right:0.5rem;">
                ${svgIcon('collapseDown')}
            </button>
            <div id="profile-dropdown" class="absolute right-0 top-full mt-1 w-48 rounded-lg border border-slate-700/50 shadow-xl z-50" style="background:var(--surface);display:none;">
                ${profiles.map((p, i) => `
                    <button onclick="handleRunProfile('${id}',${i});toggleProfileDropdown();" class="w-full text-left px-4 py-2.5 text-sm hover:bg-slate-700/40 transition-colors" style="color:var(--text);">
                        Run ${esc(p.name)}
                        <span class="block text-[10px] mt-0.5" style="color:var(--text-subtle);">${p.concurrency}c / ${p.duration} / ${p.rps ? p.rps + ' rps' : 'unlimited'}</span>
                    </button>`).join('')}
            </div>
        </div>` : '';

    // Cookie jar info
    const cookieJarInfo = svc.cookie_jar ? `<span class="text-[10px] px-1.5 py-0.5 rounded-full font-medium" style="background:rgba(6,182,212,0.15);color:#06b6d4;">Cookie Jar</span>` : '';

    return `
    ${breadcrumb({ label: 'Dashboard', href: '#/' }, { label: svc.name })}

    <div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-3 mb-8">
        <div class="min-w-0">
            <div class="flex items-center gap-3 mb-1 min-w-0">
                <h2 class="text-2xl font-bold truncate" style="color:var(--text);max-width:32rem;" title="${esc(svc.name)}">${esc(svc.name)}</h2>
                ${methodBadge(svc.method, 'lg')}
                ${cookieJarInfo}
            </div>
            <div class="flex items-center gap-3 min-w-0">
                <p class="text-sm font-mono truncate" style="color:var(--text-muted);max-width:32rem;" title="${esc(svc.url)}">${esc(svc.url)}</p>
                ${createdAt}
            </div>
        </div>
        <div class="flex flex-wrap items-center justify-end gap-2 shrink-0">
            <button onclick="handleCloneService('${id}')" class="btn btn-ghost btn-sm">${svgIcon('clone')}<span>Clone</span></button>
            <button onclick="handleEditService('${id}')" class="btn btn-ghost btn-sm">${svgIcon('edit')}<span>Edit</span></button>
            <button onclick="handleRunTest('${id}')" id="run-btn" class="btn btn-success btn-sm">${svgIcon('play')}<span class="btn-label">Run Test</span><span class="spinner"></span></button>
            ${profileDropdown}
            <a href="#/services/${id}/capacity" class="btn btn-ghost btn-sm" title="Find this service's capacity (max sustainable load)"><svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg><span>Capacity</span></a>
            <button onclick="togglePatternPanel('${id}')" class="btn btn-ghost btn-sm" title="Run a predefined load pattern (spike, ramp, soak…)"><svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 12h4l3 8 4-16 3 8h4"/></svg><span>Load Patterns</span></button>
            <button onclick="showLaunchModal('${id}')" class="btn btn-ghost btn-sm" title="Simulate a launch spike — like a TV ad or product drop"><svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 7h8m0 0v8m0-8l-8 8-4-4-6 6"/></svg><span>Simulate Launch</span></button>
            <div class="relative inline-block" id="more-actions-wrap">
                <button onclick="toggleMoreActions()" class="btn btn-ghost btn-sm" title="More actions">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 5v.01M12 12v.01M12 19v.01"/></svg>
                </button>
                <div id="more-actions-dropdown" class="absolute right-0 top-full mt-1 w-56 rounded-lg border border-slate-700/50 shadow-xl z-50" style="background:var(--surface);display:none;">
                    <button onclick="handleRunDistributed('${id}')" class="w-full text-left px-4 py-2.5 text-sm hover:bg-slate-700/40" style="color:var(--text);">Run Distributed</button>
                    <button onclick="handleGitHubComment('${id}')" class="w-full text-left px-4 py-2.5 text-sm hover:bg-slate-700/40" style="color:var(--text);">Post to GitHub PR</button>
                    <button onclick="window.open('/api/services/${id}/junit','_blank')" class="w-full text-left px-4 py-2.5 text-sm hover:bg-slate-700/40" style="color:var(--text);">Download JUnit XML</button>
                    <button onclick="window.open('/api/services/${id}/pdf','_blank')" class="w-full text-left px-4 py-2.5 text-sm hover:bg-slate-700/40" style="color:var(--text);">Save as PDF</button>
                </div>
            </div>
        </div>
    </div>

    <!-- Load Patterns panel (toggled from the top action row) -->
    <div id="pattern-panel" style="display:none;" class="mb-6"></div>

    ${results ? `
    <!-- SUMMARY: verdict + the four numbers that matter, plus narration -->
    ${launchReadiness(results, id)}
    ${renderSummaryHeader(results)}
    ${insightsHtml}
    ${assertionResultsHtml}
    <div class="flex items-center justify-end gap-2 mb-2">
        <button onclick="window.open('/api/services/${id}/results/export?format=csv','_blank')" class="btn btn-ghost btn-sm">${svgIcon('download')}<span>CSV</span></button>
        <button onclick="window.open('/api/services/${id}/results/export?format=json','_blank')" class="btn btn-ghost btn-sm">${svgIcon('download')}<span>JSON</span></button>
        <button onclick="window.open('/api/services/${id}/report','_blank')" class="btn btn-ghost btn-sm">${svgIcon('report')}<span>Report</span></button>
        <button onclick="handleShareResult('${id}')" class="btn btn-ghost btn-sm">${svgIcon('share')}<span>Share</span></button>
    </div>
    ${results.run_config ? renderRunConfigDetail(results.run_config) : ''}

    ${sectionHeader('Latency & responses', 'measured in this run')}
    ${renderMetricsBlock(results)}

    ${timelineCharts ? sectionHeader('During this run', 'per-second metrics') + timelineCharts : ''}

    ${(renderTLSCard(results) || renderRateLimitCard(results.rate_limit) || renderTimingCard(results) || renderCircuitCard(results.circuit_events)) ? sectionHeader('Reliability & connection') : ''}
    ${renderTLSCard(results)}
    ${renderRateLimitCard(results.rate_limit)}
    ${renderTimingCard(results)}
    ${renderCircuitCard(results.circuit_events)}
    ` :
    emptyState('No test results', 'Run a test to see results here.', null, null)}

    ${history.length > 0 ? renderTestHistory(id, history, compareSection) + renderTrendSection(history) + renderAnomalyAlert(history) : ''}

    <!-- PROJECTIONS: clearly-labeled estimates, kept at the very bottom -->
    ${results && capacityHtml ? `
    ${sectionHeader('Projections', 'model-based estimates — not measurements')}
    <div class="grid grid-cols-1 gap-6">
        ${capacityHtml}
    </div>` : ''}

    <!-- SETUP: configuration reference, collapsed to keep results front-and-center -->
    ${sectionHeader('Test setup')}
    <div class="card p-5 mb-6">
        <div class="flex items-center gap-2 mb-4">
            <h3 class="text-sm font-semibold flex-1" style="color:var(--text);">Configuration</h3>
            ${panelChevron('setup')}
        </div>
        <div id="panel-body-setup" style="display:${isPanelCollapsed('setup') ? 'none' : ''};">
            <div class="grid grid-cols-2 md:grid-cols-3 gap-4 text-sm">
                ${configItem('Concurrency', svc.concurrency)}
                ${configItem('Duration', svc.duration)}
                ${configItem('Timeout', svc.timeout)}
                ${svc.arrival_rate > 0 ? configItem('Arrival Rate', svc.arrival_rate + ' /sec') : ''}
                ${svc.think_time_ms > 0 ? configItem('Think Time', svc.think_time_ms + 'ms' + (svc.think_time_max_ms > 0 ? ' - ' + svc.think_time_max_ms + 'ms' : '')) : ''}
                ${svc.warmup_seconds > 0 ? configItem('Warm-up', svc.warmup_seconds + 's') : ''}
                ${svc.http2 === 0 ? configItem('HTTP/2', 'Disabled') : ''}
                ${svc.dns_cache ? configItem('DNS Cache', 'Enabled') : ''}
                ${svc.disable_keep_alive ? configItem('Keep-Alive', 'Disabled') : ''}
                ${svc.protocol && svc.protocol !== 'http' ? configItem('Protocol', svc.protocol.toUpperCase()) : ''}
                ${svc.content_type && svc.content_type !== 'json' ? configItem('Content Type', svc.content_type) : ''}
                ${svc.data_source && svc.data_source !== '[]' ? configItem('Data Source', 'Configured') : ''}
                ${svc.form_fields && svc.form_fields !== '[]' ? configItem('Form Fields', 'Configured') : ''}
            </div>
            ${hdrs.length > 0 ? `
            <div class="mt-4 pt-4 border-t border-slate-700/30">
                <div class="text-xs font-medium mb-2" style="color:var(--text-muted);">Headers</div>
                ${hdrs.map(([k,v], i) => `<div class="text-xs font-mono mb-1"><span style="color:var(--accent-text);">${esc(k)}</span><span style="color:var(--text-faint);">:</span> ${headerValueHtml(k, v, i)}</div>`).join('')}
            </div>` : ''}
            ${svcSteps.length > 0 ? `<div class="mt-4 pt-4 border-t border-slate-700/30">${renderChainVisualization(svcSteps)}</div>` : ''}
        </div>
    </div>

    <div id="compare-view"></div>`;
}

// ---- Assertion Results ----

function renderAssertionResults(results) {
    if (!results) return '';
    const assertionResults = parseJSON(results.assertion_results, []);
    if (assertionResults.length === 0) return '';

    const status = results.status || (assertionResults.every(a => a.passed) ? 'pass' : 'fail');
    const isPass = status === 'pass';
    const badgeColor = isPass ? '#10b981' : '#ef4444';
    const badgeLabel = isPass ? 'PASS' : 'FAIL';

    const unitForMetric = (metric) => {
        if (metric === 'rps') return '/s';
        if (metric === 'error_rate') return '%';
        return 'ms';
    };

    const operatorSymbol = (op) => {
        const map = { gt: '>', lt: '<', gte: '>=', lte: '<=', eq: '=' };
        return map[op] || op;
    };

    const metricLabel = (metric) => {
        const map = {
            rps: 'RPS', avg_latency: 'Avg Latency', p95_latency: 'P95 Latency',
            p99_latency: 'P99 Latency', min_latency: 'Min Latency', max_latency: 'Max Latency',
            error_rate: 'Error Rate',
        };
        return map[metric] || metric;
    };

    const rows = assertionResults.map(a => {
        const unit = unitForMetric(a.metric);
        const passed = a.passed;
        const icon = passed
            ? '<span style="color:#10b981;font-weight:bold;">&#10003;</span>'
            : '<span style="color:#ef4444;font-weight:bold;">&#10007;</span>';
        const color = passed ? '#10b981' : '#ef4444';
        const actualFmt = unit === 'ms' ? fmtLatency(a.actual) : (unit === '%' ? fmtDec(a.actual) + '%' : fmtDec(a.actual) + unit);
        const valueFmt = unit === 'ms' ? (a.value + 'ms') : (unit === '%' ? a.value + '%' : a.value + unit);
        return `<div class="flex items-center gap-2 py-1.5">
            ${icon}
            <span class="text-sm" style="color:${color};">${esc(metricLabel(a.metric))} ${operatorSymbol(a.operator)} ${valueFmt}: <span class="font-mono font-semibold">${actualFmt}</span></span>
        </div>`;
    }).join('');

    return `
    <div class="card p-5 mb-6">
        <div class="flex items-center gap-3 mb-3">
            <h3 class="text-sm font-semibold" style="color:var(--text);">Assertion Results</h3>
            <span class="px-2.5 py-0.5 rounded-full text-xs font-bold" style="background:${badgeColor}20;color:${badgeColor};">${badgeLabel}</span>
        </div>
        ${rows}
    </div>`;
}

function renderAssertionBadge(h) {
    const assertionResults = parseJSON(h.assertion_results, []);
    if (assertionResults.length === 0 && !h.status) return '<span style="color:var(--text-subtle);">-</span>';
    const status = h.status || (assertionResults.every(a => a.passed) ? 'pass' : 'fail');
    const isPass = status === 'pass';
    const color = isPass ? '#10b981' : '#ef4444';
    const label = isPass ? 'PASS' : 'FAIL';
    return `<span class="px-2 py-0.5 rounded-full text-[10px] font-bold" style="background:${color}20;color:${color};">${label}</span>`;
}

// ---- Profile Dropdown ----

function toggleProfileDropdown() {
    const dd = document.getElementById('profile-dropdown');
    if (dd) dd.style.display = dd.style.display === 'none' ? 'block' : 'none';
}

// Close profile dropdown on click outside
document.addEventListener('click', (e) => {
    const wrap = document.getElementById('profile-dropdown-wrap');
    if (wrap && !wrap.contains(e.target)) {
        const dd = document.getElementById('profile-dropdown');
        if (dd) dd.style.display = 'none';
    }
    // Close workspace dropdown when clicking outside
    const wsSelector = document.getElementById('workspace-selector');
    if (wsSelector && !wsSelector.contains(e.target)) {
        const wsDd = document.getElementById('ws-dropdown');
        if (wsDd) wsDd.style.display = 'none';
    }
});

// ---- Timeline Charts ----

function renderTimelineCharts(timeline) {
    if (!timeline || timeline.length === 0) return '';
    const chartData = timeline.map(p => ({t: p.t / 1e9, rps: p.rps || 0, lat: p.lat_ms || 0, errs: p.errs || 0}));

    return `
    <div class="grid grid-cols-2 gap-4 mt-6">
        <div class="card p-5">
            <h3 class="text-sm font-semibold mb-3" style="color:var(--text);">RPS Over Time</h3>
            ${renderSVGLineChart(chartData, 'rps', '#06b6d4', 'RPS', '/s')}
        </div>
        <div class="card p-5">
            <h3 class="text-sm font-semibold mb-3" style="color:var(--text);">Latency Over Time</h3>
            ${renderSVGLineChart(chartData, 'lat', '#7c3aed', 'Latency', 'ms')}
        </div>
    </div>`;
}

// ---- Test History ----

function renderPagination(currentPage, totalPages, onClickFn) {
    if (totalPages <= 1) return '';

    let pages = '';
    // Show limited page numbers for large page counts
    const maxVisible = 7;
    let start = 1, end = totalPages;
    if (totalPages > maxVisible) {
        start = Math.max(1, currentPage - 3);
        end = Math.min(totalPages, start + maxVisible - 1);
        if (end - start < maxVisible - 1) start = Math.max(1, end - maxVisible + 1);
    }
    if (start > 1) pages += `<button onclick="${onClickFn}(1)" class="pagination-btn px-3 py-1 rounded-lg text-xs font-medium transition-colors hover:bg-slate-700/30" style="color:var(--text-muted);">1</button>`;
    if (start > 2) pages += `<span class="px-1 text-xs" style="color:var(--text-subtle);">...</span>`;
    for (let i = start; i <= end; i++) {
        const isActive = i === currentPage;
        pages += `<button onclick="${onClickFn}(${i})" class="pagination-btn px-3 py-1 rounded-lg text-xs font-medium transition-colors ${isActive ? '' : 'hover:bg-slate-700/30'}" style="${isActive ? 'background:rgba(124,58,237,0.2);color:var(--accent-text);' : 'color:var(--text-muted);'}">${i}</button>`;
    }
    if (end < totalPages - 1) pages += `<span class="px-1 text-xs" style="color:var(--text-subtle);">...</span>`;
    if (end < totalPages) pages += `<button onclick="${onClickFn}(${totalPages})" class="pagination-btn px-3 py-1 rounded-lg text-xs font-medium transition-colors hover:bg-slate-700/30" style="color:var(--text-muted);">${totalPages}</button>`;

    return `<div class="flex items-center justify-center gap-1 mt-6">
        <button onclick="${onClickFn}(${Math.max(1, currentPage-1)})" class="pagination-btn px-2 py-1 rounded-lg text-xs" style="color:var(--text-muted);" ${currentPage===1?'disabled':''}>&laquo;</button>
        ${pages}
        <button onclick="${onClickFn}(${Math.min(totalPages, currentPage+1)})" class="pagination-btn px-2 py-1 rounded-lg text-xs" style="color:var(--text-muted);" ${currentPage===totalPages?'disabled':''}>&raquo;</button>
    </div>`;
}

function setDashboardPage(p) { state.dashboardPage = p; renderDashboardContent(); }
function setHistoryPage(p) { state.historyPage = p; router(); }

function sortableHeader(label, column, align) {
    const isActive = state.historySort.column === column;
    const arrow = isActive ? (state.historySort.direction === 'asc' ? ' &#9650;' : ' &#9660;') : '';
    const color = isActive ? 'var(--accent-text)' : 'var(--text-muted)';
    const textAlign = align || 'left';
    return `<th class="text-${textAlign} py-3 px-4 text-xs font-medium sortable-th" style="color:${color};"
                onclick="handleHistorySort('${column}')">${label}${arrow}</th>`;
}

function handleHistorySort(column) {
    if (state.historySort.column === column) {
        state.historySort.direction = state.historySort.direction === 'asc' ? 'desc' : 'asc';
    } else {
        state.historySort.column = column;
        state.historySort.direction = 'desc';
    }
    state.historyPage = 1;
    router();
}

function renderTestHistory(serviceId, history, compareBtn) {
    // Sort history
    const sorted = [...history].sort((a, b) => {
        let va, vb;
        switch(state.historySort.column) {
            case 'created_at': va = a.created_at || ''; vb = b.created_at || ''; break;
            case 'total_reqs': va = a.total_reqs || 0; vb = b.total_reqs || 0; break;
            case 'rps': va = a.rps || 0; vb = b.rps || 0; break;
            case 'avg_latency_ms': va = a.avg_latency_ms || 0; vb = b.avg_latency_ms || 0; break;
            case 'error_rate':
                va = a.total_reqs > 0 ? a.errors/a.total_reqs : 0;
                vb = b.total_reqs > 0 ? b.errors/b.total_reqs : 0;
                break;
            case 'status': va = a.status || ''; vb = b.status || ''; break;
            default: va = 0; vb = 0;
        }
        if (va < vb) return state.historySort.direction === 'asc' ? -1 : 1;
        if (va > vb) return state.historySort.direction === 'asc' ? 1 : -1;
        return 0;
    });

    // Pagination
    const totalHistoryPages = Math.ceil(sorted.length / HISTORY_PER_PAGE);
    if (state.historyPage > totalHistoryPages && totalHistoryPages > 0) state.historyPage = totalHistoryPages;
    const pagedHistory = sorted.slice((state.historyPage - 1) * HISTORY_PER_PAGE, state.historyPage * HISTORY_PER_PAGE);
    const historyPaginationHtml = renderPagination(state.historyPage, totalHistoryPages, 'setHistoryPage');

    return `
    <div class="mt-8">
        <div class="flex items-center justify-between mb-4">
            <h3 class="text-lg font-semibold" style="color:var(--text);">Test History</h3>
            ${compareBtn || ''}
        </div>
        <div class="card overflow-hidden">
            <table class="w-full text-sm">
                <thead>
                    <tr class="border-b border-slate-700/50" style="background:var(--bg);">
                        ${state.compareMode ? '<th class="py-3 px-3 w-8"></th>' : ''}
                        ${sortableHeader('Date', 'created_at', 'left')}
                        ${sortableHeader('Requests', 'total_reqs', 'right')}
                        ${sortableHeader('RPS', 'rps', 'right')}
                        ${sortableHeader('Avg Latency', 'avg_latency_ms', 'right')}
                        ${sortableHeader('Error Rate', 'error_rate', 'right')}
                        ${sortableHeader('Status', 'status', 'center')}
                        <th class="text-right py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">Note</th>
                        <th class="py-3 px-2 text-xs font-medium text-center" style="color:var(--text-muted);"></th>
                    </tr>
                </thead>
                <tbody>${pagedHistory.map(h => renderHistoryRow(serviceId, h)).join('')}</tbody>
            </table>
        </div>
        ${historyPaginationHtml}
        ${state.compareMode ? `<div class="mt-3 text-xs" style="color:var(--text-muted);">Select exactly 2 results to compare. <span id="compare-count">${state.compareSelected.length}/2 selected</span></div>` : ''}
    </div>`;
}

function renderHistoryRow(serviceId, h) {
    const errRate = h.total_reqs > 0 ? (h.errors / h.total_reqs * 100) : 0;
    const errColor = errorRateColor(errRate);
    const rowId = 'history-detail-' + h.id;
    const isSelected = state.compareSelected.includes(h.id);
    const colSpan = state.compareMode ? 10 : 9;

    const checkboxCol = state.compareMode ? `<td class="py-3 px-3">
        <input type="checkbox" ${isSelected ? 'checked' : ''} onchange="handleCompareCheck('${serviceId}','${h.id}',this.checked)"
               style="accent-color:#7c3aed;cursor:pointer;" ${!isSelected && state.compareSelected.length >= 2 ? 'disabled' : ''}>
    </td>` : '';

    const noteDisplay = h.note
        ? `<span class="text-[11px] cursor-pointer" style="color:var(--accent-text);" onclick="event.stopPropagation();showNoteInput('${serviceId}','${h.id}',this)" title="Click to edit">${esc(h.note)}</span>`
        : `<span class="text-[11px] cursor-pointer opacity-50 hover:opacity-100" style="color:var(--text-subtle);" onclick="event.stopPropagation();showNoteInput('${serviceId}','${h.id}',this)">Add note</span>`;

    // Run config badge
    let runBadge = '';
    const rcParsed = typeof h.run_config === 'string' ? parseJSON(h.run_config, null) : h.run_config;
    if (rcParsed && rcParsed.type) {
        const rc = rcParsed;
        if (rc.type === 'profile' && rc.profile_name) {
            runBadge = `<span class="ml-1.5 inline-block px-1.5 py-0.5 rounded text-[10px] font-semibold" style="background:rgba(124,58,237,0.15);color:var(--accent-text);">${esc(rc.profile_name)}</span>`;
        } else if (rc.type === 'pattern' && rc.pattern_name) {
            const shortName = rc.pattern_name.split(' ')[0];
            runBadge = `<span class="ml-1.5 inline-block px-1.5 py-0.5 rounded text-[10px] font-semibold" style="background:rgba(6,182,212,0.15);color:#22d3ee;">${esc(shortName)}</span>`;
        } else if (rc.type === 'queue') {
            runBadge = `<span class="ml-1.5 inline-block px-1.5 py-0.5 rounded text-[10px] font-semibold" style="background:rgba(100,116,139,0.15);color:var(--text-muted);">Queue</span>`;
        } else if (rc.type === 'scheduled') {
            runBadge = `<span class="ml-1.5 inline-block px-1.5 py-0.5 rounded text-[10px] font-semibold" style="background:rgba(100,116,139,0.15);color:var(--text-muted);">Scheduled</span>`;
        }
    }

    return `
        <tr class="border-b border-slate-700/30 cursor-pointer hover-row" onclick="toggleHistoryDetail('${rowId}')">
            ${checkboxCol}
            <td class="py-3 px-4 font-mono text-xs" style="color:var(--text);">${fmtDate(h.created_at)}${runBadge}</td>
            <td class="py-3 px-4 text-right font-mono text-xs" style="color:var(--text);">${fmt(h.total_reqs)}</td>
            <td class="py-3 px-4 text-right font-mono text-xs" style="color:#06b6d4;">${fmtDec(h.rps)}</td>
            <td class="py-3 px-4 text-right font-mono text-xs" style="color:var(--text);">${fmtLatency(h.avg_latency_ms)}</td>
            <td class="py-3 px-4 text-right font-mono text-xs font-medium" style="color:${errColor};">${fmtDec(errRate)}%</td>
            <td class="py-3 px-4 text-center">${renderAssertionBadge(h)}</td>
            <td class="py-3 px-4 text-right" id="note-cell-${h.id}">${noteDisplay}</td>
            <td class="py-1 px-2 text-center" style="white-space:nowrap;">
                <button onclick="event.stopPropagation();handleShareResult('${serviceId}','${h.id}')" class="btn btn-icon btn-ghost btn-sm" style="padding:0.25rem;" title="Share result">${svgIcon('share')}</button>
                <button onclick="event.stopPropagation();handleDeleteResult('${serviceId}','${h.id}')" class="btn btn-icon btn-ghost btn-sm" style="padding:0.25rem;color:#ef4444;" title="Delete this result">${svgIcon('trash')}</button>
            </td>
        </tr>
        <tr id="${rowId}" style="display:none;">
            <td colspan="${colSpan}" class="px-4 py-4" style="background:var(--bg);">
                ${renderRunConfigDetail(h.run_config)}
                <div class="grid grid-cols-4 gap-4 text-xs">
                    <div><span style="color:var(--text-subtle);">Duration</span><div class="font-mono font-medium mt-0.5" style="color:var(--text);">${fmtDuration(h.duration_ms)}</div></div>
                    <div><span style="color:var(--text-subtle);">P50</span><div class="font-mono font-medium mt-0.5" style="color:var(--text);">${fmtLatency(h.p50_latency_ms)}</div></div>
                    <div><span style="color:var(--text-subtle);">P95</span><div class="font-mono font-medium mt-0.5" style="color:var(--text);">${fmtLatency(h.p95_latency_ms)}</div></div>
                    <div><span style="color:var(--text-subtle);">P99</span><div class="font-mono font-medium mt-0.5" style="color:var(--text);">${fmtLatency(h.p99_latency_ms)}</div></div>
                    <div><span style="color:var(--text-subtle);">Min Latency</span><div class="font-mono font-medium mt-0.5" style="color:var(--text);">${fmtLatency(h.min_latency_ms)}</div></div>
                    <div><span style="color:var(--text-subtle);">Max Latency</span><div class="font-mono font-medium mt-0.5" style="color:var(--text);">${fmtLatency(h.max_latency_ms)}</div></div>
                    <div><span style="color:var(--text-subtle);">Total Errors</span><div class="font-mono font-medium mt-0.5" style="color:${errColor};">${fmt(h.errors)}</div></div>
                    <div><span style="color:var(--text-subtle);">Status Codes</span><div class="font-mono font-medium mt-0.5" style="color:var(--text);">${renderStatusCodesInline(h.status_codes)}</div></div>
                </div>
            </td>
        </tr>`;
}

function renderRunConfigDetail(rc) {
    if (typeof rc === 'string') { try { rc = JSON.parse(rc); } catch(_) { return ''; } }
    if (!rc || !rc.type) return '';
    const parts = [];
    if (rc.type === 'pattern' && rc.pattern_name) parts.push('Pattern "' + rc.pattern_name + '"');
    else if (rc.type === 'profile' && rc.profile_name) parts.push('Profile "' + rc.profile_name + '"');
    else if (rc.type === 'queue') parts.push('Queue');
    else if (rc.type === 'scheduled') parts.push('Scheduled');
    else if (rc.type === 'manual') parts.push('Manual');
    else parts.push(rc.type);
    if (rc.concurrency) parts.push(rc.concurrency + ' workers');
    if (rc.duration) parts.push(rc.duration);
    if (rc.rps) parts.push('RPS: ' + rc.rps);
    else parts.push('RPS: unlimited');
    if (rc.arrival_rate) parts.push('Arrival: ' + rc.arrival_rate + '/s');
    if (rc.think_time_ms) parts.push('Think: ' + rc.think_time_ms + 'ms');
    return `<div class="mb-3 text-xs font-mono px-2 py-1.5 rounded" style="background:rgba(71,85,105,0.15);color:var(--text-muted);">Run Config: ${esc(parts.join(' | '))}</div>`;
}

function renderRunConfigBadge(rc) {
    if (typeof rc === 'string') { try { rc = JSON.parse(rc); } catch(_) { return ''; } }
    if (!rc || !rc.type || rc.type === 'manual') return '';
    if (rc.type === 'profile' && rc.profile_name) {
        return `<span class="ml-2 inline-block px-2 py-0.5 rounded text-xs font-semibold" style="background:rgba(124,58,237,0.15);color:var(--accent-text);">${esc(rc.profile_name)}</span>`;
    }
    if (rc.type === 'pattern' && rc.pattern_name) {
        return `<span class="ml-2 inline-block px-2 py-0.5 rounded text-xs font-semibold" style="background:rgba(6,182,212,0.15);color:#22d3ee;">${esc(rc.pattern_name)}</span>`;
    }
    if (rc.type === 'queue') {
        return `<span class="ml-2 inline-block px-2 py-0.5 rounded text-xs font-semibold" style="background:rgba(100,116,139,0.15);color:var(--text-muted);">Queue</span>`;
    }
    if (rc.type === 'scheduled') {
        return `<span class="ml-2 inline-block px-2 py-0.5 rounded text-xs font-semibold" style="background:rgba(100,116,139,0.15);color:var(--text-muted);">Scheduled</span>`;
    }
    return '';
}

function renderStatusCodesInline(codes) {
    if (!codes) return '-';
    return Object.entries(codes).map(([code, count]) =>
        `<span style="color:${statusCodeColor(code)};">${statusCodeLabel(code)}</span>:${fmt(count)}`
    ).join('  ');
}

function toggleHistoryDetail(rowId) {
    const row = document.getElementById(rowId);
    if (!row) return;
    row.style.display = row.style.display === 'none' ? 'table-row' : 'none';
}

function configItem(label, value) {
    return `<div><div class="text-xs mb-0.5" style="color:var(--text-subtle);">${label}</div><div class="font-mono font-medium" style="color:var(--text);">${esc(String(value))}</div></div>`;
}

function notFound() {
    return `<div class="text-center py-20">
        <h2 class="text-xl font-semibold mb-2" style="color:var(--text);">Service not found</h2>
        <a href="#/" class="text-sm" style="color:#7c3aed;">Back to Dashboard</a>
    </div>`;
}

// ---- Notes ----

function showNoteInput(serviceId, resultId, el) {
    const cell = document.getElementById('note-cell-' + resultId);
    if (!cell) return;
    const current = el.textContent === 'Add note' ? '' : el.textContent;
    cell.innerHTML = `<input type="text" class="input-dark input-sm" style="width:150px;font-size:0.7rem;" value="${esc(current)}"
        onkeydown="if(event.key==='Enter')saveNote('${serviceId}','${resultId}',this.value);if(event.key==='Escape')router();"
        onblur="saveNote('${serviceId}','${resultId}',this.value)">`;
    cell.querySelector('input').focus();
}

async function saveNote(serviceId, resultId, note) {
    try {
        await api(`/api/services/${serviceId}/results/${resultId}/note`, {
            method: 'PUT', body: JSON.stringify({ note: note.trim() })
        });
    } catch (err) { toast('Failed to save note: ' + err.message, 'error'); }
    router();
}

// ---- Compare ----

function toggleCompareMode(serviceId) {
    state.compareMode = !state.compareMode;
    state.compareSelected = [];
    document.getElementById('compare-view').innerHTML = '';
    router();
}

function handleCompareCheck(serviceId, resultId, checked) {
    resultId = String(resultId);
    if (checked) {
        if (state.compareSelected.length < 2 && !state.compareSelected.includes(resultId)) {
            state.compareSelected.push(resultId);
        }
    } else {
        state.compareSelected = state.compareSelected.filter(id => id !== resultId);
    }
    const countEl = document.getElementById('compare-count');
    if (countEl) countEl.textContent = `${state.compareSelected.length}/2 selected`;

    // Update checkbox states without full re-render
    document.querySelectorAll('input[onchange^="handleCompareCheck"]').forEach(cb => {
        const cbResultId = cb.getAttribute('onchange').match(/'(\d+)'/g)?.[1]?.replace(/'/g, '');
        if (cbResultId) {
            cb.checked = state.compareSelected.includes(cbResultId);
            cb.disabled = !cb.checked && state.compareSelected.length >= 2;
        }
    });

    if (state.compareSelected.length === 2) {
        showComparison(serviceId);
    } else {
        const cv = document.getElementById('compare-view');
        if (cv) cv.innerHTML = '';
    }
}

async function showComparison(serviceId) {
    let history = [];
    try { history = (await api(`/api/services/${serviceId}/history`)) || []; } catch (_) {}

    const a = history.find(h => String(h.id) == state.compareSelected[0]);
    const b = history.find(h => String(h.id) == state.compareSelected[1]);
    if (!a || !b) return;

    const cv = document.getElementById('compare-view');
    if (!cv) return;

    function deltaCell(valA, valB, higherIsBetter) {
        if (valA == null || valB == null) return `<span style="color:var(--text-subtle);">-</span>`;
        const diff = valB - valA;
        const pct = valA !== 0 ? ((diff / valA) * 100) : 0;
        const improved = higherIsBetter ? diff > 0 : diff < 0;
        const color = Math.abs(pct) < 1 ? 'var(--text-muted)' : improved ? '#10b981' : '#ef4444';
        const arrow = diff > 0 ? '+' : '';
        return `<span style="color:${color};font-weight:600;">${arrow}${fmtDec(pct)}%</span>`;
    }

    const errRateA = a.total_reqs > 0 ? (a.errors / a.total_reqs * 100) : 0;
    const errRateB = b.total_reqs > 0 ? (b.errors / b.total_reqs * 100) : 0;

    const rows = [
        ['RPS', fmtDec(a.rps), fmtDec(b.rps), deltaCell(a.rps, b.rps, true)],
        ['Total Reqs', fmt(a.total_reqs), fmt(b.total_reqs), deltaCell(a.total_reqs, b.total_reqs, true)],
        ['Avg Latency', fmtLatency(a.avg_latency_ms), fmtLatency(b.avg_latency_ms), deltaCell(a.avg_latency_ms, b.avg_latency_ms, false)],
        ['P50 Latency', fmtLatency(a.p50_latency_ms), fmtLatency(b.p50_latency_ms), deltaCell(a.p50_latency_ms, b.p50_latency_ms, false)],
        ['P95 Latency', fmtLatency(a.p95_latency_ms), fmtLatency(b.p95_latency_ms), deltaCell(a.p95_latency_ms, b.p95_latency_ms, false)],
        ['P99 Latency', fmtLatency(a.p99_latency_ms), fmtLatency(b.p99_latency_ms), deltaCell(a.p99_latency_ms, b.p99_latency_ms, false)],
        ['Min Latency', fmtLatency(a.min_latency_ms), fmtLatency(b.min_latency_ms), deltaCell(a.min_latency_ms, b.min_latency_ms, false)],
        ['Max Latency', fmtLatency(a.max_latency_ms), fmtLatency(b.max_latency_ms), deltaCell(a.max_latency_ms, b.max_latency_ms, false)],
        ['Error Rate', fmtDec(errRateA) + '%', fmtDec(errRateB) + '%', deltaCell(errRateA, errRateB, false)],
    ];

    const compareIds = state.compareSelected;

    cv.innerHTML = `
    <div class="mt-8">
        <div class="flex items-center justify-between mb-4">
            <h3 class="text-lg font-semibold" style="color:var(--text);">Comparison</h3>
            <button onclick="window.open('/api/compare-report?ids=${compareIds.join(',')}','_blank')" class="btn btn-primary btn-sm">
                ${svgIcon('report')} <span>Download Comparative Report</span>
            </button>
        </div>
        <div class="card overflow-hidden">
            <table class="w-full text-sm">
                <thead>
                    <tr class="border-b border-slate-700/50" style="background:var(--bg);">
                        <th class="text-left py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">Metric</th>
                        <th class="text-right py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">${fmtDate(a.created_at)}</th>
                        <th class="text-right py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">${fmtDate(b.created_at)}</th>
                        <th class="text-right py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">Change</th>
                    </tr>
                </thead>
                <tbody>${rows.map(([metric, va, vb, delta]) => `
                    <tr class="border-b border-slate-700/30">
                        <td class="py-2.5 px-4 text-xs font-medium" style="color:var(--text-muted);">${metric}</td>
                        <td class="py-2.5 px-4 text-right font-mono text-xs" style="color:var(--text);">${va}</td>
                        <td class="py-2.5 px-4 text-right font-mono text-xs" style="color:var(--text);">${vb}</td>
                        <td class="py-2.5 px-4 text-right font-mono text-xs">${delta}</td>
                    </tr>`).join('')}
                </tbody>
            </table>
        </div>
    </div>`;
}

// ---- Service Form ----

// Fills the service form's workspace <select> with the current workspaces and
// selects the right one (the service's workspace when editing, or the active
// workspace when creating). Called after the form is mounted.
async function populateFormWorkspaces() {
    const sel = document.getElementById('form-workspace');
    if (!sel) return;
    let workspaces = state.workspaces;
    if (!workspaces || !workspaces.length) {
        try { workspaces = (await api('/api/workspaces')) || []; state.workspaces = workspaces; } catch (_) { workspaces = []; }
    }
    const want = String(sel.dataset.selected || '');
    sel.innerHTML = workspaces.map(w =>
        `<option value="${w.id}"${String(w.id) === want ? ' selected' : ''}>${esc(w.name)}</option>`
    ).join('') || '<option value="">Default</option>';
    // If nothing matched (e.g. creating with no active workspace), fall back to
    // the default workspace so the shown value matches what will be saved.
    if (!workspaces.some(w => String(w.id) === want) && state.defaultWorkspaceId != null) {
        sel.value = String(state.defaultWorkspaceId);
    }
}

function renderForm(svc) {
    const isEdit = !!svc;
    const defaults = state.settings || {};
    const s = svc || {
        name:'', url:'', method:'GET', headers:{}, body:'',
        concurrency: parseInt(defaults.default_concurrency) || 10,
        duration: defaults.default_duration || '10s',
        timeout: defaults.default_timeout || '30s',
        tags:[], group_name:'', cookie_jar: 0,
        protocol: state.pendingProtocol || 'http'
    };
    const headerKeys = Object.keys(s.headers || {});
    const showBody = ['POST','PUT','PATCH'].includes(s.method);

    // Parse assertions, profiles, steps, validations, and form fields
    const assertions = parseJSON(s.assertions, []);
    const profiles = parseJSON(s.profiles, []);
    const steps = parseJSON(s.steps, []);
    const displayValidations = parseJSON(s.validations, []);
    const displayFormFields = parseJSON(s.form_fields, []);

    // Default profiles for new service
    const defaultProfiles = [
        { name: 'Light', concurrency: 5, duration: '10s', rps: 10 },
        { name: 'Medium', concurrency: 50, duration: '30s', rps: 100 },
        { name: 'Heavy', concurrency: 200, duration: '60s', rps: 0 },
    ];
    const displayProfiles = profiles.length > 0 ? profiles : (isEdit ? [] : defaultProfiles);

    return `
    ${breadcrumb({ label: 'Dashboard', href: '#/' }, { label: isEdit ? 'Edit Service' : 'New Service' })}

    <div class="max-w-2xl">
        <h2 class="text-2xl font-bold mb-1" style="color:var(--text);">${isEdit ? 'Edit Service' : 'Create New Service'}</h2>
        <p class="text-sm mb-6" style="color:var(--text-muted);">Configure your HTTP endpoint and load test parameters.</p>

        ${!isEdit ? `
        <!-- cURL Import -->
        <div class="card p-5 mb-6" style="border-color:rgba(6,182,212,0.3);">
            <div class="flex items-center gap-2 mb-3">
                <svg class="w-4 h-4" style="color:#06b6d4;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>
                <h3 class="text-sm font-semibold" style="color:#06b6d4;">Import from cURL</h3>
            </div>
            <textarea id="curl-input" class="input-dark font-mono text-xs" rows="4" placeholder="Paste a cURL command here...&#10;&#10;Example: curl 'https://api.example.com/endpoint' -H 'Content-Type: application/json' --data-raw '{&quot;key&quot;:&quot;value&quot;}'"></textarea>
            <div class="flex items-center gap-2 mt-3">
                <button onclick="handleCurlImport()" class="btn btn-sm" style="background:rgba(6,182,212,0.15);color:#06b6d4;border:1px solid rgba(6,182,212,0.3);">
                    ${svgIcon('play')} <span>Parse & Fill</span>
                </button>
                <span class="text-[11px]" style="color:var(--text-faint);">Supports cURL commands copied from browser DevTools</span>
            </div>
        </div>` : ''}

        ${!isEdit ? `
        <div class="flex items-center gap-3 mb-4">
            <button onclick="showTemplates()" class="btn btn-ghost btn-sm">
                ${svgIcon('bolt')} <span>Use Template</span>
            </button>
            <span class="text-xs" style="color:var(--text-faint);">Start from a predefined configuration</span>
        </div>
        <div id="templates-panel" style="display:none;"></div>
        ` : ''}

        <div class="card p-6" id="service-form" data-id="${isEdit ? svc.id : ''}">
            <p class="text-[11px] mb-3" style="color:var(--text-faint);">Only <span style="color:var(--text-muted);font-weight:600;">Request</span> and <span style="color:var(--text-muted);font-weight:600;">Load</span> are required — the other tabs are optional. A dot marks tabs you've configured.</p>
            <!-- Tab Navigation -->
            <div class="flex border-b border-slate-700/40 mb-2 flex-wrap">
                ${['basic','load','validation','advanced','scenarios'].map(tab => {
                    const hasContent = {
                        validation: assertions.length > 0 || displayValidations.length > 0,
                        advanced: (s.protocol && s.protocol !== 'http') || (s.content_type && s.content_type !== 'json') || (s.data_source && s.data_source !== '[]') || (s.form_fields && s.form_fields !== '[]'),
                        scenarios: steps.length > 0,
                    }[tab];
                    const dot = hasContent ? '<span style="display:inline-block;width:5px;height:5px;border-radius:50%;background:var(--accent-text);margin-left:5px;vertical-align:middle;" title="Configured"></span>' : '';
                    return `<button onclick="switchFormTab('${tab}')" class="form-tab-btn px-4 py-2 text-xs font-medium border-b-2 transition-colors" style="color:${state.formTab === tab ? 'var(--accent-text)' : 'var(--text-subtle)'};border-color:${state.formTab === tab ? '#7c3aed' : 'transparent'};background:none;cursor:pointer;">
                        ${FORM_TABS[tab].label}${dot}
                    </button>`;
                }).join('')}
            </div>
            <p id="tab-desc" class="text-[11px] mb-5" style="color:var(--text-subtle);">${FORM_TABS[state.formTab] ? FORM_TABS[state.formTab].desc : ''}</p>

            <!-- Tab: Basic -->
            <div id="tab-basic" style="display:${state.formTab==='basic'?'block':'none'};">
            <!-- Name -->
            <div class="mb-5">
                <div class="flex items-center justify-between mb-1.5">
                    <label class="block text-xs font-medium" style="color:var(--text-muted);">Service Name <span style="color:#ef4444;">*</span></label>
                    <span id="name-counter" class="text-[10px] font-mono" style="color:var(--text-subtle);">${(s.name || '').length}/60</span>
                </div>
                <input type="text" name="name" class="input-dark" placeholder="e.g. User API, Payment Service" value="${esc(s.name)}" maxlength="60" oninput="document.getElementById('name-counter').textContent = this.value.length + '/60';">
            </div>

            <!-- URL -->
            <div class="mb-5">
                <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">URL <span style="color:#ef4444;">*</span></label>
                <input type="text" name="url" class="input-dark font-mono" placeholder="https://api.example.com/endpoint" value="${esc(s.url)}" maxlength="2048">
            </div>

            <!-- Method -->
            <div class="mb-5">
                <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">HTTP Method</label>
                <select name="method" class="input-dark" onchange="toggleBody(this.value)">
                    ${['GET','POST','PUT','DELETE','PATCH'].map(m => `<option value="${m}" ${s.method===m?'selected':''}>${m}</option>`).join('')}
                </select>
            </div>

            <!-- Tags & Group -->
            <div class="grid grid-cols-2 gap-4 mb-5">
                <div>
                    <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Tags <span class="text-[10px]" style="color:var(--text-faint);">(comma-separated)</span></label>
                    <input type="text" name="tags" class="input-dark input-sm" placeholder="api, production, v2" value="${esc(parseTags(s.tags).join(', '))}">
                </div>
                <div>
                    <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Group</label>
                    <input type="text" name="group_name" class="input-dark input-sm" placeholder="e.g. Backend, Frontend" value="${esc(s.group_name || '')}">
                </div>
            </div>

            <!-- Workspace -->
            <div class="mb-5">
                <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Workspace</label>
                <select name="workspace_id" id="form-workspace" class="input-dark input-sm" data-selected="${isEdit ? (s.workspace_id || '') : (localStorage.getItem('gload_workspace') || '')}">
                    <option value="">Default</option>
                </select>
                <div class="text-[11px] mt-1" style="color:var(--text-faint);">Which workspace this service belongs to.</div>
            </div>

            <!-- Headers -->
            <div class="mb-5">
                <div class="flex items-center justify-between mb-1.5">
                    <label class="text-xs font-medium" style="color:var(--text-muted);">Headers</label>
                    <button onclick="addHeaderRow()" class="btn btn-ghost btn-sm" style="padding:0.3rem 0.6rem;font-size:0.75rem;">
                        ${svgIcon('plus')} <span>Add</span>
                    </button>
                </div>
                <div id="headers-container" class="space-y-2">
                    ${headerKeys.length > 0 ? headerKeys.map((k, i) => headerRow(k, s.headers[k])).join('') :
                    `<p class="text-xs py-2" style="color:var(--text-faint);">No headers configured.</p>`}
                </div>
            </div>

            <!-- Body -->
            <div id="body-field" class="mb-5" style="display:${showBody?'block':'none'};">
                <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Request Body</label>
                <textarea name="body" class="input-dark" rows="4" maxlength="1000000" placeholder='{"key": "value"}'>${esc(s.body)}</textarea>
            </div>
            </div>

            <!-- Tab: Load -->
            <div id="tab-load" style="display:${state.formTab==='load'?'block':'none'};">
            <div class="mb-5">

                <!-- Mode Selector -->
                <div class="flex items-center gap-2 mb-5">
                    <span class="text-xs font-medium" style="color:var(--text-muted);">Mode:</span>
                    ${['simple','realistic','expert'].map(m => `
                        <button type="button" onclick="switchLoadMode('${m}')" class="px-3 py-1.5 rounded-lg text-xs font-medium transition-colors" id="load-mode-${m}"
                            style="${(state.loadMode||'simple')===m ? 'background:rgba(124,58,237,0.15);color:var(--accent-text);border:1px solid rgba(124,58,237,0.3);' : 'color:var(--text-subtle);border:1px solid rgba(71,85,105,0.3);'}">
                            ${{simple:'Simple',realistic:'Realistic',expert:'Expert'}[m]}
                        </button>`).join('')}
                    <span class="text-[10px] ml-2" style="color:var(--text-faint);" id="load-mode-hint">
                        ${{simple:'Basic concurrency test — just set workers and duration.',realistic:'Simulates real users with arrival rate and think time.',expert:'Full control over all parameters.'}[state.loadMode||'simple']}
                    </span>
                </div>

                <!-- Core: Always visible -->
                <div class="grid grid-cols-3 gap-4 mb-4">
                    <div id="concurrency-wrap">
                        <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Concurrent users <span class="text-[10px]" style="color:var(--text-faint);">(concurrency)</span> <span style="color:#ef4444;">*</span></label>
                        <input type="number" name="concurrency" class="input-dark input-sm" min="1" max="10000" step="1" value="${parseInt(s.concurrency) || 10}">
                        <div class="text-[11px] mt-1" style="color:var(--text-faint);">How many run at the same time</div>
                    </div>
                    <div>
                        <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Duration (seconds) <span style="color:#ef4444;">*</span></label>
                        <input type="number" name="duration" class="input-dark input-sm" min="1" max="3600" step="1" value="${parseDurationToSeconds(s.duration)}">
                        <div class="text-[11px] mt-1" style="color:var(--text-faint);">How long to run the test</div>
                    </div>
                    <div>
                        <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Timeout (seconds) <span style="color:#ef4444;">*</span></label>
                        <input type="number" name="timeout" class="input-dark input-sm" min="1" max="300" step="1" value="${parseDurationToSeconds(s.timeout)}">
                        <div class="text-[11px] mt-1" style="color:var(--text-faint);">Max wait per request</div>
                    </div>
                </div>

                <!-- Realistic Mode: Arrival Rate + Think Time -->
                <div id="load-realistic" style="display:none;">
                    <div class="p-3 mb-4 rounded-lg" style="background:rgba(6,182,212,0.05);border:1px solid rgba(6,182,212,0.15);">
                        <div class="text-[11px] mb-2" style="color:#06b6d4;">Realistic mode uses arrival rate instead of fixed concurrency. New virtual users arrive at a steady rate, each sends one request then leaves — like real traffic.</div>
                    </div>
                    <div class="grid grid-cols-3 gap-4 mb-4">
                        <div>
                            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Users per second <span class="text-[10px]" style="color:var(--text-faint);">(arrival rate)</span> <span style="color:#ef4444;">*</span></label>
                            <input type="number" name="arrival_rate" class="input-dark input-sm" min="1" max="100000" step="1" value="${parseInt(s.arrival_rate) || 100}">
                            <div class="text-[11px] mt-1" style="color:var(--text-faint);">New users spawned per second</div>
                        </div>
                        <div>
                            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Pause between requests <span class="text-[10px]" style="color:var(--text-faint);">(think time, ms)</span></label>
                            <input type="number" name="think_time_ms" class="input-dark input-sm" min="0" max="60000" step="100" value="${parseInt(s.think_time_ms) || 500}" onchange="toggleThinkMax()">
                            <div class="text-[11px] mt-1" style="color:var(--text-faint);">Pause between requests (simulates reading)</div>
                        </div>
                        <div id="think-max-wrap" style="display:${(parseInt(s.think_time_ms)||0) > 0 ? 'block' : 'none'};">
                            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Max pause <span class="text-[10px]" style="color:var(--text-faint);">(think time max, ms)</span></label>
                            <input type="number" name="think_time_max_ms" class="input-dark input-sm" min="0" max="60000" step="100" value="${parseInt(s.think_time_max_ms) || 2000}">
                            <div class="text-[11px] mt-1" style="color:var(--text-faint);">Random delay up to this value</div>
                        </div>
                    </div>
                    <div class="grid grid-cols-3 gap-4 mb-4">
                        <div>
                            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Warm-up (seconds)</label>
                            <input type="number" name="warmup_seconds" class="input-dark input-sm" min="0" max="300" step="1" value="${parseInt(s.warmup_seconds) || 5}">
                            <div class="text-[11px] mt-1" style="color:var(--text-faint);">Ignore first N seconds of results</div>
                        </div>
                    </div>
                </div>

                <!-- Expert Mode: Everything -->
                <div id="load-expert" style="display:none;">
                    <div class="p-3 mb-4 rounded-lg" style="background:rgba(245,158,11,0.05);border:1px solid rgba(245,158,11,0.15);">
                        <div class="text-[11px]" style="color:#f59e0b;">Expert mode gives full control. Leave fields at 0 or default to disable them.</div>
                    </div>

                    <h4 class="text-xs font-semibold mb-3" style="color:var(--text-muted);">Traffic Shaping</h4>
                    <div class="grid grid-cols-3 gap-4 mb-4">
                        <div>
                            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Users per second <span class="text-[10px]" style="color:var(--text-faint);">(arrival rate)</span></label>
                            <input type="number" name="arrival_rate" class="input-dark input-sm" min="0" max="100000" step="1" value="${parseInt(s.arrival_rate) || 0}" id="expert-arrival-rate">
                            <div class="text-[11px] mt-1" style="color:var(--text-faint);">0 = use concurrency mode</div>
                        </div>
                        <div>
                            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Pause between requests <span class="text-[10px]" style="color:var(--text-faint);">(think time, ms)</span></label>
                            <input type="number" name="think_time_ms" class="input-dark input-sm" min="0" max="60000" step="100" value="${parseInt(s.think_time_ms) || 0}" onchange="toggleThinkMax()" id="expert-think">
                            <div class="text-[11px] mt-1" style="color:var(--text-faint);">0 = no delay</div>
                        </div>
                        <div id="expert-think-max-wrap" style="display:${(parseInt(s.think_time_ms)||0) > 0 ? 'block' : 'none'};">
                            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Max pause <span class="text-[10px]" style="color:var(--text-faint);">(think time max, ms)</span></label>
                            <input type="number" name="think_time_max_ms" class="input-dark input-sm" min="0" max="60000" step="100" value="${parseInt(s.think_time_max_ms) || 0}">
                        </div>
                    </div>

                    <h4 class="text-xs font-semibold mb-3" style="color:var(--text-muted);">Warm-up & Batching</h4>
                    <div class="grid grid-cols-3 gap-4 mb-4">
                        <div>
                            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Warm-up (seconds)</label>
                            <input type="number" name="warmup_seconds" class="input-dark input-sm" min="0" max="300" step="1" value="${parseInt(s.warmup_seconds) || 0}">
                        </div>
                        <div>
                            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Pre-opened connections <span class="text-[10px]" style="color:var(--text-faint);">(warm-up conns)</span></label>
                            <input type="number" name="warmup_conns" class="input-dark input-sm" min="0" max="1000" step="1" value="${parseInt(s.warmup_conns) || 0}">
                        </div>
                        <div>
                            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Requests per user loop <span class="text-[10px]" style="color:var(--text-faint);">(batching)</span></label>
                            <input type="number" name="requests_per_iteration" class="input-dark input-sm" min="1" max="100" step="1" value="${parseInt(s.requests_per_iteration) || 1}">
                        </div>
                    </div>

                    <h4 class="text-xs font-semibold mb-3" style="color:var(--text-muted);">Auto-scaling <span class="text-[10px]" style="color:var(--text-faint);">(adaptive concurrency)</span></h4>
                    <div class="grid grid-cols-3 gap-4 mb-4">
                        <div class="flex items-center gap-3">
                            <label class="toggle-switch toggle-sm">
                                <input type="checkbox" name="adaptive_concurrency" ${s.adaptive_concurrency ? 'checked' : ''} onchange="toggleAdaptiveTarget()">
                                <span class="toggle-slider"></span>
                            </label>
                            <span class="text-xs" style="color:var(--text-muted);">Enable adaptive scaling</span>
                        </div>
                        <div id="adaptive-target-wrap" style="display:${s.adaptive_concurrency ? 'block' : 'none'};">
                            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Target P95 latency <span class="text-[10px]" style="color:var(--text-faint);">(ms)</span></label>
                            <input type="number" name="adaptive_target_ms" class="input-dark input-sm" min="1" max="60000" step="10" value="${parseFloat(s.adaptive_target_ms) || 500}">
                        </div>
                    </div>

                    <h4 class="text-xs font-semibold mb-3" style="color:var(--text-muted);">Connection</h4>
                    <div class="grid grid-cols-4 gap-4">
                        <div>
                            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Idle connection pool <span class="text-[10px]" style="color:var(--text-faint);">(max idle conns)</span></label>
                            <input type="number" name="max_idle_conns" class="input-dark input-sm" min="0" max="10000" step="1" value="${parseInt(s.max_idle_conns) || 100}">
                        </div>
                        <div class="flex items-center gap-3 pt-5">
                            <label class="toggle-switch toggle-sm">
                                <input type="checkbox" name="http2" ${(s.http2 === undefined || s.http2 === 1 || s.http2 === true) ? 'checked' : ''}>
                                <span class="toggle-slider"></span>
                            </label>
                            <span class="text-xs" style="color:var(--text-muted);">HTTP/2</span>
                        </div>
                        <div class="flex items-center gap-3 pt-5">
                            <label class="toggle-switch toggle-sm">
                                <input type="checkbox" name="dns_cache" ${s.dns_cache ? 'checked' : ''}>
                                <span class="toggle-slider"></span>
                            </label>
                            <span class="text-xs" style="color:var(--text-muted);">DNS Cache</span>
                        </div>
                        <div class="flex items-center gap-3 pt-5">
                            <label class="toggle-switch toggle-sm">
                                <input type="checkbox" name="disable_keep_alive" ${s.disable_keep_alive ? 'checked' : ''}>
                                <span class="toggle-slider"></span>
                            </label>
                            <span class="text-xs" style="color:var(--text-muted);">Disable Keep-Alive</span>
                        </div>
                    </div>
                </div>

            </div>

                <!-- Test Profiles (reusable load presets) -->
                <div class="border-t border-slate-700/40 pt-5 mt-4">
                    <div class="flex items-center justify-between mb-4">
                        <div>
                            <h3 class="text-sm font-semibold" style="color:var(--text);">Load presets</h3>
                            <p class="text-[11px] mt-0.5" style="color:var(--text-faint);">Saved concurrency/duration combos you can launch with one click from the service page (e.g. Light, Medium, Heavy).</p>
                        </div>
                        <button onclick="addProfileRow()" class="btn btn-ghost btn-sm" style="padding:0.3rem 0.6rem;font-size:0.75rem;">
                            ${svgIcon('plus')} <span>Add preset</span>
                        </button>
                    </div>
                    <div id="profiles-container" class="space-y-2">
                        ${displayProfiles.length > 0 ? displayProfiles.map(p => profileRow(p.name, p.concurrency, parseDurationToSeconds(p.duration), p.rps)).join('') :
                        `<p class="text-xs py-2" style="color:var(--text-faint);">No presets configured.</p>`}
                    </div>
                </div>
            </div>

            <!-- Tab: Validation -->
            <div id="tab-validation" style="display:${state.formTab==='validation'?'block':'none'};">
            <!-- Assertions -->
            <div class="mb-5">
                <div class="flex items-center justify-between mb-4">
                    <div>
                        <h3 class="text-sm font-semibold" style="color:var(--text);">Pass/fail thresholds</h3>
                        <p class="text-[11px] mt-0.5" style="color:var(--text-faint);">Fail the whole test if a metric crosses a limit. Example: P95 latency &lt; 500ms, error rate &lt; 1%.</p>
                    </div>
                    <button onclick="addAssertionRow()" class="btn btn-ghost btn-sm" style="padding:0.3rem 0.6rem;font-size:0.75rem;">
                        ${svgIcon('plus')} <span>Add threshold</span>
                    </button>
                </div>
                <div id="assertions-container" class="space-y-2">
                    ${assertions.length > 0 ? assertions.map(a => assertionRow(a.metric, a.operator, a.value)).join('') :
                    `<p class="text-xs py-2" style="color:var(--text-faint);">No assertions configured.</p>`}
                </div>
            </div>

            <!-- Response Validations -->
            <div class="border-t border-slate-700/40 pt-5 mt-2 mb-5">
                <div class="flex items-center justify-between mb-3">
                    <div>
                        <h3 class="text-sm font-semibold" style="color:var(--text);">Response checks</h3>
                        <p class="text-[11px] mt-0.5" style="color:var(--text-faint);">Verify each individual response. Example: status is 200, body contains "success".</p>
                    </div>
                    <button onclick="addValidationRow()" class="btn btn-ghost btn-sm" style="padding:0.3rem 0.6rem;font-size:0.75rem;">
                        + Add check
                    </button>
                </div>
                <div id="validations-container" class="space-y-2">
                    ${displayValidations.map(v => validationRow(v.type, v.value, v.path)).join('')}
                    ${displayValidations.length === 0 ? '<p class="text-xs py-2" style="color:var(--text-faint);">No validations configured.</p>' : ''}
                </div>
            </div>
            </div>

            <!-- Tab: Advanced -->
            <div id="tab-advanced" style="display:${state.formTab==='advanced'?'block':'none'};">
                <h4 class="text-xs font-semibold mb-3" style="color:var(--text-muted);">Protocol & Data</h4>
                <div class="grid grid-cols-3 gap-4 mb-4">
                    <div>
                        <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Protocol</label>
                        <select name="protocol" class="input-dark input-sm" onchange="document.getElementById('protocol-config-section').style.display = this.value === 'http' ? 'none' : 'block'">
                            <option value="http" ${(s.protocol||'http')==='http'?'selected':''}>HTTP</option>
                            <option value="graphql" ${s.protocol==='graphql'?'selected':''}>GraphQL</option>
                            <option value="websocket" ${s.protocol==='websocket'?'selected':''}>WebSocket</option>
                            <option value="grpc" ${s.protocol==='grpc'?'selected':''}>gRPC</option>
                            <option value="tcp" ${s.protocol==='tcp'?'selected':''}>TCP</option>
                        </select>
                    </div>
                    <div>
                        <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Content Type</label>
                        <select name="content_type" class="input-dark input-sm" onchange="document.getElementById('form-fields-section').style.display = this.value === 'multipart' ? 'block' : 'none'">
                            <option value="json" ${(s.content_type||'json')==='json'?'selected':''}>JSON</option>
                            <option value="form" ${s.content_type==='form'?'selected':''}>Form URL-Encoded</option>
                            <option value="multipart" ${s.content_type==='multipart'?'selected':''}>Multipart Form</option>
                            <option value="text" ${s.content_type==='text'?'selected':''}>Plain Text</option>
                        </select>
                    </div>
                </div>

                <div id="protocol-config-section" style="display:${(s.protocol && s.protocol !== 'http') ? 'block' : 'none'};" class="mb-4">
                    <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Protocol Config (JSON)</label>
                    <textarea name="protocol_config" class="input-dark font-mono text-xs" rows="3" placeholder='{"address":"localhost:50051","tls":"true"}'>${esc(typeof s.protocol_config === 'string' ? s.protocol_config : JSON.stringify(s.protocol_config || {}))}</textarea>
                    <div class="text-[11px] mt-1" style="color:var(--text-faint);">Plugin-specific key-value config as JSON</div>
                </div>

                <div class="mb-4">
                    <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Dynamic Data Source (JSON array)</label>
                    <textarea name="data_source" class="input-dark font-mono text-xs" rows="3" placeholder='[{"id":"1","name":"Alice"},{"id":"2","name":"Bob"}]'>${esc(s.data_source || '')}</textarea>
                    <div class="text-[11px] mt-1" style="color:var(--text-faint);">Round-robin data for {{variable}} placeholders. Also supports {{gen.uuid}}, {{gen.email}}, etc.</div>
                </div>

                <div id="form-fields-section" style="display:${s.content_type === 'multipart' ? 'block' : 'none'};" class="mb-4">
                    <div class="flex items-center justify-between mb-1.5">
                        <label class="text-xs font-medium" style="color:var(--text-muted);">Form Fields</label>
                        <button onclick="addFormFieldRow()" class="btn btn-ghost btn-sm" style="padding:0.3rem 0.6rem;font-size:0.75rem;">+ Add Field</button>
                    </div>
                    <div id="form-fields-container" class="space-y-2">
                        ${displayFormFields.length > 0 ? displayFormFields.map(f => formFieldRow(f.name, f.value, f.is_file, f.filename)).join('') : '<p class="text-xs py-2" style="color:var(--text-faint);">No form fields configured.</p>'}
                    </div>
                </div>

                <!-- Cookie/Session Toggle -->
                <div class="border-t border-slate-700/40 pt-5 mt-2 mb-5">
                    <div class="flex items-center justify-between">
                        <div>
                            <h3 class="text-sm font-semibold" style="color:var(--text);">Cookie/Session Persistence</h3>
                            <p class="text-[11px] mt-0.5" style="color:var(--text-faint);">Each virtual user maintains its own cookie jar across requests.</p>
                        </div>
                        <label class="toggle-switch">
                            <input type="checkbox" name="cookie_jar" ${s.cookie_jar ? 'checked' : ''}>
                            <span class="toggle-slider"></span>
                        </label>
                    </div>
                </div>

            </div>

            <!-- Tab: Scenarios -->
            <div id="tab-scenarios" style="display:${state.formTab==='scenarios'?'block':'none'};">
            <!-- Scenario Steps -->
            <div class="mb-5">
                <div class="flex items-center justify-between mb-4">
                    <div>
                        <h3 class="text-sm font-semibold" style="color:#06b6d4;">Scenario Steps</h3>
                        <p class="text-[11px] mt-0.5" style="color:var(--text-faint);">Define a multi-step request chain. Each step runs sequentially; extracted values carry forward.</p>
                    </div>
                    <button onclick="addStep()" class="btn btn-ghost btn-sm" style="padding:0.3rem 0.6rem;font-size:0.75rem;">
                        ${svgIcon('plus')} <span>Add Step</span>
                    </button>
                </div>
                <div id="steps-container">
                    ${steps.length > 0 ? steps.map((st, i) => stepCard(st, i, true)).join('') :
                    `<div class="steps-empty text-xs py-2" style="color:var(--text-faint);">No steps configured. Each step runs sequentially; values extracted from one step can be used in subsequent steps.</div>`}
                </div>
            </div>
            </div>

            <!-- Actions -->
            <div class="flex items-center gap-3 pt-2 border-t border-slate-700/40">
                <button onclick="handleSaveService()" id="save-btn" class="btn btn-primary">
                    <span class="btn-label">${isEdit ? 'Save Changes' : 'Create Service'}</span>
                    <span class="spinner"></span>
                </button>
                <a href="#/${isEdit ? 'services/' + svc.id : ''}" class="btn btn-ghost">Cancel</a>
            </div>
        </div>
    </div>`;
}

function headerRow(key, val) {
    return `<div class="flex gap-2 items-center header-row">
        <input type="text" class="input-dark input-sm flex-1 header-key" placeholder="Header name" value="${esc(key)}">
        <input type="text" class="input-dark input-sm flex-1 header-val" placeholder="Value" value="${esc(val)}">
        <button onclick="this.closest('.header-row').remove()" class="btn btn-icon btn-ghost btn-sm" style="padding:0.3rem;">${svgIcon('minus')}</button>
    </div>`;
}

function addHeaderRow() {
    const c = document.getElementById('headers-container');
    const p = c.querySelector('p');
    if (p) p.remove();
    c.insertAdjacentHTML('beforeend', headerRow('', ''));
    c.querySelector('.header-row:last-child .header-key').focus();
}

// ---- Form Field Row Helpers ----

function formFieldRow(name, value, isFile, filename) {
    return `<div class="flex gap-2 items-center form-field-row">
        <input type="text" class="input-dark input-sm ff-name" placeholder="Field name" value="${esc(name || '')}" style="width:120px;">
        <input type="text" class="input-dark input-sm ff-value flex-1" placeholder="Value or {{gen.uuid}}" value="${esc(value || '')}">
        <label class="flex items-center gap-1 text-[10px]" style="color:var(--text-muted);white-space:nowrap;">
            <input type="checkbox" class="ff-isfile" ${isFile ? 'checked' : ''}> File
        </label>
        <input type="text" class="input-dark input-sm ff-filename" placeholder="filename.ext" value="${esc(filename || '')}" style="width:100px;">
        <button onclick="this.closest('.form-field-row').remove()" class="btn btn-icon btn-ghost btn-sm" style="padding:0.3rem;">${svgIcon('minus')}</button>
    </div>`;
}

function addFormFieldRow() {
    const c = document.getElementById('form-fields-container');
    const p = c.querySelector('p'); if (p) p.remove();
    c.insertAdjacentHTML('beforeend', formFieldRow('', '', false, ''));
}

function collectFormFields() {
    const fields = [];
    document.querySelectorAll('.form-field-row').forEach(row => {
        const name = row.querySelector('.ff-name')?.value?.trim();
        const value = row.querySelector('.ff-value')?.value || '';
        const isFile = row.querySelector('.ff-isfile')?.checked || false;
        const filename = row.querySelector('.ff-filename')?.value?.trim() || '';
        if (name) fields.push({ name, value, is_file: isFile, filename });
    });
    return fields;
}

// ---- Assertion Row Helpers ----

const ASSERTION_METRICS = [
    { value: 'rps', label: 'RPS', defaultOp: 'gt', unit: '/s' },
    { value: 'avg_latency', label: 'Avg Latency', defaultOp: 'lt', unit: 'ms' },
    { value: 'p95_latency', label: 'P95 Latency', defaultOp: 'lt', unit: 'ms' },
    { value: 'p99_latency', label: 'P99 Latency', defaultOp: 'lt', unit: 'ms' },
    { value: 'error_rate', label: 'Error Rate', defaultOp: 'lt', unit: '%' },
    { value: 'max_latency', label: 'Max Latency', defaultOp: 'lt', unit: 'ms' },
    { value: 'min_latency', label: 'Min Latency', defaultOp: 'lt', unit: 'ms' },
];

const ASSERTION_OPERATORS = [
    { value: 'gt', label: '>' },
    { value: 'lt', label: '<' },
    { value: 'gte', label: '>=' },
    { value: 'lte', label: '<=' },
    { value: 'eq', label: '=' },
];

function assertionRow(metric, operator, value) {
    const metricInfo = ASSERTION_METRICS.find(m => m.value === metric) || ASSERTION_METRICS[0];
    return `<div class="flex gap-2 items-center assertion-row">
        <select class="input-dark input-sm assertion-metric" style="width:140px;" onchange="updateAssertionOp(this)">
            ${ASSERTION_METRICS.map(m => `<option value="${m.value}" ${metric===m.value?'selected':''}>${m.label}</option>`).join('')}
        </select>
        <select class="input-dark input-sm assertion-operator" style="width:70px;">
            ${ASSERTION_OPERATORS.map(o => `<option value="${o.value}" ${operator===o.value?'selected':''}>${o.label}</option>`).join('')}
        </select>
        <div class="flex items-center gap-1 flex-1">
            <input type="number" class="input-dark input-sm assertion-value flex-1" placeholder="Value" value="${value || ''}" min="0" step="any">
            <span class="text-[11px] font-mono" style="color:var(--text-subtle);min-width:24px;">${metricInfo.unit}</span>
        </div>
        <button onclick="removeAssertionRow(this)" class="btn btn-icon btn-ghost btn-sm" style="padding:0.3rem;">${svgIcon('minus')}</button>
    </div>`;
}

function addAssertionRow() {
    const c = document.getElementById('assertions-container');
    const p = c.querySelector('p');
    if (p) p.remove();
    c.insertAdjacentHTML('beforeend', assertionRow('p95_latency', 'lt', ''));
}

function removeAssertionRow(btn) {
    const row = btn.closest('.assertion-row');
    row.remove();
    const c = document.getElementById('assertions-container');
    if (c && c.children.length === 0) {
        c.innerHTML = '<p class="text-xs py-2" style="color:var(--text-faint);">No assertions configured.</p>';
    }
}

function updateAssertionOp(select) {
    const row = select.closest('.assertion-row');
    const metricVal = select.value;
    const metricInfo = ASSERTION_METRICS.find(m => m.value === metricVal);
    if (metricInfo) {
        const opSelect = row.querySelector('.assertion-operator');
        opSelect.value = metricInfo.defaultOp;
        const unitSpan = row.querySelector('.text-\\[11px\\]');
        if (unitSpan) unitSpan.textContent = metricInfo.unit;
    }
}

// ---- Validation Row Helpers ----

function validationRow(type, value, path) {
    return `<div class="flex gap-2 items-center validation-row">
        <select class="input-dark input-sm validation-type" style="width:140px;" onchange="toggleValidationPath(this)">
            <option value="status_code" ${type==='status_code'?'selected':''}>Status Code</option>
            <option value="contains" ${type==='contains'?'selected':''}>Body Contains</option>
            <option value="not_contains" ${type==='not_contains'?'selected':''}>Body Not Contains</option>
            <option value="json_path" ${type==='json_path'?'selected':''}>JSON Path</option>
            <option value="regex" ${type==='regex'?'selected':''}>Regex Match</option>
        </select>
        <input type="text" class="input-dark input-sm validation-path flex-1" placeholder="Path (for json_path)" value="${esc(path || '')}" style="display:${type==='json_path'?'block':'none'};">
        <input type="text" class="input-dark input-sm validation-value flex-1" placeholder="Expected value or pattern" value="${esc(value || '')}">
        <button onclick="this.closest('.validation-row').remove()" class="btn btn-icon btn-ghost btn-sm" style="padding:0.3rem;">${svgIcon('minus')}</button>
    </div>`;
}

function addValidationRow() {
    const c = document.getElementById('validations-container');
    const p = c.querySelector('p'); if (p) p.remove();
    c.insertAdjacentHTML('beforeend', validationRow('contains', '', ''));
}

function toggleValidationPath(select) {
    const row = select.closest('.validation-row');
    const pathInput = row.querySelector('.validation-path');
    pathInput.style.display = select.value === 'json_path' ? 'block' : 'none';
}

// ---- Profile Row Helpers ----

function profileRow(name, concurrency, durationSec, rps) {
    return `<div class="flex gap-2 items-center profile-row">
        <input type="text" class="input-dark input-sm profile-name" placeholder="Profile name" value="${esc(name || '')}" style="width:120px;" maxlength="50">
        <div class="flex items-center gap-1">
            <input type="number" class="input-dark input-sm profile-concurrency" placeholder="Conc." value="${parseInt(concurrency) || ''}" min="1" max="10000" step="1" style="width:80px;">
            <span class="text-[10px]" style="color:var(--text-subtle);">workers</span>
        </div>
        <div class="flex items-center gap-1">
            <input type="number" class="input-dark input-sm profile-duration" placeholder="Dur." value="${parseInt(durationSec) || ''}" min="1" max="3600" step="1" style="width:80px;">
            <span class="text-[10px]" style="color:var(--text-subtle);">sec</span>
        </div>
        <div class="flex items-center gap-1">
            <input type="number" class="input-dark input-sm profile-rps" placeholder="RPS" value="${parseInt(rps) || 0}" min="0" max="100000" step="1" style="width:80px;">
            <span class="text-[10px]" style="color:var(--text-subtle);">/s</span>
        </div>
        <button onclick="removeProfileRow(this)" class="btn btn-icon btn-ghost btn-sm" style="padding:0.3rem;">${svgIcon('minus')}</button>
    </div>`;
}

function addProfileRow() {
    const c = document.getElementById('profiles-container');
    const p = c.querySelector('p');
    if (p) p.remove();
    c.insertAdjacentHTML('beforeend', profileRow('', 10, 10, 0));
}

function removeProfileRow(btn) {
    const row = btn.closest('.profile-row');
    row.remove();
    const c = document.getElementById('profiles-container');
    if (c && c.children.length === 0) {
        c.innerHTML = '<p class="text-xs py-2" style="color:var(--text-faint);">No test profiles configured.</p>';
    }
}

function toggleBody(method) {
    const el = document.getElementById('body-field');
    if (el) el.style.display = ['POST','PUT','PATCH'].includes(method) ? 'block' : 'none';
}

// ---- Steps / Scenario Helpers ----

function extractorRow(name, source, path) {
    return `<div class="flex gap-2 items-center extractor-row">
        <input type="text" class="input-dark input-sm extractor-name flex-1" placeholder="Variable name" value="${esc(name || '')}">
        <select class="input-dark input-sm extractor-source" style="width:100px;">
            <option value="body" ${source==='body'?'selected':''}>body</option>
            <option value="header" ${source==='header'?'selected':''}>header</option>
            <option value="cookie" ${source==='cookie'?'selected':''}>cookie</option>
        </select>
        <input type="text" class="input-dark input-sm extractor-path flex-1" placeholder="e.g. data.access_token" value="${esc(path || '')}">
        <button onclick="this.closest('.extractor-row').remove()" class="btn btn-icon btn-ghost btn-sm" style="padding:0.3rem;">${svgIcon('minus')}</button>
    </div>`;
}

function stepCard(step, index, collapsed) {
    const s = step || { name: '', url: '', method: 'GET', headers: {}, body: '', extractors: [], weight: 0 };
    const extractors = s.extractors || [];
    const hdrs = Object.entries(s.headers || {});
    const showBody = ['POST','PUT','PATCH'].includes(s.method);
    const isCollapsed = collapsed !== false;
    const weight = s.weight || 0;

    return `<div class="card p-4 mb-3 step-card" data-step-index="${index}" style="border-color:rgba(6,182,212,0.2);">
        <div class="flex items-center justify-between cursor-pointer" onclick="toggleStepCollapse(this)">
            <div class="flex items-center gap-2">
                <span class="text-[10px] font-mono font-bold px-1.5 py-0.5 rounded" style="background:rgba(6,182,212,0.15);color:#06b6d4;">Step ${index + 1}</span>
                <span class="text-sm font-medium step-card-title" style="color:var(--text);">${esc(s.name || 'Untitled Step')}</span>
                <span class="text-[10px] font-mono" style="color:var(--text-subtle);">${s.method} ${esc(s.url || '')}</span>
            </div>
            <div class="flex items-center gap-2">
                <button onclick="event.stopPropagation();removeStep(this)" class="btn btn-icon btn-ghost btn-sm" style="padding:0.3rem;">${svgIcon('minus')}</button>
                <svg class="w-4 h-4 step-collapse-icon" style="color:var(--text-subtle);transition:transform .2s;${isCollapsed ? '' : 'transform:rotate(180deg);'}" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/></svg>
            </div>
        </div>
        <div class="step-body mt-4" style="display:${isCollapsed ? 'none' : 'block'};">
            <div class="grid grid-cols-2 gap-3 mb-3">
                <div>
                    <label class="block text-[11px] font-medium mb-1" style="color:var(--text-muted);">Step Name</label>
                    <input type="text" class="input-dark input-sm step-name" placeholder="e.g. Login" value="${esc(s.name || '')}" oninput="this.closest('.step-card').querySelector('.step-card-title').textContent=this.value||'Untitled Step'">
                </div>
                <div class="flex gap-3">
                    <div class="flex-1">
                        <label class="block text-[11px] font-medium mb-1" style="color:var(--text-muted);">Method</label>
                        <select class="input-dark input-sm step-method" onchange="toggleStepBody(this)">
                            ${['GET','POST','PUT','DELETE','PATCH'].map(m => `<option value="${m}" ${s.method===m?'selected':''}>${m}</option>`).join('')}
                        </select>
                    </div>
                    <div>
                        <label class="block text-[11px] font-medium mb-1" style="color:var(--text-muted);">Weight</label>
                        <div class="flex items-center gap-1" style="width:80px;">
                            <input type="number" class="input-dark input-sm step-weight" placeholder="Weight" value="${weight || 0}" min="0" max="100" step="1" style="width:70px;">
                            <span class="text-[10px]" style="color:var(--text-subtle);">wt</span>
                        </div>
                    </div>
                </div>
            </div>
            <div class="mb-3">
                <label class="block text-[11px] font-medium mb-1" style="color:var(--text-muted);">URL</label>
                <input type="text" class="input-dark input-sm step-url font-mono" placeholder="https://api.example.com/endpoint" value="${esc(s.url || '')}">
            </div>
            <div class="mb-3">
                <div class="flex items-center justify-between mb-1">
                    <label class="text-[11px] font-medium" style="color:var(--text-muted);">Headers</label>
                    <button onclick="addStepHeaderRow(this)" class="btn btn-ghost btn-sm" style="padding:0.2rem 0.4rem;font-size:0.7rem;">${svgIcon('plus')} <span>Add</span></button>
                </div>
                <div class="step-headers-container space-y-1">
                    ${hdrs.length > 0 ? hdrs.map(([k,v]) => stepHeaderRow(k, v)).join('') : `<p class="text-[11px] py-1" style="color:var(--text-faint);">No headers.</p>`}
                </div>
            </div>
            <div class="mb-3 step-body-field" style="display:${showBody?'block':'none'};">
                <label class="block text-[11px] font-medium mb-1" style="color:var(--text-muted);">Body</label>
                <textarea class="input-dark input-sm step-body-input font-mono" rows="2" placeholder='{"key": "value"}'>${esc(s.body || '')}</textarea>
            </div>
            <!-- Extractors -->
            <div class="border-t border-slate-700/30 pt-3 mt-3">
                <div class="flex items-center justify-between mb-2">
                    <div>
                        <label class="text-[11px] font-medium" style="color:var(--accent-text);">Extractors</label>
                        <span class="text-[10px] ml-1" style="color:var(--text-faint);">Extract values for use in subsequent steps</span>
                    </div>
                    <button onclick="addExtractorRow(this)" class="btn btn-ghost btn-sm" style="padding:0.2rem 0.4rem;font-size:0.7rem;">${svgIcon('plus')} <span>Add</span></button>
                </div>
                <div class="step-extractors-container space-y-1">
                    ${extractors.length > 0 ? extractors.map(e => extractorRow(e.name, e.source, e.path)).join('') : `<p class="text-[10px] py-1" style="color:var(--text-faint);">No extractors. Extracted values can be used in subsequent steps as {{variable_name}}</p>`}
                </div>
            </div>
        </div>
    </div>`;
}

function stepHeaderRow(key, val) {
    return `<div class="flex gap-2 items-center step-header-row">
        <input type="text" class="input-dark input-sm flex-1 step-header-key" placeholder="Header name" value="${esc(key)}">
        <input type="text" class="input-dark input-sm flex-1 step-header-val" placeholder="Value" value="${esc(val)}">
        <button onclick="this.closest('.step-header-row').remove()" class="btn btn-icon btn-ghost btn-sm" style="padding:0.2rem;">${svgIcon('minus')}</button>
    </div>`;
}

function addStepHeaderRow(btn) {
    const card = btn.closest('.step-card');
    const c = card.querySelector('.step-headers-container');
    const p = c.querySelector('p');
    if (p) p.remove();
    c.insertAdjacentHTML('beforeend', stepHeaderRow('', ''));
}

function toggleStepBody(select) {
    const card = select.closest('.step-card');
    const bf = card.querySelector('.step-body-field');
    if (bf) bf.style.display = ['POST','PUT','PATCH'].includes(select.value) ? 'block' : 'none';
}

function toggleStepCollapse(header) {
    const card = header.closest('.step-card');
    const body = card.querySelector('.step-body');
    const icon = card.querySelector('.step-collapse-icon');
    if (body.style.display === 'none') {
        body.style.display = 'block';
        if (icon) icon.style.transform = 'rotate(180deg)';
    } else {
        body.style.display = 'none';
        if (icon) icon.style.transform = '';
    }
}

function addExtractorRow(btn) {
    const card = btn.closest('.step-card');
    const c = card.querySelector('.step-extractors-container');
    const p = c.querySelector('p');
    if (p) p.remove();
    c.insertAdjacentHTML('beforeend', extractorRow('', 'body', ''));
}

function addStep() {
    const c = document.getElementById('steps-container');
    const p = c.querySelector('.steps-empty');
    if (p) p.remove();
    const index = c.querySelectorAll('.step-card').length;
    c.insertAdjacentHTML('beforeend', stepCard({}, index, false));
    renumberSteps();
}

function removeStep(btn) {
    const card = btn.closest('.step-card');
    card.remove();
    const c = document.getElementById('steps-container');
    if (c && c.querySelectorAll('.step-card').length === 0) {
        c.innerHTML = '<div class="steps-empty text-xs py-2" style="color:var(--text-faint);">No steps configured. Each step runs sequentially; values extracted from one step can be used in subsequent steps.</div>';
    }
    renumberSteps();
}

function renumberSteps() {
    const c = document.getElementById('steps-container');
    if (!c) return;
    c.querySelectorAll('.step-card').forEach((card, i) => {
        card.dataset.stepIndex = i;
        const badge = card.querySelector('.text-\\[10px\\].font-mono.font-bold');
        if (badge) badge.textContent = 'Step ' + (i + 1);
    });
}

function collectSteps() {
    const steps = [];
    document.querySelectorAll('#steps-container .step-card').forEach(card => {
        const headers = {};
        card.querySelectorAll('.step-header-row').forEach(row => {
            const k = row.querySelector('.step-header-key')?.value?.trim();
            const v = row.querySelector('.step-header-val')?.value?.trim() || '';
            if (k) headers[k] = v;
        });
        const extractors = [];
        card.querySelectorAll('.extractor-row').forEach(row => {
            const name = row.querySelector('.extractor-name')?.value?.trim();
            const source = row.querySelector('.extractor-source')?.value;
            const path = row.querySelector('.extractor-path')?.value?.trim();
            if (name && path) extractors.push({ name, source, path });
        });
        const stepName = card.querySelector('.step-name')?.value?.trim() || '';
        const url = card.querySelector('.step-url')?.value?.trim() || '';
        const method = card.querySelector('.step-method')?.value || 'GET';
        const body = card.querySelector('.step-body-input')?.value || '';
        const weight = parseInt(card.querySelector('.step-weight')?.value, 10) || 0;
        if (url) {
            steps.push({ name: stepName, url, method, headers, body, extractors, weight });
        }
    });
    return steps;
}

function collectValidations() {
    const validations = [];
    document.querySelectorAll('.validation-row').forEach(row => {
        const type = row.querySelector('.validation-type')?.value;
        const value = row.querySelector('.validation-value')?.value?.trim();
        const path = row.querySelector('.validation-path')?.value?.trim();
        if (type && value) {
            validations.push({ type, value, path: path || '' });
        }
    });
    return validations;
}

// ---- Chain Visualization ----

function renderChainVisualization(steps) {
    if (!steps || steps.length === 0) return '';

    const stepsHtml = steps.map((step, i) => {
        const extractors = step.extractors || [];
        const extractsStr = extractors.length > 0
            ? `<span class="text-[10px]" style="color:var(--accent-text);">extracts: ${extractors.map(e => e.name).join(', ')}</span>`
            : '';

        // Find variables used in this step (pattern: {{varname}})
        const usedVars = [];
        const searchStr = (step.url || '') + (step.body || '') + JSON.stringify(step.headers || {});
        const varMatch = searchStr.match(/\{\{(\w+)\}\}/g);
        if (varMatch) {
            varMatch.forEach(m => {
                const vName = m.replace(/\{\{|\}\}/g, '');
                if (!usedVars.includes(vName)) usedVars.push(vName);
            });
        }
        const usesStr = usedVars.length > 0
            ? `<span class="text-[10px]" style="color:#f59e0b;">uses: ${usedVars.map(v => '{{' + v + '}}').join(', ')}</span>`
            : '';

        const arrow = i < steps.length - 1
            ? `<div class="flex justify-center py-1"><svg class="w-4 h-4" style="color:var(--text-faint);" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 14l-7 7m0 0l-7-7m7 7V3"/></svg></div>`
            : '';

        return `
        <div class="flex items-center gap-3 px-3 py-2 rounded-lg" style="background:rgba(6,182,212,0.05);border:1px solid rgba(6,182,212,0.15);">
            <span class="text-[10px] font-mono font-bold px-1.5 py-0.5 rounded" style="background:rgba(6,182,212,0.15);color:#06b6d4;">${i + 1}</span>
            <div class="flex-1">
                <div class="flex items-center gap-2">
                    <span class="text-xs font-medium" style="color:var(--text);">${esc(step.name || 'Step ' + (i + 1))}</span>
                    ${methodBadge(step.method)}
                </div>
                <div class="flex items-center gap-3 mt-0.5">
                    ${usesStr}
                    ${extractsStr}
                </div>
            </div>
        </div>
        ${arrow}`;
    }).join('');

    return `
    <div class="card p-5 mb-6" style="border-color:rgba(6,182,212,0.2);">
        <div class="flex items-center gap-2 mb-3">
            <svg class="w-4 h-4" style="color:#06b6d4;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
            <h3 class="text-sm font-semibold" style="color:#06b6d4;">Request Chain</h3>
            <span class="text-[10px]" style="color:var(--text-subtle);">${steps.length} step${steps.length > 1 ? 's' : ''}</span>
        </div>
        ${stepsHtml}
    </div>`;
}

// ---- TLS Info Card ----

function renderTLSCard(results) {
    if (!results) return '';
    const proto = results.tls_protocol;
    if (!proto) return '';

    const cipher = results.tls_cipher_suite || '-';
    const server = results.tls_server_name || '-';
    const issuer = results.tls_issuer || '-';
    const handshake = results.tls_handshake_ms;
    const notAfter = results.tls_not_after;

    let expiryHtml = '-';
    let expiryColor = 'var(--text-muted)';
    if (notAfter) {
        const expDate = new Date(notAfter);
        const now = new Date();
        const daysLeft = Math.floor((expDate - now) / (1000 * 60 * 60 * 24));
        const dateStr = expDate.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
        if (daysLeft < 0) {
            expiryColor = '#ef4444';
            expiryHtml = `${dateStr} <span class="text-[10px] font-semibold">(EXPIRED)</span>`;
        } else if (daysLeft < 30) {
            expiryColor = '#f59e0b';
            expiryHtml = `${dateStr} <span class="text-[10px]">(${daysLeft}d left)</span>`;
        } else {
            expiryColor = '#10b981';
            expiryHtml = `${dateStr} <span class="text-[10px]">(${daysLeft}d left)</span>`;
        }
    }

    const handshakeHtml = handshake != null ? `
        <div class="flex items-center justify-between py-1.5 border-b border-slate-700/20">
            <span class="text-xs" style="color:var(--text-muted);">Handshake</span>
            <span class="text-xs font-mono font-medium" style="color:var(--text);">${fmtDec(handshake, 1)}ms</span>
        </div>` : '';

    return `
    <div class="card p-5 mb-6" style="border-color:rgba(16,185,129,0.2);">
        <div class="flex items-center gap-2 mb-3">
            <svg class="w-4 h-4" style="color:#10b981;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg>
            <h3 class="text-sm font-semibold" style="color:#10b981;">SSL/TLS Details</h3>
        </div>
        <div class="flex items-center justify-between py-1.5 border-b border-slate-700/20">
            <span class="text-xs" style="color:var(--text-muted);">Protocol</span>
            <span class="text-xs font-mono font-medium" style="color:var(--text);">${esc(proto)}</span>
        </div>
        <div class="flex items-center justify-between py-1.5 border-b border-slate-700/20">
            <span class="text-xs" style="color:var(--text-muted);">Cipher</span>
            <span class="text-xs font-mono font-medium" style="color:var(--text);">${esc(cipher)}</span>
        </div>
        <div class="flex items-center justify-between py-1.5 border-b border-slate-700/20">
            <span class="text-xs" style="color:var(--text-muted);">Server</span>
            <span class="text-xs font-mono font-medium" style="color:var(--text);">${esc(server)}</span>
        </div>
        <div class="flex items-center justify-between py-1.5 border-b border-slate-700/20">
            <span class="text-xs" style="color:var(--text-muted);">Issuer</span>
            <span class="text-xs font-mono font-medium" style="color:var(--text);">${esc(issuer)}</span>
        </div>
        <div class="flex items-center justify-between py-1.5 border-b border-slate-700/20">
            <span class="text-xs" style="color:var(--text-muted);">Expires</span>
            <span class="text-xs font-mono font-medium" style="color:${expiryColor};">${expiryHtml}</span>
        </div>
        ${handshakeHtml}
    </div>`;
}

// ---- Rate Limit Analysis Card ----

function renderCircuitCard(events) {
    if (!events || events.length === 0) return '';
    const stateColors = { open: '#ef4444', 'half-open': '#f59e0b', closed: '#10b981' };
    const lastState = events[events.length - 1].state;
    return `
    <div class="card p-5 mb-6" style="border-color:${lastState === 'closed' ? 'rgba(16,185,129,0.3)' : 'rgba(239,68,68,0.3)'};">
        <div class="flex items-center gap-2 mb-3">
            <svg class="w-4 h-4" style="color:${stateColors[lastState] || 'var(--text-muted)'};" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
            <h3 class="text-sm font-semibold" style="color:${stateColors[lastState] || 'var(--text-muted)'};">Circuit Breaker ${lastState === 'closed' ? '— Recovered' : '— Triggered'}</h3>
        </div>
        <div class="space-y-2">
            ${events.map(e => {
                const t = typeof e.t === 'number' ? (e.t / 1e9).toFixed(1) : '?';
                const c = stateColors[e.state] || 'var(--text-muted)';
                return `<div class="flex items-center gap-3 text-xs">
                    <span class="font-mono" style="color:var(--text-subtle);">${t}s</span>
                    <span class="px-1.5 py-0.5 rounded font-medium" style="background:${c}18;color:${c};">${e.state}</span>
                    <span style="color:var(--text-muted);">${esc(e.reason)}</span>
                </div>`;
            }).join('')}
        </div>
    </div>`;
}

function renderRateLimitCard(rateLimit) {
    if (!rateLimit || rateLimit.total_429s === 0) return '';

    return `
    <div class="card p-5 mb-6" style="border-color:rgba(245,158,11,0.3);">
        <div class="flex items-center gap-2 mb-3">
            <svg class="w-4 h-4" style="color:#f59e0b;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.618 5.984A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/></svg>
            <h3 class="text-sm font-semibold" style="color:#f59e0b;">Rate Limiting Detected</h3>
        </div>
        <div class="grid grid-cols-2 gap-4 mb-4">
            <div>
                <div class="text-xs" style="color:var(--text-subtle);">Total 429 Responses</div>
                <div class="text-lg font-bold" style="color:#f59e0b;">${fmt(rateLimit.total_429s)}</div>
            </div>
            <div>
                <div class="text-xs" style="color:var(--text-subtle);">First Hit At</div>
                <div class="text-lg font-bold" style="color:var(--text);">${rateLimit.first_hit_sec != null ? fmtDec(rateLimit.first_hit_sec, 1) + 's' : '-'}</div>
            </div>
            ${rateLimit.limit ? `<div>
                <div class="text-xs" style="color:var(--text-subtle);">Rate Limit</div>
                <div class="text-sm font-mono" style="color:var(--text);">${esc(rateLimit.limit)} req/window</div>
            </div>` : ''}
            ${rateLimit.retry_after ? `<div>
                <div class="text-xs" style="color:var(--text-subtle);">Retry After</div>
                <div class="text-sm font-mono" style="color:var(--text);">${esc(rateLimit.retry_after)}s</div>
            </div>` : ''}
            ${rateLimit.remaining ? `<div>
                <div class="text-xs" style="color:var(--text-subtle);">Remaining</div>
                <div class="text-sm font-mono" style="color:var(--text);">${esc(rateLimit.remaining)}</div>
            </div>` : ''}
            ${rateLimit.reset ? `<div>
                <div class="text-xs" style="color:var(--text-subtle);">Reset</div>
                <div class="text-sm font-mono" style="color:var(--text);">${esc(rateLimit.reset)}</div>
            </div>` : ''}
        </div>
        ${renderRateLimitChart(rateLimit.hits_over_time)}
    </div>`;
}

function renderRateLimitChart(hits) {
    if (!hits || hits.length === 0) return '';

    const W = 600, H = 140, PAD = {top: 10, right: 15, bottom: 25, left: 50};
    const plotW = W - PAD.left - PAD.right;
    const plotH = H - PAD.top - PAD.bottom;

    const maxT = Math.max(1, ...hits.map(d => d.t));
    const maxR = Math.max(1, ...hits.map(d => d.r));
    const barW = Math.max(4, Math.min(20, plotW / (hits.length + 1) - 2));

    const x = (t) => PAD.left + (t / maxT) * plotW;
    const y = (v) => PAD.top + plotH - (v / maxR) * plotH;

    let bars = '';
    hits.forEach(d => {
        const bx = x(d.t) - barW / 2;
        // Total requests bar (gray)
        if (d.r > 0) {
            bars += `<rect x="${bx}" y="${y(d.r)}" width="${barW}" height="${plotH - (plotH - (d.r / maxR) * plotH)}" rx="1" fill="var(--border-strong)" opacity="0.6"/>`;
        }
        // 429 bar (amber)
        if (d.c > 0) {
            bars += `<rect x="${bx}" y="${y(d.c)}" width="${barW}" height="${plotH - (plotH - (d.c / maxR) * plotH)}" rx="1" fill="#f59e0b" opacity="0.85"/>`;
        }
    });

    // Grid
    let grid = '';
    for (let i = 0; i <= 3; i++) {
        const yPos = PAD.top + (plotH / 3) * i;
        const val = maxR - (maxR / 3) * i;
        grid += `<line x1="${PAD.left}" y1="${yPos}" x2="${W-PAD.right}" y2="${yPos}" stroke="var(--border-strong)" stroke-width="0.5" stroke-dasharray="4,4"/>`;
        grid += `<text x="${PAD.left-8}" y="${yPos+4}" text-anchor="end" fill="var(--text-subtle)" font-size="9" font-family="JetBrains Mono">${Math.round(val)}</text>`;
    }

    // X-axis labels
    const step = Math.max(1, Math.ceil(maxT / 6));
    let xLabels = '';
    for (let t = 0; t <= maxT; t += step) {
        xLabels += `<text x="${x(t)}" y="${H - 5}" text-anchor="middle" fill="var(--text-subtle)" font-size="9" font-family="JetBrains Mono">${t}s</text>`;
    }

    return `
    <div>
        <div class="text-[10px] mb-2" style="color:var(--text-subtle);">429 Responses Over Time</div>
        <svg viewBox="0 0 ${W} ${H}" style="width:100%;">
            ${grid}${bars}${xLabels}
        </svg>
        <div class="flex items-center gap-4 mt-1 text-[10px]" style="color:var(--text-subtle);">
            <span><span style="display:inline-block;width:8px;height:8px;border-radius:2px;background:var(--border-strong);margin-right:3px;vertical-align:middle;"></span>Total Reqs</span>
            <span><span style="display:inline-block;width:8px;height:8px;border-radius:2px;background:#f59e0b;margin-right:3px;vertical-align:middle;"></span>429 Responses</span>
        </div>
    </div>`;
}

// ---- Timing Breakdown Card ----

function renderTimingCard(results) {
    const t = results?.timing;
    if (!t) return '';

    const total = (t.dns_ms || 0) + (t.tcp_ms || 0) + (t.tls_ms || 0) + (t.ttfb_ms || 0) + (t.transfer_ms || 0);
    if (total === 0) return '';

    const bar = (val, color, label) => {
        const pct = total > 0 ? (val / total * 100) : 0;
        return pct > 0 ? `<div style="width:${pct}%;background:${color};height:100%;min-width:2px;" title="${label}: ${val.toFixed(1)}ms"></div>` : '';
    };

    return `
    <div class="card p-5 mb-6">
        <h3 class="text-sm font-semibold mb-3" style="color:var(--text);">Request Timing Breakdown</h3>
        <div class="flex rounded-lg overflow-hidden h-6 mb-4">
            ${bar(t.dns_ms, '#10b981', 'DNS')}
            ${bar(t.tcp_ms, '#3b82f6', 'TCP')}
            ${bar(t.tls_ms, '#8b5cf6', 'TLS')}
            ${bar(t.ttfb_ms, '#f59e0b', 'TTFB')}
            ${bar(t.transfer_ms, '#06b6d4', 'Transfer')}
        </div>
        <div class="grid grid-cols-5 gap-2 text-center">
            <div><div class="w-3 h-3 rounded-full mx-auto mb-1" style="background:#10b981;"></div><div class="text-[10px]" style="color:var(--text-subtle);">DNS</div><div class="text-xs font-mono font-medium" style="color:var(--text);">${(t.dns_ms||0).toFixed(1)}ms</div></div>
            <div><div class="w-3 h-3 rounded-full mx-auto mb-1" style="background:#3b82f6;"></div><div class="text-[10px]" style="color:var(--text-subtle);">TCP</div><div class="text-xs font-mono font-medium" style="color:var(--text);">${(t.tcp_ms||0).toFixed(1)}ms</div></div>
            <div><div class="w-3 h-3 rounded-full mx-auto mb-1" style="background:#8b5cf6;"></div><div class="text-[10px]" style="color:var(--text-subtle);">TLS</div><div class="text-xs font-mono font-medium" style="color:var(--text);">${(t.tls_ms||0).toFixed(1)}ms</div></div>
            <div><div class="w-3 h-3 rounded-full mx-auto mb-1" style="background:#f59e0b;"></div><div class="text-[10px]" style="color:var(--text-subtle);">TTFB</div><div class="text-xs font-mono font-medium" style="color:var(--text);">${(t.ttfb_ms||0).toFixed(1)}ms</div></div>
            <div><div class="w-3 h-3 rounded-full mx-auto mb-1" style="background:#06b6d4;"></div><div class="text-[10px]" style="color:var(--text-subtle);">Transfer</div><div class="text-xs font-mono font-medium" style="color:var(--text);">${(t.transfer_ms||0).toFixed(1)}ms</div></div>
        </div>
    </div>`;
}

// ---- SVG Line Charts ----

let _chartIdCounter = 0;

function renderSVGLineChart(data, yKey, color, label, unit) {
    if (data.length < 2) return `<div class="text-xs text-center py-8" style="color:var(--text-subtle);">Collecting data...</div>`;

    const W = 600, H = 180, PAD = {top: 10, right: 15, bottom: 25, left: 50};
    const plotW = W - PAD.left - PAD.right;
    const plotH = H - PAD.top - PAD.bottom;

    const maxY = Math.max(1, ...data.map(d => d[yKey])) * 1.1;
    // Scale x over the actual [minT, maxT] window rather than assuming the
    // series starts at t=0. Without this, re-entering/refreshing a running
    // test (where the first live point starts mid-run) squishes the line into
    // the right side of the chart.
    const minT = Math.min(...data.map(d => d.t));
    const maxT = Math.max(minT + 1, ...data.map(d => d.t));
    const spanT = maxT - minT;

    const x = (t) => PAD.left + ((t - minT) / spanT) * plotW;
    const y = (v) => PAD.top + plotH - (v / maxY) * plotH;

    const points = data.map(d => `${x(d.t).toFixed(1)},${y(d[yKey]).toFixed(1)}`).join(' ');
    const areaPoints = `${x(data[0].t).toFixed(1)},${y(0).toFixed(1)} ${points} ${x(data[data.length-1].t).toFixed(1)},${y(0).toFixed(1)}`;

    const gradId = `grad-${yKey}-${++_chartIdCounter}`;

    // Grid lines (4 horizontal)
    let grid = '';
    for (let i = 0; i <= 3; i++) {
        const yPos = PAD.top + (plotH / 3) * i;
        const val = maxY - (maxY / 3) * i;
        grid += `<line x1="${PAD.left}" y1="${yPos}" x2="${W-PAD.right}" y2="${yPos}" stroke="var(--border-strong)" stroke-width="0.5" stroke-dasharray="4,4"/>`;
        grid += `<text x="${PAD.left-8}" y="${yPos+4}" text-anchor="end" fill="var(--text-subtle)" font-size="10" font-family="JetBrains Mono">${formatChartVal(val, unit)}</text>`;
    }

    // X-axis labels
    let xLabels = '';
    const step = Math.max(1, Math.floor(data.length / 6));
    for (let i = 0; i < data.length; i += step) {
        xLabels += `<text x="${x(data[i].t)}" y="${H-5}" text-anchor="middle" fill="var(--text-subtle)" font-size="10" font-family="JetBrains Mono">${data[i].t.toFixed(0)}s</text>`;
    }

    const lastPoint = data[data.length - 1];

    return `<svg viewBox="0 0 ${W} ${H}" class="w-full" style="height:200px;">
        <defs>
            <linearGradient id="${gradId}" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stop-color="${color}" stop-opacity="0.25"/>
                <stop offset="100%" stop-color="${color}" stop-opacity="0.02"/>
            </linearGradient>
        </defs>
        ${grid}
        <polygon points="${areaPoints}" fill="url(#${gradId})"/>
        <polyline points="${points}" fill="none" stroke="${color}" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"/>
        <circle cx="${x(lastPoint.t)}" cy="${y(lastPoint[yKey])}" r="4" fill="${color}" stroke="var(--bg)" stroke-width="2"/>
        ${xLabels}
        <text x="${PAD.left}" y="${PAD.top - 2}" fill="${color}" font-size="11" font-family="Inter" font-weight="600">${label}: ${formatChartVal(lastPoint[yKey], unit)}</text>
    </svg>`;
}

function formatChartVal(v, unit) {
    if (unit === 'ms') return v < 1000 ? v.toFixed(0) + 'ms' : (v/1000).toFixed(1) + 's';
    return v.toFixed(1) + (unit ? ' ' + unit : '');
}

// Compact axis-less sparkline for dashboard cards. Points are evenly spaced;
// returns '' when there isn't enough data to draw a line.
function renderSparkline(points, key, color, w = 132, h = 30) {
    const vals = (points || []).map(p => p[key] || 0);
    if (vals.length < 2) return '';
    const max = Math.max(1, ...vals);
    const min = Math.min(...vals);
    const span = Math.max(1e-6, max - min);
    const n = vals.length;
    const x = i => (i / (n - 1)) * w;
    const y = v => (h - 2) - ((v - min) / span) * (h - 4);
    const line = vals.map((v, i) => `${x(i).toFixed(1)},${y(v).toFixed(1)}`).join(' ');
    const area = `0,${h} ${line} ${w},${h}`;
    const gid = `spark-${key}-${++_chartIdCounter}`;
    return `<svg viewBox="0 0 ${w} ${h}" width="${w}" height="${h}" preserveAspectRatio="none" style="display:block;">
        <defs><linearGradient id="${gid}" x1="0" y1="0" x2="0" y2="1"><stop offset="0%" stop-color="${color}" stop-opacity="0.25"/><stop offset="100%" stop-color="${color}" stop-opacity="0"/></linearGradient></defs>
        <polygon points="${area}" fill="url(#${gid})"/>
        <polyline points="${line}" fill="none" stroke="${color}" stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round"/>
    </svg>`;
}

// Parses a field that may arrive either as a JSON string (raw history rows) or
// as an already-decoded object/array (snapshot API responses). Returns null on
// failure.
function parseMaybeJSON(v) {
    if (v == null) return null;
    if (typeof v === 'string') { try { return JSON.parse(v); } catch (_) { return null; } }
    return v;
}

// Extracts the interval timeline from a result row as an array of points, coping
// with both the string and pre-parsed representations.
function resultTimeline(r) {
    const tl = parseMaybeJSON(r && r.timeline);
    return Array.isArray(tl) ? tl : [];
}

// Multi-series line chart used to overlay several runs on shared axes.
// series: [{ name, color, points: [{ t (seconds), val }] }]. `unit` is 'ms' or ''.
function renderOverlayChart(series, unit) {
    const all = series.flatMap(s => s.points);
    if (all.length < 2) return `<div class="text-xs text-center py-8" style="color:var(--text-subtle);">No timeline data for these runs.</div>`;

    const W = 600, H = 210, PAD = { top: 12, right: 15, bottom: 28, left: 52 };
    const plotW = W - PAD.left - PAD.right, plotH = H - PAD.top - PAD.bottom;
    const maxY = Math.max(1, ...all.map(p => p.val)) * 1.1;
    const maxT = Math.max(1, ...all.map(p => p.t));
    const x = t => PAD.left + (t / maxT) * plotW;
    const y = v => PAD.top + plotH - (v / maxY) * plotH;

    let grid = '';
    for (let i = 0; i <= 3; i++) {
        const yPos = PAD.top + (plotH / 3) * i;
        const val = maxY - (maxY / 3) * i;
        grid += `<line x1="${PAD.left}" y1="${yPos}" x2="${W - PAD.right}" y2="${yPos}" stroke="var(--border-strong)" stroke-width="0.5" stroke-dasharray="4,4"/>`;
        grid += `<text x="${PAD.left - 8}" y="${yPos + 4}" text-anchor="end" fill="var(--text-subtle)" font-size="10" font-family="JetBrains Mono">${formatChartVal(val, unit)}</text>`;
    }
    let xLabels = '';
    for (let i = 0; i <= 4; i++) {
        const t = (maxT / 4) * i;
        xLabels += `<text x="${x(t)}" y="${H - 6}" text-anchor="middle" fill="var(--text-subtle)" font-size="10" font-family="JetBrains Mono">${t.toFixed(0)}s</text>`;
    }
    const lines = series.map(s => {
        if (s.points.length < 2) return '';
        const pts = s.points.map(p => `${x(p.t).toFixed(1)},${y(p.val).toFixed(1)}`).join(' ');
        return `<polyline points="${pts}" fill="none" stroke="${s.color}" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" opacity="0.9"/>`;
    }).join('');

    return `<svg viewBox="0 0 ${W} ${H}" class="w-full" style="height:210px;">
        ${grid}${lines}${xLabels}
    </svg>`;
}

// Small colored-dot legend shared by the overlay charts.
function overlayLegend(series) {
    return `<div class="flex flex-wrap gap-x-4 gap-y-1 mb-3">${series.map(s =>
        `<div class="flex items-center gap-1.5"><span style="width:10px;height:10px;border-radius:2px;background:${s.color};display:inline-block;"></span><span class="text-[11px]" style="color:var(--text-muted);">${esc(s.name)}</span></div>`
    ).join('')}</div>`;
}

// ---- Running View ----

function renderRunningView(id) {
    const svc = state.services.find(s => s.id == id);
    if (!svc) return notFound();

    return `
    ${breadcrumb({ label: 'Dashboard', href: '#/' }, { label: svc.name, href: '#/services/' + id }, { label: 'Running' })}

    <div class="flex items-center justify-between mb-6">
        <div class="flex items-center gap-3">
            <h2 class="text-2xl font-bold" style="color:var(--text);">${esc(svc.name)}</h2>
            ${methodBadge(svc.method, 'lg')}
            <span class="text-sm font-mono" style="color:var(--text-subtle);">${esc(svc.url)}</span>
        </div>
        <button onclick="handleStopTest('${id}')" id="stop-btn" class="btn btn-danger btn-sm">${svgIcon('stop')}<span>Stop Test</span></button>
    </div>

    <!-- Progress -->
    <div class="card p-5 mb-6" id="live-progress">
        <div class="flex items-center justify-between mb-3">
            <div class="flex items-center gap-3">
                <div class="pulse-dot" id="live-pulse"></div>
                <span class="text-sm font-semibold" id="live-status" style="color:#10b981;">Starting...</span>
            </div>
            <div class="text-sm font-mono" style="color:var(--text-muted);" id="live-time">--</div>
        </div>
        <div class="progress-track">
            <div class="progress-fill" id="live-bar" style="width:0%;"></div>
        </div>
        <div class="text-right text-xs mt-1.5 font-medium font-mono" style="color:var(--text-muted);" id="live-pct">0%</div>
    </div>

    <!-- Live headline numbers -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6" id="live-stats">
        ${metricCard('Total Requests', '0', 'var(--text)', 'stat-reqs', '<div id="stat-corrected"></div>')}
        ${metricCard('Requests/sec', '0', '#06b6d4', 'stat-rps', '<div class="text-[10px] mt-1" style="color:var(--text-subtle);">current throughput</div>')}
        ${metricCard('Error Rate', '0%', '#10b981', 'stat-err', '<div class="text-[10px] mt-1" style="color:var(--text-subtle);" id="stat-errcnt">0 errors</div>')}
        ${metricCard('Avg Latency', '--', '#7c3aed', 'stat-avglat', '<div id="stat-valfails"></div>')}
    </div>

    <div id="live-timing" class="mb-4" style="display:none;"></div>

    <div class="grid grid-cols-2 gap-4 mb-6" id="live-charts">
        <div class="card p-4">
            <h3 class="text-xs font-semibold mb-2" style="color:var(--text-muted);">Requests/sec <span class="font-normal" style="color:var(--text-faint);">(per interval)</span></h3>
            <div id="live-chart-rps"><div class="text-xs text-center py-8" style="color:var(--text-subtle);">Collecting data…</div></div>
        </div>
        <div class="card p-4">
            <h3 class="text-xs font-semibold mb-2" style="color:var(--text-muted);">Latency <span class="font-normal" style="color:var(--text-faint);">(per interval)</span></h3>
            <div id="live-chart-lat"><div class="text-xs text-center py-8" style="color:var(--text-subtle);">Collecting data…</div></div>
        </div>
    </div>

    <div class="live-tls-stat flex items-center gap-2 mb-4" style="display:none;">
        <svg class="w-3.5 h-3.5" style="color:#10b981;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg>
        <span class="text-xs" style="color:var(--text-muted);">TLS Handshake:</span>
        <span class="text-xs font-mono font-medium" style="color:#10b981;" id="live-tls-handshake">-</span>
    </div>

    <div id="live-metrics">${renderMetricsBlock({})}</div>
    <div id="live-actions" class="mt-6" style="display:none;"></div>`;
}

// ---- Metrics Block (shared) ----

function renderMetricsBlock(m) {
    const codes = m.status_codes || {};
    const codeEntries = Object.entries(codes).sort((a, b) => a[0].localeCompare(b[0]));
    const maxCount = Math.max(1, ...Object.values(codes));
    const extraNotes = `${m.corrected_reqs > 0 ? `<div class="text-[10px] mt-2" style="color:#f59e0b;">+${fmt(m.corrected_reqs)} coordinated-omission corrected</div>` : ''}${m.validation_failures > 0 ? `<div class="text-[10px] mt-1" style="color:#ef4444;">${fmt(m.validation_failures)} validation failures</div>` : ''}`;

    return `
    <div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
        <div class="card p-5">
            <h3 class="text-sm font-semibold mb-3" style="color:var(--text);">Latency Percentiles</h3>
            <table class="w-full text-sm mb-4">
                <tbody id="lat-table">${latRows(m)}</tbody>
            </table>
            ${renderLatencyBars(m)}
        </div>
        <div class="card p-5" id="codes-card">
            <h3 class="text-sm font-semibold mb-3" style="color:var(--text);">Status Codes</h3>
            <div id="codes-inner">${renderCodeBars(codeEntries, maxCount)}</div>
            ${extraNotes}
        </div>
    </div>`;
}

function metricCard(label, value, color, id, extra = '') {
    return `<div class="stat-card">
        <div class="text-xs font-medium mb-1" style="color:var(--text-muted);">${label}</div>
        <div class="text-2xl font-bold stat-number" style="color:${color};" id="${id}">${value}</div>${extra}</div>`;
}

function latRows(m) {
    return [['Avg',m.avg_latency_ms],['P50',m.p50_latency_ms],['P95',m.p95_latency_ms],
            ['P99',m.p99_latency_ms],['Min',m.min_latency_ms],['Max',m.max_latency_ms]]
        .map(([l,v]) => `<tr class="border-b border-slate-700/30"><td class="py-2 font-mono text-xs" style="color:var(--text-muted);">${l}</td><td class="py-2 text-right font-mono text-xs font-medium" style="color:var(--text);">${fmtLatency(v)}</td></tr>`).join('');
}

function renderCodeBars(entries, max) {
    if (!entries.length) return '<p class="text-xs" style="color:var(--text-faint);">No responses yet.</p>';
    return `<div class="space-y-3">${entries.map(([code, count]) => {
        const pct = (count / max) * 100, c = statusCodeColor(code), label = statusCodeLabel(code);
        return `<div><div class="flex justify-between mb-1"><span class="text-xs font-mono font-medium" style="color:${c};">${label}</span><span class="text-xs font-mono" style="color:var(--text-muted);">${fmt(count)}</span></div><div class="w-full h-2 rounded-full" style="background:${c}12;"><div class="h-full rounded-full" style="width:${pct}%;background:${c};transition:width .15s ease-out;"></div></div></div>`;
    }).join('')}</div>`;
}

function distBar(b) {
    return `<div class="flex-1 flex flex-col items-center justify-end h-full">
        <div class="w-full rounded-t" style="height:${b.pct}%;background:linear-gradient(to top,#7c3aed,#06b6d4);opacity:${0.4+b.pct/150};transition:height .15s ease-out;"></div>
        <div class="text-[9px] mt-1.5 font-mono whitespace-nowrap" style="color:var(--text-subtle);">${b.label}</div></div>`;
}

// ---- Live DOM Patching ----

function updateLiveMetrics(m) {
    const progress = Math.min((m.progress||0)*100, 100);
    const elapsed = fmtDuration(m.duration_ms);
    const total = fmtDuration(m.duration_total_ms);
    const remaining = (m.duration_total_ms && m.duration_ms)
        ? fmtDuration(Math.max(0, m.duration_total_ms - m.duration_ms)) : '--';
    const errorRate = m.total_reqs > 0 ? ((m.errors||0)/m.total_reqs*100) : 0;
    const errorColor = errorRate > 10 ? '#ef4444' : errorRate > 2 ? '#f59e0b' : '#10b981';

    // Circuit breaker status
    if (m.circuit_state === 'open') {
        setHtml('live-status', '<span style="color:#ef4444;">Circuit Open — Service Down</span>');
    } else if (m.circuit_state === 'half-open') {
        setHtml('live-status', '<span style="color:#f59e0b;">Probing — Testing Recovery</span>');
    } else {
        setHtml('live-status', 'Running');
    }
    setHtml('live-time', `${elapsed} / ${total} \u2014 ${remaining} remaining`);
    setStyle('live-bar', 'width', `${progress}%`);
    setHtml('live-pct', `${progress.toFixed(1)}%`);

    setText('stat-reqs', fmt(m.total_reqs));
    // Show the latest interval throughput (matches the live chart) rather than
    // the cumulative average, falling back to cumulative before any interval.
    const lastTl = Array.isArray(m.timeline) && m.timeline.length ? m.timeline[m.timeline.length - 1] : null;
    setText('stat-rps', fmtDec(lastTl ? lastTl.rps : m.rps));
    const errEl = document.getElementById('stat-err');
    if (errEl) { errEl.textContent = fmtDec(errorRate)+'%'; errEl.style.color = errorColor; }
    setText('stat-errcnt', `${fmt(m.errors||0)} errors`);
    setText('stat-avglat', fmtLatency(m.avg_latency_ms));

    const tbody = document.getElementById('lat-table');
    if (tbody) tbody.innerHTML = latRows(m);

    const codesInner = document.getElementById('codes-inner');
    if (codesInner) {
        const codes = m.status_codes || {};
        const entries = Object.entries(codes).sort((a,b)=>a[0].localeCompare(b[0]));
        codesInner.innerHTML = renderCodeBars(entries, Math.max(1, ...Object.values(codes)));
    }

    const db = document.getElementById('dist-bars');
    if (db) db.innerHTML = buildLatencyBuckets(m).map(distBar).join('');

    const rpsChart = document.getElementById('live-chart-rps');
    if (rpsChart) rpsChart.innerHTML = renderSVGLineChart(state.liveTimeSeries, 'rps', '#06b6d4', 'RPS', '/s');

    const latChart = document.getElementById('live-chart-lat');
    if (latChart) latChart.innerHTML = renderSVGLineChart(state.liveTimeSeries, 'lat', '#7c3aed', 'Latency', 'ms');

    // TLS handshake stat
    const tlsEl = document.getElementById('live-tls-handshake');
    if (tlsEl && m.tls_handshake_ms != null) {
        tlsEl.textContent = fmtDec(m.tls_handshake_ms, 1) + 'ms';
        tlsEl.closest('.live-tls-stat').style.display = 'flex';
    }

    // CO-corrected and validation failures
    const correctedEl = document.getElementById('stat-corrected');
    if (correctedEl) {
        if (m.corrected_reqs > 0) {
            correctedEl.innerHTML = `<div class="text-[10px] mt-1" style="color:#f59e0b;">+${fmt(m.corrected_reqs)} CO-corrected</div>`;
        }
    }
    const valFailsEl = document.getElementById('stat-valfails');
    if (valFailsEl) {
        if (m.validation_failures > 0) {
            valFailsEl.innerHTML = `<div class="text-[10px] mt-1" style="color:#ef4444;">${fmt(m.validation_failures)} validation fails</div>`;
        }
    }

    // Live timing breakdown
    const timingEl = document.getElementById('live-timing');
    if (timingEl && m.timing) {
        timingEl.style.display = 'block';
        const t = m.timing;
        timingEl.innerHTML = `<div class="flex items-center gap-4 text-[10px] font-mono" style="color:var(--text-muted);">
            <span style="color:#10b981;">DNS ${(t.dns_ms||0).toFixed(0)}ms</span>
            <span style="color:#3b82f6;">TCP ${(t.tcp_ms||0).toFixed(0)}ms</span>
            <span style="color:#8b5cf6;">TLS ${(t.tls_ms||0).toFixed(0)}ms</span>
            <span style="color:#f59e0b;">TTFB ${(t.ttfb_ms||0).toFixed(0)}ms</span>
            <span style="color:#06b6d4;">Transfer ${(t.transfer_ms||0).toFixed(0)}ms</span>
        </div>`;
    }

    // Rate limit warning banner
    let rlBanner = document.getElementById('live-rate-limit-banner');
    if (m.rate_limit && m.rate_limit.total_429s > 0) {
        if (!rlBanner) {
            const container = document.getElementById('live-progress')?.parentElement;
            if (container) {
                const div = document.createElement('div');
                div.id = 'live-rate-limit-banner';
                div.className = 'card p-3 mb-4';
                div.style.cssText = 'border-color:rgba(245,158,11,0.4);background:rgba(245,158,11,0.08);';
                container.insertBefore(div, container.children[1] || null);
                rlBanner = div;
            }
        }
        if (rlBanner) {
            rlBanner.innerHTML = `<div class="flex items-center gap-2">
                <svg class="w-4 h-4 shrink-0" style="color:#f59e0b;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4.5c-.77-.833-2.694-.833-3.464 0L3.34 16.5c-.77.833.192 2.5 1.732 2.5z"/></svg>
                <span class="text-sm font-medium" style="color:#f59e0b;">Rate limiting detected: ${fmt(m.rate_limit.total_429s)} HTTP 429 responses</span>
            </div>`;
        }
    }
}

function setHtml(id, h) { const e = document.getElementById(id); if (e) e.innerHTML = h; }
function setText(id, t) { const e = document.getElementById(id); if (e) e.textContent = t; }
function setStyle(id, p, v) { const e = document.getElementById(id); if (e) e.style[p] = v; }

// ---- SSE ----

function startStream(id) {
    if (state.eventSource) state.eventSource.close();
    const es = new EventSource(`/api/services/${id}/stream`);
    state.eventSource = es;

    es.addEventListener('metrics', e => {
        try {
            const m = JSON.parse(e.data);
            state.testResults = m;
            // Chart the server-computed interval timeline (true per-interval RPS
            // and latency) rather than cumulative averages, which flatten out and
            // hide real-time throughput/latency changes. Timeline `t` is in
            // nanoseconds; convert to seconds.
            if (Array.isArray(m.timeline) && m.timeline.length) {
                let ts = m.timeline.map(p => ({
                    t: (p.t || 0) / 1e9,
                    rps: p.rps || 0,
                    lat: p.lat_ms || 0,
                    errs: p.errs || 0
                }));
                // Cap to last 3600 points to bound the SVG path size.
                if (ts.length > 3600) ts = ts.slice(ts.length - 3600);
                state.liveTimeSeries = ts;
            }
            if (document.getElementById('live-progress')) updateLiveMetrics(m);
        } catch (err) { console.error('SSE parse:', err); }
    });

    es.addEventListener('done', e => {
        try { state.testResults = JSON.parse(e.data); } catch (_) {}
        es.close(); state.eventSource = null;
        notifyTestComplete(id);
        navigate(`/services/${id}`);
    });

    es.addEventListener('error', () => {
        // EventSource auto-reconnects on transient errors (readyState CONNECTING),
        // so a momentary network blip shouldn't kick the user out of the live
        // view. Only bail to the detail page once the connection is permanently
        // closed (e.g. the endpoint returned a non-2xx and won't be retried).
        if (es.readyState === EventSource.CLOSED && state.currentRoute?.page === 'running') {
            es.close(); state.eventSource = null;
            navigate(`/services/${id}`);
        }
    });
}

// ---- Capacity Probe (knee finder) ----

// Opens the capacity hub for a service (history + run button).
function handleFindCapacity(id) { navigate(`/services/${id}/capacity`); }

// ---- Simulate Launch (open-model spike) ----

function showLaunchModal(id) {
    const container = document.getElementById('modal-container');
    container.innerHTML = `
    <div class="modal-overlay" id="modal-overlay">
        <div class="modal-box" style="max-width:460px;">
            <h3 class="text-lg font-semibold mb-1" style="color:var(--text);">Simulate a launch spike</h3>
            <p class="text-sm mb-5" style="color:var(--text-muted);">Models a sudden flood of visitors — a TV ad, a viral moment. Traffic ramps to your peak in ~20 seconds, holds, then eases off. New arrivals keep coming even if the target slows down (a realistic thundering herd).</p>
            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Expected peak traffic (requests/sec)</label>
            <input id="launch-peak" type="number" min="1" value="500" class="input-dark w-full mb-4" placeholder="e.g. 500"
                   onkeydown="if(event.key==='Enter')handleSimulateLaunch('${id}')">
            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Sustain the peak for</label>
            <select id="launch-sustain" class="input-dark input-sm w-full mb-2">
                <option value="1">1 minute</option>
                <option value="3" selected>3 minutes</option>
                <option value="5">5 minutes</option>
                <option value="10">10 minutes</option>
            </select>
            <p class="text-[11px] mb-6" style="color:var(--text-faint);">Tip: run <strong>Find Capacity</strong> first to know one instance's ceiling, then size your peak accordingly. Test the real path (through your CDN/load balancer), not localhost.</p>
            <div class="flex items-center justify-end gap-3">
                <button class="btn btn-ghost btn-sm" onclick="document.getElementById('modal-container').innerHTML='';">Cancel</button>
                <button class="btn btn-primary btn-sm" onclick="handleSimulateLaunch('${id}')">Start simulation</button>
            </div>
        </div>
    </div>`;
    const ov = document.getElementById('modal-overlay');
    ov.onclick = (e) => { if (e.target.id === 'modal-overlay') container.innerHTML = ''; };
    setTimeout(() => document.getElementById('launch-peak')?.focus(), 50);
}

async function handleSimulateLaunch(id) {
    const peak = parseInt(document.getElementById('launch-peak')?.value, 10);
    const sustainMin = parseInt(document.getElementById('launch-sustain')?.value, 10) || 3;
    if (!peak || peak < 1) { toast('Enter a peak traffic value (requests/sec).', 'error'); return; }
    document.getElementById('modal-container').innerHTML = '';
    // Ramp up to peak in 20s, hold, then ease off — arrival rate (open model).
    const stages = [
        { duration: '20s', target: peak, rps: 0 },
        { duration: (sustainMin * 60) + 's', target: peak, rps: 0 },
        { duration: '30s', target: 0, rps: 0 },
    ];
    try {
        state.testResults = null;
        state.liveTimeSeries = [];
        await api(`/api/services/${id}/run-pattern`, {
            method: 'POST',
            body: JSON.stringify({ pattern_name: 'Launch Spike', stages, open_model: true }),
        });
        navigate(`/services/${id}/run`);
    } catch (err) {
        if (/already running/i.test(err.message)) { navigate(`/services/${id}/run`); return; }
        toast('Failed to start launch simulation: ' + err.message, 'error');
    }
}

// The capacity hub: header + "Run capacity test" + a history list where each
// past run expands to its full detail.
function renderCapacityView(id) {
    const svc = state.services.find(s => s.id == id);
    if (!svc) return notFound();
    return `
    ${breadcrumb({ label: 'Dashboard', href: '#/' }, { label: svc.name, href: '#/services/' + id }, { label: 'Capacity' })}

    <div class="flex items-start justify-between mb-6 flex-wrap gap-3">
        <div>
            <h2 class="text-2xl font-bold" style="color:var(--text);">Capacity — ${esc(svc.name)}</h2>
            <p class="text-sm mt-1" style="color:var(--text-muted);">Auto-ramps load to the saturation knee — the most this service sustains before latency climbs. Click a past run to see its detail.</p>
        </div>
        <button onclick="runCapacityFromHub('${id}')" id="cap-run-btn" class="btn btn-primary btn-sm">${svgIcon('play')}<span>Run capacity test</span></button>
    </div>

    <div id="capacity-content"><div class="card p-6 text-center text-sm" style="color:var(--text-muted);">Loading…</div></div>`;
}

// Decide what to show in the hub: live probing if one is running, else history.
async function initCapacityView(id) {
    try { await refreshServices(); } catch (_) {}
    const svc = state.services.find(s => s.id == id);
    if (svc && svc.is_running) showCapacityLive(id);
    else renderCapacityRuns(id);
}

function showCapacityLive(id) {
    const el = document.getElementById('capacity-content');
    const runBtn = document.getElementById('cap-run-btn');
    if (runBtn) { runBtn.disabled = true; runBtn.style.opacity = '0.5'; }
    if (el) el.innerHTML = `
        <div class="card p-6 text-center">
            <div class="flex items-center justify-center gap-3 mb-3"><div class="pulse-dot"></div><span class="text-lg font-semibold" style="color:var(--text);">Probing capacity…</span></div>
            <p class="text-sm mb-5" style="color:var(--text-muted);">Ramping load up level by level, watching where throughput stops growing and latency starts climbing. Stops automatically at the knee.</p>
            <div class="grid grid-cols-3 gap-4 max-w-xl mx-auto">
                ${metricCard('Current level', '—', '#f59e0b', 'cap-conc', '<div class="text-[10px] mt-1" style="color:var(--text-subtle);">concurrent</div>')}
                ${metricCard('Throughput', '0', '#06b6d4', 'cap-rps', '<div class="text-[10px] mt-1" style="color:var(--text-subtle);">req/sec now</div>')}
                ${metricCard('Requests', '0', 'var(--text)', 'cap-reqs')}
            </div>
            <div class="mt-5"><button onclick="handleStopTest('${id}')" id="stop-btn" class="btn btn-danger btn-sm">${svgIcon('stop')}<span>Stop</span></button></div>
            <div class="mt-6"><div id="cap-chart"><div class="text-xs text-center py-4" style="color:var(--text-subtle);">Collecting data…</div></div></div>
        </div>`;
    startCapacityStream(id);
}

async function runCapacityFromHub(id) {
    try {
        await api(`/api/services/${id}/capacity-probe`, { method: 'POST' });
        state.liveTimeSeries = [];
        showCapacityLive(id);
    } catch (err) {
        if (/already running/i.test(err.message)) { showCapacityLive(id); return; }
        toast('Failed to start capacity test: ' + err.message, 'error');
    }
}

function startCapacityStream(id) {
    if (state.eventSource) state.eventSource.close();
    const es = new EventSource(`/api/services/${id}/stream`);
    state.eventSource = es;

    es.addEventListener('metrics', e => {
        try {
            const m = JSON.parse(e.data);
            state.testResults = m;
            if (Array.isArray(m.timeline) && m.timeline.length) {
                let ts = m.timeline.map(p => ({ t: (p.t || 0) / 1e9, rps: p.rps || 0, lat: p.lat_ms || 0, conc: p.conc || 0 }));
                if (ts.length > 3600) ts = ts.slice(ts.length - 3600);
                state.liveTimeSeries = ts;
            }
            const last = state.liveTimeSeries[state.liveTimeSeries.length - 1];
            setText('cap-conc', last ? fmt(last.conc) : '—');
            setText('cap-rps', last ? fmtDec(last.rps) : '0');
            setText('cap-reqs', fmt(m.total_reqs));
            const chart = document.getElementById('cap-chart');
            if (chart && state.liveTimeSeries.length >= 2) chart.innerHTML = renderSVGLineChart(state.liveTimeSeries, 'rps', '#06b6d4', 'RPS', '/s');
        } catch (_) {}
    });

    const finish = () => {
        es.close(); state.eventSource = null;
        if (state.currentRoute?.page === 'capacity') renderCapacityRuns(id);
    };
    es.addEventListener('done', finish);
    es.addEventListener('error', () => {
        if (es.readyState === EventSource.CLOSED && state.currentRoute?.page === 'capacity') finish();
    });
}

// Loads the run history and renders it as an expandable list (newest first).
async function renderCapacityRuns(id) {
    const el = document.getElementById('capacity-content');
    const runBtn = document.getElementById('cap-run-btn');
    if (runBtn) { runBtn.disabled = false; runBtn.style.opacity = ''; }
    if (!el) return;
    let runs = [];
    try { runs = (await api(`/api/services/${id}/capacity-probe`)) || []; } catch (_) {}
    if (!runs.length) {
        el.innerHTML = emptyState('No capacity tests yet', "Run your first capacity test to find this service's saturation point and how many users it can handle.", null, null);
        return;
    }
    el.innerHTML = `<div class="space-y-3">${runs.map((run, i) => capacityRunItem(id, run, i === 0)).join('')}</div>`;
}

// One row in the history list — a summary header that expands to the detail.
function capacityRunItem(id, run, expanded) {
    const res = run.result || {};
    const saturated = res.reason !== 'max_reached';
    const knee = res.knee_concurrency || 0;
    const maxRps = res.max_rps || 0;
    const when = run.created_at ? fmtRelativeTime(run.created_at) : '';
    const ri = {
        throughput_plateau: { short: 'throughput plateau', color: '#06b6d4' },
        latency_degraded: { short: 'latency degraded', color: '#f59e0b' },
        errors: { short: 'errors', color: '#ef4444' },
        max_reached: { short: 'no saturation', color: '#94a3b8' },
    }[res.reason] || { short: res.reason || '', color: '#94a3b8' };
    const summary = saturated
        ? `~<span style="color:var(--accent-text);">${fmt(knee)}</span> concurrent · ~<span style="color:#06b6d4;">${fmtDec(maxRps)}</span> req/s`
        : `~<span style="color:#06b6d4;">${fmtDec(maxRps)}</span> req/s`;

    return `
    <div class="card overflow-hidden">
        <div class="w-full flex items-center justify-between gap-3 hover:bg-slate-700/20 transition-colors">
            <button onclick="toggleCapacityRun(${run.id})" class="flex-1 flex items-center justify-between gap-3 p-4 text-left" style="background:none;border:none;cursor:pointer;">
                <div class="flex items-center gap-3 flex-wrap">
                    <svg id="cap-chev-${run.id}" class="w-4 h-4 shrink-0" style="color:var(--text-subtle);transition:transform .15s;${expanded ? 'transform:rotate(90deg);' : ''}" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/></svg>
                    <span class="text-sm font-semibold" style="color:var(--text);">${summary}</span>
                    <span class="text-[10px] px-1.5 py-0.5 rounded-full" style="background:${ri.color}22;color:${ri.color};">${ri.short}</span>
                </div>
                <span class="text-xs" style="color:var(--text-subtle);">${when}</span>
            </button>
            <button onclick="event.stopPropagation();handleDeleteCapacityRun('${id}',${run.id})" class="btn btn-icon btn-ghost btn-sm mr-3 shrink-0" style="padding:0.25rem;color:#ef4444;" title="Delete this capacity test">${svgIcon('trash')}</button>
        </div>
        <div id="cap-detail-${run.id}" style="display:${expanded ? 'block' : 'none'};" class="px-4 pb-5 pt-1 border-t border-slate-700/30">
            ${renderCapacityDetail(id, res, run.id)}
        </div>
    </div>`;
}

async function handleDeleteCapacityRun(id, runId) {
    const confirmed = await confirmModal('Delete capacity test', 'Delete this capacity test result? This cannot be undone.', { confirmLabel: 'Delete', confirmClass: 'btn-danger' });
    if (!confirmed) return;
    try {
        await api(`/api/services/${id}/capacity-probe/${runId}`, { method: 'DELETE' });
        toast('Capacity test deleted.', 'success');
        await renderCapacityRuns(id);
    } catch (err) { toast('Delete failed: ' + err.message, 'error'); }
}

function toggleCapacityRun(runId) {
    const d = document.getElementById('cap-detail-' + runId);
    const c = document.getElementById('cap-chev-' + runId);
    if (!d) return;
    const open = d.style.display !== 'none';
    d.style.display = open ? 'none' : 'block';
    if (c) c.style.transform = open ? '' : 'rotate(90deg)';
}

// capCalc updates the "instances needed" figure as the user types a target RPS.
function capCalc(safePerInstance, runId) {
    const t = parseFloat(document.getElementById('cap-target-' + runId)?.value) || 0;
    if (t <= 0) { setText('cap-instances-' + runId, '—'); return; }
    const n = safePerInstance > 0 ? Math.max(1, Math.ceil(t / safePerInstance)) : 0;
    setText('cap-instances-' + runId, n + ' instance' + (n === 1 ? '' : 's'));
}

// The detail body for one capacity run (shown inside an expanded history row):
// a plain-language summary, a "real users" translation, scaling guidance, and
// the per-level table — no nested cards, so it reads cleanly inside the row.
function renderCapacityDetail(id, res, runId) {
    const steps = res.steps || [];
    const reasonLabel = {
        throughput_plateau: 'throughput stopped growing',
        latency_degraded: 'latency climbed sharply',
        errors: 'errors started appearing',
        max_reached: 'reached the probe ceiling without saturating',
    }[res.reason] || res.reason;

    const saturated = res.reason !== 'max_reached';
    const maxRps = res.max_rps || 0;
    const knee = res.knee_concurrency || 0;
    const base = res.baseline_latency_ms || 0;
    const sat = res.saturation_latency_ms || 0;
    const kneeStep = steps.find(s => s.concurrency === knee);
    const kneeLat = kneeStep ? kneeStep.avg_latency_ms : sat;

    const headline = saturated
        ? `Saturates at ~<span style="color:var(--accent-text);">${fmt(knee)}</span> concurrent · ~<span style="color:#06b6d4;">${fmtDec(maxRps)}</span> req/sec${kneeLat > 0 ? ` · ~<span style="color:#7c3aed;">${fmtLatency(kneeLat)}</span> responses` : ''}`
        : `Handled up to ~<span style="color:#06b6d4;">${fmtDec(maxRps)}</span> req/sec without saturating`;
    const explanation = `The probe stopped because <strong style="color:var(--text);">${reasonLabel}</strong>.${saturated ? ` Beyond ~${fmt(knee)} concurrent requests, average latency rises from <strong style="color:var(--text);">${fmtLatency(base)}</strong> to <strong style="color:var(--text);">${fmtLatency(sat)}</strong> while throughput stays flat — adding more load just adds waiting.` : ''}`;

    // "In real users" — concurrent users for common activity levels, zero input.
    const roundNice = n => n >= 10000 ? Math.round(n / 1000) * 1000 : n >= 1000 ? Math.round(n / 100) * 100 : Math.round(n / 10) * 10;
    const userProfiles = [
        { label: 'Light — browsing / reading', gap: 20 },
        { label: 'Typical — active app use', gap: 6 },
        { label: 'Heavy — very interactive', gap: 2 },
    ];
    const usersSection = maxRps > 0 ? `
    <div class="mb-6">
        <h4 class="text-sm font-semibold mb-2" style="color:var(--text);">What this means in real users</h4>
        <p class="text-sm mb-3" style="color:var(--text-muted);">Roughly how many people can use it at once — it depends on how often each makes a request. A user is mostly idle, so this is far higher than the "concurrent" number. Pick the row that fits your app.</p>
        <table class="w-full text-sm">
            <thead><tr style="border-bottom:1px solid var(--border);">
                <th class="text-left py-2 pr-4 text-xs font-semibold" style="color:var(--text-muted);">User activity</th>
                <th class="text-left py-2 px-4 text-xs font-semibold" style="color:var(--text-muted);">~1 request every</th>
                <th class="text-right py-2 pl-4 text-xs font-semibold" style="color:var(--text-muted);">Concurrent users</th>
            </tr></thead>
            <tbody>${userProfiles.map(p => `<tr style="border-bottom:1px solid var(--border);">
                <td class="py-2.5 pr-4" style="color:var(--text);">${p.label}</td>
                <td class="py-2.5 px-4 font-mono" style="color:var(--text-muted);">${p.gap}s</td>
                <td class="py-2.5 pl-4 text-right font-mono font-bold" style="color:#10b981;">~${fmt(roundNice(maxRps * p.gap))}</td>
            </tr>`).join('')}</tbody>
        </table>
        <p class="text-[11px] mt-2" style="color:var(--text-faint);">Based on ~${fmtDec(maxRps)} req/sec sustained (throughput × seconds between a user's requests). Use your analytics' real rate for a precise number.</p>
    </div>` : '';

    // Scaling guidance + interactive "target RPS → instances" (per-run element ids).
    const safePer = maxRps * 0.7;
    const reasonNote = {
        latency_degraded: 'Latency climbs sharply past the knee — beyond adding instances, caching or reducing per-request work would raise each instance\'s ceiling.',
        errors: 'Errors appear past the knee — the system fails under load, not just slows. Investigate the failures before scaling up.',
        throughput_plateau: 'Throughput is capped per instance — scaling horizontally (more instances behind a load balancer) is the direct lever.',
        max_reached: 'It never saturated within the probe range, so the real ceiling is higher.',
    }[res.reason] || '';
    const guidanceSection = maxRps > 0 ? `
    <div class="mb-6">
        <h4 class="text-sm font-semibold mb-2" style="color:var(--text);">Scaling guidance</h4>
        <p class="text-sm mb-3" style="color:var(--text-muted);">One instance sustains ~<strong style="color:var(--text);">${fmtDec(maxRps)}</strong> req/sec at the knee. For headroom, plan around ~<strong style="color:var(--text);">${fmtDec(safePer)}</strong> req/sec per instance (70%).${reasonNote ? ' ' + reasonNote : ''}</p>
        <div class="flex items-center gap-2 flex-wrap text-sm" style="color:var(--text-muted);">
            <span>To handle</span>
            <input id="cap-target-${runId}" type="number" min="1" placeholder="your expected peak" oninput="capCalc(${safePer}, ${runId})" class="input-dark input-sm" style="width:150px;">
            <span>req/sec, run</span>
            <span id="cap-instances-${runId}" class="font-bold text-base" style="color:var(--accent-text);">—</span>
            <span class="text-[11px]" style="color:var(--text-faint);">(at 70% headroom)</span>
        </div>
        <p class="text-[11px] mt-2" style="color:var(--text-faint);">Enter the peak traffic you expect — from your analytics or a launch estimate.</p>
    </div>` : '';

    // Per-level ladder, knee highlighted.
    const maxStepRps = Math.max(1, ...steps.map(s => s.rps || 0));
    const rows = steps.map(s => {
        const isKnee = s.concurrency === knee && saturated;
        const pct = (s.rps / maxStepRps) * 100;
        return `<tr style="${isKnee ? 'background:rgba(124,58,237,0.10);' : ''}border-bottom:1px solid var(--border);">
            <td class="py-2 px-3 text-sm font-mono" style="color:var(--text);">${fmt(s.concurrency)}${isKnee ? ' <span class="text-[10px]" style="color:var(--accent-text);">◄ knee</span>' : ''}</td>
            <td class="py-2 px-3">
                <div class="flex items-center gap-2">
                    <div class="flex-1 h-2 rounded-full" style="background:rgba(6,182,212,0.12);"><div class="h-full rounded-full" style="width:${pct.toFixed(0)}%;background:#06b6d4;"></div></div>
                    <span class="text-xs font-mono w-16 text-right" style="color:var(--text);">${fmtDec(s.rps)}</span>
                </div>
            </td>
            <td class="py-2 px-3 text-right text-sm font-mono" style="color:var(--text);">${fmtLatency(s.avg_latency_ms)}</td>
            <td class="py-2 px-3 text-right text-sm font-mono" style="color:${(s.error_rate||0)>0.02?'#ef4444':'var(--text-muted)'};">${fmtDec((s.error_rate||0)*100)}%</td>
        </tr>`;
    }).join('');

    return `
    <div class="flex items-start justify-between gap-3 mt-2 mb-1.5">
        <div class="text-lg font-bold" style="color:var(--text);">${headline}</div>
        <button onclick="window.open('/api/services/${id}/capacity-report?run=${runId}','_blank')" class="btn btn-ghost btn-sm shrink-0" title="Open a shareable, print-ready report">${svgIcon('report')}<span>Report / PDF</span></button>
    </div>
    <p class="text-sm mb-6" style="color:var(--text-muted);">${explanation}</p>
    ${usersSection}
    ${guidanceSection}
    <div class="rounded-lg overflow-hidden border" style="border-color:var(--border);">
        <table class="w-full">
            <thead><tr style="background:var(--surface);">
                <th class="text-left py-2 px-3 text-xs font-semibold" style="color:var(--text-muted);">Concurrency</th>
                <th class="text-left py-2 px-3 text-xs font-semibold" style="color:var(--text-muted);">Throughput (req/sec)</th>
                <th class="text-right py-2 px-3 text-xs font-semibold" style="color:var(--text-muted);">Avg Latency</th>
                <th class="text-right py-2 px-3 text-xs font-semibold" style="color:var(--text-muted);">Errors</th>
            </tr></thead>
            <tbody>${rows}</tbody>
        </table>
        <div class="px-3 py-2 text-[10px]" style="color:var(--text-subtle);">Each level was held and measured at steady state. The knee is the last level before throughput flattened.</div>
    </div>`;
}

// ---- Settings Page ----

// Must match the backend's secret mask (handlers_config.go secretMask).
const SECRET_MASK = '••••••••';

// Renders a sensitive setting (webhook/password). If it's already configured
// (server returned the mask), the input starts empty with a "configured" badge
// and a placeholder; leaving it blank keeps the stored value.
function renderSecretField(name, label, help, rawValue, opts = {}) {
    const configured = rawValue === SECRET_MASK;
    const type = opts.type || 'text';
    const placeholder = configured ? 'Configured — leave blank to keep, or type to replace' : (opts.placeholder || '');
    const statusKey = opts.statusKey || '';
    return `
    <div class="mb-5">
        <div class="flex items-center gap-2 mb-1.5">
            <label class="text-xs font-medium" style="color:var(--text-muted);">${label}</label>
            ${configured ? `<span class="text-[10px] px-1.5 py-0.5 rounded-full" style="background:var(--success-weak);color:var(--success-text);">✓ configured</span>
            <button type="button" onclick="clearSecretField('${name}', this)" class="text-[10px]" style="color:var(--text-subtle);background:none;border:none;cursor:pointer;text-decoration:underline;">clear</button>` : ''}
            ${statusKey ? `<span id="test-status-${statusKey}" class="text-[10px]"></span>` : ''}
        </div>
        <input type="${type}" name="${name}" data-secret="${configured ? '1' : '0'}" class="input-dark input-sm w-full" placeholder="${esc(placeholder)}" value="">
        ${help ? `<div class="text-[11px] mt-1" style="color:var(--text-faint);">${help}</div>` : ''}
    </div>`;
}

async function handleSendTestNotification(btn) {
    btn?.classList.add('loading');
    // Clear previous inline statuses.
    ['webhook', 'slack', 'teams', 'discord', 'email'].forEach(k => {
        const el = document.getElementById('test-status-' + k);
        if (el) el.textContent = '';
    });
    try {
        const res = await api('/api/settings/test-notification', { method: 'POST' });
        if (!res || res.attempted === 0) {
            toast(res?.message || 'No channels configured. Save a webhook or email first.', 'info');
        } else {
            const results = res.results || {};
            Object.entries(results).forEach(([k, v]) => {
                const el = document.getElementById('test-status-' + k);
                if (el) {
                    const okv = v === 'sent';
                    el.textContent = okv ? '✓ sent' : '✗ failed';
                    el.style.color = okv ? 'var(--success-text)' : 'var(--danger-text)';
                    el.title = okv ? '' : v;
                }
            });
            const okCount = Object.values(results).filter(v => v === 'sent').length;
            const failCount = Object.values(results).length - okCount;
            toast(`Test notification: ${okCount} sent${failCount ? `, ${failCount} failed` : ''}.`, failCount ? 'error' : 'success');
        }
    } catch (err) {
        toast('Test failed: ' + err.message, 'error');
    }
    btn?.classList.remove('loading');
}

async function handleTestWorkers(btn) {
    const urls = document.querySelector('[name="worker_urls"]')?.value?.trim() || '';
    const out = document.getElementById('worker-test-results');
    if (!urls) { if (out) out.innerHTML = '<span class="text-[11px]" style="color:var(--text-subtle);">Enter worker URLs first.</span>'; return; }
    btn?.classList.add('loading');
    try {
        const res = await api('/api/workers/test', { method: 'POST', body: JSON.stringify({ urls }) });
        if (out) out.innerHTML = (res || []).map(w => {
            const ok = w.status === 'healthy';
            const color = ok ? 'var(--success-text)' : 'var(--danger-text)';
            return `<div class="flex items-center justify-between text-xs py-1"><span class="font-mono" style="color:var(--text-muted);">${esc(w.url)}</span><span style="color:${color};">${ok ? '✓ reachable' : '✗ unreachable'}</span></div>`;
        }).join('');
    } catch (err) {
        if (out) out.innerHTML = `<span class="text-[11px]" style="color:var(--danger-text);">${esc(err.message)}</span>`;
    }
    btn?.classList.remove('loading');
}

async function handlePurgeNow(btn) {
    const ok = await confirmModal('Purge old results', 'Delete stored results older than the saved retention window now? This cannot be undone.', { confirmLabel: 'Purge', confirmClass: 'btn-danger' });
    if (!ok) return;
    btn?.classList.add('loading');
    try {
        const res = await api('/api/settings/purge-now', { method: 'POST' });
        if (res && res.message) toast(res.message, 'info');
        else toast(`Purged ${res?.deleted || 0} old result(s).`, 'success');
    } catch (err) {
        toast('Purge failed: ' + err.message, 'error');
    }
    btn?.classList.remove('loading');
}

// Mark a configured secret to be cleared on save.
function clearSecretField(name, btn) {
    const input = document.querySelector(`[name="${name}"]`);
    if (!input) return;
    input.dataset.secret = '0';
    input.dataset.clear = '1';
    input.placeholder = 'Will be cleared on save';
    input.value = '';
    btn.remove();
    const badge = input.parentElement.querySelector('span');
    if (badge) badge.remove();
}

async function renderSettingsPage() {
    try { state.settings = (await api('/api/settings')) || {}; } catch (_) { state.settings = {}; }
    const s = state.settings;

    // Fetch version info
    let versionInfo = '';
    try {
        const v = await api('/api/version');
        versionInfo = `<div class="mt-8 text-center text-xs" style="color:var(--text-faint);">gload ${esc(v.version)} | ${esc(v.go)} | ${esc(v.os)}/${esc(v.arch)}</div>`;
    } catch(_) {}

    // Fetch workers
    let workersHtml = '';
    try {
        const workers = await api('/api/workers');
        if (workers && workers.length > 0) {
            workersHtml = `
            <h3 class="text-lg font-bold mt-8 mb-4" style="color:var(--text);">Distributed Workers</h3>
            <div class="card p-6">
                <div class="space-y-3">
                    ${workers.map(w => `
                    <div class="flex items-center justify-between py-2 border-b border-slate-700/20">
                        <span class="text-sm font-mono" style="color:var(--text);">${esc(w.url || w)}</span>
                        <span class="text-xs px-2 py-0.5 rounded-full" style="background:${w.healthy ? 'rgba(16,185,129,0.15);color:#10b981' : 'rgba(239,68,68,0.15);color:#ef4444'};">${w.healthy ? 'Healthy' : 'Unreachable'}</span>
                    </div>`).join('')}
                </div>
                <p class="text-[11px] mt-3" style="color:var(--text-faint);">Configure workers in Settings: worker_urls (comma-separated)</p>
            </div>`;
        }
    } catch(_) {}

    return `
    ${breadcrumb({ label: 'Dashboard', href: '#/' }, { label: 'Settings' })}

    <div class="max-w-2xl">
        <h2 class="text-2xl font-bold mb-1" style="color:var(--text);">Settings</h2>
        <p class="text-sm mb-6" style="color:var(--text-muted);">Configure default load test parameters.</p>

        <div class="card p-6" id="settings-form">
            <div class="grid grid-cols-2 gap-5 mb-5">
                <div>
                    <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Default Concurrency</label>
                    <input type="number" name="default_concurrency" class="input-dark input-sm" min="1" max="10000" value="${parseInt(s.default_concurrency) || 10}">
                    <div class="text-[11px] mt-1" style="color:var(--text-faint);">Parallel workers (1-10,000)</div>
                </div>
                <div>
                    <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Default RPS Limit</label>
                    <input type="number" name="default_rps_limit" class="input-dark input-sm" min="0" max="100000" value="${parseInt(s.default_rps_limit) || 0}">
                    <div class="text-[11px] mt-1" style="color:var(--text-faint);">0 = unlimited</div>
                </div>
            </div>
            <div class="grid grid-cols-2 gap-5 mb-5">
                <div>
                    <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Default Duration (seconds)</label>
                    <input type="number" name="default_duration" class="input-dark input-sm" min="1" max="3600" value="${parseDurationToSeconds(s.default_duration || '10s')}">
                    <div class="text-[11px] mt-1" style="color:var(--text-faint);">Test duration in seconds (1-3600)</div>
                </div>
                <div>
                    <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Default Timeout (seconds)</label>
                    <input type="number" name="default_timeout" class="input-dark input-sm" min="1" max="300" value="${parseDurationToSeconds(s.default_timeout || '30s')}">
                    <div class="text-[11px] mt-1" style="color:var(--text-faint);">Per-request timeout in seconds (1-300)</div>
                </div>
            </div>

            <div class="border-t border-slate-700/40 pt-5 mt-5">
                <h4 class="text-xs font-semibold mb-3" style="color:var(--text-muted);">Data Retention</h4>
                <div>
                    <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Keep results for (days)</label>
                    <div class="flex items-center gap-2">
                        <input type="number" name="retention_days" class="input-dark input-sm" min="0" max="3650" step="1" value="${parseInt(s.retention_days) || 0}" style="width:120px;">
                        <button type="button" onclick="handlePurgeNow(this)" class="btn btn-ghost btn-sm" style="white-space:nowrap;">Purge old results now</button>
                    </div>
                    <div class="text-[11px] mt-1" style="color:var(--text-faint);">0 = keep forever. Old results are automatically deleted hourly. Save the retention value before purging.</div>
                </div>
            </div>

            <div class="mb-5">
                <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Distributed Worker URLs</label>
                <div class="flex items-center gap-2">
                    <input type="text" name="worker_urls" class="input-dark input-sm" placeholder="http://worker1:8081, http://worker2:8081" value="${esc(s.worker_urls || '')}">
                    <button type="button" onclick="handleTestWorkers(this)" class="btn btn-ghost btn-sm" style="white-space:nowrap;">Test connection</button>
                </div>
                <div id="worker-test-results" class="mt-2"></div>
                <div class="text-[11px] mt-1" style="color:var(--text-faint);">Comma-separated worker node URLs for distributed testing. Leave empty if not using.</div>
            </div>

        </div>

        <h3 class="text-lg font-bold mt-8 mb-1" style="color:var(--text);">Notifications</h3>
        <p class="text-sm mb-4" style="color:var(--text-muted);">Configure webhook notifications for test results.</p>

        <div class="card p-6" id="notifications-form">
            ${renderSecretField('webhook_url', 'Webhook URL', 'POST request with test results JSON will be sent to this URL', s.webhook_url, { placeholder: 'https://example.com/webhook', statusKey: 'webhook' })}
            ${renderSecretField('slack_webhook_url', 'Slack Webhook URL', 'Slack incoming webhook URL for formatted notifications', s.slack_webhook_url, { placeholder: 'https://hooks.slack.com/services/...', statusKey: 'slack' })}
            ${renderSecretField('teams_webhook_url', 'Microsoft Teams Webhook URL', 'Teams incoming webhook URL for card-formatted notifications', s.teams_webhook_url, { placeholder: 'https://outlook.office.com/webhook/...', statusKey: 'teams' })}
            ${renderSecretField('discord_webhook_url', 'Discord Webhook URL', 'Discord incoming webhook for embed notifications', s.discord_webhook_url, { placeholder: 'https://discord.com/api/webhooks/...', statusKey: 'discord' })}
            <div class="border-t border-slate-700/40 pt-5 mt-5">
                <div class="flex items-center gap-2 mb-3">
                    <h4 class="text-xs font-semibold" style="color:var(--text-muted);">Email Notifications</h4>
                    <span id="test-status-email" class="text-[10px]"></span>
                </div>
                <div class="grid grid-cols-2 gap-4 mb-4">
                    <div>
                        <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">SMTP Host</label>
                        <input type="text" name="smtp_host" class="input-dark input-sm" placeholder="smtp.gmail.com" value="${esc(s.smtp_host || '')}">
                    </div>
                    <div>
                        <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">SMTP Port</label>
                        <input type="number" name="smtp_port" class="input-dark input-sm" placeholder="587" value="${esc(s.smtp_port || '587')}" min="1" max="65535" step="1">
                    </div>
                </div>
                <div class="grid grid-cols-2 gap-4 mb-4">
                    <div>
                        <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">SMTP Username</label>
                        <input type="text" name="smtp_username" class="input-dark input-sm" placeholder="user@gmail.com" value="${esc(s.smtp_username || '')}">
                    </div>
                    ${renderSecretField('smtp_password', 'SMTP Password', '', s.smtp_password, { type: 'password', placeholder: 'App password' })}
                </div>
                <div class="grid grid-cols-2 gap-4 mb-4">
                    <div>
                        <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">From Address</label>
                        <input type="email" name="email_from" class="input-dark input-sm" placeholder="gload@yourcompany.com" value="${esc(s.email_from || '')}">
                    </div>
                    <div>
                        <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">To Addresses</label>
                        <input type="text" name="email_to" class="input-dark input-sm" placeholder="team@company.com, lead@company.com" value="${esc(s.email_to || '')}">
                        <div class="text-[11px] mt-1" style="color:var(--text-faint);">Comma-separated email addresses</div>
                    </div>
                </div>
            </div>
            <div class="mb-5">
                <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Notify On</label>
                <select name="notify_on" class="input-dark input-sm">
                    <option value="none"${(s.notify_on || 'none') === 'none' ? ' selected' : ''}>None</option>
                    <option value="all"${s.notify_on === 'all' ? ' selected' : ''}>All tests</option>
                    <option value="fail_only"${s.notify_on === 'fail_only' ? ' selected' : ''}>Failed tests only</option>
                </select>
                <div class="text-[11px] mt-1" style="color:var(--text-faint);"><strong>None</strong>: never notify · <strong>All tests</strong>: after every run · <strong>Failed tests only</strong>: only when an assertion fails.</div>
            </div>

            <div class="flex items-center gap-3 pt-4 border-t border-slate-700/40">
                <button onclick="handleSendTestNotification(this)" class="btn btn-ghost btn-sm">${svgIcon('bolt')}<span>Send test notification</span></button>
                <span class="text-[11px]" style="color:var(--text-faint);">Uses your saved channels — save first, then test.</span>
            </div>
        </div>

        <div class="flex items-center gap-3 mt-6">
            <button onclick="handleSaveSettings()" id="settings-save-btn" class="btn btn-primary">
                <span class="btn-label">Save Settings</span>
                <span class="spinner"></span>
            </button>
            <span class="text-xs" style="color:var(--text-subtle);">Saves everything on this page.</span>
        </div>

        ${workersHtml}
        ${versionInfo}
    </div>`;
}

// ---- Queue Page ----

async function renderQueuePage() {
    await refreshQueue();
    const q = state.queue || {};
    if (q.running || (q.items && q.items.length > 0)) startQueuePolling();

    const cur = q.current;
    const runningHtml = (q.running && cur) ? `
    <div class="card p-5 mb-6" style="border-color:rgba(16,185,129,0.3);">
        <div class="flex items-center justify-between">
            <div class="flex items-center gap-3">
                <div class="pulse-dot"></div>
                <h3 class="text-sm font-semibold" style="color:#10b981;">Currently Running</h3>
            </div>
            <a href="#/services/${cur.service_id}/run" class="btn btn-ghost btn-sm">View Live</a>
        </div>
        <div class="text-sm font-mono mt-2" style="color:var(--text);">${esc(cur.name)}</div>
    </div>` : '';

    const pending = q.items || [];
    const pendingHtml = pending.length > 0 ? `
    <div class="card overflow-hidden">
        <table class="w-full text-sm">
            <thead>
                <tr class="border-b border-slate-700/50" style="background:var(--bg);">
                    <th class="text-left py-3 px-4 text-xs font-medium" style="color:var(--text-muted);width:48px;">#</th>
                    <th class="text-left py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">Service</th>
                    <th class="text-right py-3 px-4 text-xs font-medium" style="color:var(--text-muted);width:140px;">Actions</th>
                </tr>
            </thead>
            <tbody>${pending.map((item, i) => `
                <tr class="border-b border-slate-700/30 hover-row">
                    <td class="py-3 px-4 text-xs font-mono" style="color:var(--text-subtle);">${i + 1}</td>
                    <td class="py-3 px-4 text-sm">
                        <a href="#/services/${item.service_id}" class="hover:text-violet-300 transition-colors" style="color:var(--text);text-decoration:none;">${esc(item.name || item.service_id)}</a>
                    </td>
                    <td class="py-2 px-4">
                        <div class="flex items-center justify-end gap-1">
                            <button onclick="moveQueueItem(${item.id}, -1)" ${i === 0 ? 'disabled' : ''} aria-label="Move up" title="Move up"
                                    class="btn btn-icon btn-ghost btn-sm" style="padding:0.3rem;${i === 0 ? 'opacity:0.3;' : ''}">
                                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 15l7-7 7 7"/></svg>
                            </button>
                            <button onclick="moveQueueItem(${item.id}, 1)" ${i === pending.length - 1 ? 'disabled' : ''} aria-label="Move down" title="Move down"
                                    class="btn btn-icon btn-ghost btn-sm" style="padding:0.3rem;${i === pending.length - 1 ? 'opacity:0.3;' : ''}">
                                <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/></svg>
                            </button>
                            <button onclick="removeQueueItem(${item.id})" aria-label="Remove from queue" title="Remove"
                                    class="btn btn-icon btn-ghost btn-sm" style="padding:0.3rem;color:#ef4444;">${svgIcon('x')}</button>
                        </div>
                    </td>
                </tr>`).join('')}
            </tbody>
        </table>
    </div>` : `<div class="card p-8 text-center"><p class="text-sm" style="color:var(--text-subtle);">Queue is empty.</p></div>`;

    return `
    ${breadcrumb({ label: 'Dashboard', href: '#/' }, { label: 'Queue' })}

    <div class="flex items-center justify-between mb-8">
        <div>
            <h2 class="text-2xl font-bold" style="color:var(--text);">Test Queue</h2>
            <p class="text-sm mt-1" style="color:var(--text-muted);">Manage queued load tests.</p>
        </div>
        <button onclick="handleClearQueue()" class="btn btn-ghost btn-sm">${svgIcon('trash')}<span>Clear Queue</span></button>
    </div>

    ${runningHtml}

    <h3 class="text-base font-semibold mb-4" style="color:var(--text);">Pending (${pending.length})</h3>
    ${pendingHtml}`;
}

// ---- Schedules Page ----

async function renderSchedulesPage() {
    let schedules = [];
    try { schedules = (await api('/api/schedules')) || []; } catch (_) {}
    state.schedules = schedules;

    const cronPresets = [
        { label: 'Every hour', value: '0 * * * *' },
        { label: 'Daily at 3am', value: '0 3 * * *' },
        { label: 'Every 6 hours', value: '0 */6 * * *' },
        { label: 'Weekly Monday 9am', value: '0 9 * * 1' },
    ];

    const serviceOptions = state.services.map(s =>
        `<option value="${s.id}">${esc(s.name)}</option>`
    ).join('');

    const scheduleRows = schedules.length > 0 ? schedules.map(s => {
        const svc = state.services.find(sv => sv.id == s.service_id);
        const svcName = svc ? svc.name : `Service #${s.service_id}`;
        const enabledColor = s.enabled ? '#10b981' : 'var(--text-subtle)';
        const enabledLabel = s.enabled ? 'Enabled' : 'Disabled';
        return `
        <tr class="border-b border-slate-700/30">
            <td class="py-3 px-4 text-sm" style="color:var(--text);">${esc(svcName)}</td>
            <td class="py-3 px-4">
                <div class="text-sm font-mono" style="color:var(--accent-text);">${esc(s.cron_expr)}</div>
                <div class="text-[10px]" style="color:var(--text-subtle);">${cronToHuman(s.cron_expr)}</div>
            </td>
            <td class="py-3 px-4 text-center">
                <label class="toggle-switch toggle-sm">
                    <input type="checkbox" ${s.enabled ? 'checked' : ''} onchange="handleToggleSchedule('${s.id}', this.checked)">
                    <span class="toggle-slider"></span>
                </label>
            </td>
            <td class="py-3 px-4 text-xs font-mono" style="color:var(--text-muted);">${s.last_run ? fmtRelativeTime(s.last_run) : '-'}</td>
            <td class="py-3 px-4 text-xs font-mono" style="color:var(--text-muted);">${s.next_run ? fmtDate(s.next_run) : '-'}</td>
            <td class="py-3 px-4 text-right">
                <div class="flex items-center justify-end gap-1">
                    <button onclick="handleRunSchedule('${s.service_id}')" class="btn btn-icon btn-ghost btn-sm" style="padding:0.35rem;color:#10b981;" title="Run now" aria-label="Run now">${svgIcon('play')}</button>
                    <button onclick="startEditSchedule('${s.id}','${s.service_id}','${esc(s.cron_expr)}')" class="btn btn-icon btn-ghost btn-sm" style="padding:0.35rem;" title="Edit schedule" aria-label="Edit schedule">${svgIcon('edit')}</button>
                    <button onclick="handleDeleteSchedule('${s.id}')" class="btn btn-icon btn-ghost btn-sm" style="padding:0.35rem;color:#ef4444;" title="Delete schedule" aria-label="Delete schedule">${svgIcon('trash')}</button>
                </div>
            </td>
        </tr>`;
    }).join('') : `<tr><td colspan="6" class="py-8 text-center text-sm" style="color:var(--text-subtle);">No schedules configured.</td></tr>`;

    return `
    ${breadcrumb({ label: 'Dashboard', href: '#/' }, { label: 'Schedules' })}

    <div class="flex items-center justify-between mb-8">
        <div>
            <h2 class="text-2xl font-bold" style="color:var(--text);">Schedules</h2>
            <p class="text-sm mt-1" style="color:var(--text-muted);">Automate recurring load tests with cron expressions.</p>
        </div>
        <button onclick="toggleScheduleForm()" id="add-schedule-btn" class="btn btn-primary btn-sm" ${state.services.length === 0 ? 'disabled title="Create a service first"' : ''}>
            ${svgIcon('plus')}<span>Add Schedule</span>
        </button>
    </div>

    ${state.services.length === 0 ? `
    <div class="card p-8 text-center mb-6">
        <p class="text-sm mb-3" style="color:var(--text-muted);">You need at least one service before you can schedule a test.</p>
        <a href="#/services/new" class="btn btn-primary btn-sm" style="display:inline-flex;">${svgIcon('plus')}<span>Create a Service</span></a>
    </div>` : ''}

    <!-- Add / Edit Schedule Form (hidden by default) -->
    <div id="schedule-form-wrap" style="display:none;" class="card p-5 mb-6">
        <h3 class="text-sm font-semibold mb-4" id="schedule-form-title" style="color:var(--text);">New Schedule</h3>
        <div class="grid grid-cols-2 gap-4 mb-4">
            <div>
                <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Service</label>
                <select id="schedule-service" class="input-dark input-sm">
                    ${serviceOptions}
                </select>
            </div>
            <div>
                <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Cron Expression</label>
                <input type="text" id="schedule-cron" class="input-dark input-sm font-mono" placeholder="0 3 * * *" value="" oninput="updateCronPreview()">
                <div class="text-[11px] mt-1" id="cron-preview" style="color:var(--text-faint);">minute hour day month weekday</div>
            </div>
        </div>
        <div class="mb-4">
            <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Quick Presets</label>
            <div class="flex flex-wrap gap-2">
                ${cronPresets.map(p => `
                    <button onclick="document.getElementById('schedule-cron').value='${p.value}';updateCronPreview();"
                            class="px-3 py-1.5 rounded-lg text-xs font-medium transition-colors"
                            style="background:rgba(124,58,237,0.12);color:var(--accent-text);border:1px solid rgba(124,58,237,0.2);">
                        ${esc(p.label)}
                    </button>`).join('')}
            </div>
        </div>
        <div class="flex items-center gap-3 pt-3 border-t border-slate-700/40">
            <button onclick="handleCreateSchedule()" id="schedule-submit-btn" class="btn btn-primary btn-sm">Create Schedule</button>
            <button onclick="cancelScheduleForm()" class="btn btn-ghost btn-sm">Cancel</button>
        </div>
    </div>

    <!-- Schedule List -->
    <div class="card overflow-hidden">
        <table class="w-full text-sm">
            <thead>
                <tr class="border-b border-slate-700/50" style="background:var(--bg);">
                    <th class="text-left py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">Service</th>
                    <th class="text-left py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">Schedule</th>
                    <th class="text-center py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">Status</th>
                    <th class="text-left py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">Last Run</th>
                    <th class="text-left py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">Next Run</th>
                    <th class="text-right py-3 px-4 text-xs font-medium" style="color:var(--text-muted);"></th>
                </tr>
            </thead>
            <tbody>${scheduleRows}</tbody>
        </table>
    </div>`;
}

let editingScheduleId = null;

function toggleScheduleForm() {
    const wrap = document.getElementById('schedule-form-wrap');
    if (wrap) wrap.style.display = wrap.style.display === 'none' ? 'block' : 'none';
}

function cancelScheduleForm() {
    editingScheduleId = null;
    const wrap = document.getElementById('schedule-form-wrap');
    if (wrap) wrap.style.display = 'none';
    // Reset form to "create" mode.
    const title = document.getElementById('schedule-form-title');
    const btn = document.getElementById('schedule-submit-btn');
    const svc = document.getElementById('schedule-service');
    if (title) title.textContent = 'New Schedule';
    if (btn) btn.textContent = 'Create Schedule';
    if (svc) svc.disabled = false;
}

// Live human-readable preview of the cron expression as the user types.
function updateCronPreview() {
    const input = document.getElementById('schedule-cron');
    const preview = document.getElementById('cron-preview');
    if (!input || !preview) return;
    const v = input.value.trim();
    if (!v) { preview.textContent = 'minute hour day month weekday'; return; }
    const human = cronToHuman(v);
    preview.textContent = human === v ? 'minute hour day month weekday' : human;
}

function startEditSchedule(id, serviceId, cronExpr) {
    editingScheduleId = id;
    const wrap = document.getElementById('schedule-form-wrap');
    if (wrap) wrap.style.display = 'block';
    const title = document.getElementById('schedule-form-title');
    const btn = document.getElementById('schedule-submit-btn');
    const svc = document.getElementById('schedule-service');
    const cron = document.getElementById('schedule-cron');
    if (title) title.textContent = 'Edit Schedule';
    if (btn) btn.textContent = 'Update Schedule';
    if (svc) { svc.value = serviceId; svc.disabled = true; } // service can't change on edit
    if (cron) cron.value = cronExpr;
    updateCronPreview();
    if (wrap) wrap.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

async function handleRunSchedule(serviceId) {
    await handleRunService(serviceId); // reuse the direct-run flow
}


// ============================================
// Event Handlers
// ============================================

async function handleSaveService() {
    const form = document.getElementById('service-form');
    if (!form) return;
    if (!validateServiceForm(form)) { jumpToFirstError(form); return; }

    const btn = document.getElementById('save-btn');
    if (btn?.disabled) return; // prevent double-submit
    if (btn) { btn.disabled = true; btn.classList.add('loading'); }

    const editId = form.dataset.id;
    const headers = {};
    form.querySelectorAll('.header-row').forEach(row => {
        const k = row.querySelector('.header-key')?.value?.trim();
        const v = row.querySelector('.header-val')?.value?.trim() || '';
        if (k) headers[k] = v;
    });

    const tagsRaw = form.querySelector('[name="tags"]')?.value || '';
    const tags = tagsRaw.split(',').map(t => t.trim()).filter(Boolean).join(',');
    const groupName = form.querySelector('[name="group_name"]')?.value?.trim() || '';

    // Collect assertions
    const assertions = [];
    form.querySelectorAll('.assertion-row').forEach(row => {
        const metric = row.querySelector('.assertion-metric')?.value;
        const operator = row.querySelector('.assertion-operator')?.value;
        const value = parseFloat(row.querySelector('.assertion-value')?.value);
        if (metric && operator && !isNaN(value)) {
            assertions.push({ metric, operator, value });
        }
    });

    // Collect and validate profiles
    const profiles = [];
    let profileError = false;
    form.querySelectorAll('.profile-row').forEach(row => {
        const name = row.querySelector('.profile-name')?.value?.trim();
        const concurrency = parseInt(row.querySelector('.profile-concurrency')?.value, 10);
        const durationSec = parseInt(row.querySelector('.profile-duration')?.value, 10);
        const rps = parseInt(row.querySelector('.profile-rps')?.value, 10) || 0;
        if (!name && !concurrency && !durationSec) return; // empty row, skip
        if (!name) { profileError = 'Profile name is required.'; return; }
        if (!concurrency || concurrency < 1 || concurrency > 10000) { profileError = `Profile "${name}": concurrency must be 1-10,000.`; return; }
        if (!durationSec || durationSec < 1 || durationSec > 3600) { profileError = `Profile "${name}": duration must be 1-3600 seconds.`; return; }
        if (rps < 0) { profileError = `Profile "${name}": RPS must be 0 or more.`; return; }
        profiles.push({ name, concurrency, duration: durationSec + 's', rps });
    });
    if (profileError) { toast(profileError, 'error'); return; }

    // Cookie jar
    const cookieJar = form.querySelector('[name="cookie_jar"]')?.checked ? 1 : 0;

    // Collect steps (scenario)
    const steps = collectSteps();

    const data = {
        name: form.querySelector('[name="name"]').value.trim(),
        url: form.querySelector('[name="url"]').value.trim(),
        method: form.querySelector('[name="method"]').value,
        headers, body: form.querySelector('[name="body"]')?.value || '',
        concurrency: parseInt(form.querySelector('[name="concurrency"]').value, 10) || 10,
        duration: (parseInt(form.querySelector('[name="duration"]').value, 10) || 10) + 's',
        timeout: (parseInt(form.querySelector('[name="timeout"]').value, 10) || 30) + 's',
        tags,
        group_name: groupName,
        assertions: JSON.stringify(assertions),
        profiles: JSON.stringify(profiles),
        steps: JSON.stringify(steps),
        cookie_jar: cookieJar,
        arrival_rate: state.loadMode === 'simple' ? 0 : (function() {
            let v = 0;
            form.querySelectorAll('[name="arrival_rate"]').forEach(inp => { if (!inp.disabled) v = parseInt(inp.value, 10) || 0; });
            return v;
        })(),
        think_time_ms: (function() {
            let v = 0;
            form.querySelectorAll('[name="think_time_ms"]').forEach(inp => { if (inp.offsetParent !== null) v = parseInt(inp.value, 10) || 0; });
            return v;
        })(),
        think_time_max_ms: (function() {
            let v = 0;
            form.querySelectorAll('[name="think_time_max_ms"]').forEach(inp => { if (inp.offsetParent !== null) v = parseInt(inp.value, 10) || 0; });
            return v;
        })(),
        warmup_seconds: (function() {
            let v = 0;
            form.querySelectorAll('[name="warmup_seconds"]').forEach(inp => { if (inp.offsetParent !== null) v = parseInt(inp.value, 10) || 0; });
            return v;
        })(),
        warmup_conns: parseInt(form.querySelector('[name="warmup_conns"]')?.value, 10) || 0,
        adaptive_concurrency: form.querySelector('[name="adaptive_concurrency"]')?.checked ? 1 : 0,
        adaptive_target_ms: parseFloat(form.querySelector('[name="adaptive_target_ms"]')?.value) || 500,
        requests_per_iteration: parseInt(form.querySelector('[name="requests_per_iteration"]')?.value, 10) || 1,
        http2: form.querySelector('[name="http2"]')?.checked ? 1 : 0,
        dns_cache: form.querySelector('[name="dns_cache"]')?.checked ? 1 : 0,
        disable_keep_alive: form.querySelector('[name="disable_keep_alive"]')?.checked ? 1 : 0,
        max_idle_conns: parseInt(form.querySelector('[name="max_idle_conns"]')?.value, 10) || 100,
        validations: JSON.stringify(collectValidations()),
        protocol: form.querySelector('[name="protocol"]')?.value || 'http',
        protocol_config: form.querySelector('[name="protocol_config"]')?.value?.trim() || '{}',
        content_type: form.querySelector('[name="content_type"]')?.value || 'json',
        data_source: form.querySelector('[name="data_source"]')?.value?.trim() || '[]',
        form_fields: JSON.stringify(collectFormFields()),
        workspace_id: parseInt(form.querySelector('[name="workspace_id"]')?.value, 10) || 0,
    };

    try {
        const saved = editId
            ? await api(`/api/services/${editId}`, { method: 'PUT', body: JSON.stringify(data) })
            : await api('/api/services', { method: 'POST', body: JSON.stringify(data) });
        clearFormDirty();
        toast(editId ? 'Service updated.' : 'Service created.', 'success');
        navigate(`/services/${saved.id}`);
    } catch (err) {
        toast('Failed to save: ' + err.message, 'error');
        if (btn) { btn.disabled = false; btn.classList.remove('loading'); }
    }
}

function handleEditService(id) {
    navigate(`/services/${id}/edit`);
}

async function handleRunTest(id) {
    const btn = document.getElementById('run-btn');
    btn?.classList.add('loading');
    try {
        await api(`/api/services/${id}/run`, { method: 'POST' });
        state.testResults = null;
        state.liveTimeSeries = [];
        navigate(`/services/${id}/run`);
    } catch (err) {
        // Already running → just open the live view instead of erroring.
        if (/already running/i.test(err.message)) {
            navigate(`/services/${id}/run`);
            return;
        }
        toast('Failed to start: ' + err.message, 'error');
        btn?.classList.remove('loading');
    }
}

async function handleRunProfile(id, profileIndex) {
    try {
        await api(`/api/services/${id}/run-profile`, {
            method: 'POST',
            body: JSON.stringify({ profile_index: profileIndex }),
        });
        state.testResults = null;
        state.liveTimeSeries = [];
        navigate(`/services/${id}/run`);
    } catch (err) {
        toast('Failed to start profile test: ' + err.message, 'error');
    }
}

async function handleStopTest(id) {
    const btn = document.getElementById('stop-btn');
    if (btn) {
        btn.disabled = true;
        btn.style.opacity = '0.6';
        btn.style.cursor = 'default';
        const lbl = btn.querySelector('span');
        if (lbl) lbl.textContent = 'Stopping…';
    }
    // Optimistically reflect the pending stop in the status line.
    setHtml('live-status', '<span style="color:#f59e0b;">Stopping…</span>');
    try {
        await api(`/api/services/${id}/stop`, { method: 'POST' });
        toast('Stopping test…', 'info');
    } catch (err) {
        console.error('Stop error:', err);
        toast('Failed to stop: ' + err.message, 'error');
        if (btn) {
            btn.disabled = false;
            btn.style.opacity = '';
            btn.style.cursor = '';
            const lbl = btn.querySelector('span');
            if (lbl) lbl.textContent = 'Stop Test';
        }
    }
}

async function handleDeleteService(id) {
    const svc = state.services.find(s => s.id == id);
    const confirmed = await confirmModal('Delete Service', `Are you sure you want to delete "${svc?.name || 'this service'}"? This action cannot be undone.`);
    if (!confirmed) return;
    try {
        await api(`/api/services/${id}`, { method: 'DELETE' });
        toast('Service deleted.', 'success');
        router();
    } catch (err) { toast('Delete failed: ' + err.message, 'error'); }
}

async function handleCloneService(id) {
    try {
        const cloned = await api(`/api/services/${id}/clone`, { method: 'POST' });
        toast('Service cloned.', 'success');
        navigate(`/services/${cloned.id}`);
    } catch (err) { toast('Clone failed: ' + err.message, 'error'); }
}

function toggleMoreActions() {
    const dd = document.getElementById('more-actions-dropdown');
    if (dd) dd.style.display = dd.style.display === 'none' ? 'block' : 'none';
}

async function handleRunDistributed(id) {
    try {
        await api(`/api/services/${id}/run-distributed`, { method: 'POST' });
        toast('Distributed test started.', 'success');
        navigate(`/services/${id}/run`);
    } catch (err) { toast('Failed: ' + err.message, 'error'); }
}

async function handleGitHubComment(id) {
    try {
        await api(`/api/services/${id}/github-comment`, { method: 'POST' });
        toast('Posted to GitHub PR.', 'success');
    } catch (err) { toast('Failed: ' + err.message, 'error'); }
}

document.addEventListener('click', function(e) {
    const wrap = document.getElementById('more-actions-wrap');
    if (wrap && !wrap.contains(e.target)) {
        const dd = document.getElementById('more-actions-dropdown');
        if (dd) dd.style.display = 'none';
    }
});

async function handleSaveSettings() {
    const form = document.getElementById('settings-form');
    if (!form) return;
    const btn = document.getElementById('settings-save-btn');

    // Validate
    const conc = parseInt(form.querySelector('[name="default_concurrency"]')?.value, 10);
    const dur = parseInt(form.querySelector('[name="default_duration"]')?.value, 10);
    const tout = parseInt(form.querySelector('[name="default_timeout"]')?.value, 10);
    const rps = parseInt(form.querySelector('[name="default_rps_limit"]')?.value, 10);

    if (!conc || conc < 1 || conc > 10000) { toast('Concurrency must be 1-10,000.', 'error'); return; }
    if (!dur || dur < 1 || dur > 3600) { toast('Duration must be 1-3600 seconds.', 'error'); return; }
    if (!tout || tout < 1 || tout > 300) { toast('Timeout must be 1-300 seconds.', 'error'); return; }
    if (isNaN(rps) || rps < 0) { toast('RPS limit must be 0 or more.', 'error'); return; }

    // Retention validation
    const ret = parseInt(form.querySelector('[name="retention_days"]')?.value, 10);
    if (isNaN(ret) || ret < 0 || ret > 3650) { toast('Retention must be 0-3650 days.', 'error'); return; }

    // URL validations (only if filled)
    const nForm = document.getElementById('notifications-form');
    const urlPattern = /^https?:\/\/.+/;
    const urlFields = ['webhook_url', 'slack_webhook_url', 'teams_webhook_url', 'discord_webhook_url'];
    for (const field of urlFields) {
        const val = nForm?.querySelector(`[name="${field}"]`)?.value?.trim() || '';
        if (val && !urlPattern.test(val)) { toast(`${field.replace(/_/g, ' ')}: must start with http:// or https://`, 'error'); return; }
    }

    // Worker URLs validation
    const workerUrls = form.querySelector('[name="worker_urls"]')?.value?.trim() || '';
    if (workerUrls) {
        const urls = workerUrls.split(',').map(u => u.trim()).filter(Boolean);
        for (const u of urls) {
            if (!urlPattern.test(u)) { toast(`Invalid worker URL: ${u}`, 'error'); return; }
        }
    }

    // SMTP validation (if host is filled, port must be valid)
    const smtpHost = nForm?.querySelector('[name="smtp_host"]')?.value?.trim() || '';
    const smtpPort = parseInt(nForm?.querySelector('[name="smtp_port"]')?.value, 10);
    if (smtpHost && (isNaN(smtpPort) || smtpPort < 1 || smtpPort > 65535)) { toast('SMTP port must be 1-65535.', 'error'); return; }

    // Email validation (if to is filled, from must also be filled)
    const emailFrom = nForm?.querySelector('[name="email_from"]')?.value?.trim() || '';
    const emailTo = nForm?.querySelector('[name="email_to"]')?.value?.trim() || '';
    const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    if (emailTo && !emailFrom) { toast('From address is required when To is set.', 'error'); return; }
    if (emailFrom && !emailPattern.test(emailFrom)) { toast('Invalid From email address.', 'error'); return; }
    if (emailTo) {
        const addrs = emailTo.split(',').map(e => e.trim()).filter(Boolean);
        for (const addr of addrs) {
            if (!emailPattern.test(addr)) { toast(`Invalid email: ${addr}`, 'error'); return; }
        }
    }

    btn?.classList.add('loading');

    const data = {
        default_concurrency: String(conc),
        default_duration: dur + 's',
        default_timeout: tout + 's',
        default_rps_limit: String(rps),
        retention_days: form.querySelector('[name="retention_days"]')?.value?.trim() || '0',
        worker_urls: form.querySelector('[name="worker_urls"]')?.value?.trim() || '',
        notify_on: nForm ? nForm.querySelector('[name="notify_on"]').value : 'none',
        smtp_host: nForm?.querySelector('[name="smtp_host"]')?.value?.trim() || '',
        smtp_port: nForm?.querySelector('[name="smtp_port"]')?.value?.trim() || '587',
        smtp_username: nForm?.querySelector('[name="smtp_username"]')?.value?.trim() || '',
        email_from: nForm?.querySelector('[name="email_from"]')?.value?.trim() || '',
        email_to: nForm?.querySelector('[name="email_to"]')?.value?.trim() || '',
    };

    // Secret fields (webhooks + smtp password): only send when the user typed a
    // new value or explicitly cleared them — otherwise omit so the stored value
    // is preserved (the input is blank when a secret is already configured).
    ['webhook_url', 'slack_webhook_url', 'teams_webhook_url', 'discord_webhook_url', 'smtp_password'].forEach(name => {
        const inp = nForm?.querySelector(`[name="${name}"]`);
        if (!inp) return;
        const v = inp.value.trim();
        if (v) data[name] = v;                 // new value entered
        else if (inp.dataset.clear === '1') data[name] = ''; // explicitly cleared
        // else: leave untouched (omit) so the backend keeps the stored secret
    });

    try {
        await api('/api/settings', { method: 'PUT', body: JSON.stringify(data) });
        toast('Settings saved.', 'success');
        state.settings = null; // force refresh so masks re-render correctly
    } catch (err) {
        toast('Failed to save settings: ' + err.message, 'error');
    }
    btn?.classList.remove('loading');
}

function handleExport() {
    window.open('/api/services/export', '_blank');
}

async function handleImport(event) {
    const file = event.target.files[0];
    if (!file) return;
    try {
        const text = await file.text();
        const data = JSON.parse(text);
        await api('/api/services/import', { method: 'POST', body: JSON.stringify(data) });
        toast('Services imported.', 'success');
        router();
    } catch (err) {
        toast('Import failed: ' + err.message, 'error');
    }
    event.target.value = '';
}

async function handleAddToQueue(serviceId) {
    try {
        await api('/api/queue/add', { method: 'POST', body: JSON.stringify({ service_id: parseInt(serviceId, 10) || serviceId }) });
        toast('Added to queue.', 'success');
        refreshQueue();
    } catch (err) { toast('Queue failed: ' + err.message, 'error'); }
}

// Poll the dashboard while any test is running so cards show live progress.
let livePollTimer = null;
function startLivePolling() {
    if (livePollTimer) return;
    livePollTimer = setInterval(async () => {
        if (!state.currentRoute || state.currentRoute.page !== 'dashboard') { stopLivePolling(); return; }
        try { await refreshServices(); } catch (_) {}
        renderDashboardContent();
        if (!state.services.some(s => s.is_running)) stopLivePolling();
    }, 1500);
}
function stopLivePolling() {
    if (livePollTimer) { clearInterval(livePollTimer); livePollTimer = null; }
}

async function handleStopService(serviceId) {
    try {
        await api(`/api/services/${serviceId}/stop`, { method: 'POST' });
        toast('Test stopped.', 'info');
        await refreshServices();
        renderDashboardContent();
    } catch (err) { toast('Stop failed: ' + err.message, 'error'); }
}

// Start a test immediately from the dashboard (no navigation).
async function handleRunService(serviceId) {
    const svc = state.services.find(s => s.id == serviceId);
    if (svc && svc.is_running) { toast('Test already running.', 'info'); return; }
    try {
        await api(`/api/services/${serviceId}/run`, { method: 'POST' });
        toast(`Started: ${svc ? svc.name : 'test'}`, 'success');
        await refreshServices();
        renderDashboardContent();
        startLivePolling();
    } catch (err) { toast('Run failed: ' + err.message, 'error'); }
}

async function handleClearQueue() {
    if (!(state.queue && (state.queue.items || []).length)) { toast('Queue is already empty.', 'info'); return; }
    const ok = await confirmModal('Clear queue', 'Remove all pending tests from the queue? The currently running test is not affected.');
    if (!ok) return;
    try {
        await api('/api/queue/clear', { method: 'POST' });
        toast('Queue cleared.', 'info');
        await refreshQueue();
        if (state.currentRoute && state.currentRoute.page === 'queue') router();
    } catch (err) { toast('Failed to clear queue: ' + err.message, 'error'); }
}

// Remove a single pending item by its stable id.
async function removeQueueItem(id) {
    try {
        await api('/api/queue/' + id, { method: 'DELETE' });
        await refreshQueue();
        if (state.currentRoute && state.currentRoute.page === 'queue') renderQueueContent();
    } catch (err) { toast('Failed to remove: ' + err.message, 'error'); }
}

// Move a pending item up (-1) or down (+1) and persist the new order.
async function moveQueueItem(id, dir) {
    const items = (state.queue && state.queue.items) ? state.queue.items.slice() : [];
    const i = items.findIndex(it => it.id === id);
    if (i === -1) return;
    const j = i + dir;
    if (j < 0 || j >= items.length) return;
    [items[i], items[j]] = [items[j], items[i]];
    try {
        await api('/api/queue/reorder', { method: 'POST', body: JSON.stringify({ ids: items.map(it => it.id) }) });
        await refreshQueue();
        if (state.currentRoute && state.currentRoute.page === 'queue') renderQueueContent();
    } catch (err) { toast('Failed to reorder: ' + err.message, 'error'); }
}

async function refreshQueue() {
    try { state.queue = await api('/api/queue'); } catch (_) { state.queue = null; }
}

// Re-render just the queue view content (used by live polling / actions)
// without replaying the full-page entrance animation.
function renderQueueContent() {
    const app = document.getElementById('app');
    if (app) renderQueuePage().then(html => { app.innerHTML = html; });
}

// Poll the queue while it is active so the page updates live.
let queuePollTimer = null;
function startQueuePolling() {
    if (queuePollTimer) return;
    queuePollTimer = setInterval(async () => {
        if (!state.currentRoute || state.currentRoute.page !== 'queue') { stopQueuePolling(); return; }
        await refreshQueue();
        renderQueueContent();
        const q = state.queue || {};
        if (!q.running && (!q.items || q.items.length === 0)) stopQueuePolling();
    }, 2000);
}
function stopQueuePolling() {
    if (queuePollTimer) { clearInterval(queuePollTimer); queuePollTimer = null; }
}

// ---- Schedule Handlers ----

async function handleCreateSchedule() {
    const serviceId = document.getElementById('schedule-service')?.value;
    const cronExpr = document.getElementById('schedule-cron')?.value?.trim();
    if (!serviceId || !cronExpr) {
        toast('Please select a service and enter a cron expression.', 'error');
        return;
    }
    try {
        if (editingScheduleId) {
            await api(`/api/schedules/${editingScheduleId}`, {
                method: 'PUT',
                body: JSON.stringify({ cron_expr: cronExpr }),
            });
            toast('Schedule updated.', 'success');
            editingScheduleId = null;
            router();
            return;
        }
        await api('/api/schedules', {
            method: 'POST',
            body: JSON.stringify({ service_id: parseInt(serviceId, 10), cron_expr: cronExpr }),
        });
        toast('Schedule created.', 'success');
        router();
    } catch (err) {
        toast('Failed to create schedule: ' + err.message, 'error');
    }
}

async function handleToggleSchedule(id, enabled) {
    try {
        await api(`/api/schedules/${id}`, {
            method: 'PUT',
            body: JSON.stringify({ enabled }),
        });
        toast(enabled ? 'Schedule enabled.' : 'Schedule disabled.', 'success');
        // Re-render so the recomputed next-run time is reflected.
        if (state.currentRoute && state.currentRoute.page === 'schedules') router();
    } catch (err) {
        toast('Failed to update schedule: ' + err.message, 'error');
        router();
    }
}

async function handleDeleteSchedule(id) {
    const confirmed = await confirmModal('Delete Schedule', 'Are you sure you want to delete this schedule? This action cannot be undone.');
    if (!confirmed) return;
    try {
        await api(`/api/schedules/${id}`, { method: 'DELETE' });
        toast('Schedule deleted.', 'success');
        router();
    } catch (err) { toast('Delete failed: ' + err.message, 'error'); }
}

// ---- Latency Buckets ----

function buildLatencyBuckets(m) {
    const min = m.min_latency_ms||0, max = m.max_latency_ms||0;
    const p50 = m.p50_latency_ms||0, p95 = m.p95_latency_ms||0, p99 = m.p99_latency_ms||0;
    if (max === 0) return Array.from({length:10}, () => ({label:'-', pct:0}));

    const n = 10, range = (max - min) || 1, buckets = [];
    for (let i = 0; i < n; i++) {
        const lo = min + (range/n)*i, mid = lo + (range/n)/2;
        let w = mid <= p50 ? 80 : mid <= p95 ? 30 : mid <= p99 ? 12 : 5;
        w *= Math.exp(-Math.abs(mid - p50) / range * 3);
        buckets.push({ label: fmtShort(lo), weight: w });
    }
    const mw = Math.max(1, ...buckets.map(b => b.weight));
    return buckets.map(b => ({ label: b.label, pct: (b.weight / mw) * 100 }));
}

function fmtShort(ms) {
    if (ms < 1) return '<1';
    if (ms < 1000) return `${Math.round(ms)}`;
    return `${(ms/1000).toFixed(1)}s`;
}

function fmtDate(str) {
    if (!str) return '';
    return new Date(str).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit' });
}

function fmtDateShort(str) {
    if (!str) return '';
    return new Date(str).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
}

function fmtRelativeTime(str) {
    if (!str) return '';
    const now = Date.now();
    const then = new Date(str).getTime();
    const diffMs = now - then;
    if (diffMs < 0) return 'just now';
    const diffSec = Math.floor(diffMs / 1000);
    if (diffSec < 60) return 'just now';
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${diffMin}m ago`;
    const diffHr = Math.floor(diffMin / 60);
    if (diffHr < 24) return `${diffHr}h ago`;
    const diffDay = Math.floor(diffHr / 24);
    if (diffDay === 1) return 'yesterday';
    return fmtDateShort(str);
}

function errorRateColor(rate) {
    if (rate >= 10) return '#ef4444';
    if (rate >= 2) return '#f59e0b';
    return '#10b981';
}

// ---- Trend Analysis Charts ----

function renderTrendChart(points, yKey, color, label, unit) {
    if (!points || points.length < 2) return '';
    const gradId = 'trend-grad-' + label.replace(/\s+/g, '-').toLowerCase();
    const W = 480, H = 180, PAD_L = 50, PAD_R = 20, PAD_T = 24, PAD_B = 40;
    const plotW = W - PAD_L - PAD_R;
    const plotH = H - PAD_T - PAD_B;
    const vals = points.map(p => p.y);
    const minY = Math.min(...vals) * 0.9;
    const maxY = Math.max(...vals) * 1.1 || 1;
    const rangeY = maxY - minY || 1;

    function px(i) { return PAD_L + (i / (points.length - 1)) * plotW; }
    function py(v) { return PAD_T + plotH - ((v - minY) / rangeY) * plotH; }

    // Build polyline path
    const pathParts = points.map((p, i) => `${i === 0 ? 'M' : 'L'}${px(i).toFixed(1)},${py(p.y).toFixed(1)}`);
    const linePath = pathParts.join(' ');

    // Area fill path
    const areaPath = linePath + ` L${px(points.length - 1).toFixed(1)},${PAD_T + plotH} L${PAD_L},${PAD_T + plotH} Z`;

    // Dots
    const dots = points.map((p, i) =>
        `<circle cx="${px(i).toFixed(1)}" cy="${py(p.y).toFixed(1)}" r="3.5" fill="${color}" stroke="var(--surface)" stroke-width="2">` +
        `<title>${p.label}: ${unit === 'ms' ? fmtLatency(p.y) : fmtDec(p.y) + ' ' + unit}</title></circle>`
    ).join('');

    // Y-axis grid lines (4 lines)
    let gridLines = '';
    for (let i = 0; i <= 3; i++) {
        const v = minY + (rangeY * i / 3);
        const y = py(v);
        const dispVal = unit === 'ms' ? fmtLatency(v) : fmtDec(v, 0);
        gridLines += `<line x1="${PAD_L}" y1="${y.toFixed(1)}" x2="${W - PAD_R}" y2="${y.toFixed(1)}" stroke="rgba(71,85,105,0.2)" stroke-dasharray="3,3"/>`;
        gridLines += `<text x="${PAD_L - 6}" y="${(y + 3).toFixed(1)}" text-anchor="end" fill="var(--text-subtle)" font-size="9" font-family="'JetBrains Mono',monospace">${dispVal}</text>`;
    }

    // X-axis labels (show up to 6)
    const step = Math.max(1, Math.floor(points.length / 6));
    let xLabels = '';
    for (let i = 0; i < points.length; i += step) {
        xLabels += `<text x="${px(i).toFixed(1)}" y="${H - 6}" text-anchor="middle" fill="var(--text-subtle)" font-size="8" font-family="'JetBrains Mono',monospace">${points[i].label}</text>`;
    }
    // Always show last label
    if ((points.length - 1) % step !== 0) {
        xLabels += `<text x="${px(points.length - 1).toFixed(1)}" y="${H - 6}" text-anchor="middle" fill="var(--text-subtle)" font-size="8" font-family="'JetBrains Mono',monospace">${points[points.length - 1].label}</text>`;
    }

    // Trend indicator
    const first = vals[0], last = vals[vals.length - 1];
    const trendUp = last > first;
    const trendPct = first !== 0 ? ((last - first) / first * 100) : 0;
    const isGood = (unit === 'ms') ? !trendUp : trendUp;
    const trendColor = Math.abs(trendPct) < 1 ? 'var(--text-muted)' : isGood ? '#10b981' : '#ef4444';
    const trendArrow = trendUp ? '\u2191' : '\u2193';
    const trendLabel = Math.abs(trendPct) < 1 ? 'Stable' : `${trendArrow} ${Math.abs(trendPct).toFixed(1)}%`;

    return `
    <div>
        <div class="flex items-center justify-between mb-2">
            <h4 class="text-xs font-semibold" style="color:var(--text);">${esc(label)}</h4>
            <span class="text-[10px] font-mono font-semibold" style="color:${trendColor};">${trendLabel}</span>
        </div>
        <svg viewBox="0 0 ${W} ${H}" width="100%" height="${H}" style="overflow:visible;">
            <defs>
                <linearGradient id="${gradId}" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stop-color="${color}" stop-opacity="0.25"/>
                    <stop offset="100%" stop-color="${color}" stop-opacity="0"/>
                </linearGradient>
            </defs>
            ${gridLines}
            <path d="${areaPath}" fill="url(#${gradId})"/>
            <path d="${linePath}" fill="none" stroke="${color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
            ${dots}
            ${xLabels}
        </svg>
    </div>`;
}

function renderTrendSection(history) {
    if (!history || history.length < 2) return '';

    const chronological = history.slice().reverse();
    const rpsPoints = chronological.map((h, i) => ({
        x: i,
        y: h.rps || 0,
        label: new Date(h.created_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
    }));
    const latPoints = chronological.map((h, i) => ({
        x: i,
        y: h.avg_latency_ms || 0,
        label: new Date(h.created_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
    }));

    return `
    <div class="mt-8">
        <h3 class="text-lg font-semibold mb-4" style="color:var(--text);">Performance Trend</h3>
        <div class="grid grid-cols-2 gap-4">
            <div class="card p-5">${renderTrendChart(rpsPoints, 'y', '#06b6d4', 'RPS Trend', 'rps')}</div>
            <div class="card p-5">${renderTrendChart(latPoints, 'y', '#8b5cf6', 'Avg Latency Trend', 'ms')}</div>
        </div>
    </div>`;
}

// ---- Dashboard Health Overview ----

function renderHealthOverview(services) {
    const tested = services.filter(s => s.last_result);
    if (tested.length < 2) return '';

    const maxRps = Math.max(1, ...tested.map(s => s.last_result.rps || 0));

    // Sort by RPS descending
    const sorted = tested.slice().sort((a, b) => (b.last_result.rps || 0) - (a.last_result.rps || 0));

    const rows = sorted.map(s => {
        const r = s.last_result;
        const rps = r.rps || 0;
        const pct = (rps / maxRps) * 100;
        const latency = r.avg_latency_ms || 0;
        const errRate = r.total_reqs > 0 ? (r.errors / r.total_reqs * 100) : 0;
        const barColor = errRate >= 10 ? '#ef4444' : errRate >= 2 ? '#f59e0b' : '#06b6d4';
        const errColor = errRate >= 10 ? '#ef4444' : errRate >= 2 ? '#f59e0b' : '#10b981';

        return `
        <div class="flex items-center gap-3">
            <div class="w-28 text-xs font-medium truncate" style="color:var(--text);" title="${esc(s.name)}">${esc(s.name)}</div>
            <div class="flex-1 h-6 rounded-full relative" style="background:rgba(6,182,212,0.1);">
                <div class="h-full rounded-full" style="width:${pct.toFixed(1)}%;background:${barColor};min-width:4px;transition:width .3s ease;"></div>
            </div>
            <div class="w-20 text-xs font-mono text-right" style="color:#06b6d4;">${fmtDec(rps)} /s</div>
            <div class="w-20 text-xs font-mono text-right" style="color:var(--text-muted);">${fmtLatency(latency)}</div>
            <div class="w-16 text-xs font-mono text-right" style="color:${errColor};">${fmtDec(errRate)}%</div>
        </div>`;
    }).join('');

    return `
    <div class="card p-5 mb-6">
        <h3 class="text-sm font-semibold mb-4" style="color:var(--text);">Service Health</h3>
        <div class="flex items-center gap-3 mb-3">
            <div class="w-28"></div>
            <div class="flex-1 text-[10px] font-medium" style="color:var(--text-subtle);">RPS (relative)</div>
            <div class="w-20 text-[10px] font-medium text-right" style="color:var(--text-subtle);">RPS</div>
            <div class="w-20 text-[10px] font-medium text-right" style="color:var(--text-subtle);">Latency</div>
            <div class="w-16 text-[10px] font-medium text-right" style="color:var(--text-subtle);">Errors</div>
        </div>
        <div class="space-y-3">
            ${rows}
        </div>
    </div>`;
}

// ---- Onboarding Guide ----

function renderOnboardingGuide() {
    return `
    <div class="card p-10 text-center">
        <div class="mb-6">
            <svg class="w-16 h-16 mx-auto mb-4" viewBox="0 0 24 24" fill="none"><path d="M13 2L4.09 12.63a1 1 0 00.76 1.63H11v5.74a1 1 0 001.76.65L21.67 9.37A1 1 0 0020.91 7.74H15V2.26A1 1 0 0013 2z" fill="url(#bolt-onboard)"/><defs><linearGradient id="bolt-onboard" x1="4" y1="2" x2="22" y2="22" gradientUnits="userSpaceOnUse"><stop stop-color="var(--accent-text)"/><stop offset="1" stop-color="#7c3aed"/></linearGradient></defs></svg>
            <h2 class="text-2xl font-bold mb-2" style="color:var(--text);">Welcome to gload!</h2>
            <p class="text-sm" style="color:var(--text-muted);">Get started in 3 easy steps:</p>
        </div>
        <div class="grid grid-cols-1 md:grid-cols-3 gap-6 text-left max-w-3xl mx-auto mb-8">
            <div class="card p-6" style="background:rgba(124,58,237,0.06);border-color:rgba(124,58,237,0.2);">
                <div class="w-10 h-10 rounded-xl flex items-center justify-center mb-4" style="background:rgba(124,58,237,0.15);">
                    <svg class="w-5 h-5" style="color:var(--accent-text);" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/></svg>
                </div>
                <div class="text-[10px] uppercase tracking-wider font-semibold mb-2" style="color:#7c3aed;">Step 1</div>
                <h3 class="text-sm font-semibold mb-1" style="color:var(--text);">Create a Service</h3>
                <p class="text-xs" style="color:var(--text-muted);">Add the API endpoint you want to test with method, headers, and body.</p>
            </div>
            <div class="card p-6" style="background:rgba(6,182,212,0.06);border-color:rgba(6,182,212,0.2);">
                <div class="w-10 h-10 rounded-xl flex items-center justify-center mb-4" style="background:rgba(6,182,212,0.15);">
                    <svg class="w-5 h-5" style="color:#06b6d4;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
                </div>
                <div class="text-[10px] uppercase tracking-wider font-semibold mb-2" style="color:#06b6d4;">Step 2</div>
                <h3 class="text-sm font-semibold mb-1" style="color:var(--text);">Run a Load Test</h3>
                <p class="text-xs" style="color:var(--text-muted);">Configure concurrency, duration, and run your first test.</p>
            </div>
            <div class="card p-6" style="background:rgba(16,185,129,0.06);border-color:rgba(16,185,129,0.2);">
                <div class="w-10 h-10 rounded-xl flex items-center justify-center mb-4" style="background:rgba(16,185,129,0.15);">
                    <svg class="w-5 h-5" style="color:#10b981;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 17v-2m3 2v-4m3 4v-6m2 10H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z"/></svg>
                </div>
                <div class="text-[10px] uppercase tracking-wider font-semibold mb-2" style="color:#10b981;">Step 3</div>
                <h3 class="text-sm font-semibold mb-1" style="color:var(--text);">Analyze Results</h3>
                <p class="text-xs" style="color:var(--text-muted);">View real-time metrics, compare results, and set up assertions.</p>
            </div>
        </div>
        <a href="#/services/new" class="btn btn-primary">
            ${svgIcon('plus')}<span>Create Service</span>
        </a>
        <p class="text-xs mt-4" style="color:var(--text-subtle);">Pro tip: You can also import a cURL command directly from browser DevTools!</p>
    </div>`;
}

// ---- Anomaly Detection ----

function renderAnomalyAlert(history) {
    if (!history || history.length < 5) return '';

    const chronological = history.slice().reverse();
    const latest = chronological[chronological.length - 1];

    // Calculate mean and stddev for RPS
    const rpsValues = chronological.map(h => h.rps || 0);
    const rpsMean = rpsValues.reduce((a, b) => a + b, 0) / rpsValues.length;
    const rpsStdDev = Math.sqrt(rpsValues.reduce((sum, v) => sum + Math.pow(v - rpsMean, 2), 0) / rpsValues.length);

    // Calculate mean and stddev for P95 latency
    const p95Values = chronological.map(h => h.p95_latency_ms || 0);
    const p95Mean = p95Values.reduce((a, b) => a + b, 0) / p95Values.length;
    const p95StdDev = Math.sqrt(p95Values.reduce((sum, v) => sum + Math.pow(v - p95Mean, 2), 0) / p95Values.length);

    const anomalies = [];

    // Check RPS anomaly (2 standard deviations)
    const latestRps = latest.rps || 0;
    if (rpsStdDev > 0 && Math.abs(latestRps - rpsMean) > 2 * rpsStdDev) {
        const pctChange = ((latestRps - rpsMean) / rpsMean * 100);
        const direction = pctChange < 0 ? 'dropped' : 'increased';
        anomalies.push(`RPS ${direction} ${Math.abs(pctChange).toFixed(0)}% from average (current: ${fmtDec(latestRps)}, avg: ${fmtDec(rpsMean)}, \u03c3: ${fmtDec(rpsStdDev)})`);
    }

    // Check P95 latency anomaly
    const latestP95 = latest.p95_latency_ms || 0;
    if (p95StdDev > 0 && Math.abs(latestP95 - p95Mean) > 2 * p95StdDev) {
        const pctChange = ((latestP95 - p95Mean) / p95Mean * 100);
        const direction = pctChange > 0 ? 'increased' : 'decreased';
        anomalies.push(`P95 latency ${direction} ${Math.abs(pctChange).toFixed(0)}% from average (current: ${fmtLatency(latestP95)}, avg: ${fmtLatency(p95Mean)}, \u03c3: ${fmtDec(p95StdDev, 1)}ms)`);
    }

    if (anomalies.length === 0) return '';

    return `
    <div class="mt-6">
        <div class="card p-5" style="border-color:rgba(245,158,11,0.4);background:rgba(245,158,11,0.05);">
            <div class="flex items-start gap-3">
                <svg class="w-6 h-6 shrink-0 mt-0.5" style="color:#f59e0b;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4.5c-.77-.833-2.694-.833-3.464 0L3.34 16.5c-.77.833.192 2.5 1.732 2.5z"/></svg>
                <div>
                    <h4 class="text-sm font-semibold mb-2" style="color:#f59e0b;">Anomaly Detected</h4>
                    ${anomalies.map(a => `<p class="text-xs mb-1" style="color:#fbbf24;">${esc(a)}</p>`).join('')}
                    <p class="text-[10px] mt-2" style="color:var(--text-subtle);">Based on ${history.length} historical results (2\u03c3 threshold)</p>
                </div>
            </div>
        </div>
    </div>`;
}

// ---- Load Patterns ----

async function togglePatternPanel(serviceId) {
    const panel = document.getElementById('pattern-panel');
    if (!panel) return;

    if (panel.style.display !== 'none') {
        panel.style.display = 'none';
        return;
    }

    panel.style.display = 'block';
    panel.innerHTML = '<div class="text-sm" style="color:var(--text-muted);">Loading patterns...</div>';

    // Fetch patterns if not cached
    if (!state.patterns) {
        try {
            state.patterns = (await api('/api/patterns')) || [];
        } catch (err) {
            panel.innerHTML = '<div class="text-sm" style="color:#ef4444;">Failed to load patterns: ' + esc(err.message) + '</div>';
            return;
        }
    }

    if (state.patterns.length === 0) {
        panel.innerHTML = '<div class="card p-4 text-sm" style="color:var(--text-subtle);">No load patterns available.</div>';
        return;
    }

    const cards = state.patterns.map(p => {
        const userSummary = (p.stages || []).map(s => s.target || s.users || s.concurrency || 0).join('\u2192');
        const totalDuration = (p.stages || []).reduce((sum, s) => {
            const d = parseDurationToSeconds(s.duration);
            return sum + d;
        }, 0);

        return `
        <div class="card p-4 mb-3" style="border-color:rgba(124,58,237,0.2);">
            <div class="flex items-start justify-between">
                <div class="flex-1">
                    <div class="flex items-center gap-2 mb-1">
                        <h4 class="text-sm font-semibold" style="color:var(--accent-text);">${esc(p.name)}</h4>
                    </div>
                    <p class="text-xs mb-2" style="color:var(--text-muted);">${esc(p.description || '')}</p>
                    <div class="flex items-center gap-2">
                        <span class="text-[10px] font-mono px-2 py-0.5 rounded" style="background:rgba(124,58,237,0.1);color:var(--accent-text);">
                            ${esc(userSummary)} users over ~${totalDuration}s
                        </span>
                    </div>
                    ${(p.stages && p.stages.length > 0) ? `
                    <div class="flex items-center gap-1 mt-2">
                        ${p.stages.map(s => `<div class="text-[9px] px-1.5 py-0.5 rounded" style="background:rgba(71,85,105,0.3);color:var(--text-muted);">${s.target || 0}u / ${s.duration || '?'}</div>`).join('<span style="color:var(--text-faint);">\u2192</span>')}
                    </div>` : ''}
                </div>
                <button onclick="handleRunPattern('${serviceId}','${esc(p.name)}')" class="btn btn-sm" style="background:rgba(124,58,237,0.15);color:var(--accent-text);border:1px solid rgba(124,58,237,0.3);">
                    ${svgIcon('play')} <span>Run</span>
                </button>
            </div>
        </div>`;
    }).join('');

    panel.innerHTML = `
    <div class="card p-4 mb-3" style="border-color:rgba(6,182,212,0.3);background:rgba(6,182,212,0.04);">
        <div class="flex items-center justify-between mb-1">
            <h3 class="text-sm font-semibold" style="color:#22d3ee;">Custom ramp</h3>
            <button onclick="document.getElementById('pattern-panel').style.display='none'" class="btn btn-icon btn-ghost btn-sm" style="padding:0.3rem;">${svgIcon('x')}</button>
        </div>
        <p class="text-[11px] mb-3" style="color:var(--text-faint);">Define your own stages — each ramps linearly from the previous target. Uses this service's steps, weights and think time, so you can reproduce a real launch profile exactly.</p>
        <div class="mb-1 text-[10px] font-medium uppercase tracking-wide" style="display:grid;grid-template-columns:1fr 1fr 2rem;gap:0.5rem;color:var(--text-subtle);">
            <span>Duration</span><span id="custom-target-label">Target concurrency</span><span></span>
        </div>
        <div id="custom-stages"></div>
        <div class="flex items-center flex-wrap gap-3 mt-3">
            <button onclick="addCustomStage('${serviceId}')" class="btn btn-ghost btn-sm" style="padding:0.3rem 0.6rem;">${svgIcon('plus')}<span>Add stage</span></button>
            <label class="flex items-center gap-1.5 text-[11px] cursor-pointer" style="color:var(--text-muted);" title="Treat targets as arrival rate (new requests per second) instead of concurrent users">
                <input type="checkbox" id="custom-open-model" onchange="updateCustomTargetLabel()" style="accent-color:#06b6d4;cursor:pointer;"> arrival rate (req/s) instead of concurrent users
            </label>
            <span id="custom-total" class="text-[11px] ml-auto" style="color:var(--text-subtle);"></span>
            <button onclick="handleRunCustomPattern('${serviceId}')" class="btn btn-sm" style="background:rgba(6,182,212,0.15);color:#22d3ee;border:1px solid rgba(6,182,212,0.35);">${svgIcon('play')}<span>Run custom ramp</span></button>
        </div>
    </div>
    <div class="card p-4" style="border-color:rgba(124,58,237,0.2);background:rgba(124,58,237,0.03);">
        <div class="flex items-center justify-between mb-3">
            <h3 class="text-sm font-semibold" style="color:var(--accent-text);">Predefined Load Patterns</h3>
        </div>
        ${cards}
    </div>`;
    renderCustomStages(serviceId);
}

// ---- Custom ramp builder ----

function defaultCustomStages() {
    return [
        { duration: '60s', target: 100 },
        { duration: '120s', target: 100 },
        { duration: '30s', target: 0 },
    ];
}

function renderCustomStages(serviceId) {
    const el = document.getElementById('custom-stages');
    if (!el) return;
    if (!Array.isArray(state.customStages) || state.customStages.length === 0) {
        state.customStages = defaultCustomStages();
    }
    el.innerHTML = state.customStages.map((s, i) => `
        <div class="mb-1.5" style="display:grid;grid-template-columns:1fr 1fr 2rem;gap:0.5rem;align-items:center;">
            <input type="text" value="${esc(String(s.duration))}" placeholder="e.g. 60s" onchange="updateCustomStage(${i},'duration',this.value)" class="input-dark input-sm" style="font-family:var(--font-mono);">
            <input type="number" min="0" value="${s.target}" onchange="updateCustomStage(${i},'target',this.value)" class="input-dark input-sm" style="font-family:var(--font-mono);">
            <button onclick="removeCustomStage('${serviceId}',${i})" class="btn btn-icon btn-ghost btn-sm" title="Remove stage" style="padding:0.25rem;color:#ef4444;${state.customStages.length <= 1 ? 'opacity:.3;' : ''}" ${state.customStages.length <= 1 ? 'disabled' : ''}>${svgIcon('trash')}</button>
        </div>`).join('');
    updateCustomTotal();
}

function updateCustomStage(i, field, value) {
    if (!state.customStages[i]) return;
    state.customStages[i][field] = field === 'target' ? Math.max(0, parseInt(value) || 0) : value;
    updateCustomTotal();
}

function addCustomStage(serviceId) {
    if (!Array.isArray(state.customStages)) state.customStages = [];
    const last = state.customStages[state.customStages.length - 1];
    state.customStages.push({ duration: '60s', target: last ? last.target : 100 });
    renderCustomStages(serviceId);
}

function removeCustomStage(serviceId, i) {
    if (!Array.isArray(state.customStages) || state.customStages.length <= 1) return;
    state.customStages.splice(i, 1);
    renderCustomStages(serviceId);
}

function updateCustomTargetLabel() {
    const open = document.getElementById('custom-open-model')?.checked;
    const label = document.getElementById('custom-target-label');
    if (label) label.textContent = open ? 'Target req/sec' : 'Target concurrency';
}

function updateCustomTotal() {
    const el = document.getElementById('custom-total');
    if (!el) return;
    const total = (state.customStages || []).reduce((sum, s) => sum + parseDurationToSeconds(s.duration), 0);
    el.textContent = total > 0 ? `total ~${fmtDurationShort(total)}` : '';
}

function fmtDurationShort(seconds) {
    if (seconds >= 60) {
        const m = Math.floor(seconds / 60);
        const s = Math.round(seconds % 60);
        return s > 0 ? `${m}m ${s}s` : `${m}m`;
    }
    return `${Math.round(seconds)}s`;
}

async function handleRunCustomPattern(serviceId) {
    const stages = (state.customStages || [])
        .map(s => ({ duration: String(s.duration).trim(), target: Math.max(0, parseInt(s.target) || 0) }))
        .filter(s => s.duration);
    if (stages.length === 0) { toast('Add at least one stage.', 'error'); return; }
    for (const s of stages) {
        if (parseDurationToSeconds(s.duration) <= 0) {
            toast(`Invalid duration "${s.duration}". Use forms like 30s, 2m, 90s.`, 'error');
            return;
        }
    }
    const openModel = !!document.getElementById('custom-open-model')?.checked;
    try {
        await api('/api/services/' + serviceId + '/run-pattern', {
            method: 'POST',
            body: JSON.stringify({ stages, open_model: openModel }),
        });
        state.testResults = null;
        state.liveTimeSeries = [];
        toast('Custom ramp started.', 'success');
        navigate('/services/' + serviceId + '/run');
    } catch (err) {
        toast('Failed to run custom ramp: ' + err.message, 'error');
    }
}

async function handleRunPattern(serviceId, patternName) {
    try {
        await api('/api/services/' + serviceId + '/run-pattern', {
            method: 'POST',
            body: JSON.stringify({ pattern_name: patternName }),
        });
        state.testResults = null;
        state.liveTimeSeries = [];
        toast('Pattern "' + patternName + '" started.', 'success');
        navigate('/services/' + serviceId + '/run');
    } catch (err) {
        toast('Failed to run pattern: ' + err.message, 'error');
    }
}

// ---- Cross-Service Compare Page ----

async function renderComparePage() {
    const svcs = state.services;

    // Fetch history for each service that has results
    const servicesWithResults = [];
    for (const svc of svcs) {
        if (svc.last_result) {
            let history = [];
            try { history = (await api('/api/services/' + svc.id + '/history')) || []; } catch (_) {}
            const lastResult = history.length > 0 ? history[0] : svc.last_result;
            servicesWithResults.push({ svc, lastResult });
        }
    }

    if (servicesWithResults.length < 2) {
        return `
        ${breadcrumb({ label: 'Dashboard', href: '#/' }, { label: 'Compare' })}
        <h2 class="text-2xl font-bold mb-1" style="color:var(--text);">Compare Services</h2>
        <p class="text-sm mb-6" style="color:var(--text-muted);">Compare performance metrics across services.</p>
        ${emptyState('Not enough data', 'You need at least 2 services with test results to compare.', '#/', 'Back to Dashboard')}`;
    }

    const checkboxRows = servicesWithResults.map(({ svc, lastResult }) => {
        const r = lastResult;
        const errRate = r.total_reqs > 0 ? (r.errors / r.total_reqs * 100) : 0;
        const tested = r.created_at ? `<span class="text-[10px]" style="color:var(--text-faint);"> · tested ${fmtRelativeTime(r.created_at)}</span>` : '';
        return `
        <tr class="border-b border-slate-700/30">
            <td class="py-3 px-4">
                <input type="checkbox" class="compare-svc-check" data-id="${svc.id}" aria-label="Select ${esc(svc.name)} for comparison" style="accent-color:#7c3aed;cursor:pointer;">
            </td>
            <td class="py-3 px-4">
                <div class="flex items-center gap-2">
                    ${methodBadge(svc.method)}
                    <span class="text-sm font-medium" style="color:var(--text);">${esc(svc.name)}</span>
                </div>
                <div class="text-[10px] font-mono mt-0.5" style="color:var(--text-subtle);">${esc(svc.url)}${tested}</div>
            </td>
            <td class="py-3 px-4 text-right font-mono text-xs" style="color:#06b6d4;">${fmtDec(r.rps)}</td>
            <td class="py-3 px-4 text-right font-mono text-xs" style="color:var(--text);">${fmtLatency(r.avg_latency_ms)}</td>
            <td class="py-3 px-4 text-right font-mono text-xs" style="color:${errorRateColor(errRate)};">${fmtDec(errRate)}%</td>
        </tr>`;
    }).join('');

    return `
    ${breadcrumb({ label: 'Dashboard', href: '#/' }, { label: 'Compare' })}

    <div class="flex items-center justify-between mb-8">
        <div>
            <h2 class="text-2xl font-bold" style="color:var(--text);">Compare Services</h2>
            <p class="text-sm mt-1" style="color:var(--text-muted);">Select 2-5 services to compare performance metrics side-by-side.</p>
        </div>
        <button onclick="handleCrossCompare()" id="cross-compare-btn" class="btn btn-primary btn-sm" disabled>
            ${svgIcon('compare')}<span>Compare Selected</span>
        </button>
    </div>

    <div class="card overflow-hidden mb-6">
        <table class="w-full text-sm" id="compare-svc-table">
            <thead>
                <tr class="border-b border-slate-700/50" style="background:var(--bg);">
                    <th class="py-3 px-4 w-8"></th>
                    <th class="text-left py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">Service</th>
                    <th class="text-right py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">RPS</th>
                    <th class="text-right py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">Avg Latency</th>
                    <th class="text-right py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">Error Rate</th>
                </tr>
            </thead>
            <tbody>${checkboxRows}</tbody>
        </table>
    </div>
    <div class="text-xs mb-4" style="color:var(--text-muted);" id="compare-svc-count">0 selected (min 2, max 5)</div>

    <div id="cross-compare-results"></div>`;
}

function setupCompareCheckboxListeners() {
    // Only relevant on the Compare page; avoid touching the DOM on every route.
    if (!state.currentRoute || state.currentRoute.page !== 'compare') return;
    const checkboxes = document.querySelectorAll('.compare-svc-check');
    checkboxes.forEach(cb => {
        // Replace node to strip any previously attached listeners.
        const fresh = cb.cloneNode(true);
        cb.parentNode.replaceChild(fresh, cb);
        fresh.addEventListener('change', () => {
            const allBoxes = document.querySelectorAll('.compare-svc-check');
            const checked = document.querySelectorAll('.compare-svc-check:checked');
            const count = checked.length;
            const btn = document.getElementById('cross-compare-btn');
            const countEl = document.getElementById('compare-svc-count');
            if (btn) btn.disabled = count < 2 || count > 5;
            if (countEl) countEl.textContent = count + ' selected (min 2, max 5)';

            // Disable unchecked boxes if 5 selected
            allBoxes.forEach(c => {
                if (!c.checked) c.disabled = count >= 5;
            });
        });
    });
}

async function handleCrossCompare() {
    const checked = document.querySelectorAll('.compare-svc-check:checked');
    const ids = Array.from(checked).map(cb => cb.dataset.id);
    if (ids.length < 2 || ids.length > 5) {
        toast('Select between 2 and 5 services.', 'error');
        return;
    }

    const results = [];
    for (const id of ids) {
        const svc = state.services.find(s => s.id == id);
        if (!svc) continue;
        let history = [];
        try { history = (await api('/api/services/' + id + '/history')) || []; } catch (_) {}
        const lastResult = history.length > 0 ? history[0] : svc.last_result;
        if (lastResult) results.push({ svc, r: lastResult });
    }

    if (results.length < 2) {
        toast('Not enough results to compare.', 'error');
        return;
    }

    const container = document.getElementById('cross-compare-results');
    if (!container) return;

    // Build comparison table. Throughput metrics (RPS, total requests) are NOT
    // starred as "best": they are driven by each run's load settings
    // (concurrency/duration/rate), so a higher number doesn't mean a better
    // service. Only latency and error rate — where lower is genuinely better at
    // comparable load — get the best-value highlight.
    const metrics = [
        { key: 'rps', label: 'RPS', format: v => fmtDec(v), star: false },
        { key: 'total_reqs', label: 'Total Requests', format: v => fmt(v), star: false },
        { key: 'avg_latency_ms', label: 'Avg Latency', format: v => fmtLatency(v), higherBetter: false, star: true },
        { key: 'p50_latency_ms', label: 'P50 Latency', format: v => fmtLatency(v), higherBetter: false, star: true },
        { key: 'p95_latency_ms', label: 'P95 Latency', format: v => fmtLatency(v), higherBetter: false, star: true },
        { key: 'p99_latency_ms', label: 'P99 Latency', format: v => fmtLatency(v), higherBetter: false, star: true },
    ];

    const headerCols = results.map(({ svc, r }) => {
        const cfg = parseMaybeJSON(r.run_config) || {};
        const bits = [];
        if (cfg.concurrency) bits.push(`c=${cfg.concurrency}`);
        if (cfg.duration) bits.push(esc(cfg.duration));
        if (cfg.rps) bits.push(`${cfg.rps} rps cap`);
        else if (cfg.arrival_rate) bits.push(`${cfg.arrival_rate}/s arrival`);
        const when = r.created_at ? fmtRelativeTime(r.created_at) : '';
        const meta = [when, bits.join(' · ')].filter(Boolean).join(' — ');
        return `<th class="text-right py-3 px-4 text-xs font-medium" style="color:var(--accent-text);">
            <div>${esc(svc.name)}</div>
            ${meta ? `<div class="text-[10px] font-normal mt-0.5" style="color:var(--text-subtle);">${meta}</div>` : ''}
        </th>`;
    }).join('');

    const tableRows = metrics.map(m => {
        const vals = results.map(({ r }) => r[m.key] || 0);
        const bestVal = m.star ? (m.higherBetter ? Math.max(...vals) : Math.min(...vals)) : null;
        const cells = results.map(({ r }) => {
            const v = r[m.key] || 0;
            const isBest = m.star && v === bestVal;
            const style = isBest ? 'color:#10b981;font-weight:700;' : 'color:var(--text);';
            return `<td class="py-2.5 px-4 text-right font-mono text-xs" style="${style}">${m.format(v)}${isBest ? ' *' : ''}</td>`;
        }).join('');
        return `<tr class="border-b border-slate-700/30">
            <td class="py-2.5 px-4 text-xs font-medium" style="color:var(--text-muted);">${m.label}</td>
            ${cells}
        </tr>`;
    }).join('');

    // Error rate row (special)
    const errRates = results.map(({ r }) => r.total_reqs > 0 ? (r.errors / r.total_reqs * 100) : 0);
    const bestErr = Math.min(...errRates);
    const errCells = errRates.map((er, i) => {
        const isBest = er === bestErr;
        const col = isBest ? '#10b981' : errorRateColor(er);
        return `<td class="py-2.5 px-4 text-right font-mono text-xs" style="color:${col};${isBest?'font-weight:700;':''}">${fmtDec(er)}%${isBest ? ' *' : ''}</td>`;
    }).join('');

    // Generic horizontal bar renderer, scaled to `max` and rendered in `grad`.
    const bars = (getVal, fmtVal, max, grad, track) => results.map(({ svc, r }) => {
        const v = getVal(r) || 0;
        const pct = (v / max) * 100;
        return `
        <div class="flex items-center gap-3 mb-3">
            <div class="w-24 text-xs font-medium truncate" style="color:var(--text);" title="${esc(svc.name)}">${esc(svc.name)}</div>
            <div class="flex-1 h-7 rounded relative" style="background:${track};">
                <div class="h-full rounded flex items-center justify-end px-2" style="width:${pct.toFixed(1)}%;background:${grad};min-width:40px;transition:width .3s ease;">
                    <span class="text-[10px] font-mono font-semibold" style="color:var(--text);">${fmtVal(v)}</span>
                </div>
            </div>
        </div>`;
    }).join('');

    const maxRps = Math.max(1, ...results.map(({ r }) => r.rps || 0));
    const rpsBars = bars(r => r.rps, v => fmtDec(v) + ' /s', maxRps,
        'linear-gradient(90deg,rgba(6,182,212,0.3),rgba(124,58,237,0.4))', 'rgba(6,182,212,0.08)');

    // Avg and P95 latency share one scale so the tail gap is visible.
    const maxLat = Math.max(1, ...results.map(({ r }) => Math.max(r.avg_latency_ms || 0, r.p95_latency_ms || 0)));
    const avgLatBars = bars(r => r.avg_latency_ms, v => fmtLatency(v), maxLat,
        'linear-gradient(90deg,rgba(139,92,246,0.3),rgba(6,182,212,0.35))', 'rgba(139,92,246,0.08)');
    const p95LatBars = bars(r => r.p95_latency_ms, v => fmtLatency(v), maxLat,
        'linear-gradient(90deg,rgba(139,92,246,0.35),rgba(245,158,11,0.45))', 'rgba(245,158,11,0.08)');

    // Interval timeline overlays (only shown when at least one run has data).
    const palette = ['#06b6d4', '#7c3aed', '#f59e0b', '#10b981', '#ef4444'];
    const rpsSeries = results.map(({ svc, r }, i) => ({
        name: svc.name, color: palette[i % palette.length],
        points: resultTimeline(r).map(p => ({ t: (p.t || 0) / 1e9, val: p.rps || 0 })),
    }));
    const latSeries = results.map(({ svc, r }, i) => ({
        name: svc.name, color: palette[i % palette.length],
        points: resultTimeline(r).map(p => ({ t: (p.t || 0) / 1e9, val: p.lat_ms || 0 })),
    }));
    const hasTimeline = rpsSeries.some(s => s.points.length >= 2);
    const overlaySection = hasTimeline ? `
    <div class="card p-5 mb-6">
        <h4 class="text-sm font-semibold mb-1" style="color:var(--text);">Over Time <span class="font-normal" style="color:var(--text-faint);">(per interval)</span></h4>
        <p class="text-xs mb-4" style="color:var(--text-muted);">Throughput and latency across each run's duration — reveals which service stays stable vs. degrades under load.</p>
        ${overlayLegend(rpsSeries)}
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-4">
            <div>
                <div class="text-[11px] font-medium mb-1" style="color:#06b6d4;">Requests/sec</div>
                ${renderOverlayChart(rpsSeries, '')}
            </div>
            <div>
                <div class="text-[11px] font-medium mb-1" style="color:#8b5cf6;">Latency</div>
                ${renderOverlayChart(latSeries, 'ms')}
            </div>
        </div>
    </div>` : '';

    // The comparative report endpoint takes RESULT ids (not service ids); each
    // compared run's latest result id is available on r.id.
    const selectedIds = results.map(({ r }) => r.id).filter(Boolean);

    container.innerHTML = `
    <div class="flex items-center justify-between mb-4">
        <h3 class="text-lg font-semibold" style="color:var(--text);">Comparison Results</h3>
        <button onclick="window.open('/api/compare-report?ids=${selectedIds.join(',')}','_blank')" class="btn btn-primary btn-sm">
            ${svgIcon('report')} <span>Download Comparative Report</span>
        </button>
    </div>

    <div class="grid grid-cols-1 lg:grid-cols-2 gap-4 mb-6">
        <div class="card p-5">
            <h4 class="text-sm font-semibold mb-4" style="color:#06b6d4;">RPS Comparison</h4>
            ${rpsBars}
        </div>
        <div class="card p-5">
            <h4 class="text-sm font-semibold mb-4" style="color:#8b5cf6;">Latency Comparison</h4>
            <div class="text-[10px] uppercase tracking-wider mb-2" style="color:var(--text-subtle);">Average</div>
            ${avgLatBars}
            <div class="text-[10px] uppercase tracking-wider mb-2 mt-4" style="color:var(--text-subtle);">P95</div>
            ${p95LatBars}
        </div>
    </div>

    ${overlaySection}

    <div class="card overflow-hidden">
        <table class="w-full text-sm">
            <thead>
                <tr class="border-b border-slate-700/50" style="background:var(--bg);">
                    <th class="text-left py-3 px-4 text-xs font-medium" style="color:var(--text-muted);">Metric</th>
                    ${headerCols}
                </tr>
            </thead>
            <tbody>
                ${tableRows}
                <tr class="border-b border-slate-700/30">
                    <td class="py-2.5 px-4 text-xs font-medium" style="color:var(--text-muted);">Error Rate</td>
                    ${errCells}
                </tr>
            </tbody>
        </table>
        <div class="px-4 py-2 text-[10px]" style="color:var(--text-subtle);">* = best value (latency &amp; error rate only). Throughput (RPS, total requests) isn't ranked — it depends on each run's load settings shown above, not service quality.</div>
    </div>`;
}

// ---- Mobile Menu ----

function toggleMobileMenu() {
    const menu = document.getElementById('mobile-menu');
    if (menu) {
        menu.classList.toggle('hidden');
    }
}

// Single hashchange handler — close mobile menu, route, and re-bind compare checkboxes.
// NOTE: Do NOT add another hashchange listener elsewhere; this is the only one.
window.addEventListener('hashchange', async () => {
    const menu = document.getElementById('mobile-menu');
    if (menu) menu.classList.add('hidden');
    await router();
    setupCompareCheckboxListeners();
});

// ============================================
// Theme Toggle
// ============================================

function toggleTheme() {
    state.theme = state.theme === 'dark' ? 'light' : 'dark';
    document.documentElement.setAttribute('data-theme', state.theme);
    localStorage.setItem('gload_theme', state.theme);
    updateThemeIcon();
}

function updateThemeIcon() {
    const sunSvg = '<svg class="w-4 h-4" style="color:#f59e0b;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z"/></svg>';
    const moonSvg = '<svg class="w-4 h-4" style="color:var(--text-muted);" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z"/></svg>';
    const icon = state.theme === 'dark' ? sunSvg : moonSvg;
    const btn = document.getElementById('theme-toggle');
    if (btn) btn.innerHTML = icon;
    const mobileBtn = document.getElementById('mobile-theme-icon');
    if (mobileBtn) mobileBtn.innerHTML = icon;
}

// Apply theme on load
document.documentElement.setAttribute('data-theme', state.theme);

// ============================================
// Share Result
// ============================================

function handleShareResult(serviceId, resultId) {
    let url;
    if (resultId) {
        url = `${window.location.origin}/api/services/${serviceId}/results/${resultId}/share`;
    } else {
        url = `${window.location.origin}/api/services/${serviceId}/report`;
    }
    navigator.clipboard.writeText(url).then(() => {
        toast('Share link copied to clipboard!', 'success');
    }).catch(() => {
        prompt('Share this URL:', url);
    });
}

async function handleDeleteResult(serviceId, resultId) {
    const confirmed = await confirmModal('Delete result', 'Delete this test result? This cannot be undone.', { confirmLabel: 'Delete', confirmClass: 'btn-danger' });
    if (!confirmed) return;
    try {
        await api(`/api/services/${serviceId}/results/${resultId}`, { method: 'DELETE' });
        toast('Result deleted.', 'success');
        await router();
    } catch (err) { toast('Delete failed: ' + err.message, 'error'); }
}

// ============================================
// Keyboard Shortcuts
// ============================================

// Arrow-key navigation between service cards on the dashboard.
document.addEventListener('keydown', function(e) {
    if (!state.currentRoute || state.currentRoute.page !== 'dashboard') return;
    if (['INPUT', 'TEXTAREA', 'SELECT'].includes(e.target.tagName)) return;
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    if (!['ArrowRight', 'ArrowLeft', 'ArrowUp', 'ArrowDown'].includes(e.key)) return;

    const cards = [...document.querySelectorAll('[data-service-id]')];
    if (cards.length === 0) return;
    const cur = document.activeElement && document.activeElement.dataset && document.activeElement.dataset.serviceId
        ? cards.indexOf(document.activeElement) : -1;
    let next;
    if (cur === -1) next = 0;
    else if (e.key === 'ArrowRight' || e.key === 'ArrowDown') next = Math.min(cur + 1, cards.length - 1);
    else next = Math.max(cur - 1, 0);
    e.preventDefault();
    cards[next].focus();
    cards[next].scrollIntoView({ block: 'nearest' });
});

document.addEventListener('keydown', function(e) {
    // Don't trigger shortcuts when typing in inputs
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.tagName === 'SELECT') return;
    if (e.ctrlKey || e.metaKey || e.altKey) return;

    switch(e.key) {
        case 'n': case 'N':
            e.preventDefault();
            navigate('/services/new');
            break;
        case '/':
            e.preventDefault();
            const searchInput = document.querySelector('input[placeholder="Search services..."]');
            if (searchInput) { searchInput.focus(); }
            else { navigate('/'); }
            break;
        case 'd': case 'D':
            e.preventDefault();
            navigate('/');
            break;
        case 'q': case 'Q':
            e.preventDefault();
            navigate('/queue');
            break;
        case 's': case 'S':
            e.preventDefault();
            navigate('/settings');
            break;
        case '?':
            e.preventDefault();
            showShortcutsHelp();
            break;
        case 'Escape':
            // Close any open modal
            const modal = document.querySelector('.modal-overlay');
            if (modal) modal.remove();
            break;
    }
});

function showShortcutsHelp() {
    const container = document.getElementById('modal-container');
    container.innerHTML = `
    <div class="modal-overlay" onclick="this.remove()">
        <div class="modal-box" onclick="event.stopPropagation()" style="max-width:360px;">
            <h3 class="text-lg font-semibold mb-4" style="color:var(--text);">Keyboard Shortcuts</h3>
            <div class="space-y-2 text-sm">
                ${[
                    ['N', 'New Service'],
                    ['D', 'Dashboard'],
                    ['/', 'Search'],
                    ['Q', 'Queue'],
                    ['S', 'Settings'],
                    ['Esc', 'Close modal'],
                    ['?', 'This help'],
                ].map(([key, desc]) => `
                    <div class="flex items-center justify-between py-1">
                        <span style="color:var(--text-muted);">${desc}</span>
                        <kbd class="px-2 py-0.5 rounded text-xs font-mono" style="background:rgba(71,85,105,0.3);color:var(--text);">${key}</kbd>
                    </div>
                `).join('')}
            </div>
            <button onclick="this.closest('.modal-overlay').remove()" class="btn btn-ghost btn-sm mt-4 w-full">Close</button>
        </div>
    </div>`;
}

// ---- Init ----

document.addEventListener('DOMContentLoaded', async () => {
    updateThemeIcon();
    await router();
    setupCompareCheckboxListeners();
    connectBroadcast();
    monitorConnection();
    // Ensure the global running indicator reflects reality even when landing
    // directly on a non-dashboard page (which may not fetch services itself).
    if (!state.services || !state.services.length) refreshServices();
    else updateGlobalRunning();
});

// ---- Browser Notifications ----

function notifyTestComplete(serviceId) {
    const svc = state.services.find(s => s.id == serviceId);
    const name = svc ? svc.name : 'Test';
    const m = state.testResults || {};
    const status = m.status === 'fail' ? 'FAILED' : 'PASSED';
    const rps = (m.rps || 0).toFixed(1);

    // Play notification sound
    playNotificationSound(m.status === 'fail');

    // Browser notification (if permitted)
    if (Notification.permission === 'granted') {
        const n = new Notification(`gload: ${name} ${status}`, {
            body: `RPS: ${rps} | Latency: ${fmtLatency(m.avg_latency_ms)} | Errors: ${fmt(m.errors || 0)}`,
            icon: 'data:image/svg+xml,' + encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M13 2L4.09 12.63a1 1 0 00.76 1.63H11v5.74a1 1 0 001.76.65L21.67 9.37A1 1 0 0020.91 7.74H15V2.26A1 1 0 0013 2z" fill="#7c3aed"/></svg>'),
            tag: 'gload-test-' + serviceId,
        });
        n.onclick = () => { window.focus(); n.close(); };
    } else if (Notification.permission === 'default') {
        Notification.requestPermission();
    }
}

function playNotificationSound(isFail) {
    try {
        const ctx = new (window.AudioContext || window.webkitAudioContext)();
        const osc = ctx.createOscillator();
        const gain = ctx.createGain();
        osc.connect(gain);
        gain.connect(ctx.destination);
        gain.gain.value = 0.15;

        if (isFail) {
            // Two short low beeps for failure
            osc.frequency.value = 330;
            osc.start(ctx.currentTime);
            gain.gain.setValueAtTime(0.15, ctx.currentTime);
            gain.gain.setValueAtTime(0, ctx.currentTime + 0.12);
            gain.gain.setValueAtTime(0.15, ctx.currentTime + 0.2);
            gain.gain.setValueAtTime(0, ctx.currentTime + 0.32);
            osc.stop(ctx.currentTime + 0.35);
        } else {
            // Pleasant rising chime for success
            osc.frequency.value = 523;
            osc.start(ctx.currentTime);
            osc.frequency.setValueAtTime(659, ctx.currentTime + 0.1);
            osc.frequency.setValueAtTime(784, ctx.currentTime + 0.2);
            gain.gain.setValueAtTime(0.15, ctx.currentTime);
            gain.gain.exponentialRampToValueAtTime(0.01, ctx.currentTime + 0.4);
            osc.stop(ctx.currentTime + 0.4);
        }
    } catch (_) {}
}

// Request notification permission on first interaction
document.addEventListener('click', function requestNotifPerm() {
    if (typeof Notification !== 'undefined' && Notification.permission === 'default') {
        Notification.requestPermission();
    }
    document.removeEventListener('click', requestNotifPerm);
}, { once: true });

// Block non-numeric input on all number fields (e, +, -, .)
document.addEventListener('keydown', function(e) {
    if (e.target && e.target.type === 'number' && ['e','E','+','-','.'].includes(e.key)) {
        e.preventDefault();
    }
});

// ============================================
// Keyboard Shortcuts
// ============================================

const SHORTCUTS = [
    { key: 'n', desc: 'New service',        global: true },
    { key: 'g h', desc: 'Go to Dashboard',  global: true },
    { key: 'g q', desc: 'Go to Queue',      global: true },
    { key: 'g s', desc: 'Go to Schedules',  global: true },
    { key: 'g c', desc: 'Go to Compare',    global: true },
    { key: 'g x', desc: 'Go to Settings',   global: true },
    { key: '/', desc: 'Focus search',        global: true },
    { key: 't', desc: 'Toggle theme',        global: true },
    { key: '?', desc: 'Show shortcuts help', global: true },
    { key: 'Escape', desc: 'Close modal / go back', global: true },
    { key: 'r', desc: 'Run test',           context: 'Service detail' },
    { key: 'e', desc: 'Edit service',       context: 'Service detail' },
    { key: 'd', desc: 'Delete service',     context: 'Service detail' },
    { key: 'c', desc: 'Clone service',      context: 'Service detail' },
    { key: 's', desc: 'Stop running test',  context: 'Running test' },
];

let _goPending = false; // for two-key "g ?" sequences

document.addEventListener('keydown', function(e) {
    // Ignore if user is typing in an input/textarea/select.
    const tag = (e.target.tagName || '').toLowerCase();
    if (tag === 'input' || tag === 'textarea' || tag === 'select') {
        // Only handle Escape in inputs.
        if (e.key === 'Escape') { e.target.blur(); }
        return;
    }
    // Ignore if modifier keys are held (Cmd/Ctrl combos are browser shortcuts).
    if (e.ctrlKey || e.metaKey || e.altKey) return;

    const route = state.currentRoute || {};
    const page = route.page || '';
    const id = route.id;

    // Two-key "g" prefix for navigation.
    if (_goPending) {
        _goPending = false;
        switch (e.key) {
            case 'h': navigate('/'); return;
            case 'q': navigate('/queue'); return;
            case 's': navigate('/schedules'); return;
            case 'x': navigate('/settings'); return;
            case 'c': navigate('/compare'); return;
        }
        return;
    }
    if (e.key === 'g') { _goPending = true; setTimeout(() => { _goPending = false; }, 800); return; }

    switch (e.key) {
        // Global
        case 'n':
            navigate('/services/new');
            break;
        case '/':
            e.preventDefault();
            const searchInput = document.querySelector('[placeholder*="Search"]');
            if (searchInput) searchInput.focus();
            break;
        case 't':
            toggleTheme();
            break;
        case '?':
            showShortcutsModal();
            break;
        case 'Escape':
            // Close modal if open, otherwise go back.
            if (document.getElementById('modal-overlay')) {
                document.getElementById('modal-container').innerHTML = '';
            } else if (document.getElementById('shortcuts-modal')) {
                document.getElementById('shortcuts-modal').remove();
            } else if (page !== 'dashboard') {
                window.history.back();
            }
            break;

        // Service detail context
        case 'r':
            if (page === 'service' && id) handleRunTest(id);
            break;
        case 'e':
            if (page === 'service' && id) handleEditService(id);
            break;
        case 'd':
            if (page === 'service' && id) handleDeleteService(id);
            break;
        case 'c':
            if (page === 'service' && id) handleCloneService(id);
            break;

        // Running test context
        case 's':
            if (page === 'running' && id) handleStopTest(id);
            break;
    }
});

function showShortcutsModal() {
    // Remove existing if any.
    const existing = document.getElementById('shortcuts-modal');
    if (existing) { existing.remove(); return; }

    const globalShortcuts = SHORTCUTS.filter(s => s.global);
    const contextShortcuts = SHORTCUTS.filter(s => s.context);

    const renderRow = (s) => `
        <div class="flex items-center justify-between py-1.5">
            <span class="text-xs" style="color:var(--text-muted);">${esc(s.desc)}</span>
            <div class="flex gap-1">${s.key.split(' ').map(k =>
                `<kbd class="kbd">${esc(k)}</kbd>`
            ).join('<span class="text-[10px]" style="color:var(--text-faint);">then</span>')}</div>
        </div>`;

    const overlay = document.createElement('div');
    overlay.id = 'shortcuts-modal';
    overlay.className = 'modal-overlay';
    overlay.innerHTML = `
        <div class="modal-box" style="max-width:440px;">
            <div class="flex items-center justify-between mb-4">
                <h3 class="text-base font-semibold" style="color:var(--text);">Keyboard Shortcuts</h3>
                <button onclick="document.getElementById('shortcuts-modal').remove()" class="p-1 rounded hover:bg-slate-700/30" style="color:var(--text-muted);">${svgIcon('x')}</button>
            </div>
            <div class="mb-4">
                <div class="text-[10px] font-semibold uppercase tracking-wider mb-2" style="color:var(--text-subtle);">Global</div>
                ${globalShortcuts.map(renderRow).join('')}
            </div>
            <div>
                <div class="text-[10px] font-semibold uppercase tracking-wider mb-2" style="color:var(--text-subtle);">Context-specific</div>
                ${contextShortcuts.map(s => `
                    <div class="flex items-center justify-between py-1.5">
                        <span class="text-xs" style="color:var(--text-muted);">${esc(s.desc)} <span class="text-[10px]" style="color:var(--text-faint);">(${s.context})</span></span>
                        <kbd class="kbd">${esc(s.key)}</kbd>
                    </div>
                `).join('')}
            </div>
            <div class="mt-4 pt-3 border-t border-slate-700/30 text-center">
                <span class="text-[10px]" style="color:var(--text-subtle);">Press <kbd class="kbd">?</kbd> or <kbd class="kbd">Esc</kbd> to close</span>
            </div>
        </div>`;
    overlay.addEventListener('click', function(e) {
        if (e.target === overlay) overlay.remove();
    });
    document.body.appendChild(overlay);
}

// ============================================
// Offline / Connection Status
// ============================================

let _offlineBannerVisible = false;
let _healthCheckInterval = null;
let _consecutiveFailures = 0;

function showOfflineBanner() {
    if (_offlineBannerVisible) return;
    _offlineBannerVisible = true;

    let banner = document.getElementById('offline-banner');
    if (!banner) {
        banner = document.createElement('div');
        banner.id = 'offline-banner';
        banner.className = 'offline-banner';
        banner.innerHTML = `
            <div class="flex items-center justify-center gap-2 py-2 px-4 text-xs font-medium">
                <svg class="w-4 h-4 shrink-0" style="color:#fbbf24" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4.5c-.77-.833-2.694-.833-3.464 0L3.34 16.5c-.77.833.192 2.5 1.732 2.5z"/>
                </svg>
                <span>Connection to server lost. Retrying...</span>
                <button onclick="attemptReconnect()" class="ml-2 px-2 py-0.5 rounded text-[10px] font-semibold" style="background:rgba(251,191,36,0.2);color:#fbbf24;">Retry Now</button>
            </div>`;
        document.body.insertBefore(banner, document.body.firstChild);
        // Push content down.
        const nav = document.querySelector('nav');
        if (nav) nav.style.top = '36px';
    }
    banner.style.display = 'block';

    // Start health check polling if not already running.
    if (!_healthCheckInterval) {
        _healthCheckInterval = setInterval(attemptReconnect, 10000);
    }
}

function hideOfflineBanner() {
    if (!_offlineBannerVisible) return;
    _offlineBannerVisible = false;
    _consecutiveFailures = 0;

    const banner = document.getElementById('offline-banner');
    if (banner) banner.style.display = 'none';

    const nav = document.querySelector('nav');
    if (nav) nav.style.top = '0';

    if (_healthCheckInterval) {
        clearInterval(_healthCheckInterval);
        _healthCheckInterval = null;
    }

    toast('Connection restored.', 'success');
    // Refresh data after reconnect.
    refreshServices().then(() => {
        if (state.currentRoute?.page === 'dashboard') renderDashboardContent();
    });
}

async function attemptReconnect() {
    try {
        const res = await fetch('/health', { signal: AbortSignal.timeout(5000) });
        if (res.ok) {
            hideOfflineBanner();
            // Reconnect broadcast WebSocket if needed.
            if (!broadcastWs) connectBroadcast();
        }
    } catch (_) {
        // Still offline — banner stays visible.
    }
}

// Monitor connection via broadcast WebSocket state and periodic health checks.
function monitorConnection() {
    // Patch connectBroadcast's onclose to detect disconnection.
    const origOnClose = null;
    const _origConnect = connectBroadcast;

    // Override WebSocket onerror/onclose to detect offline.
    const origInterval = setInterval(() => {
        if (broadcastWs && broadcastWs.readyState === WebSocket.OPEN) {
            _consecutiveFailures = 0;
            return;
        }
        // WebSocket not connected — try a lightweight health check.
        fetch('/health', { signal: AbortSignal.timeout(5000) })
            .then(res => {
                if (!res.ok) throw new Error();
                if (_offlineBannerVisible) hideOfflineBanner();
                _consecutiveFailures = 0;
            })
            .catch(() => {
                _consecutiveFailures++;
                if (_consecutiveFailures >= 2) showOfflineBanner();
            });
    }, 15000);

    // Also listen for browser online/offline events.
    window.addEventListener('offline', () => { _consecutiveFailures = 2; showOfflineBanner(); });
    window.addEventListener('online', attemptReconnect);
}

// hashchange listener is registered above (near toggleMobileMenu) — do not duplicate.

// ============================================
// Plugins Page
// ============================================

// Human-readable info for the built-in protocol plugins. Unknown/custom plugins
// fall back to just their name.
const PROTOCOL_INFO = {
    websocket: { label: 'WebSocket', desc: 'Opens a persistent WebSocket connection and measures message round-trip latency.', config: 'url' },
    graphql:   { label: 'GraphQL',   desc: 'Sends a GraphQL query over HTTP and validates the JSON response.', config: 'url, query' },
    grpc:      { label: 'gRPC',      desc: 'Invokes a unary gRPC method over HTTP/2.', config: 'address, method' },
    tcp:       { label: 'TCP',       desc: 'Establishes a raw TCP connection, optionally sending a payload and matching a response.', config: 'address' },
};

async function renderPluginsPage() {
    let pluginData = null;
    try { pluginData = await api('/api/plugins'); } catch (_) {}

    const protocols = pluginData?.protocols || [];
    const collectors = pluginData?.collectors || [];

    const protocolCards = protocols.length > 0
        ? protocols.map(p => {
            const info = PROTOCOL_INFO[p] || { label: p, desc: 'Custom protocol plugin.', config: '' };
            return `<div class="rounded-lg p-4" style="background:var(--surface);border:1px solid var(--border);">
                <div class="flex items-center justify-between gap-2 mb-1.5">
                    <div class="flex items-center gap-2">
                        <svg class="w-4 h-4" style="color:#06b6d4;" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"/></svg>
                        <span class="text-sm font-semibold" style="color:var(--text);">${esc(info.label)}</span>
                        <span class="text-[10px] font-mono px-1.5 py-0.5 rounded" style="background:var(--surface-2);color:var(--text-subtle);">${esc(p)}</span>
                    </div>
                    <button onclick="newServiceWithProtocol('${esc(p)}')" class="text-xs font-medium" style="color:var(--accent-text);background:none;border:none;cursor:pointer;white-space:nowrap;">Use in new service →</button>
                </div>
                <p class="text-xs mb-2" style="color:var(--text-muted);">${esc(info.desc)}</p>
                ${info.config ? `<div class="text-[11px]" style="color:var(--text-faint);">Config keys: <span class="font-mono" style="color:var(--text-subtle);">${esc(info.config)}</span></div>` : ''}
            </div>`;
        }).join('')
        : '<div class="text-sm" style="color:var(--text-subtle);">No protocol plugins registered.</div>';

    const collectorList = collectors.map(c =>
        `<span class="text-xs font-mono px-2 py-1 rounded" style="background:var(--surface-2);color:var(--text-muted);">${esc(c)}</span>`
    ).join(' ');

    return `
    ${breadcrumb({ label: 'Dashboard', href: '#/' }, { label: 'Plugins' })}

    <h2 class="text-2xl font-bold mb-1" style="color:var(--text);">Plugins</h2>
    <p class="text-sm mb-6" style="color:var(--text-muted);">gload can load-test non-HTTP endpoints through protocol plugins. Pick a protocol below, or set it on a service under <span style="color:var(--text-muted);font-weight:600;">Advanced → Protocol</span>.</p>

    <div class="card p-5 mb-6">
        <h3 class="text-sm font-semibold mb-4" style="color:var(--text);">Protocols <span class="font-normal" style="color:var(--text-faint);">— ${protocols.length} available</span></h3>
        <div class="grid grid-cols-1 lg:grid-cols-2 gap-3">${protocolCards}</div>
    </div>

    <div class="card p-5">
        <h3 class="text-sm font-semibold mb-2" style="color:var(--text);">Metric Collectors</h3>
        <p class="text-xs mb-3" style="color:var(--text-muted);">Collectors are an extension point: custom builds can register plugins that gather extra metrics during a run. Standard runs already record latency, percentiles, status codes, and throughput without one${collectors.length ? `. gload ships one reference collector:` : '.'}</p>
        ${collectors.length ? `<div class="flex flex-wrap gap-2">${collectorList}</div>
        <p class="text-[11px] mt-3" style="color:var(--text-faint);">This reference collector is not attached to standard HTTP runs — it's provided as a template for custom builds.</p>` : ''}
    </div>`;
}

// Opens the New Service form pre-set to the given protocol (on the Advanced tab).
function newServiceWithProtocol(proto) {
    state.pendingProtocol = proto;
    navigate('/services/new');
}

// ============================================
// Workspaces Page
// ============================================

async function renderWorkspacesPage() {
    let workspaces = [];
    try { workspaces = (await api('/api/workspaces')) || []; } catch (_) {}
    state.workspaces = workspaces;
    const def = workspaces.find(w => w.slug === 'default');
    state.defaultWorkspaceId = def ? def.id : null;

    // Need services to show per-workspace counts.
    if (!state.services || !state.services.length) { try { await refreshServices(); } catch (_) {} }
    const countFor = ws => (state.services || []).filter(s =>
        s.workspace_id === ws.id || (ws.slug === 'default' && !s.workspace_id)
    ).length;

    const rows = workspaces.map(ws => {
        const n = countFor(ws);
        return `
        <tr>
            <td class="py-3 px-4 text-sm font-medium" style="color:var(--text);">${esc(ws.name)}</td>
            <td class="py-3 px-4 text-sm font-mono" style="color:var(--text-muted);">${esc(ws.slug)}</td>
            <td class="py-3 px-4 text-sm" style="color:var(--text-muted);">${esc(ws.description || '-')}</td>
            <td class="py-3 px-4 text-sm text-right font-mono" style="color:var(--text-muted);">${n} service${n === 1 ? '' : 's'}</td>
            <td class="py-3 px-4 text-sm" style="color:var(--text-subtle);">${ws.created_at ? fmtDateShort(ws.created_at) : '-'}</td>
            <td class="py-3 px-4 text-right">
                ${ws.slug !== 'default' ? `<button onclick="handleDeleteWorkspace(${ws.id})" class="btn btn-ghost btn-sm" style="color:#ef4444;">Delete</button>` : '<span class="text-xs" style="color:var(--text-subtle);">Default</span>'}
            </td>
        </tr>`;
    }).join('');

    return `
    ${breadcrumb({ label: 'Dashboard', href: '#/' }, { label: 'Workspaces' })}

    <div class="flex items-center justify-between mb-6">
        <div>
            <h2 class="text-2xl font-bold mb-1" style="color:var(--text);">Workspaces</h2>
            <p class="text-sm" style="color:var(--text-muted);">Organize services into team workspaces.</p>
        </div>
        <button onclick="showCreateWorkspaceForm()" class="btn btn-primary btn-sm">
            <svg class="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M12 4v16m8-8H4"/></svg>
            <span>New Workspace</span>
        </button>
    </div>

    <div id="workspace-create-form" style="display:none;" class="card p-5 mb-6">
        <h3 class="text-sm font-semibold mb-3" style="color:var(--text);">Create Workspace</h3>
        <div class="grid grid-cols-3 gap-4 mb-4">
            <div>
                <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Name</label>
                <input type="text" id="ws-name" class="input-dark input-sm" placeholder="Team A" oninput="onWorkspaceNameInput()" onkeydown="if(event.key==='Enter')handleCreateWorkspace()">
            </div>
            <div>
                <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Slug</label>
                <input type="text" id="ws-slug" class="input-dark input-sm" placeholder="team-a" oninput="this.dataset.touched='1'" onkeydown="if(event.key==='Enter')handleCreateWorkspace()">
            </div>
            <div>
                <label class="block text-xs font-medium mb-1.5" style="color:var(--text-muted);">Description</label>
                <input type="text" id="ws-desc" class="input-dark input-sm" placeholder="Optional description" onkeydown="if(event.key==='Enter')handleCreateWorkspace()">
            </div>
        </div>
        <div class="flex items-center gap-2">
            <button onclick="handleCreateWorkspace()" class="btn btn-primary btn-sm">Create</button>
            <button onclick="document.getElementById('workspace-create-form').style.display='none'" class="btn btn-ghost btn-sm">Cancel</button>
        </div>
    </div>

    <div class="card overflow-hidden">
        <table class="w-full">
            <thead>
                <tr style="background:var(--surface);">
                    <th class="text-left py-2.5 px-4 text-xs font-semibold" style="color:var(--text-muted);">Name</th>
                    <th class="text-left py-2.5 px-4 text-xs font-semibold" style="color:var(--text-muted);">Slug</th>
                    <th class="text-left py-2.5 px-4 text-xs font-semibold" style="color:var(--text-muted);">Description</th>
                    <th class="text-right py-2.5 px-4 text-xs font-semibold" style="color:var(--text-muted);">Services</th>
                    <th class="text-left py-2.5 px-4 text-xs font-semibold" style="color:var(--text-muted);">Created</th>
                    <th class="text-right py-2.5 px-4 text-xs font-semibold" style="color:var(--text-muted);">Actions</th>
                </tr>
            </thead>
            <tbody class="divide-y divide-slate-700/30">${rows}</tbody>
        </table>
    </div>`;
}

function showCreateWorkspaceForm() {
    document.getElementById('workspace-create-form').style.display = 'block';
    document.getElementById('ws-name')?.focus();
}

// Converts a display name to a URL-friendly slug.
function slugify(str) {
    return str.toLowerCase().trim()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-+|-+$/g, '');
}

// Auto-fills the slug from the name until the user edits the slug manually.
function onWorkspaceNameInput() {
    const nameEl = document.getElementById('ws-name');
    const slugEl = document.getElementById('ws-slug');
    if (!nameEl || !slugEl) return;
    if (slugEl.dataset.touched === '1' && slugEl.value.trim() !== '') return;
    slugEl.value = slugify(nameEl.value);
}

async function handleCreateWorkspace() {
    const name = document.getElementById('ws-name').value.trim();
    let slug = document.getElementById('ws-slug').value.trim();
    const desc = document.getElementById('ws-desc').value.trim();
    if (!name) {
        toast('Name is required.', 'error');
        return;
    }
    // Derive a slug from the name if the user left it blank.
    if (!slug) slug = slugify(name);
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(slug)) {
        toast('Slug must be lowercase letters, numbers, and single hyphens (e.g. team-a).', 'error');
        return;
    }
    if ((state.workspaces || []).some(w => w.slug === slug)) {
        toast(`A workspace with slug "${slug}" already exists.`, 'error');
        return;
    }
    try {
        await api('/api/workspaces', {
            method: 'POST',
            body: JSON.stringify({ name, slug, description: desc }),
        });
        toast('Workspace created.', 'success');
        router();
    } catch (err) {
        toast('Failed to create workspace: ' + err.message, 'error');
    }
}

async function handleDeleteWorkspace(id) {
    const ok = await confirmModal('Delete Workspace', 'Are you sure? Services will be moved to the default workspace.');
    if (!ok) return;
    try {
        await api(`/api/workspaces/${id}`, { method: 'DELETE' });
        toast('Workspace deleted.', 'success');
        // If current workspace was deleted, reset to all
        if (localStorage.getItem('gload_workspace') == id) {
            localStorage.removeItem('gload_workspace');
        }
        router();
    } catch (err) {
        toast('Failed to delete workspace: ' + err.message, 'error');
    }
}

// ============================================
// Workspace Selector
// ============================================

async function updateWorkspaceSelector() {
    const container = document.getElementById('workspace-selector');
    if (!container) return;

    let workspaces = [];
    try { workspaces = (await api('/api/workspaces')) || []; } catch (_) {}

    // Cache for the service form's workspace picker and the dashboard filter.
    state.workspaces = workspaces;
    const def = workspaces.find(w => w.slug === 'default');
    state.defaultWorkspaceId = def ? def.id : null;

    const currentId = localStorage.getItem('gload_workspace') || '';
    const currentWs = workspaces.find(w => String(w.id) === currentId);
    const label = currentWs ? currentWs.name : 'All Workspaces';

    container.innerHTML = `
    <div class="relative inline-block">
        <button onclick="toggleWorkspaceDropdown()" class="nav-link text-sm font-medium px-3 py-1.5 rounded-lg flex items-center gap-1" id="ws-dropdown-btn">
            <svg class="w-3.5 h-3.5" style="color:var(--text-muted);" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"/></svg>
            <span>${esc(label)}</span>
            <svg class="w-3 h-3" style="color:var(--text-subtle);" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/></svg>
        </button>
        <div id="ws-dropdown" class="absolute left-0 top-full mt-1 w-48 rounded-lg border border-slate-700/50 shadow-xl z-50" style="background:var(--surface);display:none;">
            <button onclick="selectWorkspace('')" class="w-full text-left px-4 py-2.5 text-sm hover:bg-slate-700/40 transition-colors ${!currentId ? 'font-semibold' : ''}" style="color:var(--text);">All Workspaces</button>
            ${workspaces.map(ws => `
                <button onclick="selectWorkspace('${ws.id}')" class="w-full text-left px-4 py-2.5 text-sm hover:bg-slate-700/40 transition-colors ${String(ws.id) === currentId ? 'font-semibold' : ''}" style="color:var(--text);">${esc(ws.name)}</button>
            `).join('')}
            <div class="border-t border-slate-700/30">
                <button onclick="navigate('/workspaces');toggleWorkspaceDropdown();" class="w-full text-left px-4 py-2.5 text-xs hover:bg-slate-700/40 transition-colors" style="color:var(--text-muted);">Manage Workspaces</button>
            </div>
        </div>
    </div>`;
}

function toggleWorkspaceDropdown() {
    const dd = document.getElementById('ws-dropdown');
    if (dd) dd.style.display = dd.style.display === 'none' ? 'block' : 'none';
}

function selectWorkspace(id) {
    if (id) {
        localStorage.setItem('gload_workspace', id);
    } else {
        localStorage.removeItem('gload_workspace');
    }
    toggleWorkspaceDropdown();
    router();
}

// Apply workspace filter to dashboard service listing. Services with a legacy
// workspace_id of 0 (unassigned) count as belonging to the default workspace.
function getFilteredServices() {
    const wsId = localStorage.getItem('gload_workspace');
    if (!wsId) return state.services;
    const isDefault = state.defaultWorkspaceId != null && String(state.defaultWorkspaceId) === wsId;
    return state.services.filter(s => {
        if (String(s.workspace_id) === wsId) return true;
        return isDefault && !s.workspace_id;
    });
}
