const API_BASE = '';

let currentVMs = [];
let selectedAccount = null;
let authEnabled = false;

const els = {};

document.addEventListener('DOMContentLoaded', () => {
  cacheElements();
  bindNavigation();
  bindActions();
  checkAuth();
});

function cacheElements() {
  Object.assign(els, {
    appShell: document.getElementById('app-shell'),
    logoutBtn: document.getElementById('logout-btn'),
    authBadge: document.getElementById('auth-badge'),
    accountList: document.getElementById('account-list'),
    accountBalance: document.getElementById('account-balance'),
    selectedAccount: document.getElementById('selected-account'),
    vmList: document.getElementById('vm-list'),
    vmCount: document.getElementById('vm-count'),
    runningCount: document.getElementById('running-count'),
    stoppedCount: document.getElementById('stopped-count'),
    logs: document.getElementById('logs'),
    authForm: document.getElementById('auth-settings-form'),
    authEnabled: document.getElementById('auth-enabled'),
    authUsername: document.getElementById('auth-username'),
    authPassword: document.getElementById('auth-password'),
    authSessionHours: document.getElementById('auth-session-hours'),
    authCookieSecure: document.getElementById('auth-cookie-secure'),
    authMessage: document.getElementById('auth-settings-message'),
    configPath: document.getElementById('config-path'),
    configStatus: document.getElementById('config-status')
  });
}

function bindNavigation() {
  document.querySelectorAll('.nav-item').forEach(item => {
    item.addEventListener('click', event => {
      event.preventDefault();
      const section = item.dataset.section;
      document.querySelectorAll('.nav-item').forEach(i => i.classList.remove('active'));
      item.classList.add('active');
      document.querySelectorAll('.section').forEach(s => s.classList.remove('active'));
      document.getElementById(`${section}-section`)?.classList.add('active');
      if (section === 'settings') {
        loadAuthSettings();
        loadConfigStatus();
      }
      if (section === 'dns') {
        loadDNSPage();
      }
    });
  });
}

function bindActions() {
  els.logoutBtn.addEventListener('click', handleLogout);
  document.getElementById('refresh-current-btn').addEventListener('click', refreshVMs);
  document.getElementById('clear-log-btn').addEventListener('click', clearLogs);
  document.getElementById('reload-config-btn').addEventListener('click', reloadConfig);
  els.authForm.addEventListener('submit', saveAuthSettings);

  document.addEventListener('click', event => {
    const actionButton = event.target.closest('.action-btn[data-action]');
    if (actionButton) {
      handleVMAction(actionButton);
      return;
    }

    const balanceButton = event.target.closest('.account-balance-btn');
    if (balanceButton) {
      fetchAccountBalance(cardToAccount(balanceButton.closest('.account-card')));
      return;
    }

    const accountButton = event.target.closest('.account-load-btn');
    if (accountButton) {
      loadAccount(cardToAccount(accountButton.closest('.account-card')));
    }
  });
}

async function checkAuth() {
  try {
    const data = await fetchJSON('/api/auth', { skipAuthRedirect: true });
    authEnabled = Boolean(data.enabled);
    updateAuthUI(data);

    if (data.enabled && !data.authenticated) {
      window.location.replace('/login');
      return;
    }

    unlockApp();
    addLog('已读取本地配置，点击账号可加载机器。', 'info');
    fetchAccounts();
    loadConfigStatus();
  } catch (error) {
    window.location.replace('/login');
  }
}

async function handleLogout() {
  try {
    await fetchJSON('/api/logout', { method: 'POST', skipAuthRedirect: true });
  } catch (error) {
    addLog(`退出登录异常：${error.message}`, 'error');
  }
  window.location.replace('/login');
}

function unlockApp() {
  els.appShell.classList.remove('locked');
}

function updateAuthUI(data) {
  authEnabled = Boolean(data.enabled);
  els.logoutBtn.hidden = !authEnabled;
  els.authBadge.textContent = authEnabled
    ? (data.authenticated ? `已登录：${data.user || 'admin'}` : '需要登录')
    : '认证未开启';
  els.authBadge.classList.toggle('secure', authEnabled && data.authenticated);
}

