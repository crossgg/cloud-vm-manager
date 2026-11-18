// ==================== Azure Create & GCP Horizontal Workspace ====================

let currentGCPTab = 'instances';
let lastGCPWorkspaceAccount = '';
let azureCreateOptionsCache = null;
let gcpCreateOptionsCache = null;
let currentGCPResources = [];

// ---------- Azure Create Instance ----------

async function loadAzureCreateOptions(locationOverride = '') {
  if (!selectedAccount || selectedAccount.provider !== 'azure') return;
  const reqAcc = selectedAccount.account;
  const msgEl = document.getElementById('az-create-message');
  if (msgEl) {
    msgEl.textContent = '正在加载 Azure 区域、规格与网络选项...';
    msgEl.className = 'form-message';
  }
  try {
    const q = locationOverride ? `?location=${encodeURIComponent(locationOverride)}` : '';
    const opts = await fetchJSON(`/api/azure/${encodeURIComponent(reqAcc)}/create-options${q}`);
    if (!selectedAccount || selectedAccount.provider !== 'azure' || selectedAccount.account !== reqAcc) return;
    azureCreateOptionsCache = opts;
    renderAzureCreateOptions(opts);
    if (msgEl) msgEl.textContent = '';
  } catch (err) {
    if (msgEl) {
      msgEl.textContent = `加载创建参数失败: ${err.message}`;
      msgEl.className = 'form-message error';
    }
  }
}

function renderAzureCreateOptions(opts) {
  const rgSel = document.getElementById('az-create-rg');
  const locSel = document.getElementById('az-create-location');
  const sizeSel = document.getElementById('az-create-size');
  const imgSel = document.getElementById('az-create-image');
  const subSel = document.getElementById('az-create-subnet');
  const nsgSel = document.getElementById('az-create-nsg');

  if (rgSel) {
    const rgs = Array.isArray(opts.resourceGroups) ? opts.resourceGroups : [];
    rgSel.innerHTML = rgs.length
      ? rgs.map(r => `<option value="${escapeAttr(r)}" ${r === opts.defaultResourceGroup ? 'selected' : ''}>${escapeHtml(r)}</option>`).join('')
      : `<option value="${escapeAttr(opts.defaultResourceGroup || 'default-rg')}">${escapeHtml(opts.defaultResourceGroup || 'default-rg')}</option>`;
  }

  if (locSel) {
    const locs = Array.isArray(opts.locations) ? opts.locations : [];
    locSel.innerHTML = locs.map(l => `<option value="${escapeAttr(l)}" ${l === opts.defaultLocation ? 'selected' : ''}>${escapeHtml(getLocationText(l))} (${escapeHtml(l)})</option>`).join('');
  }

  if (sizeSel) {
    const sizes = Array.isArray(opts.vmSizes) ? opts.vmSizes : [];
    const prev = sizeSel.value;
    sizeSel.innerHTML = sizes.map(s => `<option value="${escapeAttr(s.name)}">${escapeHtml(s.label || s.name)}</option>`).join('');
    if (prev && sizes.some(s => s.name === prev)) sizeSel.value = prev;
  }

  if (imgSel) {
    const imgs = Array.isArray(opts.images) ? opts.images : [];
    const prev = imgSel.value;
    imgSel.innerHTML = imgs.map(i => `<option value="${escapeAttr(i.id)}">${escapeHtml(i.label)}</option>`).join('');
    if (prev && imgs.some(i => i.id === prev)) imgSel.value = prev;
  }

  if (subSel) {
    const subs = Array.isArray(opts.subnets) ? opts.subnets : [];
    const curLoc = locSel?.value || opts.defaultLocation;
    const filtered = subs.filter(s => !curLoc || !s.location || s.location.toLowerCase() === curLoc.toLowerCase());
    const list = filtered.length > 0 ? filtered : subs;
    subSel.innerHTML = '<option value="">自动创建默认 VNet/子网</option>' + list.map(s => {
      const v6Tag = s.hasIpv6 ? ' · 双栈' : '';
      return `<option value="${escapeAttr(s.id)}">${escapeHtml(s.vnetName)} / ${escapeHtml(s.name)} (${escapeHtml(s.ipv4Cidr || '')}${v6Tag}) [${escapeHtml(s.resourceGroup)}]</option>`;
    }).join('');
  }

  if (nsgSel) {
    const nsgs = Array.isArray(opts.nsgs) ? opts.nsgs : [];
    nsgSel.innerHTML = '<option value="">不单独绑定 / 跟随子网默认</option>' + nsgs.map(n =>
      `<option value="${escapeAttr(n.id)}">${escapeHtml(n.name)} (${escapeHtml(n.resourceGroup)} · ${escapeHtml(n.location)})</option>`
    ).join('');
  }
}

