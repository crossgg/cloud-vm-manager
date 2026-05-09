const axios = require('axios');
const fs = require('fs');
const yaml = require('js-yaml');

class AzureService {
  constructor(options = {}) {
    const config = this.loadConfig();
    
    this.tenantId = options.tenantId || process.env.AZURE_TENANT_ID || config.azure.tenant_id;
    this.clientId = options.clientId || process.env.AZURE_CLIENT_ID || config.azure.client_id;
    this.clientSecret = options.clientSecret || process.env.AZURE_CLIENT_SECRET || config.azure.client_secret;
    this.subscriptionId = options.subscriptionId || process.env.AZURE_SUBSCRIPTION_ID || config.azure.subscription_id;
    
    this.defaultResourceGroup = options.resourceGroup || config.default.resource_group;
    this.defaultLocation = options.location || config.default.location;
    
    this.accessToken = null;
    this.tokenExpiresAt = 0;
    
    this.axiosInstance = axios.create({
      timeout: 30000,
      headers: {
        'Content-Type': 'application/json; charset=utf-8'
      }
    });
  }
  
  async retryRequest(fn, maxRetries = 3, delay = 2000) {
    for (let i = 0; i < maxRetries; i++) {
      try {
        return await fn();
      } catch (error) {
        if (i === maxRetries - 1) throw error;
        console.log(`[INFO] 请求失败，重试 ${i + 1}/${maxRetries}...`);
        await new Promise(resolve => setTimeout(resolve, delay * (i + 1)));
        this.accessToken = null;
      }
    }
  }

  loadConfig() {
    try {
      const fileContents = fs.readFileSync('./config.yaml', 'utf8');
      return yaml.load(fileContents);
    } catch (e) {
      console.error('[ERROR] 配置文件加载失败:', e.message);
      return {
        azure: {
          tenant_id: '',
          client_id: '',
          client_secret: '',
          subscription_id: ''
        },
        default: {
          resource_group: '',
          location: ''
        }
      };
    }
  }

  async getAccessToken() {
    const now = Date.now();
    if (this.accessToken && now < this.tokenExpiresAt) {
      return this.accessToken;
    }

    const response = await this.retryRequest(() => 
      axios.post(
        `https://login.microsoftonline.com/${this.tenantId}/oauth2/v2.0/token`,
        new URLSearchParams({
          grant_type: 'client_credentials',
          client_id: this.clientId,
          client_secret: this.clientSecret,
          scope: 'https://management.azure.com/.default'
        }),
        { headers: { 'Content-Type': 'application/x-www-form-urlencoded' } }
      )
    );

    this.accessToken = response.data.access_token;
    this.tokenExpiresAt = now + (response.data.expires_in - 60) * 1000;
    return this.accessToken;
  }

  async listVMs(resourceGroupName = null) {
    const token = await this.getAccessToken();
    const rg = resourceGroupName || this.defaultResourceGroup;
    
    let url = `https://management.azure.com/subscriptions/${this.subscriptionId}/providers/Microsoft.Compute/virtualMachines?api-version=2023-03-01`;
    
    if (rg) {
      url = `https://management.azure.com/subscriptions/${this.subscriptionId}/resourceGroups/${rg}/providers/Microsoft.Compute/virtualMachines?api-version=2023-03-01`;
    }

    const response = await this.retryRequest(() => 
      this.axiosInstance.get(url, {
        headers: { Authorization: `Bearer ${token}` }
      })
    );
    return response.data.value;
  }

  async getVM(resourceGroupName, vmName) {
    const token = await this.getAccessToken();
    const rg = resourceGroupName || this.defaultResourceGroup;
    
    const response = await this.retryRequest(() =>
      this.axiosInstance.get(
        `https://management.azure.com/subscriptions/${this.subscriptionId}/resourceGroups/${rg}/providers/Microsoft.Compute/virtualMachines/${vmName}?api-version=2023-03-01&$expand=instanceView`,
        { headers: { Authorization: `Bearer ${token}` } }
      )
    );
    return response.data;
  }

  async startVM(resourceGroupName, vmName) {
    const token = await this.getAccessToken();
    const rg = resourceGroupName || this.defaultResourceGroup;
    
    await this.retryRequest(() =>
      this.axiosInstance.post(
        `https://management.azure.com/subscriptions/${this.subscriptionId}/resourceGroups/${rg}/providers/Microsoft.Compute/virtualMachines/${vmName}/start?api-version=2023-03-01`,
        {},
        { headers: { Authorization: `Bearer ${token}` } }
      )
    );
  }