async function fetchAccounts() {
  els.accountList.innerHTML = '<div class="empty-state compact">正在读取本地账号配置...</div>';

  try {
    const accounts = await fetchJSON('/api/accounts');
    renderAccounts(Array.isArray(accounts) ? accounts : []);
  } catch (error) {
    els.accountList.innerHTML = `<div class="empty-state compact error">读取失败：${escapeHtml(error.message)}</div>`;
    addLog(`读取账号配置失败：${error.message}`, 'error');
  }
}

function renderAccounts(accounts) {
  if (accounts.length === 0) {
    els.accountList.innerHTML = '<div class="empty-state compact">没有可用账号配置。</div>';
    return;
  }

  els.accountList.innerHTML = accounts.map(account => `
    <div class="account-card"
      data-provider="${escapeAttr(account.provider)}"
      data-account="${escapeAttr(account.account)}"
      data-group="${escapeAttr(account.group || account.provider)}">
      <button class="account-load-btn" type="button">
        <span class="account-provider">${escapeHtml(account.provider)}</span>
        <span class="account-name">${escapeHtml(account.account)}</span>
        <span class="account-group">${escapeHtml(account.group || account.provider)}</span>
      </button>
      ${account.provider === 'azure' ? '<button class="account-balance-btn" type="button">余额</button>' : ''}
    </div>
  `).join('');
}

async function fetchAccountBalance(account) {
  els.accountBalance.innerHTML = '<div class="empty-state compact">正在查询 Azure 余额...</div>';

  try {
    const data = await fetchJSON(`/api/account/${encodeURIComponent(account.provider)}/${encodeURIComponent(account.account)}/balance`);
    renderAccountBalance(data);
    addLog(`已查询 ${account.provider}/${account.account} 余额。`, 'success');
  } catch (error) {
    els.accountBalance.innerHTML = `<div class="empty-state compact error">查询失败：${escapeHtml(error.message)}</div>`;
    addLog(`余额查询失败：${error.message}`, 'error');
  }
}

function renderAccountBalance(data) {
  const total = Number(data.total || 0);
  els.accountBalance.innerHTML = `
    <div class="balance-summary">
      <span>${escapeHtml((data.provider || '').toUpperCase())} / ${escapeHtml(data.account || '')}</span>
      <strong>${total.toFixed(2)} ${escapeHtml(data.currency || '')}</strong>
      ${data.note ? `<p>${escapeHtml(data.note)}</p>` : ''}
    </div>
  `;
}

async function loadAccount(account) {
  selectedAccount = account;
  els.selectedAccount.textContent = `${account.provider} / ${account.account}`;
  els.accountBalance.innerHTML = account.provider === 'azure'
    ? '<div class="empty-state compact">点击账号右侧“余额”按钮查询。</div>'
    : '<div class="empty-state compact">当前云厂商未配置余额查询。</div>';

  document.querySelectorAll('.account-card').forEach(card => {
    card.classList.toggle('active', card.dataset.provider === account.provider && card.dataset.account === account.account);
  });
  await fetchVMs();
}

async function fetchVMs() {
  if (!selectedAccount) {
    els.vmList.innerHTML = '<div class="empty-state">请选择一个账号加载机器。</div>';
    updateStats([]);
    return;
  }

  els.vmList.innerHTML = '<div class="empty-state">正在加载机器列表...</div>';

  try {
    const vms = await fetchJSON(`/api/vms?provider=${encodeURIComponent(selectedAccount.provider)}&account=${encodeURIComponent(selectedAccount.account)}`);
    currentVMs = Array.isArray(vms) ? vms : [];
    updateStats(currentVMs);
    renderVMList(currentVMs);
    addLog(`已加载 ${selectedAccount.provider}/${selectedAccount.account}，共 ${currentVMs.length} 台机器。`, 'success');
  } catch (error) {
    els.vmList.innerHTML = `<div class="empty-state error">加载失败：${escapeHtml(error.message)}</div>`;
    addLog(`获取 VM 列表失败：${error.message}`, 'error');
  }
}

function updateStats(vms) {
  const running = vms.filter(v => v.status === 'VM running').length;
  const stopped = vms.filter(v => v.status === 'VM deallocated' || v.status === 'VM stopped').length;
  els.vmCount.textContent = vms.length;
  els.runningCount.textContent = running;
  els.stoppedCount.textContent = stopped;
}