async function createAzureDefaultNetworkFromForm() {
  if (!selectedAccount || selectedAccount.provider !== 'azure') return;
  const acc = selectedAccount.account;
  const rg = document.getElementById('az-create-rg')?.value || '';
  const location = document.getElementById('az-create-location')?.value || '';
  const enableIpv6 = Boolean(document.getElementById('az-default-net-ipv6')?.checked);
  const msgEl = document.getElementById('az-create-message');
  const btn = document.getElementById('az-create-default-net-btn');

  if (btn) btn.disabled = true;
  if (msgEl) {
    msgEl.textContent = '正在创建默认 VNet / 子网 / NSG...';
    msgEl.className = 'form-message';
  }
  try {
    const res = await fetchJSON(`/api/azure/${encodeURIComponent(acc)}/default-network`, {
      method: 'POST',
      body: { resourceGroup: rg, location, enableIpv6 }
    });
    if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
    await loadAzureCreateOptions(location);
    if (res.network?.subnetId) {
      const subSel = document.getElementById('az-create-subnet');
      if (subSel) subSel.value = res.network.subnetId;
    }
    if (res.network?.nsgId) {
      const nsgSel = document.getElementById('az-create-nsg');
      if (nsgSel) nsgSel.value = res.network.nsgId;
    }
    if (msgEl) {
      msgEl.textContent = `默认网络创建成功：${res.network?.vnetName || ''}`;
      msgEl.className = 'form-message success';
    }
  } catch (err) {
    if (msgEl) {
      msgEl.textContent = `创建默认网络失败: ${err.message}`;
      msgEl.className = 'form-message error';
    }
  } finally {
    if (btn) btn.disabled = false;
  }
}

async function submitAzureCreateForm(event) {
  event.preventDefault();
  if (!selectedAccount || selectedAccount.provider !== 'azure') return;
  const acc = selectedAccount.account;
  const msgEl = document.getElementById('az-create-message');
  const logsEl = document.getElementById('az-create-logs');
  const submitBtn = document.getElementById('az-create-submit-btn');

  const payload = {
    name: document.getElementById('az-create-name')?.value?.trim() || '',
    resourceGroup: document.getElementById('az-create-rg')?.value || '',
    location: document.getElementById('az-create-location')?.value || '',
    zone: document.getElementById('az-create-zone')?.value || '',
    vmSize: document.getElementById('az-create-size')?.value || 'Standard_B1s',
    imageId: document.getElementById('az-create-image')?.value || 'ubuntu-2404',
    osDiskType: document.getElementById('az-create-disk-type')?.value || 'StandardSSD_LRS',
    osDiskSizeGb: Number(document.getElementById('az-create-disk-size')?.value || 0),
    subnetId: document.getElementById('az-create-subnet')?.value || '',
    nsgId: document.getElementById('az-create-nsg')?.value || '',
    assignPublicIpv4: Boolean(document.getElementById('az-create-pub-v4')?.checked),
    assignIpv6: Boolean(document.getElementById('az-create-pub-v6')?.checked),
    spot: Boolean(document.getElementById('az-create-spot')?.checked),
    adminUsername: document.getElementById('az-create-user')?.value?.trim() || 'azureuser',
    authType: document.getElementById('az-create-auth-type')?.value || 'ssh',
    sshPublicKey: document.getElementById('az-create-ssh-key')?.value?.trim() || '',
    adminPassword: document.getElementById('az-create-password')?.value || '',
    customData: document.getElementById('az-create-custom-data')?.value || ''
  };

  if (submitBtn) submitBtn.disabled = true;
  if (msgEl) {
    msgEl.textContent = '正在提交 Azure 创建 VM 请求并等待部署完成（约需 30-90 秒）...';
    msgEl.className = 'form-message';
  }
  if (logsEl) {
    logsEl.hidden = true;
    logsEl.textContent = '';
  }

  try {
    const res = await fetchJSON(`/api/azure/${encodeURIComponent(acc)}/launch-instance`, {
      method: 'POST',
      body: payload
    });
    if (Array.isArray(res.logs)) {
      res.logs.forEach(l => addLog(l, 'info'));
      if (logsEl) {
        logsEl.textContent = res.logs.join('\n');
        logsEl.hidden = false;
      }
    }
    if (msgEl) {
      msgEl.textContent = `Azure VM ${payload.name} 创建成功！`;
      msgEl.className = 'form-message success';
    }
    await fetchVMs();
  } catch (err) {
    if (msgEl) {
      msgEl.textContent = `创建失败: ${err.message}`;
      msgEl.className = 'form-message error';
    }
    addLog(`创建 Azure VM 失败: ${err.message}`, 'error');
  } finally {
    if (submitBtn) submitBtn.disabled = false;
  }
}

