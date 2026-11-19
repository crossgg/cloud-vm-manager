// ==================== Notification Center & Interactive Telegram Bot ====================

let notifyState = {
  config: null,
  eventDefs: [],
  proxies: [],
  botStatus: {},
  logs: [],
  editingChannelId: '',
  bound: false,
};

const NOTIFY_CHANNEL_PARAM_SCHEMAS = {
  bark: [
    { key: 'bark_push', label: 'Bark 推送 Key 或完整 URL *', type: 'password', placeholder: '例如 https://api.day.app/your_key 或 your_key' },
    { key: 'bark_group', label: '消息分组 (可选)', type: 'text', placeholder: 'cloud-vm' },
    { key: 'bark_sound', label: '提示音 (可选)', type: 'text', placeholder: 'alarm' },
    { key: 'bark_level', label: '时效级别 (可选)', type: 'text', placeholder: 'active / timeSensitive / passive' },
    { key: 'bark_url', label: '跳转 URL (可选)', type: 'text', placeholder: 'https://...' },
  ],
  wecom_bot: [
    { key: 'qywx_key', label: '企业微信机器人 Webhook Key *', type: 'password', placeholder: 'Webhook 地址中的 key= 参数' },
    { key: 'qywx_origin', label: '企微接口反代 Origin (可选)', type: 'text', placeholder: '默认 https://qyapi.weixin.qq.com' },
  ],
  wecom_app: [
    { key: 'qywx_am', label: '企微应用参数 QYWX_AM *', type: 'password', placeholder: 'corpid,corpsecret,touser,agentid[,media_id]' },
    { key: 'qywx_origin', label: '企微接口反代 Origin (可选)', type: 'text', placeholder: '默认 https://qyapi.weixin.qq.com' },
  ],
  feishu: [
    { key: 'fskey', label: '飞书 Webhook Key 或完整 URL *', type: 'password', placeholder: 'hook/ 后面的 key 或完整 URL' },
    { key: 'fssecret', label: '飞书签名密钥 Secret (可选)', type: 'password', placeholder: '开启签名校验时填写' },
  ],
  telegram: [
    { key: 'tg_bot_token', label: 'Telegram Bot Token *', type: 'password', placeholder: '123456:ABC-DEF...' },
    { key: 'tg_user_id', label: '目标 Chat / User ID *', type: 'text', placeholder: '123456789 或 -100xxxx' },
    { key: 'tg_api_host', label: 'Telegram API Host (可选)', type: 'text', placeholder: '默认 https://api.telegram.org' },
  ],
  pushplus: [
    { key: 'push_plus_token', label: 'PushPlus Token *', type: 'password', placeholder: 'PushPlus Token' },
    { key: 'push_plus_channel', label: '发送渠道 (可选)', type: 'text', placeholder: 'wechat / webhook / cp / mail' },
    { key: 'push_plus_template', label: '消息模板 (可选)', type: 'text', placeholder: 'txt / html / markdown' },
    { key: 'push_plus_user', label: '群组编码 topic (可选)', type: 'text', placeholder: '一对多推送群组编码' },
  ],
  custom: [
    { key: 'webhook_url', label: 'Webhook URL *（支持 $title / $content）', type: 'text', placeholder: 'https://example.com/hook' },
    { key: 'webhook_method', label: '请求方法', type: 'select', options: ['POST', 'GET', 'PUT', 'PATCH'], defaultValue: 'POST' },
    { key: 'webhook_content_type', label: 'Content-Type', type: 'text', placeholder: 'application/json', defaultValue: 'application/json' },
    { key: 'webhook_body', label: '请求体模板（支持 $title / $content）', type: 'textarea', placeholder: '{"title":"$title","content":"$content"}', defaultValue: '{"title":"$title","content":"$content"}' },
    { key: 'webhook_headers', label: '自定义请求头（每行一个 Header: Value）', type: 'textarea', placeholder: 'Authorization: Bearer xxx' },
  ],
};