function renderVMList(vms) {
  if (vms.length === 0) {
    els.vmList.innerHTML = '<div class="empty-state">当前账号没有加载到 VM 实例。</div>';
    return;
  }

  const groups = groupVMs(vms);
  els.vmList.innerHTML = Object.entries(groups).map(([groupName, groupVMs]) => `
    <section class="provider-group">
      <div class="provider-group-header">
        <h3>${escapeHtml(groupName)}</h3>
        <span>${groupVMs.length} 台</span>
      </div>
      <div class="provider-vm-grid">
        ${groupVMs.map(renderVMCard).join('')}
      </div>
    </section>
  `).join('');
}

function groupVMs(vms) {
  return vms.reduce((groups, vm) => {
    const provider = vm.provider || 'azure';
    const account = vm.accountId || 'default';
    const group = vm.group || provider;
    const key = `${group} / ${provider} / ${account}`;
    groups[key] = groups[key] || [];
    groups[key].push(vm);
    return groups;
  }, {});
}

function renderVMCard(vm) {
  const provider = vm.provider || 'azure';
  const accountId = vm.accountId || 'default';
  const name = vm.name || vm.id || '';
  const id = vm.id || name;
  const status = vm.status || 'Unknown';
  const publicIP = vm.publicIP?.ipAddress || '未分配';
  const publicIPName = vm.publicIP?.name || 'N/A';
  const dnsEnabled = Boolean(vm.dnsEnabled);

  return `
    <article class="vm-card"
      data-provider="${escapeAttr(provider)}"
      data-account-id="${escapeAttr(accountId)}"
      data-id="${escapeAttr(id)}"
      data-name="${escapeAttr(name)}"
      data-status="${escapeAttr(status)}">
      <div class="vm-header">
        <div>
          <span class="vm-kicker">${escapeHtml(provider.toUpperCase())} / ${escapeHtml(accountId)}</span>
          <h3 class="vm-name">${escapeHtml(name)}</h3>
        </div>
        <span class="status-badge status-${getStatusClass(status)}">${escapeHtml(getStatusText(status))}</span>
      </div>

      <dl class="vm-info">
        <div class="info-item wide"><dt>实例名称</dt><dd>${escapeHtml(name)}</dd></div>
        <div class="info-item"><dt>区域</dt><dd>${escapeHtml(getLocationText(vm.location || vm.zone || 'N/A'))}</dd></div>
        <div class="info-item"><dt>规格</dt><dd>${escapeHtml(vm.vmSize || 'N/A')}</dd></div>
        <div class="info-item"><dt>公网 IP</dt><dd class="mono">${escapeHtml(publicIP)}</dd></div>
        <div class="info-item"><dt>公网 IP 名称</dt><dd>${escapeHtml(publicIPName)}</dd></div>
        <div class="info-item"><dt>内网 IP</dt><dd class="mono">${escapeHtml(vm.privateIP || '未分配')}</dd></div>
        <div class="info-item"><dt>资源组 / 项目 / 区间</dt><dd>${escapeHtml(vm.resourceGroup || '-')}</dd></div>
      </dl>

      <div class="vm-options">
        <label class="dns-toggle ${dnsEnabled ? '' : 'disabled'}">
          <input type="checkbox" class="change-ip-dns-toggle" ${dnsEnabled ? '' : 'disabled'}>
          <span>换 IP 后更新 DNS</span>
        </label>
      </div>

      <div class="vm-actions">
        <button class="action-btn start" type="button" data-action="start" ${status === 'VM running' ? 'disabled' : ''}>开机</button>
        <button class="action-btn stop" type="button" data-action="stop" ${status !== 'VM running' ? 'disabled' : ''}>关机</button>
        <button class="action-btn restart" type="button" data-action="restart">重启</button>
        <button class="action-btn change-ip" type="button" data-action="change-ip">换 IP</button>
        ${dnsEnabled ? '<button class="action-btn dns" type="button" data-action="update-dns">更新 DNS</button>' : ''}
        <button class="action-btn dns-bind" type="button" data-action="dns-bind">DNS 绑定</button>
      </div>
    </article>
  `;
}