// ---------- GCP Horizontal Workspace ----------

function updateGCPWorkspaceVisibility() {
  const tabsBar = document.getElementById('gcp-workspace-tabs');
  const isGCP = selectedAccount && selectedAccount.provider === 'gcp';

  if (tabsBar) tabsBar.hidden = !isGCP;

  if (!isGCP) {
    lastGCPWorkspaceAccount = '';
    document.querySelectorAll('.gcp-tab-pane').forEach(el => { el.hidden = true; });
    return;
  }

  if (selectedAccount.account !== lastGCPWorkspaceAccount) {
    lastGCPWorkspaceAccount = selectedAccount.account;
    gcpCreateOptionsCache = null;
    currentGCPResources = [];
    const wrap = document.getElementById('gcp-resources-table-wrap');
    if (wrap) {
      wrap.innerHTML = '<div class="empty-state compact">点击「扫描资源」查看当前项目的磁盘、快照与静态 IP 状态。</div>';
    }
  }

  switchGCPTab(currentGCPTab || 'instances');
}

function switchGCPTab(tab) {
  currentGCPTab = tab || 'instances';
  document.querySelectorAll('.gcp-ws-tab').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.gcpTab === currentGCPTab);
  });

  const paneMap = {
    create: 'gcp-tab-create',
    resources: 'gcp-tab-resources'
  };

  Object.values(paneMap).forEach(id => {
    const el = document.getElementById(id);
    if (el) el.hidden = true;
  });

  if (els.vmList) {
    els.vmList.hidden = currentGCPTab !== 'instances';
  }

  if (paneMap[currentGCPTab]) {
    const target = document.getElementById(paneMap[currentGCPTab]);
    if (target) target.hidden = false;
  }

  if (!selectedAccount || selectedAccount.provider !== 'gcp') return;

  if (currentGCPTab === 'create') {
    loadGCPCreateOptions();
  } else if (currentGCPTab === 'resources') {
    loadGCPResources();
  }
}

async function loadGCPCreateOptions(zoneOverride = '') {
  if (!selectedAccount || selectedAccount.provider !== 'gcp') return;
  const reqAcc = selectedAccount.account;
  const msgEl = document.getElementById('gcp-create-message');
  if (msgEl) {
    msgEl.textContent = '正在加载 GCP Zone、机型与子网信息...';
    msgEl.className = 'form-message';
  }
  try {
    const q = zoneOverride ? `?zone=${encodeURIComponent(zoneOverride)}` : '';
    const opts = await fetchJSON(`/api/gcp/${encodeURIComponent(reqAcc)}/create-options${q}`);
    if (!selectedAccount || selectedAccount.provider !== 'gcp' || selectedAccount.account !== reqAcc) return;
    gcpCreateOptionsCache = opts;
    renderGCPCreateOptions(opts);
    if (msgEl) msgEl.textContent = '';
  } catch (err) {
    if (msgEl) {
      msgEl.textContent = `加载创建参数失败: ${err.message}`;
      msgEl.className = 'form-message error';
    }
  }
}

function renderGCPCreateOptions(opts) {
  const zoneSel = document.getElementById('gcp-create-zone');
  const machSel = document.getElementById('gcp-create-machine');
  const imgSel = document.getElementById('gcp-create-image');
  const netSel = document.getElementById('gcp-create-network');

  if (zoneSel) {
    const zones = Array.isArray(opts.zones) ? opts.zones : [];
    zoneSel.innerHTML = zones.map(z => `<option value="${escapeAttr(z)}" ${z === opts.selectedZone ? 'selected' : ''}>${escapeHtml(z)}</option>`).join('');
  }
  if (machSel) {
    const machines = Array.isArray(opts.machineTypes) ? opts.machineTypes : [];
    const prev = machSel.value;
    machSel.innerHTML = machines.map(m => `<option value="${escapeAttr(m.name)}">${escapeHtml(m.name)}${m.description ? ` (${escapeHtml(m.description)})` : ''}</option>`).join('');
    if (prev && machines.some(m => m.name === prev)) machSel.value = prev;
  }
  if (imgSel) {
    const imgs = Array.isArray(opts.images) ? opts.images : [];
    const prev = imgSel.value;
    imgSel.innerHTML = imgs.map(i => `<option value="${escapeAttr(i.sourceImage)}" data-user="${escapeAttr(i.defaultUser)}">${escapeHtml(i.label)}</option>`).join('');
    if (prev && imgs.some(i => i.sourceImage === prev)) imgSel.value = prev;
  }
  if (netSel) {
    const nets = Array.isArray(opts.networks) ? opts.networks : [];
    netSel.innerHTML = nets.map(n => `<option value="${escapeAttr(n.name)}" data-auto="${n.autoCreateSubnetworks ? 'true' : 'false'}">${escapeHtml(n.name)}${n.autoCreateSubnetworks ? ' (Auto Mode)' : ' (Custom Mode)'}</option>`).join('');
  }
  syncGCPSubnetsForSelectedNetwork();
}

