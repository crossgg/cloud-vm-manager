package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

var azureService *AzureService

func main() {
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Printf("加载配置失败: %v\n", err)
		return
	}

	azureService = NewAzureService(cfg)

	r := gin.Default()

	r.GET("/api/vms", listVMs)
	r.GET("/api/vm/:name", getVM)
	r.POST("/api/vm/:name/start", startVM)
	r.POST("/api/vm/:name/stop", stopVM)
	r.POST("/api/vm/:name/restart", restartVM)
	r.POST("/api/vm/:name/change-ip", changeIP)
	r.GET("/api/balance", getBalance)
	r.GET("/api/refresh/:name", refreshVM)

	r.StaticFile("/", "./public/index.html")
	r.Static("/public", "./public")

	r.Run(":3000")
}

func listVMs(c *gin.Context) {
	vms, err := azureService.ListVMs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, vms)
}

func getVM(c *gin.Context) {
	name := c.Param("name")
	vm, err := azureService.GetVM(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, vm)
}

func startVM(c *gin.Context) {
	name := c.Param("name")
	if err := azureService.StartVM(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("正在启动VM: %s", name)})
}

func stopVM(c *gin.Context) {
	name := c.Param("name")
	if err := azureService.StopVM(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("正在停止VM: %s", name)})
}

func restartVM(c *gin.Context) {
	name := c.Param("name")
	if err := azureService.RestartVM(name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": fmt.Sprintf("正在重启VM: %s", name)})
}

func changeIP(c *gin.Context) {
	name := c.Param("name")
	var logs []string

	// 获取VM详情来找到网卡和当前IP
	vm, err := azureService.GetVM(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 获取网卡名称
	nicName, ok := vm["nicName"].(string)
	if !ok || nicName == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "无法获取网卡信息"})
		return
	}

	// 获取旧IP名称
	oldIPName := ""
	if publicIP, ok := vm["publicIP"].(map[string]interface{}); ok {
		if ipName, ok := publicIP["name"].(string); ok && ipName != "N/A" {
			oldIPName = ipName
		}
	}
	fmt.Printf("[DEBUG] 旧IP信息: %+v, 获取的旧IP名称: %s\n", vm["publicIP"], oldIPName)

	// 创建新IP名称
	newIPName := fmt.Sprintf("new-ip-%d", time.Now().Unix())

	// 1. 创建新IP
	logs = append(logs, fmt.Sprintf("[1/3] 正在创建新IP: %s...", newIPName))
	if err := azureService.CreatePublicIP(newIPName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("创建IP失败: %v", err), "logs": logs})
		return
	}
	logs = append(logs, fmt.Sprintf("[1/3] ✓ 创建新IP成功: %s", newIPName))

	// 2. 关联新IP
	logs = append(logs, "[2/3] 正在关联新IP到网卡...")
	if err := azureService.UpdateNetworkInterface(nicName, newIPName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("关联IP失败: %v", err), "logs": logs})
		return
	}
	logs = append(logs, "[2/3] ✓ 关联新IP成功")

	// 3. 删除旧IP（如果存在）
	fmt.Printf("[DEBUG] 删除前 - oldIPName: %s, newIPName: %s\n", oldIPName, newIPName)
	if oldIPName != "" {
		logs = append(logs, fmt.Sprintf("[3/3] 正在删除旧IP: %s...", oldIPName))
		if err := azureService.DeletePublicIP(oldIPName); err != nil {
			logs = append(logs, fmt.Sprintf("[3/3] ⚠ 删除旧IP失败: %v (继续执行)", err))
		} else {
			logs = append(logs, fmt.Sprintf("[3/3] ✓ 删除旧IP成功: %s", oldIPName))
		}
	} else {
		logs = append(logs, "[3/3] 无旧IP需要删除")
	}

	// 刷新获取新IP信息
	var newIpAddress string
	updatedVM, err := azureService.GetVM(name)
	if err == nil {
		if publicIP, ok := updatedVM["publicIP"].(map[string]interface{}); ok {
			if ipAddr, ok := publicIP["ipAddress"].(string); ok {
				newIpAddress = ipAddr
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"message":      "IP更换成功",
		"newIpAddress": newIpAddress,
		"logs":         logs,
	})
}

func getBalance(c *gin.Context) {
	balance, err := azureService.GetSubscriptionBalance()
	if err != nil {
		fmt.Printf("[DEBUG] 获取余额失败: %v\n", err)
		c.JSON(http.StatusOK, gin.H{
			"total":    691.06,
			"currency": "CNY",
			"note":     "获取余额失败，显示默认值",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"total":    balance.Total,
		"currency": balance.Currency,
		"note":     "",
	})
}

func refreshVM(c *gin.Context) {
	name := c.Param("name")
	vm, err := azureService.GetVM(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, vm)
}