async function handleVMAction(button) {
  const card = button.closest('.vm-card');
  const action = button.dataset.action;
  const vm = {
    provider: card.dataset.provider,
    accountId: card.dataset.accountId,
    id: card.dataset.id,
    name: card.dataset.name,
    status: card.dataset.status
  };
  const labels = {
    start: '开机',
    stop: '关机',
    restart: '重启',
    'change-ip': '换 IP',
    'update-dns': '更新 DNS'
  };

  if (action === 'dns-bind') {
    openDNSBindingModal(vm);
    return;
  }
  if (action === 'start' && vm.status === 'VM running') {
    addLog(`VM ${vm.name} 已经在运行中。`, 'info');
    return;
  }
  if (action === 'stop' && vm.status !== 'VM running') {
    addLog(`VM ${vm.name} 当前不是运行状态。`, 'info');
    return;
  }

  addLog(`正在执行 ${labels[action]}：${vm.accountId}/${vm.name}`, 'info');
  button.disabled = true;

  try {
    const data = await fetchJSON(vmActionURL(vm, action, card), { method: 'POST' });
    if (Array.isArray(data.logs)) data.logs.forEach(log => addLog(log, 'info'));

    if (action === 'change-ip' && data.newIpAddress) {
      addLog(`换 IP 成功，新 IP：${data.newIpAddress}`, 'success');
    } else if (action === 'update-dns' && data.newIpAddress) {
      addLog(`DNS 已按当前 IP 更新：${data.newIpAddress}`, 'success');
    } else {
      addLog(data.message || `${labels[action]}请求已提交。`, 'success');
    }

    setTimeout(() => refreshVM(vm), refreshDelay(action));
  } catch (error) {
    addLog(`${labels[action]}失败：${error.message}`, 'error');
  } finally {
    button.disabled = false;
  }
}

function vmActionURL(vm, action, card) {
  const base = `/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}`;
  if (action === 'change-ip') {
    const updateDNS = card?.querySelector('.change-ip-dns-toggle')?.checked === true;
    return `${base}/change-ip?update_dns=${updateDNS ? 'true' : 'false'}`;
  }
  if (action === 'update-dns') return `${base}/update-dns`;
  return `${base}/${action}`;
}

async function refreshVM(vm) {
  try {
    const data = await fetchJSON(`/api/refresh/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}`);
    addLog(`VM ${vm.accountId}/${vm.name} 状态已更新：${getStatusText(data.status)}`, 'info');
    fetchVMs();
  } catch (error) {
    addLog(`刷新 VM 失败：${error.message}`, 'error');
  }
}

function refreshVMs() {
  if (!selectedAccount) {
    addLog('请先选择一个账号。', 'info');
    return;
  }
  addLog(`正在刷新 ${selectedAccount.provider}/${selectedAccount.account} 的 VM 列表...`, 'info');
  fetchVMs();
}

async function loadAuthSettings() {
  try {
    const data = await fetchJSON('/api/settings/auth');
    els.authEnabled.checked = Boolean(data.enabled);
    els.authUsername.value = data.username || '';
    els.authPassword.value = '';
    els.authSessionHours.value = data.session_hours || 12;
    els.authCookieSecure.checked = Boolean(data.cookie_secure);
    els.authMessage.textContent = data.has_password ? '当前已配置密码哈希。' : '当前未配置密码。';
    els.authMessage.className = 'form-message';
  } catch (error) {
    showFormMessage(error.message, true);
  }
}

async function saveAuthSettings(event) {
  event.preventDefault();
  showFormMessage('正在保存...', false);

  try {
    const data = await fetchJSON('/api/settings/auth', {
      method: 'POST',
      body: {
        enabled: els.authEnabled.checked,
        username: els.authUsername.value.trim(),
        password: els.authPassword.value,
        session_hours: Number(els.authSessionHours.value || 12),
        cookie_secure: els.authCookieSecure.checked
      }
    });
    els.authPassword.value = '';
    updateAuthUI({ enabled: data.auth.enabled, authenticated: true, user: data.auth.username });
    showFormMessage('认证配置已保存，并已自动重载生效。', false);
    addLog('认证配置已保存并自动重载。', 'success');
    loadConfigStatus();
  } catch (error) {
    showFormMessage(error.message, true);
  }
}