function syncGCPSubnetsForSelectedNetwork() {
  const netSel = document.getElementById('gcp-create-network');
  const subSel = document.getElementById('gcp-create-subnet');
  if (!netSel || !subSel || !gcpCreateOptionsCache) return;

  const netName = netSel.value;
  const subs = (gcpCreateOptionsCache.subnets || []).filter(s => !netName || s.network === netName);
  subSel.innerHTML = subs.length
    ? subs.map(s => {
        const v6 = s.stackType === 'IPV4_IPV6' ? ` · ${s.ipv6AccessType || 'IPv6'}` : '';
        return `<option value="${escapeAttr(s.name)}" data-stack="${escapeAttr(s.stackType)}" data-access="${escapeAttr(s.ipv6AccessType || '')}">${escapeHtml(s.name)} (${escapeHtml(s.ipv4Cidr || '')}${escapeHtml(v6)})</option>`;
      }).join('')
    : '<option value="">default</option>';
  syncGCPCreateAutoVPCConfirm();
}

function syncGCPCreateAutoVPCConfirm() {
  const wrap = document.getElementById('gcp-create-auto-vpc-confirm-wrap');
  const pubV6 = Boolean(document.getElementById('gcp-create-pub-v6')?.checked);
  const netSel = document.getElementById('gcp-create-network');
  const subSel = document.getElementById('gcp-create-subnet');
  if (!wrap || !netSel) return;

  const isAutoNet = netSel.selectedOptions?.[0]?.dataset?.auto === 'true';
  const subStack = subSel?.selectedOptions?.[0]?.dataset?.stack || '';
  const subAccess = subSel?.selectedOptions?.[0]?.dataset?.access || '';
  const subAlreadyExternalV6 = subStack === 'IPV4_IPV6' && subAccess === 'EXTERNAL';
  wrap.hidden = !(pubV6 && isAutoNet && !subAlreadyExternalV6);
}

async function submitGCPCreateForm(event) {
  event.preventDefault();
  if (!selectedAccount || selectedAccount.provider !== 'gcp') return;
  const acc = selectedAccount.account;
  const msgEl = document.getElementById('gcp-create-message');
  const logsEl = document.getElementById('gcp-create-logs');
  const submitBtn = document.getElementById('gcp-create-submit-btn');

  const confirmWrap = document.getElementById('gcp-create-auto-vpc-confirm-wrap');
  const confirmIrreversible = Boolean(document.getElementById('gcp-create-confirm-irreversible')?.checked);
  if (confirmWrap && !confirmWrap.hidden && !confirmIrreversible) {
    if (msgEl) {
      msgEl.textContent = '请先勾选确认：默认 Auto Mode VPC 开启外部 IPv6 需不可逆转换为 Custom Mode。';
      msgEl.className = 'form-message error';
    }
    return;
  }

  const rawTags = document.getElementById('gcp-create-tags')?.value || '';
  const tags = rawTags.split(',').map(s => s.trim()).filter(Boolean);

  const payload = {
    name: document.getElementById('gcp-create-name')?.value?.trim() || '',
    zone: document.getElementById('gcp-create-zone')?.value || '',
    machineType: document.getElementById('gcp-create-machine')?.value || 'e2-micro',
    sourceImage: document.getElementById('gcp-create-image')?.value || '',
    diskType: document.getElementById('gcp-create-disk-type')?.value || 'pd-standard',
    diskSizeGb: Number(document.getElementById('gcp-create-disk-size')?.value || 10),
    network: document.getElementById('gcp-create-network')?.value || 'default',
    subnetwork: document.getElementById('gcp-create-subnet')?.value || '',
    networkTier: document.getElementById('gcp-create-tier')?.value || 'PREMIUM',
    assignPublicIpv4: Boolean(document.getElementById('gcp-create-pub-v4')?.checked),
    assignIpv6: Boolean(document.getElementById('gcp-create-pub-v6')?.checked),
    confirmIrreversible,
    spot: Boolean(document.getElementById('gcp-create-spot')?.checked),
    sshUser: document.getElementById('gcp-create-ssh-user')?.value?.trim() || 'debian',
    sshPublicKey: document.getElementById('gcp-create-ssh-key')?.value?.trim() || '',
    startupScript: document.getElementById('gcp-create-startup-script')?.value || '',
    tags
  };

  if (submitBtn) submitBtn.disabled = true;
  if (msgEl) {
    msgEl.textContent = '正在创建 GCP 实例并等待操作完成...';
    msgEl.className = 'form-message';
  }
  if (logsEl) {
    logsEl.hidden = true;
    logsEl.textContent = '';
  }

  try {
    const res = await fetchJSON(`/api/gcp/${encodeURIComponent(acc)}/launch-instance`, {
      method: 'POST',
      body: payload
    });
    if (Array.isArray(res.logs)) {
      res.logs.forEach(l => addLog(l, 'info'));
      if (logsEl) {
        logsEl.textContent = res.logs.join('\n');
        logsEl.hidden = false;
      }
    }
    if (msgEl) {
      msgEl.textContent = `GCP 实例 ${payload.name} 创建成功！`;
      msgEl.className = 'form-message success';
    }
    await fetchVMs();
  } catch (err) {
    if (msgEl) {
      msgEl.textContent = `创建失败: ${err.message}`;
      msgEl.className = 'form-message error';
    }
    addLog(`创建 GCP 实例失败: ${err.message}`, 'error');
  } finally {
    if (submitBtn) submitBtn.disabled = false;
  }
}

