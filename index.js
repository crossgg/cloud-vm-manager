const AzureService = require('./azure-service');

async function main() {
  const azure = new AzureService();

  console.log('=== Azure VM 管理工具 ===\n');

  try {
    console.log('1. 获取VM列表...');
    const vms = await azure.listVMs();
    console.log(`   找到 ${vms.length} 个VM实例:`);
    vms.forEach(vm => {
      console.log(`   - ${vm.name}`);
    });

    if (vms.length > 0) {
      const vmName = vms[0].name;
      
      console.log(`\n2. 获取VM "${vmName}" 详细信息...`);
      const vmInfo = await azure.getVMWithIP(undefined, vmName);
      console.log(`   名称: ${vmInfo.name}`);
      console.log(`   区域: ${vmInfo.location}`);
      console.log(`   类型: ${vmInfo.vmSize}`);
      console.log(`   状态: ${vmInfo.status.status}`);
      console.log(`   网卡: ${vmInfo.nicName}`);
      console.log(`   公网IP: ${vmInfo.publicIP?.ipAddress || '未分配'}`);
      console.log(`   IP名称: ${vmInfo.publicIP?.name || 'N/A'}`);
      console.log(`   IP类型: ${vmInfo.publicIP?.allocationMethod || 'N/A'}`);
    }

    console.log('\n3. 列出所有公网IP...');
    const ips = await azure.listPublicIPs();
    console.log(`   找到 ${ips.length} 个公网IP:`);
    ips.forEach(ip => {
      console.log(`   - ${ip.name}: ${ip.ipAddress || '(未分配)'} [${ip.allocationMethod}]`);
    });

    console.log('\n=== 操作完成 ===');
    
  } catch (error) {
    console.error('\n操作失败:', error.response?.data || error.message);
  }
}

main();

async function testChangeIP() {
  const azure = new AzureService();
  
  try {
    console.log('=== 测试更换IP ===\n');
    
    const vms = await azure.listVMs();
    if (vms.length === 0) {
      console.log('没有找到VM实例');
      return;
    }
    
    const vmName = vms[0].name;
    const vmInfo = await azure.getVMWithIP(undefined, vmName);
    
    console.log(`当前VM: ${vmName}`);
    console.log(`当前公网IP: ${vmInfo.publicIP?.ipAddress || '未分配'}`);
    console.log(`当前IP名称: ${vmInfo.publicIP?.name || 'N/A'}`);
    console.log(`网卡名称: ${vmInfo.nicName}`);
    
    if (!vmInfo.publicIP || !vmInfo.nicName) {
      console.log('无法获取当前IP或网卡信息');
      return;
    }
    
    const newIpName = 'new-ip-' + Date.now();
    
    console.log(`\n开始更换IP...`);
    console.log(`旧IP: ${vmInfo.publicIP.name}`);
    console.log(`新IP: ${newIpName}`);
    
    const result = await azure.changePublicIP(
      undefined,
      vmInfo.nicName,
      vmInfo.publicIP.name,
      newIpName,
      undefined
    );
    
    console.log(result);
    console.log('\n验证新IP...');
    
    const newVmInfo = await azure.getVMWithIP(undefined, vmName);
    console.log(`新公网IP: ${newVmInfo.publicIP?.ipAddress || '未分配'}`);
    console.log(`新IP名称: ${newVmInfo.publicIP?.name || 'N/A'}`);
    
    console.log('\n=== IP更换测试完成 ===');
    
  } catch (error) {
    console.error('\nIP更换失败:', error.response?.data || error.message);
  }
}

testChangeIP();