async function reloadConfig() {
  try {
    await fetchJSON('/api/config/reload', { method: 'POST' });
    addLog('配置文件已手动重载。', 'success');
    showFormMessage('配置文件已重载。', false);
    loadConfigStatus();
    fetchAccounts();
  } catch (error) {
    addLog(`重载配置失败：${error.message}`, 'error');
    showFormMessage(error.message, true);
  }
}

async function loadConfigStatus() {
  try {
    const data = await fetchJSON('/api/config/status');
    els.configPath.textContent = data.path || '-';
    els.configStatus.textContent = data.lastReloadError ? `失败：${data.lastReloadError}` : '正常，等待手动重载';
  } catch (error) {
    els.configStatus.textContent = `读取失败：${error.message}`;
  }
}

function showFormMessage(message, isError) {
  els.authMessage.textContent = message;
  els.authMessage.className = `form-message ${isError ? 'error' : 'success'}`;
}

function refreshDelay(action) {
  if (action === 'restart') return 8000;
  if (action === 'stop' || action === 'change-ip') return 5000;
  return 3000;
}

function getStatusClass(status) {
  if (status === 'VM running') return 'running';
  if (status === 'VM deallocated' || status === 'VM stopped') return 'stopped';
  return 'unknown';
}

function getStatusText(status) {
  if (status === 'VM running') return '运行中';
  if (status === 'VM deallocated' || status === 'VM stopped') return '已停止';
  return status || '未知';
}

function getLocationText(location) {
  const locations = {
    koreacentral: '韩国中部',
    koreasouth: '韩国南部',
    eastasia: '东亚',
    southeastasia: '东南亚',
    centralus: '美国中部',
    eastus: '美国东部',
    westus: '美国西部'
  };
  return locations[location] || location;
}

function cardToAccount(card) {
  return {
    provider: card.dataset.provider,
    account: card.dataset.account,
    group: card.dataset.group
  };
}

async function fetchJSON(path, options = {}) {
  const fetchOptions = {
    method: options.method || 'GET',
    cache: 'no-store',
    headers: { Accept: 'application/json' }
  };

  if (options.body !== undefined) {
    fetchOptions.headers['Content-Type'] = 'application/json';
    fetchOptions.body = JSON.stringify(options.body);
  }

  const response = await fetch(`${API_BASE}${path}`, fetchOptions);
  if (response.status === 401 && !options.skipAuthRedirect) {
    window.location.replace('/login');
    throw new Error('需要登录');
  }

  const text = await response.text();
  let data = {};
  if (text) {
    try {
      data = JSON.parse(text);
    } catch (error) {
      throw new Error(text);
    }
  }

  if (!response.ok || data.error) {
    throw new Error(data.error || `HTTP ${response.status}`);
  }
  return data;
}

function addLog(message, type = 'info') {
  const timestamp = new Date().toLocaleTimeString('zh-CN');
  const entry = document.createElement('div');
  entry.className = `log-entry log-${type}`;
  entry.innerHTML = `<span class="log-time">[${timestamp}]</span>${escapeHtml(message)}`;
  els.logs.prepend(entry);

  while (els.logs.children.length > 80) {
    els.logs.removeChild(els.logs.lastChild);
  }
}

function clearLogs() {
  els.logs.innerHTML = '';
  addLog('日志已清空。', 'info');
}

function escapeHtml(value) {
  return String(value ?? '').replace(/[&<>"']/g, char => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;'
  }[char]));
}

function escapeAttr(value) {
  return escapeHtml(value);
}

// ==================== DNS Management Page ====================

function loadDNSPage() {
  loadCFAccounts();
  loadDNSBindingsList();
  loadDNSRaw();
}