async function loadGCPResources() {
  if (!selectedAccount || selectedAccount.provider !== 'gcp') return;
  const reqAcc = selectedAccount.account;
  const wrap = document.getElementById('gcp-resources-table-wrap');
  const msgEl = document.getElementById('gcp-resources-message');
  if (wrap) wrap.innerHTML = '<div class="empty-state compact">正在扫描项目磁盘、快照与静态 IP...</div>';
  if (msgEl) msgEl.textContent = '';

  try {
    const data = await fetchJSON(`/api/gcp/${encodeURIComponent(reqAcc)}/resources`);
    if (!selectedAccount || selectedAccount.provider !== 'gcp' || selectedAccount.account !== reqAcc) return;
    currentGCPResources = Array.isArray(data?.items) ? data.items : [];
    const sum = data?.summary || {};
    document.getElementById('gcp-res-total').textContent = sum.totalCount ?? currentGCPResources.length;
    document.getElementById('gcp-res-orphan-disks').textContent = sum.orphanDiskCount ?? 0;
    document.getElementById('gcp-res-snapshots').textContent = sum.snapshotCount ?? 0;
    document.getElementById('gcp-res-idle-ips').textContent = sum.unusedStaticIps ?? 0;
    renderGCPResourcesTable(currentGCPResources);
  } catch (err) {
    if (wrap) wrap.innerHTML = `<div class="empty-state compact error">扫描失败：${escapeHtml(err.message)}</div>`;
  }
}

function renderGCPResourcesTable(items) {
  const wrap = document.getElementById('gcp-resources-table-wrap');
  if (!wrap) return;
  if (!items || items.length === 0) {
    wrap.innerHTML = '<div class="empty-state compact">当前项目没有扫描到磁盘、快照或静态 IP。</div>';
    return;
  }

  const catLabels = {
    disk: '磁盘',
    snapshot: '快照',
    address: '静态 IP'
  };

  wrap.innerHTML = `
    <div class="az-table-scroll">
      <table class="azure-nsg-table">
        <thead>
          <tr>
            <th style="width: 34px;"><input id="gcp-res-check-all" type="checkbox"></th>
            <th>名称</th>
            <th>类型</th>
            <th>位置 / 范围</th>
            <th>详情</th>
            <th>状态 / 计费提醒</th>
          </tr>
        </thead>
        <tbody>
          ${items.map(item => {
            const detail = item.ipAddress || (item.sizeGb ? `${item.sizeGb} GB` : '-');
            const badge = item.orphan
              ? '<span class="az-risk-badge orphan_billable">孤立/闲置计费</span>'
              : '<span class="az-risk-badge free_eligible">使用中</span>';
            return `
              <tr>
                <td><input class="gcp-res-checkbox" type="checkbox" value="${escapeAttr(item.id)}" data-orphan="${item.orphan ? 'true' : 'false'}" ${item.orphan ? '' : 'disabled'}></td>
                <td><strong>${escapeHtml(item.name)}</strong>${item.attachedTo ? `<div class="field-hint">挂载：${escapeHtml(item.attachedTo)}</div>` : ''}</td>
                <td>${escapeHtml(catLabels[item.category] || item.category)}</td>
                <td>${escapeHtml(item.scope || '-')}</td>
                <td class="mono">${escapeHtml(detail)}</td>
                <td>${badge}<div class="field-hint">${escapeHtml(item.billingNote || '')}</div></td>
              </tr>
            `;
          }).join('')}
        </tbody>
      </table>
    </div>
  `;
}