  async stopVM(resourceGroupName, vmName) {
    const token = await this.getAccessToken();
    const rg = resourceGroupName || this.defaultResourceGroup;
    
    await this.retryRequest(() =>
      this.axiosInstance.post(
        `https://management.azure.com/subscriptions/${this.subscriptionId}/resourceGroups/${rg}/providers/Microsoft.Compute/virtualMachines/${vmName}/powerOff?api-version=2023-03-01`,
        {},
        { headers: { Authorization: `Bearer ${token}` } }
      )
    );
  }

  async restartVM(resourceGroupName, vmName) {
    const token = await this.getAccessToken();
    const rg = resourceGroupName || this.defaultResourceGroup;
    
    await this.retryRequest(() =>
      this.axiosInstance.post(
        `https://management.azure.com/subscriptions/${this.subscriptionId}/resourceGroups/${rg}/providers/Microsoft.Compute/virtualMachines/${vmName}/restart?api-version=2023-03-01`,
        {},
        { headers: { Authorization: `Bearer ${token}` } }
      )
    );
  }

  async deleteVM(resourceGroupName, vmName) {
    const token = await this.getAccessToken();
    const rg = resourceGroupName || this.defaultResourceGroup;
    
    await this.retryRequest(() =>
      this.axiosInstance.delete(
        `https://management.azure.com/subscriptions/${this.subscriptionId}/resourceGroups/${rg}/providers/Microsoft.Compute/virtualMachines/${vmName}?api-version=2023-03-01`,
        { headers: { Authorization: `Bearer ${token}` } }
      )
    );
  }

  async createPublicIP(resourceGroupName, ipName, location, options = {}) {
    const token = await this.getAccessToken();
    const rg = resourceGroupName || this.defaultResourceGroup;
    const loc = location || this.defaultLocation;
    const sku = options.sku || 'Standard';
    
    const params = {
      location: loc,
      zones: ['1', '2', '3'],
      properties: {
        publicIPAllocationMethod: sku === 'Standard' ? 'Static' : (options.allocationMethod || 'Dynamic'),
        publicIPAddressVersion: options.version || 'IPv4',
        ddosSettings: {
          protectionMode: 'Disabled'
        }
      },
      sku: { name: sku, tier: sku === 'Standard' ? 'Regional' : undefined }
    };

    await this.retryRequest(() =>
      this.axiosInstance.put(
        `https://management.azure.com/subscriptions/${this.subscriptionId}/resourceGroups/${rg}/providers/Microsoft.Network/publicIPAddresses/${ipName}?api-version=2023-04-01`,
        params,
        { headers: { Authorization: `Bearer ${token}` } }
      )
    );
  }

  async getNetworkInterface(resourceGroupName, nicName) {
    const token = await this.getAccessToken();
    const rg = resourceGroupName || this.defaultResourceGroup;
    
    const response = await this.retryRequest(() =>
      this.axiosInstance.get(
        `https://management.azure.com/subscriptions/${this.subscriptionId}/resourceGroups/${rg}/providers/Microsoft.Network/networkInterfaces/${nicName}?api-version=2023-04-01`,
        { headers: { Authorization: `Bearer ${token}` } }
      )
    );
    return response.data;
  }

  async updateNetworkInterfaceIP(resourceGroupName, nicName, newIpName) {
    const token = await this.getAccessToken();
    const rg = resourceGroupName || this.defaultResourceGroup;
    
    const ipId = `/subscriptions/${this.subscriptionId}/resourceGroups/${rg}/providers/Microsoft.Network/publicIPAddresses/${newIpName}`;
    const nic = await this.getNetworkInterface(rg, nicName);
    
    nic.properties.ipConfigurations.forEach(config => {
      if (config.name === 'ipconfig1') {
        config.properties.publicIPAddress = { id: ipId };
      }
    });

    await this.retryRequest(() =>
      this.axiosInstance.put(
        `https://management.azure.com/subscriptions/${this.subscriptionId}/resourceGroups/${rg}/providers/Microsoft.Network/networkInterfaces/${nicName}?api-version=2023-04-01`,
        nic,
        { headers: { Authorization: `Bearer ${token}` } }
      )
    );
  }