function bindNotifyEventsOnce() {
  if (notifyState.bound) return;
  notifyState.bound = true;

  document.getElementById('notify-refresh-btn')?.addEventListener('click', () => loadNotifyPage());
  document.getElementById('notify-save-all-btn')?.addEventListener('click', () => saveNotifyAllConfig());
  document.getElementById('tg-bot-test-conn-btn')?.addEventListener('click', () => testTelegramBot(false));
  document.getElementById('tg-bot-test-send-btn')?.addEventListener('click', () => testTelegramBot(true));
  document.getElementById('notify-new-channel-btn')?.addEventListener('click', () => resetNotifyChannelForm());
  document.getElementById('notify-ch-reset-btn')?.addEventListener('click', () => resetNotifyChannelForm());
  document.getElementById('notify-ch-type')?.addEventListener('change', () => renderChannelParamFields({}));
  document.getElementById('notify-channel-form')?.addEventListener('submit', saveNotifyChannelFromForm);
  document.getElementById('notify-ch-test-btn')?.addEventListener('click', () => testNotifyChannelFromForm());
  document.getElementById('notify-channels-list')?.addEventListener('click', handleNotifyChannelListClick);
  document.getElementById('notify-logs-refresh-btn')?.addEventListener('click', () => refreshNotifyLogs());
}

async function loadNotifyPage() {
  bindNotifyEventsOnce();
  try {
    const data = await fetchJSON('/api/notify/config');
    notifyState.config = data.config || {};
    notifyState.eventDefs = Array.isArray(data.eventDefs) ? data.eventDefs : [];
    notifyState.proxies = Array.isArray(data.proxies) ? data.proxies : [];
    notifyState.botStatus = data.botStatus || {};
    notifyState.logs = Array.isArray(data.logs) ? data.logs : [];

    const summaryEl = document.getElementById('notify-config-summary');
    if (summaryEl) {
      summaryEl.textContent = `配置文件：${data.configPath || 'config/notify.conf'} | 已配置渠道：${(notifyState.config.channels || []).length} 个`;
    }

    renderGlobalEventCheckboxes();
    renderTelegramBotPanel();
    renderNotifyChannelsList();
    if (!notifyState.editingChannelId) {
      resetNotifyChannelForm();
    }
    renderNotifyLogs();
  } catch (err) {
    setNotifyMessage('notify-global-message', `加载通知配置失败: ${err.message}`, 'error');
  }
}

function renderGlobalEventCheckboxes() {
  const wrap = document.getElementById('notify-global-events');
  if (!wrap) return;
  const enabled = new Set(notifyState.config?.enabledEvents || []);
  wrap.innerHTML = notifyState.eventDefs.map(def => `
    <label class="notify-trigger-card">
      <div class="notify-trigger-head">
        <input type="checkbox" class="notify-global-ev-cb" value="${escapeAttr(def.id)}" ${enabled.has(def.id) ? 'checked' : ''}>
        <strong>${escapeHtml(def.label)}</strong>
      </div>
      <span>${escapeHtml(def.description)}</span>
    </label>
  `).join('');
}

function renderEventCheckboxesInto(containerId, className, selectedList) {
  const wrap = document.getElementById(containerId);
  if (!wrap) return;
  const selected = new Set(selectedList || []);
  wrap.innerHTML = notifyState.eventDefs.map(def => `
    <label class="switch-row compact">
      <input type="checkbox" class="${escapeAttr(className)}" value="${escapeAttr(def.id)}" ${selected.has(def.id) ? 'checked' : ''}>
      <span>${escapeHtml(def.label)}</span>
    </label>
  `).join('');
}

function populateProxySelect(selectEl, selectedVal, includeDirectOption = false) {
  if (!selectEl) return;
  const options = [];
  if (includeDirectOption) {
    options.push(`<option value="none">无备用</option>`);
    options.push(`<option value="direct">直连 (Direct)</option>`);
  } else {
    options.push(`<option value="none">直连（不使用代理）</option>`);
  }
  for (const p of notifyState.proxies) {
    const label = p.remark ? `${p.remark} (${p.url})` : p.url;
    options.push(`<option value="${escapeAttr(p.id)}">${escapeHtml(label)}</option>`);
  }
  selectEl.innerHTML = options.join('');
  selectEl.value = selectedVal || 'none';
  if (!selectEl.value) {
    selectEl.value = 'none';
  }
}

