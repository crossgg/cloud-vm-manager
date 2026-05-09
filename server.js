process.env.NODE_ENV = 'production';

if (process.platform === 'win32') {
  const { spawnSync } = require('child_process');
  spawnSync('chcp', ['65001'], { stdio: 'ignore' });
}

const express = require('express');
const cors = require('cors');
const AzureService = require('./azure-service');

const app = express();
const port = 3000;

app.use(cors());
app.use(express.json());
app.use(express.static('public'));

const azure = new AzureService();

function logAction(action, vmName, details = '') {
  const timestamp = new Date().toISOString();
  console.log(`[${timestamp}] [${action}] VM: ${vmName} ${details}`);
}

app.get('/api/vms', async (req, res) => {
  try {
    const vms = await azure.listVMs();
    const vmDetails = await Promise.all(vms.map(vm => azure.getVMWithIP(null, vm.name)));
    res.json(vmDetails);
  } catch (error) {
    console.error('[ERROR] Failed to get VM list:', error.message);
    res.status(500).json({ error: error.message });
  }
});

app.get('/api/vm/:name', async (req, res) => {
  try {
    const vmName = req.params.name;
    const vm = await azure.getVMWithIP(null, vmName);
    res.json(vm);
  } catch (error) {
    console.error('[ERROR] Failed to get VM details:', error.message);
    res.status(500).json({ error: error.message });
  }
});

app.post('/api/vm/:name/start', async (req, res) => {
  try {
    const vmName = req.params.name;
    logAction('START', vmName);
    await azure.startVM(null, vmName);
    res.json({ success: true, message: `Starting VM: ${vmName}` });
  } catch (error) {
    console.error('[ERROR] Failed to start VM:', error.message);
    res.status(500).json({ error: error.message });
  }
});

app.post('/api/vm/:name/stop', async (req, res) => {
  try {
    const vmName = req.params.name;
    logAction('STOP', vmName);
    await azure.stopVM(null, vmName);
    res.json({ success: true, message: `Stopping VM: ${vmName}` });
  } catch (error) {
    console.error('[ERROR] Failed to stop VM:', error.message);
    res.status(500).json({ error: error.message });
  }
});

app.post('/api/vm/:name/restart', async (req, res) => {
  try {
    const vmName = req.params.name;
    logAction('RESTART', vmName);
    await azure.restartVM(null, vmName);
    res.json({ success: true, message: `Restarting VM: ${vmName}` });
  } catch (error) {
    console.error('[ERROR] Failed to restart VM:', error.message);
    res.status(500).json({ error: error.message });
  }
});

app.post('/api/vm/:name/change-ip', async (req, res) => {
  try {
    const vmName = req.params.name;
    const vm = await azure.getVMWithIP(null, vmName);
    
    if (!vm.publicIP || !vm.nicName) {
      return res.status(400).json({ error: 'Cannot get current IP or NIC info' });
    }
    
    const oldIpName = vm.publicIP.name;
    const newIpName = 'new-ip-' + Date.now();
    
    logAction('CHANGE_IP', vmName, `Old IP: ${oldIpName} -> New IP: ${newIpName}`);
    
    const result = await azure.changePublicIP(null, vm.nicName, oldIpName, newIpName, null);
    
    const newVm = await azure.getVMWithIP(null, vmName);
    result.newIpAddress = newVm.publicIP?.ipAddress || 'Not assigned';
    
    res.json(result);
  } catch (error) {
    console.error('[ERROR] Failed to change IP:', error.message);
    res.status(500).json({ error: error.message });
  }
});

app.get('/api/balance', async (req, res) => {
  try {
    const balance = await azure.getSubscriptionBalance();
    res.json(balance);
  } catch (error) {
    console.error('[ERROR] Failed to get balance:', error.message);
    res.status(500).json({ error: error.message });
  }
});

app.get('/api/refresh/:name', async (req, res) => {
  try {
    const vmName = req.params.name;
    const vm = await azure.getVMWithIP(null, vmName);
    res.json(vm);
  } catch (error) {
    console.error('[ERROR] Failed to refresh VM info:', error.message);
    res.status(500).json({ error: error.message });
  }
});

app.listen(port, () => {
  console.log(`[INFO] Server running on http://localhost:${port}`);
});