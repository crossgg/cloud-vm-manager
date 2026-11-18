// ==================== Multi-Cloud Centralized IP Management ====================

let ociIPModalVM = null;
let ociIPInfo = null;
let ociIPSelectedDNSIPv6 = '';
let ociIPSubformMode = '';

function openOCIIPModal(vm) {
  ociIPModalVM = vm;
  ociIPInfo = null;
  ociIPSelectedDNSIPv6 = '';
  ociIPSubformMode = '';
  const providerLabel = (vm.provider || '').toUpperCase();
  document.getElementById('oci-ip-modal-title').textContent = `${vm.name || vm.id} - IP管理 (${providerLabel})`;
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

function getIPCapabilities(info) {
  const caps = info?.capabilities || {};
  const prov = (info?.provider || ociIPModalVM?.provider || 'oci').toLowerCase();
  return {
    provider: prov,
    canAddSecondaryPrivateIpv4: caps.canAddSecondaryPrivateIpv4 ?? (prov !== 'gcp'),
    canAttachReservedIpv4: caps.canAttachReservedIpv4 ?? true,
    canCreateReservedIpv4: caps.canCreateReservedIpv4 ?? true,
    canDetachPublicIpv4: caps.canDetachPublicIpv4 ?? true,
    canAttachIpv6: caps.canAttachIpv6 ?? true,
    canChangeIpv6: caps.canChangeIpv6 ?? true,
    canDeleteIpv6: caps.canDeleteIpv6 ?? true,
    canEnableSubnetIpv6: caps.canEnableSubnetIpv6 ?? true,
    ipv6PrefixConfigurable: caps.ipv6PrefixConfigurable ?? (prov === 'oci'),
    hasIdlePublicIpFeeWarning: caps.hasIdlePublicIpFeeWarning ?? (prov !== 'oci'),
    notes: Array.isArray(caps.notes) ? caps.notes : []
  };
}

function getPublicIPBadge(pub, provider) {
  if (!pub) return '';
  const lifetime = String(pub.lifetime || '').toUpperCase();
  const sku = pub.sku ? ` · ${pub.sku}` : '';
  if (provider === 'azure') {
    const label = lifetime === 'EPHEMERAL' ? `动态 IP${sku}` : `静态 IP${sku}`;
    return `<span class="oci-ip-badge ${lifetime === 'EPHEMERAL' ? 'ephemeral' : 'reserved'}">${escapeHtml(label)}</span>`;
  }
  if (provider === 'gcp') {
    const isRes = lifetime === 'RESERVED';
    return `<span class="oci-ip-badge ${isRes ? 'reserved' : 'ephemeral'}">${isRes ? '静态保留 IP' : '临时 IP'}</span>`;
  }
  const isRes = lifetime === 'RESERVED';
  return `<span class="oci-ip-badge ${isRes ? 'reserved' : 'ephemeral'}">${isRes ? '保留 IP' : '临时 IP'}</span>`;
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
    body.innerHTML = '<div class="empty-state compact">未找到已附加的网络接口。</div>';
    return;
  }

  const caps = getIPCapabilities(info);
  const prov = caps.provider;

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

  const hasUnboundPrivV4 = vnics.some(v => (v.privateIps || []).some(p => !p.publicIp));
  const attachV4Btn = document.getElementById('oci-ip-attach-v4-btn');
  if (attachV4Btn) {
    const canV4 = caps.canAddSecondaryPrivateIpv4 || hasUnboundPrivV4;
    attachV4Btn.hidden = false;
    attachV4Btn.disabled = !canV4;
    attachV4Btn.title = canV4
      ? '为网卡分配公网 IPv4 或新增辅助私网 IPv4'
      : '当前网卡已绑定外部 IPv4，且该云平台不支持同网卡新增辅助 IPv4';
  }

  const attachReservedBtn = document.getElementById('oci-ip-attach-reserved-btn');
  if (attachReservedBtn) {
    attachReservedBtn.hidden = !caps.canAttachReservedIpv4;
    attachReservedBtn.textContent = prov === 'azure' ? '附加静态IP' : '附加保留IP';
  }

  const attachV6Btn = document.getElementById('oci-ip-attach-v6-btn');
  if (attachV6Btn) {
    attachV6Btn.hidden = !caps.canAttachIpv6;
    attachV6Btn.disabled = false;
    attachV6Btn.classList.toggle('oci-btn-muted', !hasAnyIPv6Subnet);
    attachV6Btn.textContent = '附加 IPv6';
    attachV6Btn.title = hasAnyIPv6Subnet
      ? '为网卡分配或启用 IPv6 地址'
      : '当前子网未开启 IPv6，点击将打开「开启子网 IPv6」向导';
  }

  const enableV6Btn = document.getElementById('oci-ip-enable-v6-btn');
  if (enableV6Btn) {
    enableV6Btn.hidden = !caps.canEnableSubnetIpv6 || !needsIPv6Setup;
    enableV6Btn.textContent = hasAnyIPv6Subnet ? '补全子网 IPv6' : '开启子网 IPv6';
    enableV6Btn.title = '查看网络 / 子网 IPv6 就绪状态并一键开启双栈';
  }

  const notesBanner = caps.notes.length > 0
    ? `<div class="field-hint" style="margin-bottom: 10px; padding: 8px 10px; border-radius: 8px; background: rgba(255,255,255,0.03); border: 1px solid var(--panel-border); line-height: 1.55;">
        ${caps.notes.map(n => `<div>• ${escapeHtml(n)}</div>`).join('')}
      </div>`
    : '';

  body.innerHTML = notesBanner + vnics.map(vnic => {
    const privRows = (vnic.privateIps || []).map(priv => {
      const pub = priv.publicIp;
      const lifetime = String(pub?.lifetime || '').toUpperCase();
      const isReserved = lifetime === 'RESERVED';
      const allowChangeV4 = pub && (prov === 'azure' || !isReserved);
      const delTitle = prov === 'azure'
        ? '移除/删除公网 IP'
        : (isReserved ? '解绑保留 IP' : '删除公网 IP');

      return `
        <div class="oci-ip-row">
          <span class="oci-ip-addr">${escapeHtml(priv.ipAddress || '未分配')}</span>
          ${priv.isPrimary ? '<span class="oci-ip-badge primary">主私网 IP</span>' : '<span class="oci-ip-badge ephemeral">辅助私网 IP</span>'}
          ${pub ? `
            <span class="oci-ip-addr oci-ip-pub">${escapeHtml(pub.ipAddress)}</span>
            ${getPublicIPBadge(pub, prov)}
            ${allowChangeV4 ? `<button class="secondary-btn oci-ip-btn-sm oci-ip-change-v4-btn" type="button" data-private-id="${escapeAttr(priv.id)}">换IP</button>` : ''}
            <button class="ghost-btn oci-ip-btn-circle oci-ip-btn-danger oci-ip-del-v4-btn" type="button"
              data-vnic-id="${escapeAttr(vnic.id)}"
              data-private-id="${escapeAttr(priv.id)}"
              data-public-id="${escapeAttr(pub.id)}"
              data-public-ip="${escapeAttr(pub.ipAddress)}"
              data-is-primary="${priv.isPrimary ? 'true' : 'false'}"
              data-is-reserved="${isReserved ? 'true' : 'false'}"
              title="${escapeAttr(delTitle)}">×</button>
          ` : `
            <span class="oci-ip-badge ephemeral">无公网 IP</span>
            ${(!priv.isPrimary && caps.canAddSecondaryPrivateIpv4) ? `<button class="ghost-btn oci-ip-btn-circle oci-ip-btn-danger oci-ip-del-priv-btn" type="button" data-private-id="${escapeAttr(priv.id)}" title="删除辅助私网 IP">×</button>` : ''}
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
          ${ip6.canChange && caps.canChangeIpv6 ? `<button class="secondary-btn oci-ip-btn-sm oci-ip-change-v6-btn" type="button" data-vnic-id="${escapeAttr(vnic.id)}" data-ipv6-id="${escapeAttr(ip6.id)}">换IP</button>` : ''}
          ${caps.canDeleteIpv6 ? `<button class="ghost-btn oci-ip-btn-sm oci-ip-btn-danger oci-ip-del-v6-btn" type="button" data-ipv6-id="${escapeAttr(ip6.id)}" data-ip="${escapeAttr(displayAddr)}">删除IP</button>` : ''}
        </div>
      `;
    }).join('');

    const nicSubHint = [vnic.subnetName, vnic.ipv4CIDR || vnic.ipv4Cidr].filter(Boolean).join(' · ');

    return `
      <div class="oci-vnic-card">
        <div class="oci-vnic-header">
          <span class="oci-vnic-title">${escapeHtml(vnic.displayName || vnic.id)}</span>
          ${vnic.isPrimary ? `<span class="oci-ip-badge primary">${prov === 'oci' ? '主 VNIC' : '主网卡'}</span>` : ''}
          <span class="oci-ip-badge attached">${vnic.lifecycleState === 'ATTACHED' ? '已附加' : escapeHtml(vnic.lifecycleState)}</span>
          ${nicSubHint ? `<span class="field-hint" style="margin-left:auto;">${escapeHtml(nicSubHint)}</span>` : ''}
        </div>
        ${vnic.macAddress ? `<div class="oci-vnic-mac">MAC 地址: ${escapeHtml(vnic.macAddress)}</div>` : ''}
        <div class="oci-ip-section-label">IPv4 地址:</div>
        ${privRows || '<div class="field-hint">暂无私网 IP</div>'}
        <div class="oci-ip-section-label">IPv6:</div>
        ${ipv6Rows || `<div class="field-hint">${(vnic.ipv6CidrBlocks || []).length > 0 ? '暂无 IPv6 地址（可点击下方「附加 IPv6」）' : '当前子网未开启 IPv6（可点击下方「开启子网 IPv6」）'}</div>`}
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
  const caps = getIPCapabilities(ociIPInfo);
  const prov = caps.provider;

  if (mode === 'ipv4') {
    const privWithoutPub = [];
    vnics.forEach(v => {
      (v.privateIps || []).forEach(p => {
        if (!p.publicIp) privWithoutPub.push({ vnic: v, priv: p });
      });
    });

    const allowSecondary = caps.canAddSecondaryPrivateIpv4;
    const defaultSecondary = allowSecondary && privWithoutPub.length === 0;

    subform.innerHTML = `
      <div class="oci-ip-subform-title">
        <span>附加 IPv4</span>
        <button class="ghost-btn oci-ip-btn-sm" type="button" id="oci-subform-close">取消</button>
      </div>
      <div class="form-grid">
        <label class="field compact">
          <span>操作类型</span>
          <select id="oci-v4-mode">
            <option value="ephemeral_public" ${privWithoutPub.length === 0 ? 'disabled' : (defaultSecondary ? '' : 'selected')}>${prov === 'azure' ? '为已有内网 IP 分配公网 IPv4' : '为已有私网 IP 分配临时公网 IP'}</option>
            ${allowSecondary ? `<option value="secondary_private" ${defaultSecondary ? 'selected' : ''}>新增辅助私网 IP</option>` : ''}
          </select>
        </label>
        <label class="field compact" id="oci-v4-priv-wrap">
          <span>目标私网 IP</span>
          <select id="oci-v4-priv-select">
            ${privWithoutPub.length > 0
              ? privWithoutPub.map(item => `<option value="${escapeAttr(item.priv.id)}" data-vnic-id="${escapeAttr(item.vnic.id)}">${escapeHtml(item.priv.ipAddress || item.vnic.displayName)} (${escapeHtml(item.vnic.displayName)})</option>`).join('')
              : '<option value="">所有私网 IP 均已绑定公网 IP</option>'}
          </select>
        </label>
        <label class="field compact" id="oci-v4-vnic-wrap" hidden>
          <span>目标网卡</span>
          <select id="oci-v4-vnic-select">
            ${vnics.map(v => `<option value="${escapeAttr(v.id)}">${escapeHtml(v.displayName)} (${escapeHtml(v.ipv4Cidr || '')})</option>`).join('')}
          </select>
        </label>
        <label class="field compact" id="oci-v4-ip-wrap" hidden>
          <span>指定内网 IP（留空自动分配）</span>
          <input id="oci-v4-custom-ip" type="text" placeholder="例如 10.0.0.182">
        </label>
      </div>
      <label class="switch-row compact" id="oci-v4-assign-pub-wrap" style="margin-top: 8px;" hidden>
        <input id="oci-v4-assign-pub" type="checkbox">
        <span>同时分配公网 IPv4</span>
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
    subform.innerHTML = buildOCIEnableIPv6SubformHTML(vnics, defaultVnic.id, prov);
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

    if (!caps.ipv6PrefixConfigurable) {
      const hint = prov === 'azure'
        ? 'Azure 将自动创建 Standard SKU 公网 IPv6（免费）并绑定到所选网卡的双栈 IPv6 配置。'
        : 'GCP 将直接把所选网卡切换为 IPV4_IPV6 双栈并自动分配一个 /96 外部 IPv6 前缀。';
      subform.innerHTML = `
        <div class="oci-ip-subform-title">
          <span>附加 IPv6</span>
          <button class="ghost-btn oci-ip-btn-sm" type="button" id="oci-subform-close">取消</button>
        </div>
        <div class="field-hint" style="margin-bottom: 10px;">${escapeHtml(hint)}</div>
        <div class="form-grid">
          <label class="field compact">
            <span>目标网卡</span>
            <select id="oci-v6-vnic-select">
              ${v6Vnics.map(v => `<option value="${escapeAttr(v.id)}">${escapeHtml(v.displayName)} (${escapeHtml((v.ipv6CidrBlocks || [])[0] || '')})</option>`).join('')}
            </select>
          </label>
        </div>
        <div class="oci-ip-subform-actions">
          <button class="primary-btn oci-ip-btn-sm" type="button" id="oci-v6-submit">确认分配 IPv6</button>
        </div>
      `;
      subform.hidden = false;
      return;
    }

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
    const privCandidates = [];
    vnics.forEach(v => {
      (v.privateIps || []).forEach(p => {
        const isEphemeralGCP = prov === 'gcp' && p.publicIp && String(p.publicIp.lifetime || '').toUpperCase() === 'EPHEMERAL';
        if (!p.publicIp || isEphemeralGCP) {
          privCandidates.push({ vnic: v, priv: p, isEphemeralGCP });
        }
      });
    });
    const reservedList = ociIPInfo.reservedIps || [];
    const defaultNew = reservedList.length === 0 || (prov === 'gcp' && privCandidates.some(c => c.isEphemeralGCP));

    subform.innerHTML = `
      <div class="oci-ip-subform-title">
        <span>${prov === 'azure' ? '附加静态公网 IPv4' : '附加保留 IP'}</span>
        <button class="ghost-btn oci-ip-btn-sm" type="button" id="oci-subform-close">取消</button>
      </div>
      <div class="form-grid">
        <label class="field compact">
          <span>目标内网 IP / 网卡</span>
          <select id="oci-res-priv-select">
            ${privCandidates.length > 0
              ? privCandidates.map(item => `<option value="${escapeAttr(item.priv.id)}" data-vnic-id="${escapeAttr(item.vnic.id)}">${escapeHtml(item.priv.ipAddress || item.vnic.displayName)} (${escapeHtml(item.vnic.displayName)})${item.isEphemeralGCP ? ' · 将固定当前临时 IP' : ''}</option>`).join('')
              : '<option value="">无可用的空余内网 IP（请先解绑现有公网 IP）</option>'}
          </select>
        </label>
        <label class="field compact">
          <span>来源</span>
          <select id="oci-res-mode">
            <option value="existing" ${reservedList.length === 0 ? 'disabled' : (defaultNew ? '' : 'selected')}>选择已有未绑定的静态/保留 IP</option>
            <option value="new" ${defaultNew ? 'selected' : ''}>${prov === 'gcp' ? '新建静态 IP（或固定当前临时 IP）' : '新建并绑定静态/保留 IP'}</option>
          </select>
        </label>
        <label class="field compact" id="oci-res-existing-wrap" ${defaultNew ? 'hidden' : ''}>
          <span>选择静态/保留 IP</span>
          <select id="oci-res-id-select">
            ${reservedList.map(r => `<option value="${escapeAttr(r.id)}">${escapeHtml(r.ipAddress)} (${escapeHtml(r.displayName || r.id)})</option>`).join('')}
          </select>
        </label>
        <label class="field compact" id="oci-res-new-wrap" ${defaultNew ? '' : 'hidden'}>
          <span>新静态 IP 名称（可选）</span>
          <input id="oci-res-new-name" type="text" placeholder="例如 static-ip-1">
        </label>
      </div>
      <div class="oci-ip-subform-actions">
        <button class="primary-btn oci-ip-btn-sm" type="button" id="oci-res-submit" ${privCandidates.length === 0 ? 'disabled' : ''}>确认附加</button>
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
      setOCIIPModalMessage('没有可分配公网 IP 的空余内网 IP。', true);
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
  const caps = getIPCapabilities(ociIPInfo);
  const vnicId = document.getElementById('oci-v6-vnic-select')?.value || '';
  const updateDns = document.getElementById('oci-ip-update-dns-checkbox')?.checked === true;

  const body = { vnicId, updateDns };
  if (caps.ipv6PrefixConfigurable) {
    const ipv6SubnetCidr = document.getElementById('oci-v6-prefix-select')?.value || '';
    const cidrPrefixLength = Number(document.getElementById('oci-v6-prefix-len')?.value || 128);
    const assignMode = document.getElementById('oci-v6-assign-mode')?.value || 'auto';
    const ipAddress = document.getElementById('oci-v6-manual-ip')?.value?.trim() || '';

    if (!Number.isInteger(cidrPrefixLength) || cidrPrefixLength < 80 || cidrPrefixLength > 128) {
      setOCIIPModalMessage('CIDR 前缀长度必须在 80 到 128 之间。', true);
      return;
    }
    if (assignMode === 'manual' && !ipAddress) {
      setOCIIPModalMessage('手动分配模式下请输入 IPv6 地址。', true);
      return;
    }
    body.ipv6SubnetCidr = ipv6SubnetCidr;
    body.cidrPrefixLength = cidrPrefixLength;
    body.assignMode = assignMode;
    body.ipAddress = ipAddress;
  }

  setOCIIPModalMessage('正在分配 IPv6...', false);
  try {
    const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips/attach-ipv6`, {
      method: 'POST',
      body
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
  const privSelect = document.getElementById('oci-res-priv-select');
  const privateIpId = privSelect?.value || '';
  const vnicId = privSelect?.selectedOptions?.[0]?.dataset?.vnicId || '';
  const mode = document.getElementById('oci-res-mode')?.value || 'existing';
  const reservedPublicIpId = document.getElementById('oci-res-id-select')?.value || '';
  const displayName = document.getElementById('oci-res-new-name')?.value?.trim() || '';
  const updateDns = document.getElementById('oci-ip-update-dns-checkbox')?.checked === true;

  if (!privateIpId) {
    setOCIIPModalMessage('没有可用的目标内网 IP / 网卡。', true);
    return;
  }

  setOCIIPModalMessage('正在附加静态/保留 IP...', false);
  try {
    const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips/attach-reserved`, {
      method: 'POST',
      body: {
        vnicId,
        privateIpId,
        reservedPublicIpId,
        createNew: mode === 'new',
        displayName,
        updateDns
      }
    });
    if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
    setOCIIPModalMessage('静态/保留 IP 附加成功。', false, true);
    closeOCIIPSubform();
    await loadOCIIPInfo();
    fetchVMs();
  } catch (err) {
    setOCIIPModalMessage(err.message, true);
    addLog(`附加静态/保留 IP 失败：${err.message}`, 'error');
  }
}

async function handleOCIIPActionClick(event) {
  if (!ociIPModalVM) return;
  const vm = ociIPModalVM;
  const prov = (vm.provider || '').toLowerCase();
  const updateDns = document.getElementById('oci-ip-update-dns-checkbox')?.checked === true;

  const changeV4Btn = event.target.closest('.oci-ip-change-v4-btn');
  if (changeV4Btn) {
    const privateIpId = changeV4Btn.dataset.privateId;
    const msg = prov === 'azure'
      ? '确认更换该公网 IPv4？（将新建 Standard 公网 IP 并自动删除旧 IP 以避免闲置计费）'
      : '确认更换该公网 IPv4？';
    if (!confirm(msg)) return;
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
    const pubIP = delV4Btn.dataset.publicIp || '';

    let deletePublicIp = true;
    if (prov === 'azure') {
      if (!confirm(`确认移除公网 IPv4 (${pubIP})？`)) return;
      deletePublicIp = confirm('是否同时彻底删除该 Azure 公网 IP 资源？\n点「确定」= 解绑并删除（推荐，避免闲置按小时扣费）\n点「取消」= 仅解绑并保留以便之后复用');
    } else if (prov === 'gcp') {
      if (isReserved) {
        if (!confirm(`确认解绑静态保留 IP (${pubIP})？`)) return;
        deletePublicIp = confirm('是否同时删除释放该静态外部 IP 资源？\n点「确定」= 解绑并删除释放（停止计费）\n点「取消」= 仅解绑保留（闲置静态 IP 仍计费）');
      } else {
        if (!confirm(`确认解绑并释放临时外部 IPv4 (${pubIP})？`)) return;
        deletePublicIp = true;
      }
    } else {
      const msg = isReserved
        ? '确认解绑该保留公网 IP？（保留 IP 本身不会被销毁）'
        : '确认删除该临时公网 IP？删除后该 IP 地址无法恢复。';
      if (!confirm(msg)) return;
    }

    const deletePrivateIp = (!isPrimary && prov !== 'gcp')
      ? confirm('是否同时删除该辅助私网 IP？（点“取消”则仅移除公网 IP）')
      : false;

    delV4Btn.disabled = true;
    setOCIIPModalMessage('正在处理...', false);
    try {
      const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips/delete-ipv4`, {
        method: 'POST',
        body: {
          vnicId: delV4Btn.dataset.vnicId || '',
          privateIpId: delV4Btn.dataset.privateId,
          publicIpId: delV4Btn.dataset.publicId,
          deletePrivateIp,
          deletePublicIp
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
    const msg = prov === 'gcp'
      ? 'GCP 更换 IPv6 将先关闭双栈再重新开启，期间 IPv6 会短暂断开（注：GCP 池回收后也可能分配到相同 /96 前缀），确认继续？'
      : 'IPv6 换 IP 将更换新的公网 IPv6 地址并清理旧地址，确认继续？';
    if (!confirm(msg)) return;
    changeV6Btn.disabled = true;
    setOCIIPModalMessage('正在更换 IPv6...', false);
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
    if (!confirm(`确认删除/移除 IPv6 地址 ${delV6Btn.dataset.ip || ''}？`)) return;
    delV6Btn.disabled = true;
    setOCIIPModalMessage('正在移除 IPv6...', false);
    try {
      const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips/delete-ipv6`, {
        method: 'POST',
        body: { ipv6Id: delV6Btn.dataset.ipv6Id }
      });
      if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
      setOCIIPModalMessage('IPv6 已移除。', false, true);
      await loadOCIIPInfo();
      fetchVMs();
    } catch (err) {
      setOCIIPModalMessage(err.message, true);
      addLog(`移除 IPv6 失败：${err.message}`, 'error');
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

  const checkedRadio = document.querySelector('input[name="oci-dns-ipv6"]:checked');
  const ipv6 = checkedRadio ? checkedRadio.value : ociIPSelectedDNSIPv6;

  if (dnsBindings.hasAAAA && !ipv6) {
    setOCIIPModalMessage('实例绑定了 AAAA 记录，但当前没有可用的公网 IPv6 地址。', true);
    return;
  }
  if (!ipv4 && !ipv6) {
    setOCIIPModalMessage('当前没有可用于更新 DNS 的公网 IPv4 或 IPv6 地址。', true);
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

function buildOCIEnableIPv6SubformHTML(vnics, targetVnicId, prov = 'oci') {
  const targetVnic = vnics.find(v => v.id === targetVnicId) || vnics[0];
  const setup = targetVnic?.ipv6Setup || {};
  const hasSub = Array.isArray(targetVnic?.ipv6CidrBlocks) && targetVnic.ipv6CidrBlocks.length > 0;

  if (prov === 'azure') {
    const vnetText = setup.vcnHasIpv6
      ? `已开启 (${(setup.vcnIpv6Cidrs || []).join(', ')})`
      : '未开启（将自动为 VNet 添加 ULA IPv6 /48 地址空间）';
    const subText = hasSub
      ? `已开启 (${(targetVnic.ipv6CidrBlocks || []).join(', ')})`
      : '未开启（将从 VNet IPv6 空间分配空闲 /64 前缀）';
    const skuText = setup.hasBasicSkuConflict
      ? (setup.blockingReason || '检测到 Basic SKU 冲突')
      : '无 Basic SKU 冲突（支持 Standard 公网 IPv6）';
    const suggestedHex = setup.suggestedHex || '01';
    const usedHexes = Array.isArray(setup.usedHexes) ? setup.usedHexes : [];

    return `
      <div class="oci-ip-subform-title">
        <span>一键开启 Azure 双栈 IPv6（VNet /48 → 子网 /64）</span>
        <button class="ghost-btn oci-ip-btn-sm" type="button" id="oci-subform-close">取消</button>
      </div>
      <div class="field-hint" style="margin-bottom: 10px; line-height: 1.65;">
        <div>1. VNet IPv6 地址空间：${setup.vcnHasIpv6 ? '✅' : '⏳'} ${escapeHtml(vnetText)}</div>
        <div>2. 子网 IPv6 (/64)：${hasSub ? '✅' : '⏳'} ${escapeHtml(subText)}</div>
        <div>3. 公网 IP SKU 兼容性：${setup.hasBasicSkuConflict ? '❌' : '✅'} ${escapeHtml(skuText)}</div>
        <div>4. 防火墙提醒：ℹ️ ${escapeHtml(setup.firewallHint || '请确保 NSG 放行所需端口')}</div>
      </div>
      <div class="form-grid">
        <label class="field compact">
          <span>目标网卡</span>
          <select id="oci-enable-v6-vnic">
            ${vnics.map(v => `<option value="${escapeAttr(v.id)}" ${v.id === targetVnic.id ? 'selected' : ''}>${escapeHtml(v.displayName)}</option>`).join('')}
          </select>
        </label>
        <label class="field compact">
          <span>子网 /64 前缀 Hex（00-FF）</span>
          <input id="oci-enable-v6-hex" type="text" maxlength="2" value="${escapeAttr(suggestedHex)}" placeholder="例如 01" ${hasSub ? 'disabled' : ''}>
          ${usedHexes.length ? `<small class="field-hint">同 VNet 已占用 Hex：${escapeHtml(usedHexes.join(', '))}</small>` : ''}
        </label>
      </div>
      <div class="oci-ip-subform-actions">
        <button class="primary-btn oci-ip-btn-sm" type="button" id="oci-enable-v6-submit" ${setup.canEnable === false ? 'disabled' : ''}>${hasSub ? '子网已就绪' : '确认开启子网双栈'}</button>
      </div>
    `;
  }

  if (prov === 'gcp') {
    const vpcText = setup.isAutoModeVpc
      ? '默认 Auto Mode VPC（开启外部 IPv6 需先不可逆转换为 Custom Mode）'
      : 'Custom Mode VPC（支持直接为子网开启外部 IPv6）';
    const subText = hasSub
      ? `已开启 EXTERNAL 双栈 (${(targetVnic.ipv6CidrBlocks || []).join(', ') || 'IPV4_IPV6'})`
      : (setup.blockingReason || '未开启（将更新子网为 IPV4_IPV6 + EXTERNAL）');
    const fwText = setup.needsFirewallIpv6Rule
      ? (setup.firewallHint || '未检测到 ::/0 入站规则')
      : '已检测到包含 ::/0 的 IPv6 防火墙规则';

    return `
      <div class="oci-ip-subform-title">
        <span>一键开启 GCP 子网外部 IPv6 双栈</span>
        <button class="ghost-btn oci-ip-btn-sm" type="button" id="oci-subform-close">取消</button>
      </div>
      <div class="field-hint" style="margin-bottom: 10px; line-height: 1.65;">
        <div>1. VPC 模式：${setup.isAutoModeVpc ? '⚠️' : '✅'} ${escapeHtml(vpcText)}</div>
        <div>2. 子网双栈状态：${hasSub ? '✅' : (setup.canEnable === false ? '❌' : '⏳')} ${escapeHtml(subText)}</div>
        <div>3. 防火墙规则：${setup.needsFirewallIpv6Rule ? 'ℹ️' : '✅'} ${escapeHtml(fwText)}</div>
      </div>
      <div class="form-grid">
        <label class="field compact">
          <span>目标网卡</span>
          <select id="oci-enable-v6-vnic">
            ${vnics.map(v => `<option value="${escapeAttr(v.id)}" ${v.id === targetVnic.id ? 'selected' : ''}>${escapeHtml(v.displayName)}</option>`).join('')}
          </select>
        </label>
      </div>
      ${setup.requiresConfirmation ? `
        <label class="switch-row compact" style="margin-top: 8px; color: var(--amber, #f59e0b);">
          <input id="gcp-enable-v6-confirm-irreversible" type="checkbox">
          <span>我已了解并确认：将当前默认 VPC 从 Auto Mode 不可逆转换为 Custom Mode（现有实例与子网不受影响）</span>
        </label>
      ` : ''}
      <div class="oci-ip-subform-actions">
        <button class="primary-btn oci-ip-btn-sm" type="button" id="oci-enable-v6-submit" ${setup.canEnable === false ? 'disabled' : ''}>确认开启子网 IPv6</button>
      </div>
    `;
  }

  // OCI default wizard
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
  const prov = (vm.provider || '').toLowerCase();
  const vnicId = document.getElementById('oci-enable-v6-vnic')?.value || '';
  const hexInput = document.getElementById('oci-enable-v6-hex');
  const hexByte = hexInput?.value?.trim() || '';
  const addDefaultRoute = Boolean(document.getElementById('oci-enable-v6-add-route')?.checked);
  const addSecurityListEgress = Boolean(document.getElementById('oci-enable-v6-add-sl-egress')?.checked);
  const confirmCheckbox = document.getElementById('gcp-enable-v6-confirm-irreversible');
  const confirmIrreversible = Boolean(confirmCheckbox?.checked);

  if (prov === 'gcp' && confirmCheckbox && !confirmIrreversible) {
    setOCIIPModalMessage('请先勾选确认框：默认 Auto Mode VPC 转换为 Custom Mode 不可逆。', true);
    return;
  }

  if (hexInput && !hexInput.disabled && !/^[0-9a-fA-F]{2}$/.test(hexByte)) {
    setOCIIPModalMessage('请输入有效的 2 位十六进制字符（00 - FF）。', true);
    return;
  }

  const btn = document.getElementById('oci-enable-v6-submit');
  if (btn) btn.disabled = true;
  setOCIIPModalMessage('正在配置网络与子网 IPv6...', false);
  try {
    const res = await fetchJSON(`/api/vm/${encodeURIComponent(vm.provider)}/${encodeURIComponent(vm.accountId)}/${encodeURIComponent(vm.id)}/ips/enable-ipv6`, {
      method: 'POST',
      body: { vnicId, hexByte, addDefaultRoute, addSecurityListEgress, confirmIrreversible }
    });
    if (Array.isArray(res.logs)) res.logs.forEach(l => addLog(l, 'info'));
    setOCIIPModalMessage(`子网 IPv6 已开启：${res.subnetCidr || ''}。`, false, true);
    closeOCIIPSubform();
    await loadOCIIPInfo();
    const hasSubV6 = (ociIPInfo?.vnics || []).some(v => Array.isArray(v.ipv6CidrBlocks) && v.ipv6CidrBlocks.length > 0);
    if (hasSubV6) {
      openOCIIPSubform('ipv6');
    }
    fetchVMs();
  } catch (err) {
    setOCIIPModalMessage(err.message, true);
    addLog(`开启子网 IPv6 失败：${err.message}`, 'error');
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
      const caps = getIPCapabilities(ociIPInfo);
      if (sf) sf.innerHTML = buildOCIEnableIPv6SubformHTML(ociIPInfo.vnics || [], e.target.value, caps.provider);
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