function renderTelegramBotPanel() {
  const bot = notifyState.config?.telegramBot || {};
  const st = notifyState.botStatus || {};

  const badge = document.getElementById('tg-bot-status-badge');
  const meta = document.getElementById('tg-bot-status-meta');
  if (badge) {
    badge.className = 'notify-badge';
    if (!st.enabled) {
      badge.textContent = '未启用';
    } else if (st.connected) {
      badge.classList.add('ok');
      badge.textContent = st.botUsername ? `在线 @${st.botUsername}` : '在线运行中';
    } else if (st.lastError) {
      badge.classList.add('err');
      badge.textContent = '连接异常';
    } else {
      badge.classList.add('warn');
      badge.textContent = st.running ? '连接中...' : '未启动';
    }
  }
  if (meta) {
    const parts = [];
    if (st.lastPollAt) parts.push(`最近轮询: ${st.lastPollAt}`);
    if (st.updatesProcessed) parts.push(`已处理更新: ${st.updatesProcessed}`);
    if (st.whitelistWarning) parts.push(`⚠️ ${st.whitelistWarning}`);
    if (st.lastError) parts.push(`错误: ${st.lastError}`);
    meta.textContent = parts.join(' | ');
  }

  document.getElementById('tg-bot-enabled').checked = !!bot.enabled;
  document.getElementById('tg-bot-auto-notify').checked = bot.autoNotify !== false;
  document.getElementById('tg-bot-token').value = bot.botToken || '';
  document.getElementById('tg-bot-api-host').value = bot.apiHost || 'https://api.telegram.org';
  populateProxySelect(document.getElementById('tg-bot-proxy'), bot.proxyId || 'none', false);
  populateProxySelect(document.getElementById('tg-bot-fallback'), bot.fallbackProxyId || 'none', true);
  document.getElementById('tg-bot-allowed-users').value = (bot.allowedUserIds || []).join(',');
  document.getElementById('tg-bot-allowed-chats').value = (bot.allowedChatIds || []).join(',');
  document.getElementById('tg-bot-notify-chats').value = (bot.notifyChatIds || []).join(',');
  document.getElementById('tg-bot-notify-cooldown').value = bot.notifyCooldownSec ?? 300;
  renderEventCheckboxesInto('tg-bot-notify-events', 'tg-bot-ev-cb', bot.notifyEvents || []);
}

function collectBotFormConfig() {
  return {
    enabled: !!document.getElementById('tg-bot-enabled')?.checked,
    botToken: (document.getElementById('tg-bot-token')?.value || '').trim(),
    apiHost: (document.getElementById('tg-bot-api-host')?.value || '').trim() || 'https://api.telegram.org',
    proxyId: document.getElementById('tg-bot-proxy')?.value || 'none',
    fallbackProxyId: document.getElementById('tg-bot-fallback')?.value || 'none',
    allowedUserIds: parseIntCSV(document.getElementById('tg-bot-allowed-users')?.value || ''),
    allowedChatIds: parseIntCSV(document.getElementById('tg-bot-allowed-chats')?.value || ''),
    autoNotify: !!document.getElementById('tg-bot-auto-notify')?.checked,
    notifyChatIds: parseIntCSV(document.getElementById('tg-bot-notify-chats')?.value || ''),
    notifyCooldownSec: parseInt(document.getElementById('tg-bot-notify-cooldown')?.value || '300', 10) || 0,
    notifyEvents: Array.from(document.querySelectorAll('.tg-bot-ev-cb:checked')).map(el => el.value),
  };
}

function collectGlobalEvents() {
  return Array.from(document.querySelectorAll('.notify-global-ev-cb:checked')).map(el => el.value);
}