async function cleanupSelectedGCPResources() {
  if (!selectedAccount || selectedAccount.provider !== 'gcp') return;
  const acc = selectedAccount.account;
  const ids = Array.from(document.querySelectorAll('.gcp-res-checkbox:checked')).map(el => el.value);
  const msgEl = document.getElementById('gcp-resources-message');
  if (ids.length === 0) {
    if (msgEl) {
      msgEl.textContent = '请先勾选至少一项孤立/闲置资源。';
      msgEl.className = 'form-message error';
    }
    return;
  }
  if (!confirm(`确认永久删除所选的 ${ids.length} 项 GCP 资源？此操作不可恢复！`)) return;

  if (msgEl) {
    msgEl.textContent = '正在清理所选 GCP 资源...';
    msgEl.className = 'form-message';
  }
  try {
    const res = await fetchJSON(`/api/gcp/${encodeURIComponent(acc)}/resources/cleanup`, {
      method: 'POST',
      body: { resourceIds: ids }
    });
    const okCount = Array.isArray(res.deleted) ? res.deleted.length : 0;
    const errList = Array.isArray(res.errors) ? res.errors : [];
    if (errList.length > 0) {
      if (msgEl) {
        msgEl.textContent = `成功删除 ${okCount} 项，失败 ${errList.length} 项：${errList.join('; ')}`;
        msgEl.className = 'form-message error';
      }
    } else {
      if (msgEl) {
        msgEl.textContent = `已成功清理 ${okCount} 项闲置资源。`;
        msgEl.className = 'form-message success';
      }
    }
    await loadGCPResources();
  } catch (err) {
    if (msgEl) {
      msgEl.textContent = `清理失败: ${err.message}`;
      msgEl.className = 'form-message error';
    }
  }
}

// ---------- Event Bindings ----------