function renderCFAccountRow(a = {}) {
  const name = a.name || '';
  const remark = a.remark || '';
  const apiToken = a.api_token || '';
  const zoneId = a.zone_id || '';
  
  return `
    <div class="dns-cf-row cf-row-item">
      <div class="dns-cf-fields">
        <div class="field compact">
          <span>账号名称</span>
          <input type="text" class="cf-name-input" value="${escapeAttr(name)}" placeholder="例：cf01" ${name ? 'disabled' : ''}>
        </div>
        <div class="field compact">
          <span>备注</span>
          <input type="text" class="cf-remark-input" value="${escapeAttr(remark)}" placeholder="备注/说明">
        </div>
        <div class="field compact">
          <span>API Token</span>
          <input type="text" class="cf-token-input" value="${escapeAttr(apiToken)}" placeholder="API Token">
        </div>
        <div class="field compact">
          <span>Zone ID</span>
          <input type="text" class="cf-zone-input" value="${escapeAttr(zoneId)}" placeholder="Zone ID">
        </div>
      </div>
      <button class="ghost-btn dns-remove-btn cf-row-delete-btn" type="button">删除</button>
    </div>
  `;
}

function addCFAccountRow() {
  const el = document.getElementById('cf-accounts-list');
  if (el.querySelector('.empty-state')) {
    el.innerHTML = '';
  }
  const div = document.createElement('div');
  div.innerHTML = renderCFAccountRow();
  el.appendChild(div.firstElementChild);
}

async function loadCFAccounts() {
  const el = document.getElementById('cf-accounts-list');
  try {
    const accounts = await fetchJSON('/api/dns/cloudflare');
    if (!Array.isArray(accounts) || accounts.length === 0) {
      el.innerHTML = '<div class="empty-state compact">暂无 Cloudflare 账号配置。</div>';
      return;
    }
    el.innerHTML = accounts.map(a => renderCFAccountRow(a)).join('');
  } catch (err) {
    el.innerHTML = `<div class="empty-state compact error">${escapeHtml(err.message)}</div>`;
  }
}

async function saveCFAccounts() {
  const el = document.getElementById('cf-accounts-list');
  const rows = el.querySelectorAll('.cf-row-item');
  const accounts = [];
  
  for (const row of rows) {
    const nameInput = row.querySelector('.cf-name-input');
    const name = nameInput.value.trim();
    if (!name) continue;
    
    const remark = row.querySelector('.cf-remark-input').value.trim();
    const apiToken = row.querySelector('.cf-token-input').value.trim();
    const zoneId = row.querySelector('.cf-zone-input').value.trim();
    
    accounts.push({ name, remark, api_token: apiToken, zone_id: zoneId });
  }
  
  const msgEl = document.getElementById('cf-message');
  msgEl.textContent = '保存中...';
  msgEl.className = 'form-message';
  
  try {
    await fetchJSON('/api/dns/cloudflare', {
      method: 'POST',
      body: { accounts }
    });
    msgEl.textContent = 'Cloudflare 配置保存成功。';
    msgEl.className = 'form-message success';
    addLog('Cloudflare 配置已更新并自动重载。', 'success');
    loadDNSPage();
  } catch (err) {
    msgEl.textContent = err.message;
    msgEl.className = 'form-message error';
  }
}

async function loadDNSBindingsList() {
  const el = document.getElementById('dns-bindings-list');
  try {
    const bindings = await fetchJSON('/api/dns/bindings');
    if (!Array.isArray(bindings) || bindings.length === 0) {
      el.innerHTML = '<div class="empty-state compact">暂无 DNS 绑定。可在 VM 卡片中添加。</div>';
      return;
    }
    el.innerHTML = bindings.map(b => `
      <div class="dns-binding-card">
        <div class="dns-binding-info">
          <strong>${escapeHtml(b.name)}</strong>
          <span>${escapeHtml(b.provider)}/${escapeHtml(b.account)} → ${escapeHtml(b.domain)}</span>
          <span class="dns-binding-detail">CF: ${escapeHtml(b.cloudflare)} | VM: ${escapeHtml(b.vm)} | ${escapeHtml(b.type)} | TTL=${b.ttl} | Proxied=${b.proxied}</span>
        </div>
        <button class="ghost-btn dns-delete-binding-btn" type="button" data-name="${escapeAttr(b.name)}">删除</button>
      </div>
    `).join('');
  } catch (err) {
    el.innerHTML = `<div class="empty-state compact error">${escapeHtml(err.message)}</div>`;
  }
}