async function saveNotifyAllConfig(msgTargetId = 'notify-global-message') {
  try {
    const payload = {
      enabledEvents: collectGlobalEvents(),
      telegramBot: collectBotFormConfig(),
      channels: notifyState.config?.channels || [],
    };
    const res = await fetchJSON('/api/notify/config', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    notifyState.config = res.config || payload;
    notifyState.botStatus = res.botStatus || {};
    renderTelegramBotPanel();
    renderNotifyChannelsList();
    setNotifyMessage(msgTargetId, '通知与 Telegram Bot 配置已保存并热重载生效。', 'success');
    addLog('已保存通知与 Telegram Bot 配置。', 'success');
  } catch (err) {
    setNotifyMessage(msgTargetId, `保存失败: ${err.message}`, 'error');
  }
}

async function testTelegramBot(sendMessage) {
  setNotifyMessage('tg-bot-message', sendMessage ? '正在检测连通并发送测试消息...' : '正在检测 Bot 连通性...', '');
  try {
    const res = await fetchJSON('/api/notify/test-bot', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        bot: collectBotFormConfig(),
        sendMessage: !!sendMessage,
      }),
    });
    setNotifyMessage('tg-bot-message', res.message || '测试成功', 'success');
    refreshNotifyLogs();
  } catch (err) {
    setNotifyMessage('tg-bot-message', `测试失败: ${err.message}`, 'error');
    refreshNotifyLogs();
  }
}

function renderNotifyChannelsList() {
  const listEl = document.getElementById('notify-channels-list');
  if (!listEl) return;
  const channels = notifyState.config?.channels || [];
  if (!channels.length) {
    listEl.innerHTML = '<div class="empty-state compact">暂无单向通知渠道，可在下方添加。</div>';
    return;
  }
  const proxyMap = new Map(notifyState.proxies.map(p => [p.id, p.remark || p.url]));
  listEl.innerHTML = channels.map(ch => {
    const proxyLabel = ch.proxyId && ch.proxyId !== 'none' ? (proxyMap.get(ch.proxyId) || ch.proxyId) : '直连';
    const evCount = (ch.events || []).length;
    return `
      <div class="notify-channel-row" data-id="${escapeAttr(ch.id)}">
        <div class="notify-channel-main">
          <div class="notify-channel-title">
            <strong>${escapeHtml(ch.name || ch.id)}</strong>
            <span class="notify-tag">${escapeHtml(ch.type)}</span>
            <span class="notify-badge ${ch.enabled ? 'ok' : ''}">${ch.enabled ? '已启用' : '已停用'}</span>
          </div>
          <div class="notify-channel-sub">
            代理: ${escapeHtml(proxyLabel)} | 订阅触发点: ${evCount} 个 | 冷却: ${ch.cooldownSec || 0}s
          </div>
        </div>
        <div class="notify-channel-actions">
          <button type="button" class="secondary-btn oci-ip-btn-sm" data-action="test" data-id="${escapeAttr(ch.id)}">测试</button>
          <button type="button" class="secondary-btn oci-ip-btn-sm" data-action="edit" data-id="${escapeAttr(ch.id)}">编辑</button>
          <button type="button" class="ghost-btn oci-ip-btn-sm" data-action="delete" data-id="${escapeAttr(ch.id)}">删除</button>
        </div>
      </div>
    `;
  }).join('');
}

function resetNotifyChannelForm() {
  notifyState.editingChannelId = '';
  document.getElementById('notify-ch-id').value = '';
  document.getElementById('notify-ch-editor-title').textContent = '添加通知渠道';
  document.getElementById('notify-ch-name').value = '';
  document.getElementById('notify-ch-type').value = 'bark';
  document.getElementById('notify-ch-enabled').checked = true;
  document.getElementById('notify-ch-cooldown').value = 300;
  document.getElementById('notify-ch-accounts').value = '';
  populateProxySelect(document.getElementById('notify-ch-proxy'), 'none', false);
  populateProxySelect(document.getElementById('notify-ch-fallback'), 'none', true);
  renderEventCheckboxesInto('notify-ch-events', 'notify-ch-ev-cb', notifyState.config?.enabledEvents || []);
  renderChannelParamFields({});
  setNotifyMessage('notify-ch-message', '', '');
}

