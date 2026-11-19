// ==================== Notification Center & Interactive Telegram Bot ====================

let notifyState = {
  config: null,
  eventDefs: [],
  proxies: [],
  botStatus: {},
  logs: [],
  activeTab: 'triggers',
  selectedChannelId: '',
  creatingChannel: false,
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
  document.getElementById('notify-open-logs-btn')?.addEventListener('click', openNotifyLogsModal);
  document.getElementById('notify-logs-modal-close')?.addEventListener('click', closeNotifyLogsModal);
  document.getElementById('notify-logs-modal-done')?.addEventListener('click', closeNotifyLogsModal);
  document.getElementById('notify-logs-modal')?.addEventListener('click', (e) => {
    if (e.target.id === 'notify-logs-modal') closeNotifyLogsModal();
  });
  document.getElementById('notify-logs-refresh-btn')?.addEventListener('click', () => refreshNotifyLogs());

  document.getElementById('notify-workspace-tabs')?.addEventListener('click', (e) => {
    const tabBtn = e.target.closest('[data-notify-tab]');
    if (!tabBtn) return;
    switchNotifyTab(tabBtn.dataset.notifyTab);
  });

  // Tab 1: Triggers
  document.getElementById('notify-triggers-select-all-btn')?.addEventListener('click', () => {
    document.querySelectorAll('.notify-global-ev-cb').forEach(cb => { cb.checked = true; });
  });
  document.getElementById('notify-triggers-default-btn')?.addEventListener('click', () => {
    const defMap = new Map(notifyState.eventDefs.map(d => [d.id, !!d.defaultOn]));
    document.querySelectorAll('.notify-global-ev-cb').forEach(cb => {
      cb.checked = !!defMap.get(cb.value);
    });
  });
  document.getElementById('notify-save-triggers-btn')?.addEventListener('click', () => saveTriggersOnly());

  // Tab 2: Telegram Bot
  document.getElementById('notify-save-bot-btn')?.addEventListener('click', () => saveTelegramBotOnly());
  document.getElementById('tg-bot-test-conn-btn')?.addEventListener('click', () => testTelegramBot(false));
  document.getElementById('tg-bot-test-send-btn')?.addEventListener('click', () => testTelegramBot(true));

  // Tab 3: One-way Channels (vertical rail + right editor)
  document.getElementById('notify-new-channel-btn')?.addEventListener('click', () => startNewNotifyChannel());
  document.getElementById('notify-channels-tabs')?.addEventListener('click', (e) => {
    const btn = e.target.closest('.notify-vtab[data-channel-id]');
    if (!btn) return;
    selectNotifyChannel(btn.dataset.channelId);
  });
  document.getElementById('notify-ch-type')?.addEventListener('change', () => renderChannelParamFields({}));
  document.getElementById('notify-channel-form')?.addEventListener('submit', saveNotifyChannelFromForm);
  document.getElementById('notify-ch-test-btn')?.addEventListener('click', () => testNotifyChannelFromForm());
  document.getElementById('notify-ch-delete-btn')?.addEventListener('click', () => deleteSelectedNotifyChannel());
}

function switchNotifyTab(tabName) {
  if (!['triggers', 'bot', 'channels'].includes(tabName)) tabName = 'triggers';
  notifyState.activeTab = tabName;

  document.querySelectorAll('#notify-workspace-tabs [data-notify-tab]').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.notifyTab === tabName);
  });
  document.querySelectorAll('#notify-section .notify-tab-pane').forEach(pane => {
    pane.hidden = pane.id !== `notify-tab-${tabName}`;
  });
}

