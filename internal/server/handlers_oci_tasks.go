package server

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/crossgg/cloud-vm-manager/internal/provider/oci"
	"github.com/gin-gonic/gin"
)

var ociCreateTaskStore = struct {
	mu      sync.RWMutex
	manager *oci.OCICreateTaskManager
}{}

func resolveOCICreateTaskPath(configPath string) string {
	if strings.TrimSpace(configPath) == "" {
		return filepath.Join("config", "oci_create_tasks.json")
	}
	return filepath.Join(filepath.Dir(configPath), "oci_create_tasks.json")
}

func InitOCICreateTasks() {
	configPath := currentConfigPath()
	storePath := resolveOCICreateTaskPath(configPath)

	manager := oci.NewOCICreateTaskManager(storePath, func(account string) (*oci.OCIService, error) {
		service, _, ok := serviceSnapshot("oci", account)
		if !ok || service == nil {
			return nil, fmt.Errorf("OCI 账号 %q 不存在或未加载", account)
		}
		ociSvc, ok := service.(*oci.OCIService)
		if !ok {
			return nil, fmt.Errorf("账号 %q 不是 OCI 服务", account)
		}
		return ociSvc, nil
	})

	if err := manager.LoadAndAutoResume(); err != nil {
		fmt.Printf("warning: failed to load OCI create tasks from %s: %v\n", storePath, err)
	}

	ociCreateTaskStore.mu.Lock()
	if ociCreateTaskStore.manager != nil {
		ociCreateTaskStore.manager.StopAll()
	}
	ociCreateTaskStore.manager = manager
	ociCreateTaskStore.mu.Unlock()
}

func getOCICreateTaskManager(c *gin.Context) (*oci.OCICreateTaskManager, bool) {
	ociCreateTaskStore.mu.RLock()
	manager := ociCreateTaskStore.manager
	ociCreateTaskStore.mu.RUnlock()
	if manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "OCI create task manager is not initialized"})
		return nil, false
	}
	return manager, true
}

func getOCICreateOptions(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	shape := c.Query("shape")
	options, err := ociSvc.GetCreateOptions(shape)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, options)
}

func getOCICreateImages(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	shape := c.Query("shape")
	images, err := ociSvc.ListImagesForShape(shape)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"images": images})
}

func listOCICreateTasks(c *gin.Context) {
	manager, ok := getOCICreateTaskManager(c)
	if !ok {
		return
	}
	account := c.Query("account")
	c.JSON(http.StatusOK, gin.H{"tasks": manager.ListTasks(account)})
}

func createOCICreateTask(c *gin.Context) {
	manager, ok := getOCICreateTaskManager(c)
	if !ok {
		return
	}
	var input oci.OCICreateTaskInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, _, exists := serviceSnapshot("oci", input.Account); !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("OCI account %q not found", input.Account)})
		return
	}

	task, err := manager.CreateTask(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "task": task})
}

func updateOCICreateTask(c *gin.Context) {
	manager, ok := getOCICreateTaskManager(c)
	if !ok {
		return
	}
	taskID := c.Param("taskID")
	var input oci.OCICreateTaskInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, _, exists := serviceSnapshot("oci", input.Account); !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("OCI account %q not found", input.Account)})
		return
	}

	task, err := manager.UpdateTask(taskID, input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "task": task})
}

func deleteOCICreateTask(c *gin.Context) {
	manager, ok := getOCICreateTaskManager(c)
	if !ok {
		return
	}
	taskID := c.Param("taskID")
	if err := manager.DeleteTask(taskID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func startOCICreateTask(c *gin.Context) {
	manager, ok := getOCICreateTaskManager(c)
	if !ok {
		return
	}
	taskID := c.Param("taskID")
	task, err := manager.StartTask(taskID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "task": task})
}

func stopOCICreateTask(c *gin.Context) {
	manager, ok := getOCICreateTaskManager(c)
	if !ok {
		return
	}
	taskID := c.Param("taskID")
	task, err := manager.StopTask(taskID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "task": task})
}

func createOCIDefaultNetwork(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	var payload struct {
		EnableIPv6 bool `json:"enableIpv6"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil && err.Error() != "EOF" {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	res, err := ociSvc.CreateDefaultNetwork(payload.EnableIPv6)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"vcn":     res.VCN,
		"subnet":  res.Subnet,
		"logs":    res.Logs,
	})
}

func launchOCIInstanceOnce(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	var input oci.OCICreateTaskInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	task := oci.OCICreateTask{
		Name:                input.Name,
		Account:             c.Param("account"),
		Template:            input.Template,
		AvailabilityDomain:  input.AvailabilityDomain,
		DisplayName:         input.DisplayName,
		Shape:               input.Shape,
		OCPUs:               input.OCPUs,
		MemoryInGBs:         input.MemoryInGBs,
		BootVolumeSizeInGBs: input.BootVolumeSizeInGBs,
		ImageID:             input.ImageID,
		SubnetID:            input.SubnetID,
		AssignPublicIP:      input.AssignPublicIP,
		AssignIPv6IP:        input.AssignIPv6IP,
		NSGIDs:              input.NSGIDs,
		SSHPublicKey:        input.SSHPublicKey,
		CloudInit:           input.CloudInit,
	}
	instanceID, err := ociSvc.LaunchInstance(task)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"instanceId": instanceID,
		"message":    fmt.Sprintf("实例创建请求已提交，ID: %s", instanceID),
	})
}