function editNotifyChannel(id) {
  const ch = (notifyState.config?.channels || []).find(item => item.id === id);
  if (!ch) return;
  notifyState.editingChannelId = ch.id;
  document.getElementById('notify-ch-id').value = ch.id;
  document.getElementById('notify-ch-editor-title').textContent = `编辑通知渠道：${ch.name || ch.id}`;
  document.getElementById('notify-ch-name').value = ch.name || '';
  document.getElementById('notify-ch-type').value = ch.type || 'bark';
  document.getElementById('notify-ch-enabled').checked = !!ch.enabled;
  document.getElementById('notify-ch-cooldown').value = ch.cooldownSec ?? 300;
  document.getElementById('notify-ch-accounts').value = (ch.accounts || []).join(',');
  populateProxySelect(document.getElementById('notify-ch-proxy'), ch.proxyId || 'none', false);
  populateProxySelect(document.getElementById('notify-ch-fallback'), ch.fallbackProxyId || 'none', true);
  renderEventCheckboxesInto('notify-ch-events', 'notify-ch-ev-cb', ch.events || []);
  renderChannelParamFields(ch.params || {});
  setNotifyMessage('notify-ch-message', '', '');
  document.getElementById('notify-channel-form')?.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
}

function renderChannelParamFields(existingParams) {
  const wrap = document.getElementById('notify-ch-params');
  if (!wrap) return;
  const chType = document.getElementById('notify-ch-type')?.value || 'bark';
  const schema = NOTIFY_CHANNEL_PARAM_SCHEMAS[chType] || [];
  wrap.innerHTML = schema.map(field => {
    const val = existingParams[field.key] ?? field.defaultValue ?? '';
    if (field.type === 'select') {
      const opts = (field.options || []).map(o => `<option value="${escapeAttr(o)}" ${val === o ? 'selected' : ''}>${escapeHtml(o)}</option>`).join('');
      return `
        <label class="field">
          <span>${escapeHtml(field.label)}</span>
          <select class="notify-ch-param-input" data-key="${escapeAttr(field.key)}">${opts}</select>
        </label>
      `;
    }
    if (field.type === 'textarea') {
      return `
        <label class="field" style="grid-column: 1 / -1;">
          <span>${escapeHtml(field.label)}</span>
          <textarea class="notify-ch-param-input" data-key="${escapeAttr(field.key)}" rows="3" placeholder="${escapeAttr(field.placeholder || '')}">${escapeHtml(val)}</textarea>
        </label>
      `;
    }
    return `
      <label class="field">
        <span>${escapeHtml(field.label)}</span>
        <input class="notify-ch-param-input" data-key="${escapeAttr(field.key)}" type="${field.type === 'password' ? 'password' : 'text'}" autocomplete="off" value="${escapeAttr(val)}" placeholder="${escapeAttr(field.placeholder || '')}">
      </label>
    `;
  }).join('');
}

function collectChannelFromForm() {
  const params = {};
  document.querySelectorAll('.notify-ch-param-input').forEach(el => {
    const key = el.dataset.key;
    if (key) {
      params[key] = el.value;
    }
  });
  return {
    id: (document.getElementById('notify-ch-id')?.value || '').trim(),
    name: (document.getElementById('notify-ch-name')?.value || '').trim(),
    type: document.getElementById('notify-ch-type')?.value || 'bark',
    enabled: !!document.getElementById('notify-ch-enabled')?.checked,
    proxyId: document.getElementById('notify-ch-proxy')?.value || 'none',
    fallbackProxyId: document.getElementById('notify-ch-fallback')?.value || 'none',
    cooldownSec: parseInt(document.getElementById('notify-ch-cooldown')?.value || '300', 10) || 0,
    accounts: (document.getElementById('notify-ch-accounts')?.value || '').split(',').map(s => s.trim()).filter(Boolean),
    events: Array.from(document.querySelectorAll('.notify-ch-ev-cb:checked')).map(el => el.value),
    params,
  };
}

async function saveNotifyChannelFromForm(event) {
  event.preventDefault();
  const ch = collectChannelFromForm();
  if (!ch.name) {
    setNotifyMessage('notify-ch-message', '请填写渠道名称', 'error');
    return;
  }
  const list = Array.isArray(notifyState.config?.channels) ? [...notifyState.config.channels] : [];
  if (ch.id) {
    const idx = list.findIndex(item => item.id === ch.id);
    if (idx >= 0) {
      list[idx] = ch;
    } else {
      list.push(ch);
    }
  } else {
    list.push(ch);
  }
  notifyState.config = { ...(notifyState.config || {}), channels: list };
  await saveNotifyAllConfig('notify-ch-message');
  resetNotifyChannelForm();
}

