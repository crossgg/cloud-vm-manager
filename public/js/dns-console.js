(() => {
  'use strict';

  const dnsConsoleState = {
    initialized: false,
    activeTab: 'console', // 'console' | 'bindings' | 'accounts'
    accounts: [],
    selectedAccount: '',
    domains: [],
    selectedDomain: '',
    capabilities: {
      provider: 'cloudflare',
      supportsProxied: true,
      supportsLine: false,
      supportsStatus: false,
      supportsRemark: true,
      defaultLine: '',
      availableLines: [],
    },
    records: [],
    searchQuery: '',
    typeFilter: 'ALL',
    editingRecordId: '',
    editingAccountOriginalName: '',
    bindings: [],
    previewData: null,
  };

  function el(id) {
    return document.getElementById(id);
  }

  function escapeHTML(str) {
    return String(str ?? '')
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function providerDisplayName(provider, edition) {
    const p = String(provider || 'cloudflare').toLowerCase();
    const ed = String(edition || '').toLowerCase();
    switch (p) {
      case 'cloudflare':
        return 'Cloudflare';
      case 'alidns':
        return '阿里云 DNS';
      case 'tencent':
        return ed === 'intl' ? '腾讯云国际版 (DNSPod)' : '腾讯云国内版 (DNSPod)';
      case 'dnspod':
        return ed === 'intl' ? 'DNSPod 国际版' : 'DNSPod 国内版';
      default:
        return provider || 'Unknown';
    }
  }

  function providerShortLabel(provider, edition) {
    const p = String(provider || 'cloudflare').toLowerCase();
    const ed = String(edition || '').toLowerCase();
    if (p === 'cloudflare') return 'Cloudflare';
    if (p === 'alidns') return '阿里云 DNS';
    if (p === 'tencent') return ed === 'intl' ? '腾讯云 Intl' : '腾讯云 CN';
    if (p === 'dnspod') return ed === 'intl' ? 'DNSPod Intl' : 'DNSPod CN';
    return p;
  }

  function showMessage(targetId, text, isError = false) {
    const box = el(targetId);
    if (!box) return;
    box.textContent = text || '';
    box.className = 'form-message' + (text ? (isError ? ' error' : ' success') : '');
  }

  async function apiRequest(url, options = {}) {
    const opts = {
      credentials: 'same-origin',
      headers: {
        'Content-Type': 'application/json',
        ...(options.headers || {}),
      },
      ...options,
    };
    const resp = await fetch(url, opts);
    if (resp.status === 401) {
      window.location.href = '/login';
      throw new Error('未登录或会话已过期');
    }
    const text = await resp.text();
    let data = {};
    try {
      data = text ? JSON.parse(text) : {};
    } catch {
      data = { raw: text };
    }
    if (!resp.ok) {
      throw new Error(data.error || data.message || text || `HTTP ${resp.status}`);
    }
    return data;
  }

  function initDNSConsole() {
    if (dnsConsoleState.initialized) return;
    dnsConsoleState.initialized = true;

    document.querySelectorAll('#dns-workspace-tabs .oci-ws-tab').forEach((btn) => {
      btn.addEventListener('click', () => {
        switchDNSTab(btn.dataset.dnsTab || 'console');
      });
    });

    el('dns-console-reload-btn')?.addEventListener('click', async () => {
      try {
        await apiRequest('/api/config/reload', { method: 'POST', body: '{}' });
        await loadDNSPageData();
        if (typeof appendLog === 'function') appendLog('DNS 配置已重载');
      } catch (err) {
        showMessage('dns-console-message', err.message, true);
      }
    });

    // Console selectors & pills (lazy: only fetch records after account AND domain are chosen)
    el('dns-console-account-pills')?.addEventListener('click', (e) => {
      const pill = e.target.closest('[data-dns-acc]');
      if (!pill) return;
      selectConsoleAccount(pill.getAttribute('data-dns-acc') || '');
    });

    el('dns-console-account')?.addEventListener('change', (e) => {
      selectConsoleAccount(e.target.value);
    });

    el('dns-console-domain-pills')?.addEventListener('click', (e) => {
      const pill = e.target.closest('[data-dns-dom]');
      if (!pill) return;
      selectConsoleDomain(pill.getAttribute('data-dns-dom') || '');
    });

    el('dns-console-domain')?.addEventListener('change', (e) => {
      selectConsoleDomain(e.target.value);
    });

    el('dns-console-refresh')?.addEventListener('click', () => loadConsoleRecords());
    el('dns-console-search')?.addEventListener('input', (e) => {
      dnsConsoleState.searchQuery = e.target.value.trim().toLowerCase();
      renderRecordsTable();
    });

    document.querySelectorAll('#dns-type-filter-pills .dns-filter-pill').forEach((btn) => {
      btn.addEventListener('click', () => {
        dnsConsoleState.typeFilter = btn.dataset.type || 'ALL';
        document.querySelectorAll('#dns-type-filter-pills .dns-filter-pill').forEach((b) => {
          b.classList.toggle('active', b === btn);
        });
        renderRecordsTable();
      });
    });

    el('dns-record-add-btn')?.addEventListener('click', () => openRecordEditor(null));
    el('dns-record-cancel-btn')?.addEventListener('click', closeRecordEditor);
    el('dns-record-save-btn')?.addEventListener('click', saveConsoleRecord);
    el('dns-rec-type')?.addEventListener('change', syncRecordEditorFields);

    el('dns-export-btn')?.addEventListener('click', exportCurrentZone);
    el('dns-import-open-btn')?.addEventListener('click', openImportModal);
    el('dns-import-close')?.addEventListener('click', closeImportModal);
    el('dns-import-cancel')?.addEventListener('click', closeImportModal);
    el('dns-import-file')?.addEventListener('change', handleImportFileSelect);
    el('dns-import-preview-btn')?.addEventListener('click', previewZoneImport);
    el('dns-import-apply-btn')?.addEventListener('click', applyZoneImport);

    // Account configuration visual selectors & form
    el('dns-provider-cards')?.addEventListener('click', (e) => {
      const card = e.target.closest('[data-prov]');
      if (!card) return;
      const prov = card.getAttribute('data-prov');
      if (el('dns-acc-provider')) el('dns-acc-provider').value = prov;
      syncAccountProviderForm();
    });

    el('dns-edition-pills')?.addEventListener('click', (e) => {
      const seg = e.target.closest('[data-edition]');
      if (!seg) return;
      const ed = seg.getAttribute('data-edition');
      if (el('dns-acc-edition')) el('dns-acc-edition').value = ed;
      syncAccountProviderForm();
    });

    el('dns-acc-provider')?.addEventListener('change', syncAccountProviderForm);
    el('dns-acc-reset-btn')?.addEventListener('click', resetAccountForm);
    el('dns-acc-save-btn')?.addEventListener('click', saveAccountForm);
    el('dns-acc-delete-current-btn')?.addEventListener('click', () => {
      if (dnsConsoleState.editingAccountOriginalName) {
        deleteAccountByName(dnsConsoleState.editingAccountOriginalName);
      }
    });

    // Table & Rail delegated actions
    el('dns-records-table-wrap')?.addEventListener('click', handleRecordsTableClick);
    el('dns-bindings-list')?.addEventListener('click', handleBindingsListClick);
    el('dns-accounts-list')?.addEventListener('click', handleAccountsListClick);
  }

  function switchDNSTab(tab) {
    dnsConsoleState.activeTab = tab;
    document.querySelectorAll('#dns-workspace-tabs .oci-ws-tab').forEach((btn) => {
      btn.classList.toggle('active', btn.dataset.dnsTab === tab);
    });
    const consolePane = el('dns-tab-console');
    const bindingsPane = el('dns-tab-bindings');
    const accountsPane = el('dns-tab-accounts');
    if (consolePane) consolePane.hidden = tab !== 'console';
    if (bindingsPane) bindingsPane.hidden = tab !== 'bindings';
    if (accountsPane) accountsPane.hidden = tab !== 'accounts';

    if (tab === 'bindings') loadDNSBindingsTab();
    if (tab === 'accounts') renderAccountsTab();
  }

  async function loadDNSPageData() {
    initDNSConsole();
    showMessage('dns-console-message', '');
    try {
      const data = await apiRequest('/api/dns/accounts');
      dnsConsoleState.accounts = Array.isArray(data.accounts) ? data.accounts : [];
    } catch (err) {
      dnsConsoleState.accounts = [];
      showMessage('dns-console-message', '加载 DNS 账号失败: ' + err.message, true);
    }

    // Never auto-fetch remote DNS provider APIs upon entering the DNS page.
    if (dnsConsoleState.selectedAccount && !dnsConsoleState.accounts.some((a) => a.name === dnsConsoleState.selectedAccount)) {
      dnsConsoleState.selectedAccount = '';
      dnsConsoleState.selectedDomain = '';
      dnsConsoleState.domains = [];
      dnsConsoleState.records = [];
    }

    renderAccountSelectors();
    renderDomainSelectors();
    renderProviderAndStats();
    updateConsoleToolbarState();
    renderRecordsTable();
    renderAccountsTab();

    if (dnsConsoleState.activeTab === 'bindings') {
      await loadDNSBindingsTab();
    }
  }

  function renderAccountSelectors() {
    const sel = el('dns-console-account');
    const pillsEl = el('dns-console-account-pills');
    const accounts = dnsConsoleState.accounts || [];

    if (sel) {
      if (!accounts.length) {
        sel.innerHTML = '<option value="">-- 暂无 DNS 账号 --</option>';
        sel.disabled = true;
      } else {
        sel.disabled = false;
        sel.innerHTML =
          '<option value="">-- 请选择 DNS 账号 --</option>' +
          accounts
            .map(
              (a) =>
                `<option value="${escapeHTML(a.name)}">${escapeHTML(a.name)} (${escapeHTML(providerShortLabel(a.provider, a.edition))})</option>`
            )
            .join('');
        sel.value = dnsConsoleState.selectedAccount || '';
      }
    }

    if (pillsEl) {
      if (!accounts.length) {
        pillsEl.innerHTML = '<span style="font-size:12.5px;color:var(--muted);">请先在「DNS 账号配置」添加账号</span>';
      } else {
        pillsEl.innerHTML = accounts
          .map(
            (a) => `<button type="button" class="fw-pill ${a.name === dnsConsoleState.selectedAccount ? 'active' : ''}" data-dns-acc="${escapeHTML(a.name)}">
              <span>${escapeHTML(a.name)}</span>
              <span class="dns-provider-badge ${escapeHTML(a.provider || 'cloudflare')}" style="padding:1px 6px;font-size:10.5px;">${escapeHTML(providerShortLabel(a.provider, a.edition))}</span>
            </button>`
          )
          .join('');
      }
    }
  }

  function renderDomainSelectors() {
    const sel = el('dns-console-domain');
    const pillsEl = el('dns-console-domain-pills');
    const domains = dnsConsoleState.domains || [];
    const hasAcc = Boolean(dnsConsoleState.selectedAccount);

    if (sel) {
      if (!hasAcc) {
        sel.innerHTML = '<option value="">-- 请先选择账号与域名 --</option>';
        sel.disabled = true;
      } else if (!domains.length) {
        sel.innerHTML = '<option value="">-- 未配置管理域名 --</option>';
        sel.disabled = true;
      } else {
        sel.disabled = false;
        sel.innerHTML =
          '<option value="">-- 请选择管理域名 --</option>' +
          domains.map((d) => `<option value="${escapeHTML(d.domain)}">${escapeHTML(d.domain)}</option>`).join('');
        sel.value = dnsConsoleState.selectedDomain || '';
      }
    }

    if (pillsEl) {
      if (!hasAcc || !domains.length) {
        pillsEl.innerHTML = '';
      } else {
        pillsEl.innerHTML = domains
          .map(
            (d) =>
              `<button type="button" class="dns-filter-pill ${d.domain === dnsConsoleState.selectedDomain ? 'active' : ''}" data-dns-dom="${escapeHTML(d.domain)}">${escapeHTML(d.domain)}</button>`
          )
          .join('');
      }
    }
  }

  function updateConsoleToolbarState() {
    const ready = Boolean(dnsConsoleState.selectedAccount && dnsConsoleState.selectedDomain);
    ['dns-console-refresh', 'dns-record-add-btn', 'dns-import-open-btn', 'dns-export-btn'].forEach((id) => {
      const btn = el(id);
      if (btn) btn.disabled = !ready;
    });
    const filterbar = el('dns-console-filterbar');
    if (filterbar) filterbar.hidden = !ready;
  }

  async function selectConsoleAccount(accName) {
    dnsConsoleState.selectedAccount = accName || '';
    dnsConsoleState.selectedDomain = '';
    dnsConsoleState.records = [];
    closeRecordEditor();
    showMessage('dns-console-message', '');

    const accObj = dnsConsoleState.accounts.find((a) => a.name === accName);
    if (accObj?.capabilities) {
      dnsConsoleState.capabilities = accObj.capabilities;
    }

    renderAccountSelectors();

    if (!accObj) {
      dnsConsoleState.domains = [];
      renderDomainSelectors();
      renderProviderAndStats();
      updateConsoleToolbarState();
      renderRecordsTable();
      return;
    }

    // Use local configured domains without hitting external API whenever present.
    if (Array.isArray(accObj.domains) && accObj.domains.length > 0) {
      dnsConsoleState.domains = accObj.domains.map((d) => ({ domain: d }));
      renderDomainSelectors();
      renderProviderAndStats();
      updateConsoleToolbarState();
      renderRecordsTable();
      return;
    }

    // Cloudflare with empty domains list: resolve single Zone ID domain name (does not list records yet).
    try {
      const data = await apiRequest(`/api/dns/console/domains?account=${encodeURIComponent(accName)}`);
      dnsConsoleState.domains = Array.isArray(data.domains) ? data.domains : [];
      if (data.capabilities) dnsConsoleState.capabilities = data.capabilities;
    } catch (err) {
      dnsConsoleState.domains = [];
      showMessage('dns-console-message', err.message, true);
    }
    renderDomainSelectors();
    renderProviderAndStats();
    updateConsoleToolbarState();
    renderRecordsTable();
  }

  async function selectConsoleDomain(domain) {
    dnsConsoleState.selectedDomain = domain || '';
    dnsConsoleState.records = [];
    closeRecordEditor();
    renderDomainSelectors();
    updateConsoleToolbarState();

    if (!dnsConsoleState.selectedAccount || !dnsConsoleState.selectedDomain) {
      renderProviderAndStats();
      renderRecordsTable();
      return;
    }

    await loadConsoleRecords();
  }

  async function loadConsoleRecords() {
    const acc = dnsConsoleState.selectedAccount;
    const domain = dnsConsoleState.selectedDomain;
    const wrap = el('dns-records-table-wrap');
    if (!acc || !domain) {
      dnsConsoleState.records = [];
      renderProviderAndStats();
      updateConsoleToolbarState();
      renderRecordsTable();
      return;
    }
    if (wrap) {
      wrap.innerHTML = `<div class="empty-state compact">正在加载 ${escapeHTML(domain)} 的解析记录...</div>`;
    }
    showMessage('dns-console-message', '');
    try {
      const data = await apiRequest(
        `/api/dns/console/records?account=${encodeURIComponent(acc)}&domain=${encodeURIComponent(domain)}`
      );
      dnsConsoleState.records = Array.isArray(data.records) ? data.records : [];
      if (data.capabilities) dnsConsoleState.capabilities = data.capabilities;
      renderProviderAndStats();
      renderRecordsTable();
    } catch (err) {
      dnsConsoleState.records = [];
      renderProviderAndStats();
      if (wrap) {
        wrap.innerHTML = `<div class="empty-state compact">加载失败: ${escapeHTML(err.message)}</div>`;
      }
      showMessage('dns-console-message', err.message, true);
    }
  }

  function renderProviderAndStats() {
    const badge = el('dns-console-provider-badge');
    const stats = el('dns-console-stats');
    const accObj = dnsConsoleState.accounts.find((a) => a.name === dnsConsoleState.selectedAccount);
    const caps = dnsConsoleState.capabilities || {};
    if (badge) {
      if (accObj) {
        const prov = accObj.provider || 'cloudflare';
        badge.className = `dns-provider-badge ${escapeHTML(prov)}`;
        badge.textContent = providerDisplayName(prov, accObj.edition);
        badge.hidden = false;
      } else {
        badge.hidden = true;
      }
    }
    if (stats) {
      const recs = dnsConsoleState.records || [];
      if (!dnsConsoleState.selectedAccount || !dnsConsoleState.selectedDomain) {
        stats.innerHTML = '';
        return;
      }
      const total = recs.length;
      const disabled = recs.filter((r) => String(r.status || '').toUpperCase() === 'DISABLE').length;
      const proxied = recs.filter((r) => r.proxied).length;
      let html = `<span class="dns-stat-chip">当前域名 <strong>${escapeHTML(dnsConsoleState.selectedDomain)}</strong></span>`;
      html += `<span class="dns-stat-chip">总记录 <strong>${total}</strong></span>`;
      if (caps.supportsStatus) {
        html += `<span class="dns-stat-chip">启用 <strong>${total - disabled}</strong></span>`;
        if (disabled > 0) {
          html += `<span class="dns-stat-chip">暂停 <strong>${disabled}</strong></span>`;
        }
      }
      if (caps.supportsProxied) {
        html += `<span class="dns-stat-chip">Cloudflare 代理 <strong>${proxied}</strong></span>`;
      }
      stats.innerHTML = html;
    }
  }

  function renderRecordsTable() {
    const wrap = el('dns-records-table-wrap');
    if (!wrap) return;

    if (!dnsConsoleState.accounts.length) {
      wrap.innerHTML =
        '<div class="empty-state compact"><i class="bi bi-cloud-slash" style="font-size:28px;display:block;margin-bottom:8px;opacity:0.6;"></i><strong>尚未配置任何 DNS 账号</strong><p style="color:var(--muted);font-size:13px;margin-top:4px;">请切换到「DNS 账号配置」添加 Cloudflare、阿里云、腾讯云或 DNSPod 账号。</p></div>';
      return;
    }
    if (!dnsConsoleState.selectedAccount) {
      wrap.innerHTML =
        '<div class="empty-state compact"><i class="bi bi-globe2" style="font-size:32px;display:block;margin-bottom:8px;opacity:0.6;"></i><strong>未加载任何解析记录</strong><p style="color:var(--muted);font-size:13px;margin-top:4px;">请在上方选择 DNS 账号与管理域名后加载解析记录。</p></div>';
      return;
    }
    if (!dnsConsoleState.domains.length) {
      wrap.innerHTML =
        '<div class="empty-state compact">当前 DNS 账号未配置可管理的域名。请在「DNS 账号配置」中填写需要管理的根域名。</div>';
      return;
    }
    if (!dnsConsoleState.selectedDomain) {
      wrap.innerHTML = `<div class="empty-state compact"><i class="bi bi-signpost-split" style="font-size:30px;display:block;margin-bottom:8px;opacity:0.6;"></i><strong>已选定账号「${escapeHTML(
        dnsConsoleState.selectedAccount
      )}」</strong><p style="color:var(--muted);font-size:13px;margin-top:4px;">请在上方点击或选择一个管理域名以加载解析记录。</p></div>`;
      return;
    }

    const caps = dnsConsoleState.capabilities || {};
    const q = dnsConsoleState.searchQuery;
    const tf = dnsConsoleState.typeFilter;
    const filtered = (dnsConsoleState.records || []).filter((r) => {
      if (tf !== 'ALL' && String(r.type || '').toUpperCase() !== tf) return false;
      if (q) {
        const hay = `${r.name} ${r.fqdn} ${r.value} ${r.remark || ''} ${r.type}`.toLowerCase();
        if (!hay.includes(q)) return false;
      }
      return true;
    });

    if (!filtered.length) {
      wrap.innerHTML = '<div class="empty-state compact">没有匹配的解析记录。</div>';
      return;
    }

    let extraCols = '';
    if (caps.supportsLine) extraCols += '<th>线路</th>';
    if (caps.supportsProxied) extraCols += '<th>代理状态</th>';
    if (caps.supportsStatus) extraCols += '<th>状态</th>';

    const rowsHtml = filtered
      .map((r) => {
        const recType = String(r.type || 'A').toUpperCase();
        const isDisabled = String(r.status || 'ENABLE').toUpperCase() === 'DISABLE';
        const ttlLabel = Number(r.ttl) === 1 ? 'Auto' : String(r.ttl || 600);
        const prioBadge =
          r.priority !== undefined && r.priority !== null
            ? `<span class="dns-stat-chip" style="padding:1px 6px;font-size:11px;margin-right:6px;">MX ${escapeHTML(r.priority)}</span>`
            : '';

        let midCells = '';
        if (caps.supportsLine) {
          midCells += `<td>${escapeHTML(r.line || caps.defaultLine || '默认')}</td>`;
        }
        if (caps.supportsProxied) {
          const canProxy = ['A', 'AAAA', 'CNAME'].includes(recType);
          midCells += `<td>${
            canProxy
              ? `<span class="dns-proxy-pill ${r.proxied ? 'on' : 'off'}">${r.proxied ? '已代理 (CDN)' : '仅 DNS'}</span>`
              : '<span class="muted" style="font-size:12px;">-</span>'
          }</td>`;
        }
        if (caps.supportsStatus) {
          midCells += `<td><span class="dns-status-pill ${isDisabled ? 'off' : 'on'}">${isDisabled ? '暂停' : '启用'}</span></td>`;
        }

        const statusBtn = caps.supportsStatus
          ? `<button class="action-btn secondary" type="button" data-rec-action="toggle-status" data-id="${escapeHTML(r.id)}" data-enabled="${isDisabled ? 'true' : 'false'}">${isDisabled ? '启用' : '暂停'}</button>`
          : '';

        return `<tr class="${isDisabled ? 'is-disabled' : ''}">
          <td><span class="dns-type-badge type-${escapeHTML(recType)}">${escapeHTML(recType)}</span></td>
          <td class="dns-mono-cell" title="${escapeHTML(r.fqdn || '')}">${escapeHTML(r.name || '@')}</td>
          <td>
            <div class="dns-value-cell">
              ${prioBadge}
              <span class="dns-value-text" title="${escapeHTML(r.value)}">${escapeHTML(r.value)}</span>
              <button class="dns-copy-btn" type="button" data-copy="${escapeHTML(r.value)}">复制</button>
            </div>
            ${r.remark ? `<div style="font-size:11.5px;color:var(--muted);margin-top:3px;">备注: ${escapeHTML(r.remark)}</div>` : ''}
          </td>
          <td class="dns-mono-cell">${escapeHTML(ttlLabel)}</td>
          ${midCells}
          <td>
            <div class="dns-row-actions">
              ${statusBtn}
              <button class="action-btn secondary" type="button" data-rec-action="edit" data-id="${escapeHTML(r.id)}">编辑</button>
              <button class="action-btn danger" type="button" data-rec-action="delete" data-id="${escapeHTML(r.id)}">删除</button>
            </div>
          </td>
        </tr>`;
      })
      .join('');

    wrap.innerHTML = `<div class="dns-table-wrap">
      <table class="dns-records-table">
        <thead>
          <tr>
            <th>类型</th>
            <th>主机记录</th>
            <th>记录值</th>
            <th>TTL</th>
            ${extraCols}
            <th>操作</th>
          </tr>
        </thead>
        <tbody>${rowsHtml}</tbody>
      </table>
    </div>`;
  }

  function openRecordEditor(rec) {
    const editor = el('dns-record-editor');
    if (!editor) return;
    if (!dnsConsoleState.selectedAccount || !dnsConsoleState.selectedDomain) {
      showMessage('dns-console-message', '请先选择 DNS 账号和管理域名', true);
      return;
    }
    const caps = dnsConsoleState.capabilities || {};
    dnsConsoleState.editingRecordId = rec ? rec.id : '';
    el('dns-record-editor-title').textContent = rec
      ? `编辑解析记录 (${rec.name || '@'} ${rec.type})`
      : `新增解析记录 (${dnsConsoleState.selectedDomain})`;

    el('dns-rec-type').value = rec ? String(rec.type || 'A').toUpperCase() : 'A';
    el('dns-rec-name').value = rec ? rec.name || '@' : '';
    el('dns-rec-value').value = rec ? rec.value || '' : '';
    el('dns-rec-ttl').value = rec && rec.ttl ? String(rec.ttl) : caps.supportsProxied ? '1' : '600';
    el('dns-rec-priority').value = rec && rec.priority !== undefined && rec.priority !== null ? String(rec.priority) : '10';
    el('dns-rec-proxied').checked = Boolean(rec && rec.proxied);
    el('dns-rec-remark').value = rec ? rec.remark || '' : '';

    const lineSel = el('dns-rec-line');
    if (lineSel) {
      const lines = caps.availableLines && caps.availableLines.length ? caps.availableLines : [caps.defaultLine || 'default'];
      const currentLine = rec && rec.line ? rec.line : caps.defaultLine || lines[0];
      const allLines = lines.includes(currentLine) ? lines : [currentLine, ...lines];
      lineSel.innerHTML = allLines.map((l) => `<option value="${escapeHTML(l)}">${escapeHTML(l)}</option>`).join('');
      lineSel.value = currentLine;
    }

    showMessage('dns-record-editor-msg', '');
    syncRecordEditorFields();
    editor.hidden = false;
    el('dns-rec-name')?.focus();
  }

  function syncRecordEditorFields() {
    const caps = dnsConsoleState.capabilities || {};
    const recType = String(el('dns-rec-type')?.value || 'A').toUpperCase();
    const prioWrap = el('dns-rec-priority-wrap');
    const lineWrap = el('dns-rec-line-wrap');
    const proxyWrap = el('dns-rec-proxied-wrap');
    const remarkWrap = el('dns-rec-remark-wrap');

    if (prioWrap) prioWrap.hidden = recType !== 'MX' && recType !== 'SRV';
    if (lineWrap) lineWrap.hidden = !caps.supportsLine;
    if (proxyWrap) proxyWrap.hidden = !caps.supportsProxied || !['A', 'AAAA', 'CNAME'].includes(recType);
    if (remarkWrap) remarkWrap.hidden = caps.supportsRemark === false;
  }

  function closeRecordEditor() {
    dnsConsoleState.editingRecordId = '';
    const editor = el('dns-record-editor');
    if (editor) editor.hidden = true;
    showMessage('dns-record-editor-msg', '');
  }

  async function saveConsoleRecord() {
    const acc = dnsConsoleState.selectedAccount;
    const domain = dnsConsoleState.selectedDomain;
    const recType = String(el('dns-rec-type')?.value || 'A').toUpperCase();
    const name = String(el('dns-rec-name')?.value || '').trim() || '@';
    const value = String(el('dns-rec-value')?.value || '').trim();
    const ttl = parseInt(el('dns-rec-ttl')?.value || '600', 10) || 600;
    const prioRaw = parseInt(el('dns-rec-priority')?.value || '10', 10);
    const caps = dnsConsoleState.capabilities || {};

    if (!value) {
      showMessage('dns-record-editor-msg', '请填写记录值', true);
      return;
    }

    const record = {
      name,
      type: recType,
      value,
      ttl,
      proxied: caps.supportsProxied ? Boolean(el('dns-rec-proxied')?.checked) : false,
      line: caps.supportsLine ? el('dns-rec-line')?.value || caps.defaultLine : '',
      remark: el('dns-rec-remark')?.value?.trim() || '',
    };
    if (recType === 'MX' || recType === 'SRV') {
      record.priority = Number.isFinite(prioRaw) ? prioRaw : 10;
    }

    const saveBtn = el('dns-record-save-btn');
    if (saveBtn) saveBtn.disabled = true;
    showMessage('dns-record-editor-msg', '保存中...');
    try {
      if (dnsConsoleState.editingRecordId) {
        await apiRequest('/api/dns/console/records', {
          method: 'PUT',
          body: JSON.stringify({
            account: acc,
            domain,
            id: dnsConsoleState.editingRecordId,
            record,
          }),
        });
      } else {
        await apiRequest('/api/dns/console/records', {
          method: 'POST',
          body: JSON.stringify({
            account: acc,
            domain,
            record,
          }),
        });
      }
      closeRecordEditor();
      await loadConsoleRecords();
      showMessage('dns-console-message', '解析记录保存成功');
    } catch (err) {
      showMessage('dns-record-editor-msg', err.message, true);
    } finally {
      if (saveBtn) saveBtn.disabled = false;
    }
  }

  async function handleRecordsTableClick(e) {
    const copyBtn = e.target.closest('[data-copy]');
    if (copyBtn) {
      const txt = copyBtn.getAttribute('data-copy') || '';
      try {
        await navigator.clipboard.writeText(txt);
        const old = copyBtn.textContent;
        copyBtn.textContent = '已复制';
        setTimeout(() => {
          copyBtn.textContent = old;
        }, 1200);
      } catch {
        // Ignore clipboard errors
      }
      return;
    }

    const actionBtn = e.target.closest('[data-rec-action]');
    if (!actionBtn) return;
    const action = actionBtn.getAttribute('data-rec-action');
    const id = actionBtn.getAttribute('data-id');
    const rec = dnsConsoleState.records.find((r) => String(r.id) === String(id));
    if (!rec) return;

    if (action === 'edit') {
      openRecordEditor(rec);
      return;
    }

    if (action === 'toggle-status') {
      const nextEnabled = actionBtn.getAttribute('data-enabled') === 'true';
      try {
        await apiRequest('/api/dns/console/records/status', {
          method: 'PATCH',
          body: JSON.stringify({
            account: dnsConsoleState.selectedAccount,
            domain: dnsConsoleState.selectedDomain,
            id: rec.id,
            enabled: nextEnabled,
          }),
        });
        await loadConsoleRecords();
      } catch (err) {
        showMessage('dns-console-message', err.message, true);
      }
      return;
    }

    if (action === 'delete') {
      if (!window.confirm(`确定删除解析记录 ${rec.name} (${rec.type}: ${rec.value}) 吗？`)) {
        return;
      }
      try {
        await apiRequest(
          `/api/dns/console/records?account=${encodeURIComponent(dnsConsoleState.selectedAccount)}&domain=${encodeURIComponent(dnsConsoleState.selectedDomain)}&id=${encodeURIComponent(rec.id)}`,
          { method: 'DELETE' }
        );
        await loadConsoleRecords();
        showMessage('dns-console-message', '记录已删除');
      } catch (err) {
        showMessage('dns-console-message', err.message, true);
      }
    }
  }

  function exportCurrentZone() {
    const acc = dnsConsoleState.selectedAccount;
    const domain = dnsConsoleState.selectedDomain;
    if (!acc || !domain) {
      showMessage('dns-console-message', '请先选择账号和域名', true);
      return;
    }
    window.location.href = `/api/dns/console/export?account=${encodeURIComponent(acc)}&domain=${encodeURIComponent(domain)}`;
  }

  function openImportModal() {
    const acc = dnsConsoleState.selectedAccount;
    const domain = dnsConsoleState.selectedDomain;
    if (!acc || !domain) {
      showMessage('dns-console-message', '请先选择要导入的 DNS 账号与目标域名', true);
      return;
    }
    dnsConsoleState.previewData = null;
    el('dns-import-modal-title').textContent = `导入解析记录到 ${domain} (.zone / .txt)`;
    el('dns-import-file').value = '';
    el('dns-import-content').value = '';
    el('dns-import-overwrite').checked = false;
    el('dns-import-preview-summary').textContent = '';
    el('dns-import-preview-wrap').innerHTML = '';
    el('dns-import-preview-wrap').hidden = true;
    el('dns-import-apply-btn').disabled = true;
    showMessage('dns-import-message', '');
    el('dns-import-modal').hidden = false;
  }

  function closeImportModal() {
    const modal = el('dns-import-modal');
    if (modal) modal.hidden = true;
  }

  function handleImportFileSelect(e) {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      if (el('dns-import-content')) {
        el('dns-import-content').value = String(reader.result || '');
      }
      previewZoneImport();
    };
    reader.readAsText(file);
  }

  async function previewZoneImport() {
    const acc = dnsConsoleState.selectedAccount;
    const domain = dnsConsoleState.selectedDomain;
    const content = el('dns-import-content')?.value || '';
    if (!content.trim()) {
      showMessage('dns-import-message', '请先选择文件或粘贴 Zone 内容', true);
      return;
    }
    showMessage('dns-import-message', '正在解析并对比现有记录...');
    el('dns-import-apply-btn').disabled = true;
    try {
      const preview = await apiRequest('/api/dns/console/import-preview', {
        method: 'POST',
        body: JSON.stringify({ account: acc, domain, content }),
      });
      dnsConsoleState.previewData = preview;
      showMessage('dns-import-message', '');

      const summaryEl = el('dns-import-preview-summary');
      const warnText = preview.warnings?.length ? ` · 系统跳过 ${preview.warnings.length} 条 (SOA/根域NS)` : '';
      summaryEl.innerHTML = `解析 <strong>${preview.totalParsed || 0}</strong> 条记录：
        <span style="color:#34d399;font-weight:600;">新增 ${preview.createCount || 0}</span> ·
        <span style="color:#fbbf24;font-weight:600;">同名同类型冲突 ${preview.conflictCount || 0}</span>${escapeHTML(warnText)}`;

      const items = Array.isArray(preview.items) ? preview.items : [];
      const wrap = el('dns-import-preview-wrap');
      if (!items.length) {
        wrap.innerHTML = '<div class="empty-state compact">未解析到可导入的用户记录。</div>';
      } else {
        const rows = items
          .map((item) => {
            const r = item.record || {};
            const actLabel = item.action === 'conflict' ? '冲突' : item.action === 'skip_system' ? '跳过' : '新增';
            return `<tr>
              <td><span class="dns-import-action-badge ${escapeHTML(item.action)}">${actLabel}</span></td>
              <td class="dns-mono-cell">${escapeHTML(r.name)}</td>
              <td><span class="dns-type-badge type-${escapeHTML(r.type)}">${escapeHTML(r.type)}</span></td>
              <td class="dns-mono-cell">${escapeHTML(r.value)}${item.existingValue ? `<div style="font-size:11px;color:var(--muted);">原值: ${escapeHTML(item.existingValue)}</div>` : ''}</td>
              <td class="dns-mono-cell">${escapeHTML(r.ttl)}</td>
            </tr>`;
          })
          .join('');
        wrap.innerHTML = `<table class="dns-records-table">
          <thead><tr><th>处理</th><th>主机记录</th><th>类型</th><th>记录值</th><th>TTL</th></tr></thead>
          <tbody>${rows}</tbody>
        </table>`;
      }
      wrap.hidden = false;
      el('dns-import-apply-btn').disabled = items.length === 0;
    } catch (err) {
      showMessage('dns-import-message', err.message, true);
    }
  }

  async function applyZoneImport() {
    const acc = dnsConsoleState.selectedAccount;
    const domain = dnsConsoleState.selectedDomain;
    const content = el('dns-import-content')?.value || '';
    const overwrite = Boolean(el('dns-import-overwrite')?.checked);
    const btn = el('dns-import-apply-btn');
    if (btn) btn.disabled = true;
    showMessage('dns-import-message', '正在批量导入记录，请稍候...');
    try {
      const res = await apiRequest('/api/dns/console/import', {
        method: 'POST',
        body: JSON.stringify({ account: acc, domain, content, overwrite }),
      });
      const summary = `导入完成：新增 ${res.created || 0}，覆盖 ${res.updated || 0}，跳过 ${res.skipped || 0}，失败 ${res.failed || 0}`;
      showMessage('dns-import-message', summary, (res.failed || 0) > 0);
      await loadConsoleRecords();
      if ((res.failed || 0) === 0) {
        setTimeout(() => closeImportModal(), 1000);
      }
    } catch (err) {
      showMessage('dns-import-message', err.message, true);
    } finally {
      if (btn) btn.disabled = false;
    }
  }

  // ================= Tab 2: VM Bindings =================
  async function loadDNSBindingsTab() {
    const listEl = el('dns-bindings-list');
    if (!listEl) return;
    listEl.innerHTML = '<div class="empty-state compact">加载中...</div>';
    try {
      const data = await apiRequest('/api/dns/bindings');
      dnsConsoleState.bindings = Array.isArray(data.bindings) ? data.bindings : [];
      if (!dnsConsoleState.bindings.length) {
        listEl.innerHTML = '<div class="empty-state compact">暂无 VM DNS 绑定。请在云实例卡片的「DNS 绑定」弹窗中添加绑定。</div>';
        return;
      }
      listEl.innerHTML = dnsConsoleState.bindings
        .map(
          (b, idx) => `<div class="binding-item">
          <div>
            <strong>${escapeHTML(b.domain)}</strong>
            <span class="dns-type-badge type-${escapeHTML(b.type || 'A')}" style="margin-left:6px;">${escapeHTML(b.type || 'A')}</span>
            <span class="dns-provider-badge ${escapeHTML(b.dns_provider || 'cloudflare')}" style="margin-left:6px;">${escapeHTML(b.dns_account || b.cloudflare)}</span>
            ${b.proxied ? '<span class="dns-proxy-pill on" style="margin-left:6px;">Proxied</span>' : ''}
            <div style="font-size:12px;color:var(--muted);margin-top:4px;">
              绑定实例：<code>${escapeHTML(b.provider)}</code> / <code>${escapeHTML(b.account)}</code> / <code>${escapeHTML(b.vm)}</code> · TTL: ${escapeHTML(b.ttl || 600)}
            </div>
          </div>
          <button class="action-btn danger" type="button" data-delete-binding-idx="${idx}">删除</button>
        </div>`
        )
        .join('');
    } catch (err) {
      listEl.innerHTML = `<div class="empty-state compact">加载失败: ${escapeHTML(err.message)}</div>`;
    }
  }

  async function handleBindingsListClick(e) {
    const btn = e.target.closest('[data-delete-binding-idx]');
    if (!btn) return;
    const idx = parseInt(btn.getAttribute('data-delete-binding-idx'), 10);
    if (!window.confirm('确定删除该 VM DNS 绑定吗？')) return;
    try {
      await apiRequest('/api/dns/delete-binding', {
        method: 'POST',
        body: JSON.stringify({ index: idx }),
      });
      await loadDNSBindingsTab();
    } catch (err) {
      showMessage('dns-bindings-message', err.message, true);
    }
  }

  // ================= Tab 3: Accounts (Modern Master-Detail) =================
  function renderAccountsTab() {
    const listEl = el('dns-accounts-list');
    const countEl = el('dns-accounts-count');
    if (!listEl) return;
    const accounts = dnsConsoleState.accounts || [];
    if (countEl) countEl.textContent = `${accounts.length} 个已配置账号`;

    if (!accounts.length) {
      listEl.innerHTML = '<div class="empty-state compact">暂无已配置账号，请在右侧表单新建。</div>';
      syncAccountProviderForm();
      return;
    }

    const editingName = dnsConsoleState.editingAccountOriginalName;
    listEl.innerHTML = accounts
      .map((acc) => {
        const prov = acc.provider || 'cloudflare';
        const isActive = editingName === acc.name;
        const domChips =
          acc.domains && acc.domains.length
            ? acc.domains.map((d) => `<span class="dns-domain-chip">${escapeHTML(d)}</span>`).join('')
            : `<span style="font-size:11.5px;color:var(--muted);">${
                prov === 'cloudflare' ? '通过 Zone ID 识别' : '未配置域名'
              }</span>`;
        const credHint =
          prov === 'cloudflare'
            ? `Zone: ${acc.zone_id || '-'}`
            : prov === 'alidns'
            ? `Key: ${acc.access_key_id || '-'}`
            : prov === 'tencent'
            ? `SecretId: ${acc.secret_id || '-'}`
            : `TokenID: ${acc.token_id || '-'}`;

        return `<div class="dns-account-item ${isActive ? 'active' : ''}" data-acc-select="${escapeHTML(acc.name)}">
          <div class="dns-account-item-head">
            <span class="dns-account-item-name">${escapeHTML(acc.name)}</span>
            <span class="dns-provider-badge ${escapeHTML(prov)}">${escapeHTML(providerShortLabel(prov, acc.edition))}</span>
          </div>
          <div class="dns-account-item-meta">${escapeHTML(credHint)} · ${escapeHTML(acc.secret_masked || '已配置')}</div>
          ${acc.remark ? `<div class="dns-account-item-meta">备注: ${escapeHTML(acc.remark)}</div>` : ''}
          <div class="dns-domain-chips">${domChips}</div>
        </div>`;
      })
      .join('');
    syncAccountProviderForm();
  }

  function syncAccountProviderForm() {
    const prov = el('dns-acc-provider')?.value || 'cloudflare';
    const ed = el('dns-acc-edition')?.value || 'cn';

    document.querySelectorAll('#dns-provider-cards .dns-provider-card').forEach((card) => {
      card.classList.toggle('active', card.getAttribute('data-prov') === prov);
    });

    const edGroup = el('dns-acc-edition-group');
    if (edGroup) edGroup.hidden = prov !== 'tencent' && prov !== 'dnspod';

    document.querySelectorAll('#dns-edition-pills .dns-seg-btn').forEach((btn) => {
      btn.classList.toggle('active', btn.getAttribute('data-edition') === ed);
    });

    const modeBadge = el('dns-acc-mode-badge');
    if (modeBadge) {
      modeBadge.className = `dns-provider-badge ${escapeHTML(prov)}`;
      modeBadge.textContent = providerDisplayName(prov, ed);
    }

    ['cloudflare', 'alidns', 'tencent', 'dnspod'].forEach((p) => {
      const box = el(`dns-cred-${p}`);
      if (box) box.hidden = p !== prov;
    });

    const domainsHint = el('dns-acc-domains-hint');
    if (domainsHint) {
      domainsHint.textContent =
        prov === 'cloudflare'
          ? '可选：留空则通过 Zone ID 解析单个域名（不枚举账号下其他域名）'
          : '必填：仅管理此处显式填写的根域名（多个域名用逗号或换行分隔，不会枚举账号全部域名）';
    }

    const delBtn = el('dns-acc-delete-current-btn');
    if (delBtn) delBtn.hidden = !dnsConsoleState.editingAccountOriginalName;
  }

  function resetAccountForm() {
    dnsConsoleState.editingAccountOriginalName = '';
    if (el('dns-acc-name')) el('dns-acc-name').value = '';
    if (el('dns-acc-remark')) el('dns-acc-remark').value = '';
    if (el('dns-acc-provider')) el('dns-acc-provider').value = 'cloudflare';
    if (el('dns-acc-edition')) el('dns-acc-edition').value = 'cn';
    if (el('dns-acc-domains')) el('dns-acc-domains').value = '';
    [
      'dns-acc-cf-token',
      'dns-acc-cf-zone',
      'dns-acc-ali-ak',
      'dns-acc-ali-sk',
      'dns-acc-tx-sid',
      'dns-acc-tx-skey',
      'dns-acc-dp-tid',
      'dns-acc-dp-token',
    ].forEach((id) => {
      if (el(id)) el(id).value = '';
    });
    if (el('dns-acc-form-title')) el('dns-acc-form-title').textContent = '新增 DNS 账号';
    showMessage('dns-acc-message', '');
    renderAccountsTab();
  }

  function fillAccountForm(acc) {
    dnsConsoleState.editingAccountOriginalName = acc.name;
    if (el('dns-acc-form-title')) el('dns-acc-form-title').textContent = `编辑账号: ${acc.name}`;
    el('dns-acc-name').value = acc.name || '';
    el('dns-acc-remark').value = acc.remark || '';
    el('dns-acc-provider').value = acc.provider || 'cloudflare';
    el('dns-acc-edition').value = acc.edition || 'cn';
    el('dns-acc-domains').value = (acc.domains || []).join(', ');
    el('dns-acc-cf-zone').value = acc.zone_id || '';
    el('dns-acc-cf-token').value = '';
    el('dns-acc-ali-ak').value = acc.access_key_id || '';
    el('dns-acc-ali-sk').value = '';
    el('dns-acc-tx-sid').value = acc.secret_id || '';
    el('dns-acc-tx-skey').value = '';
    el('dns-acc-dp-tid').value = acc.token_id || '';
    el('dns-acc-dp-token').value = '';
    showMessage('dns-acc-message', '密钥已脱敏隐藏，若不修改密钥请留空。');
    renderAccountsTab();
  }

  async function saveAccountForm() {
    const prov = el('dns-acc-provider')?.value || 'cloudflare';
    const payload = {
      name: el('dns-acc-name')?.value?.trim() || '',
      provider: prov,
      edition: el('dns-acc-edition')?.value || 'cn',
      remark: el('dns-acc-remark')?.value?.trim() || '',
      domains_text: el('dns-acc-domains')?.value || '',
      api_token: el('dns-acc-cf-token')?.value?.trim() || '',
      zone_id: el('dns-acc-cf-zone')?.value?.trim() || '',
      access_key_id: el('dns-acc-ali-ak')?.value?.trim() || '',
      access_key_secret: el('dns-acc-ali-sk')?.value?.trim() || '',
      secret_id: el('dns-acc-tx-sid')?.value?.trim() || '',
      secret_key: el('dns-acc-tx-skey')?.value?.trim() || '',
      token_id: el('dns-acc-dp-tid')?.value?.trim() || '',
      token: el('dns-acc-dp-token')?.value?.trim() || '',
    };

    const btn = el('dns-acc-save-btn');
    if (btn) btn.disabled = true;
    showMessage('dns-acc-message', '保存中...');
    try {
      await apiRequest('/api/dns/accounts', {
        method: 'POST',
        body: JSON.stringify({
          mode: 'upsert',
          original_name: dnsConsoleState.editingAccountOriginalName || payload.name,
          account: payload,
        }),
      });
      resetAccountForm();
      await loadDNSPageData();
      showMessage('dns-acc-message', 'DNS 账号保存成功');
    } catch (err) {
      showMessage('dns-acc-message', err.message, true);
    } finally {
      if (btn) btn.disabled = false;
    }
  }

  async function deleteAccountByName(name) {
    if (!window.confirm(`确定删除 DNS 账号「${name}」吗？`)) return;
    try {
      await apiRequest('/api/dns/accounts', {
        method: 'POST',
        body: JSON.stringify({ mode: 'delete', name }),
      });
      if (dnsConsoleState.selectedAccount === name) {
        dnsConsoleState.selectedAccount = '';
        dnsConsoleState.selectedDomain = '';
        dnsConsoleState.domains = [];
        dnsConsoleState.records = [];
      }
      resetAccountForm();
      await loadDNSPageData();
    } catch (err) {
      showMessage('dns-acc-message', err.message, true);
    }
  }

  async function handleAccountsListClick(e) {
    const card = e.target.closest('[data-acc-select]');
    if (!card) return;
    const name = card.getAttribute('data-acc-select');
    const acc = dnsConsoleState.accounts.find((a) => a.name === name);
    if (acc) {
      fillAccountForm(acc);
    }
  }

  // ================= Multi-Provider VM DNS Binding Modal =================
  let vmModalCtx = {
    provider: '',
    account: '',
    vmName: '',
    dnsAccounts: [],
    bindings: [],
  };

  function bindVMModalEventsOnce() {
    const modal = el('dns-binding-modal');
    if (!modal || modal.dataset.dnsV2Bound === '1') return;
    modal.dataset.dnsV2Bound = '1';

    el('dns-v2-modal-close')?.addEventListener('click', () => {
      modal.hidden = true;
    });
    modal.addEventListener('click', (e) => {
      if (e.target === modal) modal.hidden = true;
    });
    el('dns-v2-modal-add')?.addEventListener('click', () => {
      const defaultAcc = vmModalCtx.dnsAccounts[0] || { name: '', provider: 'cloudflare', supportsProxied: true };
      vmModalCtx.bindings.push({
        dns_account: defaultAcc.name || '',
        domain: '',
        type: 'A',
        ttl: defaultAcc.supportsProxied ? 1 : 600,
        proxied: false,
      });
      renderVMModalRows();
    });
    el('dns-v2-modal-save')?.addEventListener('click', saveVMModalBindings);
    el('dns-modal-body')?.addEventListener('change', handleVMModalRowChange);
    el('dns-modal-body')?.addEventListener('input', handleVMModalRowChange);
    el('dns-modal-body')?.addEventListener('click', (e) => {
      const del = e.target.closest('[data-vm-dns-del]');
      if (!del) return;
      const idx = parseInt(del.getAttribute('data-vm-dns-del'), 10);
      vmModalCtx.bindings.splice(idx, 1);
      renderVMModalRows();
    });
  }

  function findVMDNSAccount(name) {
    return vmModalCtx.dnsAccounts.find((a) => a.name === name) || null;
  }

  function renderVMModalRows() {
    const body = el('dns-modal-body');
    if (!body) return;
    if (!vmModalCtx.dnsAccounts.length) {
      body.innerHTML =
        '<div class="empty-state compact">暂无可用的 DNS 账号，请先前往「DNS 管理 -> DNS 账号配置」添加。</div>';
      return;
    }
    if (!vmModalCtx.bindings.length) {
      body.innerHTML =
        '<div class="empty-state compact">该实例尚未绑定任何 DNS 记录，点击下方「+ 添加绑定」即可。</div>';
      return;
    }

    body.innerHTML = vmModalCtx.bindings
      .map((b, i) => {
        const selectedName = b.dns_account || b.cloudflare || vmModalCtx.dnsAccounts[0]?.name || '';
        b.dns_account = selectedName;
        const accMeta = findVMDNSAccount(selectedName) || { supportsProxied: false, provider: 'cloudflare', domains: [] };
        const domainHint = accMeta.domains?.length ? `例如 host.${accMeta.domains[0]}` : '例如 app.example.com';
        const accOptions = vmModalCtx.dnsAccounts
          .map(
            (a) =>
              `<option value="${escapeHTML(a.name)}" ${a.name === selectedName ? 'selected' : ''}>${escapeHTML(a.name)} (${escapeHTML(providerShortLabel(a.provider, a.edition))})</option>`
          )
          .join('');

        return `<div class="dns-binding-row" data-row-idx="${i}" style="padding:12px;border:1px solid var(--panel-border);border-radius:var(--radius-sm);margin-bottom:10px;background:var(--panel-soft);">
          <div style="display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-bottom:8px;">
            <div class="field" style="margin-bottom:0;">
              <span>DNS 账号</span>
              <select data-vm-field="dns_account" data-idx="${i}">${accOptions}</select>
            </div>
            <div class="field" style="margin-bottom:0;">
              <span>记录类型</span>
              <select data-vm-field="type" data-idx="${i}">
                <option value="A" ${String(b.type || 'A').toUpperCase() !== 'AAAA' ? 'selected' : ''}>A (IPv4)</option>
                <option value="AAAA" ${String(b.type || '').toUpperCase() === 'AAAA' ? 'selected' : ''}>AAAA (IPv6)</option>
              </select>
            </div>
          </div>
          <div style="display:grid;grid-template-columns:2fr 1fr auto;gap:10px;align-items:end;">
            <div class="field" style="margin-bottom:0;">
              <span>完整域名</span>
              <input type="text" data-vm-field="domain" data-idx="${i}" value="${escapeHTML(b.domain || '')}" placeholder="${escapeHTML(domainHint)}">
            </div>
            <div class="field" style="margin-bottom:0;">
              <span>TTL</span>
              <input type="number" min="1" data-vm-field="ttl" data-idx="${i}" value="${escapeHTML(b.ttl || (accMeta.supportsProxied ? 1 : 600))}">
            </div>
            <button class="action-btn danger" type="button" data-vm-dns-del="${i}" style="height:40px;">删除</button>
          </div>
          ${
            accMeta.supportsProxied
              ? `<div style="margin-top:8px;">
                  <label class="checkbox-label" style="display:inline-flex;align-items:center;gap:6px;font-size:12.5px;cursor:pointer;">
                    <input type="checkbox" data-vm-field="proxied" data-idx="${i}" ${b.proxied ? 'checked' : ''}>
                    <span>开启 Cloudflare 代理 (Proxied CDN)</span>
                  </label>
                </div>`
              : `<div style="margin-top:6px;font-size:11.5px;color:var(--muted);">当前服务商 (${escapeHTML(providerShortLabel(accMeta.provider, accMeta.edition))}) 无 CDN 代理开关</div>`
          }
        </div>`;
      })
      .join('');
  }

  function handleVMModalRowChange(e) {
    const target = e.target;
    const idx = parseInt(target.getAttribute('data-idx'), 10);
    const field = target.getAttribute('data-vm-field');
    if (!Number.isFinite(idx) || !field || !vmModalCtx.bindings[idx]) return;

    if (field === 'proxied') {
      vmModalCtx.bindings[idx].proxied = Boolean(target.checked);
      return;
    }
    if (field === 'ttl') {
      vmModalCtx.bindings[idx].ttl = parseInt(target.value || '600', 10) || 600;
      return;
    }
    vmModalCtx.bindings[idx][field] = target.value;
    if (field === 'dns_account') {
      const accMeta = findVMDNSAccount(target.value);
      if (accMeta && !accMeta.supportsProxied) {
        vmModalCtx.bindings[idx].proxied = false;
      }
      renderVMModalRows();
    }
  }

  async function openVMDNSModal(provider, account, vmName) {
    bindVMModalEventsOnce();
    vmModalCtx = {
      provider,
      account,
      vmName,
      dnsAccounts: [],
      bindings: [],
    };
    const modal = el('dns-binding-modal');
    if (el('dns-modal-title')) {
      el('dns-modal-title').textContent = `DNS 绑定配置 · ${vmName}`;
    }
    if (el('dns-modal-body')) {
      el('dns-modal-body').innerHTML = '<div class="empty-state compact">加载中...</div>';
    }
    showMessage('dns-modal-message', '');
    if (modal) modal.hidden = false;

    try {
      const data = await apiRequest(
        `/api/vm/${encodeURIComponent(provider)}/${encodeURIComponent(account)}/${encodeURIComponent(vmName)}/dns`
      );
      vmModalCtx.dnsAccounts = Array.isArray(data.dns_accounts)
        ? data.dns_accounts
        : (data.cloudflare_accounts || []).map((n) => ({ name: n, provider: 'cloudflare', supportsProxied: true }));
      vmModalCtx.bindings = Array.isArray(data.bindings) ? data.bindings : [];
      renderVMModalRows();
    } catch (err) {
      if (el('dns-modal-body')) {
        el('dns-modal-body').innerHTML = `<div class="empty-state compact">加载失败: ${escapeHTML(err.message)}</div>`;
      }
    }
  }

  async function saveVMModalBindings() {
    const saveBtn = el('dns-v2-modal-save');
    if (saveBtn) saveBtn.disabled = true;
    showMessage('dns-modal-message', '保存中...');
    try {
      await apiRequest(
        `/api/vm/${encodeURIComponent(vmModalCtx.provider)}/${encodeURIComponent(vmModalCtx.account)}/${encodeURIComponent(vmModalCtx.vmName)}/dns`,
        {
          method: 'POST',
          body: JSON.stringify({ bindings: vmModalCtx.bindings }),
        }
      );
      showMessage('dns-modal-message', '绑定配置已保存');
      setTimeout(() => {
        const modal = el('dns-binding-modal');
        if (modal) modal.hidden = true;
      }, 600);
    } catch (err) {
      showMessage('dns-modal-message', err.message, true);
    } finally {
      if (saveBtn) saveBtn.disabled = false;
    }
  }

  window.dnsConsoleModule = {
    loadDNSPage: loadDNSPageData,
    openVMDNSModal,
    switchDNSTab,
  };
})();