async function deleteDNSBindingByName(name) {
  if (!confirm(`确认删除 DNS 绑定「${name}」？`)) return;
  try {
    await fetchJSON('/api/dns/delete-binding', { method: 'POST', body: { name } });
    addLog(`已删除 DNS 绑定：${name}`, 'success');
    loadDNSBindingsList();
    loadDNSRaw();
    if (selectedAccount) fetchVMs();
  } catch (err) {
    addLog(`删除失败：${err.message}`, 'error');
  }
}

async function loadDNSRaw() {
  const el = document.getElementById('dns-raw-content');
  try {
    const data = await fetchJSON('/api/dns/raw');
    el.textContent = data.content || '（空）';
  } catch (err) {
    el.textContent = `加载失败：${err.message}`;
  }
}

async function reloadDNSConfig() {
  try {
    await fetchJSON('/api/config/reload', { method: 'POST' });
    addLog('DNS 配置已重载。', 'success');
    loadDNSPage();
  } catch (err) {
    addLog(`重载失败：${err.message}`, 'error');
  }
}

// Bind DNS page events
document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('dns-reload-btn')?.addEventListener('click', reloadDNSConfig);
  document.getElementById('add-cf-account-btn')?.addEventListener('click', addCFAccountRow);
  document.getElementById('save-cf-accounts-btn')?.addEventListener('click', saveCFAccounts);
  document.getElementById('dns-raw-refresh-btn')?.addEventListener('click', loadDNSRaw);
  
  // Delete binding delegation
  document.getElementById('dns-bindings-list')?.addEventListener('click', e => {
    const btn = e.target.closest('.dns-delete-binding-btn');
    if (btn) deleteDNSBindingByName(btn.dataset.name);
  });
  
  // Delete CF account row delegation
  document.getElementById('cf-accounts-list')?.addEventListener('click', e => {
    const btn = e.target.closest('.cf-row-delete-btn');
    if (btn) {
      const row = btn.closest('.cf-row-item');
      if (row) row.remove();
      const el = document.getElementById('cf-accounts-list');
      if (el.children.length === 0) {
        el.innerHTML = '<div class="empty-state compact">暂无 Cloudflare 账号配置。</div>';
      }
    }
  });
});

let dnsModalVM = null;
let dnsModalCFAccounts = [];

function openDNSBindingModal(vm) {
  dnsModalVM = vm;
  const modal = document.getElementById('dns-binding-modal');
  document.getElementById('dns-modal-title').textContent = `DNS 绑定 - ${vm.provider}/${vm.accountId}/${vm.id}`;
  document.getElementById('dns-modal-body').innerHTML = '<div class="empty-state compact">加载中...</div>';
  document.getElementById('dns-modal-message').textContent = '';
  modal.hidden = false;
  loadVMDNSBindings(vm);
}

function closeDNSModal() {
  document.getElementById('dns-binding-modal').hidden = true;
  dnsModalVM = null;
}

