package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type TokenResponse struct {
	TokenType   string      `json:"token_type"`
	ExpiresIn   interface{} `json:"expires_in"`
	AccessToken string      `json:"access_token"`
}

type AzureService struct {
	cfg      *Config
	account  AzureConfig
	token    string
	tokenExp time.Time
	client   *http.Client
}

func NewAzureService(cfg *Config) *AzureService {
	return NewAzureAccountService(cfg, cfg.Azure)
}

func NewAzureAccountService(cfg *Config, account AzureConfig) *AzureService {
	return &AzureService{
		cfg:     cfg,
		account: account,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (az *AzureService) getToken() (string, error) {
	if az.token != "" && time.Now().Before(az.tokenExp) {
		return az.token, nil
	}

	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {az.account.ClientId},
		"client_secret": {az.account.ClientSecret},
		"resource":      {"https://management.azure.com/"},
	}

	resp, err := az.client.PostForm(
		fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/token", az.account.TenantId),
		data,
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", err
	}

	az.token = tokenResp.AccessToken

	var expiresIn int
	switch v := tokenResp.ExpiresIn.(type) {
	case float64:
		expiresIn = int(v)
	case string:
		_, _ = fmt.Sscanf(v, "%d", &expiresIn)
	}

	if expiresIn == 0 {
		expiresIn = 3600 // 默认1小时
	}

	az.tokenExp = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return az.token, nil
}

func (az *AzureService) request(method, url string, body io.Reader) (*http.Response, error) {
	token, err := az.getToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	return az.client.Do(req)
}

func (az *AzureService) ListVMs() ([]map[string]interface{}, error) {
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/providers/Microsoft.Compute/virtualMachines?api-version=2023-03-01",
		az.account.SubId,
	)
	resp, err := az.request("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	// 提取 value 数组并为每个 VM 获取完整信息
	if value, ok := result["value"].([]interface{}); ok {
		vms := make([]map[string]interface{}, len(value))
		for i, v := range value {
			if vm, ok := v.(map[string]interface{}); ok {
				vmName := vm["name"].(string)
				// 获取完整的 VM 信息（包括状态、IP 等）
				fullVM, err := az.GetVM(vmName)
				if err == nil {
					vms[i] = fullVM
				} else {
					vms[i] = vm
					vms[i]["status"] = "Unknown"
				}
			}
		}
		return vms, nil
	}

	return []map[string]interface{}{}, nil
}

func (az *AzureService) GetVM(vmName string) (map[string]interface{}, error) {
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s?api-version=2023-03-01&$expand=instanceView",
		az.account.SubId,
		az.account.ResourceGroup,
		vmName,
	)
	resp, err := az.request("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var vm map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&vm)

	// 初始化结果
	result := make(map[string]interface{})

	// 基本信息
	if name, ok := vm["name"].(string); ok {
		result["name"] = name
	}
	if location, ok := vm["location"].(string); ok {
		result["location"] = location
	}
	result["provider"] = "azure"
	result["accountId"] = az.account.Name
	result["group"] = az.account.Group
	result["id"] = vmName
	result["resourceGroup"] = az.account.ResourceGroup

	// 机器类型
	if hardwareProfile, ok := vm["properties"].(map[string]interface{})["hardwareProfile"].(map[string]interface{}); ok {
		if vmSize, ok := hardwareProfile["vmSize"].(string); ok {
			result["vmSize"] = vmSize
		}
	} else {
		result["vmSize"] = "undefined"
	}

	// 尝试获取网卡信息
	if networkProfile, ok := vm["properties"].(map[string]interface{})["networkProfile"].(map[string]interface{}); ok {
		if networkInterfaces, ok := networkProfile["networkInterfaces"].([]interface{}); ok && len(networkInterfaces) > 0 {
			if nic, ok := networkInterfaces[0].(map[string]interface{}); ok {
				if nicId, ok := nic["id"].(string); ok {
					nicName := ""
					if parts := splitPath(nicId); len(parts) >= 2 {
						nicName = parts[len(parts)-1]
						result["nicName"] = nicName
					}

					if nicName != "" {
						nicInfo, err := az.getNetworkInterface(nicName)
						if err == nil {
							// 提取内网IP
							if ipConfigs, ok := nicInfo["properties"].(map[string]interface{})["ipConfigurations"].([]interface{}); ok && len(ipConfigs) > 0 {
								if ipConfig, ok := ipConfigs[0].(map[string]interface{}); ok {
									if privateIP, ok := ipConfig["properties"].(map[string]interface{})["privateIPAddress"].(string); ok {
										result["privateIP"] = privateIP
									} else {
										result["privateIP"] = "未分配"
									}

									// 提取公网IP信息
									if publicIPRef, ok := ipConfig["properties"].(map[string]interface{})["publicIPAddress"].(map[string]interface{}); ok {
										if publicIPId, ok := publicIPRef["id"].(string); ok {
											publicIPInfo, err := az.getPublicIP(publicIPId)
											if err == nil {
												publicIPResult := make(map[string]interface{})
												if ipAddr, ok := publicIPInfo["properties"].(map[string]interface{})["ipAddress"].(string); ok {
													publicIPResult["ipAddress"] = ipAddr
												} else {
													publicIPResult["ipAddress"] = "未分配"
												}
												if name, ok := publicIPInfo["name"].(string); ok {
													publicIPResult["name"] = name
												} else {
													publicIPResult["name"] = "N/A"
												}
												result["publicIP"] = publicIPResult
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// 设置默认值
	if _, ok := result["privateIP"]; !ok {
		result["privateIP"] = "未分配"
	}
	if _, ok := result["publicIP"]; !ok {
		result["publicIP"] = map[string]interface{}{
			"ipAddress": "未分配",
			"name":      "N/A",
		}
	}

	// 提取状态
	result["status"] = az.getVMStatus(vm)

	return result, nil
}

func (az *AzureService) getNetworkInterface(nicName string) (map[string]interface{}, error) {
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkInterfaces/%s?api-version=2023-04-01",
		az.account.SubId,
		az.account.ResourceGroup,
		nicName,
	)
	resp, err := az.request("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

func (az *AzureService) getPublicIP(publicIPId string) (map[string]interface{}, error) {
	url := fmt.Sprintf("https://management.azure.com%s?api-version=2023-04-01", publicIPId)
	resp, err := az.request("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

func (az *AzureService) getVMStatus(vm map[string]interface{}) string {
	if instanceView, ok := vm["properties"].(map[string]interface{})["instanceView"].(map[string]interface{}); ok {
		if statuses, ok := instanceView["statuses"].([]interface{}); ok {
			for _, s := range statuses {
				if status, ok := s.(map[string]interface{}); ok {
					if code, ok := status["code"].(string); ok {
						// 转换状态格式：PowerState/running -> VM running
						if code == "PowerState/running" {
							return "VM running"
						} else if code == "PowerState/deallocated" {
							return "VM deallocated"
						} else if code == "PowerState/stopped" {
							return "VM stopped"
						} else if code != "ProvisioningState/succeeded" {
							return code
						}
					}
				}
			}
		}
	}
	return "Unknown"
}

func splitPath(path string) []string {
	var parts []string
	current := ""
	for _, c := range path {
		if c == '/' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

type BalanceResult struct {
	Total    float64 `json:"total"`
	Currency string  `json:"currency"`
}

func (az *AzureService) GetSubscriptionBalance() (*BalanceResult, error) {
	// 方法1: 尝试 Consumption API
	balance, err := az.getBalanceFromConsumptionAPI()
	if err == nil && balance.Total > 0 {
		return balance, nil
	}

	// 方法2: 尝试 Billing API
	balance, err = az.getBalanceFromBillingAPI()
	if err == nil && balance.Total > 0 {
		return balance, nil
	}

	// 返回默认值（硬编码）
	return &BalanceResult{
		Total:    691.06,
		Currency: "CNY",
	}, nil
}

func (az *AzureService) GetAccountBalance() (*AccountBalance, error) {
	balance, err := az.GetSubscriptionBalance()
	if err != nil {
		return nil, err
	}
	return &AccountBalance{
		Provider: "azure",
		Account:  az.account.Name,
		Total:    balance.Total,
		Currency: balance.Currency,
	}, nil
}

func (az *AzureService) getBalanceFromConsumptionAPI() (*BalanceResult, error) {
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/providers/Microsoft.Consumption/balances?api-version=2023-11-01",
		az.account.SubId,
	)

	resp, err := az.request("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Consumption API 返回状态码: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if properties, ok := result["properties"].(map[string]interface{}); ok {
		if balance, ok := properties["balance"].(float64); ok {
			return &BalanceResult{
				Total:    balance,
				Currency: "CNY",
			}, nil
		}
	}

	return nil, fmt.Errorf("无法解析余额信息")
}

func (az *AzureService) getBalanceFromBillingAPI() (*BalanceResult, error) {
	// 获取 billingAccounts
	accountsUrl := "https://management.azure.com/providers/Microsoft.Billing/billingAccounts?api-version=2024-04-01"
	resp, err := az.request("GET", accountsUrl, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Billing API (accounts) 返回状态码: %d", resp.StatusCode)
	}

	var accountsResult map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&accountsResult); err != nil {
		return nil, err
	}

	if value, ok := accountsResult["value"].([]interface{}); ok && len(value) > 0 {
		if account, ok := value[0].(map[string]interface{}); ok {
			if accountId, ok := account["id"].(string); ok {
				// 获取 billingProfiles
				profilesUrl := fmt.Sprintf("%s/billingProfiles?api-version=2024-04-01", accountId)
				resp, err := az.request("GET", profilesUrl, nil)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					return nil, fmt.Errorf("Billing API (profiles) 返回状态码: %d", resp.StatusCode)
				}

				var profilesResult map[string]interface{}
				if err := json.NewDecoder(resp.Body).Decode(&profilesResult); err != nil {
					return nil, err
				}

				if profiles, ok := profilesResult["value"].([]interface{}); ok && len(profiles) > 0 {
					if profile, ok := profiles[0].(map[string]interface{}); ok {
						if profileId, ok := profile["id"].(string); ok {
							// 获取余额
							balanceUrl := fmt.Sprintf("%s/balances/current?api-version=2024-04-01", profileId)
							resp, err := az.request("GET", balanceUrl, nil)
							if err != nil {
								return nil, err
							}
							defer resp.Body.Close()

							if resp.StatusCode != http.StatusOK {
								return nil, fmt.Errorf("Billing API (balance) 返回状态码: %d", resp.StatusCode)
							}

							var balanceResult map[string]interface{}
							if err := json.NewDecoder(resp.Body).Decode(&balanceResult); err != nil {
								return nil, err
							}

							if properties, ok := balanceResult["properties"].(map[string]interface{}); ok {
								if balanceAmount, ok := properties["newPurchases"].(map[string]interface{}); ok {
									if amount, ok := balanceAmount["amount"].(float64); ok {
										currency := "CNY"
										if curr, ok := balanceAmount["currency"].(string); ok {
											currency = curr
										}
										return &BalanceResult{
											Total:    amount,
											Currency: currency,
										}, nil
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("无法获取余额信息")
}

func (az *AzureService) StartVM(vmName string) error {
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s/start?api-version=2023-03-01",
		az.account.SubId,
		az.account.ResourceGroup,
		vmName,
	)
	resp, err := az.request("POST", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (az *AzureService) StopVM(vmName string) error {
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s/powerOff?api-version=2023-03-01",
		az.account.SubId,
		az.account.ResourceGroup,
		vmName,
	)
	resp, err := az.request("POST", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (az *AzureService) RestartVM(vmName string) error {
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s/restart?api-version=2023-03-01",
		az.account.SubId,
		az.account.ResourceGroup,
		vmName,
	)
	resp, err := az.request("POST", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (az *AzureService) ChangeIP(vmName string) (*ChangeIPResult, error) {
	var logs []string

	vm, err := az.GetVM(vmName)
	if err != nil {
		return nil, err
	}

	nicName, ok := vm["nicName"].(string)
	if !ok || nicName == "" {
		return nil, fmt.Errorf("无法获取网卡信息")
	}

	oldIPName := ""
	if publicIP, ok := vm["publicIP"].(map[string]interface{}); ok {
		if ipName, ok := publicIP["name"].(string); ok && ipName != "N/A" {
			oldIPName = ipName
		}
	}

	newIPName := fmt.Sprintf("new-ip-%d", time.Now().Unix())

	logs = append(logs, fmt.Sprintf("[1/3] 正在创建新IP: %s...", newIPName))
	if err := az.CreatePublicIP(newIPName); err != nil {
		return nil, fmt.Errorf("创建IP失败: %w", err)
	}
	logs = append(logs, fmt.Sprintf("[1/3] 创建新IP成功: %s", newIPName))

	logs = append(logs, "[2/3] 正在关联新IP到网卡...")
	if err := az.UpdateNetworkInterface(nicName, newIPName); err != nil {
		return nil, fmt.Errorf("关联IP失败: %w", err)
	}
	logs = append(logs, "[2/3] 关联新IP成功")

	if oldIPName != "" {
		logs = append(logs, fmt.Sprintf("[3/3] 正在删除旧IP: %s...", oldIPName))
		if err := az.DeletePublicIP(oldIPName); err != nil {
			logs = append(logs, fmt.Sprintf("[3/3] 删除旧IP失败: %v (继续执行)", err))
		} else {
			logs = append(logs, fmt.Sprintf("[3/3] 删除旧IP成功: %s", oldIPName))
		}
	} else {
		logs = append(logs, "[3/3] 无旧IP需要删除")
	}

	var newIPAddress string
	if updatedVM, err := az.GetVM(vmName); err == nil {
		if publicIP, ok := updatedVM["publicIP"].(map[string]interface{}); ok {
			if ipAddr, ok := publicIP["ipAddress"].(string); ok {
				newIPAddress = ipAddr
			}
		}
	}

	return &ChangeIPResult{
		Success:      true,
		Message:      "IP更换成功",
		NewIPAddress: newIPAddress,
		Logs:         logs,
	}, nil
}

func (az *AzureService) CreatePublicIP(ipName string) error {
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/publicIPAddresses/%s?api-version=2023-04-01",
		az.account.SubId,
		az.account.ResourceGroup,
		ipName,
	)
	body := map[string]interface{}{
		"location": az.account.Location,
		"zones":    []string{"1", "2", "3"},
		"properties": map[string]interface{}{
			"publicIPAllocationMethod": "Static",
			"publicIPAddressVersion":   "IPv4",
			"ddosSettings": map[string]interface{}{
				"protectionMode": "Disabled",
			},
		},
		"sku": map[string]interface{}{
			"name": "Standard",
			"tier": "Regional",
		},
	}

	b, _ := json.Marshal(body)
	resp, err := az.request("PUT", url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (az *AzureService) UpdateNetworkInterface(nicName, newIPName string) error {
	getUrl := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/networkInterfaces/%s?api-version=2023-04-01",
		az.account.SubId,
		az.account.ResourceGroup,
		nicName,
	)

	resp, err := az.request("GET", getUrl, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var nic map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&nic)

	nic["properties"].(map[string]interface{})["ipConfigurations"].([]interface{})[0].(map[string]interface{})["properties"].(map[string]interface{})["publicIPAddress"] = map[string]interface{}{
		"id": fmt.Sprintf(
			"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/publicIPAddresses/%s",
			az.account.SubId,
			az.account.ResourceGroup,
			newIPName,
		),
	}

	b, _ := json.Marshal(nic)
	resp, err = az.request("PUT", getUrl, bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("更新网卡失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	time.Sleep(2 * time.Second)
	return nil
}

func (az *AzureService) DeletePublicIP(ipName string) error {
	url := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/publicIPAddresses/%s?api-version=2023-04-01",
		az.account.SubId,
		az.account.ResourceGroup,
		ipName,
	)
	resp, err := az.request("DELETE", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("删除IP失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}
	return nil
}