async function testNotifyChannelFromForm() {
  const ch = collectChannelFromForm();
  setNotifyMessage('notify-ch-message', '正在发送测试通知...', '');
  try {
    const res = await fetchJSON('/api/notify/test-channel', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(ch),
    });
    setNotifyMessage('notify-ch-message', res.message || '测试发送成功', 'success');
    refreshNotifyLogs();
  } catch (err) {
    setNotifyMessage('notify-ch-message', `测试失败: ${err.message}`, 'error');
    refreshNotifyLogs();
  }
}

async function handleNotifyChannelListClick(event) {
  const btn = event.target.closest('button[data-action]');
  if (!btn) return;
  const action = btn.dataset.action;
  const id = btn.dataset.id;
  if (action === 'edit') {
    editNotifyChannel(id);
    return;
  }
  if (action === 'delete') {
    if (!confirm('确定删除该通知渠道？')) return;
    const list = (notifyState.config?.channels || []).filter(c => c.id !== id);
    notifyState.config = { ...(notifyState.config || {}), channels: list };
    if (notifyState.editingChannelId === id) {
      resetNotifyChannelForm();
    }
    await saveNotifyAllConfig('notify-global-message');
    return;
  }
  if (action === 'test') {
    const ch = (notifyState.config?.channels || []).find(c => c.id === id);
    if (!ch) return;
    setNotifyMessage('notify-global-message', `正在测试渠道「${ch.name || ch.id}」...`, '');
    try {
      const res = await fetchJSON('/api/notify/test-channel', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(ch),
      });
      setNotifyMessage('notify-global-message', `「${ch.name || ch.id}」${res.message || '测试成功'}`, 'success');
      refreshNotifyLogs();
    } catch (err) {
      setNotifyMessage('notify-global-message', `「${ch.name || ch.id}」测试失败: ${err.message}`, 'error');
      refreshNotifyLogs();
    }
  }
}

async function refreshNotifyLogs() {
  try {
    const data = await fetchJSON('/api/notify/logs');
    notifyState.logs = Array.isArray(data.logs) ? data.logs : [];
    if (data.botStatus) {
      notifyState.botStatus = data.botStatus;
      renderTelegramBotPanel();
    }
    renderNotifyLogs();
  } catch (_) {}
}

function renderNotifyLogs() {
  const wrap = document.getElementById('notify-logs-list');
  if (!wrap) return;
  if (!notifyState.logs.length) {
    wrap.innerHTML = '<div class="empty-state compact">暂无推送记录。</div>';
    return;
  }
  wrap.innerHTML = notifyState.logs.map(item => `
    <div class="notify-log-row">
      <div class="notify-log-head">
        <span class="notify-badge ${item.success ? 'ok' : 'err'}">${item.success ? '成功' : '失败'}</span>
        <strong>${escapeHtml(item.target)}</strong>
        <span class="notify-tag">${escapeHtml(item.channelType)}</span>
        <span class="notify-tag">${escapeHtml(item.eventType)}</span>
        <span class="notify-log-time">${escapeHtml(item.time)}</span>
      </div>
      <div class="notify-log-body">
        ${escapeHtml(item.title)}${item.error ? ` — <span class="notify-err-text">${escapeHtml(item.error)}</span>` : ''}
      </div>
    </div>
  `).join('');
}

function parseIntCSV(raw) {
  return raw.split(',')
    .map(s => s.trim())
    .filter(Boolean)
    .map(s => Number(s))
    .filter(n => Number.isSafeInteger(n) && n !== 0);
}

function setNotifyMessage(id, text, type) {
  const el = document.getElementById(id);
  if (!el) return;
  el.textContent = text || '';
  el.className = 'form-message' + (type ? ` ${type}` : '');
}

document.addEventListener('DOMContentLoaded', () => {
  document.querySelector('.nav-item[data-section="notify"]')?.addEventListener('click', () => {
    loadNotifyPage();
  });
});