document.addEventListener('DOMContentLoaded', () => {
  // Azure Create bindings
  document.getElementById('az-create-reload-btn')?.addEventListener('click', () => {
    const loc = document.getElementById('az-create-location')?.value || '';
    loadAzureCreateOptions(loc);
  });
  document.getElementById('az-create-location')?.addEventListener('change', e => {
    loadAzureCreateOptions(e.target.value);
  });
  document.getElementById('az-create-auth-type')?.addEventListener('change', e => {
    const isPass = e.target.value === 'password';
    const sshWrap = document.getElementById('az-create-ssh-wrap');
    const passWrap = document.getElementById('az-create-pass-wrap');
    if (sshWrap) sshWrap.hidden = isPass;
    if (passWrap) passWrap.hidden = !isPass;
  });
  document.getElementById('az-create-ssh-file')?.addEventListener('change', async e => {
    const file = e.target.files?.[0];
    if (!file) return;
    const text = await file.text();
    const area = document.getElementById('az-create-ssh-key');
    if (area) area.value = text.trim();
  });
  document.getElementById('az-create-default-net-btn')?.addEventListener('click', createAzureDefaultNetworkFromForm);
  document.getElementById('az-create-form')?.addEventListener('submit', submitAzureCreateForm);
  document.querySelectorAll('.az-template-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const tpl = btn.dataset.template;
      const sizeSel = document.getElementById('az-create-size');
      if (!sizeSel) return;
      const map = { b1s: 'Standard_B1s', b2ats: 'Standard_B2ats_v2', b2pts: 'Standard_B2pts_v2' };
      const target = map[tpl];
      if (target) {
        if (!Array.from(sizeSel.options).some(o => o.value === target)) {
          sizeSel.add(new Option(target, target));
        }
        sizeSel.value = target;
      }
    });
  });

  // GCP Workspace bindings
  document.getElementById('gcp-workspace-tabs')?.addEventListener('click', e => {
    const btn = e.target.closest('.gcp-ws-tab');
    if (!btn) return;
    switchGCPTab(btn.dataset.gcpTab);
  });
  document.getElementById('gcp-create-reload-btn')?.addEventListener('click', () => {
    const zone = document.getElementById('gcp-create-zone')?.value || '';
    loadGCPCreateOptions(zone);
  });
  document.getElementById('gcp-create-zone')?.addEventListener('change', e => {
    loadGCPCreateOptions(e.target.value);
  });
  document.getElementById('gcp-create-network')?.addEventListener('change', syncGCPSubnetsForSelectedNetwork);
  document.getElementById('gcp-create-subnet')?.addEventListener('change', syncGCPCreateAutoVPCConfirm);
  document.getElementById('gcp-create-pub-v6')?.addEventListener('change', syncGCPCreateAutoVPCConfirm);
  document.getElementById('gcp-create-image')?.addEventListener('change', e => {
    const usr = e.target.selectedOptions?.[0]?.dataset?.user;
    const usrInput = document.getElementById('gcp-create-ssh-user');
    if (usr && usrInput) usrInput.value = usr;
  });
  document.getElementById('gcp-create-ssh-file')?.addEventListener('change', async e => {
    const file = e.target.files?.[0];
    if (!file) return;
    const text = await file.text();
    const area = document.getElementById('gcp-create-ssh-key');
    if (area) area.value = text.trim();
  });
  document.querySelectorAll('.gcp-template-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      const tpl = btn.dataset.template;
      const machSel = document.getElementById('gcp-create-machine');
      const diskType = document.getElementById('gcp-create-disk-type');
      const diskSize = document.getElementById('gcp-create-disk-size');
      if (tpl === 'e2_micro') {
        if (machSel) {
          if (!Array.from(machSel.options).some(o => o.value === 'e2-micro')) machSel.add(new Option('e2-micro', 'e2-micro'));
          machSel.value = 'e2-micro';
        }
        if (diskType) diskType.value = 'pd-standard';
        if (diskSize) diskSize.value = '30';
      } else if (tpl === 'e2_small' && machSel) {
        if (!Array.from(machSel.options).some(o => o.value === 'e2-small')) machSel.add(new Option('e2-small', 'e2-small'));
        machSel.value = 'e2-small';
      } else if (tpl === 'e2_medium' && machSel) {
        if (!Array.from(machSel.options).some(o => o.value === 'e2-medium')) machSel.add(new Option('e2-medium', 'e2-medium'));
        machSel.value = 'e2-medium';
      }
    });
  });
  document.getElementById('gcp-create-form')?.addEventListener('submit', submitGCPCreateForm);

  // GCP Resources bindings
  document.getElementById('gcp-resources-scan-btn')?.addEventListener('click', loadGCPResources);
  document.getElementById('gcp-clean-select-orphans-btn')?.addEventListener('click', () => {
    document.querySelectorAll('.gcp-res-checkbox[data-orphan="true"]').forEach(chk => { chk.checked = true; });
  });
  document.getElementById('gcp-clean-selected-btn')?.addEventListener('click', cleanupSelectedGCPResources);
  document.getElementById('gcp-resources-table-wrap')?.addEventListener('change', e => {
    if (e.target.id === 'gcp-res-check-all') {
      const checked = e.target.checked;
      document.querySelectorAll('.gcp-res-checkbox:not(:disabled)').forEach(chk => { chk.checked = checked; });
    }
  });

  // OCI Idle Resources bindings
  document.getElementById('oci-resources-scan-btn')?.addEventListener('click', loadOCIResources);
  document.getElementById('oci-clean-select-idle-btn')?.addEventListener('click', () => {
    document.querySelectorAll('.oci-res-checkbox[data-idle="true"]').forEach(chk => { chk.checked = true; });
  });
  document.getElementById('oci-clean-selected-btn')?.addEventListener('click', cleanupSelectedOCIResources);
  document.getElementById('oci-resources-table-wrap')?.addEventListener('change', e => {
    if (e.target.id === 'oci-res-check-all') {
      const checked = e.target.checked;
      document.querySelectorAll('.oci-res-checkbox:not(:disabled)').forEach(chk => { chk.checked = checked; });
    }
  });
});

// ---------- OCI Idle Resources Cleanup ----------

async function loadOCIResources() {
  if (!selectedAccount || selectedAccount.provider !== 'oci') return;
  const reqAcc = selectedAccount.account;
  const wrap = document.getElementById('oci-resources-table-wrap');
  const msgEl = document.getElementById('oci-resources-message');
  if (wrap) wrap.innerHTML = '<div class="empty-state compact">正在扫描 OCI 启动盘、数据卷、保留 IP、备份与自定义镜像...</div>';
  if (msgEl) msgEl.textContent = '';

  try {
    const data = await fetchJSON(`/api/oci/${encodeURIComponent(reqAcc)}/resources`);
    if (!selectedAccount || selectedAccount.provider !== 'oci' || selectedAccount.account !== reqAcc) return;
    const items = Array.isArray(data?.items) ? data.items : [];
    const sum = data?.summary || {};
    document.getElementById('oci-res-idle-vols').textContent = `${sum.idleVolumeCount ?? 0} 个 (${sum.idleVolumeGb ?? 0} GB)`;
    document.getElementById('oci-res-total-vol-gb').textContent = `总块存储已用 ${sum.totalVolumeGb ?? 0} / 200 GB`;
    document.getElementById('oci-res-idle-ips').textContent = sum.idleReservedIps ?? 0;
    document.getElementById('oci-res-backups').textContent = sum.backupCount ?? 0;
    document.getElementById('oci-res-images').textContent = sum.customImageCount ?? 0;
    renderOCIResourcesTable(items);
  } catch (err) {
    if (wrap) wrap.innerHTML = `<div class="empty-state compact error">扫描失败：${escapeHtml(err.message)}</div>`;
  }
}

