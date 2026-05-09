const API_BASE = '';

function addLog(message, type = 'info') {
  const logsContainer = document.getElementById('logs');
  const timestamp = new Date().toLocaleTimeString('zh-CN');
  const logEntry = document.createElement('div');
  logEntry.className = `log-entry log-${type}`;
  logEntry.innerHTML = `<span class="log-time">[${timestamp}]</span>${message}`;
  logsContainer.prepend(logEntry);
  
  while (logsContainer.children.length > 50) {
    logsContainer.removeChild(logsContainer.lastChild);
  }
}

function clearLogs() {
  document.getElementById('logs').innerHTML = '';
  addLog('日志已清空', 'info');
}

async function fetchBalance() {
  try {
    const response = await fetch(`${API_BASE}/api/balance`);
    const data = await response.json();
    
    if (data.error) {
      document.getElementById('balance').textContent = '获取失败';
      addLog(`获取余额失败: ${data.error}`, 'error');
      return;
    }
    
    let displayText = `${data.total.toFixed(2)} ${data.currency}`;
    if (data.note) {
      displayText += ' *';
    }
    
    document.getElementById('balance').textContent = displayText;
    
    if (data.note) {
      addLog(`获取余额: ${data.total.toFixed(2)} ${data.currency} (${data.note})`, 'info');
    } else {
      addLog(`获取余额成功: ${data.total.toFixed(2)} ${data.currency}`, 'success');
    }
  } catch (error) {
    document.getElementById('balance').textContent = '获取失败';
    addLog(`获取余额异常: ${error.message}`, 'error');
  }
}

async function fetchVMs() {
  try {
    const vmList = document.getElementById('vm-list');
    vmList.innerHTML = '<div class="loading">加载中...</div>';
    
    const response = await fetch(`${API_BASE}/api/vms`);
    const vms = await response.json();
    
    if (vms.error) {
      vmList.innerHTML = `<div class="loading">加载失败: ${vms.error}</div>`;
      addLog(`获取VM列表失败: ${vms.error}`, 'error');
      return;
    }
    
    updateStats(vms);
    renderVMList(vms);
    addLog(`成功获取 ${vms.length} 个VM实例`, 'success');
  } catch (error) {
    document.getElementById('vm-list').innerHTML = `<div class="loading">加载异常: ${error.message}</div>`;
    addLog(`获取VM列表异常: ${error.message}`, 'error');
  }
}

function updateStats(vms) {
  const running = vms.filter(v => v.status === 'VM running').length;
  const stopped = vms.filter(v => v.status === 'VM deallocated' || v.status === 'VM stopped').length;
  
  document.getElementById('vm-count').textContent = vms.length;
  document.getElementById('running-count').textContent = running;
  document.getElementById('stopped-count').textContent = stopped;
}

function renderVMList(vms) {
  const vmList = document.getElementById('vm-list');
  
  if (vms.length === 0) {
    vmList.innerHTML = '<div class="loading">暂无VM实例</div>';
    return;
  }
  
  vmList.innerHTML = vms.map(vm => `
    <div class="vm-card">
      <div class="vm-header">
        <span class="vm-name">${vm.name}</span>
        <span class="status-badge status-${getStatusClass(vm.status)}">${getStatusText(vm.status)}</span>
      </div>
      <div class="vm-info">
        <div class="info-item">
          <span class="info-label">区域</span>
          <span class="info-value">${getLocationText(vm.location)}</span>
        </div>
        <div class="info-item">
          <span class="info-label">机器类型</span>
          <span class="info-value">${vm.vmSize}</span>
        </div>
        <div class="info-item">
          <span class="info-label">公网IP</span>
          <span class="info-value mono">${vm.publicIP?.ipAddress || '未分配'}</span>
        </div>
        <div class="info-item">
          <span class="info-label">公网IP名称</span>
          <span class="info-value">${vm.publicIP?.name || 'N/A'}</span>
        </div>
        <div class="info-item">
          <span class="info-label">内网IP</span>
          <span class="info-value mono">${vm.privateIP || '未分配'}</span>
        </div>
        <div class="info-item">
          <span class="info-label">资源组</span>
          <span class="info-value">${vm.resourceGroup}</span>
        </div>
      </div>
      <div class="vm-actions">
        <button class="action-btn start" onclick="handleStart('${vm.name}', '${vm.status}')" ${vm.status === 'VM running' ? 'disabled' : ''}>
          ▶️ 开机
        </button>
        <button class="action-btn stop" onclick="handleStop('${vm.name}', '${vm.status}')" ${vm.status !== 'VM running' ? 'disabled' : ''}>
          ⏹️ 关机
        </button>
        <button class="action-btn restart" onclick="handleRestart('${vm.name}')">
          🔄 重启
        </button>
        <button class="action-btn change-ip" onclick="handleChangeIP('${vm.name}')">
          🔀 换IP
        </button>
      </div>
    </div>
  `).join('');
}

