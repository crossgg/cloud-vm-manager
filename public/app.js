const API_BASE = '';

let currentVMs = [];
let selectedAccount = null;
let authEnabled = false;
let proxyPool = [];
let proxyBindings = [];
let proxyAccounts = [];

const els = {};

let firewallAccounts = [];
let selectedFirewallAccount = null; // { provider, account, group }
let currentGCPFirewalls = [];
let currentOciSecurityLists = [];
let currentOciAccountNSGs = [];
let currentOciRouteTables = [];
let ociFirewallTab = 'security-lists';
let currentOciRouteTable = null;
let ociRouteTableRulesData = [];
let currentAzureNSGs = [];
let currentAzureNSG = null;
let azureNSGDirection = 'Inbound';
let currentOciAccountSL = null;
let currentOciAccountModalType = 'security-list';
let ociAccountSLDirection = 'ingress';
let ociAccountSLData = { ingress: [], egress: [] };

document.addEventListener('DOMContentLoaded', () => {
  cacheElements();
  bindNavigation();
  bindActions();
  checkAuth();
  fetchOverview();
});

function cacheElements() {
  Object.assign(els, {
    appShell: document.getElementById('app-shell'),
    logoutBtn: document.getElementById('logout-btn'),
    authBadge: document.getElementById('auth-badge'),
    accountList: document.getElementById('account-list'),
    sidebarAccountsMenu: document.getElementById('sidebar-accounts-menu'),
    selectedAccount: document.getElementById('selected-account'),
    vmList: document.getElementById('vm-list'),
    configCount: document.getElementById('config-count'),
    configMeta: document.getElementById('config-meta'),
    vmCount: document.getElementById('vm-count'),
    instanceMeta: document.getElementById('instance-meta'),
    runningCount: document.getElementById('running-count'),
    runningMeta: document.getElementById('running-meta'),
    stoppedCount: document.getElementById('stopped-count'),
    stoppedMeta: document.getElementById('stopped-meta'),
    overviewRefreshBtn: document.getElementById('overview-refresh-btn'),

    // Multi-Cloud Firewall Management Elements
    sidebarFirewallMenu: document.getElementById('sidebar-firewall-menu'),
    firewallToggleBtn: document.getElementById('firewall-toggle-btn'),
    fwRefreshBtn: document.getElementById('fw-refresh-btn'),
    fwCreateBtn: document.getElementById('fw-create-btn'),
    fwCreateBtnLabel: document.getElementById('fw-create-btn-label'),
    fwAccountPills: document.getElementById('fw-account-pills'),
    fwAccountSelect: document.getElementById('fw-account-select'),
    fwAccountMeta: document.getElementById('fw-account-meta'),
    fwContentContainer: document.getElementById('fw-content-container'),

    // GCP Firewall Modal
    gcpFwModal: document.getElementById('gcp-firewall-modal'),
    gcpFwForm: document.getElementById('gcp-fw-form'),
    gcpFwModalTitle: document.getElementById('gcp-fw-modal-title'),
    gcpFwModalClose: document.getElementById('gcp-fw-modal-close'),
    gcpFwModalCancel: document.getElementById('gcp-fw-modal-cancel'),
    gcpFwModalMsg: document.getElementById('gcp-fw-modal-message'),
    gcpFwName: document.getElementById('gcp-fw-name'),
    gcpFwPriority: document.getElementById('gcp-fw-priority'),
    gcpFwDirection: document.getElementById('gcp-fw-direction'),
    gcpFwAction: document.getElementById('gcp-fw-action'),
    gcpProtoModeCustom: document.getElementById('gcp-proto-mode-custom'),
    gcpProtoModeAll: document.getElementById('gcp-proto-mode-all'),
    gcpProtoCustomContainer: document.getElementById('gcp-proto-custom-container'),
    gcpProtoCheckTcp: document.getElementById('gcp-proto-check-tcp'),
    gcpProtoTcpPorts: document.getElementById('gcp-proto-tcp-ports'),
    gcpProtoCheckUdp: document.getElementById('gcp-proto-check-udp'),
    gcpProtoUdpPorts: document.getElementById('gcp-proto-udp-ports'),
    gcpProtoCheckIcmp: document.getElementById('gcp-proto-check-icmp'),
    gcpProtoCheckOther: document.getElementById('gcp-proto-check-other'),
    gcpProtoOtherName: document.getElementById('gcp-proto-other-name'),
    gcpProtoOtherPorts: document.getElementById('gcp-proto-other-ports'),
    gcpFwIpRanges: document.getElementById('gcp-fw-ip-ranges'),
    gcpFwIpLabel: document.getElementById('gcp-fw-ip-label'),
    gcpFwTargetTags: document.getElementById('gcp-fw-target-tags'),
    gcpFwDescription: document.getElementById('gcp-fw-description'),
    gcpFwDisabled: document.getElementById('gcp-fw-disabled'),
    gcpFwEditMode: document.getElementById('gcp-fw-edit-mode'),

    // Azure NSG Modal
    azureNsgModal: document.getElementById('azure-nsg-modal'),
    azureNsgModalTitle: document.getElementById('azure-nsg-modal-title'),
    azureNsgModalClose: document.getElementById('azure-nsg-modal-close'),
    azureNsgTabInbound: document.getElementById('azure-nsg-tab-inbound'),
    azureNsgTabOutbound: document.getElementById('azure-nsg-tab-outbound'),
    azureNsgAddRuleBtn: document.getElementById('azure-nsg-add-rule-btn'),
    azureNsgAddForm: document.getElementById('azure-nsg-add-form'),
    azureNsgRulesContainer: document.getElementById('azure-nsg-rules-container'),
    azureNsgModalDone: document.getElementById('azure-nsg-modal-done'),
    azRuleName: document.getElementById('az-rule-name'),
    azRulePriority: document.getElementById('az-rule-priority'),
    azRuleAccess: document.getElementById('az-rule-access'),
    azRuleProtocol: document.getElementById('az-rule-protocol'),
    azRuleSourceIp: document.getElementById('az-rule-source-ip'),
    azRuleDestPort: document.getElementById('az-rule-dest-port'),
    azRuleDesc: document.getElementById('az-rule-desc'),
    azRuleCancelBtn: document.getElementById('az-rule-cancel-btn'),
    azureNsgFormMsg: document.getElementById('azure-nsg-form-msg'),

    // OCI Account Security List Modal
    ociAccSlModal: document.getElementById('oci-account-sl-modal'),
    ociAccSlModalTitle: document.getElementById('oci-acc-sl-modal-title'),
    ociAccSlModalClose: document.getElementById('oci-acc-sl-modal-close'),
    ociAccSlTabIngress: document.getElementById('oci-acc-sl-tab-ingress'),
    ociAccSlTabEgress: document.getElementById('oci-acc-sl-tab-egress'),
    ociAccSlModalBody: document.getElementById('oci-acc-sl-modal-body'),
    ociAccSlAdd: document.getElementById('oci-acc-sl-add'),
    ociAccSlSave: document.getElementById('oci-acc-sl-save'),
    ociAccSlMsg: document.getElementById('oci-acc-sl-message'),

    // OCI Account Route Table Modal
    ociRtModal: document.getElementById('oci-route-table-modal'),
    ociRtModalTitle: document.getElementById('oci-rt-modal-title'),
    ociRtModalClose: document.getElementById('oci-rt-modal-close'),
    ociRtMeta: document.getElementById('oci-rt-meta'),
    ociRtQuickV6Btn: document.getElementById('oci-rt-quick-v6-btn'),
    ociRtQuickV4Btn: document.getElementById('oci-rt-quick-v4-btn'),
    ociRtModalBody: document.getElementById('oci-rt-modal-body'),
    ociRtAdd: document.getElementById('oci-rt-add'),
    ociRtSave: document.getElementById('oci-rt-save'),
    ociRtMsg: document.getElementById('oci-rt-message'),

    logs: document.getElementById('logs'),
    authForm: document.getElementById('auth-settings-form'),
    authEnabled: document.getElementById('auth-enabled'),
    authUsername: document.getElementById('auth-username'),
    authPassword: document.getElementById('auth-password'),
    authSessionHours: document.getElementById('auth-session-hours'),
    authCookieSecure: document.getElementById('auth-cookie-secure'),
    authMessage: document.getElementById('auth-settings-message'),
    configPath: document.getElementById('config-path'),
    configStatus: document.getElementById('config-status'),
    updateCurrentVersion: document.getElementById('update-current-version'),
    updateLatestVersion: document.getElementById('update-latest-version'),
    updateAssetName: document.getElementById('update-asset-name'),
    updateProxyMode: document.getElementById('update-proxy-mode'),
    updateCustomProxy: document.getElementById('update-custom-proxy'),
    updateCustomProxyField: document.getElementById('update-custom-proxy-field'),
    updateMessage: document.getElementById('update-message'),
    updateReleaseNotesBox: document.getElementById('update-release-notes-box'),
    updateReleaseNotesTitle: document.getElementById('update-release-notes-title'),
    updateReleaseLink: document.getElementById('update-release-link'),
    updateReleaseNotesBody: document.getElementById('update-release-notes-body'),
    saveUpdateProxyBtn: document.getElementById('save-update-proxy-btn'),

    // Cloud Account Management Elements
    addAccountBtn: document.getElementById('add-account-btn'),
    settingsAccountsList: document.getElementById('settings-accounts-list'),
    accountModal: document.getElementById('account-modal'),
    accountForm: document.getElementById('account-form'),
    accountModalTitle: document.getElementById('account-modal-title'),
    accountModalClose: document.getElementById('account-modal-close'),
    accountModalCancel: document.getElementById('account-modal-cancel'),
    accountModalMsg: document.getElementById('account-modal-message'),
    accProvider: document.getElementById('acc-provider'),
    accName: document.getElementById('acc-name'),
    accGroup: document.getElementById('acc-group'),
    accountEditMode: document.getElementById('account-edit-mode'),
    accountOriginalName: document.getElementById('account-original-name'),
    keyFileHiddenInput: document.getElementById('key-file-hidden-input')
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
        loadUpdateStatus();
      }
      if (section === 'overview') {
        fetchOverview();
      }
      if (section === 'dns') {
        loadDNSPage();
      }
      if (section === 'proxies') {
        loadProxyPage();
      }
      if (section === 'firewall' || section === 'gcp-firewall') {
        const fwMenu = document.getElementById('sidebar-firewall-menu');
        const fwToggle = document.getElementById('firewall-toggle-btn');
        if (fwMenu && fwMenu.classList.contains('collapsed')) {
          fwMenu.classList.remove('collapsed');
          fwMenu.style.display = 'flex';
          const icon = fwToggle?.querySelector('i');
          if (icon) icon.className = 'bi bi-dash-lg';
        }
        if (!selectedFirewallAccount) {
          initFirewallSection();
        } else {
          fetchCurrentFirewalls();
        }
      }
    });
  });
}

function bindActions() {
  els.logoutBtn.addEventListener('click', handleLogout);
  els.overviewRefreshBtn?.addEventListener('click', () => {
    fetchOverview();
    addLog('已刷新概览统计。', 'info');
  });
  document.getElementById('refresh-current-btn').addEventListener('click', refreshVMs);
  document.getElementById('clear-log-btn').addEventListener('click', clearLogs);
  document.getElementById('reload-config-btn').addEventListener('click', reloadConfig);

  // Cloud Account Management bindings
  els.addAccountBtn?.addEventListener('click', () => openAccountModal(null));
  els.accountModalClose?.addEventListener('click', closeAccountModal);
  els.accountModalCancel?.addEventListener('click', closeAccountModal);
  els.accountForm?.addEventListener('submit', handleAccountFormSubmit);
  els.accProvider?.addEventListener('change', () => updateAccountProviderFields());
  document.getElementById('account-modal')?.addEventListener('click', (e) => {
    if (e.target.id === 'account-modal') closeAccountModal();
  });
  document.addEventListener('click', (e) => {
    const btn = e.target.closest('.upload-key-btn');
    if (btn) {
      const targetId = btn.dataset.target;
      if (els.keyFileHiddenInput) {
        els.keyFileHiddenInput.dataset.targetInput = targetId;
        els.keyFileHiddenInput.value = '';
        els.keyFileHiddenInput.click();
      }
    }
  });
  els.keyFileHiddenInput?.addEventListener('change', handleKeyFileUpload);
  els.settingsAccountsList?.addEventListener('click', handleSettingsAccountsAction);
  document.getElementById('check-update-btn')?.addEventListener('click', () => loadUpdateStatus(true));
  document.getElementById('apply-update-btn')?.addEventListener('click', applyUpdate);
  els.updateCurrentVersion?.addEventListener('click', (e) => {
    if (e.target.closest('#reset-update-btn')) {
      resetToBuiltinVersion();
    }
  });
  els.saveUpdateProxyBtn?.addEventListener('click', saveUpdateProxy);
  els.updateProxyMode?.addEventListener('change', updateProxyModeChanged);
  els.authForm.addEventListener('submit', saveAuthSettings);
  document.getElementById('proxy-create-form')?.addEventListener('submit', createProxyEntry);
  document.getElementById('proxy-import-toggle')?.addEventListener('click', toggleProxyImport);
  document.getElementById('proxy-import-submit')?.addEventListener('click', importProxyEntries);
  document.getElementById('proxy-refresh-btn')?.addEventListener('click', loadProxyPage);
  document.getElementById('proxy-list')?.addEventListener('click', handleProxyRowAction);
  document.getElementById('proxy-binding-form')?.addEventListener('submit', saveProxyBinding);
  document.getElementById('proxy-binding-account')?.addEventListener('change', applySelectedAccountBinding);
  document.getElementById('proxy-binding-primary')?.addEventListener('change', renderProxySelectors);
  document.getElementById('proxy-bindings-list')?.addEventListener('click', handleProxyBindingAction);

  // Multi-Cloud Firewall action bindings
  els.fwRefreshBtn?.addEventListener('click', () => fetchCurrentFirewalls());
  els.fwCreateBtn?.addEventListener('click', () => handleFirewallCreateClick());
  els.fwAccountSelect?.addEventListener('change', (e) => handleFirewallAccountSelect(e.target.value));
  els.fwAccountPills?.addEventListener('click', (e) => {
    const pill = e.target.closest('.fw-pill');
    if (pill) handleFirewallPillClick(pill);
  });

  // GCP Firewall action bindings
  els.gcpFwForm?.addEventListener('submit', handleGCPFirewallSubmit);
  els.gcpFwModalClose?.addEventListener('click', closeGCPFirewallModal);
  els.gcpFwModalCancel?.addEventListener('click', closeGCPFirewallModal);
  els.gcpFwDirection?.addEventListener('change', updateGCPProtocolUI);

  // GCP Protocol mode toggle (custom vs all)
  document.querySelectorAll('input[name="gcp-proto-mode"]').forEach(radio => {
    radio.addEventListener('change', updateGCPProtocolUI);
  });

  // GCP Protocol checkboxes toggle
  ['tcp', 'udp', 'icmp', 'other'].forEach(proto => {
    const chk = document.getElementById(`gcp-proto-check-${proto}`);
    chk?.addEventListener('change', () => {
      const item = document.getElementById(`gcp-proto-item-${proto}`);
      const body = document.getElementById(`gcp-proto-body-${proto}`);
      if (item) item.classList.toggle('active', chk.checked);
      if (body) body.style.display = chk.checked ? '' : 'none';
    });
  });

  // Quick chips inside GCP custom protocol container
  els.gcpProtoCustomContainer?.addEventListener('click', (e) => {
    const btn = e.target.closest('.port-chip');
    if (!btn) return;
    const chipsGroup = btn.closest('.quick-port-chips');
    const targetId = chipsGroup?.dataset?.target;
    if (!targetId) return;
    const targetInput = document.getElementById(targetId);
    if (!targetInput) return;
    const port = btn.dataset.port;
    if (!port) return;

    const currentVal = targetInput.value.trim();
    if (!currentVal) {
      targetInput.value = port;
    } else {
      const existing = currentVal.split(',').map(s => s.trim());
      if (!existing.includes(port)) {
        targetInput.value = currentVal + ', ' + port;
      }
    }
    targetInput.focus();
  });

  // Azure NSG Modal bindings
  els.azureNsgModalClose?.addEventListener('click', closeAzureNSGModal);
  els.azureNsgModalDone?.addEventListener('click', closeAzureNSGModal);
  els.azureNsgTabInbound?.addEventListener('click', () => switchAzureNSGDirection('Inbound'));
  els.azureNsgTabOutbound?.addEventListener('click', () => switchAzureNSGDirection('Outbound'));
  els.azureNsgAddRuleBtn?.addEventListener('click', toggleAzureNSGAddForm);
  els.azRuleCancelBtn?.addEventListener('click', hideAzureNSGAddForm);
  els.azureNsgAddForm?.addEventListener('submit', handleAzureNSGAddSubmit);
  document.getElementById('azure-quick-ports')?.addEventListener('click', handleAzureQuickPortClick);
  els.azureNsgRulesContainer?.addEventListener('click', handleAzureRulesContainerClick);

  // OCI Account Security List Modal bindings
  els.ociAccSlModalClose?.addEventListener('click', closeOciAccountSLModal);
  els.ociAccSlTabIngress?.addEventListener('click', () => switchOciAccountSLDirection('ingress'));
  els.ociAccSlTabEgress?.addEventListener('click', () => switchOciAccountSLDirection('egress'));
  els.ociAccSlAdd?.addEventListener('click', addOciAccountSLRuleRow);
  els.ociAccSlSave?.addEventListener('click', saveOciAccountSecurityListRules);
  els.ociAccSlModalBody?.addEventListener('click', (e) => {
    const btn = e.target.closest('.sg-rule-delete-btn');
    if (btn) {
      btn.closest('.sg-rule-row')?.remove();
      if (!els.ociAccSlModalBody.querySelector('.sg-rule-row')) {
        els.ociAccSlModalBody.innerHTML = `<div class="empty-state compact">暂无${ociAccountSLDirection === 'egress' ? '出站' : '入站'}规则</div>`;
      }
    }
  });
  els.ociAccSlModalBody?.addEventListener('change', (e) => {
    const protocolSelect = e.target.closest('.sg-f-protocol');
    if (protocolSelect) {
      applySecurityProtocolPreset(protocolSelect.closest('.sg-rule-row'));
      updateSecurityListProtocolControls();
    }
    const icmpTypeSelect = e.target.closest('.sg-f-icmp-type');
    if (icmpTypeSelect) {
      const row = icmpTypeSelect.closest('.sg-rule-row');
      const codeSelect = row?.querySelector('.sg-f-icmp-code');
      if (codeSelect) codeSelect.innerHTML = icmpCodeOptionsHtml(icmpTypeSelect.value, '');
    }
  });

  // OCI Account Route Table Modal bindings
  els.ociRtModalClose?.addEventListener('click', closeOciRouteTableModal);
  els.ociRtQuickV6Btn?.addEventListener('click', () => quickAddOciDefaultRoute('::/0'));
  els.ociRtQuickV4Btn?.addEventListener('click', () => quickAddOciDefaultRoute('0.0.0.0/0'));
  els.ociRtAdd?.addEventListener('click', addOciRouteRuleRow);
  els.ociRtSave?.addEventListener('click', saveOciRouteTableRules);
  els.ociRtModalBody?.addEventListener('click', (e) => {
    const btn = e.target.closest('.oci-rt-delete-btn');
    if (btn) {
      btn.closest('.sg-rule-row')?.remove();
      saveCurrentOciRouteTableInputState();
      renderOciRouteTableRules();
    }
  });
  els.ociRtModalBody?.addEventListener('change', (e) => {
    const entitySelect = e.target.closest('.oci-rt-entity-select');
    if (entitySelect) {
      const row = entitySelect.closest('.sg-rule-row');
      const customWrap = row?.querySelector('.oci-rt-entity-custom-wrap');
      if (customWrap) customWrap.hidden = entitySelect.value !== '__custom__';
    }
  });

  // Unified Firewall table container clicks
  els.fwContentContainer?.addEventListener('click', handleFirewallTableAction);
  document.addEventListener('click', event => {
    // 1. Toggle for Level 2 (instances menu)
    const instToggle = event.target.closest('#instances-toggle-btn');
    if (instToggle) {
      event.stopPropagation();
      event.preventDefault();
      const menu = document.getElementById('sidebar-accounts-menu');
      const icon = instToggle.querySelector('i');
      if (menu) {
        const isCollapsed = menu.classList.toggle('collapsed');
        menu.style.display = isCollapsed ? 'none' : 'flex';
        if (icon) {
          icon.className = isCollapsed ? 'bi bi-plus-lg' : 'bi bi-dash-lg';
        }
      }
      return;
    }

    // Toggle for Level 2 (firewall menu)
    const fwToggle = event.target.closest('#firewall-toggle-btn');
    if (fwToggle) {
      event.stopPropagation();
      event.preventDefault();
      const menu = document.getElementById('sidebar-firewall-menu');
      const icon = fwToggle.querySelector('i');
      if (menu) {
        const isCollapsed = menu.classList.toggle('collapsed');
        menu.style.display = isCollapsed ? 'none' : 'flex';
        if (icon) {
          icon.className = isCollapsed ? 'bi bi-plus-lg' : 'bi bi-dash-lg';
        }
      }
      return;
    }

    // 2. Toggle for Level 3 (group accounts in instances or firewall)
    const groupToggle = event.target.closest('.sub-nav-group-title .sub-nav-toggle');
    if (groupToggle) {
      event.stopPropagation();
      event.preventDefault();
      const groupTitle = groupToggle.closest('.sub-nav-group-title');
      const group = groupTitle.closest('.sub-nav-group');
      const items = group.querySelector('.sub-nav-group-items');
      const icon = groupToggle.querySelector('i');
      if (items) {
        const isCollapsed = group.classList.toggle('collapsed');
        items.style.display = isCollapsed ? 'none' : 'flex';
        if (icon) {
          icon.className = isCollapsed ? 'bi bi-plus-lg' : 'bi bi-dash-lg';
        }
      }
      return;
    }

    const actionButton = event.target.closest('.action-btn[data-action]');
    if (actionButton) {
      handleVMAction(actionButton);
      return;
    }

    // Sub-nav Firewall Account Item Click
    const fwAccountBtn = event.target.closest('.sub-nav-firewall-item');
    if (fwAccountBtn) {
      const provider = fwAccountBtn.dataset.provider;
      const accountName = fwAccountBtn.dataset.account;

      const fwNavBtn = document.getElementById('nav-firewall-btn');
      if (fwNavBtn) {
        document.querySelectorAll('.nav-item').forEach(i => i.classList.remove('active'));
        fwNavBtn.classList.add('active');
        document.querySelectorAll('.section').forEach(s => s.classList.remove('active'));
        document.getElementById('firewall-section')?.classList.add('active');
      }

      selectFirewallAccount(provider, accountName);
      return;
    }

    const accountBtn = event.target.closest('.account-pill, .sub-nav-account-item:not(.sub-nav-firewall-item)');
    if (accountBtn) {
      const provider = accountBtn.dataset.provider;
      const accountName = accountBtn.dataset.account;
      const group = accountBtn.dataset.group;

      if (accountBtn.classList.contains('sub-nav-account-item')) {
        const instancesNavBtn = document.getElementById('nav-instances-btn');
        if (instancesNavBtn) {
          // Switch tab to instances-section
          instancesNavBtn.click();
        }
      }

      loadAccount({ provider, account: accountName, group });
    }
  });
  initLogPanelCollapse();
}

function initLogPanelCollapse() {
  const logHeader = document.querySelector('.log-header');
  const logPanel = document.querySelector('.log-panel');
  if (!logHeader || !logPanel) return;

  const titleSpan = logHeader.querySelector('span');
  if (titleSpan) {
    titleSpan.style.display = 'inline-flex';
    titleSpan.style.alignItems = 'center';
    titleSpan.style.gap = '8px';

    const icon = document.createElement('i');
    icon.id = 'log-toggle-icon';
    icon.className = 'bi bi-dash-lg';
    icon.style.fontSize = '12px';
    titleSpan.appendChild(icon);
  }

  const savedCollapsed = localStorage.getItem('log-panel-collapsed') === 'true';
  if (savedCollapsed) {
    logPanel.classList.add('collapsed');
    const icon = document.getElementById('log-toggle-icon');
    if (icon) icon.className = 'bi bi-plus-lg';
  }

  logHeader.addEventListener('click', event => {
    if (event.target.closest('#clear-log-btn')) return;

    const isCollapsed = logPanel.classList.toggle('collapsed');
    localStorage.setItem('log-panel-collapsed', isCollapsed);
    const icon = document.getElementById('log-toggle-icon');
    if (icon) {
      icon.className = isCollapsed ? 'bi bi-plus-lg' : 'bi bi-dash-lg';
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
    loadUpdateStatus();
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
    const accountsArr = Array.isArray(accounts) ? accounts : [];
    proxyAccounts = accountsArr;
    renderAccounts(accountsArr);
    renderSidebarAccounts(accountsArr);
    renderSidebarFirewallAccounts(accountsArr);
    renderFirewallAccounts(accountsArr);
    renderSettingsAccounts(accountsArr);
    fetchOverview();
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
    <button class="account-pill" 
      data-provider="${escapeAttr(account.provider)}"
      data-account="${escapeAttr(account.account)}"
      data-group="${escapeAttr(account.group || account.provider)}"
      type="button">
      <span class="provider-badge">${escapeHtml(account.provider)}</span>
      <span>${escapeHtml(account.account)}</span>
    </button>
  `).join('');
}

function renderSidebarAccounts(accounts) {
  if (!els.sidebarAccountsMenu) return;
  if (accounts.length === 0) {
    els.sidebarAccountsMenu.innerHTML = '';
    return;
  }

  const groups = {};
  accounts.forEach(account => {
    const groupName = account.group || account.provider;
    if (!groups[groupName]) {
      groups[groupName] = [];
    }
    groups[groupName].push(account);
  });

  els.sidebarAccountsMenu.innerHTML = Object.entries(groups).map(([groupName, groupAccounts]) => `
    <div class="sub-nav-group collapsed">
      <div class="sub-nav-group-title">
        <span class="sub-nav-group-title-content">
          <i class="bi bi-folder2-open"></i>
          <span>${escapeHtml(groupName)}</span>
        </span>
        <span class="sub-nav-toggle"><i class="bi bi-plus-lg"></i></span>
      </div>
      <div class="sub-nav-group-items" style="display: none;">
        ${groupAccounts.map(account => `
          <a class="sub-nav-account-item"
            data-provider="${escapeAttr(account.provider)}"
            data-account="${escapeAttr(account.account)}"
            data-group="${escapeAttr(account.group || account.provider)}">
            <i class="bi bi-cloud"></i>
            <span>${escapeHtml(account.account)}</span>
          </a>
        `).join('')}
      </div>
    </div>
  `).join('');
}

function renderSidebarFirewallAccounts(accounts) {
  if (!els.sidebarFirewallMenu) return;
  const fwAccounts = (accounts || []).filter(a => ['gcp', 'oci', 'azure'].includes((a.provider || '').toLowerCase()));
  if (fwAccounts.length === 0) {
    els.sidebarFirewallMenu.innerHTML = '';
    return;
  }

  const groups = {};
  fwAccounts.forEach(account => {
    const groupName = account.group || account.provider;
    if (!groups[groupName]) {
      groups[groupName] = [];
    }
    groups[groupName].push(account);
  });

  els.sidebarFirewallMenu.innerHTML = Object.entries(groups).map(([groupName, groupAccounts]) => `
    <div class="sub-nav-group collapsed">
      <div class="sub-nav-group-title">
        <span class="sub-nav-group-title-content">
          <i class="bi bi-shield-check"></i>
          <span>${escapeHtml(groupName)}</span>
        </span>
        <span class="sub-nav-toggle"><i class="bi bi-plus-lg"></i></span>
      </div>
      <div class="sub-nav-group-items" style="display: none;">
        ${groupAccounts.map(account => {
          const prov = (account.provider || '').toLowerCase();
          const isSelected = selectedFirewallAccount &&
            selectedFirewallAccount.provider.toLowerCase() === prov &&
            selectedFirewallAccount.account === account.account;
          return `
            <a class="sub-nav-account-item sub-nav-firewall-item ${isSelected ? 'active' : ''}"
              data-provider="${escapeAttr(prov)}"
              data-account="${escapeAttr(account.account)}"
              data-group="${escapeAttr(account.group || account.provider)}">
              <i class="bi bi-shield"></i>
              <span>${escapeHtml(account.account)}</span>
            </a>
          `;
        }).join('')}
      </div>
    </div>
  `).join('');
}

async function loadAccount(account) {
  selectedAccount = account;
  els.selectedAccount.textContent = `${account.provider} / ${account.account}`;

  document.querySelectorAll('.account-pill, .sub-nav-account-item').forEach(el => {
    el.classList.toggle('active', el.dataset.provider === account.provider && el.dataset.account === account.account);
  });
  updateOCIWorkspaceVisibility();
  updateAzureWorkspaceVisibility();
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
  fetchOverview();
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
  const ipv6List = Array.isArray(vm.ipv6s) ? vm.ipv6s : [];
  const ipv6Text = vm.ipv6 || (ipv6List.length > 0 ? ipv6List[0].ipAddress : '未分配');
  const ipv6Badge = ipv6List.length > 1 ? ` (+${ipv6List.length - 1})` : '';
  const dnsEnabled = Boolean(vm.dnsEnabled);
  const isOCI = provider === 'oci';

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
        <div class="info-item"><dt>公网 IPv4</dt><dd class="mono">${escapeHtml(publicIP)}</dd></div>
        <div class="info-item"><dt>公网 IP 名称</dt><dd>${escapeHtml(publicIPName)}</dd></div>
        <div class="info-item"><dt>内网 IP</dt><dd class="mono">${escapeHtml(vm.privateIP || '未分配')}</dd></div>
        ${isOCI || ipv6Text !== '未分配' ? `<div class="info-item"><dt>IPv6</dt><dd class="mono">${escapeHtml(ipv6Text + ipv6Badge)}</dd></div>` : ''}
        <div class="info-item"><dt>资源组 / 项目 / 区间</dt><dd>${escapeHtml(vm.resourceGroup || '-')}</dd></div>
      </dl>

      ${isOCI ? '' : `
      <div class="vm-options">
        <label class="dns-toggle ${dnsEnabled ? '' : 'disabled'}">
          <input type="checkbox" class="change-ip-dns-toggle" ${dnsEnabled ? '' : 'disabled'}>
          <span>换 IP 后更新 DNS</span>
        </label>
      </div>
      `}

      <div class="vm-actions">
        <button class="action-btn start" type="button" data-action="start" ${status === 'VM running' ? 'disabled' : ''}>开机</button>
        <button class="action-btn stop" type="button" data-action="stop" ${status !== 'VM running' ? 'disabled' : ''}>关机</button>
        <button class="action-btn restart" type="button" data-action="restart">重启</button>
        ${isOCI
          ? '<button class="action-btn change-ip" type="button" data-action="ip-manage">IP 管理</button>'
          : '<button class="action-btn change-ip" type="button" data-action="change-ip">换 IP</button>'
        }
        ${dnsEnabled ? '<button class="action-btn dns" type="button" data-action="update-dns">更新 DNS</button>' : ''}
        <button class="action-btn dns-bind" type="button" data-action="dns-bind">DNS 绑定</button>
        ${isOCI ? '<button class="action-btn edit" type="button" data-action="edit">编辑</button>' : ''}
        ${isOCI ? '<button class="action-btn security-list" type="button" data-action="security-list">安全规则</button>' : ''}
        <button class="action-btn firewall" type="button" data-action="firewall">防火墙</button>
        ${isOCI ? '<button class="action-btn oci-ip-btn-danger" type="button" data-action="terminate">删除实例</button>' : ''}
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
    'update-dns': '更新 DNS',
    edit: '编辑'
  };

  if (action === 'dns-bind') {
    openDNSBindingModal(vm);
    return;
  }
  if (action === 'security-list') {
    openSecurityListModal(vm);
    return;
  }
  if (action === 'edit') {
    openOCIEditModal(vm);
    return;
  }
  if (action === 'ip-manage') {
    openOCIIPModal(vm);
    return;
  }
  if (action === 'terminate') {
    openOCITerminateModal(vm);
    return;
  }
  if (action === 'firewall' || action === 'gcp-firewall') {
    switchToFirewall(vm.provider, vm.accountId);
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

// ------------------------------------------
// Cloud Account Management Functions
// ------------------------------------------
function renderSettingsAccounts(accounts) {
  if (!els.settingsAccountsList) return;
  if (!accounts || accounts.length === 0) {
    els.settingsAccountsList.innerHTML = '<div class="empty-state compact">当前暂未配置任何云账号，点击上方「+ 添加云账号」开始配置。</div>';
    return;
  }

  els.settingsAccountsList.innerHTML = accounts.map(acc => {
    const prov = (acc.provider || '').toLowerCase();
    const name = acc.account;
    const group = acc.group || prov;
    return `
      <div class="account-manage-card" data-provider="${escapeAttr(prov)}" data-account="${escapeAttr(name)}">
        <div class="account-manage-info">
          <span class="prov-badge ${escapeAttr(prov)}">${escapeHtml(prov.toUpperCase())}</span>
          <span class="account-manage-name">${escapeHtml(name)}</span>
          <span class="account-manage-meta">分组: <code>${escapeHtml(group)}</code></span>
        </div>
        <div class="account-manage-actions">
          <button class="ghost-btn account-edit-btn" type="button" data-provider="${escapeAttr(prov)}" data-account="${escapeAttr(name)}" title="编辑此账号">
            <i class="bi bi-pencil"></i>
            <span>编辑</span>
          </button>
          <button class="ghost-btn account-delete-btn" type="button" data-provider="${escapeAttr(prov)}" data-account="${escapeAttr(name)}" style="color: var(--red);" title="从配置文件中删除">
            <i class="bi bi-trash"></i>
            <span>删除</span>
          </button>
        </div>
      </div>
    `;
  }).join('');
}

async function handleSettingsAccountsAction(event) {
  const editBtn = event.target.closest('.account-edit-btn');
  if (editBtn) {
    const provider = editBtn.dataset.provider;
    const account = editBtn.dataset.account;
    await openAccountModal({ provider, account });
    return;
  }

  const delBtn = event.target.closest('.account-delete-btn');
  if (delBtn) {
    const provider = delBtn.dataset.provider;
    const account = delBtn.dataset.account;
    await deleteAccountHandler(provider, account);
    return;
  }
}

async function openAccountModal(accountData = null) {
  if (!els.accountModal) return;
  els.accountModal.hidden = false;
  if (els.accountModalMsg) {
    els.accountModalMsg.textContent = '';
    els.accountModalMsg.className = 'form-message';
  }

  if (accountData) {
    els.accountEditMode.value = 'edit';
    els.accountOriginalName.value = accountData.account;
    els.accountModalTitle.textContent = `编辑云账号 - [${accountData.provider.toUpperCase()}] ${accountData.account}`;
    els.accProvider.value = accountData.provider.toLowerCase();
    els.accProvider.disabled = true;
    els.accName.value = accountData.account;
    updateAccountProviderFields(accountData.provider.toLowerCase());

    try {
      if (els.accountModalMsg) {
        els.accountModalMsg.textContent = '正在读取账号配置详情...';
        els.accountModalMsg.className = 'form-message';
      }
      const data = await fetchJSON(`/api/accounts/${encodeURIComponent(accountData.provider)}/${encodeURIComponent(accountData.account)}`);
      els.accGroup.value = data.group || '';
      const f = data.fields || {};
      if (accountData.provider === 'azure') {
        if (document.getElementById('acc-az-sub')) document.getElementById('acc-az-sub').value = f.subscription_id || '';
        if (document.getElementById('acc-az-tenant')) document.getElementById('acc-az-tenant').value = f.tenant_id || '';
        if (document.getElementById('acc-az-client-id')) document.getElementById('acc-az-client-id').value = f.client_id || '';
        if (document.getElementById('acc-az-client-secret')) document.getElementById('acc-az-client-secret').value = f.client_secret || '';
        if (document.getElementById('acc-az-rg')) document.getElementById('acc-az-rg').value = f.resource_group || '';
        if (document.getElementById('acc-az-location')) document.getElementById('acc-az-location').value = f.location || '';
      } else if (accountData.provider === 'oci') {
        if (document.getElementById('acc-oci-user')) document.getElementById('acc-oci-user').value = f.user || '';
        if (document.getElementById('acc-oci-fingerprint')) document.getElementById('acc-oci-fingerprint').value = f.fingerprint || '';
        if (document.getElementById('acc-oci-tenancy')) document.getElementById('acc-oci-tenancy').value = f.tenancy || '';
        if (document.getElementById('acc-oci-compartment')) document.getElementById('acc-oci-compartment').value = f.compartment_id || '';
        if (document.getElementById('acc-oci-region')) document.getElementById('acc-oci-region').value = f.region || '';
        if (document.getElementById('acc-oci-key-file')) document.getElementById('acc-oci-key-file').value = f.key_file || '';
      } else if (accountData.provider === 'gcp') {
        if (document.getElementById('acc-gcp-project')) document.getElementById('acc-gcp-project').value = f.project_id || '';
        if (document.getElementById('acc-gcp-email')) document.getElementById('acc-gcp-email').value = f.client_email || '';
        if (document.getElementById('acc-gcp-key-file')) document.getElementById('acc-gcp-key-file').value = f.key_file || '';
      }
      if (els.accountModalMsg) {
        els.accountModalMsg.textContent = '';
        els.accountModalMsg.className = 'form-message';
      }
    } catch (err) {
      if (els.accountModalMsg) {
        els.accountModalMsg.textContent = `获取详情失败：${err.message}`;
        els.accountModalMsg.className = 'form-message error';
      }
    }
  } else {
    els.accountEditMode.value = 'create';
    els.accountOriginalName.value = '';
    els.accountModalTitle.textContent = '添加云账号';
    els.accProvider.disabled = false;
    els.accProvider.value = 'azure';
    els.accName.value = '';
    els.accGroup.value = '';

    document.querySelectorAll('#account-modal input:not([type="hidden"]):not([type="submit"]):not([type="button"])').forEach(input => {
      input.value = '';
    });
    updateAccountProviderFields('azure');
  }
}

function closeAccountModal() {
  if (els.accountModal) {
    els.accountModal.hidden = true;
  }
}

function updateAccountProviderFields(prov = null) {
  const provider = prov || els.accProvider?.value || 'azure';
  document.querySelectorAll('.acc-provider-fields').forEach(el => el.style.display = 'none');
  const target = document.getElementById(`acc-fields-${provider}`);
  if (target) {
    target.style.display = 'block';
  }
}

async function handleKeyFileUpload(event) {
  const file = event.target.files?.[0];
  if (!file) return;
  const targetInputId = event.target.dataset.targetInput;
  const targetInput = document.getElementById(targetInputId);

  const formData = new FormData();
  formData.append('keyFile', file);

  if (els.accountModalMsg) {
    els.accountModalMsg.textContent = `正在上传密钥文件 ${file.name}...`;
    els.accountModalMsg.className = 'form-message';
  }

  try {
    const response = await fetch(`${API_BASE}/api/config/upload-key`, {
      method: 'POST',
      body: formData
    });
    const data = await response.json();
    if (!response.ok || data.error) {
      throw new Error(data.error || '上传密钥失败');
    }

    if (targetInput) {
      targetInput.value = data.path;
    }
    if (els.accountModalMsg) {
      els.accountModalMsg.textContent = `密钥文件已安全存入 ${data.path}`;
      els.accountModalMsg.className = 'form-message success';
    }
    addLog(`密钥文件已上传: ${data.path}`, 'success');
  } catch (err) {
    if (els.accountModalMsg) {
      els.accountModalMsg.textContent = `上传失败: ${err.message}`;
      els.accountModalMsg.className = 'form-message error';
    }
    addLog(`上传密钥文件失败: ${err.message}`, 'error');
  }
}

async function handleAccountFormSubmit(event) {
  event.preventDefault();
  const provider = els.accProvider.value;
  const account = els.accName.value.trim();
  const originalAccount = els.accountOriginalName.value.trim();
  const group = els.accGroup.value.trim();
  const editMode = els.accountEditMode.value;

  if (!account) {
    if (els.accountModalMsg) {
      els.accountModalMsg.textContent = '请输入账号标识名称';
      els.accountModalMsg.className = 'form-message error';
    }
    return;
  }

  const fields = {};
  if (group) fields.group = group;

  if (provider === 'azure') {
    fields.subscription_id = document.getElementById('acc-az-sub')?.value.trim() || '';
    fields.tenant_id = document.getElementById('acc-az-tenant')?.value.trim() || '';
    fields.client_id = document.getElementById('acc-az-client-id')?.value.trim() || '';
    fields.client_secret = document.getElementById('acc-az-client-secret')?.value.trim() || '';
    fields.resource_group = document.getElementById('acc-az-rg')?.value.trim() || '';
    fields.location = document.getElementById('acc-az-location')?.value.trim() || '';
  } else if (provider === 'oci') {
    fields.user = document.getElementById('acc-oci-user')?.value.trim() || '';
    fields.fingerprint = document.getElementById('acc-oci-fingerprint')?.value.trim() || '';
    fields.tenancy = document.getElementById('acc-oci-tenancy')?.value.trim() || '';
    fields.compartment_id = document.getElementById('acc-oci-compartment')?.value.trim() || '';
    fields.region = document.getElementById('acc-oci-region')?.value.trim() || '';
    fields.key_file = document.getElementById('acc-oci-key-file')?.value.trim() || '';
  } else if (provider === 'gcp') {
    fields.project_id = document.getElementById('acc-gcp-project')?.value.trim() || '';
    fields.client_email = document.getElementById('acc-gcp-email')?.value.trim() || '';
    fields.key_file = document.getElementById('acc-gcp-key-file')?.value.trim() || '';
  }

  const submitBtn = els.accountForm.querySelector('button[type="submit"]');
  if (submitBtn) submitBtn.disabled = true;
  if (els.accountModalMsg) {
    els.accountModalMsg.textContent = '正在保存并热重载配置...';
    els.accountModalMsg.className = 'form-message';
  }

  try {
    const res = await fetchJSON('/api/accounts', {
      method: 'POST',
      body: {
        provider,
        account,
        originalAccount: editMode === 'edit' ? originalAccount : '',
        group,
        fields
      }
    });

    addLog(res.message || `云账号 ${account} 保存成功。`, 'success');
    closeAccountModal();
    await fetchAccounts();
    loadConfigStatus();
  } catch (err) {
    if (els.accountModalMsg) {
      els.accountModalMsg.textContent = `保存失败: ${err.message}`;
      els.accountModalMsg.className = 'form-message error';
    }
    addLog(`保存云账号失败: ${err.message}`, 'error');
  } finally {
    if (submitBtn) submitBtn.disabled = false;
  }
}

async function deleteAccountHandler(provider, account) {
  if (!confirm(`确定要从配置文件中彻底删除云账号 [${provider.toUpperCase()}] ${account} 吗？\n删除后将立即热重载生效！`)) {
    return;
  }
  addLog(`正在删除云账号 [${provider}] ${account}...`, 'info');

  try {
    const res = await fetchJSON(`/api/accounts/${encodeURIComponent(provider)}/${encodeURIComponent(account)}`, {
      method: 'DELETE'
    });
    addLog(res.message || `云账号 ${account} 已删除。`, 'success');
    await fetchAccounts();
    loadConfigStatus();
  } catch (err) {
    addLog(`删除云账号失败: ${err.message}`, 'error');
    alert(`删除失败: ${err.message}`);
  }
}

function formatInlineReleaseMarkdown(text) {
  let out = escapeHtml(text);
  out = out.replace(/`([^`]+)`/g, '<code>$1</code>');
  out = out.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
  return out;
}

function renderReleaseNotesHTML(markdown) {
  const lines = String(markdown || '').split(/\r?\n/);
  const parts = [];
  let inList = false;
  const closeList = () => {
    if (inList) {
      parts.push('</ul>');
      inList = false;
    }
  };
  for (const raw of lines) {
    const line = raw.trim();
    if (!line) {
      closeList();
      continue;
    }
    if (/^###\s+/.test(line)) {
      closeList();
      parts.push(`<h5>${formatInlineReleaseMarkdown(line.replace(/^###\s+/, ''))}</h5>`);
    } else if (/^##?\s+/.test(line)) {
      closeList();
      parts.push(`<h4>${formatInlineReleaseMarkdown(line.replace(/^##?\s+/, ''))}</h4>`);
    } else if (/^[-*]\s+/.test(line)) {
      if (!inList) {
        parts.push('<ul>');
        inList = true;
      }
      parts.push(`<li>${formatInlineReleaseMarkdown(line.replace(/^[-*]\s+/, ''))}</li>`);
    } else {
      closeList();
      parts.push(`<p style="margin: 4px 0;">${formatInlineReleaseMarkdown(line)}</p>`);
    }
  }
  closeList();
  return parts.join('');
}

function renderUpdateReleaseNotes(data) {
  if (!els.updateReleaseNotesBox) return;
  const notes = (data?.releaseNotes || '').trim();
  const url = (data?.releaseUrl || '').trim();
  if (!notes && !url) {
    els.updateReleaseNotesBox.hidden = true;
    return;
  }
  if (els.updateReleaseNotesTitle) {
    els.updateReleaseNotesTitle.textContent = data?.latestVersion ? `版本更新说明 (${data.latestVersion})` : '版本更新说明';
  }
  if (els.updateReleaseLink) {
    if (url && /^https?:\/\//i.test(url)) {
      els.updateReleaseLink.href = url;
      els.updateReleaseLink.hidden = false;
    } else {
      els.updateReleaseLink.hidden = true;
    }
  }
  if (els.updateReleaseNotesBody) {
    els.updateReleaseNotesBody.innerHTML = notes ? renderReleaseNotesHTML(notes) : '<span style="color:var(--muted);">该版本暂无详细更新说明。</span>';
  }
  els.updateReleaseNotesBox.hidden = false;
}

async function loadUpdateStatus(isManual = false) {
  if (!els.updateMessage) return;
  if (isManual) {
    els.updateMessage.textContent = '正在检查更新...';
    els.updateMessage.className = 'form-message';
  } else {
    els.updateMessage.textContent = '';
    els.updateMessage.className = 'form-message';
  }
  try {
    const proxy = selectedUpdateProxy();
    const queryParts = [];
    if (isManual) {
      queryParts.push('check=true');
    }
    if (proxy) {
      queryParts.push(`download_proxy=${encodeURIComponent(proxy)}`);
    }
    const query = queryParts.length ? `?${queryParts.join('&')}` : '';
    const data = await fetchJSON(`/api/update/status${query}`);
    let verHtml = escapeHtml(data.currentVersion || '-');
    if (data.isRuntime) {
      verHtml += ` <span class="fw-badge fw-badge-egress" style="margin-left: 6px; font-size: 11px; vertical-align: middle;">在线热更版</span> <button type="button" id="reset-update-btn" class="ghost-btn" style="padding: 2px 6px; font-size: 11px; margin-left: 4px; vertical-align: middle;" title="清除热更文件并恢复使用容器镜像内置版本"><i class="bi bi-arrow-counterclockwise"></i> 恢复镜像版</button>`;
    } else if (data.currentVersion) {
      verHtml += ` <span class="fw-badge fw-badge-ingress" style="margin-left: 6px; font-size: 11px; vertical-align: middle;">容器镜像版</span>`;
    }
    els.updateCurrentVersion.innerHTML = verHtml;
    applyDefaultUpdateProxy(data.downloadProxy || '');
    if (isManual) {
      els.updateLatestVersion.textContent = data.latestVersion || '-';
      els.updateAssetName.textContent = data.assetName || '-';
      if (data.checkError) {
        els.updateMessage.textContent = `检查失败：${data.checkError}`;
        if (els.updateReleaseNotesBox) els.updateReleaseNotesBox.hidden = true;
        els.updateMessage.className = 'form-message error';
      } else {
        els.updateMessage.textContent = data.updateAvailable ? '发现可用更新。' : '当前已是最新版本。';
        els.updateMessage.className = `form-message ${data.updateAvailable ? 'success' : ''}`;
        renderUpdateReleaseNotes(data);
      }
    }
  } catch (error) {
    if (isManual) {
      els.updateMessage.textContent = `检查失败：${error.message}`;
      if (els.updateReleaseNotesBox) els.updateReleaseNotesBox.hidden = true;
      els.updateMessage.className = 'form-message error';
    }
  }
}

async function applyUpdate() {
  if (!els.updateMessage) return;
  if (!confirm('确认下载更新并重启程序？')) return;
  els.updateMessage.textContent = '正在下载并安装更新...';
  els.updateMessage.className = 'form-message';
  try {
    const data = await fetchJSON('/api/update/apply', {
      method: 'POST',
      body: { downloadProxy: selectedUpdateProxy() }
    });
    els.updateMessage.textContent = `已安装 ${data.latestVersion || ''}，程序正在重启...`;
    els.updateMessage.className = 'form-message success';
    addLog('程序更新已安装，等待容器自动重启。', 'success');
  } catch (error) {
    els.updateMessage.textContent = `更新失败：${error.message}`;
    els.updateMessage.className = 'form-message error';
  }
}

async function resetToBuiltinVersion() {
  if (!els.updateMessage) return;
  if (!confirm('确认清除在线热更文件并恢复使用容器镜像内置版本？程序将平滑重启。')) return;
  els.updateMessage.textContent = '正在恢复容器镜像内置版本并重启...';
  els.updateMessage.className = 'form-message';
  try {
    const data = await fetchJSON('/api/update/reset', { method: 'POST' });
    els.updateMessage.textContent = data.message || '已重置，程序正在重启...';
    els.updateMessage.className = 'form-message success';
    addLog('已清除在线热更文件，正在恢复使用容器镜像内置版本。', 'success');
    setTimeout(() => location.reload(), 3000);
  } catch (error) {
    els.updateMessage.textContent = `重置失败：${error.message}`;
    els.updateMessage.className = 'form-message error';
  }
}

async function saveUpdateProxy() {
  if (!els.updateMessage) return;
  els.updateMessage.textContent = '正在保存加速源...';
  els.updateMessage.className = 'form-message';
  try {
    const proxy = selectedUpdateProxy();
    const data = await fetchJSON('/api/settings/update', {
      method: 'POST',
      body: { downloadProxy: proxy }
    });
    els.updateMessage.textContent = data.message || '加速源配置已保存。';
    els.updateMessage.className = 'form-message success';
    addLog('加速源配置已保存并重载。', 'success');
  } catch (error) {
    els.updateMessage.textContent = `保存失败：${error.message}`;
    els.updateMessage.className = 'form-message error';
  }
}

function applyDefaultUpdateProxy(proxy) {
  if (!els.updateProxyMode || els.updateProxyMode.dataset.initialized === 'true') return;
  if (proxy === 'https://gh-proxy.com/' || proxy === 'https://gh-proxy.com') {
    els.updateProxyMode.value = 'https://gh-proxy.com/';
  } else if (proxy === '') {
    els.updateProxyMode.value = '';
  } else {
    els.updateProxyMode.value = 'custom';
    els.updateCustomProxy.value = proxy;
  }
  els.updateProxyMode.dataset.initialized = 'true';
  updateProxyModeChanged();
}

function updateProxyModeChanged() {
  if (!els.updateCustomProxyField || !els.updateProxyMode) return;
  els.updateCustomProxyField.hidden = els.updateProxyMode.value !== 'custom';
}

function selectedUpdateProxy() {
  if (!els.updateProxyMode) return '';
  if (els.updateProxyMode.value === 'custom') {
    return els.updateCustomProxy?.value?.trim() || '';
  }
  return els.updateProxyMode.value;
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

async function loadProxyPage() {
  const list = document.getElementById('proxy-list');
  const bindingsList = document.getElementById('proxy-bindings-list');
  if (!list || !bindingsList) return;
  list.innerHTML = '<div class="empty-state compact">正在读取代理池...</div>';

  try {
    const [proxyData, accounts] = await Promise.all([
      fetchJSON('/api/proxies'),
      fetchJSON('/api/accounts')
    ]);
    proxyPool = Array.isArray(proxyData.proxies) ? proxyData.proxies : [];
    proxyBindings = Array.isArray(proxyData.bindings) ? proxyData.bindings : [];
    proxyAccounts = Array.isArray(accounts) ? accounts : [];
    renderProxyPool();
    renderProxyBindingAccounts();
    renderProxySelectors();
    applySelectedAccountBinding();
    renderProxyBindings();
    const summary = document.getElementById('proxy-pool-summary');
    if (summary) {
      const path = proxyData.configPath ? ` · ${proxyData.configPath}` : '';
      summary.textContent = `${proxyPool.length} 个代理 · ${proxyBindings.length} 个账号绑定${path}`;
    }
  } catch (error) {
    list.innerHTML = `<div class="empty-state compact error">读取失败：${escapeHtml(error.message)}</div>`;
    showProxyMessage('proxy-create-message', error.message, true);
  }
}

function renderProxyPool() {
  const list = document.getElementById('proxy-list');
  if (!list) return;
  if (proxyPool.length === 0) {
    list.innerHTML = '<div class="empty-state compact">代理池为空。</div>';
    return;
  }
  list.innerHTML = proxyPool.map(proxy => {
    const protocol = proxyProtocolLabel(proxy.url);
    return `
      <div class="proxy-row" data-proxy-id="${escapeAttr(proxy.id)}">
        <span class="proxy-protocol-badge">${escapeHtml(protocol)}</span>
        <input class="proxy-row-input proxy-url-input mono" type="text" value="${escapeAttr(proxy.url)}" aria-label="代理地址">
        <input class="proxy-row-input proxy-remark-input" type="text" maxlength="160" value="${escapeAttr(proxy.remark || '')}" placeholder="备注" aria-label="代理备注">
        <span class="proxy-status" data-proxy-status><i class="bi bi-circle"></i><span>未测试</span></span>
        <span class="proxy-row-actions">
          <button class="proxy-icon-btn" type="button" data-proxy-action="save" title="保存代理"><i class="bi bi-floppy"></i></button>
          <button class="proxy-icon-btn" type="button" data-proxy-action="test" title="测试代理"><i class="bi bi-speedometer2"></i></button>
          <button class="proxy-icon-btn danger" type="button" data-proxy-action="delete" title="删除代理"><i class="bi bi-trash3"></i></button>
        </span>
      </div>`;
  }).join('');
}

function proxyProtocolLabel(rawURL) {
  const scheme = String(rawURL || '').split(':', 1)[0].toUpperCase();
  return scheme.startsWith('SOCK') ? 'SOCKS5' : 'HTTP';
}

async function createProxyEntry(event) {
  event.preventDefault();
  const protocol = document.getElementById('proxy-create-protocol').value;
  const addressInput = document.getElementById('proxy-create-address');
  const remarkInput = document.getElementById('proxy-create-remark');
  let address = addressInput.value.trim();
  if (!address.includes('://')) address = `${protocol}://${address}`;
  showProxyMessage('proxy-create-message', '正在添加...', false);
  try {
    await fetchJSON('/api/proxies', {
      method: 'POST',
      body: { url: address, remark: remarkInput.value.trim() }
    });
    addressInput.value = '';
    remarkInput.value = '';
    showProxyMessage('proxy-create-message', '代理已添加。', false);
    addLog('代理池已添加一条代理。', 'success');
    await loadProxyPage();
  } catch (error) {
    showProxyMessage('proxy-create-message', error.message, true);
  }
}

function toggleProxyImport() {
  const panel = document.getElementById('proxy-import-panel');
  if (!panel) return;
  panel.hidden = !panel.hidden;
  if (!panel.hidden) document.getElementById('proxy-import-content')?.focus();
}

async function importProxyEntries() {
  const content = document.getElementById('proxy-import-content')?.value || '';
  showProxyMessage('proxy-import-message', '正在导入...', false);
  try {
    const result = await fetchJSON('/api/proxies/import', {
      method: 'POST',
      body: { content }
    });
    document.getElementById('proxy-import-content').value = '';
    showProxyMessage('proxy-import-message', `已导入 ${result.imported} 条代理。`, false);
    addLog(`代理池批量导入 ${result.imported} 条代理。`, 'success');
    await loadProxyPage();
  } catch (error) {
    showProxyMessage('proxy-import-message', error.message, true);
  }
}

async function handleProxyRowAction(event) {
  const button = event.target.closest('[data-proxy-action]');
  if (!button) return;
  const row = button.closest('.proxy-row');
  const proxyID = row?.dataset.proxyId;
  if (!row || !proxyID) return;
  const action = button.dataset.proxyAction;

  if (action === 'delete') {
    if (!confirm('删除此代理？使用它作为主代理的账号绑定也会被删除。')) return;
    button.disabled = true;
    try {
      await fetchJSON(`/api/proxies/${encodeURIComponent(proxyID)}`, { method: 'DELETE' });
      addLog('代理已删除。', 'success');
      await loadProxyPage();
    } catch (error) {
      showProxyMessage('proxy-create-message', error.message, true);
      button.disabled = false;
    }
    return;
  }

  if (action === 'save') {
    button.disabled = true;
    try {
      await fetchJSON(`/api/proxies/${encodeURIComponent(proxyID)}`, {
        method: 'PUT',
        body: {
          url: row.querySelector('.proxy-url-input').value.trim(),
          remark: row.querySelector('.proxy-remark-input').value.trim()
        }
      });
      showProxyMessage('proxy-create-message', '代理修改已保存。', false);
      await loadProxyPage();
    } catch (error) {
      showProxyMessage('proxy-create-message', error.message, true);
      button.disabled = false;
    }
    return;
  }

  if (action === 'test') {
    const status = row.querySelector('[data-proxy-status]');
    button.disabled = true;
    status.className = 'proxy-status';
    status.innerHTML = '<i class="bi bi-arrow-repeat"></i><span>测试中</span>';
    try {
      const result = await fetchJSON(`/api/proxies/${encodeURIComponent(proxyID)}/test`, { method: 'POST' });
      if (result.success) {
        status.className = 'proxy-status success';
        status.innerHTML = `<i class="bi bi-check-circle"></i><span>${escapeHtml(result.latencyMs)} ms</span>`;
      } else {
        status.className = 'proxy-status error';
        status.innerHTML = `<i class="bi bi-x-circle"></i><span title="${escapeAttr(result.error || '连接失败')}">连接失败</span>`;
      }
    } catch (error) {
      status.className = 'proxy-status error';
      status.innerHTML = `<i class="bi bi-x-circle"></i><span title="${escapeAttr(error.message)}">测试失败</span>`;
    } finally {
      button.disabled = false;
    }
  }
}

function renderProxyBindingAccounts() {
  const select = document.getElementById('proxy-binding-account');
  if (!select) return;
  const previous = select.value;
  if (proxyAccounts.length === 0) {
    select.innerHTML = '<option value="">无可用云账号</option>';
    select.disabled = true;
    const submit = document.querySelector('.proxy-binding-submit');
    if (submit) submit.disabled = true;
    return;
  }
  select.disabled = false;
  const submit = document.querySelector('.proxy-binding-submit');
  if (submit) submit.disabled = proxyPool.length === 0;
  select.innerHTML = proxyAccounts.map((account, index) => `
    <option value="${index}" data-provider="${escapeAttr(account.provider)}" data-account="${escapeAttr(account.account)}">
      ${escapeHtml(account.provider.toUpperCase())} / ${escapeHtml(account.account)}
    </option>`).join('');
  if (Array.from(select.options).some(option => option.value === previous)) select.value = previous;
}

function renderProxySelectors() {
  const primary = document.getElementById('proxy-binding-primary');
  const fallback = document.getElementById('proxy-binding-fallback');
  if (!primary || !fallback) return;
  const currentPrimary = primary.value;
  const currentFallback = fallback.value;
  if (proxyPool.length === 0) {
    primary.innerHTML = '<option value="">请先添加代理</option>';
    fallback.innerHTML = '<option value="none">None（不回退）</option><option value="direct">直连</option>';
    primary.disabled = true;
    fallback.disabled = true;
    const submit = document.querySelector('.proxy-binding-submit');
    if (submit) submit.disabled = true;
    return;
  }
  primary.disabled = false;
  fallback.disabled = false;
  const submit = document.querySelector('.proxy-binding-submit');
  if (submit) submit.disabled = proxyAccounts.length === 0;
  primary.innerHTML = proxyPool.map(proxy => `<option value="${escapeAttr(proxy.id)}">${escapeHtml(proxyOptionLabel(proxy))}</option>`).join('');
  if (proxyPool.some(proxy => proxy.id === currentPrimary)) primary.value = currentPrimary;
  const selectedPrimary = primary.value;
  fallback.innerHTML = '<option value="none">None（不回退）</option><option value="direct">直连</option>' + proxyPool
    .filter(proxy => proxy.id !== selectedPrimary)
    .map(proxy => `<option value="${escapeAttr(proxy.id)}">${escapeHtml(proxyOptionLabel(proxy))}</option>`)
    .join('');
  const fallbackExists = currentFallback === 'none' || currentFallback === 'direct' ||
    proxyPool.some(proxy => proxy.id === currentFallback && proxy.id !== selectedPrimary);
  fallback.value = fallbackExists ? currentFallback : 'none';
}

function applySelectedAccountBinding() {
  const accountOption = document.getElementById('proxy-binding-account')?.selectedOptions[0];
  const primary = document.getElementById('proxy-binding-primary');
  const fallback = document.getElementById('proxy-binding-fallback');
  if (!accountOption || !primary || !fallback || proxyPool.length === 0) return;
  const binding = proxyBindings.find(item =>
    item.provider === accountOption.dataset.provider && item.account === accountOption.dataset.account
  );
  primary.value = binding && proxyPool.some(proxy => proxy.id === binding.proxyId)
    ? binding.proxyId
    : proxyPool[0].id;
  renderProxySelectors();
  const fallbackValue = binding?.fallbackProxyId || 'none';
  const fallbackExists = fallbackValue === 'none' || fallbackValue === 'direct' ||
    proxyPool.some(proxy => proxy.id === fallbackValue && proxy.id !== primary.value);
  fallback.value = fallbackExists ? fallbackValue : 'none';
}

function proxyOptionLabel(proxy) {
  return proxy.remark ? `${proxy.remark} · ${redactProxyURL(proxy.url)}` : redactProxyURL(proxy.url);
}

function redactProxyURL(rawURL) {
  try {
    const parsed = new URL(rawURL);
    if (parsed.password) parsed.password = '*****';
    return parsed.toString();
  } catch (error) {
    return rawURL;
  }
}

async function saveProxyBinding(event) {
  event.preventDefault();
  const accountOption = document.getElementById('proxy-binding-account').selectedOptions[0];
  const provider = accountOption?.dataset.provider || '';
  const account = accountOption?.dataset.account || '';
  const proxyId = document.getElementById('proxy-binding-primary').value;
  const fallbackProxyId = document.getElementById('proxy-binding-fallback').value;
  showProxyMessage('proxy-binding-message', '正在保存绑定...', false);
  try {
    await fetchJSON('/api/proxy-bindings', {
      method: 'PUT',
      body: { provider, account, proxyId, fallbackProxyId }
    });
    showProxyMessage('proxy-binding-message', '账号代理绑定已生效。', false);
    addLog(`账号代理绑定已保存：${provider}/${account}`, 'success');
    await loadProxyPage();
  } catch (error) {
    showProxyMessage('proxy-binding-message', error.message, true);
  }
}

function renderProxyBindings() {
  const list = document.getElementById('proxy-bindings-list');
  if (!list) return;
  if (proxyBindings.length === 0) {
    list.innerHTML = '<div class="empty-state compact">暂无账号绑定。</div>';
    return;
  }
  list.innerHTML = proxyBindings.map(binding => {
    const primary = proxyPool.find(proxy => proxy.id === binding.proxyId);
    const fallback = proxyPool.find(proxy => proxy.id === binding.fallbackProxyId);
    const fallbackValue = binding.fallbackProxyId || 'none';
    const fallbackLabel = fallbackValue === 'none'
      ? 'None（不回退）'
      : fallbackValue === 'direct'
        ? '直连'
        : fallback ? proxyOptionLabel(fallback) : fallbackValue;
    const route = `${primary ? proxyOptionLabel(primary) : binding.proxyId} → ${fallbackLabel}`;
    return `
      <div class="proxy-binding-row"
        data-provider="${escapeAttr(binding.provider)}"
        data-account="${escapeAttr(binding.account)}">
        <span class="proxy-binding-target">${escapeHtml(binding.provider.toUpperCase())} / ${escapeHtml(binding.account)}</span>
        <span class="proxy-binding-route">${escapeHtml(route)}</span>
        <button class="proxy-icon-btn danger" type="button" data-binding-action="delete" title="解除绑定"><i class="bi bi-x-lg"></i></button>
      </div>`;
  }).join('');
}

async function handleProxyBindingAction(event) {
  const button = event.target.closest('[data-binding-action="delete"]');
  if (!button) return;
  const row = button.closest('.proxy-binding-row');
  if (!row) return;
  button.disabled = true;
  const query = new URLSearchParams({
    provider: row.dataset.provider,
    account: row.dataset.account
  });
  try {
    await fetchJSON(`/api/proxy-bindings?${query}`, { method: 'DELETE' });
    addLog(`已解除账号代理绑定：${row.dataset.provider}/${row.dataset.account}`, 'success');
    await loadProxyPage();
  } catch (error) {
    showProxyMessage('proxy-binding-message', error.message, true);
    button.disabled = false;
  }
}

function showProxyMessage(elementID, message, isError) {
  const element = document.getElementById(elementID);
  if (!element) return;
  element.textContent = message;
  element.className = `form-message ${isError ? 'error' : 'success'}`;
}

async function fetchJSON(path, options = {}) {
  const fetchOptions = {
    method: options.method || 'GET',
    cache: 'no-store',
    headers: { Accept: 'application/json' }
  };

  if (options.body !== undefined) {
    fetchOptions.headers['Content-Type'] = 'application/json';
    fetchOptions.body = typeof options.body === 'string' ? options.body : JSON.stringify(options.body);
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

// ==================== OCI Data Transfer Monitoring ====================

function updateDTMonitorVisibility() {
  const panel = document.getElementById('dt-monitor-panel');
  if (!panel) return;
  if (selectedAccount && selectedAccount.provider === 'oci') {
    panel.hidden = false;
    document.getElementById('dt-monitor-account-label').textContent = selectedAccount.account;
    loadDTMonitorStatus();
  } else {
    panel.hidden = true;
  }
}

async function loadDTMonitorStatus() {
  if (!selectedAccount || selectedAccount.provider !== 'oci') return;
  const reqAccount = selectedAccount.account;

  // Clear display to prevent showing previous account's data
  document.getElementById('dt-usage-value').textContent = '加载中...';
  document.getElementById('dt-threshold-value').textContent = '-';
  document.getElementById('dt-percentage-value').textContent = '0%';
  const progressFill = document.getElementById('dt-progress-fill');
  if (progressFill) {
    progressFill.style.width = '0%';
    progressFill.className = 'dt-progress-fill';
  }
  document.getElementById('dt-last-update').textContent = '正在获取最新状态...';

  try {
    const data = await fetchJSON(`/api/oci/${encodeURIComponent(reqAccount)}/data-transfer/status`);
    if (selectedAccount && selectedAccount.account === reqAccount) {
      updateDTDisplay(data);
    }
  } catch (error) {
    if (selectedAccount && selectedAccount.account === reqAccount) {
      addLog(`加载数据传输监控状态失败：${error.message}`, 'error');
    }
  }
}

function updateDTDisplay(data) {
  const result = data.lastResult;
  const config = data.config || {};

  // Update stats
  if (result && !result.error) {
    document.getElementById('dt-usage-value').textContent = `${result.usageGB.toFixed(2)} GB`;
    document.getElementById('dt-threshold-value').textContent = `${result.threshold.toFixed(0)} GB`;
    const pct = Math.min(result.percentage, 100);
    document.getElementById('dt-percentage-value').textContent = `${result.percentage.toFixed(1)}%`;
    const progressFill = document.getElementById('dt-progress-fill');
    if (progressFill) {
      progressFill.style.width = `${pct}%`;
      progressFill.className = 'dt-progress-fill' + (result.percentage > 90 ? ' danger' : result.percentage > 70 ? ' warning' : '');
    }

    if (result.queryTime) {
      const t = new Date(result.queryTime);
      document.getElementById('dt-last-update').textContent = `最后更新：${t.toLocaleString('zh-CN')}`;
    }
  } else if (result && result.error) {
    document.getElementById('dt-usage-value').textContent = '查询失败';
    document.getElementById('dt-last-update').textContent = `错误：${result.error}`;
  } else {
    document.getElementById('dt-usage-value').textContent = '-';
    document.getElementById('dt-threshold-value').textContent = '-';
    document.getElementById('dt-percentage-value').textContent = '0%';
    const progressFill = document.getElementById('dt-progress-fill');
    if (progressFill) {
      progressFill.style.width = '0%';
      progressFill.className = 'dt-progress-fill';
    }
    document.getElementById('dt-last-update').textContent = '暂无监控数据，请点击手动获取或开启监控';
  }

  // Update monitor status badge
  const statusBadge = document.getElementById('dt-monitor-status');
  if (data.running) {
    statusBadge.textContent = '运行中';
    statusBadge.className = 'dt-monitor-status-badge on';
  } else {
    statusBadge.textContent = config.enabled ? '已停止' : '未启用';
    statusBadge.className = 'dt-monitor-status-badge off';
  }

  // Update monitor logs
  const logsEl = document.getElementById('dt-monitor-logs');
  if (logsEl) {
    const logs = Array.isArray(data.logs) ? data.logs.slice().reverse() : [];
    logsEl.innerHTML = logs.length > 0
      ? logs.map(l => `<div>${escapeHtml(l)}</div>`).join('')
      : '暂无监控日志';
  }

  // Update settings form
  document.getElementById('dt-enabled').checked = Boolean(config.enabled);
  document.getElementById('dt-interval').value = config.interval || 300;
  document.getElementById('dt-threshold-input').value = config.threshold || 9000;
  document.getElementById('dt-auto-stop').checked = Boolean(config.autoStop);
  document.getElementById('dt-stop-method').value = config.stopMethod || 'soft';
}

async function fetchDTManual() {
  if (!selectedAccount || selectedAccount.provider !== 'oci') return;
  const reqAccount = selectedAccount.account;
  const btn = document.getElementById('dt-fetch-btn');
  btn.disabled = true;
  btn.innerHTML = '<i class="bi bi-hourglass-split"></i> 查询中...';
  addLog(`正在查询 OCI 账号 ${reqAccount} 的数据传输用量...`, 'info');

  try {
    const result = await fetchJSON(`/api/oci/${encodeURIComponent(reqAccount)}/data-transfer`);
    if (selectedAccount && selectedAccount.account === reqAccount) {
      if (result.error) {
        addLog(`数据传输查询失败：${result.error}`, 'error');
      } else {
        addLog(`OCI ${reqAccount} 当月数据传输：${result.usageGB.toFixed(2)} GB / ${result.threshold.toFixed(0)} GB (${result.percentage.toFixed(1)}%)`, 'success');
      }
      loadDTMonitorStatus();
    }
  } catch (error) {
    if (selectedAccount && selectedAccount.account === reqAccount) {
      addLog(`数据传输查询失败：${error.message}`, 'error');
    }
  } finally {
    if (selectedAccount && selectedAccount.account === reqAccount) {
      btn.disabled = false;
      btn.innerHTML = '<i class="bi bi-arrow-clockwise"></i> 手动获取';
    }
  }
}

function toggleDTSettings() {
  const panel = document.getElementById('dt-settings-panel');
  panel.hidden = !panel.hidden;
}

async function saveDTConfig() {
  if (!selectedAccount || selectedAccount.provider !== 'oci') return;
  const msgEl = document.getElementById('dt-config-message');
  msgEl.textContent = '保存中...';
  msgEl.className = 'form-message';

  const autoStop = document.getElementById('dt-auto-stop').checked;
  if (autoStop && !confirm('⚠️ 你确定要启用"超阈值自动停机"功能吗？\n\n当数据传输用量超过阈值时，系统将自动停止该账号下所有正在运行的实例。')) {
    msgEl.textContent = '已取消。';
    msgEl.className = 'form-message';
    return;
  }

  const payload = {
    enabled: document.getElementById('dt-enabled').checked,
    interval: Math.max(60, Number(document.getElementById('dt-interval').value) || 300),
    threshold: Math.max(1, Number(document.getElementById('dt-threshold-input').value) || 9000),
    autoStop: autoStop,
    stopMethod: document.getElementById('dt-stop-method').value || 'soft'
  };

  try {
    await fetchJSON(`/api/oci/${encodeURIComponent(selectedAccount.account)}/data-transfer/config`, {
      method: 'POST',
      body: payload
    });
    msgEl.textContent = '监控配置已保存。';
    msgEl.className = 'form-message success';
    addLog(`OCI ${selectedAccount.account} 数据传输监控配置已保存。`, 'success');
    loadDTMonitorStatus();
  } catch (error) {
    msgEl.textContent = error.message;
    msgEl.className = 'form-message error';
  }
}

// Bind DT monitor events
document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('dt-fetch-btn')?.addEventListener('click', fetchDTManual);
  document.getElementById('dt-settings-toggle')?.addEventListener('click', toggleDTSettings);
  document.getElementById('dt-save-config-btn')?.addEventListener('click', saveDTConfig);
});

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

function maskDomain(domain) {
  if (!domain) return '';
  const parts = domain.split('.');
  if (parts.length <= 1) return domain;
  return parts[0] + '.' + parts.slice(1).map(() => '**').join('.');
}

function maskRawDNSConfig(rawText) {
  if (!rawText) return rawText;
  return rawText.replace(/^(domain\s*=\s*)([^\r\n]+)/gm, (match, prefix, domainVal) => {
    return prefix + maskDomain(domainVal.trim());
  });
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
          <span>${escapeHtml(b.provider)}/${escapeHtml(b.account)} → ${escapeHtml(maskDomain(b.domain))}</span>
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
    el.textContent = maskRawDNSConfig(data.content) || '（空）';
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

// ==================== OCI Instance Edit ====================

let ociEditModalVM = null;
let ociEditOptions = null;
let ociEditLoadSeq = 0;

const OCI_SHAPE_FAMILIES = [
  { key: 'amd', label: 'AMD', description: '弹性 OCPU 数。当代 AMD 处理器。' },
  { key: 'intel', label: 'Intel', description: '弹性 OCPU 数。当代 Intel 处理器。' },
  { key: 'ampere', label: 'Ampere', description: '基于 ARM 的处理器。' },
  { key: 'special', label: '专用和上一代', description: '密集 I/O、GPU、HPC、Generic 以及旧规格。' }
];

function openOCIEditModal(vm) {
  ociEditModalVM = vm;
  ociEditOptions = null;
  document.getElementById('oci-edit-title').textContent = `编辑 OCI 实例 - ${vm.accountId}/${vm.name}`;
  document.getElementById('oci-edit-body').innerHTML = '<div class="empty-state compact">加载中...</div>';
  document.getElementById('oci-edit-message').textContent = '';
  document.getElementById('oci-edit-message').className = 'form-message';
  document.getElementById('oci-edit-save').disabled = true;
  document.getElementById('oci-edit-modal').hidden = false;
  loadOCIEditOptions(vm);
}

function closeOCIEditModal() {
  document.getElementById('oci-edit-modal').hidden = true;
  ociEditModalVM = null;
  ociEditOptions = null;
}

async function loadOCIEditOptions(vm, shape = '', preserve = {}) {
  const seq = ++ociEditLoadSeq;
  const msgEl = document.getElementById('oci-edit-message');
  if (shape) {
    msgEl.textContent = '正在读取该规格的可用上限...';
    msgEl.className = 'form-message';
  }

  try {
    const query = shape ? `?shape=${encodeURIComponent(shape)}` : '';
    const data = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/edit-options${query}`);
    if (seq !== ociEditLoadSeq || !ociEditModalVM) return;
    ociEditOptions = data;
    renderOCIEditForm(data, preserve);
    document.getElementById('oci-edit-save').disabled = false;
  } catch (err) {
    if (seq !== ociEditLoadSeq) return;
    document.getElementById('oci-edit-body').innerHTML = `<div class="empty-state compact error">${escapeHtml(err.message)}</div>`;
    document.getElementById('oci-edit-save').disabled = true;
  }
}

function currentOCIEditFormValues() {
  return {
    displayName: document.getElementById('oci-edit-display-name')?.value?.trim() || '',
    allowDowntime: document.getElementById('oci-edit-allow-downtime')?.checked ?? true
  };
}

function renderOCIEditForm(data, preserve = {}) {
  const instance = data.instance || {};
  const shape = data.selectedShape || {};
  const sameShape = shape.name === instance.shape;
  const ocpus = preserve.ocpus ?? (sameShape ? instance.ocpus : ociShapeDefaultOcpus(shape, data.limits?.ocpu));
  const memory = preserve.memoryInGBs ?? (sameShape ? instance.memoryInGBs : ociShapeDefaultMemory(shape, ocpus, data.limits?.memory));
  const displayName = preserve.displayName ?? instance.displayName ?? instance.id ?? '';
  const allowDowntime = preserve.allowDowntime ?? true;

  document.getElementById('oci-edit-body').innerHTML = `
    <div class="oci-edit-summary">
      <div><span>当前规格</span><strong>${escapeHtml(instance.shape || '-')}</strong></div>
      <div><span>当前 OCPU</span><strong>${escapeHtml(formatOCIAmount(instance.ocpus || 0))}</strong></div>
      <div><span>当前内存</span><strong>${escapeHtml(formatOCIAmount(instance.memoryInGBs || 0))} GB</strong></div>
      <div><span>可用区</span><strong>${escapeHtml(instance.availabilityDomain || '-')}</strong></div>
    </div>

    <label class="field">
      <span>实例名称</span>
      <input id="oci-edit-display-name" type="text" value="${escapeAttr(displayName)}">
    </label>

    <div class="oci-edit-section">
      <div class="oci-edit-section-title">配置系列</div>
      <div class="oci-family-grid">
        ${ociFamilyCardsHTML(data.shapes || [], shape.family)}
      </div>
    </div>

    <label class="field">
      <span>配置名称</span>
      <select id="oci-edit-shape">
        ${ociShapeOptionsHTML(data.shapes || [], shape.name)}
      </select>
    </label>

    <div class="oci-selected-shape">
      <div>
        <strong>${escapeHtml(shape.name || '-')}</strong>
        <span>${escapeHtml(shape.processorDescription || (shape.isFlexible ? 'Flex shape' : 'Fixed shape'))}</span>
      </div>
      <div class="oci-shape-badges">
        ${shape.isFlexible ? '<span>Flex</span>' : '<span>固定规格</span>'}
        ${shape.maxVnicAttachments ? `<span>最大 VNIC ${escapeHtml(shape.maxVnicAttachments)}</span>` : ''}
      </div>
    </div>

    <div class="oci-edit-grid">
      <label class="field">
        <span>OCPU 数</span>
        <input id="oci-edit-ocpus" type="number" min="0" step="0.25" value="${escapeAttr(ociNumberInputValue(ocpus))}" ${shape.isFlexible ? '' : 'disabled'}>
        <small id="oci-edit-ocpu-range" class="field-hint"></small>
      </label>
      <label class="field">
        <span>内存（GB）</span>
        <input id="oci-edit-memory" type="number" min="0" step="1" value="${escapeAttr(ociNumberInputValue(memory))}" ${shape.isFlexible ? '' : 'disabled'}>
        <small id="oci-edit-memory-range" class="field-hint"></small>
      </label>
    </div>

    <label class="switch-row oci-downtime-row">
      <input id="oci-edit-allow-downtime" type="checkbox" ${allowDowntime ? 'checked' : ''}>
      <span>允许停机完成规格变更</span>
    </label>

    <div id="oci-edit-limit-note" class="oci-limit-note"></div>
  `;

  updateOCIEditRangeHints();
  renderOCIEditWarnings(data.warnings || []);
}

function ociFamilyCardsHTML(shapes, selectedFamily) {
  return OCI_SHAPE_FAMILIES.map(family => {
    const count = shapes.filter(shape => shape.family === family.key).length;
    const disabled = count === 0;
    return `
      <button class="oci-family-card ${selectedFamily === family.key ? 'active' : ''}" data-family="${escapeAttr(family.key)}" type="button" ${disabled ? 'disabled' : ''}>
        <strong>${escapeHtml(family.label)}</strong>
        <span>${escapeHtml(family.description)}</span>
      </button>
    `;
  }).join('');
}

function ociShapeOptionsHTML(shapes, selectedName) {
  return OCI_SHAPE_FAMILIES.map(family => {
    const familyShapes = shapes.filter(shape => shape.family === family.key);
    if (familyShapes.length === 0) return '';
    return `
      <optgroup label="${escapeAttr(family.label)}">
        ${familyShapes.map(shape => `
          <option value="${escapeAttr(shape.name)}" ${shape.name === selectedName ? 'selected' : ''}>
            ${escapeHtml(shape.name)}${shape.isFlexible ? ' · Flex' : ''}
          </option>
        `).join('')}
      </optgroup>
    `;
  }).join('');
}

function updateOCIEditRangeHints() {
  if (!ociEditOptions?.selectedShape) return;
  const shape = ociEditOptions.selectedShape;
  const ocpuInput = document.getElementById('oci-edit-ocpus');
  const memoryInput = document.getElementById('oci-edit-memory');
  const ocpuRangeEl = document.getElementById('oci-edit-ocpu-range');
  const memoryRangeEl = document.getElementById('oci-edit-memory-range');
  const noteEl = document.getElementById('oci-edit-limit-note');
  if (!ocpuInput || !memoryInput || !ocpuRangeEl || !memoryRangeEl) return;

  const ranges = ociEditComputedRanges();
  ocpuInput.min = ranges.ocpuMin;
  ocpuInput.max = ranges.ocpuMax;
  memoryInput.min = ranges.memoryMin;
  memoryInput.max = ranges.memoryMax;

  ocpuRangeEl.textContent = ociRangeText('OCPU', ranges.ocpuMin, ranges.ocpuMax, ociEditOptions.limits?.ocpu);
  memoryRangeEl.textContent = ociRangeText('内存', ranges.memoryMin, ranges.memoryMax, ociEditOptions.limits?.memory, 'GB');

  const notes = [];
  if (!shape.isFlexible) {
    notes.push('当前选择的是固定规格，OCPU 和内存会随规格自动确定。');
  }
  notes.push('当前仅显示规格允许范围；账号配额和实时容量由 OCI 在保存时最终校验。');
  noteEl.textContent = notes.join(' ');
}

function ociEditComputedRanges() {
  const shape = ociEditOptions.selectedShape || {};
  const limits = ociEditOptions.limits || {};
  const ocpuLimit = limits.ocpu || {};
  const memoryLimit = limits.memory || {};

  const ocpuMin = numberOrFallback(ocpuLimit.shapeMin, shape.ocpuOptions?.min, shape.ocpus, 0);
  const ocpuMax = numberOrFallback(ocpuLimit.effectiveMax, ocpuLimit.shapeMax, shape.ocpuOptions?.max, shape.ocpus, ocpuMin);
  const selectedOcpus = Number(document.getElementById('oci-edit-ocpus')?.value || ocpuMin) || ocpuMin;

  let memoryMin = numberOrFallback(memoryLimit.shapeMin, shape.memoryOptions?.minInGBs, shape.memoryInGBs, 0);
  let memoryMax = numberOrFallback(memoryLimit.effectiveMax, memoryLimit.shapeMax, shape.memoryOptions?.maxInGBs, shape.memoryInGBs, memoryMin);
  if (shape.memoryOptions?.minPerOcpuInGBs) {
    memoryMin = Math.max(memoryMin, selectedOcpus * Number(shape.memoryOptions.minPerOcpuInGBs));
  }
  if (shape.memoryOptions?.maxPerOcpuInGBs) {
    memoryMax = Math.min(memoryMax, selectedOcpus * Number(shape.memoryOptions.maxPerOcpuInGBs));
  }

  return { ocpuMin, ocpuMax, memoryMin, memoryMax };
}

function ociRangeText(label, min, max, limit = {}, unit = '') {
  const suffix = unit ? ` ${unit}` : '';
  return `${label} 应介于 ${formatOCIAmount(min)} 和 ${formatOCIAmount(max)}${suffix} 之间。规格范围 ${formatOCIAmount(min)} - ${formatOCIAmount(limit.shapeMax || max)}${suffix}`;
}

function renderOCIEditWarnings(warnings) {
  const msgEl = document.getElementById('oci-edit-message');
  if (!warnings.length) {
    msgEl.textContent = '';
    msgEl.className = 'form-message';
    return;
  }
  msgEl.textContent = warnings.join('；');
  msgEl.className = 'form-message';
}

function ociShapeDefaultOcpus(shape = {}, limit = {}) {
  return numberOrFallback(shape.memoryOptions?.defaultOcpus, shape.ocpus, limit.shapeMin, shape.ocpuOptions?.min, 0);
}

function ociShapeDefaultMemory(shape = {}, ocpus = 0, limit = {}) {
  const direct = numberOrFallback(shape.memoryOptions?.defaultMemoryInGBs, shape.memoryInGBs, 0);
  if (direct > 0) return direct;
  if (shape.memoryOptions?.defaultPerOcpuInGBs && ocpus > 0) {
    return Number(shape.memoryOptions.defaultPerOcpuInGBs) * ocpus;
  }
  return numberOrFallback(limit.shapeMin, shape.memoryOptions?.minInGBs, 0);
}

function ociNumberInputValue(value) {
  const n = Number(value);
  if (!Number.isFinite(n) || n <= 0) return '';
  return formatOCIAmount(n);
}

function numberOrFallback(...values) {
  for (const value of values) {
    const n = Number(value);
    if (Number.isFinite(n) && n > 0) return n;
  }
  return 0;
}

function formatOCIAmount(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return '0';
  if (Math.abs(n - Math.round(n)) < 0.000001) return String(Math.round(n));
  return n.toFixed(2).replace(/\.?0+$/, '');
}

async function saveOCIEditModal() {
  if (!ociEditModalVM || !ociEditOptions?.selectedShape) return;
  const msgEl = document.getElementById('oci-edit-message');
  const saveBtn = document.getElementById('oci-edit-save');
  const shape = ociEditOptions.selectedShape;
  const ranges = ociEditComputedRanges();
  const displayName = document.getElementById('oci-edit-display-name')?.value?.trim() || '';
  const ocpus = Number(document.getElementById('oci-edit-ocpus')?.value || 0);
  const memoryInGBs = Number(document.getElementById('oci-edit-memory')?.value || 0);
  const allowDowntime = document.getElementById('oci-edit-allow-downtime')?.checked === true;

  if (!displayName) {
    msgEl.textContent = '实例名称不能为空。';
    msgEl.className = 'form-message error';
    return;
  }
  if (shape.isFlexible) {
    if (ocpus < ranges.ocpuMin || ocpus > ranges.ocpuMax) {
      msgEl.textContent = `OCPU 数应介于 ${formatOCIAmount(ranges.ocpuMin)} 和 ${formatOCIAmount(ranges.ocpuMax)} 之间。`;
      msgEl.className = 'form-message error';
      return;
    }
    if (memoryInGBs < ranges.memoryMin || memoryInGBs > ranges.memoryMax) {
      msgEl.textContent = `内存应介于 ${formatOCIAmount(ranges.memoryMin)} GB 和 ${formatOCIAmount(ranges.memoryMax)} GB 之间。`;
      msgEl.className = 'form-message error';
      return;
    }
  }
  if (allowDowntime && ociEditModalVM.status === 'VM running' && !confirm('调整 OCI 规格可能导致实例短暂重启，确认提交？')) {
    return;
  }

  msgEl.textContent = '正在提交 OCI 实例编辑请求...';
  msgEl.className = 'form-message';
  saveBtn.disabled = true;

  try {
    const vm = ociEditModalVM;
    const payload = {
      displayName,
      shape: shape.name,
      ocpus,
      memoryInGBs,
      baselineOcpuUtilization: ociEditOptions.instance?.baselineOcpuUtilization || '',
      allowDowntime
    };
    const data = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/edit`, {
      method: 'POST',
      body: payload
    });
    addLog(data.message || `OCI 实例 ${vm.name} 编辑请求已提交。`, 'success');
    closeOCIEditModal();
    setTimeout(() => refreshVM(vm), 5000);
  } catch (err) {
    msgEl.textContent = err.message;
    msgEl.className = 'form-message error';
  } finally {
    saveBtn.disabled = false;
  }
}

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('oci-edit-close')?.addEventListener('click', closeOCIEditModal);
  document.getElementById('oci-edit-cancel')?.addEventListener('click', closeOCIEditModal);
  document.getElementById('oci-edit-save')?.addEventListener('click', saveOCIEditModal);
  document.getElementById('oci-edit-modal')?.addEventListener('click', event => {
    if (event.target.id === 'oci-edit-modal') closeOCIEditModal();
  });
  document.getElementById('oci-edit-body')?.addEventListener('click', event => {
    const familyBtn = event.target.closest('.oci-family-card[data-family]');
    if (!familyBtn || familyBtn.disabled || !ociEditModalVM || !ociEditOptions) return;
    const family = familyBtn.dataset.family;
    const nextShape = (ociEditOptions.shapes || []).find(shape => shape.family === family);
    if (!nextShape || nextShape.name === ociEditOptions.selectedShape?.name) return;
    loadOCIEditOptions(ociEditModalVM, nextShape.name, currentOCIEditFormValues());
  });
  document.getElementById('oci-edit-body')?.addEventListener('change', event => {
    if (event.target.id === 'oci-edit-shape' && ociEditModalVM) {
      loadOCIEditOptions(ociEditModalVM, event.target.value, currentOCIEditFormValues());
    }
  });
  document.getElementById('oci-edit-body')?.addEventListener('input', event => {
    if (event.target.id === 'oci-edit-ocpus') {
      updateOCIEditRangeHints();
    }
  });
});

let securityListModalVM = null;
let securityListModalLists = [];
let securityListModalNSGs = [];
let securityListModalDirection = 'ingress';
let securityListModalResourceType = 'security-list';

const SECURITY_PROTOCOL_OPTIONS = [
  { value: 'all', label: '所有协议' },
  { value: '1', label: 'ICMP' },
  { value: '6', label: 'TCP' },
  { value: '17', label: 'UDP' },
  { value: '6', label: 'SSH (TCP/22)', minPort: 22, maxPort: 22 },
  { value: '6', label: 'HTTP (TCP/80)', minPort: 80, maxPort: 80 },
  { value: '6', label: 'HTTPS (TCP/443)', minPort: 443, maxPort: 443 },
  { value: '6', label: 'RDP (TCP/3389)', minPort: 3389, maxPort: 3389 },
  { value: '50', label: 'ESP (50)' },
  { value: '51', label: 'AH (51)' },
  { value: '58', label: 'IPv6 ICMP (58)' },
  { value: '47', label: 'GRE (47)' },
  { value: '132', label: 'SCTP (132)' },
  { value: '4', label: 'IPv4 (4)' },
  { value: '41', label: 'IPv6 (41)' }
];

const ICMP_TYPE_OPTIONS = [
  { value: '', label: '全部' },
  { value: '0', label: '0 - Echo Reply' },
  { value: '3', label: '3 - Destination Unreachable' },
  { value: '4', label: '4 - Source Quench' },
  { value: '5', label: '5 - Redirect' },
  { value: '8', label: '8 - Echo Request (Ping)' },
  { value: '9', label: '9 - Router Advertisement' },
  { value: '10', label: '10 - Router Solicitation' },
  { value: '11', label: '11 - Time Exceeded' },
  { value: '12', label: '12 - Parameter Problem' },
  { value: '13', label: '13 - Timestamp' },
  { value: '14', label: '14 - Timestamp Reply' }
];

const ICMP_CODE_OPTIONS = {
  '': [{ value: '', label: '全部' }],
  '0': [{ value: '', label: '全部' }, { value: '0', label: '0 - Echo Reply' }],
  '3': [
    { value: '', label: '全部' },
    { value: '0', label: '0 - Network Unreachable' },
    { value: '1', label: '1 - Host Unreachable' },
    { value: '2', label: '2 - Protocol Unreachable' },
    { value: '3', label: '3 - Port Unreachable' },
    { value: '4', label: '4 - Fragmentation Needed' },
    { value: '13', label: '13 - Communication Administratively Prohibited' }
  ],
  '5': [
    { value: '', label: '全部' },
    { value: '0', label: '0 - Redirect Datagram for Network' },
    { value: '1', label: '1 - Redirect Datagram for Host' }
  ],
  '8': [{ value: '', label: '全部' }, { value: '0', label: '0 - Echo Request' }],
  '11': [
    { value: '', label: '全部' },
    { value: '0', label: '0 - TTL Exceeded' },
    { value: '1', label: '1 - Fragment Reassembly Time Exceeded' }
  ],
  '12': [
    { value: '', label: '全部' },
    { value: '0', label: '0 - Pointer Indicates Error' },
    { value: '1', label: '1 - Missing Required Option' },
    { value: '2', label: '2 - Bad Length' }
  ]
};

function openSecurityListModal(vm) {
  securityListModalVM = vm;
  securityListModalResourceType = 'security-list';
  securityListModalDirection = 'ingress';
  const modal = document.getElementById('security-list-modal');
  document.getElementById('sg-modal-title').textContent = `OCI 安全规则 - ${vm.accountId}/${vm.name}`;
  document.getElementById('sg-modal-list').innerHTML = '';
  document.getElementById('sg-modal-body').innerHTML = '<div class="empty-state compact">加载中...</div>';
  document.getElementById('sg-modal-message').textContent = '';
  document.getElementById('sg-new-nsg-name').value = '';
  updateSecurityResourceTabs();
  updateSecurityRuleDirectionTabs();
  modal.hidden = false;
  loadSecurityLists(vm);
}

function closeSecurityListModal() {
  document.getElementById('security-list-modal').hidden = true;
  securityListModalVM = null;
  securityListModalLists = [];
  securityListModalNSGs = [];
}

async function loadSecurityLists(vm, selectedListId = '') {
  const listSelect = document.getElementById('sg-modal-list');
  const body = document.getElementById('sg-modal-body');
  const msgEl = document.getElementById('sg-modal-message');
  msgEl.textContent = '';
  document.getElementById('sg-modal-resource-label').textContent = securityListModalResourceType === 'network-security-group'
    ? '网络安全组'
    : '安全列表';
  document.getElementById('sg-nsg-create-row').hidden = securityListModalResourceType !== 'network-security-group';

  try {
    const path = securityListModalResourceType === 'network-security-group'
      ? `/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/network-security-groups`
      : `/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/security-lists`;
    const data = await fetchJSON(path);
    if (securityListModalResourceType === 'network-security-group') {
      securityListModalNSGs = Array.isArray(data.networkSecurityGroups) ? data.networkSecurityGroups : [];
      renderAvailableNSGOptions(Array.isArray(data.availableNetworkSecurityGroups) ? data.availableNetworkSecurityGroups : [], securityListModalNSGs.length > 0);
    } else {
      securityListModalLists = Array.isArray(data.securityLists) ? data.securityLists : [];
    }

    const resources = currentSecurityResources();
    if (resources.length === 0) {
      listSelect.innerHTML = securityListModalResourceType === 'network-security-group'
        ? '<option value="">未关联网络安全组</option>'
        : '<option value="">未关联安全列表</option>';
      body.innerHTML = securityListModalResourceType === 'network-security-group'
        ? '<div class="empty-state compact">主 VNIC 未关联网络安全组，可在上方选择已有网络安全组绑定或新建并关联。</div>'
        : '<div class="empty-state compact">主 VNIC 所在子网未关联 OCI 安全列表。</div>';
      return;
    }

    const nextSelected = selectedListId || resources[0].id;
    listSelect.innerHTML = resources.map(list => `
      <option value="${escapeAttr(list.id)}" ${list.id === nextSelected ? 'selected' : ''}>${escapeHtml(list.name || list.id)}</option>
    `).join('');
    renderSecurityListRules(nextSelected);
  } catch (err) {
    body.innerHTML = `<div class="empty-state compact error">${escapeHtml(err.message)}</div>`;
  }
}

function selectedSecurityList() {
  const selectedID = document.getElementById('sg-modal-list')?.value || '';
  return currentSecurityResources().find(list => list.id === selectedID) || null;
}

function currentSecurityResources() {
  return securityListModalResourceType === 'network-security-group'
    ? securityListModalNSGs
    : securityListModalLists;
}

function renderSecurityListRules(listId) {
  const body = document.getElementById('sg-modal-body');
  const list = currentSecurityResources().find(item => item.id === listId);
  if (!list) {
    body.innerHTML = '<div class="empty-state compact">请选择安全列表。</div>';
    return;
  }

  const rules = securityListModalDirection === 'egress'
    ? (Array.isArray(list.egressRules) ? list.egressRules : [])
    : (Array.isArray(list.ingressRules) ? list.ingressRules : []);
  if (rules.length === 0) {
    body.innerHTML = `<div class="empty-state compact">暂无${securityListModalDirection === 'egress' ? '出站' : '入站'}规则，点击下方“添加安全规则”创建。</div>`;
    return;
  }

  body.innerHTML = rules.map((rule, index) => securityListRuleRowHtml(rule, index)).join('');
  updateSecurityListProtocolControls();
}

function securityListRuleRowHtml(rule = {}, index = 0) {
  const protocol = normalizeSecurityListProtocol(rule.protocol || '6');
  const minPort = rule.minPort ?? '';
  const maxPort = rule.maxPort ?? '';
  const icmpType = rule.icmpType ?? '';
  const icmpCode = rule.icmpCode ?? '';
  const endpoint = securityListModalDirection === 'egress'
    ? (rule.destination || '0.0.0.0/0')
    : (rule.source || '0.0.0.0/0');
  const endpointType = securityListModalDirection === 'egress'
    ? (rule.destinationType || 'CIDR_BLOCK')
    : (rule.sourceType || 'CIDR_BLOCK');
  const description = rule.description || '';
  const rowLabel = `规则 ${index + 1}`;
  const endpointLabel = securityListModalDirection === 'egress' ? '目标 CIDR' : '来源 CIDR';
  const endpointTypeLabel = securityListModalDirection === 'egress' ? '目标类型' : '来源类型';
  const protocolOption = SECURITY_PROTOCOL_OPTIONS.find(option =>
    option.value === protocol && (option.minPort ?? '') === minPort && (option.maxPort ?? '') === maxPort
  );
  const protocolSelectValue = protocolOption ? securityProtocolOptionValue(protocolOption) : securityProtocolOptionValue({value: protocol});

  return `<div class="sg-rule-row" data-rule-id="${escapeAttr(rule.id || '')}">
    <div class="sg-rule-meta">
      <strong>${escapeHtml(rowLabel)}</strong>
      <span>${rule.id ? escapeHtml(rule.id) : (securityListModalDirection === 'egress' ? '出站' : '入站') + '安全规则'}</span>
    </div>
    <div class="sg-rule-fields">
      <label class="field compact">
        <span>协议</span>
        <select class="sg-f-protocol">${securityProtocolOptionsHtml(protocolSelectValue)}</select>
      </label>
      <label class="field compact">
        <span>${endpointLabel}</span>
        <input type="text" class="sg-f-endpoint" value="${escapeAttr(endpoint)}" placeholder="0.0.0.0/0">
      </label>
      <label class="field compact">
        <span>端口起</span>
        <input type="number" class="sg-f-min-port" min="1" max="65535" value="${escapeAttr(minPort)}" placeholder="全部">
      </label>
      <label class="field compact">
        <span>端口止</span>
        <input type="number" class="sg-f-max-port" min="1" max="65535" value="${escapeAttr(maxPort)}" placeholder="同起始">
      </label>
      <label class="field compact">
        <span>ICMP 类型</span>
        <select class="sg-f-icmp-type">${icmpTypeOptionsHtml(icmpType)}</select>
      </label>
      <label class="field compact">
        <span>ICMP 代码</span>
        <select class="sg-f-icmp-code">${icmpCodeOptionsHtml(icmpType, icmpCode)}</select>
      </label>
      <label class="field compact">
        <span>${endpointTypeLabel}</span>
        <select class="sg-f-endpoint-type">
          <option value="CIDR_BLOCK" ${endpointType === 'CIDR_BLOCK' ? 'selected' : ''}>CIDR</option>
          <option value="SERVICE_CIDR_BLOCK" ${endpointType === 'SERVICE_CIDR_BLOCK' ? 'selected' : ''}>Service CIDR</option>
          <option value="NETWORK_SECURITY_GROUP" ${endpointType === 'NETWORK_SECURITY_GROUP' ? 'selected' : ''}>Network Security Group</option>
        </select>
      </label>
      <label class="switch-row compact">
        <input type="checkbox" class="sg-f-stateless" ${rule.isStateless ? 'checked' : ''}>
        <span>无状态</span>
      </label>
      <label class="field compact sg-description-field">
        <span>描述</span>
        <input type="text" class="sg-f-description" value="${escapeAttr(description)}" placeholder="可选">
      </label>
    </div>
    <div class="sg-rule-footer">
      <div class="sg-allow-summary">
        <span>允许</span>
        <strong>${escapeHtml(securityRuleAllowText({ protocol, minPort, maxPort, icmpType, icmpCode }))}</strong>
      </div>
      <button class="ghost-btn dns-remove-btn sg-rule-delete-btn" type="button">删除</button>
    </div>
  </div>`;
}

function normalizeSecurityListProtocol(protocol) {
  const value = String(protocol || '').toLowerCase();
  if (value === 'tcp') return '6';
  if (value === 'udp') return '17';
  if (value === 'icmp') return '1';
  if (value === 'all') return value;
  const parsed = Number(value);
  if (Number.isInteger(parsed) && parsed >= 0 && parsed <= 255) return String(parsed);
  return 'all';
}

function securityProtocolOptionValue(option) {
  return [option.value, option.minPort ?? '', option.maxPort ?? ''].join('|');
}

function securityProtocolOptionsHtml(selected) {
  return SECURITY_PROTOCOL_OPTIONS.map(option => {
    const value = securityProtocolOptionValue(option);
    return `<option value="${escapeAttr(value)}" ${value === selected ? 'selected' : ''}>${escapeHtml(option.label)}</option>`;
  }).join('');
}

function parseSecurityProtocolSelection(value) {
  const [protocol, minPort, maxPort] = String(value || 'all').split('|');
  return {
    protocol: protocol || 'all',
    minPort: minPort === '' ? null : Number(minPort),
    maxPort: maxPort === '' ? null : Number(maxPort)
  };
}

function icmpTypeOptionsHtml(selected) {
  const selectedValue = selected === null || selected === undefined ? '' : String(selected);
  return ICMP_TYPE_OPTIONS.map(option => `
    <option value="${escapeAttr(option.value)}" ${option.value === selectedValue ? 'selected' : ''}>${escapeHtml(option.label)}</option>
  `).join('');
}

function icmpCodeOptionsHtml(type, selected) {
  const typeValue = type === null || type === undefined ? '' : String(type);
  const selectedValue = selected === null || selected === undefined ? '' : String(selected);
  const options = ICMP_CODE_OPTIONS[typeValue] || ICMP_CODE_OPTIONS[''];
  return options.map(option => `
    <option value="${escapeAttr(option.value)}" ${option.value === selectedValue ? 'selected' : ''}>${escapeHtml(option.label)}</option>
  `).join('');
}

function protocolDisplayName(protocol) {
  if (protocol === 'all') return '所有协议';
  if (protocol === '1') return 'ICMP';
  if (protocol === '6') return 'TCP';
  if (protocol === '17') return 'UDP';
  return `协议 ${protocol}`;
}

function securityRuleAllowText(rule) {
  const protocol = normalizeSecurityListProtocol(rule.protocol);
  if (protocol === 'all') return '所有端口的所有流量';
  if (protocol === '6' || protocol === '17') {
    const minPort = rule.minPort ?? '';
    const maxPort = rule.maxPort ?? '';
    if (minPort === '' && maxPort === '') return `以下端口的 ${protocolDisplayName(protocol)} 流量：全部`;
    const endPort = maxPort === '' ? minPort : maxPort;
    return `以下端口的 ${protocolDisplayName(protocol)} 流量：${minPort}-${endPort}`;
  }
  if (protocol === '1') {
    const type = rule.icmpType ?? '';
    const code = rule.icmpCode ?? '';
    if (type === '' && code === '') return '以下项的 ICMP 流量：全部';
    if (code === '') return `ICMP 类型 ${type}：全部代码`;
    return `ICMP 类型 ${type}，代码 ${code}`;
  }
  return `${protocolDisplayName(protocol)} 流量`;
}

function addSecurityListRuleRow() {
  const list = selectedSecurityList();
  if (!list) return;

  const body = document.getElementById('sg-modal-body');
  if (body.querySelector('.empty-state')) body.innerHTML = '';
  const index = body.querySelectorAll('.sg-rule-row').length;
  body.insertAdjacentHTML('beforeend', securityListRuleRowHtml({
    protocol: '6',
    source: '0.0.0.0/0',
    destination: '0.0.0.0/0',
    sourceType: 'CIDR_BLOCK',
    destinationType: 'CIDR_BLOCK',
    minPort: 22,
    maxPort: 22,
    isStateless: false
  }, index));
  updateSecurityListProtocolControls();
}

function collectSecurityListRules() {
  const rows = document.getElementById('sg-modal-body').querySelectorAll('.sg-rule-row');
  return Array.from(rows).map(row => {
    const selectedProtocol = parseSecurityProtocolSelection(row.querySelector('.sg-f-protocol')?.value || 'all');
    const protocol = selectedProtocol.protocol;
    const minPort = securityRuleNumberValue(row.querySelector('.sg-f-min-port')?.value);
    const maxPort = securityRuleNumberValue(row.querySelector('.sg-f-max-port')?.value);
    const icmpType = securityRuleNumberValue(row.querySelector('.sg-f-icmp-type')?.value);
    const icmpCode = securityRuleNumberValue(row.querySelector('.sg-f-icmp-code')?.value);
    const endpoint = row.querySelector('.sg-f-endpoint')?.value?.trim() || '';
    const endpointType = row.querySelector('.sg-f-endpoint-type')?.value || 'CIDR_BLOCK';
    const rule = {
      id: row.dataset.ruleId || '',
      protocol,
      minPort: protocol === '6' || protocol === '17' ? minPort : null,
      maxPort: protocol === '6' || protocol === '17' ? maxPort : null,
      icmpType: protocol === '1' ? icmpType : null,
      icmpCode: protocol === '1' ? icmpCode : null,
      description: row.querySelector('.sg-f-description')?.value?.trim() || '',
      isStateless: row.querySelector('.sg-f-stateless')?.checked || false
    };
    if (securityListModalDirection === 'egress') {
      rule.destination = endpoint;
      rule.destinationType = endpointType;
    } else {
      rule.source = endpoint;
      rule.sourceType = endpointType;
    }
    return rule;
  });
}

function securityRuleNumberValue(value) {
  const trimmed = String(value || '').trim();
  if (trimmed === '') return null;
  const parsed = Number(trimmed);
  return Number.isFinite(parsed) ? parsed : null;
}

async function saveSecurityListRules() {
  const vm = securityListModalVM;
  const list = selectedSecurityList();
  const msgEl = document.getElementById('sg-modal-message');
  if (!vm || !list) return;

  msgEl.textContent = '保存中...';
  msgEl.className = 'form-message';

  try {
    const currentRules = collectSecurityListRules();
    const path = securityListModalResourceType === 'network-security-group'
      ? `/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/network-security-groups/${encodeURIComponent(list.id)}/rules`
      : `/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/security-lists/${encodeURIComponent(list.id)}/rules`;
    await fetchJSON(path, {
      method: 'POST',
      body: {
        ingressRules: securityListModalDirection === 'ingress' ? currentRules : (list.ingressRules || []),
        egressRules: securityListModalDirection === 'egress' ? currentRules : (list.egressRules || [])
      }
    });
    msgEl.textContent = '安全规则已保存。';
    msgEl.className = 'form-message success';
    addLog(`OCI 安全规则已保存：${list.name || list.id}`, 'success');
    loadSecurityLists(vm, list.id);
  } catch (err) {
    msgEl.textContent = err.message;
    msgEl.className = 'form-message error';
  }
}

function updateSecurityListProtocolControls() {
  document.querySelectorAll('.sg-rule-row').forEach(row => {
    const protocol = parseSecurityProtocolSelection(row.querySelector('.sg-f-protocol')?.value || 'all').protocol;
    const portDisabled = protocol !== '6' && protocol !== '17';
    const icmpDisabled = protocol !== '1';
    row.querySelectorAll('.sg-f-min-port, .sg-f-max-port').forEach(input => {
      input.disabled = portDisabled;
    });
    row.querySelectorAll('.sg-f-icmp-type, .sg-f-icmp-code').forEach(input => {
      input.disabled = icmpDisabled;
    });
  });
}

function applySecurityProtocolPreset(row) {
  if (!row) return;
  const selected = parseSecurityProtocolSelection(row.querySelector('.sg-f-protocol')?.value || 'all');
  const minPort = row.querySelector('.sg-f-min-port');
  const maxPort = row.querySelector('.sg-f-max-port');
  if ((selected.protocol === '6' || selected.protocol === '17') && selected.minPort !== null) {
    if (minPort) minPort.value = selected.minPort;
    if (maxPort) maxPort.value = selected.maxPort ?? selected.minPort;
  }
  if (selected.protocol !== '1') {
    const icmpType = row.querySelector('.sg-f-icmp-type');
    const icmpCode = row.querySelector('.sg-f-icmp-code');
    if (icmpType) icmpType.value = '';
    if (icmpCode) icmpCode.innerHTML = icmpCodeOptionsHtml('', '');
  }
}

function updateSecurityRuleDirectionTabs() {
  document.querySelectorAll('.sg-direction-tab').forEach(tab => {
    tab.classList.toggle('active', tab.dataset.direction === securityListModalDirection);
  });
}

function updateSecurityResourceTabs() {
  document.querySelectorAll('.sg-resource-tab').forEach(tab => {
    tab.classList.toggle('active', tab.dataset.resourceType === securityListModalResourceType);
  });
}

function renderAvailableNSGOptions(available, hasAttached) {
  const select = document.getElementById('sg-existing-nsg-select');
  const attachBtn = document.getElementById('sg-attach-nsg');
  const detachBtn = document.getElementById('sg-detach-nsg');
  if (detachBtn) detachBtn.disabled = !hasAttached;
  if (!select) return;

  const unattached = available.filter(item => !item.attached);
  if (unattached.length === 0) {
    select.innerHTML = '<option value="">同 VCN 下暂无其他可绑定的网络安全组</option>';
    if (attachBtn) attachBtn.disabled = true;
    return;
  }
  select.innerHTML = unattached.map(item => `
    <option value="${escapeAttr(item.id)}">${escapeHtml(item.name || item.id)}</option>
  `).join('');
  if (attachBtn) attachBtn.disabled = false;
}

async function attachNetworkSecurityGroupForVM() {
  const vm = securityListModalVM;
  const msgEl = document.getElementById('sg-modal-message');
  const groupId = document.getElementById('sg-existing-nsg-select')?.value || '';
  if (!vm || !groupId) return;

  msgEl.textContent = '正在绑定网络安全组...';
  msgEl.className = 'form-message';

  try {
    await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/network-security-groups/attach`, {
      method: 'POST',
      body: { groupId }
    });
    msgEl.textContent = '网络安全组已绑定到实例。';
    msgEl.className = 'form-message success';
    addLog(`OCI 网络安全组已绑定：${groupId}`, 'success');
    loadSecurityLists(vm, groupId);
  } catch (err) {
    msgEl.textContent = err.message;
    msgEl.className = 'form-message error';
  }
}

async function detachNetworkSecurityGroupForVM() {
  const vm = securityListModalVM;
  const msgEl = document.getElementById('sg-modal-message');
  const selected = selectedSecurityList();
  if (!vm || !selected) return;
  if (!confirm(`确认从当前实例解绑网络安全组「${selected.name || selected.id}」？\n（仅解除关联，不会删除该网络安全组）`)) return;

  msgEl.textContent = '正在解绑网络安全组...';
  msgEl.className = 'form-message';

  try {
    await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/network-security-groups/${encodeURIComponent(selected.id)}/detach`, {
      method: 'POST'
    });
    msgEl.textContent = '已从当前实例解绑网络安全组。';
    msgEl.className = 'form-message success';
    addLog(`OCI 网络安全组已解绑：${selected.name || selected.id}`, 'success');
    loadSecurityLists(vm);
  } catch (err) {
    msgEl.textContent = err.message;
    msgEl.className = 'form-message error';
  }
}

async function createNetworkSecurityGroupForVM() {
  const vm = securityListModalVM;
  const msgEl = document.getElementById('sg-modal-message');
  if (!vm) return;

  const name = document.getElementById('sg-new-nsg-name')?.value?.trim() || '';
  msgEl.textContent = '创建网络安全组中...';
  msgEl.className = 'form-message';

  try {
    const data = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/network-security-groups`, {
      method: 'POST',
      body: { name }
    });
    const created = data.networkSecurityGroup || {};
    msgEl.textContent = '网络安全组已创建并关联。';
    msgEl.className = 'form-message success';
    addLog(`OCI 网络安全组已创建并关联：${created.name || created.id || name}`, 'success');
    document.getElementById('sg-new-nsg-name').value = '';
    loadSecurityLists(vm, created.id || '');
  } catch (err) {
    msgEl.textContent = err.message;
    msgEl.className = 'form-message error';
  }
}

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('sg-modal-close')?.addEventListener('click', closeSecurityListModal);
  document.getElementById('sg-modal-add')?.addEventListener('click', addSecurityListRuleRow);
  document.getElementById('sg-modal-save')?.addEventListener('click', saveSecurityListRules);
  document.getElementById('sg-create-nsg')?.addEventListener('click', createNetworkSecurityGroupForVM);
  document.getElementById('sg-attach-nsg')?.addEventListener('click', attachNetworkSecurityGroupForVM);
  document.getElementById('sg-detach-nsg')?.addEventListener('click', detachNetworkSecurityGroupForVM);
  document.getElementById('sg-modal-list')?.addEventListener('change', event => {
    renderSecurityListRules(event.target.value);
  });
  document.querySelectorAll('.sg-resource-tab').forEach(tab => {
    tab.addEventListener('click', () => {
      securityListModalResourceType = tab.dataset.resourceType || 'security-list';
      updateSecurityResourceTabs();
      if (securityListModalVM) loadSecurityLists(securityListModalVM);
    });
  });
  document.querySelectorAll('.sg-direction-tab').forEach(tab => {
    tab.addEventListener('click', () => {
      securityListModalDirection = tab.dataset.direction || 'ingress';
      updateSecurityRuleDirectionTabs();
      const list = selectedSecurityList();
      renderSecurityListRules(list ? list.id : '');
    });
  });
  document.getElementById('security-list-modal')?.addEventListener('click', event => {
    if (event.target.id === 'security-list-modal') closeSecurityListModal();
  });
  document.getElementById('sg-modal-body')?.addEventListener('click', event => {
    const btn = event.target.closest('.sg-rule-delete-btn');
    if (!btn) return;
    btn.closest('.sg-rule-row')?.remove();
    if (!document.getElementById('sg-modal-body').querySelector('.sg-rule-row')) {
      document.getElementById('sg-modal-body').innerHTML = `<div class="empty-state compact">暂无${securityListModalDirection === 'egress' ? '出站' : '入站'}规则</div>`;
    }
  });
  document.getElementById('sg-modal-body')?.addEventListener('change', event => {
    const protocolSelect = event.target.closest('.sg-f-protocol');
    if (protocolSelect) {
      applySecurityProtocolPreset(protocolSelect.closest('.sg-rule-row'));
      updateSecurityListProtocolControls();
    }
    const icmpTypeSelect = event.target.closest('.sg-f-icmp-type');
    if (icmpTypeSelect) {
      const row = icmpTypeSelect.closest('.sg-rule-row');
      const codeSelect = row?.querySelector('.sg-f-icmp-code');
      if (codeSelect) codeSelect.innerHTML = icmpCodeOptionsHtml(icmpTypeSelect.value, '');
    }
  });
});

// ==========================================
// Overview (Backend Cached Stats) Functions
// ==========================================

async function fetchOverview() {
  try {
    const data = await fetchJSON('/api/overview');
    if (!data) return;

    if (els.configCount) {
      els.configCount.textContent = data.configCount ?? 0;
    }
    if (els.configMeta) {
      const providers = data.providerCounts || {};
      const parts = Object.entries(providers).map(([p, count]) => `${p.toUpperCase()}: ${count}`);
      els.configMeta.textContent = parts.length > 0 ? parts.join(' / ') : '暂无配置';
    }

    if (els.vmCount) {
      els.vmCount.textContent = data.instanceCount ?? 0;
    }
    if (els.instanceMeta) {
      const cached = data.cachedAccountCount ?? 0;
      const total = data.configCount ?? 0;
      const timeStr = data.lastUpdatedAt ? `，更新于 ${data.lastUpdatedAt.slice(11)}` : '';
      els.instanceMeta.textContent = `已缓存 ${cached}/${total} 个账号${timeStr}`;
    }

    if (els.runningCount) {
      els.runningCount.textContent = data.runningCount ?? 0;
    }
    if (els.stoppedCount) {
      els.stoppedCount.textContent = data.stoppedCount ?? 0;
    }
  } catch (error) {
    console.error('获取概览统计失败:', error);
  }
}

// ==========================================
// Multi-Cloud Firewall Management Functions
// ==========================================

function renderFirewallAccounts(accounts) {
  firewallAccounts = (accounts || []).filter(a => ['gcp', 'oci', 'azure'].includes((a.provider || '').toLowerCase()));
  if (!els.fwAccountPills || !els.fwAccountSelect) return;

  if (firewallAccounts.length === 0) {
    els.fwAccountPills.innerHTML = '<span class="empty-state compact" style="margin: 0;">暂无可管理防火墙的云账号</span>';
    els.fwAccountSelect.innerHTML = '<option value="">无可用账号</option>';
    if (els.fwContentContainer) {
      els.fwContentContainer.innerHTML = '<div class="empty-state compact">未检测到已配置的 GCP / OCI / Azure 账号。可在配置文件中添加账号。</div>';
    }
    return;
  }

  // Render pills
  els.fwAccountPills.innerHTML = firewallAccounts.map(a => {
    const prov = (a.provider || '').toLowerCase();
    const isSelected = selectedFirewallAccount &&
      selectedFirewallAccount.provider.toLowerCase() === prov &&
      selectedFirewallAccount.account === a.account;
    return `
      <div class="fw-pill ${isSelected ? 'active' : ''}" data-provider="${escapeAttr(prov)}" data-account="${escapeAttr(a.account)}">
        <span class="prov-badge ${escapeAttr(prov)}">${escapeHtml(prov.toUpperCase())}</span>
        <span>${escapeHtml(a.account)}</span>
      </div>
    `;
  }).join('');

  // Render select options with placeholder
  els.fwAccountSelect.innerHTML = '<option value="">-- 请选择要加载的云账号 --</option>' + firewallAccounts.map(a => {
    const prov = (a.provider || '').toLowerCase();
    const val = `${prov}:${a.account}`;
    return `<option value="${escapeAttr(val)}">[${prov.toUpperCase()}] ${escapeHtml(a.account)}${a.group ? ` (${escapeHtml(a.group)})` : ''}</option>`;
  }).join('');

  // If previously selected account is no longer valid, clear selection
  if (selectedFirewallAccount && !firewallAccounts.some(a => a.provider.toLowerCase() === selectedFirewallAccount.provider.toLowerCase() && a.account === selectedFirewallAccount.account)) {
    selectedFirewallAccount = null;
  }

  syncFirewallAccountUI();
}

function syncFirewallAccountUI() {
  if (!selectedFirewallAccount) {
    if (els.fwAccountSelect) els.fwAccountSelect.value = '';
    if (els.fwAccountPills) {
      els.fwAccountPills.querySelectorAll('.fw-pill').forEach(pill => pill.classList.remove('active'));
    }
    document.querySelectorAll('.sub-nav-firewall-item').forEach(el => el.classList.remove('active'));
    if (els.fwAccountMeta) els.fwAccountMeta.innerHTML = '请选择要管理的云账号';
    if (els.fwCreateBtn) els.fwCreateBtn.style.display = 'none';
    if (els.fwContentContainer && !els.fwContentContainer.querySelector('table')) {
      els.fwContentContainer.innerHTML = `
        <div class="empty-state compact">
          <i class="bi bi-shield-lock" style="font-size: 32px; display: block; margin-bottom: 8px; color: var(--muted);"></i>
          未加载任何防火墙配置，请在左侧侧边栏展开列表或上方点击具体云账号按需加载
        </div>
      `;
    }
    return;
  }

  const prov = selectedFirewallAccount.provider.toLowerCase();
  const acc = selectedFirewallAccount.account;
  const val = `${prov}:${acc}`;

  if (els.fwAccountSelect) {
    els.fwAccountSelect.value = val;
  }

  if (els.fwAccountPills) {
    els.fwAccountPills.querySelectorAll('.fw-pill').forEach(pill => {
      const match = pill.dataset.provider === prov && pill.dataset.account === acc;
      pill.classList.toggle('active', match);
    });
  }

  document.querySelectorAll('.sub-nav-firewall-item').forEach(el => {
    el.classList.toggle('active', el.dataset.provider === prov && el.dataset.account === acc);
  });

  const provNames = {
    gcp: 'Google Cloud (VPC 全局防火墙)',
    oci: 'Oracle Cloud (安全列表 Security List)',
    azure: 'Microsoft Azure (网络安全组 NSG)'
  };
  if (els.fwAccountMeta) {
    els.fwAccountMeta.innerHTML = `当前厂商：<strong>${escapeHtml(provNames[prov] || prov.toUpperCase())}</strong> &nbsp;|&nbsp; 账号：<code>${escapeHtml(acc)}</code>`;
  }

  if (els.fwCreateBtn) {
    if (prov === 'gcp') {
      els.fwCreateBtn.style.display = 'inline-flex';
      if (els.fwCreateBtnLabel) els.fwCreateBtnLabel.textContent = '新建 VPC 规则';
    } else {
      els.fwCreateBtn.style.display = 'none';
    }
  }
}

function handleFirewallAccountSelect(val) {
  if (!val) return;
  const [provider, ...rest] = val.split(':');
  const account = rest.join(':');
  selectFirewallAccount(provider, account);
}

function handleFirewallPillClick(pill) {
  const provider = pill.dataset.provider;
  const account = pill.dataset.account;
  selectFirewallAccount(provider, account);
}

function selectFirewallAccount(provider, account) {
  const match = firewallAccounts.find(a => a.provider.toLowerCase() === provider.toLowerCase() && a.account === account);
  if (match) {
    selectedFirewallAccount = {
      provider: match.provider.toLowerCase(),
      account: match.account,
      group: match.group
    };
    syncFirewallAccountUI();
    fetchCurrentFirewalls();
  }
}

function initFirewallSection() {
  syncFirewallAccountUI();
  if (selectedFirewallAccount) {
    fetchCurrentFirewalls();
  }
}

function switchToFirewall(provider, accountId) {
  document.querySelectorAll('.nav-item').forEach(i => {
    i.classList.toggle('active', i.dataset.section === 'firewall');
  });
  document.querySelectorAll('.section').forEach(s => s.classList.remove('active'));
  document.getElementById('firewall-section')?.classList.add('active');

  if (provider && accountId) {
    selectFirewallAccount(provider, accountId);
  } else {
    initFirewallSection();
  }
}

function handleFirewallCreateClick() {
  if (!selectedFirewallAccount) return;
  if (selectedFirewallAccount.provider === 'gcp') {
    openGCPFirewallModal(null);
  }
}

async function fetchCurrentFirewalls() {
  if (!selectedFirewallAccount) {
    if (els.fwContentContainer) {
      els.fwContentContainer.innerHTML = '<div class="empty-state compact">请先选择云账号。</div>';
    }
    return;
  }
  const prov = selectedFirewallAccount.provider.toLowerCase();
  const acc = selectedFirewallAccount.account;

  if (prov === 'gcp') {
    await fetchGCPFirewalls(acc);
  } else if (prov === 'oci') {
    await fetchOciFirewallResources(acc);
  } else if (prov === 'azure') {
    await fetchAzureNSGs(acc);
  }
}

// ------------------------------------------
// Table Action Delegation
// ------------------------------------------
function handleFirewallTableAction(event) {
  // GCP actions
  const gcpEdit = event.target.closest('.gcp-fw-edit-btn');
  if (gcpEdit) {
    const name = gcpEdit.dataset.name;
    const rule = currentGCPFirewalls.find(r => r.name === name);
    if (rule) openGCPFirewallModal(rule);
    return;
  }
  const gcpToggle = event.target.closest('.gcp-fw-toggle-btn');
  if (gcpToggle) {
    const name = gcpToggle.dataset.name;
    const disabled = gcpToggle.dataset.disabled === 'true';
    toggleGCPFirewall(name, disabled);
    return;
  }
  const gcpDel = event.target.closest('.gcp-fw-delete-btn');
  if (gcpDel) {
    const name = gcpDel.dataset.name;
    deleteGCPFirewall(name);
    return;
  }

  // OCI SL & NSG actions
  const ociSubTab = event.target.closest('.oci-fw-subtab');
  if (ociSubTab) {
    const tab = ociSubTab.dataset.tab;
    if (tab && tab !== ociFirewallTab) {
      ociFirewallTab = tab;
      fetchCurrentFirewalls();
    }
    return;
  }
  const ociManage = event.target.closest('.oci-sl-manage-btn');
  if (ociManage) {
    const listId = ociManage.dataset.id;
    const list = currentOciSecurityLists.find(l => l.id === listId);
    if (list) openOciAccountSLModal(list, 'security-list');
    return;
  }
  const ociNsgManage = event.target.closest('.oci-nsg-manage-btn');
  if (ociNsgManage) {
    const groupId = ociNsgManage.dataset.id;
    const group = currentOciAccountNSGs.find(g => g.id === groupId);
    if (group) openOciAccountSLModal(group, 'nsg');
    return;
  }
  const ociNsgRename = event.target.closest('.oci-nsg-rename-btn');
  if (ociNsgRename) {
    const groupId = ociNsgRename.dataset.id;
    const group = currentOciAccountNSGs.find(g => g.id === groupId);
    if (group) renameOciAccountNSG(group);
    return;
  }
  const ociNsgDelete = event.target.closest('.oci-nsg-delete-btn');
  if (ociNsgDelete) {
    const groupId = ociNsgDelete.dataset.id;
    const group = currentOciAccountNSGs.find(g => g.id === groupId);
    if (group) deleteOciAccountNSG(group);
    return;
  }

  const ociRtManage = event.target.closest('.oci-rt-manage-btn');
  if (ociRtManage) {
    const rtId = ociRtManage.dataset.id;
    const rt = currentOciRouteTables.find(t => t.id === rtId);
    if (rt) openOciRouteTableModal(rt);
    return;
  }

  // Azure NSG actions
  const azManage = event.target.closest('.azure-nsg-manage-btn');
  if (azManage) {
    const name = azManage.dataset.name;
    const nsg = currentAzureNSGs.find(n => n.name === name);
    if (nsg) openAzureNSGModal(nsg);
    return;
  }
}

// ------------------------------------------
// GCP Firewall Methods
// ------------------------------------------
async function fetchGCPFirewalls(account) {
  if (!account && selectedFirewallAccount) account = selectedFirewallAccount.account;
  if (!account) return;

  if (els.fwContentContainer) {
    els.fwContentContainer.innerHTML = '<div class="empty-state compact">正在加载 GCP 防火墙规则...</div>';
  }

  try {
    const data = await fetchJSON(`/api/gcp/${encodeURIComponent(account)}/firewalls`);
    currentGCPFirewalls = Array.isArray(data?.firewalls) ? data.firewalls : [];
    renderGCPFirewallsTable(currentGCPFirewalls);
    addLog(`已加载 GCP 账号 ${account} 的防火墙规则，共 ${currentGCPFirewalls.length} 条。`, 'info');
  } catch (error) {
    if (els.fwContentContainer) {
      els.fwContentContainer.innerHTML = `<div class="empty-state compact error">加载规则失败：${escapeHtml(error.message)}</div>`;
    }
    addLog(`加载 GCP 防火墙规则失败：${error.message}`, 'error');
  }
}

function renderGCPFirewallsTable(rules) {
  if (!els.fwContentContainer) return;
  if (!rules || rules.length === 0) {
    els.fwContentContainer.innerHTML = '<div class="empty-state compact">当前 VPC 下暂无防火墙规则。点击右上角「新建 VPC 规则」创建。</div>';
    return;
  }

  const sorted = [...rules].sort((a, b) => (a.priority || 1000) - (b.priority || 1000) || (a.name || '').localeCompare(b.name || ''));

  els.fwContentContainer.innerHTML = `
    <table class="gcp-fw-table">
      <thead>
        <tr>
          <th>规则名称</th>
          <th>优先级</th>
          <th>方向</th>
          <th>动作</th>
          <th>协议与端口</th>
          <th>IP 网段</th>
          <th>目标标记 (Tags)</th>
          <th>状态</th>
          <th style="text-align: right;">操作</th>
        </tr>
      </thead>
      <tbody>
        ${sorted.map(renderGCPFirewallRow).join('')}
      </tbody>
    </table>
  `;
}

function renderGCPFirewallRow(rule) {
  const isAllow = (rule.action || 'ALLOW').toUpperCase() === 'ALLOW';
  const isIngress = (rule.direction || 'INGRESS').toUpperCase() === 'INGRESS';
  const isEnabled = !rule.disabled;

  let protoPortsHtml = '';
  if (rule.protocols && rule.protocols.length > 0) {
    if (rule.protocols.some(p => (p.ipProtocol || '').toLowerCase() === 'all')) {
      protoPortsHtml = '<span class="fw-proto-chip all">全部协议 (All)</span>';
    } else {
      protoPortsHtml = rule.protocols.map(p => {
        const pName = (p.ipProtocol || '').toUpperCase();
        const pPorts = p.ports && p.ports.length ? `:${p.ports.join(', ')}` : (pName === 'ICMP' ? '' : ' (全部)');
        const protoKey = (p.ipProtocol || '').toLowerCase();
        const cls = ['tcp', 'udp', 'icmp'].includes(protoKey) ? protoKey : 'other';
        return `<span class="fw-proto-chip ${cls}">${escapeHtml(pName + pPorts)}</span>`;
      }).join(' ');
    }
  } else {
    const proto = rule.ipProtocol || 'tcp';
    if (proto.toLowerCase() === 'all') {
      protoPortsHtml = '<span class="fw-proto-chip all">全部协议 (All)</span>';
    } else {
      const portsStr = rule.ports && rule.ports.length ? `:${rule.ports.join(', ')}` : (proto.toLowerCase() === 'icmp' ? '' : ' (全部)');
      const protoKey = proto.toLowerCase();
      const cls = ['tcp', 'udp', 'icmp'].includes(protoKey) ? protoKey : 'other';
      protoPortsHtml = `<span class="fw-proto-chip ${cls}">${escapeHtml(proto.toUpperCase() + portsStr)}</span>`;
    }
  }

  const ipRanges = isIngress 
    ? (rule.sourceRanges?.length ? rule.sourceRanges.join(', ') : '0.0.0.0/0')
    : (rule.destinationRanges?.length ? rule.destinationRanges.join(', ') : '0.0.0.0/0');

  const tagsHtml = rule.targetTags && rule.targetTags.length
    ? rule.targetTags.map(t => `<span class="fw-tag-pill">${escapeHtml(t)}</span>`).join('')
    : '<span style="color: var(--muted); font-size: 11px;">全部实例</span>';

  return `
    <tr data-name="${escapeAttr(rule.name)}">
      <td>
        <strong style="color: var(--text);">${escapeHtml(rule.name)}</strong>
        ${rule.description ? `<div style="color: var(--muted); font-size: 11px; margin-top: 2px;">${escapeHtml(rule.description)}</div>` : ''}
      </td>
      <td>${rule.priority ?? 1000}</td>
      <td>
        <span class="fw-badge ${isIngress ? 'fw-badge-ingress' : 'fw-badge-egress'}">
          ${isIngress ? '入站' : '出站'}
        </span>
      </td>
      <td>
        <span class="fw-badge ${isAllow ? 'fw-badge-allow' : 'fw-badge-deny'}">
          ${isAllow ? '允许' : '拒绝'}
        </span>
      </td>
      <td>${protoPortsHtml}</td>
      <td><span style="font-family: monospace; font-size: 12px;">${escapeHtml(ipRanges)}</span></td>
      <td>${tagsHtml}</td>
      <td>
        <span class="fw-status-badge ${isEnabled ? 'enabled' : 'disabled'}">
          <i class="bi ${isEnabled ? 'bi-check-circle-fill' : 'bi-dash-circle'}"></i>
          <span>${isEnabled ? '已启用' : '已禁用'}</span>
        </span>
      </td>
      <td style="text-align: right;">
        <div class="fw-actions-cell" style="justify-content: flex-end;">
          <button class="ghost-btn gcp-fw-toggle-btn" type="button" data-name="${escapeAttr(rule.name)}" data-disabled="${rule.disabled ? 'true' : 'false'}" title="${isEnabled ? '禁用规则' : '启用规则'}">
            <i class="bi ${isEnabled ? 'bi-pause-circle' : 'bi-play-circle'}"></i>
            <span>${isEnabled ? '禁用' : '启用'}</span>
          </button>
          <button class="ghost-btn gcp-fw-edit-btn" type="button" data-name="${escapeAttr(rule.name)}" title="编辑规则">
            <i class="bi bi-pencil"></i>
            <span>编辑</span>
          </button>
          <button class="ghost-btn gcp-fw-delete-btn" type="button" data-name="${escapeAttr(rule.name)}" style="color: var(--red);" title="删除规则">
            <i class="bi bi-trash"></i>
          </button>
        </div>
      </td>
    </tr>
  `;
}

function openGCPFirewallModal(rule = null) {
  if (!els.gcpFwModal) return;
  els.gcpFwModal.hidden = false;
  if (els.gcpFwModalMsg) {
    els.gcpFwModalMsg.textContent = '';
    els.gcpFwModalMsg.className = 'form-message';
  }

  const setProtoCheck = (proto, checked, ports = '', extraName = '') => {
    const chk = document.getElementById(`gcp-proto-check-${proto}`);
    const item = document.getElementById(`gcp-proto-item-${proto}`);
    const body = document.getElementById(`gcp-proto-body-${proto}`);
    if (chk) chk.checked = checked;
    if (item) item.classList.toggle('active', checked);
    if (body) body.style.display = checked ? '' : 'none';
    if (proto === 'tcp' && els.gcpProtoTcpPorts) els.gcpProtoTcpPorts.value = ports;
    if (proto === 'udp' && els.gcpProtoUdpPorts) els.gcpProtoUdpPorts.value = ports;
    if (proto === 'other') {
      if (els.gcpProtoOtherName) els.gcpProtoOtherName.value = extraName;
      if (els.gcpProtoOtherPorts) els.gcpProtoOtherPorts.value = ports;
    }
  };

  if (rule) {
    els.gcpFwEditMode.value = 'edit';
    els.gcpFwModalTitle.textContent = `编辑 GCP 防火墙规则 - ${rule.name}`;
    els.gcpFwName.value = rule.name;
    els.gcpFwName.disabled = true;
    els.gcpFwPriority.value = rule.priority ?? 1000;
    els.gcpFwDirection.value = (rule.direction || 'INGRESS').toUpperCase();
    els.gcpFwAction.value = (rule.action || 'ALLOW').toUpperCase();

    // Protocol handling
    const protocols = (rule.protocols && rule.protocols.length > 0)
      ? rule.protocols
      : (rule.ipProtocol ? [{ ipProtocol: rule.ipProtocol, ports: rule.ports }] : []);

    const isAll = protocols.some(p => (p.ipProtocol || '').toLowerCase() === 'all');
    if (isAll) {
      if (els.gcpProtoModeAll) els.gcpProtoModeAll.checked = true;
      setProtoCheck('tcp', false);
      setProtoCheck('udp', false);
      setProtoCheck('icmp', false);
      setProtoCheck('other', false);
    } else {
      if (els.gcpProtoModeCustom) els.gcpProtoModeCustom.checked = true;
      setProtoCheck('tcp', false);
      setProtoCheck('udp', false);
      setProtoCheck('icmp', false);
      setProtoCheck('other', false);

      let anyChecked = false;
      protocols.forEach(p => {
        const protoLower = (p.ipProtocol || '').toLowerCase();
        const pPortsStr = p.ports && p.ports.length ? p.ports.join(', ') : '';
        if (protoLower === 'tcp') {
          setProtoCheck('tcp', true, pPortsStr);
          anyChecked = true;
        } else if (protoLower === 'udp') {
          setProtoCheck('udp', true, pPortsStr);
          anyChecked = true;
        } else if (protoLower === 'icmp') {
          setProtoCheck('icmp', true);
          anyChecked = true;
        } else if (protoLower && protoLower !== 'all') {
          setProtoCheck('other', true, pPortsStr, protoLower);
          anyChecked = true;
        }
      });
      if (!anyChecked) {
        setProtoCheck('tcp', true);
      }
    }

    const isIngress = (rule.direction || 'INGRESS').toUpperCase() === 'INGRESS';
    els.gcpFwIpRanges.value = isIngress 
      ? (rule.sourceRanges?.length ? rule.sourceRanges.join(', ') : '0.0.0.0/0')
      : (rule.destinationRanges?.length ? rule.destinationRanges.join(', ') : '0.0.0.0/0');
    els.gcpFwTargetTags.value = rule.targetTags && rule.targetTags.length ? rule.targetTags.join(', ') : '';
    els.gcpFwDescription.value = rule.description || '';
    els.gcpFwDisabled.checked = Boolean(rule.disabled);
  } else {
    els.gcpFwEditMode.value = 'create';
    els.gcpFwModalTitle.textContent = '新建 GCP 防火墙规则';
    els.gcpFwName.value = '';
    els.gcpFwName.disabled = false;
    els.gcpFwPriority.value = 1000;
    els.gcpFwDirection.value = 'INGRESS';
    els.gcpFwAction.value = 'ALLOW';

    if (els.gcpProtoModeCustom) els.gcpProtoModeCustom.checked = true;
    setProtoCheck('tcp', true, '');
    setProtoCheck('udp', false, '');
    setProtoCheck('icmp', false);
    setProtoCheck('other', false, '', '');

    els.gcpFwIpRanges.value = '0.0.0.0/0';
    els.gcpFwTargetTags.value = '';
    els.gcpFwDescription.value = '';
    els.gcpFwDisabled.checked = false;
  }
  updateGCPProtocolUI();
}

function closeGCPFirewallModal() {
  if (els.gcpFwModal) {
    els.gcpFwModal.hidden = true;
  }
}

function updateGCPProtocolUI() {
  const isAll = els.gcpProtoModeAll?.checked;
  if (els.gcpProtoCustomContainer) {
    els.gcpProtoCustomContainer.style.display = isAll ? 'none' : 'flex';
  }
  const isIngress = els.gcpFwDirection?.value === 'INGRESS';
  if (els.gcpFwIpLabel) {
    els.gcpFwIpLabel.innerHTML = isIngress 
      ? '来源 IP 网段 <small style="color: var(--muted);">(默认 0.0.0.0/0)</small>'
      : '目的 IP 网段 <small style="color: var(--muted);">(默认 0.0.0.0/0)</small>';
  }
}

async function handleGCPFirewallSubmit(event) {
  event.preventDefault();
  const account = selectedFirewallAccount?.account;
  if (!account) {
    showFormMessage(els.gcpFwModalMsg, '未选择 GCP 账号', 'error');
    return;
  }

  const editMode = els.gcpFwEditMode.value;
  const name = els.gcpFwName.value.trim();
  const priority = parseInt(els.gcpFwPriority.value, 10) || 1000;
  const direction = els.gcpFwDirection.value;
  const action = els.gcpFwAction.value;

  const isAll = els.gcpProtoModeAll?.checked;
  let protocols = [];

  if (isAll) {
    protocols.push({ ipProtocol: 'all' });
  } else {
    const isTcp = els.gcpProtoCheckTcp?.checked;
    const isUdp = els.gcpProtoCheckUdp?.checked;
    const isIcmp = els.gcpProtoCheckIcmp?.checked;
    const isOther = els.gcpProtoCheckOther?.checked;

    if (!isTcp && !isUdp && !isIcmp && !isOther) {
      showFormMessage(els.gcpFwModalMsg, '请至少勾选一个协议（如 TCP、UDP 或 ICMP）', 'error');
      return;
    }

    if (isTcp) {
      const tcpRaw = (els.gcpProtoTcpPorts?.value || '').trim();
      const tcpPorts = tcpRaw ? tcpRaw.split(',').map(s => s.trim()).filter(Boolean) : [];
      protocols.push({ ipProtocol: 'tcp', ports: tcpPorts });
    }
    if (isUdp) {
      const udpRaw = (els.gcpProtoUdpPorts?.value || '').trim();
      const udpPorts = udpRaw ? udpRaw.split(',').map(s => s.trim()).filter(Boolean) : [];
      protocols.push({ ipProtocol: 'udp', ports: udpPorts });
    }
    if (isIcmp) {
      protocols.push({ ipProtocol: 'icmp' });
    }
    if (isOther) {
      const otherName = (els.gcpProtoOtherName?.value || '').trim().toLowerCase();
      if (!otherName) {
        showFormMessage(els.gcpFwModalMsg, '请填写其他协议的名称或协议号（如 esp, ah, sctp 等）', 'error');
        return;
      }
      const otherRaw = (els.gcpProtoOtherPorts?.value || '').trim();
      const otherPorts = otherRaw ? otherRaw.split(',').map(s => s.trim()).filter(Boolean) : [];
      protocols.push({ ipProtocol: otherName, ports: otherPorts });
    }
  }

  const rawRanges = els.gcpFwIpRanges.value.trim();
  const ranges = rawRanges ? rawRanges.split(',').map(s => s.trim()).filter(Boolean) : ['0.0.0.0/0'];
  const rawTags = els.gcpFwTargetTags.value.trim();
  const targetTags = rawTags ? rawTags.split(',').map(s => s.trim()).filter(Boolean) : [];
  const description = els.gcpFwDescription.value.trim();
  const disabled = els.gcpFwDisabled.checked;

  const rulePayload = {
    name,
    priority,
    direction,
    action,
    protocols,
    ipProtocol: protocols[0]?.ipProtocol || 'tcp',
    ports: protocols[0]?.ports || [],
    sourceRanges: direction === 'INGRESS' ? ranges : [],
    destinationRanges: direction === 'EGRESS' ? ranges : [],
    targetTags,
    description,
    disabled
  };

  const submitBtn = els.gcpFwModal.querySelector('button[type="submit"]');
  if (submitBtn) submitBtn.disabled = true;
  showFormMessage(els.gcpFwModalMsg, '正在保存防火墙规则到 GCP...', 'info');

  try {
    const url = editMode === 'edit'
      ? `/api/gcp/${encodeURIComponent(account)}/firewalls/${encodeURIComponent(name)}`
      : `/api/gcp/${encodeURIComponent(account)}/firewalls`;
    const method = editMode === 'edit' ? 'PUT' : 'POST';

    await fetchJSON(url, {
      method,
      body: rulePayload
    });

    addLog(`GCP 防火墙规则 ${name} ${editMode === 'edit' ? '修改' : '创建'}成功。`, 'success');
    closeGCPFirewallModal();
    await fetchGCPFirewalls(account);
  } catch (error) {
    showFormMessage(els.gcpFwModalMsg, `保存失败: ${error.message}`, 'error');
    addLog(`保存 GCP 防火墙规则失败: ${error.message}`, 'error');
  } finally {
    if (submitBtn) submitBtn.disabled = false;
  }
}

async function toggleGCPFirewall(name, currentlyDisabled) {
  const account = selectedFirewallAccount?.account;
  if (!account) return;
  const newDisabled = !currentlyDisabled;
  const actionText = newDisabled ? '禁用' : '启用';
  addLog(`正在${actionText} GCP 防火墙规则 ${name}...`, 'info');

  try {
    await fetchJSON(`/api/gcp/${encodeURIComponent(account)}/firewalls/${encodeURIComponent(name)}/toggle`, {
      method: 'PATCH',
      body: { disabled: newDisabled }
    });
    addLog(`GCP 防火墙规则 ${name} 已${actionText}。`, 'success');
    await fetchGCPFirewalls(account);
  } catch (error) {
    addLog(`${actionText} GCP 防火墙规则失败: ${error.message}`, 'error');
  }
}

async function deleteGCPFirewall(name) {
  const account = selectedFirewallAccount?.account;
  if (!account) return;
  if (!confirm(`确定要删除 GCP 防火墙规则 ${name} 吗？此操作不可撤销！`)) {
    return;
  }
  addLog(`正在删除 GCP 防火墙规则 ${name}...`, 'info');

  try {
    await fetchJSON(`/api/gcp/${encodeURIComponent(account)}/firewalls/${encodeURIComponent(name)}`, {
      method: 'DELETE'
    });
    addLog(`GCP 防火墙规则 ${name} 已删除。`, 'success');
    await fetchGCPFirewalls(account);
  } catch (error) {
    addLog(`删除 GCP 防火墙规则失败: ${error.message}`, 'error');
  }
}

function switchToGCPFirewall(account) {
  switchToFirewall('gcp', account);
}

// ------------------------------------------
// OCI Account Security Lists & NSGs Methods
// ------------------------------------------
function renderOciFirewallSubTabsHTML() {
  return `
    <div class="oci-fw-subtabs">
      <button class="sg-resource-tab oci-fw-subtab ${ociFirewallTab === 'security-lists' ? 'active' : ''}" type="button" data-tab="security-lists">安全列表 (Security Lists)</button>
      <button class="sg-resource-tab oci-fw-subtab ${ociFirewallTab === 'network-security-groups' ? 'active' : ''}" type="button" data-tab="network-security-groups">网络安全组 (NSG)</button>
      <button class="sg-resource-tab oci-fw-subtab ${ociFirewallTab === 'route-tables' ? 'active' : ''}" type="button" data-tab="route-tables">路由表 (Route Tables)</button>
    </div>
  `;
}

async function fetchOciFirewallResources(account) {
  if (ociFirewallTab === 'network-security-groups') {
    await fetchOciAccountNSGs(account);
  } else if (ociFirewallTab === 'route-tables') {
    await fetchOciRouteTables(account);
  } else {
    await fetchOciSecurityLists(account);
  }
}

async function fetchOciSecurityLists(account) {
  if (!account && selectedFirewallAccount) account = selectedFirewallAccount.account;
  if (!account) return;

  if (els.fwContentContainer) {
    els.fwContentContainer.innerHTML = `${renderOciFirewallSubTabsHTML()}<div class="empty-state compact">正在加载 OCI 安全列表...</div>`;
  }

  try {
    const data = await fetchJSON(`/api/oci/${encodeURIComponent(account)}/security-lists`);
    currentOciSecurityLists = Array.isArray(data?.securityLists) ? data.securityLists : [];
    renderOciSecurityListsTable(currentOciSecurityLists);
    addLog(`已加载 OCI 账号 ${account} 的安全列表，共 ${currentOciSecurityLists.length} 个。`, 'info');
  } catch (error) {
    if (els.fwContentContainer) {
      els.fwContentContainer.innerHTML = `${renderOciFirewallSubTabsHTML()}<div class="empty-state compact error">加载 OCI 安全列表失败：${escapeHtml(error.message)}</div>`;
    }
    addLog(`加载 OCI 安全列表失败：${error.message}`, 'error');
  }
}

function renderOciSecurityListsTable(lists) {
  if (!els.fwContentContainer) return;
  if (!lists || lists.length === 0) {
    els.fwContentContainer.innerHTML = `${renderOciFirewallSubTabsHTML()}<div class="empty-state compact">当前 OCI 账号下暂无安全列表 (Security List)。</div>`;
    return;
  }

  els.fwContentContainer.innerHTML = `${renderOciFirewallSubTabsHTML()}
    <table class="gcp-fw-table">
      <thead>
        <tr>
          <th>安全列表名称</th>
          <th>入站规则数</th>
          <th>出站规则数</th>
          <th>OCID 标识</th>
          <th style="text-align: right;">操作</th>
        </tr>
      </thead>
      <tbody>
        ${lists.map(list => {
          const inCount = Array.isArray(list.ingressRules) ? list.ingressRules.length : 0;
          const outCount = Array.isArray(list.egressRules) ? list.egressRules.length : 0;
          return `
            <tr data-id="${escapeAttr(list.id)}">
              <td>
                <strong style="color: var(--text);">${escapeHtml(list.name || list.id)}</strong>
              </td>
              <td><span class="fw-badge fw-badge-ingress">${inCount} 条入站</span></td>
              <td><span class="fw-badge fw-badge-egress">${outCount} 条出站</span></td>
              <td><code style="font-size: 11px; color: var(--muted);">${escapeHtml((list.id || '').slice(0, 36))}...</code></td>
              <td style="text-align: right;">
                <div class="fw-actions-cell" style="justify-content: flex-end;">
                  <button class="secondary-btn oci-sl-manage-btn" type="button" data-id="${escapeAttr(list.id)}" title="管理入站/出站规则">
                    <i class="bi bi-shield-lock"></i>
                    <span>管理规则</span>
                  </button>
                </div>
              </td>
            </tr>
          `;
        }).join('')}
      </tbody>
    </table>
  `;
}

async function fetchOciAccountNSGs(account) {
  if (!account && selectedFirewallAccount) account = selectedFirewallAccount.account;
  if (!account) return;

  if (els.fwContentContainer) {
    els.fwContentContainer.innerHTML = `${renderOciFirewallSubTabsHTML()}<div class="empty-state compact">正在加载 OCI 网络安全组 (NSG)...</div>`;
  }

  try {
    const data = await fetchJSON(`/api/oci/${encodeURIComponent(account)}/network-security-groups`);
    currentOciAccountNSGs = Array.isArray(data?.networkSecurityGroups) ? data.networkSecurityGroups : [];
    renderOciNSGsTable(currentOciAccountNSGs);
    addLog(`已加载 OCI 账号 ${account} 的网络安全组，共 ${currentOciAccountNSGs.length} 个。`, 'info');
  } catch (error) {
    if (els.fwContentContainer) {
      els.fwContentContainer.innerHTML = `${renderOciFirewallSubTabsHTML()}<div class="empty-state compact error">加载 OCI 网络安全组失败：${escapeHtml(error.message)}</div>`;
    }
    addLog(`加载 OCI 网络安全组失败：${error.message}`, 'error');
  }
}

function renderOciNSGsTable(groups) {
  if (!els.fwContentContainer) return;
  if (!groups || groups.length === 0) {
    els.fwContentContainer.innerHTML = `${renderOciFirewallSubTabsHTML()}<div class="empty-state compact">当前 OCI 账号下暂无网络安全组 (NSG)。可在实例「安全规则 → 网络安全组」中创建。</div>`;
    return;
  }

  els.fwContentContainer.innerHTML = `${renderOciFirewallSubTabsHTML()}
    <table class="gcp-fw-table">
      <thead>
        <tr>
          <th>网络安全组名称</th>
          <th>入站规则数</th>
          <th>出站规则数</th>
          <th>OCID 标识</th>
          <th style="text-align: right;">操作</th>
        </tr>
      </thead>
      <tbody>
        ${groups.map(group => {
          const inCount = Array.isArray(group.ingressRules) ? group.ingressRules.length : 0;
          const outCount = Array.isArray(group.egressRules) ? group.egressRules.length : 0;
          return `
            <tr data-id="${escapeAttr(group.id)}">
              <td>
                <strong style="color: var(--text);">${escapeHtml(group.name || group.id)}</strong>
              </td>
              <td><span class="fw-badge fw-badge-ingress">${inCount} 条入站</span></td>
              <td><span class="fw-badge fw-badge-egress">${outCount} 条出站</span></td>
              <td><code style="font-size: 11px; color: var(--muted);">${escapeHtml((group.id || '').slice(0, 36))}...</code></td>
              <td style="text-align: right;">
                <div class="fw-actions-cell" style="justify-content: flex-end;">
                  <button class="secondary-btn oci-nsg-manage-btn" type="button" data-id="${escapeAttr(group.id)}">
                    <i class="bi bi-shield-lock"></i>
                    <span>管理规则</span>
                  </button>
                  <button class="ghost-btn oci-nsg-rename-btn" type="button" data-id="${escapeAttr(group.id)}">
                    <i class="bi bi-pencil"></i>
                    <span>重命名</span>
                  </button>
                  <button class="ghost-btn oci-nsg-delete-btn" type="button" data-id="${escapeAttr(group.id)}" style="color: var(--red);" title="删除网络安全组">
                    <i class="bi bi-trash"></i>
                    <span>删除</span>
                  </button>
                </div>
              </td>
            </tr>
          `;
        }).join('')}
      </tbody>
    </table>
  `;
}

async function renameOciAccountNSG(group) {
  const account = selectedFirewallAccount?.account;
  if (!account || !group) return;

  const newName = prompt('请输入新的网络安全组名称：', group.name || '');
  if (newName === null) return;
  const trimmed = newName.trim();
  if (!trimmed) {
    addLog('网络安全组名称不能为空。', 'error');
    return;
  }

  try {
    await fetchJSON(`/api/oci/${encodeURIComponent(account)}/network-security-groups/${encodeURIComponent(group.id)}`, {
      method: 'PUT',
      body: { name: trimmed }
    });
    addLog(`OCI 网络安全组已重命名为：${trimmed}`, 'success');
    await fetchOciAccountNSGs(account);
  } catch (err) {
    addLog(`重命名 OCI 网络安全组失败：${err.message}`, 'error');
  }
}

async function deleteOciAccountNSG(group) {
  const account = selectedFirewallAccount?.account;
  if (!account || !group) return;

  if (!confirm(`确认永久删除网络安全组「${group.name || group.id}」？\n\n注意：若该网络安全组仍绑定在实例 VNIC 上，将提示先解绑。此操作不可撤销！`)) {
    return;
  }

  try {
    await fetchJSON(`/api/oci/${encodeURIComponent(account)}/network-security-groups/${encodeURIComponent(group.id)}`, {
      method: 'DELETE'
    });
    addLog(`OCI 网络安全组已删除：${group.name || group.id}`, 'success');
    await fetchOciAccountNSGs(account);
  } catch (err) {
    alert(`删除失败：\n${err.message}`);
    addLog(`删除 OCI 网络安全组失败：${err.message}`, 'error');
  }
}

function openOciAccountSLModal(list, resourceType = 'security-list') {
  if (!els.ociAccSlModal || !list) return;
  currentOciAccountSL = list;
  currentOciAccountModalType = resourceType;
  ociAccountSLDirection = 'ingress';
  ociAccountSLData = {
    ingress: JSON.parse(JSON.stringify(list.ingressRules || [])),
    egress: JSON.parse(JSON.stringify(list.egressRules || []))
  };

  if (els.ociAccSlModalTitle) {
    const label = resourceType === 'nsg' ? 'OCI 网络安全组' : 'OCI 安全列表';
    els.ociAccSlModalTitle.textContent = `${label} - ${list.name || list.id}`;
  }
  if (els.ociAccSlSave) {
    els.ociAccSlSave.textContent = resourceType === 'nsg' ? '保存网络安全组规则' : '保存安全列表规则';
  }
  if (els.ociAccSlMsg) {
    els.ociAccSlMsg.textContent = '';
    els.ociAccSlMsg.className = 'form-message';
  }

  els.ociAccSlModal.hidden = false;
  updateOciAccountSLTabs();
  renderOciAccountSLRules();
}

function closeOciAccountSLModal() {
  if (els.ociAccSlModal) {
    els.ociAccSlModal.hidden = true;
  }
  currentOciAccountSL = null;
}

function switchOciAccountSLDirection(dir) {
  saveCurrentOciAccountSLInputState();
  ociAccountSLDirection = dir;
  updateOciAccountSLTabs();
  renderOciAccountSLRules();
}

function updateOciAccountSLTabs() {
  if (els.ociAccSlTabIngress) {
    els.ociAccSlTabIngress.classList.toggle('active', ociAccountSLDirection === 'ingress');
  }
  if (els.ociAccSlTabEgress) {
    els.ociAccSlTabEgress.classList.toggle('active', ociAccountSLDirection === 'egress');
  }
}

function renderOciAccountSLRules() {
  if (!els.ociAccSlModalBody) return;
  const rules = ociAccountSLData[ociAccountSLDirection] || [];
  if (rules.length === 0) {
    els.ociAccSlModalBody.innerHTML = `<div class="empty-state compact">暂无${ociAccountSLDirection === 'egress' ? '出站' : '入站'}安全规则，点击下方「+ 添加安全规则」创建</div>`;
    return;
  }

  els.ociAccSlModalBody.innerHTML = rules.map((rule, index) => ociAccountSLRuleRowHtml(rule, index, ociAccountSLDirection)).join('');
  updateSecurityListProtocolControls();
}

function ociAccountSLRuleRowHtml(rule = {}, index = 0, direction = 'ingress') {
  const protocol = normalizeSecurityListProtocol(rule.protocol || '6');
  const minPort = rule.minPort ?? '';
  const maxPort = rule.maxPort ?? '';
  const icmpType = rule.icmpType ?? '';
  const icmpCode = rule.icmpCode ?? '';
  const endpoint = direction === 'egress'
    ? (rule.destination || '0.0.0.0/0')
    : (rule.source || '0.0.0.0/0');
  const endpointType = direction === 'egress'
    ? (rule.destinationType || 'CIDR_BLOCK')
    : (rule.sourceType || 'CIDR_BLOCK');
  const description = rule.description || '';
  const rowLabel = `规则 ${index + 1}`;
  const endpointLabel = direction === 'egress' ? '目标 CIDR' : '来源 CIDR';
  const endpointTypeLabel = direction === 'egress' ? '目标类型' : '来源类型';
  const protocolOption = SECURITY_PROTOCOL_OPTIONS.find(option =>
    option.value === protocol && (option.minPort ?? '') === minPort && (option.maxPort ?? '') === maxPort
  );
  const protocolSelectValue = protocolOption ? securityProtocolOptionValue(protocolOption) : securityProtocolOptionValue({value: protocol});

  return `<div class="sg-rule-row" data-rule-id="${escapeAttr(rule.id || '')}">
    <div class="sg-rule-meta">
      <strong>${escapeHtml(rowLabel)}</strong>
      <span>${rule.id ? escapeHtml(rule.id) : (direction === 'egress' ? '出站' : '入站') + '安全规则'}</span>
    </div>
    <div class="sg-rule-fields">
      <label class="field compact">
        <span>协议</span>
        <select class="sg-f-protocol">${securityProtocolOptionsHtml(protocolSelectValue)}</select>
      </label>
      <label class="field compact">
        <span>${endpointLabel}</span>
        <input type="text" class="sg-f-endpoint" value="${escapeAttr(endpoint)}" placeholder="0.0.0.0/0">
      </label>
      <label class="field compact">
        <span>端口起</span>
        <input type="number" class="sg-f-min-port" min="1" max="65535" value="${escapeAttr(minPort)}" placeholder="全部">
      </label>
      <label class="field compact">
        <span>端口止</span>
        <input type="number" class="sg-f-max-port" min="1" max="65535" value="${escapeAttr(maxPort)}" placeholder="同起始">
      </label>
      <label class="field compact">
        <span>ICMP 类型</span>
        <select class="sg-f-icmp-type">${icmpTypeOptionsHtml(icmpType)}</select>
      </label>
      <label class="field compact">
        <span>ICMP 代码</span>
        <select class="sg-f-icmp-code">${icmpCodeOptionsHtml(icmpType, icmpCode)}</select>
      </label>
      <label class="field compact">
        <span>${endpointTypeLabel}</span>
        <select class="sg-f-endpoint-type">
          <option value="CIDR_BLOCK" ${endpointType === 'CIDR_BLOCK' ? 'selected' : ''}>CIDR</option>
          <option value="SERVICE_CIDR_BLOCK" ${endpointType === 'SERVICE_CIDR_BLOCK' ? 'selected' : ''}>Service CIDR</option>
          <option value="NETWORK_SECURITY_GROUP" ${endpointType === 'NETWORK_SECURITY_GROUP' ? 'selected' : ''}>Network Security Group</option>
        </select>
      </label>
      <label class="switch-row compact">
        <input type="checkbox" class="sg-f-stateless" ${rule.isStateless ? 'checked' : ''}>
        <span>无状态</span>
      </label>
      <label class="field compact sg-description-field">
        <span>描述</span>
        <input type="text" class="sg-f-description" value="${escapeAttr(description)}" placeholder="可选">
      </label>
    </div>
    <div class="sg-rule-footer">
      <div class="sg-allow-summary">
        <span>允许</span>
        <strong>${escapeHtml(securityRuleAllowText({ protocol, minPort, maxPort, icmpType, icmpCode }))}</strong>
      </div>
      <button class="ghost-btn dns-remove-btn sg-rule-delete-btn" type="button">删除</button>
    </div>
  </div>`;
}

function saveCurrentOciAccountSLInputState() {
  if (!els.ociAccSlModalBody) return;
  const rows = els.ociAccSlModalBody.querySelectorAll('.sg-rule-row');
  const rules = Array.from(rows).map(row => {
    const selectedProtocol = parseSecurityProtocolSelection(row.querySelector('.sg-f-protocol')?.value || 'all');
    const protocol = selectedProtocol.protocol;
    const minPort = securityRuleNumberValue(row.querySelector('.sg-f-min-port')?.value);
    const maxPort = securityRuleNumberValue(row.querySelector('.sg-f-max-port')?.value);
    const icmpType = securityRuleNumberValue(row.querySelector('.sg-f-icmp-type')?.value);
    const icmpCode = securityRuleNumberValue(row.querySelector('.sg-f-icmp-code')?.value);
    const endpoint = row.querySelector('.sg-f-endpoint')?.value?.trim() || '';
    const endpointType = row.querySelector('.sg-f-endpoint-type')?.value || 'CIDR_BLOCK';
    const rule = {
      id: row.dataset.ruleId || '',
      protocol,
      minPort: protocol === '6' || protocol === '17' ? minPort : null,
      maxPort: protocol === '6' || protocol === '17' ? maxPort : null,
      icmpType: protocol === '1' ? icmpType : null,
      icmpCode: protocol === '1' ? icmpCode : null,
      description: row.querySelector('.sg-f-description')?.value?.trim() || '',
      isStateless: row.querySelector('.sg-f-stateless')?.checked || false
    };
    if (ociAccountSLDirection === 'egress') {
      rule.destination = endpoint;
      rule.destinationType = endpointType;
    } else {
      rule.source = endpoint;
      rule.sourceType = endpointType;
    }
    return rule;
  });
  ociAccountSLData[ociAccountSLDirection] = rules;
}

function addOciAccountSLRuleRow() {
  saveCurrentOciAccountSLInputState();
  ociAccountSLData[ociAccountSLDirection].push({
    protocol: '6',
    minPort: 80,
    maxPort: 80,
    source: '0.0.0.0/0',
    destination: '0.0.0.0/0',
    sourceType: 'CIDR_BLOCK',
    destinationType: 'CIDR_BLOCK',
    isStateless: false,
    description: ''
  });
  renderOciAccountSLRules();
}

async function saveOciAccountSecurityListRules() {
  const account = selectedFirewallAccount?.account;
  const list = currentOciAccountSL;
  if (!account || !list) return;

  saveCurrentOciAccountSLInputState();

  if (els.ociAccSlMsg) {
    els.ociAccSlMsg.textContent = '正在保存规则到 OCI...';
    els.ociAccSlMsg.className = 'form-message';
  }
  if (els.ociAccSlSave) els.ociAccSlSave.disabled = true;

  try {
    const isNSG = currentOciAccountModalType === 'nsg';
    const endpoint = isNSG
      ? `/api/oci/${encodeURIComponent(account)}/network-security-groups/${encodeURIComponent(list.id)}/rules`
      : `/api/oci/${encodeURIComponent(account)}/security-lists/${encodeURIComponent(list.id)}/rules`;
    const resourceLabel = isNSG ? '网络安全组' : '安全列表';

    await fetchJSON(endpoint, {
      method: 'PUT',
      body: {
        ingressRules: ociAccountSLData.ingress,
        egressRules: ociAccountSLData.egress
      }
    });

    list.ingressRules = ociAccountSLData.ingress;
    list.egressRules = ociAccountSLData.egress;

    if (els.ociAccSlMsg) {
      els.ociAccSlMsg.textContent = `${resourceLabel}规则已成功保存。`;
      els.ociAccSlMsg.className = 'form-message success';
    }
    addLog(`OCI ${resourceLabel} ${list.name || list.id} 规则已保存。`, 'success');
    fetchCurrentFirewalls();
  } catch (err) {
    if (els.ociAccSlMsg) {
      els.ociAccSlMsg.textContent = `保存失败: ${err.message}`;
      els.ociAccSlMsg.className = 'form-message error';
    }
    addLog(`保存 OCI 规则失败: ${err.message}`, 'error');
  } finally {
    if (els.ociAccSlSave) els.ociAccSlSave.disabled = false;
  }
}

// ------------------------------------------
// OCI Account Route Tables Methods
// ------------------------------------------
async function fetchOciRouteTables(account) {
  if (!account && selectedFirewallAccount) account = selectedFirewallAccount.account;
  if (!account) return;

  if (els.fwContentContainer) {
    els.fwContentContainer.innerHTML = `${renderOciFirewallSubTabsHTML()}<div class="empty-state compact">正在加载 OCI 路由表...</div>`;
  }

  try {
    const data = await fetchJSON(`/api/oci/${encodeURIComponent(account)}/route-tables`);
    currentOciRouteTables = Array.isArray(data?.routeTables) ? data.routeTables : [];
    renderOciRouteTablesTable(currentOciRouteTables);
    addLog(`已加载 OCI 账号 ${account} 的路由表，共 ${currentOciRouteTables.length} 个。`, 'info');
  } catch (error) {
    if (els.fwContentContainer) {
      els.fwContentContainer.innerHTML = `${renderOciFirewallSubTabsHTML()}<div class="empty-state compact error">加载 OCI 路由表失败：${escapeHtml(error.message)}</div>`;
    }
    addLog(`加载 OCI 路由表失败：${error.message}`, 'error');
  }
}

function renderOciRouteTablesTable(tables) {
  if (!els.fwContentContainer) return;
  if (!tables || tables.length === 0) {
    els.fwContentContainer.innerHTML = `${renderOciFirewallSubTabsHTML()}<div class="empty-state compact">当前 OCI 账号下暂无路由表 (Route Table)。</div>`;
    return;
  }

  els.fwContentContainer.innerHTML = `${renderOciFirewallSubTabsHTML()}
    <table class="gcp-fw-table">
      <thead>
        <tr>
          <th>路由表名称</th>
          <th>所属 VCN / 关联子网</th>
          <th>路由规则</th>
          <th>OCID 标识</th>
          <th style="text-align: right;">操作</th>
        </tr>
      </thead>
      <tbody>
        ${tables.map(rt => {
          const rules = Array.isArray(rt.routeRules) ? rt.routeRules : [];
          const subnets = Array.isArray(rt.subnetNames) ? rt.subnetNames : [];
          const hasV4 = rules.some(r => r.destination === '0.0.0.0/0');
          const hasV6 = rules.some(r => r.destination === '::/0');
          return `
            <tr data-id="${escapeAttr(rt.id)}">
              <td><strong style="color: var(--text);">${escapeHtml(rt.name || rt.id)}</strong></td>
              <td>
                <div style="font-size: 12.5px; color: var(--text);">${escapeHtml(rt.vcnName || rt.vcnId || '-')}</div>
                <div style="font-size: 11.5px; color: var(--muted);">${subnets.length ? `子网: ${escapeHtml(subnets.join(', '))}` : '未关联子网'}</div>
              </td>
              <td>
                <div style="display: flex; gap: 6px; flex-wrap: wrap; align-items: center;">
                  <span class="fw-badge fw-badge-egress">${rules.length} 条规则</span>
                  ${hasV4 ? '<span class="oci-ip-badge primary">0.0.0.0/0</span>' : ''}
                  ${hasV6 ? '<span class="oci-ip-badge attached">::/0</span>' : '<span class="oci-ip-badge ephemeral">无 ::/0</span>'}
                </div>
              </td>
              <td><code style="font-size: 11px; color: var(--muted);">${escapeHtml((rt.id || '').slice(0, 36))}...</code></td>
              <td style="text-align: right;">
                <div class="fw-actions-cell" style="justify-content: flex-end;">
                  <button class="secondary-btn oci-rt-manage-btn" type="button" data-id="${escapeAttr(rt.id)}">
                    <i class="bi bi-signpost-split"></i>
                    <span>管理路由</span>
                  </button>
                </div>
              </td>
            </tr>
          `;
        }).join('')}
      </tbody>
    </table>
  `;
}

function openOciRouteTableModal(rt) {
  if (!els.ociRtModal || !rt) return;
  currentOciRouteTable = rt;
  ociRouteTableRulesData = JSON.parse(JSON.stringify(rt.routeRules || []));

  if (els.ociRtModalTitle) {
    els.ociRtModalTitle.textContent = `OCI 路由表 - ${rt.name || rt.id}`;
  }
  if (els.ociRtMeta) {
    const subnets = Array.isArray(rt.subnetNames) && rt.subnetNames.length ? rt.subnetNames.join(', ') : '未关联子网';
    els.ociRtMeta.textContent = `所属 VCN：${rt.vcnName || rt.vcnId || '-'} | 关联子网：${subnets}`;
  }
  if (els.ociRtMsg) {
    els.ociRtMsg.textContent = '';
    els.ociRtMsg.className = 'form-message';
  }
  els.ociRtModal.hidden = false;
  renderOciRouteTableRules();
}

function closeOciRouteTableModal() {
  if (els.ociRtModal) {
    els.ociRtModal.hidden = true;
  }
  currentOciRouteTable = null;
}

function ociRouteRuleRowHtml(rule = {}, index = 0, entities = []) {
  const dest = rule.destination || '';
  const destType = rule.destinationType || 'CIDR_BLOCK';
  const entityId = rule.networkEntityId || '';
  const desc = rule.description || '';
  const matched = entities.some(e => e.id === entityId);
  const useCustom = Boolean(entityId) && !matched;
  const selectValue = useCustom ? '__custom__' : (matched ? entityId : (entities[0]?.id || '__custom__'));

  return `
    <div class="sg-rule-row">
      <div class="sg-rule-meta">
        <strong>规则 ${index + 1}</strong>
        <span>${escapeHtml(rule.routeType || 'STATIC')}</span>
      </div>
      <div class="sg-rule-fields">
        <label class="field compact">
          <span>目标类型</span>
          <select class="oci-rt-dest-type">
            <option value="CIDR_BLOCK" ${destType === 'CIDR_BLOCK' ? 'selected' : ''}>CIDR_BLOCK</option>
            <option value="SERVICE_CIDR_BLOCK" ${destType === 'SERVICE_CIDR_BLOCK' ? 'selected' : ''}>SERVICE_CIDR_BLOCK</option>
          </select>
        </label>
        <label class="field compact">
          <span>目标 CIDR / 服务</span>
          <input type="text" class="oci-rt-dest" value="${escapeAttr(dest)}" placeholder="例如 ::/0 或 0.0.0.0/0">
        </label>
        <label class="field compact">
          <span>目标网关 / 实体</span>
          <select class="oci-rt-entity-select">
            ${entities.map(ent => `<option value="${escapeAttr(ent.id)}" ${selectValue === ent.id ? 'selected' : ''}>[${escapeHtml(ent.type)}] ${escapeHtml(ent.name)}</option>`).join('')}
            <option value="__custom__" ${selectValue === '__custom__' ? 'selected' : ''}>自定义 OCID...</option>
          </select>
        </label>
        <label class="field compact oci-rt-entity-custom-wrap" ${selectValue === '__custom__' ? '' : 'hidden'}>
          <span>自定义实体 OCID</span>
          <input type="text" class="oci-rt-entity-custom" value="${escapeAttr(entityId)}" placeholder="ocid1.internetgateway...">
        </label>
        <label class="field compact sg-description-field">
          <span>描述</span>
          <input type="text" class="oci-rt-desc" value="${escapeAttr(desc)}" placeholder="可选">
        </label>
      </div>
      <div class="sg-rule-footer">
        <div class="sg-allow-summary">
          <span>路由目标</span>
          <strong>${escapeHtml(dest || '-')} → ${escapeHtml(rule.networkEntityName || entityId || '待选择')}</strong>
        </div>
        <button class="ghost-btn dns-remove-btn oci-rt-delete-btn" type="button">删除</button>
      </div>
    </div>
  `;
}

function renderOciRouteTableRules() {
  if (!els.ociRtModalBody || !currentOciRouteTable) return;
  const entities = Array.isArray(currentOciRouteTable.availableEntities) ? currentOciRouteTable.availableEntities : [];
  if (ociRouteTableRulesData.length === 0) {
    els.ociRtModalBody.innerHTML = '<div class="empty-state compact">暂无路由规则，可点击上方快捷补充按钮或下方「+ 添加路由规则」</div>';
    return;
  }
  els.ociRtModalBody.innerHTML = ociRouteTableRulesData.map((r, idx) => ociRouteRuleRowHtml(r, idx, entities)).join('');
}

function saveCurrentOciRouteTableInputState() {
  if (!els.ociRtModalBody) return;
  const rows = els.ociRtModalBody.querySelectorAll('.sg-rule-row');
  const entities = Array.isArray(currentOciRouteTable?.availableEntities) ? currentOciRouteTable.availableEntities : [];
  ociRouteTableRulesData = Array.from(rows).map(row => {
    const selectVal = row.querySelector('.oci-rt-entity-select')?.value || '';
    const networkEntityId = selectVal === '__custom__'
      ? (row.querySelector('.oci-rt-entity-custom')?.value?.trim() || '')
      : selectVal;
    const matched = entities.find(e => e.id === networkEntityId);
    return {
      destinationType: row.querySelector('.oci-rt-dest-type')?.value || 'CIDR_BLOCK',
      destination: row.querySelector('.oci-rt-dest')?.value?.trim() || '',
      networkEntityId,
      networkEntityName: matched ? `[${matched.type}] ${matched.name}` : '',
      description: row.querySelector('.oci-rt-desc')?.value?.trim() || ''
    };
  });
}

function addOciRouteRuleRow() {
  saveCurrentOciRouteTableInputState();
  const entities = Array.isArray(currentOciRouteTable?.availableEntities) ? currentOciRouteTable.availableEntities : [];
  const defaultEntity = entities.find(e => e.type === 'IGW') || entities[0] || null;
  ociRouteTableRulesData.push({
    destinationType: 'CIDR_BLOCK',
    destination: '::/0',
    networkEntityId: defaultEntity ? defaultEntity.id : '',
    networkEntityName: defaultEntity ? `[${defaultEntity.type}] ${defaultEntity.name}` : '',
    description: ''
  });
  renderOciRouteTableRules();
}

function quickAddOciDefaultRoute(cidr) {
  if (!currentOciRouteTable) return;
  saveCurrentOciRouteTableInputState();
  const exists = ociRouteTableRulesData.some(r => (r.destination || '').trim() === cidr);
  if (exists) {
    if (els.ociRtMsg) {
      els.ociRtMsg.textContent = `当前路由表已存在 ${cidr} 规则，无需重复添加。`;
      els.ociRtMsg.className = 'form-message';
    }
    return;
  }
  const entities = Array.isArray(currentOciRouteTable.availableEntities) ? currentOciRouteTable.availableEntities : [];
  const igw = entities.find(e => e.type === 'IGW') || entities[0] || null;
  if (!igw) {
    if (els.ociRtMsg) {
      els.ociRtMsg.textContent = '未检测到所属 VCN 的互联网网关 (IGW)，请点击「+ 添加路由规则」手动填写实体 OCID。';
      els.ociRtMsg.className = 'form-message error';
    }
    return;
  }
  ociRouteTableRulesData.push({
    destinationType: 'CIDR_BLOCK',
    destination: cidr,
    networkEntityId: igw.id,
    networkEntityName: `[${igw.type}] ${igw.name}`,
    description: cidr === '::/0' ? 'IPv6 default route' : 'IPv4 default route'
  });
  renderOciRouteTableRules();
  if (els.ociRtMsg) {
    els.ociRtMsg.textContent = `已添加 ${cidr} → [${igw.type}] ${igw.name}，请点击「保存路由规则」提交生效。`;
    els.ociRtMsg.className = 'form-message success';
  }
}

async function saveOciRouteTableRules() {
  const account = selectedFirewallAccount?.account;
  const rt = currentOciRouteTable;
  if (!account || !rt) return;

  saveCurrentOciRouteTableInputState();
  if (els.ociRtMsg) {
    els.ociRtMsg.textContent = '正在保存路由规则到 OCI...';
    els.ociRtMsg.className = 'form-message';
  }
  if (els.ociRtSave) els.ociRtSave.disabled = true;

  try {
    await fetchJSON(`/api/oci/${encodeURIComponent(account)}/route-tables/${encodeURIComponent(rt.id)}/rules`, {
      method: 'PUT',
      body: { routeRules: ociRouteTableRulesData }
    });
    if (els.ociRtMsg) {
      els.ociRtMsg.textContent = '路由规则已成功保存。';
      els.ociRtMsg.className = 'form-message success';
    }
    addLog(`OCI 路由表 ${rt.name || rt.id} 规则已保存。`, 'success');
    await fetchOciRouteTables(account);
    const updated = currentOciRouteTables.find(item => item.id === rt.id);
    if (updated) currentOciRouteTable = updated;
  } catch (err) {
    if (els.ociRtMsg) {
      els.ociRtMsg.textContent = `保存失败: ${err.message}`;
      els.ociRtMsg.className = 'form-message error';
    }
    addLog(`保存 OCI 路由规则失败: ${err.message}`, 'error');
  } finally {
    if (els.ociRtSave) els.ociRtSave.disabled = false;
  }
}

// ------------------------------------------
// Azure NSG Management Methods
// ------------------------------------------
async function fetchAzureNSGs(account) {
  if (!account && selectedFirewallAccount) account = selectedFirewallAccount.account;
  if (!account) return;

  if (els.fwContentContainer) {
    els.fwContentContainer.innerHTML = '<div class="empty-state compact">正在加载 Azure 网络安全组 (NSG)...</div>';
  }

  try {
    const data = await fetchJSON(`/api/azure/${encodeURIComponent(account)}/security-groups`);
    currentAzureNSGs = Array.isArray(data?.securityGroups) ? data.securityGroups : [];
    renderAzureNSGsTable(currentAzureNSGs);
    addLog(`已加载 Azure 账号 ${account} 的网络安全组，共 ${currentAzureNSGs.length} 个。`, 'info');
  } catch (error) {
    if (els.fwContentContainer) {
      els.fwContentContainer.innerHTML = `<div class="empty-state compact error">加载 Azure 网络安全组失败：${escapeHtml(error.message)}</div>`;
    }
    addLog(`加载 Azure 网络安全组失败：${error.message}`, 'error');
  }
}

function renderAzureNSGsTable(nsgs) {
  if (!els.fwContentContainer) return;
  if (!nsgs || nsgs.length === 0) {
    els.fwContentContainer.innerHTML = '<div class="empty-state compact">当前订阅下未发现网络安全组 (NSG)。</div>';
    return;
  }

  els.fwContentContainer.innerHTML = `
    <table class="gcp-fw-table">
      <thead>
        <tr>
          <th>安全组名称</th>
          <th>资源组</th>
          <th>区域</th>
          <th>自定义规则 (入站 / 出站)</th>
          <th>系统默认规则</th>
          <th style="text-align: right;">操作</th>
        </tr>
      </thead>
      <tbody>
        ${nsgs.map(nsg => {
          const rules = Array.isArray(nsg.securityRules) ? nsg.securityRules : [];
          const inCount = rules.filter(r => (r.direction || '').toLowerCase() === 'inbound').length;
          const outCount = rules.filter(r => (r.direction || '').toLowerCase() === 'outbound').length;
          const defCount = Array.isArray(nsg.defaultSecurityRules) ? nsg.defaultSecurityRules.length : 0;
          return `
            <tr data-name="${escapeAttr(nsg.name)}">
              <td>
                <strong style="color: var(--text);">${escapeHtml(nsg.name)}</strong>
              </td>
              <td><code>${escapeHtml(nsg.resourceGroup || '-')}</code></td>
              <td>${escapeHtml(getLocationText(nsg.location || 'N/A'))}</td>
              <td>
                <span class="fw-badge fw-badge-ingress">${inCount} 入站</span>
                <span class="fw-badge fw-badge-egress">${outCount} 出站</span>
              </td>
              <td><span style="color: var(--muted); font-size: 12px;">${defCount} 条系统规则</span></td>
              <td style="text-align: right;">
                <div class="fw-actions-cell" style="justify-content: flex-end;">
                  <button class="secondary-btn azure-nsg-manage-btn" type="button" data-name="${escapeAttr(nsg.name)}" title="管理入站/出站规则">
                    <i class="bi bi-shield-check"></i>
                    <span>管理规则</span>
                  </button>
                </div>
              </td>
            </tr>
          `;
        }).join('')}
      </tbody>
    </table>
  `;
}

function openAzureNSGModal(nsg) {
  if (!els.azureNsgModal || !nsg) return;
  currentAzureNSG = nsg;
  azureNSGDirection = 'Inbound';
  hideAzureNSGAddForm();

  if (els.azureNsgModalTitle) {
    els.azureNsgModalTitle.textContent = `Azure 网络安全组 - ${nsg.name} (${nsg.resourceGroup || nsg.location})`;
  }

  updateAzureNSGTabs();
  renderAzureNSGRulesList();
  els.azureNsgModal.hidden = false;
}

function closeAzureNSGModal() {
  if (els.azureNsgModal) {
    els.azureNsgModal.hidden = true;
  }
  currentAzureNSG = null;
}

function switchAzureNSGDirection(dir) {
  azureNSGDirection = dir;
  updateAzureNSGTabs();
  hideAzureNSGAddForm();
  renderAzureNSGRulesList();
}

function updateAzureNSGTabs() {
  if (els.azureNsgTabInbound) {
    els.azureNsgTabInbound.classList.toggle('active', azureNSGDirection === 'Inbound');
  }
  if (els.azureNsgTabOutbound) {
    els.azureNsgTabOutbound.classList.toggle('active', azureNSGDirection === 'Outbound');
  }
}

function renderAzureNSGRulesList() {
  if (!els.azureNsgRulesContainer || !currentAzureNSG) return;
  const isMatchDir = (r) => (r.direction || '').toLowerCase() === azureNSGDirection.toLowerCase();
  const customRules = (currentAzureNSG.securityRules || []).filter(isMatchDir).sort((a, b) => (a.priority || 1000) - (b.priority || 1000));
  const defRules = (currentAzureNSG.defaultSecurityRules || []).filter(isMatchDir).sort((a, b) => (a.priority || 65000) - (b.priority || 65000));

  if (customRules.length === 0 && defRules.length === 0) {
    els.azureNsgRulesContainer.innerHTML = `<div class="empty-state compact">暂无${azureNSGDirection === 'Inbound' ? '入站' : '出站'}规则，点击右上角「添加规则」创建</div>`;
    return;
  }

  const renderRow = (rule, isDefault) => {
    const isAllow = (rule.access || 'Allow').toLowerCase() === 'allow';
    const port = rule.destinationPortRange || '*';
    const proto = rule.protocol || '*';
    const src = rule.sourceAddressPrefix || '*';
    return `
      <tr style="${isDefault ? 'opacity: 0.72; background: rgba(0,0,0,0.02);' : ''}">
        <td><strong>${rule.priority ?? '-'}</strong></td>
        <td>
          <span style="font-weight: 500;">${escapeHtml(rule.name)}</span>
          ${isDefault ? '<span class="prov-badge" style="background: var(--panel-border); color: var(--muted); margin-left: 6px;">系统默认</span>' : ''}
          ${rule.description ? `<div style="color: var(--muted); font-size: 11px;">${escapeHtml(rule.description)}</div>` : ''}
        </td>
        <td><code>${escapeHtml(port)}</code></td>
        <td><code>${escapeHtml(proto)}</code></td>
        <td><span style="font-family: monospace; font-size: 11px;">${escapeHtml(src)}</span></td>
        <td>
          <span class="fw-badge ${isAllow ? 'fw-badge-allow' : 'fw-badge-deny'}">
            ${isAllow ? 'Allow 允许' : 'Deny 拒绝'}
          </span>
        </td>
        <td style="text-align: right;">
          ${isDefault 
            ? '<span style="color: var(--muted); font-size: 12px;" title="系统默认规则不可修改"><i class="bi bi-lock"></i> 只读</span>'
            : `<button class="ghost-btn azure-rule-delete-btn" type="button" data-name="${escapeAttr(rule.name)}" style="color: var(--red);" title="删除规则">
                 <i class="bi bi-trash"></i>
                 <span>删除</span>
               </button>`
          }
        </td>
      </tr>
    `;
  };

  els.azureNsgRulesContainer.innerHTML = `
    <table class="azure-nsg-table">
      <thead>
        <tr>
          <th>优先级</th>
          <th>规则名称</th>
          <th>端口</th>
          <th>协议</th>
          <th>源地址</th>
          <th>动作</th>
          <th style="text-align: right;">操作</th>
        </tr>
      </thead>
      <tbody>
        ${customRules.map(r => renderRow(r, false)).join('')}
        ${defRules.map(r => renderRow(r, true)).join('')}
      </tbody>
    </table>
  `;
}

function toggleAzureNSGAddForm() {
  if (!els.azureNsgAddForm) return;
  const isHidden = els.azureNsgAddForm.style.display === 'none' || !els.azureNsgAddForm.style.display;
  if (isHidden) {
    els.azureNsgAddForm.style.display = 'block';
    // Calculate smart next priority
    const isMatchDir = (r) => (r.direction || '').toLowerCase() === azureNSGDirection.toLowerCase();
    const customRules = (currentAzureNSG?.securityRules || []).filter(isMatchDir);
    let nextPriority = 1000;
    if (customRules.length > 0) {
      const maxP = Math.max(...customRules.map(r => r.priority || 0));
      nextPriority = Math.min(4090, maxP + 10);
    }
    if (els.azRulePriority) els.azRulePriority.value = nextPriority;
    if (els.azRuleName) els.azRuleName.value = '';
    if (els.azRuleDestPort) els.azRuleDestPort.value = '80';
    if (els.azRuleSourceIp) els.azRuleSourceIp.value = '*';
    if (els.azRuleAccess) els.azRuleAccess.value = 'Allow';
    if (els.azRuleProtocol) els.azRuleProtocol.value = 'Tcp';
    if (els.azRuleDesc) els.azRuleDesc.value = '';
    if (els.azureNsgFormMsg) {
      els.azureNsgFormMsg.textContent = '';
      els.azureNsgFormMsg.className = 'form-message';
    }
  } else {
    els.azureNsgAddForm.style.display = 'none';
  }
}

function hideAzureNSGAddForm() {
  if (els.azureNsgAddForm) {
    els.azureNsgAddForm.style.display = 'none';
  }
}

function handleAzureQuickPortClick(event) {
  const btn = event.target.closest('.port-chip');
  if (!btn) return;
  const port = btn.dataset.port;
  if (!port) return;

  if (els.azRuleDestPort) els.azRuleDestPort.value = port;
  if (els.azRuleProtocol) els.azRuleProtocol.value = (port === '*' ? '*' : 'Tcp');
  if (els.azRuleName && (!els.azRuleName.value || els.azRuleName.value.startsWith('Allow-'))) {
    const cleanPort = port.replace(/[,*]/g, '_');
    els.azRuleName.value = `Allow-${cleanPort}`;
  }
}

async function handleAzureNSGAddSubmit(event) {
  event.preventDefault();
  const account = selectedFirewallAccount?.account;
  const nsg = currentAzureNSG;
  if (!account || !nsg) return;

  const name = els.azRuleName.value.trim();
  const priority = parseInt(els.azRulePriority.value, 10) || 1000;
  const access = els.azRuleAccess.value;
  const protocol = els.azRuleProtocol.value;
  const sourceAddressPrefix = els.azRuleSourceIp.value.trim() || '*';
  const destinationPortRange = els.azRuleDestPort.value.trim();
  const description = els.azRuleDesc.value.trim();

  if (!name) {
    showFormMessage(els.azureNsgFormMsg, '请输入规则名称', 'error');
    return;
  }
  if (!destinationPortRange) {
    showFormMessage(els.azureNsgFormMsg, '请输入目标端口', 'error');
    return;
  }

  const payload = {
    name,
    priority,
    direction: azureNSGDirection,
    access,
    protocol,
    sourceAddressPrefix,
    sourcePortRange: '*',
    destinationAddressPrefix: '*',
    destinationPortRange,
    description
  };

  const submitBtn = els.azureNsgAddForm.querySelector('button[type="submit"]');
  if (submitBtn) submitBtn.disabled = true;
  showFormMessage(els.azureNsgFormMsg, '正在保存安全规则到 Azure...', 'info');

  try {
    const url = `/api/azure/${encodeURIComponent(account)}/security-groups/${encodeURIComponent(nsg.name)}/rules?resourceGroup=${encodeURIComponent(nsg.resourceGroup || '')}`;
    await fetchJSON(url, {
      method: 'POST',
      body: payload
    });

    addLog(`Azure 安全组 ${nsg.name} 规则 ${name} 添加成功。`, 'success');
    hideAzureNSGAddForm();

    // Re-fetch NSGs and refresh modal
    const refreshData = await fetchJSON(`/api/azure/${encodeURIComponent(account)}/security-groups`);
    currentAzureNSGs = Array.isArray(refreshData?.securityGroups) ? refreshData.securityGroups : [];
    renderAzureNSGsTable(currentAzureNSGs);
    const updatedNSG = currentAzureNSGs.find(n => n.name === nsg.name);
    if (updatedNSG) {
      currentAzureNSG = updatedNSG;
      renderAzureNSGRulesList();
    }
  } catch (error) {
    showFormMessage(els.azureNsgFormMsg, `添加规则失败: ${error.message}`, 'error');
    addLog(`添加 Azure 安全规则失败: ${error.message}`, 'error');
  } finally {
    if (submitBtn) submitBtn.disabled = false;
  }
}

function handleAzureRulesContainerClick(event) {
  const delBtn = event.target.closest('.azure-rule-delete-btn');
  if (!delBtn) return;
  const ruleName = delBtn.dataset.name;
  if (ruleName) deleteAzureNSGRule(ruleName);
}

async function deleteAzureNSGRule(ruleName) {
  const account = selectedFirewallAccount?.account;
  const nsg = currentAzureNSG;
  if (!account || !nsg || !ruleName) return;

  if (!confirm(`确定要删除 Azure NSG 安全规则 "${ruleName}" 吗？此操作不可撤销！`)) {
    return;
  }

  addLog(`正在删除 Azure 安全组 ${nsg.name} 规则 ${ruleName}...`, 'info');

  try {
    const url = `/api/azure/${encodeURIComponent(account)}/security-groups/${encodeURIComponent(nsg.name)}/rules/${encodeURIComponent(ruleName)}?resourceGroup=${encodeURIComponent(nsg.resourceGroup || '')}`;
    await fetchJSON(url, { method: 'DELETE' });

    addLog(`Azure 安全组 ${nsg.name} 规则 ${ruleName} 已删除。`, 'success');

    // Re-fetch NSGs and refresh modal
    const refreshData = await fetchJSON(`/api/azure/${encodeURIComponent(account)}/security-groups`);
    currentAzureNSGs = Array.isArray(refreshData?.securityGroups) ? refreshData.securityGroups : [];
    renderAzureNSGsTable(currentAzureNSGs);
    const updatedNSG = currentAzureNSGs.find(n => n.name === nsg.name);
    if (updatedNSG) {
      currentAzureNSG = updatedNSG;
      renderAzureNSGRulesList();
    }
  } catch (error) {
    addLog(`删除 Azure 安全规则失败: ${error.message}`, 'error');
  }
}


// ==================== OCI Centralized IP Management ====================

let ociIPModalVM = null;
let ociIPInfo = null;
let ociIPSelectedDNSIPv6 = '';
let ociIPSubformMode = '';

function openOCIIPModal(vm) {
  ociIPModalVM = vm;
  ociIPInfo = null;
  ociIPSelectedDNSIPv6 = '';
  ociIPSubformMode = '';
  document.getElementById('oci-ip-modal-title').textContent = `${vm.name || vm.id} - IP管理`;
  document.getElementById('oci-ip-modal-body').innerHTML = '<div class="empty-state compact">加载中...</div>';
  const subform = document.getElementById('oci-ip-subform');
  if (subform) {
    subform.hidden = true;
    subform.innerHTML = '';
  }
  const msgEl = document.getElementById('oci-ip-modal-message');
  if (msgEl) {
    msgEl.textContent = '';
    msgEl.className = 'form-message';
  }
  document.getElementById('oci-ip-modal').hidden = false;
  loadOCIIPInfo();
}

function closeOCIIPModal() {
  document.getElementById('oci-ip-modal').hidden = true;
  ociIPModalVM = null;
  ociIPInfo = null;
  ociIPSubformMode = '';
}

async function loadOCIIPInfo() {
  if (!ociIPModalVM) return;
  const vm = ociIPModalVM;
  const body = document.getElementById('oci-ip-modal-body');

  try {
    const data = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips`);
    if (!ociIPModalVM || ociIPModalVM.id !== vm.id) return;
    ociIPInfo = data;
    renderOCIIPModal(data);
  } catch (err) {
    if (body) {
      body.innerHTML = `<div class="empty-state compact error">加载 IP 信息失败：${escapeHtml(err.message)}</div>`;
    }
  }
}

function renderOCIIPModal(info) {
  const body = document.getElementById('oci-ip-modal-body');
  if (!body) return;
  const vnics = Array.isArray(info?.vnics) ? info.vnics : [];
  if (vnics.length === 0) {
    body.innerHTML = '<div class="empty-state compact">未找到已附加的 VNIC。</div>';
    return;
  }

  // Ensure default selected /128 IPv6 stays valid if present
  const allChangeableV6 = [];
  vnics.forEach(v => {
    (v.ipv6s || []).forEach(ip6 => {
      if (ip6.canChange && ip6.ipAddress) allChangeableV6.push(ip6.ipAddress);
    });
  });
  if (!allChangeableV6.includes(ociIPSelectedDNSIPv6)) {
    ociIPSelectedDNSIPv6 = allChangeableV6[0] || '';
  }

  const dnsBindings = info?.dnsBindings || {};
  const hasAnyDNS = Boolean(dnsBindings.hasAny);
  const dnsCheckbox = document.getElementById('oci-ip-update-dns-checkbox');
  const dnsManualBtn = document.getElementById('oci-ip-manual-dns-btn');
  const dnsHint = document.getElementById('oci-ip-dns-hint');

  if (dnsCheckbox) {
    dnsCheckbox.disabled = !hasAnyDNS;
    if (!hasAnyDNS) dnsCheckbox.checked = false;
  }
  if (dnsManualBtn) {
    dnsManualBtn.disabled = !hasAnyDNS;
  }
  if (dnsHint) {
    if (!hasAnyDNS) {
      dnsHint.textContent = '未绑定 DNS，请先在实例卡片点击「DNS 绑定」';
    } else {
      const types = [];
      if (dnsBindings.hasA) types.push('A');
      if (dnsBindings.hasAAAA) types.push('AAAA');
      dnsHint.textContent = `已绑定 DNS：${types.join(' / ') || '其他'}`;
    }
  }

  const hasAnyIPv6Subnet = vnics.some(v => Array.isArray(v.ipv6CidrBlocks) && v.ipv6CidrBlocks.length > 0);
  const needsIPv6Setup = vnics.some(v => {
    const hasSub = Array.isArray(v.ipv6CidrBlocks) && v.ipv6CidrBlocks.length > 0;
    const hasRt = v.ipv6Setup ? Boolean(v.ipv6Setup.routeHasIpv6Default) : true;
    return !hasSub || !hasRt;
  });
  const attachV6Btn = document.getElementById('oci-ip-attach-v6-btn');
  if (attachV6Btn) {
    attachV6Btn.disabled = false;
    attachV6Btn.classList.toggle('oci-btn-muted', !hasAnyIPv6Subnet);
    attachV6Btn.textContent = '附加 IPv6';
    attachV6Btn.title = hasAnyIPv6Subnet
      ? '为 VNIC 分配新的 IPv6 地址或前缀'
      : '当前子网未开启 IPv6，点击将提示并打开「开启子网 IPv6」向导';
  }
  const enableV6Btn = document.getElementById('oci-ip-enable-v6-btn');
  if (enableV6Btn) {
    enableV6Btn.hidden = !needsIPv6Setup;
    enableV6Btn.textContent = hasAnyIPv6Subnet ? '补全子网 IPv6' : '开启子网 IPv6';
    enableV6Btn.title = '查看 VCN / 子网 / 路由表 / 安全列表 IPv6 就绪状态并一键开启';
  }

  body.innerHTML = vnics.map(vnic => {
    const privRows = (vnic.privateIps || []).map(priv => {
      const pub = priv.publicIp;
      const isReserved = pub && String(pub.lifetime || '').toUpperCase() === 'RESERVED';
      return `
        <div class="oci-ip-row">
          <span class="oci-ip-addr">${escapeHtml(priv.ipAddress)}</span>
          ${priv.isPrimary ? '<span class="oci-ip-badge primary">主私网 IP</span>' : '<span class="oci-ip-badge ephemeral">辅助私网 IP</span>'}
          ${pub ? `
            <span class="oci-ip-addr oci-ip-pub">${escapeHtml(pub.ipAddress)}</span>
            <span class="oci-ip-badge ${isReserved ? 'reserved' : 'ephemeral'}">${isReserved ? '保留 IP' : '临时 IP'}</span>
            ${isReserved ? '' : `<button class="secondary-btn oci-ip-btn-sm oci-ip-change-v4-btn" type="button" data-private-id="${escapeAttr(priv.id)}">换IP</button>`}
            <button class="ghost-btn oci-ip-btn-circle oci-ip-btn-danger oci-ip-del-v4-btn" type="button"
              data-private-id="${escapeAttr(priv.id)}"
              data-public-id="${escapeAttr(pub.id)}"
              data-is-primary="${priv.isPrimary ? 'true' : 'false'}"
              data-is-reserved="${isReserved ? 'true' : 'false'}"
              title="${isReserved ? '解绑保留 IP' : '删除公网 IP'}">×</button>
          ` : `
            <span class="oci-ip-badge ephemeral">无公网 IP</span>
            ${priv.isPrimary ? '' : `<button class="ghost-btn oci-ip-btn-circle oci-ip-btn-danger oci-ip-del-priv-btn" type="button" data-private-id="${escapeAttr(priv.id)}" title="删除辅助私网 IP">×</button>`}
          `}
        </div>
      `;
    }).join('');

    const ipv6Rows = (vnic.ipv6s || []).map(ip6 => {
      const displayAddr = ip6.cidrPrefixLength && ip6.cidrPrefixLength !== 128
        ? `${ip6.ipAddress}/${ip6.cidrPrefixLength}`
        : ip6.ipAddress;
      const isCheckedDNS = ip6.canChange && ip6.ipAddress === ociIPSelectedDNSIPv6;
      return `
        <div class="oci-ip-row">
          ${ip6.canChange && dnsBindings.hasAAAA ? `
            <label class="oci-ip-dns-select" title="选定此 IPv6 用于手动更新 DNS (AAAA)">
              <input type="radio" name="oci-dns-ipv6" value="${escapeAttr(ip6.ipAddress)}" ${isCheckedDNS ? 'checked' : ''}>
            </label>
          ` : ''}
          <span class="oci-ip-addr">${escapeHtml(displayAddr)}</span>
          ${ip6.cidrPrefixLength && ip6.cidrPrefixLength !== 128 ? `<span class="oci-ip-badge ephemeral">/${ip6.cidrPrefixLength} 前缀</span>` : ''}
          ${ip6.canChange ? `<button class="secondary-btn oci-ip-btn-sm oci-ip-change-v6-btn" type="button" data-vnic-id="${escapeAttr(vnic.id)}" data-ipv6-id="${escapeAttr(ip6.id)}">换IP</button>` : ''}
          <button class="ghost-btn oci-ip-btn-sm oci-ip-btn-danger oci-ip-del-v6-btn" type="button" data-ipv6-id="${escapeAttr(ip6.id)}" data-ip="${escapeAttr(displayAddr)}">删除IP</button>
        </div>
      `;
    }).join('');

    return `
      <div class="oci-vnic-card">
        <div class="oci-vnic-header">
          <span class="oci-vnic-title">${escapeHtml(vnic.displayName || vnic.id)}</span>
          ${vnic.isPrimary ? '<span class="oci-ip-badge primary">主 VNIC</span>' : ''}
          <span class="oci-ip-badge attached">${vnic.lifecycleState === 'ATTACHED' ? '已附加' : escapeHtml(vnic.lifecycleState)}</span>
        </div>
        ${vnic.macAddress ? `<div class="oci-vnic-mac">MAC 地址: ${escapeHtml(vnic.macAddress)}</div>` : ''}
        <div class="oci-ip-section-label">私网 IP:</div>
        ${privRows || '<div class="field-hint">暂无私网 IP</div>'}
        <div class="oci-ip-section-label">IPv6:</div>
        ${ipv6Rows || `<div class="field-hint">${(vnic.ipv6CidrBlocks || []).length > 0 ? '暂无 IPv6 地址' : '当前子网未开启 IPv6'}</div>`}
      </div>
    `;
  }).join('');
}

function setOCIIPModalMessage(text, isError = false, isSuccess = false) {
  const msgEl = document.getElementById('oci-ip-modal-message');
  if (!msgEl) return;
  msgEl.textContent = text || '';
  msgEl.className = 'form-message' + (isError ? ' error' : isSuccess ? ' success' : '');
}

function closeOCIIPSubform() {
  const subform = document.getElementById('oci-ip-subform');
  if (subform) {
    subform.hidden = true;
    subform.innerHTML = '';
  }
  ociIPSubformMode = '';
}

function openOCIIPSubform(mode) {
  if (!ociIPInfo) return;
  const subform = document.getElementById('oci-ip-subform');
  if (!subform) return;

  if (ociIPSubformMode === mode && !subform.hidden) {
    closeOCIIPSubform();
    return;
  }

  ociIPSubformMode = mode;
  const vnics = ociIPInfo.vnics || [];

  if (mode === 'ipv4') {
    const privWithoutPub = [];
    vnics.forEach(v => {
      (v.privateIps || []).forEach(p => {
        if (!p.publicIp) privWithoutPub.push({ vnic: v, priv: p });
      });
    });

    subform.innerHTML = `
      <div class="oci-ip-subform-title">
        <span>附加 IPv4</span>
        <button class="ghost-btn oci-ip-btn-sm" type="button" id="oci-subform-close">取消</button>
      </div>
      <div class="form-grid">
        <label class="field compact">
          <span>操作类型</span>
          <select id="oci-v4-mode">
            <option value="ephemeral_public" ${privWithoutPub.length === 0 ? 'disabled' : ''}>为已有私网 IP 分配临时公网 IP</option>
            <option value="secondary_private" ${privWithoutPub.length === 0 ? 'selected' : ''}>新增辅助私网 IP</option>
          </select>
        </label>
        <label class="field compact" id="oci-v4-priv-wrap">
          <span>目标私网 IP</span>
          <select id="oci-v4-priv-select">
            ${privWithoutPub.length > 0
              ? privWithoutPub.map(item => `<option value="${escapeAttr(item.priv.id)}" data-vnic-id="${escapeAttr(item.vnic.id)}">${escapeHtml(item.priv.ipAddress)} (${escapeHtml(item.vnic.displayName)})</option>`).join('')
              : '<option value="">所有私网 IP 均已绑定公网 IP</option>'}
          </select>
        </label>
        <label class="field compact" id="oci-v4-vnic-wrap" hidden>
          <span>目标 VNIC</span>
          <select id="oci-v4-vnic-select">
            ${vnics.map(v => `<option value="${escapeAttr(v.id)}">${escapeHtml(v.displayName)} (${escapeHtml(v.ipv4Cidr || '')})</option>`).join('')}
          </select>
        </label>
        <label class="field compact" id="oci-v4-ip-wrap" hidden>
          <span>指定私网 IP（留空自动分配）</span>
          <input id="oci-v4-custom-ip" type="text" placeholder="例如 10.0.0.182">
        </label>
      </div>
      <label class="switch-row compact" id="oci-v4-assign-pub-wrap" style="margin-top: 8px;" hidden>
        <input id="oci-v4-assign-pub" type="checkbox">
        <span>同时分配临时公网 IP</span>
      </label>
      <div class="oci-ip-subform-actions">
        <button class="primary-btn oci-ip-btn-sm" type="button" id="oci-v4-submit">确认附加 IPv4</button>
      </div>
    `;
    subform.hidden = false;
    syncOCIIPv4SubformFields();
    return;
  }

  if (mode === 'enable-ipv6') {
    const defaultVnic = vnics.find(v => !Array.isArray(v.ipv6CidrBlocks) || v.ipv6CidrBlocks.length === 0) || vnics[0];
    subform.innerHTML = buildOCIEnableIPv6SubformHTML(vnics, defaultVnic.id);
    subform.hidden = false;
    return;
  }

  if (mode === 'ipv6') {
    const v6Vnics = vnics.filter(v => Array.isArray(v.ipv6CidrBlocks) && v.ipv6CidrBlocks.length > 0);
    if (v6Vnics.length === 0) {
      setOCIIPModalMessage('当前子网尚未开启 IPv6，无法直接附加 IPv6，已为您打开「开启子网 IPv6」向导。', true);
      ociIPSubformMode = '';
      openOCIIPSubform('enable-ipv6');
      return;
    }
    const firstVnic = v6Vnics[0];

    subform.innerHTML = `
      <div class="oci-ip-subform-title">
        <span>附加 IPv6</span>
        <button class="ghost-btn oci-ip-btn-sm" type="button" id="oci-subform-close">取消</button>
      </div>
      <div class="form-grid">
        <label class="field compact">
          <span>目标 VNIC</span>
          <select id="oci-v6-vnic-select">
            ${v6Vnics.map(v => `<option value="${escapeAttr(v.id)}">${escapeHtml(v.displayName)}</option>`).join('')}
          </select>
        </label>
        <label class="field compact">
          <span>子网前缀</span>
          <select id="oci-v6-prefix-select">
            ${(firstVnic.ipv6CidrBlocks || []).map(cidr => `<option value="${escapeAttr(cidr)}">${escapeHtml(cidr)}</option>`).join('')}
          </select>
        </label>
        <label class="field compact">
          <span>CIDR 前缀长度（80 - 128）</span>
          <input id="oci-v6-prefix-len" type="number" min="80" max="128" value="128">
        </label>
        <label class="field compact">
          <span>分配方式</span>
          <select id="oci-v6-assign-mode">
            <option value="auto" selected>自动从前缀分配 IPv6 地址</option>
            <option value="manual">手动从前缀分配 IPv6 地址</option>
          </select>
        </label>
      </div>
      <label class="field compact" id="oci-v6-manual-wrap" style="margin-top: 8px;" hidden>
        <span>手动指定 IPv6 地址</span>
        <input id="oci-v6-manual-ip" type="text" placeholder="请输入所选子网前缀范围内的 IPv6 地址">
      </label>
      <div class="oci-ip-subform-actions">
        <button class="primary-btn oci-ip-btn-sm" type="button" id="oci-v6-submit">确认分配 IPv6</button>
      </div>
    `;
    subform.hidden = false;
    return;
  }

  if (mode === 'reserved') {
    const privWithoutPub = [];
    vnics.forEach(v => {
      (v.privateIps || []).forEach(p => {
        if (!p.publicIp) privWithoutPub.push({ vnic: v, priv: p });
      });
    });
    const reservedList = ociIPInfo.reservedIps || [];

    subform.innerHTML = `
      <div class="oci-ip-subform-title">
        <span>附加保留 IP</span>
        <button class="ghost-btn oci-ip-btn-sm" type="button" id="oci-subform-close">取消</button>
      </div>
      <div class="form-grid">
        <label class="field compact">
          <span>目标私网 IP（需先无公网 IP）</span>
          <select id="oci-res-priv-select">
            ${privWithoutPub.length > 0
              ? privWithoutPub.map(item => `<option value="${escapeAttr(item.priv.id)}">${escapeHtml(item.priv.ipAddress)} (${escapeHtml(item.vnic.displayName)})</option>`).join('')
              : '<option value="">无可用的空余私网 IP（请先删除/解绑现有公网 IP）</option>'}
          </select>
        </label>
        <label class="field compact">
          <span>来源</span>
          <select id="oci-res-mode">
            <option value="existing" ${reservedList.length === 0 ? 'disabled' : ''}>选择已有未绑定的保留 IP</option>
            <option value="new" ${reservedList.length === 0 ? 'selected' : ''}>新建并绑定保留 IP</option>
          </select>
        </label>
        <label class="field compact" id="oci-res-existing-wrap" ${reservedList.length === 0 ? 'hidden' : ''}>
          <span>选择保留 IP</span>
          <select id="oci-res-id-select">
            ${reservedList.map(r => `<option value="${escapeAttr(r.id)}">${escapeHtml(r.ipAddress)} (${escapeHtml(r.displayName || r.id)})</option>`).join('')}
          </select>
        </label>
        <label class="field compact" id="oci-res-new-wrap" ${reservedList.length === 0 ? '' : 'hidden'}>
          <span>新保留 IP 名称（可选）</span>
          <input id="oci-res-new-name" type="text" placeholder="例如 reserved-ip">
        </label>
      </div>
      <div class="oci-ip-subform-actions">
        <button class="primary-btn oci-ip-btn-sm" type="button" id="oci-res-submit" ${privWithoutPub.length === 0 ? 'disabled' : ''}>确认附加保留 IP</button>
      </div>
    `;
    subform.hidden = false;
  }
}

function syncOCIIPv4SubformFields() {
  const mode = document.getElementById('oci-v4-mode')?.value || 'ephemeral_public';
  const isSecondary = mode === 'secondary_private';
  const privWrap = document.getElementById('oci-v4-priv-wrap');
  const vnicWrap = document.getElementById('oci-v4-vnic-wrap');
  const ipWrap = document.getElementById('oci-v4-ip-wrap');
  const pubWrap = document.getElementById('oci-v4-assign-pub-wrap');
  if (privWrap) privWrap.hidden = isSecondary;
  if (vnicWrap) vnicWrap.hidden = !isSecondary;
  if (ipWrap) ipWrap.hidden = !isSecondary;
  if (pubWrap) pubWrap.hidden = !isSecondary;
}

async function submitOCIIPv4Attach() {
  if (!ociIPModalVM) return;
  const vm = ociIPModalVM;
  const mode = document.getElementById('oci-v4-mode')?.value || 'ephemeral_public';
  const updateDns = document.getElementById('oci-ip-update-dns-checkbox')?.checked === true;

  const payload = { mode, updateDns };
  if (mode === 'ephemeral_public') {
    const privSelect = document.getElementById('oci-v4-priv-select');
    const privId = privSelect?.value || '';
    if (!privId) {
      setOCIIPModalMessage('没有可分配公网 IP 的空余私网 IP。', true);
      return;
    }
    payload.privateIpId = privId;
    payload.vnicId = privSelect.selectedOptions?.[0]?.dataset?.vnicId || '';
  } else {
    payload.vnicId = document.getElementById('oci-v4-vnic-select')?.value || '';
    payload.ipAddress = document.getElementById('oci-v4-custom-ip')?.value?.trim() || '';
    payload.assignPublicIp = document.getElementById('oci-v4-assign-pub')?.checked === true;
  }

  setOCIIPModalMessage('正在附加 IPv4...', false);
  try {
    const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips/attach-ipv4`, {
      method: 'POST',
      body: payload
    });
    if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
    setOCIIPModalMessage('IPv4 附加成功。', false, true);
    closeOCIIPSubform();
    await loadOCIIPInfo();
    fetchVMs();
  } catch (err) {
    setOCIIPModalMessage(err.message, true);
    addLog(`附加 IPv4 失败：${err.message}`, 'error');
  }
}

async function submitOCIIPv6Attach() {
  if (!ociIPModalVM) return;
  const vm = ociIPModalVM;
  const vnicId = document.getElementById('oci-v6-vnic-select')?.value || '';
  const ipv6SubnetCidr = document.getElementById('oci-v6-prefix-select')?.value || '';
  const cidrPrefixLength = Number(document.getElementById('oci-v6-prefix-len')?.value || 128);
  const assignMode = document.getElementById('oci-v6-assign-mode')?.value || 'auto';
  const ipAddress = document.getElementById('oci-v6-manual-ip')?.value?.trim() || '';
  const updateDns = document.getElementById('oci-ip-update-dns-checkbox')?.checked === true;

  if (!Number.isInteger(cidrPrefixLength) || cidrPrefixLength < 80 || cidrPrefixLength > 128) {
    setOCIIPModalMessage('CIDR 前缀长度必须在 80 到 128 之间。', true);
    return;
  }
  if (assignMode === 'manual' && !ipAddress) {
    setOCIIPModalMessage('手动分配模式下请输入 IPv6 地址。', true);
    return;
  }

  setOCIIPModalMessage('正在分配 IPv6...', false);
  try {
    const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips/attach-ipv6`, {
      method: 'POST',
      body: { vnicId, ipv6SubnetCidr, cidrPrefixLength, assignMode, ipAddress, updateDns }
    });
    if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
    setOCIIPModalMessage('IPv6 分配成功。', false, true);
    closeOCIIPSubform();
    await loadOCIIPInfo();
    fetchVMs();
  } catch (err) {
    setOCIIPModalMessage(err.message, true);
    addLog(`分配 IPv6 失败：${err.message}`, 'error');
  }
}

async function submitOCIReservedAttach() {
  if (!ociIPModalVM) return;
  const vm = ociIPModalVM;
  const privateIpId = document.getElementById('oci-res-priv-select')?.value || '';
  const mode = document.getElementById('oci-res-mode')?.value || 'existing';
  const reservedPublicIpId = document.getElementById('oci-res-id-select')?.value || '';
  const displayName = document.getElementById('oci-res-new-name')?.value?.trim() || '';
  const updateDns = document.getElementById('oci-ip-update-dns-checkbox')?.checked === true;

  if (!privateIpId) {
    setOCIIPModalMessage('没有可用的空余私网 IP。', true);
    return;
  }

  setOCIIPModalMessage('正在附加保留 IP...', false);
  try {
    const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips/attach-reserved`, {
      method: 'POST',
      body: {
        privateIpId,
        reservedPublicIpId,
        createNew: mode === 'new',
        displayName,
        updateDns
      }
    });
    if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
    setOCIIPModalMessage('保留 IP 附加成功。', false, true);
    closeOCIIPSubform();
    await loadOCIIPInfo();
    fetchVMs();
  } catch (err) {
    setOCIIPModalMessage(err.message, true);
    addLog(`附加保留 IP 失败：${err.message}`, 'error');
  }
}

async function handleOCIIPActionClick(event) {
  if (!ociIPModalVM) return;
  const vm = ociIPModalVM;
  const updateDns = document.getElementById('oci-ip-update-dns-checkbox')?.checked === true;

  const changeV4Btn = event.target.closest('.oci-ip-change-v4-btn');
  if (changeV4Btn) {
    const privateIpId = changeV4Btn.dataset.privateId;
    if (!confirm('确认更换该私网 IP 绑定的临时公网 IPv4？')) return;
    changeV4Btn.disabled = true;
    setOCIIPModalMessage('正在更换公网 IPv4...', false);
    try {
      const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips/change-ipv4`, {
        method: 'POST',
        body: { privateIpId, updateDns }
      });
      if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
      setOCIIPModalMessage(`公网 IPv4 更换成功：${res.newIpAddress || ''}`, false, true);
      await loadOCIIPInfo();
      fetchVMs();
    } catch (err) {
      setOCIIPModalMessage(err.message, true);
      addLog(`更换 IPv4 失败：${err.message}`, 'error');
    } finally {
      changeV4Btn.disabled = false;
    }
    return;
  }

  const delV4Btn = event.target.closest('.oci-ip-del-v4-btn');
  if (delV4Btn) {
    const isReserved = delV4Btn.dataset.isReserved === 'true';
    const isPrimary = delV4Btn.dataset.isPrimary === 'true';
    const msg = isReserved
      ? '确认解绑该保留公网 IP？（保留 IP 本身不会被销毁）'
      : '确认删除该临时公网 IP？删除后该 IP 地址无法恢复。';
    if (!confirm(msg)) return;
    delV4Btn.disabled = true;
    setOCIIPModalMessage('正在处理...', false);
    try {
      const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips/delete-ipv4`, {
        method: 'POST',
        body: {
          privateIpId: delV4Btn.dataset.privateId,
          publicIpId: delV4Btn.dataset.publicId,
          deletePrivateIp: !isPrimary && confirm('是否同时删除该辅助私网 IP？（点“取消”则仅移除公网 IP）')
        }
      });
      if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
      setOCIIPModalMessage('操作完成。', false, true);
      await loadOCIIPInfo();
      fetchVMs();
    } catch (err) {
      setOCIIPModalMessage(err.message, true);
      addLog(`移除 IPv4 失败：${err.message}`, 'error');
    } finally {
      delV4Btn.disabled = false;
    }
    return;
  }

  const delPrivBtn = event.target.closest('.oci-ip-del-priv-btn');
  if (delPrivBtn) {
    if (!confirm('确认删除该辅助私网 IP？')) return;
    delPrivBtn.disabled = true;
    setOCIIPModalMessage('正在删除辅助私网 IP...', false);
    try {
      const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips/delete-ipv4`, {
        method: 'POST',
        body: {
          privateIpId: delPrivBtn.dataset.privateId,
          deletePrivateIp: true
        }
      });
      if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
      setOCIIPModalMessage('辅助私网 IP 已删除。', false, true);
      await loadOCIIPInfo();
      fetchVMs();
    } catch (err) {
      setOCIIPModalMessage(err.message, true);
      addLog(`删除辅助私网 IP 失败：${err.message}`, 'error');
    } finally {
      delPrivBtn.disabled = false;
    }
    return;
  }

  const changeV6Btn = event.target.closest('.oci-ip-change-v6-btn');
  if (changeV6Btn) {
    if (!confirm('IPv6 换 IP 将先删除旧 IPv6 地址，再自动重新分配新地址，确认继续？')) return;
    changeV6Btn.disabled = true;
    setOCIIPModalMessage('正在更换 IPv6（先删后建）...', false);
    try {
      const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips/change-ipv6`, {
        method: 'POST',
        body: {
          vnicId: changeV6Btn.dataset.vnicId,
          ipv6Id: changeV6Btn.dataset.ipv6Id,
          updateDns
        }
      });
      if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
      if (res.newIpAddress) ociIPSelectedDNSIPv6 = res.newIpAddress;
      setOCIIPModalMessage(`IPv6 更换成功：${res.newIpAddress || ''}`, false, true);
      await loadOCIIPInfo();
      fetchVMs();
    } catch (err) {
      setOCIIPModalMessage(err.message, true);
      addLog(`更换 IPv6 失败：${err.message}`, 'error');
    } finally {
      changeV6Btn.disabled = false;
    }
    return;
  }

  const delV6Btn = event.target.closest('.oci-ip-del-v6-btn');
  if (delV6Btn) {
    if (!confirm(`确认删除 IPv6 地址 ${delV6Btn.dataset.ip || ''}？`)) return;
    delV6Btn.disabled = true;
    setOCIIPModalMessage('正在删除 IPv6...', false);
    try {
      const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips/delete-ipv6`, {
        method: 'POST',
        body: { ipv6Id: delV6Btn.dataset.ipv6Id }
      });
      if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
      setOCIIPModalMessage('IPv6 已删除。', false, true);
      await loadOCIIPInfo();
      fetchVMs();
    } catch (err) {
      setOCIIPModalMessage(err.message, true);
      addLog(`删除 IPv6 失败：${err.message}`, 'error');
    } finally {
      delV6Btn.disabled = false;
    }
  }
}

async function manualUpdateOCIDNSFromModal() {
  if (!ociIPModalVM || !ociIPInfo) return;
  const vm = ociIPModalVM;
  const dnsBindings = ociIPInfo.dnsBindings || {};
  if (!dnsBindings.hasAny) return;

  // Determine primary public IPv4
  let ipv4 = '';
  for (const v of (ociIPInfo.vnics || [])) {
    for (const p of (v.privateIps || [])) {
      if (p.publicIp?.ipAddress) {
        ipv4 = p.publicIp.ipAddress;
        if (v.isPrimary && p.isPrimary) break;
      }
    }
    if (ipv4) break;
  }

  // Use the user-selected specific /128 IPv6
  const checkedRadio = document.querySelector('input[name="oci-dns-ipv6"]:checked');
  const ipv6 = checkedRadio ? checkedRadio.value : ociIPSelectedDNSIPv6;

  if (dnsBindings.hasAAAA && !ipv6) {
    setOCIIPModalMessage('实例绑定了 AAAA 记录，但当前没有可用的 128 位 IPv6 地址。', true);
    return;
  }
  if (!ipv4 && !ipv6) {
    setOCIIPModalMessage('当前没有可用于更新 DNS 的公网 IPv4 或 128 位 IPv6 地址。', true);
    return;
  }

  const btn = document.getElementById('oci-ip-manual-dns-btn');
  if (btn) btn.disabled = true;
  setOCIIPModalMessage('正在更新 DNS 记录...', false);
  try {
    const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/update-dns`, {
      method: 'POST',
      body: { ipv4, ipv6 }
    });
    if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
    setOCIIPModalMessage('DNS 更新完成。', false, true);
  } catch (err) {
    setOCIIPModalMessage(`更新 DNS 失败：${err.message}`, true);
    addLog(`更新 DNS 失败：${err.message}`, 'error');
  } finally {
    if (btn) btn.disabled = false;
  }
}

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('oci-ip-modal-close')?.addEventListener('click', closeOCIIPModal);
  document.getElementById('oci-ip-modal-done')?.addEventListener('click', closeOCIIPModal);
  document.getElementById('oci-ip-modal')?.addEventListener('click', e => {
    if (e.target.id === 'oci-ip-modal') closeOCIIPModal();
  });
  document.getElementById('oci-ip-modal-body')?.addEventListener('click', handleOCIIPActionClick);
  document.getElementById('oci-ip-modal-body')?.addEventListener('change', e => {
    if (e.target.name === 'oci-dns-ipv6') {
      ociIPSelectedDNSIPv6 = e.target.value;
    }
  });
  document.getElementById('oci-ip-attach-v4-btn')?.addEventListener('click', () => openOCIIPSubform('ipv4'));
  document.getElementById('oci-ip-attach-v6-btn')?.addEventListener('click', () => openOCIIPSubform('ipv6'));
  document.getElementById('oci-ip-enable-v6-btn')?.addEventListener('click', () => openOCIIPSubform('enable-ipv6'));
  document.getElementById('oci-ip-attach-reserved-btn')?.addEventListener('click', () => openOCIIPSubform('reserved'));
  document.getElementById('oci-ip-manual-dns-btn')?.addEventListener('click', manualUpdateOCIDNSFromModal);

  document.getElementById('oci-ip-subform')?.addEventListener('click', e => {
    if (e.target.id === 'oci-subform-close') closeOCIIPSubform();
    if (e.target.id === 'oci-v4-submit') submitOCIIPv4Attach();
    if (e.target.id === 'oci-v6-submit') submitOCIIPv6Attach();
    if (e.target.id === 'oci-enable-v6-submit') submitOCIEnableSubnetIPv6();
    if (e.target.id === 'oci-res-submit') submitOCIReservedAttach();
  });
  document.getElementById('oci-ip-subform')?.addEventListener('change', e => {
    if (e.target.id === 'oci-v4-mode') syncOCIIPv4SubformFields();
    if (e.target.id === 'oci-enable-v6-vnic' && ociIPInfo) {
      const sf = document.getElementById('oci-ip-subform');
      if (sf) sf.innerHTML = buildOCIEnableIPv6SubformHTML(ociIPInfo.vnics || [], e.target.value);
    }
    if (e.target.id === 'oci-v6-vnic-select' && ociIPInfo) {
      const v = (ociIPInfo.vnics || []).find(item => item.id === e.target.value);
      const prefixSelect = document.getElementById('oci-v6-prefix-select');
      if (prefixSelect && v) {
        prefixSelect.innerHTML = (v.ipv6CidrBlocks || []).map(c => `<option value="${escapeAttr(c)}">${escapeHtml(c)}</option>`).join('');
      }
    }
    if (e.target.id === 'oci-v6-assign-mode') {
      const wrap = document.getElementById('oci-v6-manual-wrap');
      if (wrap) wrap.hidden = e.target.value !== 'manual';
    }
    if (e.target.id === 'oci-res-mode') {
      const isNew = e.target.value === 'new';
      const exWrap = document.getElementById('oci-res-existing-wrap');
      const newWrap = document.getElementById('oci-res-new-wrap');
      if (exWrap) exWrap.hidden = isNew;
      if (newWrap) newWrap.hidden = !isNew;
    }
  });
});


function buildOCIEnableIPv6SubformHTML(vnics, targetVnicId) {
  const targetVnic = vnics.find(v => v.id === targetVnicId) || vnics[0];
  const setup = targetVnic?.ipv6Setup || {};
  const hasSub = Array.isArray(targetVnic?.ipv6CidrBlocks) && targetVnic.ipv6CidrBlocks.length > 0;
  const suggestedHex = setup.suggestedHex || '7E';
  const usedHexes = Array.isArray(setup.usedHexes) ? setup.usedHexes : [];
  const vcnText = setup.vcnHasIpv6
    ? `已开启 (${(setup.vcnIpv6Cidrs || []).join(', ')})`
    : '未开启（将自动为 VCN 分配 Oracle IPv6 /56 前缀）';
  const subText = hasSub
    ? `已开启 (${(targetVnic.ipv6CidrBlocks || []).join(', ')})`
    : '未开启（需输入 2 位十六进制字符分配 /64 子网段）';
  const rtText = setup.routeHasIpv6Default
    ? '已配置 ::/0 默认 IPv6 路由'
    : (setup.hasInternetGateway ? '未配置 ::/0（检测到 IGW，可自动添加）' : '未配置 ::/0（未检测到 IGW，请检查网关）');
  const slText = setup.securityListHasIpv6Egress
    ? '已存在 ::/0 (所有协议) 出站规则'
    : '未检测到 ::/0 出站规则（默认不自动修改，请前往「防火墙管理」添加对应出站/入站规则）';
  return `
    <div class="oci-ip-subform-title">
      <span>开启子网 IPv6（VCN /56 → 子网 /64 → 路由表）</span>
      <button class="ghost-btn oci-ip-btn-sm" type="button" id="oci-subform-close">取消</button>
    </div>
    <div class="field-hint" style="margin-bottom: 10px; line-height: 1.65;">
      <div>1. VCN IPv6 (/56)：${setup.vcnHasIpv6 ? '✅' : '⏳'} ${escapeHtml(vcnText)}</div>
      <div>2. 子网 IPv6 (/64)：${hasSub ? '✅' : '⏳'} ${escapeHtml(subText)}</div>
      <div>3. 路由表 (::/0)：${setup.routeHasIpv6Default ? '✅' : '⏳'} ${escapeHtml(rtText)}</div>
      <div>4. 安全列表（防火墙）：${setup.securityListHasIpv6Egress ? '✅' : 'ℹ️'} ${escapeHtml(slText)}</div>
    </div>
    <div class="form-grid">
      <label class="field compact">
        <span>目标 VNIC</span>
        <select id="oci-enable-v6-vnic">
          ${vnics.map(v => `<option value="${escapeAttr(v.id)}" ${v.id === targetVnic.id ? 'selected' : ''}>${escapeHtml(v.displayName)}</option>`).join('')}
        </select>
      </label>
      <label class="field compact">
        <span>子网 /64 前缀 Hex（00-FF）</span>
        <input id="oci-enable-v6-hex" type="text" maxlength="2" value="${escapeAttr(suggestedHex)}" placeholder="例如 7E" ${hasSub ? 'disabled' : ''}>
        ${usedHexes.length ? `<small class="field-hint">同 VCN 已占用 Hex：${escapeHtml(usedHexes.join(', '))}</small>` : ''}
      </label>
    </div>
    <div style="display:flex; flex-direction:column; gap:6px; margin-top:8px;">
      <label style="display:flex; align-items:center; gap:6px; font-size:13px;">
        <input id="oci-enable-v6-add-route" type="checkbox" ${setup.routeHasIpv6Default ? '' : 'checked'}>
        <span>自动在子网路由表添加默认 IPv6 路由 (::/0 → 互联网网关 IGW)</span>
      </label>
      <label style="display:flex; align-items:center; gap:6px; font-size:13px;">
        <input id="oci-enable-v6-add-sl-egress" type="checkbox">
        <span>可选：在子网默认安全列表自动添加出站 ::/0 (所有协议)</span>
      </label>
      <div class="field-hint">提示：如需允许 IPv6 出站或放行入站端口，请前往左侧「防火墙管理」添加对应规则。</div>
    </div>
    <div class="oci-ip-subform-actions">
      <button class="primary-btn oci-ip-btn-sm" type="button" id="oci-enable-v6-submit">${hasSub ? '补全路由/配置' : '确认开启子网 IPv6'}</button>
    </div>
  `;
}

async function submitOCIEnableSubnetIPv6() {
  if (!ociIPModalVM) return;
  const vm = ociIPModalVM;
  const vnicId = document.getElementById('oci-enable-v6-vnic')?.value || '';
  const hexInput = document.getElementById('oci-enable-v6-hex');
  const hexByte = hexInput?.value?.trim() || '';
  const addDefaultRoute = Boolean(document.getElementById('oci-enable-v6-add-route')?.checked);
  const addSecurityListEgress = Boolean(document.getElementById('oci-enable-v6-add-sl-egress')?.checked);

  if (!hexInput?.disabled && !/^[0-9a-fA-F]{2}$/.test(hexByte)) {
    setOCIIPModalMessage('请输入有效的 2 位十六进制字符（00 - FF），例如 7E。', true);
    return;
  }

  const btn = document.getElementById('oci-enable-v6-submit');
  if (btn) btn.disabled = true;
  setOCIIPModalMessage('正在为 VCN/子网开启 IPv6...', false);
  try {
    const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips/enable-ipv6`, {
      method: 'POST',
      body: { vnicId, hexByte, addDefaultRoute, addSecurityListEgress }
    });
    if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
    setOCIIPModalMessage(`子网 IPv6 已配置：${res.subnetCidr || ''}。请按需前往「防火墙管理」检查 IPv6 出站/入站规则。`, false, true);
    closeOCIIPSubform();
    await loadOCIIPInfo();
    const hasSubV6 = (ociIPInfo?.vnics || []).some(v => Array.isArray(v.ipv6CidrBlocks) && v.ipv6CidrBlocks.length > 0);
    if (hasSubV6) {
      openOCIIPSubform('ipv6');
    }
  } catch (err) {
    setOCIIPModalMessage(err.message, true);
    addLog(`开启子网 IPv6 失败：${err.message}`, 'error');
  } finally {
    if (btn) btn.disabled = false;
  }
}

// ==================== OCI Horizontal Workspace & 7 Tabs ====================

let currentOCITab = 'instances';
let lastOCIWorkspaceAccount = '';
let currentOCICreateTasks = [];
let ociCreateTasksPollTimer = null;
let ociCreateTaskOptionsCache = null;

function updateOCIWorkspaceVisibility() {
  const tabsBar = document.getElementById('oci-workspace-tabs');
  const isOCI = selectedAccount && selectedAccount.provider === 'oci';

  if (ociCreateTasksPollTimer) {
    clearInterval(ociCreateTasksPollTimer);
    ociCreateTasksPollTimer = null;
  }

  if (tabsBar) tabsBar.hidden = !isOCI;

  if (!isOCI) {
    lastOCIWorkspaceAccount = '';
    document.querySelectorAll('.oci-tab-pane').forEach(el => { el.hidden = true; });
    if (els.vmList) els.vmList.hidden = false;
    return;
  }

  if (selectedAccount.account !== lastOCIWorkspaceAccount) {
    lastOCIWorkspaceAccount = selectedAccount.account;
    ociCreateTaskOptionsCache = null;
    ociOSCurrentBucket = '';
    ociOSCurrentPrefix = '';
    const editInput = document.getElementById('oci-task-edit-id');
    if (editInput) editInput.value = '';
  }

  switchOCITab(currentOCITab || 'instances');
  ociCreateTasksPollTimer = setInterval(() => {
    if (!selectedAccount || selectedAccount.provider !== 'oci') return;
    if (currentOCITab === 'tasks' && currentOCICreateTasks.some(t => t.status === 'running')) {
      loadOCICreateTasks(true);
    }
  }, 5000);
}

function switchOCITab(tab) {
  currentOCITab = tab || 'instances';
  document.querySelectorAll('.oci-ws-tab').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.ociTab === currentOCITab);
  });

  const paneMap = {
    overview: 'oci-tab-overview',
    monitoring: 'oci-tab-monitoring',
    users: 'oci-tab-users',
    create: 'oci-tab-create',
    tasks: 'oci-tab-tasks',
    storage: 'oci-tab-storage'
  };

  Object.values(paneMap).forEach(id => {
    const el = document.getElementById(id);
    if (el) el.hidden = true;
  });

  if (els.vmList) {
    els.vmList.hidden = currentOCITab !== 'instances';
  }

  if (paneMap[currentOCITab]) {
    const target = document.getElementById(paneMap[currentOCITab]);
    if (target) target.hidden = false;
  }

  if (!selectedAccount || selectedAccount.provider !== 'oci') return;

  if (currentOCITab === 'overview') {
    loadOCIOverview();
  } else if (currentOCITab === 'monitoring') {
    const lbl = document.getElementById('dt-monitor-account-label');
    if (lbl) lbl.textContent = selectedAccount.account;
    loadDTMonitorStatus();
  } else if (currentOCITab === 'users') {
    loadOCIIAM();
  } else if (currentOCITab === 'create') {
    const editId = document.getElementById('oci-task-edit-id')?.value || '';
    if (!editId) {
      prepareOCICreatePage('amd_free', false);
    }
  } else if (currentOCITab === 'tasks') {
    const lbl = document.getElementById('oci-tasks-account-label');
    if (lbl) lbl.textContent = selectedAccount.account;
    loadOCICreateTasks();
  } else if (currentOCITab === 'storage') {
    loadOCIObjectStorage();
  }
}

// ==================== Tab 1: Overview (Cost & Quotas) ====================

async function loadOCIOverview() {
  if (!selectedAccount || selectedAccount.provider !== 'oci') return;
  const acc = selectedAccount.account;
  const metaEl = document.getElementById('oci-overview-meta');
  const box = document.getElementById('oci-overview-content');
  if (metaEl) metaEl.textContent = `账号：${acc} · 正在加载成本与配额...`;
  if (box) box.innerHTML = '<div class="empty-state compact">正在从 OCI 读取概述数据...</div>';

  try {
    const data = await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/overview`);
    if (!selectedAccount || selectedAccount.account !== acc) return;
    renderOCIOverview(data);
  } catch (err) {
    if (box) box.innerHTML = `<div class="empty-state compact error">加载概述失败：${escapeHtml(err.message)}</div>`;
  }
}

function renderOCIOverview(data) {
  const metaEl = document.getElementById('oci-overview-meta');
  const box = document.getElementById('oci-overview-content');
  if (!box) return;

  if (metaEl) {
    metaEl.textContent = `租户：${data.tenancyName || data.tenancyId || '-'} | 当前区域：${data.region || '-'} | Home Region：${data.homeRegion || '-'}`;
  }

  let costCardHTML = '';
  if (data.costStatus === 'ok') {
    const items = Array.isArray(data.costByService) ? data.costByService : [];
    const breakdown = items.length > 0
      ? `<div style="margin-top: 8px; font-size: 12px; color: var(--muted);">${items.slice(0, 6).map(i => `${escapeHtml(i.service)}: ${Number(i.amount || 0).toFixed(2)} ${escapeHtml(data.currency || 'USD')}`).join(' · ')}</div>`
      : '<div style="margin-top: 6px; font-size: 12px; color: var(--muted);">当月暂无计费记录（免费额度内）</div>';
    costCardHTML = `
      <div class="oci-overview-card">
        <span class="stat-label">当月预估费用 (${escapeHtml(data.periodStart)} ~ ${escapeHtml(data.periodEnd)})</span>
        <strong>${Number(data.totalCost || 0).toFixed(2)} ${escapeHtml(data.currency || 'USD')}</strong>
        ${breakdown}
      </div>
    `;
  } else {
    costCardHTML = `
      <div class="oci-overview-card">
        <span class="stat-label">当月成本 (${escapeHtml(data.periodStart)} ~ ${escapeHtml(data.periodEnd)})</span>
        <strong style="font-size: 15px; color: var(--muted);">${data.costStatus === 'unauthorized' ? '无成本查询权限' : '查询异常'}</strong>
        <div style="margin-top: 6px; font-size: 12px; color: var(--muted);">${escapeHtml(data.costMessage || '')}</div>
      </div>
    `;
  }

  let quotaHTML = '';
  if (data.limitsStatus === 'ok') {
    const quotas = Array.isArray(data.quotas) ? data.quotas : [];
    quotaHTML = quotas.length > 0
      ? `<div class="oci-quota-grid">${quotas.map(q => `
          <div class="oci-quota-card">
            <div class="oci-quota-top">
              <span>${escapeHtml(q.label || q.name)}</span>
              <span class="oci-quota-nums">已用 ${formatOCIAmount(q.used)} / 配额 ${formatOCIAmount(q.limit)}</span>
            </div>
            <div class="oci-quota-nums">剩余可用：<strong>${formatOCIAmount(q.available)}</strong>${q.availabilityDomain ? ` (${escapeHtml(q.availabilityDomain)})` : ''}</div>
          </div>
        `).join('')}</div>`
      : '<div class="empty-state compact">未查询到关键配额数据。</div>';
  } else {
    quotaHTML = `<div class="empty-state compact">${escapeHtml(data.limitsMessage || '暂无配额数据')}</div>`;
  }

  box.innerHTML = `
    <div class="oci-overview-grid">
      ${costCardHTML}
      <div class="oci-overview-card">
        <span class="stat-label">当前区域 / Home Region</span>
        <strong style="font-size: 17px;">${escapeHtml(data.region || '-')} / ${escapeHtml(data.homeRegion || '-')}</strong>
        <div style="margin-top: 6px; font-size: 12px; color: var(--muted);">区间：<code>${escapeHtml((data.compartmentId || '-').slice(0, 36))}...</code></div>
      </div>
    </div>
    <h4 style="margin: 8px 0 10px;">关键服务配额 (${escapeHtml(data.region || '')})</h4>
    ${quotaHTML}
  `;
}

// ==================== Tab 2: Terminate OCI Instance Modal ====================

let ociTerminateVM = null;

function openOCITerminateModal(vm) {
  ociTerminateVM = vm;
  const modal = document.getElementById('oci-terminate-modal');
  if (!modal) return;

  const name = vm.name || vm.id || '';
  document.getElementById('oci-terminate-target-name').textContent = name;
  document.getElementById('oci-terminate-expected-name').textContent = name;
  const input = document.getElementById('oci-terminate-confirm-input');
  if (input) input.value = '';
  const confirmBtn = document.getElementById('oci-terminate-confirm');
  if (confirmBtn) confirmBtn.disabled = true;
  const msgEl = document.getElementById('oci-terminate-message');
  if (msgEl) {
    msgEl.textContent = '';
    msgEl.className = 'form-message';
  }
  const radios = document.querySelectorAll('input[name="oci-terminate-boot"]');
  if (radios[0]) radios[0].checked = true;
  modal.hidden = false;
}

function closeOCITerminateModal() {
  const modal = document.getElementById('oci-terminate-modal');
  if (modal) modal.hidden = true;
  ociTerminateVM = null;
}

async function confirmOCITerminate() {
  if (!ociTerminateVM) return;
  const vm = ociTerminateVM;
  const inputVal = document.getElementById('oci-terminate-confirm-input')?.value?.trim() || '';
  if (inputVal !== (vm.name || vm.id)) return;

  const preserveBootVolume = document.querySelector('input[name="oci-terminate-boot"]:checked')?.value === 'true';
  const btn = document.getElementById('oci-terminate-confirm');
  const msgEl = document.getElementById('oci-terminate-message');
  if (btn) btn.disabled = true;
  if (msgEl) {
    msgEl.textContent = '正在提交删除请求...';
    msgEl.className = 'form-message';
  }

  try {
    const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/terminate`, {
      method: 'POST',
      body: { preserveBootVolume }
    });
    addLog(`${res.message || '已删除 OCI 实例'}：${vm.name}`, 'success');
    closeOCITerminateModal();
    setTimeout(() => fetchVMs(), 2500);
  } catch (err) {
    if (msgEl) {
      msgEl.textContent = err.message;
      msgEl.className = 'form-message error';
    }
    if (btn) btn.disabled = false;
  }
}

// ==================== Tab 4: IAM Users, Groups & API Keys ====================

let ociIAMData = null;
let ociIAMPolicyScope = '';
let ociAPIKeyModalUser = null;
let ociGeneratedPrivateKeyPEM = '';

const OCI_POLICY_TEMPLATES = [
  {
    id: 'tenant_admin',
    usecase: 'account',
    label: '租户全权管理 (Let tenant admins manage all resources)',
    tenancyOnly: true,
    build: (g) => [`Allow group ${g} to manage all-resources in tenancy`]
  },
  {
    id: 'compartment_admin',
    usecase: 'account',
    label: '区间全权管理 (Let compartment admins manage all resources)',
    build: (g, loc) => [`Allow group ${g} to manage all-resources in ${loc}`]
  },
  {
    id: 'read_all',
    usecase: 'account',
    label: '全资源只读审计 (Let users read all resources)',
    build: (g, loc) => [`Allow group ${g} to read all-resources in ${loc}`]
  },
  {
    id: 'inspect_all',
    usecase: 'account',
    label: '仅查看资源列表与元数据 (Let users inspect all resources)',
    build: (g, loc) => [`Allow group ${g} to inspect all-resources in ${loc}`]
  },
  {
    id: 'iam_group_admin',
    usecase: 'account',
    label: 'IAM 用户与用户组管理 (Let user/group admins manage users & groups)',
    tenancyOnly: true,
    build: (g) => [
      `Allow group ${g} to manage users in tenancy`,
      `Allow group ${g} to manage groups in tenancy`
    ]
  },
  {
    id: 'compute_launch',
    usecase: 'compute',
    label: '启动与管理计算实例 (Let users launch compute instances)',
    build: (g, loc) => [
      `Allow group ${g} to manage instance-family in ${loc}`,
      `Allow group ${g} to use volume-family in ${loc}`,
      `Allow group ${g} to use virtual-network-family in ${loc}`
    ]
  },
  {
    id: 'compute_only',
    usecase: 'compute',
    label: '仅管理计算实例家族 (Let instance admins manage compute instances)',
    build: (g, loc) => [`Allow group ${g} to manage instance-family in ${loc}`]
  },
  {
    id: 'network_admin',
    usecase: 'network',
    label: '管理云网络 (Let network admins manage a cloud network)',
    build: (g, loc) => [`Allow group ${g} to manage virtual-network-family in ${loc}`]
  },
  {
    id: 'volume_admin',
    usecase: 'storage',
    label: '管理块存储卷与备份 (Let volume admins manage block volumes & backups)',
    build: (g, loc) => [`Allow group ${g} to manage volume-family in ${loc}`]
  },
  {
    id: 'object_admin',
    usecase: 'storage',
    label: '管理对象存储 Bucket 与对象 (Let object storage admins manage buckets & objects)',
    build: (g, loc) => [
      `Allow group ${g} to manage buckets in ${loc}`,
      `Allow group ${g} to manage objects in ${loc}`
    ]
  },
  {
    id: 'object_rw',
    usecase: 'storage',
    label: '读写对象存储文件 (Let object storage users read/write objects)',
    build: (g, loc) => [
      `Allow group ${g} to read buckets in ${loc}`,
      `Allow group ${g} to manage objects in ${loc}`
    ]
  },
  {
    id: 'adb_admin',
    usecase: 'database',
    label: '管理自治数据库 (Let database admins manage autonomous databases)',
    build: (g, loc) => [`Allow group ${g} to manage autonomous-database-family in ${loc}`]
  }
];

async function loadOCIIAM() {
  if (!selectedAccount || selectedAccount.provider !== 'oci') return;
  const acc = selectedAccount.account;
  const container = document.getElementById('oci-iam-content');
  const msgEl = document.getElementById('oci-iam-message');
  if (msgEl) { msgEl.textContent = ''; msgEl.className = 'form-message'; }
  if (container) container.innerHTML = '<div class="empty-state compact">正在加载 IAM 用户、用户组与策略...</div>';

  try {
    const data = await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/iam`);
    if (!selectedAccount || selectedAccount.account !== acc) return;
    ociIAMData = data;
    const compartments = Array.isArray(data?.compartments) ? data.compartments : [];
    const validScope = ociIAMPolicyScope && compartments.some(c => c.id === ociIAMPolicyScope);
    if (!validScope) {
      ociIAMPolicyScope = data?.tenancyId || compartments[0]?.id || '';
    } else if (data?.tenancyId && ociIAMPolicyScope !== data.tenancyId) {
      const pRes = await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/iam/policies?compartmentId=${encodeURIComponent(ociIAMPolicyScope)}`);
      if (!selectedAccount || selectedAccount.account !== acc) return;
      ociIAMData.policies = Array.isArray(pRes?.policies) ? pRes.policies : [];
    }
    renderOCIIAM(ociIAMData);
  } catch (err) {
    if (container) container.innerHTML = `<div class="empty-state compact error">加载 IAM 失败：${escapeHtml(err.message)}</div>`;
  }
}

async function loadOCIIAMPolicies(compartmentId) {
  if (!selectedAccount || selectedAccount.provider !== 'oci') return;
  const acc = selectedAccount.account;
  ociIAMPolicyScope = compartmentId || ociIAMData?.tenancyId || '';
  const msgEl = document.getElementById('oci-iam-message');
  try {
    const data = await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/iam/policies?compartmentId=${encodeURIComponent(ociIAMPolicyScope)}`);
    if (!selectedAccount || selectedAccount.account !== acc) return;
    if (ociIAMData) {
      ociIAMData.policies = Array.isArray(data?.policies) ? data.policies : [];
      ociIAMData.policyError = '';
      renderOCIIAM(ociIAMData);
    }
  } catch (err) {
    if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
  }
}

function getOCICompartmentName(compId) {
  const compartments = Array.isArray(ociIAMData?.compartments) ? ociIAMData.compartments : [];
  const found = compartments.find(c => c.id === compId);
  return found ? found.name : (compId || '-');
}

function renderOCIIAM(data) {
  const container = document.getElementById('oci-iam-content');
  if (!container) return;
  const users = Array.isArray(data?.users) ? data.users : [];
  const groups = Array.isArray(data?.groups) ? data.groups : [];
  const compartments = Array.isArray(data?.compartments) ? data.compartments : [];
  const policies = Array.isArray(data?.policies) ? data.policies : [];
  const metaEl = document.getElementById('oci-iam-meta');
  if (metaEl) metaEl.textContent = `Home Region：${data?.homeRegion || '-'} · 共 ${users.length} 个用户，${groups.length} 个用户组，${policies.length} 条策略`;

  const userGroupSelect = document.getElementById('oci-iam-user-group');
  if (userGroupSelect) {
    userGroupSelect.innerHTML = '<option value="">暂不加入用户组</option>' +
      groups.map(g => `<option value="${escapeAttr(g.id)}">${escapeHtml(g.name)}</option>`).join('');
  }

  const groupOptions = groups.map(g => `<option value="${escapeAttr(g.id)}">${escapeHtml(g.name)}</option>`).join('');
  const scopeOptions = compartments.map(c => `<option value="${escapeAttr(c.id)}" ${c.id === ociIAMPolicyScope ? 'selected' : ''}>${escapeHtml(c.name)}</option>`).join('');

  container.innerHTML = `
    <h4 style="margin: 8px 0;">用户列表</h4>
    ${users.length === 0 ? '<div class="empty-state compact">暂无用户</div>' : `
      <div class="gcp-fw-table-container">
        <table class="gcp-fw-table">
          <thead>
            <tr>
              <th>用户名</th>
              <th>状态 / MFA</th>
              <th>所属用户组</th>
              <th style="text-align: right;">操作</th>
            </tr>
          </thead>
          <tbody>
            ${users.map(u => `
              <tr>
                <td>
                  <strong>${escapeHtml(u.name)}</strong>
                  ${u.email ? `<div style="font-size:11px; color:var(--muted);">${escapeHtml(u.email)}</div>` : ''}
                </td>
                <td>
                  <span class="oci-ip-badge attached">${escapeHtml(u.lifecycleState || 'ACTIVE')}</span>
                  ${u.isMfaActivated ? '<span class="oci-ip-badge primary">MFA</span>' : ''}
                </td>
                <td>
                  <div style="display:flex; flex-wrap:wrap; gap:6px; align-items:center;">
                    ${(u.groups || []).map(g => `
                      <span class="fw-tag-pill">
                        ${escapeHtml(g.groupName)}
                        <button type="button" class="oci-iam-remove-mem-btn" data-membership-id="${escapeAttr(g.membershipId)}" title="移出该组" style="background:none;border:0;color:var(--red);cursor:pointer;margin-left:4px;">×</button>
                      </span>
                    `).join('')}
                    ${groups.length > 0 ? `
                      <select class="oci-iam-add-group-sel" data-user-id="${escapeAttr(u.id)}" style="height:28px;font-size:12px;background:var(--panel);color:var(--text);border:1px solid var(--panel-border);border-radius:4px;">
                        <option value="">+ 加入组...</option>
                        ${groupOptions}
                      </select>
                    ` : ''}
                  </div>
                </td>
                <td style="text-align:right;">
                  <div class="fw-actions-cell" style="justify-content:flex-end;">
                    <button class="secondary-btn oci-ip-btn-sm oci-iam-keys-btn" type="button" data-user-id="${escapeAttr(u.id)}" data-user-name="${escapeAttr(u.name)}">API Key</button>
                    <button class="ghost-btn oci-ip-btn-sm oci-iam-pwd-btn" type="button" data-user-id="${escapeAttr(u.id)}" data-user-name="${escapeAttr(u.name)}">重置密码</button>
                    <button class="ghost-btn oci-ip-btn-sm oci-ip-btn-danger oci-iam-del-user-btn" type="button" data-user-id="${escapeAttr(u.id)}" data-user-name="${escapeAttr(u.name)}">删除</button>
                  </div>
                </td>
              </tr>
            `).join('')}
          </tbody>
        </table>
      </div>
    `}

    <h4 style="margin: 18px 0 8px;">用户组列表</h4>
    ${groups.length === 0 ? '<div class="empty-state compact">暂无用户组</div>' : `
      <div class="gcp-fw-table-container">
        <table class="gcp-fw-table">
          <thead>
            <tr>
              <th>组名称</th>
              <th>描述</th>
              <th>OCID</th>
              <th style="text-align: right;">操作</th>
            </tr>
          </thead>
          <tbody>
            ${groups.map(g => `
              <tr>
                <td><strong>${escapeHtml(g.name)}</strong></td>
                <td>${escapeHtml(g.description || '-')}</td>
                <td><code style="font-size:11px; color:var(--muted);">${escapeHtml((g.id || '').slice(0, 36))}...</code></td>
                <td style="text-align:right;">
                  <div class="fw-actions-cell" style="justify-content:flex-end;">
                    <button class="secondary-btn oci-ip-btn-sm oci-iam-group-policy-btn" type="button" data-group-name="${escapeAttr(g.name)}">分配策略</button>
                    <button class="ghost-btn oci-ip-btn-sm oci-ip-btn-danger oci-iam-del-group-btn" type="button" data-group-id="${escapeAttr(g.id)}" data-group-name="${escapeAttr(g.name)}">删除组</button>
                  </div>
                </td>
              </tr>
            `).join('')}
          </tbody>
        </table>
      </div>
    `}

    <div class="oci-iam-section-head">
      <h4>策略管理 (Policies)</h4>
      <div class="oci-iam-policy-tools">
        <span style="font-size:12px; color:var(--muted);">所属区间：</span>
        <select id="oci-iam-policy-scope">${scopeOptions}</select>
        <button id="oci-iam-inline-new-policy-btn" class="secondary-btn oci-ip-btn-sm" type="button">+ 新建策略</button>
      </div>
    </div>
    ${data?.policyError ? `<div class="empty-state compact error" style="margin-bottom:10px;">${escapeHtml(data.policyError)}</div>` : ''}
    ${policies.length === 0 ? '<div class="empty-state compact">当前区间暂无策略，点击「+ 新建策略」或用户组行的「分配策略」快速创建。</div>' : `
      <div class="gcp-fw-table-container">
        <table class="gcp-fw-table">
          <thead>
            <tr>
              <th>策略名称</th>
              <th>所属区间</th>
              <th>策略语句 (Statements)</th>
              <th style="text-align: right;">操作</th>
            </tr>
          </thead>
          <tbody>
            ${policies.map(p => `
              <tr>
                <td>
                  <strong>${escapeHtml(p.name)}</strong>
                  ${p.description ? `<div style="font-size:11px; color:var(--muted);">${escapeHtml(p.description)}</div>` : ''}
                </td>
                <td><span class="oci-ip-badge">${escapeHtml(getOCICompartmentName(p.compartmentId))}</span></td>
                <td>
                  <div class="oci-policy-stmts">
                    ${(p.statements || []).map(s => `<code class="oci-policy-stmt-code">${escapeHtml(s)}</code>`).join('')}
                  </div>
                </td>
                <td style="text-align:right;">
                  <div class="fw-actions-cell" style="justify-content:flex-end;">
                    <button class="secondary-btn oci-ip-btn-sm oci-iam-edit-policy-btn" type="button" data-policy-id="${escapeAttr(p.id)}">编辑</button>
                    <button class="ghost-btn oci-ip-btn-sm oci-ip-btn-danger oci-iam-del-policy-btn" type="button" data-policy-id="${escapeAttr(p.id)}" data-policy-name="${escapeAttr(p.name)}">删除</button>
                  </div>
                </td>
              </tr>
            `).join('')}
          </tbody>
        </table>
      </div>
    `}
  `;
}

function populateOCIPolicyModalSelects(presetGroupName = '') {
  const groups = Array.isArray(ociIAMData?.groups) ? ociIAMData.groups : [];
  const compartments = Array.isArray(ociIAMData?.compartments) ? ociIAMData.compartments : [];

  const compSelect = document.getElementById('oci-policy-compartment');
  if (compSelect) {
    compSelect.innerHTML = compartments.map(c =>
      `<option value="${escapeAttr(c.id)}">${escapeHtml(c.name)}</option>`
    ).join('');
    if (ociIAMPolicyScope && compartments.some(c => c.id === ociIAMPolicyScope)) {
      compSelect.value = ociIAMPolicyScope;
    }
  }

  const groupOptionsHTML = groups.length > 0
    ? groups.map(g => `<option value="${escapeAttr(g.name)}">${escapeHtml(g.name)}</option>`).join('')
    : '<option value="Administrators">Administrators</option>';
  ['oci-policy-tpl-group', 'oci-policy-stmt-group'].forEach(id => {
    const el = document.getElementById(id);
    if (el) {
      el.innerHTML = groupOptionsHTML;
      if (presetGroupName && groups.some(g => g.name === presetGroupName)) {
        el.value = presetGroupName;
      }
    }
  });

  const locOptionsHTML = '<option value="tenancy">tenancy (整个租户)</option>' +
    compartments.filter(c => !c.isRoot).map(c =>
      `<option value="${escapeAttr(`compartment ${c.name}`)}">compartment ${escapeHtml(c.name)}</option>`
    ).join('');
  ['oci-policy-tpl-location', 'oci-policy-stmt-location'].forEach(id => {
    const el = document.getElementById(id);
    if (el) el.innerHTML = locOptionsHTML;
  });

  renderOCIPolicyTemplateOptions();
}

function renderOCIPolicyTemplateOptions() {
  const usecase = document.getElementById('oci-policy-usecase')?.value || 'all';
  const tplSelect = document.getElementById('oci-policy-template');
  if (!tplSelect) return;
  const list = OCI_POLICY_TEMPLATES.filter(t => usecase === 'all' || t.usecase === usecase);
  tplSelect.innerHTML = list.map(t => `<option value="${escapeAttr(t.id)}">${escapeHtml(t.label)}</option>`).join('');
  updateOCIPolicyTemplatePreview();
}

function buildOCIPolicyTemplateStatements() {
  const tplId = document.getElementById('oci-policy-template')?.value || OCI_POLICY_TEMPLATES[0].id;
  const tpl = OCI_POLICY_TEMPLATES.find(t => t.id === tplId) || OCI_POLICY_TEMPLATES[0];
  const groupName = document.getElementById('oci-policy-tpl-group')?.value || 'Administrators';
  const locationEl = document.getElementById('oci-policy-tpl-location');
  if (locationEl) {
    if (tpl.tenancyOnly) {
      locationEl.value = 'tenancy';
      locationEl.disabled = true;
    } else {
      locationEl.disabled = false;
    }
  }
  const loc = locationEl?.value || 'tenancy';
  return tpl.build(groupName, loc);
}

function updateOCIPolicyTemplatePreview() {
  const preview = document.getElementById('oci-policy-tpl-preview');
  if (!preview) return;
  preview.textContent = buildOCIPolicyTemplateStatements().join('\n');
}

function appendOCIPolicyStatements(newLines) {
  const textarea = document.getElementById('oci-policy-statements');
  if (!textarea) return;
  const existing = textarea.value.split(/\r?\n/).map(s => s.trim()).filter(Boolean);
  for (const line of newLines) {
    const clean = String(line || '').trim();
    if (clean && !existing.includes(clean)) {
      existing.push(clean);
    }
  }
  textarea.value = existing.join('\n');
}

function openOCIPolicyModal(opts = {}) {
  const { policy = null, presetGroupName = '' } = opts;
  const isEdit = Boolean(policy);
  document.getElementById('oci-policy-edit-id').value = isEdit ? policy.id : '';
  document.getElementById('oci-policy-modal-title').textContent = isEdit
    ? `编辑 IAM 策略 - ${policy.name}`
    : (presetGroupName ? `新建 IAM 策略（为组 ${presetGroupName} 授权）` : '新建 IAM 策略 (Policy)');

  populateOCIPolicyModalSelects(presetGroupName);

  const nameInput = document.getElementById('oci-policy-name');
  const compSelect = document.getElementById('oci-policy-compartment');
  const descInput = document.getElementById('oci-policy-description');
  const stmtsInput = document.getElementById('oci-policy-statements');
  const msgEl = document.getElementById('oci-policy-message');
  if (msgEl) { msgEl.textContent = ''; msgEl.className = 'form-message'; }

  if (isEdit) {
    if (nameInput) { nameInput.value = policy.name || ''; nameInput.disabled = true; }
    if (compSelect) { compSelect.value = policy.compartmentId || ociIAMPolicyScope; compSelect.disabled = true; }
    if (descInput) descInput.value = policy.description || '';
    if (stmtsInput) stmtsInput.value = (policy.statements || []).join('\n');
  } else {
    const suggestedName = presetGroupName ? `${presetGroupName.toLowerCase().replace(/[^a-z0-9-_]/g, '-')}-policy` : '';
    if (nameInput) { nameInput.value = suggestedName; nameInput.disabled = false; }
    if (compSelect) { compSelect.disabled = false; }
    if (descInput) descInput.value = presetGroupName ? `Policy for group ${presetGroupName}` : '';
    if (stmtsInput) stmtsInput.value = '';
  }

  document.getElementById('oci-iam-policy-modal').hidden = false;
}

async function saveOCIPolicy() {
  if (!selectedAccount || selectedAccount.provider !== 'oci') return;
  const acc = selectedAccount.account;
  const editId = document.getElementById('oci-policy-edit-id')?.value?.trim() || '';
  const name = document.getElementById('oci-policy-name')?.value?.trim() || '';
  const compartmentId = document.getElementById('oci-policy-compartment')?.value || '';
  const description = document.getElementById('oci-policy-description')?.value?.trim() || '';
  const rawStatements = document.getElementById('oci-policy-statements')?.value || '';
  const statements = rawStatements.split(/\r?\n/).map(s => s.trim()).filter(Boolean);
  const msgEl = document.getElementById('oci-policy-message');
  const btn = document.getElementById('oci-policy-submit-btn');

  if (!editId && !name) {
    if (msgEl) { msgEl.textContent = '请填写策略名称'; msgEl.className = 'form-message error'; }
    return;
  }
  if (statements.length === 0) {
    if (msgEl) { msgEl.textContent = '请至少添加一条策略语句'; msgEl.className = 'form-message error'; }
    return;
  }

  if (btn) btn.disabled = true;
  if (msgEl) { msgEl.textContent = '正在提交到 OCI...'; msgEl.className = 'form-message'; }
  try {
    if (editId) {
      await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/iam/policies/${encodeURIComponent(editId)}`, {
        method: 'PUT',
        body: { description, statements }
      });
    } else {
      await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/iam/policies`, {
        method: 'POST',
        body: { compartmentId, name, description, statements }
      });
      ociIAMPolicyScope = compartmentId || ociIAMPolicyScope;
    }
    document.getElementById('oci-iam-policy-modal').hidden = true;
    const pageMsg = document.getElementById('oci-iam-message');
    if (pageMsg) {
      pageMsg.textContent = editId ? `策略 ${name} 已更新` : `策略 ${name} 已创建`;
      pageMsg.className = 'form-message success';
    }
    await loadOCIIAM();
  } catch (err) {
    if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
  } finally {
    if (btn) btn.disabled = false;
  }
}

async function openOCIAPIKeyModal(userId, userName) {
  ociAPIKeyModalUser = { id: userId, name: userName };
  ociGeneratedPrivateKeyPEM = '';
  document.getElementById('oci-apikey-modal-title').textContent = `API Key 管理 - ${userName}`;
  document.getElementById('oci-apikey-generated-box').hidden = true;
  document.getElementById('oci-apikey-generated-pem').value = '';
  document.getElementById('oci-apikey-input').value = '';
  const msgEl = document.getElementById('oci-apikey-message');
  if (msgEl) { msgEl.textContent = ''; msgEl.className = 'form-message'; }
  document.getElementById('oci-iam-apikey-modal').hidden = false;
  await loadOCIUserAPIKeys();
}

async function loadOCIUserAPIKeys() {
  if (!selectedAccount || !ociAPIKeyModalUser) return;
  const listEl = document.getElementById('oci-apikey-list');
  if (listEl) listEl.innerHTML = '<div class="empty-state compact">加载中...</div>';
  try {
    const data = await fetchJSON(`/api/oci/${encodeURIComponent(selectedAccount.account)}/iam/users/${encodeURIComponent(ociAPIKeyModalUser.id)}/api-keys`);
    const keys = Array.isArray(data?.apiKeys) ? data.apiKeys : [];
    if (listEl) {
      listEl.innerHTML = keys.length === 0
        ? '<div class="empty-state compact">该用户暂无 API Key</div>'
        : keys.map(k => `
            <div class="oci-apikey-row">
              <div>
                <div class="mono" style="font-size:12px;">${escapeHtml(k.fingerprint)}</div>
                <div style="font-size:11px; color:var(--muted);">${escapeHtml(k.lifecycleState || '')} · ${escapeHtml(k.timeCreated || '')}</div>
              </div>
              <button class="ghost-btn oci-ip-btn-sm oci-ip-btn-danger oci-apikey-del-btn" type="button" data-fp="${escapeAttr(k.fingerprint)}">删除</button>
            </div>
          `).join('');
    }
  } catch (err) {
    if (listEl) listEl.innerHTML = `<div class="empty-state compact error">${escapeHtml(err.message)}</div>`;
  }
}

// ==================== Tab 5 & 6: Inline Create Instance & Task Management ====================

const OCI_CREATE_TEMPLATES = {
  amd_free: {
    label: 'AMD 免费实例',
    shape: 'VM.Standard.E2.1.Micro',
    ocpus: 1,
    memoryInGBs: 1,
    bootVolumeSizeInGBs: 47,
    namePrefix: 'oci-amd-free'
  },
  arm_free: {
    label: 'ARM 免费实例',
    shape: 'VM.Standard.A1.Flex',
    ocpus: 2,
    memoryInGBs: 12,
    bootVolumeSizeInGBs: 50,
    namePrefix: 'oci-arm-free'
  },
  custom: {
    label: '自定义实例',
    shape: '',
    ocpus: 1,
    memoryInGBs: 1,
    bootVolumeSizeInGBs: 50,
    namePrefix: 'oci-custom'
  }
};

async function prepareOCICreatePage(templateOrTask = 'amd_free', shouldSwitchTab = true) {
  if (shouldSwitchTab) {
    switchOCITab('create');
  }
  const form = document.getElementById('oci-task-form');
  if (!form) return;

  const isEdit = typeof templateOrTask === 'object' && templateOrTask !== null;
  const templateKey = isEdit ? (templateOrTask.template || 'custom') : (templateOrTask || 'amd_free');
  const tpl = OCI_CREATE_TEMPLATES[templateKey] || OCI_CREATE_TEMPLATES.custom;

  document.getElementById('oci-task-edit-id').value = isEdit ? templateOrTask.id : '';
  document.getElementById('oci-create-page-title').innerHTML = isEdit
    ? `<i class="bi bi-pencil-square"></i> 编辑创建任务 - ${escapeHtml(templateOrTask.name)}`
    : `<i class="bi bi-plus-circle"></i> 创建 OCI 实例 (${escapeHtml(tpl.label)})`;

  const saveBtn = document.getElementById('oci-task-save');
  const cancelEditBtn = document.getElementById('oci-task-cancel-edit');
  if (saveBtn) saveBtn.textContent = isEdit ? '保存任务修改' : '保存为自动抢购任务';
  if (cancelEditBtn) cancelEditBtn.hidden = !isEdit;

  const msgEl = document.getElementById('oci-task-message');
  if (msgEl) {
    msgEl.textContent = '';
    msgEl.className = 'form-message';
  }

  const accountSelect = document.getElementById('oci-task-account');
  const ociAccounts = (proxyAccounts || []).filter(a => (a.provider || '').toLowerCase() === 'oci');
  const targetAccount = isEdit ? templateOrTask.account : (selectedAccount?.provider === 'oci' ? selectedAccount.account : (ociAccounts[0]?.account || ''));
  if (accountSelect) {
    accountSelect.innerHTML = ociAccounts.map(a => `
      <option value="${escapeAttr(a.account)}" ${a.account === targetAccount ? 'selected' : ''}>${escapeHtml(a.account)}</option>
    `).join('');
  }

  document.querySelectorAll('.oci-template-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.template === templateKey);
  });

  if (isEdit) {
    document.getElementById('oci-task-name').value = templateOrTask.name || '';
    document.getElementById('oci-task-display-name').value = templateOrTask.displayName || '';
    document.getElementById('oci-task-boot-size').value = templateOrTask.bootVolumeSizeInGBs || 50;
    document.getElementById('oci-task-ocpus').value = templateOrTask.ocpus || 1;
    document.getElementById('oci-task-memory').value = templateOrTask.memoryInGBs || 1;
    document.getElementById('oci-task-min-interval').value = templateOrTask.minIntervalSec || 30;
    document.getElementById('oci-task-max-interval').value = templateOrTask.maxIntervalSec || 60;
    document.getElementById('oci-task-assign-public').checked = Boolean(templateOrTask.assignPublicIp);
    document.getElementById('oci-task-assign-ipv6').checked = Boolean(templateOrTask.assignIpv6Ip);
    document.getElementById('oci-task-ssh-key').value = templateOrTask.sshPublicKey || '';
    document.getElementById('oci-task-cloud-init').value = templateOrTask.cloudInit || '';
  } else {
    document.getElementById('oci-task-name').value = tpl.namePrefix;
    document.getElementById('oci-task-display-name').value = tpl.namePrefix;
    document.getElementById('oci-task-boot-size').value = tpl.bootVolumeSizeInGBs;
    document.getElementById('oci-task-ocpus').value = tpl.ocpus;
    document.getElementById('oci-task-memory').value = tpl.memoryInGBs;
    document.getElementById('oci-task-min-interval').value = 30;
    document.getElementById('oci-task-max-interval').value = 60;
    document.getElementById('oci-task-assign-public').checked = true;
    document.getElementById('oci-task-assign-ipv6').checked = false;
    document.getElementById('oci-task-ssh-key').value = '';
    document.getElementById('oci-task-cloud-init').value = '';
  }

  form.dataset.template = templateKey;
  const desiredShape = isEdit ? templateOrTask.shape : tpl.shape;
  await loadOCICreateTaskOptions(targetAccount, desiredShape, isEdit ? templateOrTask : null);
}

async function loadOCICreateTaskOptions(account, desiredShape = '', restoreTask = null) {
  if (!account) return;
  const adSelect = document.getElementById('oci-task-ad');
  const shapeSelect = document.getElementById('oci-task-shape');
  const imageSelect = document.getElementById('oci-task-image');
  const vcnSelect = document.getElementById('oci-task-vcn');
  const msgEl = document.getElementById('oci-task-message');

  if (adSelect) adSelect.innerHTML = '<option value="">加载可用域中...</option>';
  if (shapeSelect) shapeSelect.innerHTML = '<option value="">加载规格中...</option>';
  if (imageSelect) imageSelect.innerHTML = '<option value="">加载兼容镜像中...</option>';

  try {
    const query = desiredShape ? `?shape=${encodeURIComponent(desiredShape)}` : '';
    const data = await fetchJSON(`/api/oci/${encodeURIComponent(account)}/create-options${query}`);
    ociCreateTaskOptionsCache = data;

    const ads = Array.isArray(data.availabilityDomains) ? data.availabilityDomains : [];
    if (adSelect) {
      adSelect.innerHTML = ads.map(ad => `<option value="${escapeAttr(ad)}">${escapeHtml(ad)}</option>`).join('');
      if (restoreTask?.availabilityDomain && ads.includes(restoreTask.availabilityDomain)) {
        adSelect.value = restoreTask.availabilityDomain;
      }
    }

    const shapes = Array.isArray(data.shapes) ? data.shapes : [];
    if (shapeSelect) {
      shapeSelect.innerHTML = shapes.map(s => `<option value="${escapeAttr(s.name)}">${escapeHtml(s.name)}${s.isFlexible ? ' · Flex' : ''}</option>`).join('');
      if (desiredShape && shapes.some(s => s.name === desiredShape)) {
        shapeSelect.value = desiredShape;
      }
    }
    syncOCICreateShapeInputs(shapes, shapeSelect?.value || '', !restoreTask);

    const vcns = Array.isArray(data.vcns) ? data.vcns : [];
    const subnets = Array.isArray(data.subnets) ? data.subnets : [];
    if (vcnSelect) {
      vcnSelect.innerHTML = vcns.length > 0
        ? vcns.map(v => `<option value="${escapeAttr(v.id)}">${escapeHtml(v.displayName)} (${escapeHtml(v.cidrBlock || '')})</option>`).join('')
        : '<option value="">暂无 VCN，请点击上方一键创建默认网络</option>';
      if (restoreTask?.vcnId && vcns.some(v => v.id === restoreTask.vcnId)) {
        vcnSelect.value = restoreTask.vcnId;
      } else if (restoreTask?.subnetId) {
        const sub = subnets.find(s => s.id === restoreTask.subnetId);
        if (sub?.vcnId) vcnSelect.value = sub.vcnId;
      }
    }
    const netHint = document.getElementById('oci-create-net-hint');
    if (netHint) {
      netHint.textContent = vcns.length === 0 || subnets.length === 0
        ? '当前区域未检测到 VCN/子网，建议点击右侧「一键创建默认 VCN/子网」'
        : '如需新建干净网络，也可点击右侧「一键创建默认 VCN/子网」';
    }
    renderOCICreateSubnetsAndNSGs(vcnSelect?.value || '', restoreTask?.subnetId || '', restoreTask?.nsgIds || []);

    if (Array.isArray(data.images) && data.images.length > 0) {
      renderOCICreateImages(data.images, restoreTask?.imageId || '');
    } else if (shapeSelect?.value) {
      await refreshOCICreateImages(account, shapeSelect.value, restoreTask?.imageId || '');
    }
  } catch (err) {
    if (msgEl) {
      msgEl.textContent = `加载创建参数失败：${err.message}`;
      msgEl.className = 'form-message error';
    }
  }
}

function syncOCICreateShapeInputs(shapes, shapeName, applyDefaults = false) {
  const shape = (shapes || []).find(s => s.name === shapeName);
  const ocpuInput = document.getElementById('oci-task-ocpus');
  const memInput = document.getElementById('oci-task-memory');
  if (!shape || !ocpuInput || !memInput) return;

  const isFlex = Boolean(shape.isFlexible);
  ocpuInput.disabled = !isFlex;
  memInput.disabled = !isFlex;
  if (!isFlex) {
    ocpuInput.value = shape.ocpus || 1;
    memInput.value = shape.memoryInGBs || 1;
  } else if (applyDefaults && shape.name !== 'VM.Standard.A1.Flex') {
    ocpuInput.value = shape.ocpuOptions?.min || 1;
    memInput.value = shape.memoryOptions?.minInGBs || 1;
  }
}

function renderOCICreateSubnetsAndNSGs(vcnId, selectedSubnetId = '', selectedNSGs = []) {
  if (!ociCreateTaskOptionsCache) return;
  const subnets = (ociCreateTaskOptionsCache.subnets || []).filter(s => !vcnId || s.vcnId === vcnId);
  const subnetSelect = document.getElementById('oci-task-subnet');
  if (subnetSelect) {
    subnetSelect.innerHTML = subnets.length > 0
      ? subnets.map(s => {
          const v6Tag = (s.ipv6CidrBlocks || []).length > 0 ? ' · 支持IPv6' : '';
          const pubTag = s.prohibitPublicIpOnVnic ? ' · 私有子网' : ' · 公有子网';
          return `<option value="${escapeAttr(s.id)}">${escapeHtml(s.displayName)} (${escapeHtml(s.cidrBlock || '')}${pubTag}${v6Tag})</option>`;
        }).join('')
      : '<option value="">该 VCN 下暂无子网</option>';
    if (selectedSubnetId && subnets.some(s => s.id === selectedSubnetId)) {
      subnetSelect.value = selectedSubnetId;
    }
  }

  syncOCICreateSubnetCapabilities();

  const nsgs = (ociCreateTaskOptionsCache.nsgs || []).filter(g => !vcnId || g.vcnId === vcnId);
  const nsgBox = document.getElementById('oci-task-nsgs');
  if (nsgBox) {
    if (nsgs.length === 0) {
      nsgBox.innerHTML = '<span class="field-hint">当前 VCN 下暂无网络安全组</span>';
    } else {
      nsgBox.innerHTML = nsgs.map(g => `
        <label class="checkbox-label">
          <input type="checkbox" class="oci-task-nsg-check" value="${escapeAttr(g.id)}" ${selectedNSGs.includes(g.id) ? 'checked' : ''}>
          <span>${escapeHtml(g.name || g.id)}</span>
        </label>
      `).join('');
    }
  }
}

function syncOCICreateSubnetCapabilities() {
  if (!ociCreateTaskOptionsCache) return;
  const subnetId = document.getElementById('oci-task-subnet')?.value || '';
  const subnet = (ociCreateTaskOptionsCache.subnets || []).find(s => s.id === subnetId);
  const pubCheck = document.getElementById('oci-task-assign-public');
  const v6Check = document.getElementById('oci-task-assign-ipv6');

  if (pubCheck && subnet) {
    if (subnet.prohibitPublicIpOnVnic) {
      pubCheck.checked = false;
      pubCheck.disabled = true;
    } else {
      pubCheck.disabled = false;
    }
  }
  if (v6Check && subnet) {
    const hasV6 = Array.isArray(subnet.ipv6CidrBlocks) && subnet.ipv6CidrBlocks.length > 0;
    v6Check.disabled = !hasV6;
    if (!hasV6) v6Check.checked = false;
  }
}

async function refreshOCICreateImages(account, shape, selectedImageId = '') {
  const imageSelect = document.getElementById('oci-task-image');
  if (!account || !shape || !imageSelect) return;
  imageSelect.innerHTML = '<option value="">加载兼容镜像中...</option>';
  try {
    const data = await fetchJSON(`/api/oci/${encodeURIComponent(account)}/create-images?shape=${encodeURIComponent(shape)}`);
    renderOCICreateImages(Array.isArray(data?.images) ? data.images : [], selectedImageId);
  } catch (err) {
    imageSelect.innerHTML = `<option value="">加载镜像失败: ${escapeHtml(err.message)}</option>`;
  }
}

function renderOCICreateImages(images, selectedImageId = '') {
  const imageSelect = document.getElementById('oci-task-image');
  if (!imageSelect) return;
  if (!images || images.length === 0) {
    imageSelect.innerHTML = '<option value="">当前规格下未找到可用镜像</option>';
    return;
  }
  imageSelect.innerHTML = images.map(img => `
    <option value="${escapeAttr(img.id)}">${escapeHtml(img.displayName)} (${escapeHtml(img.operatingSystem || '')} ${escapeHtml(img.operatingSystemVersion || '')})</option>
  `).join('');
  if (selectedImageId && images.some(img => img.id === selectedImageId)) {
    imageSelect.value = selectedImageId;
  }
}

function collectOCICreateFormPayload() {
  const form = document.getElementById('oci-task-form');
  const imageSelect = document.getElementById('oci-task-image');
  const nsgIds = Array.from(document.querySelectorAll('.oci-task-nsg-check:checked')).map(el => el.value);

  return {
    account: document.getElementById('oci-task-account')?.value || '',
    name: document.getElementById('oci-task-name')?.value?.trim() || '',
    template: form?.dataset?.template || 'custom',
    displayName: document.getElementById('oci-task-display-name')?.value?.trim() || '',
    availabilityDomain: document.getElementById('oci-task-ad')?.value || '',
    shape: document.getElementById('oci-task-shape')?.value || '',
    ocpus: Number(document.getElementById('oci-task-ocpus')?.value || 1),
    memoryInGBs: Number(document.getElementById('oci-task-memory')?.value || 1),
    bootVolumeSizeInGBs: Number(document.getElementById('oci-task-boot-size')?.value || 50),
    imageId: imageSelect?.value || '',
    imageName: imageSelect?.selectedOptions?.[0]?.textContent || '',
    vcnId: document.getElementById('oci-task-vcn')?.value || '',
    subnetId: document.getElementById('oci-task-subnet')?.value || '',
    assignPublicIp: document.getElementById('oci-task-assign-public')?.checked === true,
    assignIpv6Ip: document.getElementById('oci-task-assign-ipv6')?.checked === true,
    nsgIds,
    sshPublicKey: document.getElementById('oci-task-ssh-key')?.value?.trim() || '',
    cloudInit: document.getElementById('oci-task-cloud-init')?.value || '',
    minIntervalSec: Number(document.getElementById('oci-task-min-interval')?.value || 30),
    maxIntervalSec: Number(document.getElementById('oci-task-max-interval')?.value || 60)
  };
}

async function createOCIDefaultNetworkFromPage() {
  const account = document.getElementById('oci-task-account')?.value || selectedAccount?.account || '';
  if (!account) return;
  const enableIpv6 = document.getElementById('oci-default-net-ipv6')?.checked === true;
  const btn = document.getElementById('oci-create-default-net-btn');
  const msgEl = document.getElementById('oci-task-message');
  if (btn) btn.disabled = true;
  if (msgEl) {
    msgEl.textContent = '正在创建默认 VCN、互联网网关与公有子网...';
    msgEl.className = 'form-message';
  }

  try {
    const res = await fetchJSON(`/api/oci/${encodeURIComponent(account)}/default-network`, {
      method: 'POST',
      body: { enableIpv6 }
    });
    if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
    if (msgEl) {
      msgEl.textContent = `默认网络创建成功：${res.subnet?.displayName || res.subnet?.id || ''}`;
      msgEl.className = 'form-message success';
    }
    const shape = document.getElementById('oci-task-shape')?.value || '';
    await loadOCICreateTaskOptions(account, shape, {
      availabilityDomain: document.getElementById('oci-task-ad')?.value || '',
      imageId: document.getElementById('oci-task-image')?.value || '',
      vcnId: res.vcn?.id || '',
      subnetId: res.subnet?.id || ''
    });
  } catch (err) {
    if (msgEl) {
      msgEl.textContent = err.message;
      msgEl.className = 'form-message error';
    }
  } finally {
    if (btn) btn.disabled = false;
  }
}

async function launchOCIInstanceOnceFromPage() {
  const payload = collectOCICreateFormPayload();
  const msgEl = document.getElementById('oci-task-message');
  const btn = document.getElementById('oci-launch-once-btn');
  if (!payload.account) return;
  if (btn) btn.disabled = true;
  if (msgEl) {
    msgEl.textContent = '正在向 OCI 提交创建实例请求...';
    msgEl.className = 'form-message';
  }
  try {
    const res = await fetchJSON(`/api/oci/${encodeURIComponent(payload.account)}/launch-instance`, {
      method: 'POST',
      body: payload
    });
    if (msgEl) {
      msgEl.textContent = res.message || `创建成功！实例 ID: ${res.instanceId || ''}`;
      msgEl.className = 'form-message success';
    }
    addLog(`OCI 实例单次创建成功：${res.instanceId || ''}`, 'success');
    setTimeout(() => fetchVMs(), 3000);
  } catch (err) {
    if (msgEl) {
      msgEl.textContent = err.message;
      msgEl.className = 'form-message error';
    }
    addLog(`OCI 实例单次创建失败：${err.message}`, 'error');
  } finally {
    if (btn) btn.disabled = false;
  }
}

async function saveOCICreateTaskForm(event) {
  event.preventDefault();
  const msgEl = document.getElementById('oci-task-message');
  const saveBtn = document.getElementById('oci-task-save');
  const editId = document.getElementById('oci-task-edit-id')?.value || '';
  const payload = collectOCICreateFormPayload();

  if (payload.maxIntervalSec < payload.minIntervalSec) {
    msgEl.textContent = '最大请求间隔不能小于最小请求间隔。';
    msgEl.className = 'form-message error';
    return;
  }

  saveBtn.disabled = true;
  msgEl.textContent = '正在保存任务...';
  msgEl.className = 'form-message';

  try {
    const endpoint = editId ? `/api/oci/create-tasks/${encodeURIComponent(editId)}` : '/api/oci/create-tasks';
    await fetchJSON(endpoint, {
      method: editId ? 'PUT' : 'POST',
      body: payload
    });
    addLog(`OCI 创建任务「${payload.name}」已保存。`, 'success');
    document.getElementById('oci-task-edit-id').value = '';
    switchOCITab('tasks');
  } catch (err) {
    msgEl.textContent = err.message;
    msgEl.className = 'form-message error';
  } finally {
    saveBtn.disabled = false;
  }
}

async function loadOCICreateTasks(silent = false) {
  if (!selectedAccount || selectedAccount.provider !== 'oci') return;
  const reqAccount = selectedAccount.account;
  try {
    const data = await fetchJSON(`/api/oci/create-tasks?account=${encodeURIComponent(reqAccount)}`);
    if (!selectedAccount || selectedAccount.account !== reqAccount) return;
    currentOCICreateTasks = Array.isArray(data?.tasks) ? data.tasks : [];
    renderOCICreateTasks(currentOCICreateTasks);
  } catch (err) {
    if (!silent) {
      addLog(`加载 OCI 创建任务失败：${err.message}`, 'error');
    }
  }
}

function renderOCICreateTasks(tasks) {
  const listEl = document.getElementById('oci-tasks-list');
  if (!listEl) return;
  if (!tasks || tasks.length === 0) {
    listEl.innerHTML = '<div class="empty-state compact">暂无创建任务，可在「创建实例」或点击上方模板按钮创建。</div>';
    return;
  }

  const statusLabels = {
    idle: '未启动',
    running: '抢购运行中',
    stopped: '已停止',
    succeeded: '创建成功'
  };

  listEl.innerHTML = tasks.map(task => {
    const isRunning = task.status === 'running';
    const logs = Array.isArray(task.logs) ? task.logs.slice(-8).reverse() : [];
    const specText = task.shape && task.shape.toLowerCase().includes('.flex')
      ? `${task.shape} (${task.ocpus}C / ${task.memoryInGBs}G)`
      : (task.shape || '-');
    return `
      <div class="oci-task-card" data-task-id="${escapeAttr(task.id)}">
        <div class="oci-task-top">
          <div>
            <div class="oci-task-title-row">
              <h4>${escapeHtml(task.name || task.displayName || task.id)}</h4>
              <span class="oci-task-status ${escapeAttr(task.status)}">${escapeHtml(statusLabels[task.status] || task.status)}</span>
              <span class="oci-ip-badge primary">抢购次数: ${task.attempts || 0}</span>
            </div>
            <div class="oci-task-meta">
              <span>规格: <strong>${escapeHtml(specText)}</strong></span>
              <span>可用域: ${escapeHtml(task.availabilityDomain || '-')}</span>
              <span>磁盘: ${task.bootVolumeSizeInGBs || 50} GB</span>
              <span>间隔: ${task.minIntervalSec}-${task.maxIntervalSec}s</span>
              ${task.lastAttemptAt ? `<span>上次尝试: ${escapeHtml(new Date(task.lastAttemptAt).toLocaleString('zh-CN'))}</span>` : ''}
            </div>
          </div>
          <div class="oci-task-actions">
            ${isRunning
              ? `<button class="secondary-btn oci-ip-btn-sm oci-task-stop-btn" type="button" data-id="${escapeAttr(task.id)}">停止</button>`
              : `<button class="primary-btn oci-ip-btn-sm oci-task-start-btn" type="button" data-id="${escapeAttr(task.id)}">${task.status === 'succeeded' ? '再次运行' : '启动'}</button>`
            }
            <button class="ghost-btn oci-ip-btn-sm oci-task-edit-btn" type="button" data-id="${escapeAttr(task.id)}" ${isRunning ? 'disabled' : ''}>编辑</button>
            <button class="ghost-btn oci-ip-btn-sm oci-ip-btn-danger oci-task-delete-btn" type="button" data-id="${escapeAttr(task.id)}">删除</button>
          </div>
        </div>
        ${task.lastMessage ? `<div class="oci-task-last-msg">${escapeHtml(task.lastMessage)}</div>` : ''}
        ${task.lastError ? `<div class="oci-task-last-err">最近错误: ${escapeHtml(task.lastError)}</div>` : ''}
        ${logs.length > 0 ? `<div class="oci-task-logs">${logs.map(l => `<div>${escapeHtml(l)}</div>`).join('')}</div>` : ''}
      </div>
    `;
  }).join('');
}

async function handleOCITaskListClick(event) {
  const startBtn = event.target.closest('.oci-task-start-btn');
  if (startBtn) {
    const id = startBtn.dataset.id;
    startBtn.disabled = true;
    try {
      await fetchJSON(`/api/oci/create-tasks/${encodeURIComponent(id)}/start`, { method: 'POST' });
      addLog(`OCI 创建任务已启动：${id}`, 'success');
      await loadOCICreateTasks();
    } catch (err) {
      addLog(`启动创建任务失败：${err.message}`, 'error');
    } finally {
      startBtn.disabled = false;
    }
    return;
  }

  const stopBtn = event.target.closest('.oci-task-stop-btn');
  if (stopBtn) {
    const id = stopBtn.dataset.id;
    stopBtn.disabled = true;
    try {
      await fetchJSON(`/api/oci/create-tasks/${encodeURIComponent(id)}/stop`, { method: 'POST' });
      addLog(`OCI 创建任务已停止：${id}`, 'info');
      await loadOCICreateTasks();
    } catch (err) {
      addLog(`停止创建任务失败：${err.message}`, 'error');
    } finally {
      stopBtn.disabled = false;
    }
    return;
  }

  const editBtn = event.target.closest('.oci-task-edit-btn');
  if (editBtn) {
    const task = currentOCICreateTasks.find(t => t.id === editBtn.dataset.id);
    if (task) prepareOCICreatePage(task, true);
    return;
  }

  const delBtn = event.target.closest('.oci-task-delete-btn');
  if (delBtn) {
    const id = delBtn.dataset.id;
    if (!confirm('确认删除此 OCI 创建任务？')) return;
    try {
      await fetchJSON(`/api/oci/create-tasks/${encodeURIComponent(id)}`, { method: 'DELETE' });
      addLog(`OCI 创建任务已删除：${id}`, 'info');
      await loadOCICreateTasks();
    } catch (err) {
      addLog(`删除创建任务失败：${err.message}`, 'error');
    }
  }
}

// ==================== Tab 7: Object Storage ====================

let ociOSCurrentBucket = '';
let ociOSCurrentPrefix = '';
let ociOSNamespace = '';
let ociOSBucketsCount = 0;

function updateOCIOSMeta(bucketInfo = null) {
  const metaEl = document.getElementById('oci-os-meta');
  if (!metaEl) return;
  const base = `命名空间：${ociOSNamespace || '-'} · 共 ${ociOSBucketsCount} 个 Bucket`;
  if (!bucketInfo) {
    metaEl.textContent = base;
    return;
  }
  const access = bucketInfo.publicAccessType || 'NoPublicAccess';
  const count = Number(bucketInfo.approximateCount || 0);
  const size = formatBytes(bucketInfo.approximateSize || 0);
  metaEl.textContent = `${base} · 当前 ${bucketInfo.name} (${access})：${count} 个对象，共 ${size}`;
}

async function loadOCIObjectStorage() {
  if (!selectedAccount || selectedAccount.provider !== 'oci') return;
  const acc = selectedAccount.account;
  const select = document.getElementById('oci-os-bucket-select');
  const wrap = document.getElementById('oci-os-objects-wrap');
  const msgEl = document.getElementById('oci-os-message');
  if (msgEl) { msgEl.textContent = ''; msgEl.className = 'form-message'; }

  try {
    const data = await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/os/buckets`);
    if (!selectedAccount || selectedAccount.account !== acc) return;
    const buckets = Array.isArray(data?.buckets) ? data.buckets : [];
    ociOSNamespace = data?.namespace || '';
    ociOSBucketsCount = buckets.length;
    updateOCIOSMeta(null);

    if (select) {
      if (buckets.length === 0) {
        select.innerHTML = '<option value="">暂无 Bucket</option>';
        ociOSCurrentBucket = '';
        ociOSCurrentPrefix = '';
        if (wrap) wrap.innerHTML = '<div class="empty-state compact">当前区域暂无 Bucket，可点击上方「+ 创建 Bucket」。</div>';
        return;
      }
      select.innerHTML = buckets.map(b => `<option value="${escapeAttr(b.name)}">${escapeHtml(b.name)}</option>`).join('');
      if (ociOSCurrentBucket && buckets.some(b => b.name === ociOSCurrentBucket)) {
        select.value = ociOSCurrentBucket;
      } else {
        ociOSCurrentBucket = buckets[0].name;
        ociOSCurrentPrefix = '';
      }
      await loadOCIObjects(true);
    }
  } catch (err) {
    if (wrap) wrap.innerHTML = `<div class="empty-state compact error">加载对象存储失败：${escapeHtml(err.message)}</div>`;
  }
}

async function loadOCIObjects(includeBucket = false) {
  if (!selectedAccount || !ociOSCurrentBucket) return;
  const acc = selectedAccount.account;
  const wrap = document.getElementById('oci-os-objects-wrap');
  const crumb = document.getElementById('oci-os-breadcrumb');

  if (crumb) {
    const parts = ociOSCurrentPrefix.split('/').filter(Boolean);
    let accPath = '';
    crumb.innerHTML = `<button class="oci-os-crumb" type="button" data-prefix="">/${escapeHtml(ociOSCurrentBucket)}</button>` +
      parts.map(p => {
        accPath += p + '/';
        return ` / <button class="oci-os-crumb" type="button" data-prefix="${escapeAttr(accPath)}">${escapeHtml(p)}</button>`;
      }).join('');
  }

  if (wrap) wrap.innerHTML = '<div class="empty-state compact">加载对象列表中...</div>';
  try {
    const extra = includeBucket ? '&includeBucket=1' : '';
    const data = await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/os/buckets/${encodeURIComponent(ociOSCurrentBucket)}/objects?prefix=${encodeURIComponent(ociOSCurrentPrefix)}${extra}`);
    if (data?.bucketInfo) {
      updateOCIOSMeta(data.bucketInfo);
    }
    const prefixes = Array.isArray(data?.prefixes) ? data.prefixes : [];
    const objects = Array.isArray(data?.objects) ? data.objects : [];

    if (prefixes.length === 0 && objects.length === 0) {
      wrap.innerHTML = '<div class="empty-state compact">当前目录为空。</div>';
      return;
    }

    wrap.innerHTML = `
      <div class="gcp-fw-table-container">
        <table class="gcp-fw-table">
          <thead>
            <tr>
              <th>名称</th>
              <th>修改时间</th>
              <th>大小</th>
              <th style="text-align: right;">操作</th>
            </tr>
          </thead>
          <tbody>
            ${prefixes.map(p => {
              const display = p.slice(ociOSCurrentPrefix.length);
              return `
                <tr>
                  <td><button class="oci-os-crumb" type="button" data-prefix="${escapeAttr(p)}" style="font-weight:700;"><i class="bi bi-folder"></i> ${escapeHtml(display)}</button></td>
                  <td>-</td>
                  <td>文件夹</td>
                  <td style="text-align:right;">-</td>
                </tr>
              `;
            }).join('')}
            ${objects.map(o => {
              const display = o.name.slice(ociOSCurrentPrefix.length);
              return `
                <tr>
                  <td><span class="mono" style="font-size:12.5px;"><i class="bi bi-file-earmark"></i> ${escapeHtml(display)}</span></td>
                  <td style="font-size:12px; color:var(--muted);">${escapeHtml(o.timeModified ? new Date(o.timeModified).toLocaleString('zh-CN') : '-')}</td>
                  <td>${formatBytes(o.size || 0)}</td>
                  <td style="text-align:right;">
                    <div class="fw-actions-cell" style="justify-content:flex-end;">
                      <a class="secondary-btn oci-ip-btn-sm" href="/api/oci/${encodeURIComponent(acc)}/os/buckets/${encodeURIComponent(ociOSCurrentBucket)}/download?object=${encodeURIComponent(o.name)}">下载</a>
                      <button class="ghost-btn oci-ip-btn-sm oci-os-par-btn" type="button" data-object="${escapeAttr(o.name)}">预认证链接</button>
                      <button class="ghost-btn oci-ip-btn-sm oci-ip-btn-danger oci-os-del-obj-btn" type="button" data-object="${escapeAttr(o.name)}">删除</button>
                    </div>
                  </td>
                </tr>
              `;
            }).join('')}
          </tbody>
        </table>
      </div>
    `;
  } catch (err) {
    if (wrap) wrap.innerHTML = `<div class="empty-state compact error">${escapeHtml(err.message)}</div>`;
  }
}

function formatBytes(bytes) {
  const b = Number(bytes) || 0;
  if (b < 1024) return `${b} B`;
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`;
  if (b < 1024 * 1024 * 1024) return `${(b / (1024 * 1024)).toFixed(2)} MB`;
  return `${(b / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

// ==================== Event Bindings for OCI Workspace ====================

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('oci-workspace-tabs')?.addEventListener('click', e => {
    const btn = e.target.closest('.oci-ws-tab');
    if (btn && btn.dataset.ociTab) switchOCITab(btn.dataset.ociTab);
  });

  // Overview
  document.getElementById('oci-overview-refresh-btn')?.addEventListener('click', loadOCIOverview);

  // Terminate Modal
  document.getElementById('oci-terminate-close')?.addEventListener('click', closeOCITerminateModal);
  document.getElementById('oci-terminate-cancel')?.addEventListener('click', closeOCITerminateModal);
  document.getElementById('oci-terminate-modal')?.addEventListener('click', e => {
    if (e.target.id === 'oci-terminate-modal') closeOCITerminateModal();
  });
  document.getElementById('oci-terminate-confirm-input')?.addEventListener('input', e => {
    const expected = ociTerminateVM ? (ociTerminateVM.name || ociTerminateVM.id) : '';
    const confirmBtn = document.getElementById('oci-terminate-confirm');
    if (confirmBtn) confirmBtn.disabled = !expected || e.target.value.trim() !== expected;
  });
  document.getElementById('oci-terminate-confirm')?.addEventListener('click', confirmOCITerminate);

  // IAM Users, Groups & Policies
  document.getElementById('oci-iam-refresh-btn')?.addEventListener('click', loadOCIIAM);
  document.getElementById('oci-iam-new-policy-btn')?.addEventListener('click', () => openOCIPolicyModal());
  document.getElementById('oci-iam-new-user-btn')?.addEventListener('click', () => {
    document.getElementById('oci-iam-create-user-box').hidden = false;
    document.getElementById('oci-iam-create-group-box').hidden = true;
  });
  document.getElementById('oci-iam-user-cancel')?.addEventListener('click', () => {
    document.getElementById('oci-iam-create-user-box').hidden = true;
  });
  document.getElementById('oci-iam-new-group-btn')?.addEventListener('click', () => {
    document.getElementById('oci-iam-create-group-box').hidden = false;
    document.getElementById('oci-iam-create-user-box').hidden = true;
  });
  document.getElementById('oci-iam-group-cancel')?.addEventListener('click', () => {
    document.getElementById('oci-iam-create-group-box').hidden = true;
  });
  document.getElementById('oci-iam-user-submit')?.addEventListener('click', async () => {
    if (!selectedAccount) return;
    const name = document.getElementById('oci-iam-user-name')?.value?.trim() || '';
    const description = document.getElementById('oci-iam-user-desc')?.value?.trim() || '';
    const email = document.getElementById('oci-iam-user-email')?.value?.trim() || '';
    const groupId = document.getElementById('oci-iam-user-group')?.value || '';
    const msgEl = document.getElementById('oci-iam-message');
    try {
      await fetchJSON(`/api/oci/${encodeURIComponent(selectedAccount.account)}/iam/users`, {
        method: 'POST',
        body: { name, description, email, groupId }
      });
      document.getElementById('oci-iam-create-user-box').hidden = true;
      document.getElementById('oci-iam-user-name').value = '';
      if (msgEl) { msgEl.textContent = `用户 ${name} 已创建`; msgEl.className = 'form-message success'; }
      await loadOCIIAM();
    } catch (err) {
      if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
    }
  });
  document.getElementById('oci-iam-group-submit')?.addEventListener('click', async () => {
    if (!selectedAccount) return;
    const name = document.getElementById('oci-iam-group-name')?.value?.trim() || '';
    const description = document.getElementById('oci-iam-group-desc')?.value?.trim() || '';
    const msgEl = document.getElementById('oci-iam-message');
    try {
      await fetchJSON(`/api/oci/${encodeURIComponent(selectedAccount.account)}/iam/groups`, {
        method: 'POST',
        body: { name, description }
      });
      document.getElementById('oci-iam-create-group-box').hidden = true;
      document.getElementById('oci-iam-group-name').value = '';
      if (msgEl) { msgEl.textContent = `用户组 ${name} 已创建`; msgEl.className = 'form-message success'; }
      await loadOCIIAM();
    } catch (err) {
      if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
    }
  });

  document.getElementById('oci-iam-content')?.addEventListener('click', async e => {
    if (!selectedAccount) return;
    const acc = selectedAccount.account;
    const msgEl = document.getElementById('oci-iam-message');

    const keysBtn = e.target.closest('.oci-iam-keys-btn');
    if (keysBtn) {
      openOCIAPIKeyModal(keysBtn.dataset.userId, keysBtn.dataset.userName);
      return;
    }
    const pwdBtn = e.target.closest('.oci-iam-pwd-btn');
    if (pwdBtn) {
      if (!confirm(`确认重置用户「${pwdBtn.dataset.userName}」的控制台一次性登录密码？`)) return;
      try {
        const res = await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/iam/users/${encodeURIComponent(pwdBtn.dataset.userId)}/password`, { method: 'POST' });
        alert(`用户 ${pwdBtn.dataset.userName} 一次性控制台密码：\n\n${res.password}\n\n请妥善保存，下次登录需修改密码。`);
      } catch (err) {
        if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
      }
      return;
    }
    const delUserBtn = e.target.closest('.oci-iam-del-user-btn');
    if (delUserBtn) {
      if (!confirm(`确认删除 IAM 用户「${delUserBtn.dataset.userName}」？`)) return;
      try {
        await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/iam/users/${encodeURIComponent(delUserBtn.dataset.userId)}`, { method: 'DELETE' });
        await loadOCIIAM();
      } catch (err) {
        if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
      }
      return;
    }
    const delGrpBtn = e.target.closest('.oci-iam-del-group-btn');
    if (delGrpBtn) {
      if (!confirm(`确认删除用户组「${delGrpBtn.dataset.groupName}」？`)) return;
      try {
        await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/iam/groups/${encodeURIComponent(delGrpBtn.dataset.groupId)}`, { method: 'DELETE' });
        await loadOCIIAM();
      } catch (err) {
        if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
      }
      return;
    }
    const remMemBtn = e.target.closest('.oci-iam-remove-mem-btn');
    if (remMemBtn) {
      try {
        await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/iam/memberships/${encodeURIComponent(remMemBtn.dataset.membershipId)}`, { method: 'DELETE' });
        await loadOCIIAM();
      } catch (err) {
        if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
      }
      return;
    }
    const grpPolicyBtn = e.target.closest('.oci-iam-group-policy-btn');
    if (grpPolicyBtn) {
      openOCIPolicyModal({ presetGroupName: grpPolicyBtn.dataset.groupName || '' });
      return;
    }
    if (e.target.closest('#oci-iam-inline-new-policy-btn')) {
      openOCIPolicyModal();
      return;
    }
    const editPolicyBtn = e.target.closest('.oci-iam-edit-policy-btn');
    if (editPolicyBtn) {
      const policy = (ociIAMData?.policies || []).find(p => p.id === editPolicyBtn.dataset.policyId);
      if (policy) openOCIPolicyModal({ policy });
      return;
    }
    const delPolicyBtn = e.target.closest('.oci-iam-del-policy-btn');
    if (delPolicyBtn) {
      if (!confirm(`确认删除策略「${delPolicyBtn.dataset.policyName}」？`)) return;
      try {
        await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/iam/policies/${encodeURIComponent(delPolicyBtn.dataset.policyId)}`, { method: 'DELETE' });
        await loadOCIIAM();
      } catch (err) {
        if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
      }
    }
  });

  document.getElementById('oci-iam-content')?.addEventListener('change', async e => {
    const scopeSel = e.target.closest('#oci-iam-policy-scope');
    if (scopeSel) {
      await loadOCIIAMPolicies(scopeSel.value);
      return;
    }
    const sel = e.target.closest('.oci-iam-add-group-sel');
    if (!sel || !sel.value || !selectedAccount) return;
    try {
      await fetchJSON(`/api/oci/${encodeURIComponent(selectedAccount.account)}/iam/memberships`, {
        method: 'POST',
        body: { userId: sel.dataset.userId, groupId: sel.value }
      });
      await loadOCIIAM();
    } catch (err) {
      const msgEl = document.getElementById('oci-iam-message');
      if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
    }
  });

  // Policy Modal
  document.getElementById('oci-policy-modal-close')?.addEventListener('click', () => {
    document.getElementById('oci-iam-policy-modal').hidden = true;
  });
  document.getElementById('oci-policy-cancel-btn')?.addEventListener('click', () => {
    document.getElementById('oci-iam-policy-modal').hidden = true;
  });
  document.getElementById('oci-iam-policy-modal')?.addEventListener('click', e => {
    if (e.target.id === 'oci-iam-policy-modal') document.getElementById('oci-iam-policy-modal').hidden = true;
  });
  document.getElementById('oci-policy-usecase')?.addEventListener('change', renderOCIPolicyTemplateOptions);
  document.getElementById('oci-policy-template')?.addEventListener('change', updateOCIPolicyTemplatePreview);
  document.getElementById('oci-policy-tpl-group')?.addEventListener('change', updateOCIPolicyTemplatePreview);
  document.getElementById('oci-policy-tpl-location')?.addEventListener('change', updateOCIPolicyTemplatePreview);
  document.getElementById('oci-policy-apply-tpl-btn')?.addEventListener('click', () => {
    appendOCIPolicyStatements(buildOCIPolicyTemplateStatements());
  });
  document.getElementById('oci-policy-add-stmt-btn')?.addEventListener('click', () => {
    const groupName = document.getElementById('oci-policy-stmt-group')?.value || 'Administrators';
    const verb = document.getElementById('oci-policy-stmt-verb')?.value || 'manage';
    const resource = document.getElementById('oci-policy-stmt-resource')?.value || 'all-resources';
    const location = document.getElementById('oci-policy-stmt-location')?.value || 'tenancy';
    appendOCIPolicyStatements([`Allow group ${groupName} to ${verb} ${resource} in ${location}`]);
  });
  document.getElementById('oci-policy-submit-btn')?.addEventListener('click', saveOCIPolicy);

  // API Key Modal
  document.getElementById('oci-apikey-modal-close')?.addEventListener('click', () => {
    document.getElementById('oci-iam-apikey-modal').hidden = true;
  });
  document.getElementById('oci-apikey-done-btn')?.addEventListener('click', () => {
    document.getElementById('oci-iam-apikey-modal').hidden = true;
  });
  document.getElementById('oci-iam-apikey-modal')?.addEventListener('click', e => {
    if (e.target.id === 'oci-iam-apikey-modal') document.getElementById('oci-iam-apikey-modal').hidden = true;
  });
  document.getElementById('oci-apikey-upload-btn')?.addEventListener('click', async () => {
    if (!selectedAccount || !ociAPIKeyModalUser) return;
    const key = document.getElementById('oci-apikey-input')?.value?.trim() || '';
    const msgEl = document.getElementById('oci-apikey-message');
    try {
      await fetchJSON(`/api/oci/${encodeURIComponent(selectedAccount.account)}/iam/users/${encodeURIComponent(ociAPIKeyModalUser.id)}/api-keys`, {
        method: 'POST',
        body: { key, autoGenerate: false }
      });
      document.getElementById('oci-apikey-input').value = '';
      if (msgEl) { msgEl.textContent = 'API Key 上传成功'; msgEl.className = 'form-message success'; }
      await loadOCIUserAPIKeys();
    } catch (err) {
      if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
    }
  });
  document.getElementById('oci-apikey-auto-btn')?.addEventListener('click', async () => {
    if (!selectedAccount || !ociAPIKeyModalUser) return;
    const msgEl = document.getElementById('oci-apikey-message');
    try {
      const res = await fetchJSON(`/api/oci/${encodeURIComponent(selectedAccount.account)}/iam/users/${encodeURIComponent(ociAPIKeyModalUser.id)}/api-keys`, {
        method: 'POST',
        body: { autoGenerate: true }
      });
      ociGeneratedPrivateKeyPEM = res.privateKeyPem || '';
      document.getElementById('oci-apikey-generated-pem').value = ociGeneratedPrivateKeyPEM;
      document.getElementById('oci-apikey-generated-box').hidden = !ociGeneratedPrivateKeyPEM;
      if (msgEl) { msgEl.textContent = `已生成并绑定 API Key (指纹: ${res.apiKey?.fingerprint || ''})`; msgEl.className = 'form-message success'; }
      await loadOCIUserAPIKeys();
    } catch (err) {
      if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
    }
  });
  document.getElementById('oci-apikey-download-pem')?.addEventListener('click', () => {
    if (!ociGeneratedPrivateKeyPEM) return;
    const blob = new Blob([ociGeneratedPrivateKeyPEM], { type: 'application/x-pem-file' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = `${ociAPIKeyModalUser?.name || 'oci'}_api_key.pem`;
    a.click();
    URL.revokeObjectURL(a.href);
  });
  document.getElementById('oci-apikey-list')?.addEventListener('click', async e => {
    const delBtn = e.target.closest('.oci-apikey-del-btn');
    if (!delBtn || !selectedAccount || !ociAPIKeyModalUser) return;
    if (!confirm(`确认删除指纹为 ${delBtn.dataset.fp} 的 API Key？`)) return;
    const msgEl = document.getElementById('oci-apikey-message');
    try {
      await fetchJSON(`/api/oci/${encodeURIComponent(selectedAccount.account)}/iam/users/${encodeURIComponent(ociAPIKeyModalUser.id)}/api-keys/${encodeURIComponent(delBtn.dataset.fp)}`, {
        method: 'DELETE'
      });
      await loadOCIUserAPIKeys();
    } catch (err) {
      if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
    }
  });

  // Create Instance & Task Management
  document.getElementById('oci-tasks-refresh-btn')?.addEventListener('click', () => loadOCICreateTasks());
  document.getElementById('oci-task-new-amd-btn')?.addEventListener('click', () => prepareOCICreatePage('amd_free', true));
  document.getElementById('oci-task-new-arm-btn')?.addEventListener('click', () => prepareOCICreatePage('arm_free', true));
  document.getElementById('oci-task-new-btn')?.addEventListener('click', () => prepareOCICreatePage('custom', true));
  document.getElementById('oci-tasks-list')?.addEventListener('click', handleOCITaskListClick);
  document.getElementById('oci-create-reload-opts-btn')?.addEventListener('click', () => {
    const account = document.getElementById('oci-task-account')?.value || selectedAccount?.account || '';
    const shape = document.getElementById('oci-task-shape')?.value || '';
    loadOCICreateTaskOptions(account, shape, null);
  });
  document.getElementById('oci-task-cancel-edit')?.addEventListener('click', () => prepareOCICreatePage('amd_free', false));
  document.getElementById('oci-create-default-net-btn')?.addEventListener('click', createOCIDefaultNetworkFromPage);
  document.getElementById('oci-launch-once-btn')?.addEventListener('click', launchOCIInstanceOnceFromPage);
  document.getElementById('oci-task-form')?.addEventListener('submit', saveOCICreateTaskForm);

  document.querySelectorAll('.oci-template-btn').forEach(btn => {
    btn.addEventListener('click', async () => {
      const key = btn.dataset.template || 'custom';
      const tpl = OCI_CREATE_TEMPLATES[key] || OCI_CREATE_TEMPLATES.custom;
      const form = document.getElementById('oci-task-form');
      if (form) form.dataset.template = key;
      document.querySelectorAll('.oci-template-btn').forEach(b => b.classList.toggle('active', b === btn));

      const account = document.getElementById('oci-task-account')?.value || '';
      const shapeSelect = document.getElementById('oci-task-shape');
      if (tpl.shape && shapeSelect && Array.from(shapeSelect.options).some(o => o.value === tpl.shape)) {
        shapeSelect.value = tpl.shape;
      }
      document.getElementById('oci-task-boot-size').value = tpl.bootVolumeSizeInGBs;
      document.getElementById('oci-task-ocpus').value = tpl.ocpus;
      document.getElementById('oci-task-memory').value = tpl.memoryInGBs;
      if (!document.getElementById('oci-task-edit-id')?.value) {
        document.getElementById('oci-task-name').value = tpl.namePrefix;
        document.getElementById('oci-task-display-name').value = tpl.namePrefix;
      }
      if (ociCreateTaskOptionsCache) {
        syncOCICreateShapeInputs(ociCreateTaskOptionsCache.shapes, shapeSelect?.value || '', key !== 'arm_free');
      }
      if (account && shapeSelect?.value) {
        await refreshOCICreateImages(account, shapeSelect.value);
      }
    });
  });

  document.getElementById('oci-task-account')?.addEventListener('change', e => {
    const form = document.getElementById('oci-task-form');
    const tpl = OCI_CREATE_TEMPLATES[form?.dataset?.template || 'amd_free'] || OCI_CREATE_TEMPLATES.custom;
    loadOCICreateTaskOptions(e.target.value, tpl.shape, null);
  });

  document.getElementById('oci-task-shape')?.addEventListener('change', async e => {
    const account = document.getElementById('oci-task-account')?.value || '';
    const shapeName = e.target.value;
    if (ociCreateTaskOptionsCache) {
      syncOCICreateShapeInputs(ociCreateTaskOptionsCache.shapes, shapeName, true);
    }
    await refreshOCICreateImages(account, shapeName);
  });

  document.getElementById('oci-task-vcn')?.addEventListener('change', e => {
    renderOCICreateSubnetsAndNSGs(e.target.value);
  });
  document.getElementById('oci-task-subnet')?.addEventListener('change', syncOCICreateSubnetCapabilities);

  document.getElementById('oci-task-ssh-file')?.addEventListener('change', e => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      const textarea = document.getElementById('oci-task-ssh-key');
      if (textarea) textarea.value = String(reader.result || '').trim();
    };
    reader.readAsText(file);
    e.target.value = '';
  });

  // Object Storage
  document.getElementById('oci-os-refresh-btn')?.addEventListener('click', loadOCIObjectStorage);
  document.getElementById('oci-os-bucket-select')?.addEventListener('change', e => {
    ociOSCurrentBucket = e.target.value;
    ociOSCurrentPrefix = '';
    loadOCIObjects(true);
  });
  document.getElementById('oci-os-new-bucket-btn')?.addEventListener('click', () => {
    document.getElementById('oci-os-create-bucket-box').hidden = false;
  });
  document.getElementById('oci-os-bucket-cancel')?.addEventListener('click', () => {
    document.getElementById('oci-os-create-bucket-box').hidden = true;
  });
  document.getElementById('oci-os-bucket-submit')?.addEventListener('click', async () => {
    if (!selectedAccount) return;
    const name = document.getElementById('oci-os-bucket-name')?.value?.trim() || '';
    const publicAccessType = document.getElementById('oci-os-bucket-access')?.value || 'NoPublicAccess';
    const msgEl = document.getElementById('oci-os-message');
    try {
      await fetchJSON(`/api/oci/${encodeURIComponent(selectedAccount.account)}/os/buckets`, {
        method: 'POST',
        body: { name, publicAccessType }
      });
      document.getElementById('oci-os-create-bucket-box').hidden = true;
      document.getElementById('oci-os-bucket-name').value = '';
      ociOSCurrentBucket = name;
      ociOSCurrentPrefix = '';
      if (msgEl) { msgEl.textContent = `Bucket ${name} 已创建`; msgEl.className = 'form-message success'; }
      await loadOCIObjectStorage();
    } catch (err) {
      if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
    }
  });
  document.getElementById('oci-os-del-bucket-btn')?.addEventListener('click', async () => {
    if (!selectedAccount || !ociOSCurrentBucket) return;
    if (!confirm(`确认永久删除 Bucket「${ociOSCurrentBucket}」？`)) return;
    const msgEl = document.getElementById('oci-os-message');
    try {
      await fetchJSON(`/api/oci/${encodeURIComponent(selectedAccount.account)}/os/buckets/${encodeURIComponent(ociOSCurrentBucket)}`, {
        method: 'DELETE'
      });
      ociOSCurrentBucket = '';
      ociOSCurrentPrefix = '';
      if (msgEl) { msgEl.textContent = 'Bucket 已删除'; msgEl.className = 'form-message success'; }
      await loadOCIObjectStorage();
    } catch (err) {
      if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
    }
  });
  document.getElementById('oci-os-new-folder-btn')?.addEventListener('click', async () => {
    if (!selectedAccount || !ociOSCurrentBucket) return;
    const name = prompt('请输入文件夹名称：');
    if (!name || !name.trim()) return;
    const folder = ociOSCurrentPrefix + name.trim().replace(/^\/+|\/+$/g, '');
    const msgEl = document.getElementById('oci-os-message');
    try {
      await fetchJSON(`/api/oci/${encodeURIComponent(selectedAccount.account)}/os/buckets/${encodeURIComponent(ociOSCurrentBucket)}/folder`, {
        method: 'POST',
        body: { folder }
      });
      await loadOCIObjects();
    } catch (err) {
      if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
    }
  });
  document.getElementById('oci-os-upload-input')?.addEventListener('change', async e => {
    const file = e.target.files?.[0];
    if (!file || !selectedAccount || !ociOSCurrentBucket) return;
    const msgEl = document.getElementById('oci-os-message');
    if (msgEl) { msgEl.textContent = `正在上传 ${file.name}...`; msgEl.className = 'form-message'; }
    try {
      const fd = new FormData();
      fd.append('file', file);
      fd.append('prefix', ociOSCurrentPrefix);
      const res = await fetch(`/api/oci/${encodeURIComponent(selectedAccount.account)}/os/buckets/${encodeURIComponent(ociOSCurrentBucket)}/upload`, {
        method: 'POST',
        body: fd
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || '上传失败');
      if (msgEl) { msgEl.textContent = `已上传：${data.objectName}`; msgEl.className = 'form-message success'; }
      await loadOCIObjects();
    } catch (err) {
      if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
    } finally {
      e.target.value = '';
    }
  });
  document.getElementById('oci-os-breadcrumb')?.addEventListener('click', e => {
    const btn = e.target.closest('.oci-os-crumb');
    if (!btn) return;
    ociOSCurrentPrefix = btn.dataset.prefix || '';
    loadOCIObjects();
  });
  document.getElementById('oci-os-objects-wrap')?.addEventListener('click', async e => {
    if (!selectedAccount || !ociOSCurrentBucket) return;
    const acc = selectedAccount.account;
    const msgEl = document.getElementById('oci-os-message');

    const folderBtn = e.target.closest('.oci-os-crumb');
    if (folderBtn) {
      ociOSCurrentPrefix = folderBtn.dataset.prefix || '';
      loadOCIObjects();
      return;
    }
    const parBtn = e.target.closest('.oci-os-par-btn');
    if (parBtn) {
      const hoursStr = prompt('请输入预认证链接有效小时数（默认 24 小时）：', '24');
      if (hoursStr === null) return;
      const expiresHours = Math.max(1, Number(hoursStr) || 24);
      try {
        const res = await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/os/buckets/${encodeURIComponent(ociOSCurrentBucket)}/par`, {
          method: 'POST',
          body: { objectName: parBtn.dataset.object, expiresHours }
        });
        prompt(`预认证链接（有效期 ${expiresHours} 小时）：`, res.par?.fullUrl || '');
      } catch (err) {
        if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
      }
      return;
    }
    const delBtn = e.target.closest('.oci-os-del-obj-btn');
    if (delBtn) {
      const obj = delBtn.dataset.object;
      if (!confirm(`确认删除对象「${obj}」？`)) return;
      try {
        await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/os/buckets/${encodeURIComponent(ociOSCurrentBucket)}/objects?object=${encodeURIComponent(obj)}`, {
          method: 'DELETE'
        });
        await loadOCIObjects();
      } catch (err) {
        if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
      }
    }
  });

  initAzureWorkspaceListeners();
});

// ==================== Azure Horizontal Workspace (Billing & Orphan Cleanup) ====================

let currentAzureTab = 'instances';
let lastAzureWorkspaceAccount = '';
let currentAzureResources = [];
let currentAzureCleanupPayload = null;

function updateAzureWorkspaceVisibility() {
  const tabsBar = document.getElementById('azure-workspace-tabs');
  const isAzure = selectedAccount && selectedAccount.provider === 'azure';

  if (tabsBar) tabsBar.hidden = !isAzure;

  if (!isAzure) {
    lastAzureWorkspaceAccount = '';
    document.querySelectorAll('.azure-tab-pane').forEach(el => { el.hidden = true; });
    return;
  }

  if (selectedAccount.account !== lastAzureWorkspaceAccount) {
    lastAzureWorkspaceAccount = selectedAccount.account;
    currentAzureResources = [];
    const tableWrap = document.getElementById('az-resources-table-wrap');
    if (tableWrap) {
      tableWrap.innerHTML = '<div class="empty-state compact">点击「扫描资源」查看当前账号下的资源绑定状态与计费风险。</div>';
    }
  }

  switchAzureTab(currentAzureTab || 'instances');
}

function switchAzureTab(tab) {
  currentAzureTab = tab || 'instances';
  document.querySelectorAll('.azure-ws-tab').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.azureTab === currentAzureTab);
  });

  const paneMap = {
    billing: 'azure-tab-billing',
    resources: 'azure-tab-resources'
  };

  Object.values(paneMap).forEach(id => {
    const el = document.getElementById(id);
    if (el) el.hidden = true;
  });

  if (els.vmList) {
    els.vmList.hidden = currentAzureTab !== 'instances';
  }

  if (paneMap[currentAzureTab]) {
    const target = document.getElementById(paneMap[currentAzureTab]);
    if (target) target.hidden = false;
  }

  if (!selectedAccount || selectedAccount.provider !== 'azure') return;

  if (currentAzureTab === 'billing') {
    loadAzureBillingOverview();
  } else if (currentAzureTab === 'resources') {
    loadAzureResources();
  }
}

function formatUSD(val) {
  return `$${Number(val || 0).toFixed(2)}`;
}

async function loadAzureBillingOverview() {
  if (!selectedAccount || selectedAccount.provider !== 'azure') return;
  const acc = selectedAccount.account;
  const metaEl = document.getElementById('az-billing-meta');
  if (metaEl) metaEl.textContent = `账号：${acc} · 正在查询 Cost Management 消费数据...`;

  try {
    const data = await fetchJSON(`/api/azure/${encodeURIComponent(acc)}/cost-overview`);
    if (!selectedAccount || selectedAccount.account !== acc) return;
    renderAzureBillingOverview(data.overview, data.monitor);
  } catch (err) {
    if (metaEl) metaEl.textContent = `查询失败：${err.message}`;
    addLog(`Azure 消费查询失败：${err.message}`, 'error');
  }
}

function renderAzureBillingOverview(ov, mon) {
  if (!ov) return;
  const metaEl = document.getElementById('az-billing-meta');
  if (metaEl) {
    const offerTag = ov.isStudentOffer ? '学生/限额订阅' : (ov.quotaId || '标准订阅');
    metaEl.textContent = `订阅：${ov.subscriptionName || ov.account} | 类型：${offerTag} | 状态：${ov.subscriptionState || '-'} | 消费上限：${ov.spendingLimit || '-'}`;
  }

  const guideEl = document.getElementById('az-credit-guide-banner');
  if (guideEl) {
    guideEl.hidden = ov.hasCreditConfig === true;
  }

  const billCurr = ov.billingCurrency || 'CNY';
  const mtdEl = document.getElementById('az-mtd-cost');
  const mtdSub = document.getElementById('az-mtd-sub');
  if (mtdEl) mtdEl.textContent = `${formatUSD(ov.monthToDateCostUSD)} USD`;
  if (mtdSub) {
    mtdSub.textContent = ov.costStatus === 'ok'
      ? `计费币种约 ${Number(ov.monthToDateCostBilling || 0).toFixed(2)} ${billCurr}`
      : (ov.costMessage || 'Cost Management 查询失败');
  }

  const periodEl = document.getElementById('az-period-cost');
  const periodSub = document.getElementById('az-period-sub');
  if (periodEl) periodEl.textContent = `${formatUSD(ov.periodTotalCostUSD)} USD`;
  if (periodSub) {
    periodSub.textContent = ov.remainingBasis === 'since_start_date' && ov.startDate
      ? `自 ${ov.startDate} 起始日至今`
      : `近 12 个月 (${ov.historyStartDate || ''} ~ ${ov.historyEndDate || ''})`;
  }

  const remEl = document.getElementById('az-remaining-credit');
  const remSub = document.getElementById('az-remaining-sub');
  if (ov.estimatedRemainingUSD !== undefined && ov.estimatedRemainingUSD !== null) {
    if (remEl) remEl.textContent = `${formatUSD(ov.estimatedRemainingUSD)} USD`;
    if (remSub) remSub.textContent = `按初始赠金 ${formatUSD(ov.initialCredit)} 扣减估算`;
  } else {
    if (remEl) remEl.textContent = '未配置初始赠金';
    if (remSub) remSub.textContent = '填写初始赠金后自动计算';
  }

  const expEl = document.getElementById('az-expire-days');
  if (ov.daysUntilExpire !== undefined && ov.daysUntilExpire !== null) {
    if (expEl) {
      expEl.textContent = ov.daysUntilExpire >= 0
        ? `剩 ${ov.daysUntilExpire} 天`
        : `已过期 ${Math.abs(ov.daysUntilExpire)} 天`;
    }
  } else {
    if (expEl) expEl.textContent = '未配置到期日';
  }

  // Service cost breakdown
  const svcListEl = document.getElementById('az-service-cost-list');
  if (svcListEl) {
    if (ov.costStatus !== 'ok') {
      svcListEl.innerHTML = `<div class="empty-state compact error">${escapeHtml(ov.costMessage || '查询失败')}</div>`;
    } else if (!ov.monthByService || ov.monthByService.length === 0) {
      svcListEl.innerHTML = '<div class="empty-state compact">本月暂无消费记录（$0.00）</div>';
    } else {
      svcListEl.className = 'az-cost-list';
      svcListEl.innerHTML = ov.monthByService.map(item => `
        <div class="az-cost-row">
          <span>${escapeHtml(item.service)}</span>
          <span class="az-cost-val">${formatUSD(item.costUSD)} USD <small style="color: var(--muted);">(${Number(item.costBilling || 0).toFixed(2)} ${escapeHtml(billCurr)})</small></span>
        </div>
      `).join('');
    }
  }

  // Monthly history
  const histEl = document.getElementById('az-monthly-cost-list');
  if (histEl) {
    if (!ov.monthlyHistory || ov.monthlyHistory.length === 0) {
      histEl.innerHTML = '<div class="empty-state compact">暂无历史月度消费记录</div>';
    } else {
      histEl.className = 'az-cost-list';
      histEl.innerHTML = [...ov.monthlyHistory].reverse().map(m => `
        <div class="az-cost-row">
          <span>${escapeHtml(m.month)}</span>
          <span class="az-cost-val">${formatUSD(m.costUSD)} USD <small style="color: var(--muted);">(${Number(m.costBilling || 0).toFixed(2)} ${escapeHtml(billCurr)})</small></span>
        </div>
      `).join('');
    }
  }

  if (mon) {
    renderAzureMonitorStatus(mon);
  }
}

function renderAzureMonitorStatus(mon) {
  const statusEl = document.getElementById('az-monitor-status');
  if (statusEl) {
    statusEl.textContent = mon.running ? '运行中' : '未启用';
    statusEl.className = `dt-monitor-status-badge ${mon.running ? 'on' : 'off'}`;
  }

  const cfg = mon.config || {};
  const setVal = (id, val) => { const el = document.getElementById(id); if (el) el.value = val; };
  const setChk = (id, val) => { const el = document.getElementById(id); if (el) el.checked = !!val; };

  setVal('az-mon-initial', cfg.initialCredit > 0 ? cfg.initialCredit : '');
  setVal('az-mon-start-date', cfg.startDate || '');
  setVal('az-mon-expire-date', cfg.expireDate || '');
  setChk('az-mon-enabled', cfg.enabled);
  setVal('az-mon-interval', cfg.interval || 3600);
  setVal('az-mon-days-before', cfg.daysBeforeExpire || 0);
  setVal('az-mon-min-remaining', cfg.minRemainingCredit || 0);
  setVal('az-mon-max-monthly', cfg.maxMonthlyCost || 0);
  setVal('az-mon-action', cfg.triggerAction || 'alert');
  setChk('az-mon-confirm-delete-all', cfg.confirmAutoDeleteAll);
  updateAzureDangerConfirmVisibility();

  const logsEl = document.getElementById('az-monitor-logs');
  if (logsEl) {
    const logs = Array.isArray(mon.logs) ? mon.logs : [];
    logsEl.textContent = logs.length > 0 ? logs.slice().reverse().join('\n') : '暂无监控日志';
  }
}

function updateAzureDangerConfirmVisibility() {
  const action = document.getElementById('az-mon-action')?.value || 'alert';
  const row = document.getElementById('az-mon-confirm-delete-row');
  if (row) {
    row.hidden = action !== 'delete_all';
  }
}

async function saveAzureMonitorConfig() {
  if (!selectedAccount || selectedAccount.provider !== 'azure') return;
  const acc = selectedAccount.account;
  const msgEl = document.getElementById('az-mon-config-message');
  const btn = document.getElementById('az-mon-save-btn');

  const payload = {
    enabled: !!document.getElementById('az-mon-enabled')?.checked,
    interval: Math.max(300, Number(document.getElementById('az-mon-interval')?.value) || 3600),
    initialCredit: Math.max(0, Number(document.getElementById('az-mon-initial')?.value) || 0),
    startDate: document.getElementById('az-mon-start-date')?.value || '',
    expireDate: document.getElementById('az-mon-expire-date')?.value || '',
    daysBeforeExpire: Math.max(0, Number(document.getElementById('az-mon-days-before')?.value) || 0),
    minRemainingCredit: Math.max(0, Number(document.getElementById('az-mon-min-remaining')?.value) || 0),
    maxMonthlyCost: Math.max(0, Number(document.getElementById('az-mon-max-monthly')?.value) || 0),
    triggerAction: document.getElementById('az-mon-action')?.value || 'alert',
    confirmAutoDeleteAll: !!document.getElementById('az-mon-confirm-delete-all')?.checked
  };

  if (btn) btn.disabled = true;
  if (msgEl) { msgEl.textContent = '正在保存并同步配置...'; msgEl.className = 'form-message'; }

  try {
    const res = await fetchJSON(`/api/azure/${encodeURIComponent(acc)}/monitor/config`, {
      method: 'POST',
      body: payload
    });
    if (msgEl) { msgEl.textContent = '已保存赠金与监控配置。'; msgEl.className = 'form-message success'; }
    if (res.status) renderAzureMonitorStatus(res.status);
    await loadAzureBillingOverview();
  } catch (err) {
    if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
  } finally {
    if (btn) btn.disabled = false;
  }
}

async function loadAzureResources() {
  if (!selectedAccount || selectedAccount.provider !== 'azure') return;
  const acc = selectedAccount.account;
  const scope = document.getElementById('az-resource-scope')?.value || 'rg';
  const wrap = document.getElementById('az-resources-table-wrap');
  const msgEl = document.getElementById('az-resources-message');
  if (msgEl) { msgEl.textContent = ''; msgEl.className = 'form-message'; }
  if (wrap) wrap.innerHTML = '<div class="empty-state compact">正在扫描 Azure 资源及绑定关系...</div>';

  try {
    const data = await fetchJSON(`/api/azure/${encodeURIComponent(acc)}/resources?scope=${encodeURIComponent(scope)}`);
    if (!selectedAccount || selectedAccount.account !== acc) return;
    currentAzureResources = Array.isArray(data.items) ? data.items : [];
    renderAzureResources(data);
  } catch (err) {
    if (wrap) wrap.innerHTML = `<div class="empty-state compact error">扫描失败：${escapeHtml(err.message)}</div>`;
  }
}

function renderAzureResources(data) {
  const s = data.summary || {};
  const setText = (id, val) => { const el = document.getElementById(id); if (el) el.textContent = val; };
  setText('az-res-total', String(s.totalCount ?? 0));
  setText('az-res-orphan-ips', String(s.orphanPublicIpCount ?? 0));
  setText('az-res-orphan-disks', String(s.orphanDiskCount ?? 0));
  setText('az-res-billable', `${s.billableRiskCount ?? 0}（快照 ${s.snapshotCount ?? 0}）`);

  const metaEl = document.getElementById('az-resources-meta');
  if (metaEl) {
    const scopeLabel = data.scope === 'rg' && data.targetResourceGroup
      ? `资源组 ${data.targetResourceGroup}`
      : '整个订阅（已排除系统 NetworkWatcherRG）';
    metaEl.textContent = `扫描范围：${scopeLabel} · 孤立资源共 ${s.orphanTotalCount ?? 0} 项`;
  }

  const wrap = document.getElementById('az-resources-table-wrap');
  if (!wrap) return;
  const items = Array.isArray(data.items) ? data.items : [];
  if (items.length === 0) {
    wrap.innerHTML = '<div class="empty-state compact">当前扫描范围内未发现资源。</div>';
    return;
  }

  const riskLabel = {
    orphan_billable: '孤立计费',
    likely_billable: '可能计费',
    free_eligible: '免费额度内',
    free_network: '免费网络',
    unknown: '未知'
  };
  const catLabel = {
    vm: '虚拟机',
    public_ip: '公网 IP',
    disk: '托管磁盘',
    snapshot: '快照',
    nic: '网卡',
    vnet: '虚拟网络',
    nsg: '安全组',
    other: '其他'
  };

  wrap.innerHTML = `
    <div class="az-table-scroll">
      <table class="az-resource-table">
        <thead>
          <tr>
            <th><input id="az-res-check-all" type="checkbox" title="全选"></th>
            <th>资源名称</th>
            <th>类型</th>
            <th>资源组</th>
            <th>规格 / 详情</th>
            <th>绑定状态</th>
            <th>计费风险评估</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          ${items.map(item => {
            const specParts = [];
            if (item.sku) specParts.push(item.sku);
            if (item.sizeGB) specParts.push(`${item.sizeGB} GB`);
            if (item.ipAddress) specParts.push(item.ipAddress);
            if (item.allocationMethod) specParts.push(item.allocationMethod);
            const specText = specParts.join(' · ') || '-';
            const attachHTML = item.orphan
              ? '<span class="az-risk-badge orphan_billable">孤立未绑定</span>'
              : (item.attachedTo ? escapeHtml(item.attachedTo) : '<span style="color: var(--muted);">-</span>');
            return `
              <tr>
                <td><input class="az-res-row-check" type="checkbox" value="${escapeAttr(item.id)}"></td>
                <td><strong>${escapeHtml(item.name)}</strong></td>
                <td>${escapeHtml(catLabel[item.category] || item.category)}</td>
                <td><code>${escapeHtml(item.resourceGroup || '-')}</code></td>
                <td>${escapeHtml(specText)}</td>
                <td>${attachHTML}</td>
                <td>
                  <span class="az-risk-badge ${escapeAttr(item.billingRisk)}">${escapeHtml(riskLabel[item.billingRisk] || item.billingRisk)}</span>
                  <div style="font-size: 11.5px; color: var(--muted); margin-top: 3px;">${escapeHtml(item.billingNote || '')}</div>
                </td>
                <td>
                  <button class="ghost-btn oci-ip-btn-sm oci-ip-btn-danger az-res-single-del-btn" type="button" data-id="${escapeAttr(item.id)}">删除</button>
                </td>
              </tr>
            `;
          }).join('')}
        </tbody>
      </table>
    </div>
  `;
}

function openAzureCleanupModal(spec) {
  const modal = document.getElementById('azure-cleanup-modal');
  const titleEl = document.getElementById('azure-cleanup-modal-title');
  const descEl = document.getElementById('azure-cleanup-modal-desc');
  const listEl = document.getElementById('azure-cleanup-preview-list');
  const confirmBox = document.getElementById('azure-cleanup-confirm-box');
  const highRiskChk = document.getElementById('azure-cleanup-highrisk-check');
  const msgEl = document.getElementById('azure-cleanup-modal-msg');
  if (!modal || !listEl) return;

  const previewItems = spec.previewItems || [];
  if (previewItems.length === 0) {
    const resMsg = document.getElementById('az-resources-message');
    if (resMsg) {
      resMsg.textContent = '当前条件下没有匹配的待清理资源。';
      resMsg.className = 'form-message';
    }
    return;
  }

  currentAzureCleanupPayload = spec.payload;
  if (titleEl) titleEl.textContent = spec.title || '确认清理 Azure 资源';
  if (descEl) descEl.textContent = spec.desc || '';
  if (msgEl) { msgEl.textContent = ''; msgEl.className = 'form-message'; }

  const needsHighRiskCheck = spec.highRisk || previewItems.some(i => i.category === 'vm' || !i.orphan);
  if (confirmBox) confirmBox.hidden = !needsHighRiskCheck;
  if (highRiskChk) highRiskChk.checked = false;

  listEl.innerHTML = previewItems.map(item => `
    <div class="az-cleanup-preview-item">
      <span><strong>${escapeHtml(item.name)}</strong> <small style="color: var(--muted);">(${escapeHtml(item.category)} · ${escapeHtml(item.resourceGroup)})</small></span>
      <span class="az-risk-badge ${escapeAttr(item.billingRisk)}">${item.orphan ? '孤立' : '在用/关联'}</span>
    </div>
  `).join('');

  modal.hidden = false;
}

function closeAzureCleanupModal() {
  const modal = document.getElementById('azure-cleanup-modal');
  if (modal) modal.hidden = true;
  currentAzureCleanupPayload = null;
}

async function confirmAzureCleanup() {
  if (!selectedAccount || selectedAccount.provider !== 'azure' || !currentAzureCleanupPayload) return;
  const confirmBox = document.getElementById('azure-cleanup-confirm-box');
  const highRiskChk = document.getElementById('azure-cleanup-highrisk-check');
  const msgEl = document.getElementById('azure-cleanup-modal-msg');
  const confirmBtn = document.getElementById('azure-cleanup-modal-confirm');

  if (confirmBox && !confirmBox.hidden && !highRiskChk?.checked) {
    if (msgEl) { msgEl.textContent = '请先勾选二次确认开关后再执行删除。'; msgEl.className = 'form-message error'; }
    return;
  }

  const acc = selectedAccount.account;
  if (confirmBtn) confirmBtn.disabled = true;
  if (msgEl) { msgEl.textContent = '正在按依赖顺序清理资源（Azure 异步操作可能需要数十秒）...'; msgEl.className = 'form-message'; }

  try {
    const res = await fetchJSON(`/api/azure/${encodeURIComponent(acc)}/resources/cleanup`, {
      method: 'POST',
      body: currentAzureCleanupPayload
    });
    if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, res.success ? 'info' : 'error'));

    const resMsg = document.getElementById('az-resources-message');
    if (resMsg) {
      const deletedCount = Array.isArray(res.deleted) ? res.deleted.length : 0;
      const failedCount = Array.isArray(res.failed) ? res.failed.length : 0;
      resMsg.textContent = failedCount > 0
        ? `清理完成：成功 ${deletedCount} 项，失败 ${failedCount} 项（详情见底部操作日志）。`
        : `清理完成：已成功删除 ${deletedCount} 项资源。`;
      resMsg.className = failedCount > 0 ? 'form-message error' : 'form-message success';
    }

    closeAzureCleanupModal();
    await loadAzureResources();
    await fetchVMs();
  } catch (err) {
    if (msgEl) { msgEl.textContent = err.message; msgEl.className = 'form-message error'; }
  } finally {
    if (confirmBtn) confirmBtn.disabled = false;
  }
}

function initAzureWorkspaceListeners() {
  document.getElementById('azure-workspace-tabs')?.addEventListener('click', e => {
    const btn = e.target.closest('.azure-ws-tab');
    if (!btn) return;
    switchAzureTab(btn.dataset.azureTab);
  });

  document.getElementById('az-billing-refresh-btn')?.addEventListener('click', loadAzureBillingOverview);
  document.getElementById('az-mon-settings-toggle')?.addEventListener('click', () => {
    const p = document.getElementById('az-mon-settings-panel');
    if (p) p.hidden = !p.hidden;
  });
  document.getElementById('az-guide-open-settings-btn')?.addEventListener('click', () => {
    const p = document.getElementById('az-mon-settings-panel');
    if (p) {
      p.hidden = false;
      p.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    }
    document.getElementById('az-mon-initial')?.focus();
  });
  document.getElementById('az-mon-action')?.addEventListener('change', updateAzureDangerConfirmVisibility);
  document.getElementById('az-mon-save-btn')?.addEventListener('click', saveAzureMonitorConfig);
  document.getElementById('az-mon-start-btn')?.addEventListener('click', async () => {
    if (!selectedAccount || selectedAccount.provider !== 'azure') return;
    const res = await fetchJSON(`/api/azure/${encodeURIComponent(selectedAccount.account)}/monitor/start`, { method: 'POST' });
    if (res.status) renderAzureMonitorStatus(res.status);
  });
  document.getElementById('az-mon-stop-btn')?.addEventListener('click', async () => {
    if (!selectedAccount || selectedAccount.provider !== 'azure') return;
    await fetchJSON(`/api/azure/${encodeURIComponent(selectedAccount.account)}/monitor/stop`, { method: 'POST' });
    await loadAzureBillingOverview();
  });

  document.getElementById('az-resources-scan-btn')?.addEventListener('click', loadAzureResources);
  document.getElementById('az-resource-scope')?.addEventListener('change', loadAzureResources);

  document.getElementById('az-resources-table-wrap')?.addEventListener('change', e => {
    if (e.target.id === 'az-res-check-all') {
      const checked = e.target.checked;
      document.querySelectorAll('.az-res-row-check').forEach(cb => { cb.checked = checked; });
    }
  });

  document.getElementById('az-resources-table-wrap')?.addEventListener('click', e => {
    const delBtn = e.target.closest('.az-res-single-del-btn');
    if (!delBtn) return;
    const id = delBtn.dataset.id;
    const item = currentAzureResources.find(r => r.id === id);
    if (!item) return;
    openAzureCleanupModal({
      title: '确认删除单个资源',
      desc: `即将从 Azure 删除以下资源（资源组：${item.resourceGroup}）：`,
      previewItems: [item],
      highRisk: item.category === 'vm' || !item.orphan,
      payload: {
        mode: 'selected',
        scope: document.getElementById('az-resource-scope')?.value || 'rg',
        resourceIds: [item.id]
      }
    });
  });

  document.getElementById('az-clean-orphans-btn')?.addEventListener('click', () => {
    const incIP = !!document.getElementById('az-opt-orphan-ips')?.checked;
    const incDisk = !!document.getElementById('az-opt-orphan-disks')?.checked;
    const incNIC = !!document.getElementById('az-opt-orphan-nics')?.checked;
    const incSnap = !!document.getElementById('az-opt-snapshots')?.checked;
    const preview = currentAzureResources.filter(i =>
      (incIP && i.category === 'public_ip' && i.orphan) ||
      (incDisk && i.category === 'disk' && i.orphan) ||
      (incNIC && i.category === 'nic' && i.orphan) ||
      (incSnap && i.category === 'snapshot')
    );
    openAzureCleanupModal({
      title: '确认清理孤立计费资源',
      desc: '以下为当前筛选出的未绑定/孤立资源（不会影响在用免费 VM）：',
      previewItems: preview,
      highRisk: false,
      payload: {
        mode: 'orphans',
        scope: document.getElementById('az-resource-scope')?.value || 'rg',
        includeOrphanIps: incIP,
        includeOrphanDisks: incDisk,
        includeOrphanNics: incNIC,
        includeSnapshots: incSnap
      }
    });
  });

  document.getElementById('az-clean-selected-btn')?.addEventListener('click', () => {
    const ids = Array.from(document.querySelectorAll('.az-res-row-check:checked')).map(cb => cb.value);
    const preview = currentAzureResources.filter(i => ids.includes(i.id));
    openAzureCleanupModal({
      title: '确认删除表格勾选资源',
      desc: `已勾选 ${preview.length} 项资源，将按依赖顺序（VM → 网卡 → IP → 磁盘）执行删除：`,
      previewItems: preview,
      highRisk: preview.some(i => i.category === 'vm' || !i.orphan),
      payload: {
        mode: 'selected',
        scope: document.getElementById('az-resource-scope')?.value || 'rg',
        resourceIds: ids
      }
    });
  });

  document.getElementById('az-clean-all-btn')?.addEventListener('click', () => {
    const keepNet = !!document.getElementById('az-opt-keep-network')?.checked;
    const preview = currentAzureResources.filter(i => !(keepNet && (i.category === 'vnet' || i.category === 'nsg')));
    openAzureCleanupModal({
      title: '⚠️ 高危：一键删除所有计费项目',
      desc: keepNet
        ? '将按依赖顺序删除以下全部计费资源（含在用 VM、磁盘、公网 IP、网卡、快照），保留免费 VNet/NSG 框架：'
        : '将按依赖顺序删除扫描范围内的全部资源（含 VM、磁盘、IP、网卡、快照、VNet 与 NSG）：',
      previewItems: preview,
      highRisk: true,
      payload: {
        mode: 'all_billable',
        scope: document.getElementById('az-resource-scope')?.value || 'rg',
        includeNetwork: !keepNet
      }
    });
  });

  document.getElementById('azure-cleanup-modal-close')?.addEventListener('click', closeAzureCleanupModal);
  document.getElementById('azure-cleanup-modal-cancel')?.addEventListener('click', closeAzureCleanupModal);
  document.getElementById('azure-cleanup-modal-confirm')?.addEventListener('click', confirmAzureCleanup);
}