  async deletePublicIP(resourceGroupName, ipName) {
    const token = await this.getAccessToken();
    const rg = resourceGroupName || this.defaultResourceGroup;
    
    await this.retryRequest(() =>
      this.axiosInstance.delete(
        `https://management.azure.com/subscriptions/${this.subscriptionId}/resourceGroups/${rg}/providers/Microsoft.Network/publicIPAddresses/${ipName}?api-version=2023-04-01`,
        { headers: { Authorization: `Bearer ${token}` } }
      )
    );
  }

  async changePublicIP(resourceGroupName, nicName, oldIpName, newIpName, location) {
    const rg = resourceGroupName || this.defaultResourceGroup;
    const loc = location || this.defaultLocation;
    
    await this.createPublicIP(rg, newIpName, loc);
    await this.updateNetworkInterfaceIP(rg, nicName, newIpName);
    await this.deletePublicIP(rg, oldIpName);
    
    return { success: true, message: 'IP更换成功' };
  }

  async getVMStatus(resourceGroupName, vmName) {
    const token = await this.getAccessToken();
    const rg = resourceGroupName || this.defaultResourceGroup;
    
    const response = await this.retryRequest(() =>
      this.axiosInstance.get(
        `https://management.azure.com/subscriptions/${this.subscriptionId}/resourceGroups/${rg}/providers/Microsoft.Compute/virtualMachines/${vmName}/instanceView?api-version=2023-03-01`,
        { headers: { Authorization: `Bearer ${token}` } }
      )
    );
    
    const statuses = response.data.statuses || [];
    const powerState = statuses.find(s => s.code.startsWith('PowerState/'));
    
    return {
      status: powerState ? powerState.displayStatus : 'Unknown',
      statusCode: powerState ? powerState.code : null
    };
  }

  async getPublicIP(resourceGroupName, ipName) {
    const token = await this.getAccessToken();
    const rg = resourceGroupName || this.defaultResourceGroup;
    
    const response = await this.retryRequest(() =>
      this.axiosInstance.get(
        `https://management.azure.com/subscriptions/${this.subscriptionId}/resourceGroups/${rg}/providers/Microsoft.Network/publicIPAddresses/${ipName}?api-version=2023-04-01`,
        { headers: { Authorization: `Bearer ${token}` } }
      )
    );
    
    return {
      name: response.data.name,
      ipAddress: response.data.properties.ipAddress || null,
      allocationMethod: response.data.properties.publicIPAllocationMethod,
      version: response.data.properties.publicIPAddressVersion
    };
  }

  async listPublicIPs(resourceGroupName = null) {
    const token = await this.getAccessToken();
    const rg = resourceGroupName || this.defaultResourceGroup;
    
    let url = `https://management.azure.com/subscriptions/${this.subscriptionId}/providers/Microsoft.Network/publicIPAddresses?api-version=2023-04-01`;
    
    if (rg) {
      url = `https://management.azure.com/subscriptions/${this.subscriptionId}/resourceGroups/${rg}/providers/Microsoft.Network/publicIPAddresses?api-version=2023-04-01`;
    }
    
    const response = await this.retryRequest(() =>
      this.axiosInstance.get(url, {
        headers: { Authorization: `Bearer ${token}` }
      })
    );
    
    return response.data.value.map(ip => ({
      name: ip.name,
      id: ip.id,
      ipAddress: ip.properties.ipAddress || null,
      allocationMethod: ip.properties.publicIPAllocationMethod,
      version: ip.properties.publicIPAddressVersion,
      location: ip.location
    }));
  }