function getStatusClass(status) {
  if (status === 'VM running') return 'running';
  if (status === 'VM deallocated' || status === 'VM stopped') return 'stopped';
  return 'unknown';
}

function getStatusText(status) {
  if (status === 'VM running') return '运行中';
  if (status === 'VM deallocated') return '已停止';
  if (status === 'VM stopped') return '已停止';
  return status;
}

function getLocationText(location) {
  const locations = {
    'koreacentral': '韩国中部',
    'koreasouth': '韩国南部',
    'eastasia': '东亚',
    'southeastasia': '东南亚',
    'centralus': '美国中部',
    'eastus': '美国东部',
    'westus': '美国西部'
  };
  return locations[location] || location;
}

async function handleStart(vmName, currentStatus) {
  if (currentStatus === 'VM running') {
    addLog(`VM ${vmName} 已经在运行中`, 'info');
    return;
  }
  
  addLog(`正在启动VM: ${vmName}`, 'info');
  
  try {
    const response = await fetch(`${API_BASE}/api/vm/${vmName}/start`, { method: 'POST' });
    const data = await response.json();
    
    if (data.error) {
      addLog(`启动失败: ${data.error}`, 'error');
      return;
    }
    
    addLog(`${data.message}`, 'success');
    setTimeout(() => refreshVM(vmName), 3000);
  } catch (error) {
    addLog(`启动异常: ${error.message}`, 'error');
  }
}

async function handleStop(vmName, currentStatus) {
  if (currentStatus !== 'VM running') {
    addLog(`VM ${vmName} 已经停止`, 'info');
    return;
  }
  
  addLog(`正在停止VM: ${vmName}`, 'info');
  
  try {
    const response = await fetch(`${API_BASE}/api/vm/${vmName}/stop`, { method: 'POST' });
    const data = await response.json();
    
    if (data.error) {
      addLog(`停止失败: ${data.error}`, 'error');
      return;
    }
    
    addLog(`${data.message}`, 'success');
    setTimeout(() => refreshVM(vmName), 5000);
  } catch (error) {
    addLog(`停止异常: ${error.message}`, 'error');
  }
}

async function handleRestart(vmName) {
  addLog(`正在重启VM: ${vmName}`, 'info');
  
  try {
    const response = await fetch(`${API_BASE}/api/vm/${vmName}/restart`, { method: 'POST' });
    const data = await response.json();
    
    if (data.error) {
      addLog(`重启失败: ${data.error}`, 'error');
      return;
    }
    
    addLog(`${data.message}`, 'success');
    setTimeout(() => refreshVM(vmName), 8000);
  } catch (error) {
    addLog(`重启异常: ${error.message}`, 'error');
  }
}

async function handleChangeIP(vmName) {
  addLog(`正在更换VM ${vmName} 的公网IP...`, 'info');
  
  try {
    const response = await fetch(`${API_BASE}/api/vm/${vmName}/change-ip`, { method: 'POST' });
    const data = await response.json();
    
    if (data.error) {
      addLog(`更换IP失败: ${data.error}`, 'error');
      return;
    }
    
    addLog(`IP更换成功! 新IP: ${data.newIpAddress}`, 'success');
    setTimeout(() => refreshVM(vmName), 2000);
  } catch (error) {
    addLog(`更换IP异常: ${error.message}`, 'error');
  }
}

async function refreshVM(vmName) {
  try {
    const response = await fetch(`${API_BASE}/api/refresh/${vmName}`);
    const vm = await response.json();
    
    if (vm.error) {
      addLog(`刷新VM ${vmName} 失败: ${vm.error}`, 'error');
      return;
    }
    
    addLog(`VM ${vmName} 状态已更新: ${getStatusText(vm.status)}`, 'info');
    fetchVMs();
  } catch (error) {
    addLog(`刷新VM异常: ${error.message}`, 'error');
  }
}

function refreshVMs() {
  addLog('正在刷新VM列表...', 'info');
  fetchVMs();
}

document.addEventListener('DOMContentLoaded', () => {
  const navItems = document.querySelectorAll('.nav-item');
  navItems.forEach(item => {
    item.addEventListener('click', (e) => {
      e.preventDefault();
      const section = item.dataset.section;
      
      navItems.forEach(i => i.classList.remove('active'));
      item.classList.add('active');
      
      document.querySelectorAll('.section').forEach(s => s.classList.remove('active'));
      document.getElementById(`${section}-section`).classList.add('active');
    });
  });
  
  fetchBalance();
  fetchVMs();
});