function renderOCIResourcesTable(items) {
  const wrap = document.getElementById('oci-resources-table-wrap');
  if (!wrap) return;
  if (!items || items.length === 0) {
    wrap.innerHTML = '<div class="empty-state compact">当前 OCI 账号没有扫描到卷、保留 IP、备份或自定义镜像。</div>';
    return;
  }

  const catLabels = {
    boot_volume: '启动盘',
    block_volume: '数据卷',
    reserved_ip: '保留公网 IP',
    boot_backup: '引导卷备份',
    volume_backup: '数据卷备份',
    custom_image: '自定义镜像'
  };

  wrap.innerHTML = `
    <div class="az-table-scroll">
      <table class="azure-nsg-table">
        <thead>
          <tr>
            <th style="width: 34px;"><input id="oci-res-check-all" type="checkbox"></th>
            <th>名称</th>
            <th>类型</th>
            <th>可用域 / 详情</th>
            <th>状态 / 配额提醒</th>
          </tr>
        </thead>
        <tbody>
          ${items.map(item => {
            const detailParts = [item.availabilityDomain, item.ipAddress || (item.sizeGb ? `${item.sizeGb} GB` : '')].filter(Boolean);
            const badge = item.idle
              ? '<span class="az-risk-badge orphan_billable">闲置 / 可清理</span>'
              : '<span class="az-risk-badge free_eligible">使用中</span>';
            return `
              <tr>
                <td><input class="oci-res-checkbox" type="checkbox" value="${escapeAttr(item.id)}" data-idle="${item.idle ? 'true' : 'false'}" ${item.idle ? '' : 'disabled'}></td>
                <td><strong>${escapeHtml(item.name)}</strong>${item.attachedTo ? `<div class="field-hint">绑定：${escapeHtml(item.attachedTo)}</div>` : ''}</td>
                <td>${escapeHtml(catLabels[item.category] || item.category)}</td>
                <td class="mono">${escapeHtml(detailParts.join(' · ') || '-')}</td>
                <td>${badge}<div class="field-hint">${escapeHtml(item.billingNote || '')}</div></td>
              </tr>
            `;
          }).join('')}
        </tbody>
      </table>
    </div>
  `;
}

async function cleanupSelectedOCIResources() {
  if (!selectedAccount || selectedAccount.provider !== 'oci') return;
  const acc = selectedAccount.account;
  const ids = Array.from(document.querySelectorAll('.oci-res-checkbox:checked')).map(el => el.value);
  const msgEl = document.getElementById('oci-resources-message');
  if (ids.length === 0) {
    if (msgEl) {
      msgEl.textContent = '请先勾选至少一项闲置资源。';
      msgEl.className = 'form-message error';
    }
    return;
  }
  if (!confirm(`确认永久删除所选的 ${ids.length} 项 OCI 闲置资源？此操作不可恢复！`)) return;

  if (msgEl) {
    msgEl.textContent = '正在清理所选 OCI 闲置资源...';
    msgEl.className = 'form-message';
  }
  try {
    const res = await fetchJSON(`/api/oci/${encodeURIComponent(acc)}/resources/cleanup`, {
      method: 'POST',
      body: { resourceIds: ids }
    });
    const okCount = Array.isArray(res.deleted) ? res.deleted.length : 0;
    const errList = Array.isArray(res.errors) ? res.errors : [];
    if (errList.length > 0) {
      if (msgEl) {
        msgEl.textContent = `成功清理 ${okCount} 项，失败 ${errList.length} 项：${errList.join('; ')}`;
        msgEl.className = 'form-message error';
      }
    } else {
      if (msgEl) {
        msgEl.textContent = `已成功清理 ${okCount} 项 OCI 闲置资源。`;
        msgEl.className = 'form-message success';
      }
    }
    await loadOCIResources();
  } catch (err) {
    if (msgEl) {
      msgEl.textContent = `清理失败: ${err.message}`;
      msgEl.className = 'form-message error';
    }
  }
}