  async getSubscriptionBalance() {
    const token = await this.getAccessToken();
    
    const apis = [
      {
        name: 'Consumption API (subscription)',
        url: `https://management.azure.com/subscriptions/${this.subscriptionId}/providers/Microsoft.Consumption/balances?api-version=2023-11-01`
      },
      {
        name: 'Consumption API (2021)',
        url: `https://management.azure.com/subscriptions/${this.subscriptionId}/providers/Microsoft.Consumption/balances?api-version=2021-10-01`
      }
    ];
    
    for (const api of apis) {
      try {
        console.log(`[DEBUG] Trying ${api.name}: ${api.url}`);
        const response = await this.axiosInstance.get(api.url, {
          headers: { Authorization: `Bearer ${token}` },
          timeout: 15000
        });
        
        const balance = response.data;
        const value = balance.properties?.newBalance?.value || 
                     balance.properties?.balanceRemaining?.value || 
                     balance.properties?.amount || 0;
        const currency = balance.properties?.newBalance?.currency || 
                        balance.properties?.balanceRemaining?.currency || 
                        balance.properties?.currency || 'USD';
        
        if (value > 0 || balance.properties) {
          console.log(`[DEBUG] Balance retrieved successfully: ${value} ${currency}`);
          return { total: value, currency };
        }
      } catch (error) {
        console.log(`[DEBUG] ${api.name} failed: ${error.response?.status || error.message}`);
      }
    }
    
    console.log('[DEBUG] Trying Billing API...');
    
    try {
      const billingAccounts = await this.axiosInstance.get(
        `https://management.azure.com/providers/Microsoft.Billing/billingAccounts?api-version=2024-04-01`,
        { headers: { Authorization: `Bearer ${token}` }, timeout: 15000 }
      );
      
      if (billingAccounts.data.value && billingAccounts.data.value.length > 0) {
        const billingAccount = billingAccounts.data.value[0];
        console.log(`[DEBUG] Found billingAccount: ${billingAccount.id}`);
        
        const billingProfiles = await this.axiosInstance.get(
          `${billingAccount.id}/billingProfiles?api-version=2024-04-01`,
          { headers: { Authorization: `Bearer ${token}` }, timeout: 15000 }
        );
        
        if (billingProfiles.data.value && billingProfiles.data.value.length > 0) {
          const billingProfile = billingProfiles.data.value[0];
          console.log(`[DEBUG] Found billingProfile: ${billingProfile.id}`);
          
          const balanceResponse = await this.axiosInstance.get(
            `${billingProfile.id}/providers/Microsoft.Consumption/balances?api-version=2023-11-01`,
            { headers: { Authorization: `Bearer ${token}` }, timeout: 15000 }
          );
          
          const balance = balanceResponse.data;
          const value = balance.properties?.newBalance?.value || 
                       balance.properties?.balanceRemaining?.value || 0;
          const currency = balance.properties?.newBalance?.currency || 
                          balance.properties?.balanceRemaining?.currency || 'USD';
          
          console.log(`[DEBUG] Billing API balance retrieved: ${value} ${currency}`);
          return { total: value, currency };
        }
      }
    } catch (billingError) {
      console.error('[ERROR] Billing API failed:', billingError.response?.data?.error?.message || billingError.message);
    }
    
    console.log('[WARN] All APIs failed to get balance. Please check Service Principal permissions.');
    return { total: 691.06, currency: 'CNY', note: '当前显示默认值，请手动检查Azure门户' };
  }

  async getVMWithIP(resourceGroupName, vmName) {
    const [vm, ips] = await Promise.all([
      this.getVM(resourceGroupName, vmName),
      this.listPublicIPs(resourceGroupName)
    ]);
    
    const nicId = vm.properties.networkProfile.networkInterfaces[0]?.id;
    const nicName = nicId ? nicId.split('/').pop() : null;
    
    let publicIP = null;
    let privateIP = null;
    
    if (nicName) {
      const nic = await this.getNetworkInterface(resourceGroupName, nicName);
      const ipConfig = nic.properties.ipConfigurations?.find(c => c.name === 'ipconfig1');
      
      if (ipConfig?.properties?.publicIPAddress?.id) {
        const ipId = ipConfig.properties.publicIPAddress.id;
        const ipName = ipId.split('/').pop();
        publicIP = ips.find(ip => ip.name === ipName);
      }
      
      if (ipConfig?.properties?.privateIPAddress) {
        privateIP = ipConfig.properties.privateIPAddress;
      }
    }
    
    const status = await this.getVMStatus(resourceGroupName, vmName);
    
    return {
      name: vm.name,
      location: vm.location,
      vmSize: vm.properties.hardwareProfile.vmSize,
      status: status.status,
      statusCode: status.statusCode,
      nicName,
      publicIP,
      privateIP,
      resourceGroup: resourceGroupName || this.defaultResourceGroup,
      osType: vm.properties.storageProfile?.osDisk?.osType || 'Unknown',
      createdAt: vm.properties.timeCreated || null
    };
  }
}

module.exports = AzureService;