async function loadVMDNSBindings(vm) {
  try {
    const data = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/dns`);
    dnsModalCFAccounts = Array.isArray(data.cloudflare_accounts) ? data.cloudflare_accounts : [];
    const bindings = Array.isArray(data.bindings) ? data.bindings : [];
    renderDNSModalBindings(bindings);
  } catch (err) {
    document.getElementById('dns-modal-body').innerHTML = `<div class="empty-state compact error">${escapeHtml(err.message)}</div>`;
  }
}

function cfSelectHtml(selected) {
  return dnsModalCFAccounts.map(cf => `<option value="${escapeAttr(cf.name)}" ${cf.name === selected ? 'selected' : ''}>${escapeHtml(cf.name)}</option>`).join('');
}

function renderDNSModalBindings(bindings) {
  const body = document.getElementById('dns-modal-body');
  if (bindings.length === 0) {
    body.innerHTML = '<div class="empty-state compact">暂无 DNS 绑定，点击下方“添加绑定”创建。</div>';
    return;
  }
  body.innerHTML = bindings.map((b, i) => dnsBindingRowHtml(b, i)).join('');
}

function dnsBindingRowHtml(b, i) {
  return `<div class="dns-binding-row" data-index="${i}">
  <div class="dns-binding-fields">
    <label class="field compact"><span>绑定名称</span><input type="text" class="dns-f-name" value="${escapeAttr(b.name || '')}"></label>
    <label class="field compact"><span>Cloudflare</span><select class="dns-f-cf">${cfSelectHtml(b.cloudflare)}</select></label>
    <label class="field compact"><span>域名</span><input type="text" class="dns-f-domain" value="${escapeAttr(b.domain || '')}" placeholder="sub.example.com"></label>
    <label class="field compact"><span>类型</span><select class="dns-f-type"><option value="A" ${b.type==='A'?'selected':''}>A</option><option value="AAAA" ${b.type==='AAAA'?'selected':''}>AAAA</option><option value="CNAME" ${b.type==='CNAME'?'selected':''}>CNAME</option></select></label>
    <label class="field compact"><span>TTL</span><input type="number" class="dns-f-ttl" value="${b.ttl||1}" min="1"></label>
    <label class="switch-row compact"><input type="checkbox" class="dns-f-proxied" ${b.proxied?'checked':''}><span>Proxied</span></label>
  </div>
  <button class="ghost-btn dns-remove-btn" type="button" data-index="${i}">删除</button>
</div>`;
}

function addDNSBindingRow() {
  if (!dnsModalVM) return;
  const body = document.getElementById('dns-modal-body');
  if (body.querySelector('.empty-state')) body.innerHTML = '';
  const vm = dnsModalVM;
  const idx = body.querySelectorAll('.dns-binding-row').length;
  const name = `${vm.provider}-${vm.accountId}-${vm.id}-${idx}`;
  const cf = dnsModalCFAccounts.length > 0 ? dnsModalCFAccounts[0].name : '';
  body.insertAdjacentHTML('beforeend', dnsBindingRowHtml({ name, cloudflare: cf, domain: '', type: 'A', ttl: 1, proxied: false }, idx));
}

function removeDNSBindingRow(index) {
  const body = document.getElementById('dns-modal-body');
  const row = body.querySelector(`.dns-binding-row[data-index="${index}"]`);
  if (row) row.remove();
  if (!body.querySelector('.dns-binding-row')) {
    body.innerHTML = '<div class="empty-state compact">暂无 DNS 绑定</div>';
  }
}

async function saveDNSModal() {
  if (!dnsModalVM) return;
  const vm = dnsModalVM;
  const rows = document.getElementById('dns-modal-body').querySelectorAll('.dns-binding-row');
  const msgEl = document.getElementById('dns-modal-message');
  const bindings = [];
  for (const row of rows) {
    bindings.push({
      name: row.querySelector('.dns-f-name')?.value?.trim() || '',
      cloudflare: row.querySelector('.dns-f-cf')?.value || '',
      domain: row.querySelector('.dns-f-domain')?.value?.trim() || '',
      type: row.querySelector('.dns-f-type')?.value || 'A',
      ttl: Number(row.querySelector('.dns-f-ttl')?.value) || 1,
      proxied: row.querySelector('.dns-f-proxied')?.checked || false
    });
  }
  msgEl.textContent = '保存中...';
  msgEl.className = 'form-message';
  try {
    await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/dns`, { method: 'POST', body: { bindings } });
    msgEl.textContent = '已保存并重载生效。';
    msgEl.className = 'form-message success';
    addLog(`DNS 绑定已保存：${vm.provider}/${vm.accountId}/${vm.id}`, 'success');
    if (selectedAccount) fetchVMs();
  } catch (err) {
    msgEl.textContent = err.message;
    msgEl.className = 'form-message error';
  }
}

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('dns-modal-close')?.addEventListener('click', closeDNSModal);
  document.getElementById('dns-modal-add')?.addEventListener('click', addDNSBindingRow);
  document.getElementById('dns-modal-save')?.addEventListener('click', saveDNSModal);
  document.getElementById('dns-binding-modal')?.addEventListener('click', e => {
    if (e.target.id === 'dns-binding-modal') closeDNSModal();
  });
  document.getElementById('dns-modal-body')?.addEventListener('click', e => {
    const btn = e.target.closest('.dns-remove-btn');
    if (btn) removeDNSBindingRow(btn.dataset.index);
  });
});