async function loadNotifyPage() {
  bindNotifyEventsOnce();
  switchNotifyTab(notifyState.activeTab || 'triggers');
  try {
    const data = await fetchJSON('/api/notify/config');
    notifyState.config = data.config || {};
    notifyState.eventDefs = Array.isArray(data.eventDefs) ? data.eventDefs : [];
    notifyState.proxies = Array.isArray(data.proxies) ? data.proxies : [];
    notifyState.botStatus = data.botStatus || {};
    notifyState.logs = Array.isArray(data.logs) ? data.logs : [];

    const summaryEl = document.getElementById('notify-config-summary');
    if (summaryEl) {
      const chCount = (notifyState.config.channels || []).length;
      const evCount = (notifyState.config.enabledEvents || []).length;
      summaryEl.textContent = `配置文件：${data.configPath || 'config/notify.conf'} | 已开启触发点：${evCount} 个 | 单向渠道：${chCount} 个`;
    }

    renderGlobalEventCheckboxes();
    renderTelegramBotPanel();
    syncChannelSelectionAfterLoad();
    renderNotifyLogs();
  } catch (err) {
    setNotifyMessage('notify-triggers-message', `加载通知配置失败: ${err.message}`, 'error');
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

async function persistNotifyPayload(payload, msgTargetId, successText) {
  const res = await fetchJSON('/api/notify/config', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  });
  notifyState.config = res.config || payload;
  notifyState.botStatus = res.botStatus || {};
  const summaryEl = document.getElementById('notify-config-summary');
  if (summaryEl) {
    const chCount = (notifyState.config.channels || []).length;
    const evCount = (notifyState.config.enabledEvents || []).length;
    summaryEl.textContent = `配置文件：config/notify.conf | 已开启触发点：${evCount} 个 | 单向渠道：${chCount} 个`;
  }
  renderTelegramBotPanel();
  setNotifyMessage(msgTargetId, successText, 'success');
  addLog(successText, 'success');
}

async function saveTriggersOnly() {
  try {
    const cur = notifyState.config || {};
    const payload = {
      enabledEvents: collectGlobalEvents(),
      telegramBot: cur.telegramBot || {},
      channels: cur.channels || [],
    };
    await persistNotifyPayload(payload, 'notify-triggers-message', '全局通知触发点配置已保存并生效。');
  } catch (err) {
    setNotifyMessage('notify-triggers-message', `保存失败: ${err.message}`, 'error');
  }
}

async function saveTelegramBotOnly() {
  try {
    const cur = notifyState.config || {};
    const payload = {
      enabledEvents: cur.enabledEvents || [],
      telegramBot: collectBotFormConfig(),
      channels: cur.channels || [],
    };
    await persistNotifyPayload(payload, 'tg-bot-message', '交互式 Telegram Bot 配置已保存并热重载。');
  } catch (err) {
    setNotifyMessage('tg-bot-message', `保存失败: ${err.message}`, 'error');
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

function syncChannelSelectionAfterLoad() {
  const channels = notifyState.config?.channels || [];
  if (notifyState.creatingChannel) {
    renderVerticalChannelTabs();
    return;
  }
  if (notifyState.selectedChannelId && channels.some(c => c.id === notifyState.selectedChannelId)) {
    selectNotifyChannel(notifyState.selectedChannelId);
    return;
  }
  if (channels.length > 0) {
    selectNotifyChannel(channels[0].id);
    return;
  }
  startNewNotifyChannel();
}

function renderVerticalChannelTabs() {
  const railEl = document.getElementById('notify-channels-tabs');
  if (!railEl) return;
  const channels = notifyState.config?.channels || [];
  if (!channels.length) {
    railEl.innerHTML = '<div class="empty-state compact">暂无渠道，请填写右侧表单添加</div>';
    return;
  }
  railEl.innerHTML = channels.map(ch => {
    const active = !notifyState.creatingChannel && notifyState.selectedChannelId === ch.id;
    return `
      <button type="button" class="notify-vtab ${active ? 'active' : ''}" data-channel-id="${escapeAttr(ch.id)}">
        <span class="notify-vtab-dot ${ch.enabled ? 'on' : ''}"></span>
        <span class="notify-vtab-name">${escapeHtml(ch.name || ch.id)}</span>
        <span class="notify-tag">${escapeHtml(ch.type)}</span>
      </button>
    `;
  }).join('');
}

function startNewNotifyChannel() {
  notifyState.creatingChannel = true;
  notifyState.selectedChannelId = '';
  renderVerticalChannelTabs();

  document.getElementById('notify-ch-id').value = '';
  document.getElementById('notify-ch-editor-title').textContent = '添加通知渠道';
  document.getElementById('notify-ch-delete-btn').hidden = true;
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

function selectNotifyChannel(id) {
  const ch = (notifyState.config?.channels || []).find(item => item.id === id);
  if (!ch) return;
  notifyState.creatingChannel = false;
  notifyState.selectedChannelId = ch.id;
  renderVerticalChannelTabs();

  document.getElementById('notify-ch-id').value = ch.id;
  document.getElementById('notify-ch-editor-title').textContent = ch.name || ch.id;
  document.getElementById('notify-ch-delete-btn').hidden = false;
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
  const cur = notifyState.config || {};
  const list = Array.isArray(cur.channels) ? [...cur.channels] : [];
  let savedIdx = -1;
  if (ch.id) {
    savedIdx = list.findIndex(item => item.id === ch.id);
    if (savedIdx >= 0) {
      list[savedIdx] = ch;
    } else {
      list.push(ch);
      savedIdx = list.length - 1;
    }
  } else {
    list.push(ch);
    savedIdx = list.length - 1;
  }

  try {
    const payload = {
      enabledEvents: cur.enabledEvents || [],
      telegramBot: cur.telegramBot || {},
      channels: list,
    };
    await persistNotifyPayload(payload, 'notify-ch-message', `渠道「${ch.name}」已保存并生效。`);
    const updatedList = notifyState.config?.channels || [];
    const target = updatedList[savedIdx] || updatedList[updatedList.length - 1];
    if (target) {
      selectNotifyChannel(target.id);
      setNotifyMessage('notify-ch-message', `渠道「${target.name || target.id}」已保存并生效。`, 'success');
    } else {
      renderVerticalChannelTabs();
    }
  } catch (err) {
    setNotifyMessage('notify-ch-message', `保存失败: ${err.message}`, 'error');
  }
}

async function deleteSelectedNotifyChannel() {
  const id = notifyState.selectedChannelId;
  if (!id) return;
  const ch = (notifyState.config?.channels || []).find(item => item.id === id);
  if (!confirm(`确定删除通知渠道「${ch?.name || id}」？`)) return;

  try {
    const cur = notifyState.config || {};
    const nextList = (cur.channels || []).filter(item => item.id !== id);
    await persistNotifyPayload({
      enabledEvents: cur.enabledEvents || [],
      telegramBot: cur.telegramBot || {},
      channels: nextList,
    }, 'notify-ch-message', `已删除通知渠道「${ch?.name || id}」。`);
    notifyState.selectedChannelId = '';
    syncChannelSelectionAfterLoad();
  } catch (err) {
    setNotifyMessage('notify-ch-message', `删除失败: ${err.message}`, 'error');
  }
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

function openNotifyLogsModal() {
  const modal = document.getElementById('notify-logs-modal');
  if (modal) modal.hidden = false;
  refreshNotifyLogs();
}

function closeNotifyLogsModal() {
  const modal = document.getElementById('notify-logs-modal');
  if (modal) modal.hidden = true;
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
