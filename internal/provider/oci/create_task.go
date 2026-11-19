package oci

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	OCICreateTaskStatusIdle      = "idle"
	OCICreateTaskStatusRunning   = "running"
	OCICreateTaskStatusStopped   = "stopped"
	OCICreateTaskStatusSucceeded = "succeeded"

	maxCreateTaskLogs = 80
)

var createTaskSleepUnit = time.Second

type OCICreateImageOption struct {
	ID              string `json:"id"`
	DisplayName     string `json:"displayName"`
	OperatingSystem string `json:"operatingSystem,omitempty"`
	OSVersion       string `json:"operatingSystemVersion,omitempty"`
	TimeCreated     string `json:"timeCreated,omitempty"`
}

type OCIVCNOption struct {
	ID             string   `json:"id"`
	DisplayName    string   `json:"displayName"`
	CIDRBlock      string   `json:"cidrBlock,omitempty"`
	IPv6CIDRBlocks []string `json:"ipv6CidrBlocks"`
}

type OCISubnetOption struct {
	ID                    string   `json:"id"`
	VCNID                 string   `json:"vcnId"`
	DisplayName           string   `json:"displayName"`
	CIDRBlock             string   `json:"cidrBlock,omitempty"`
	IPv6CIDRBlocks        []string `json:"ipv6CidrBlocks"`
	ProhibitPublicIPOnNIC bool     `json:"prohibitPublicIpOnVnic"`
}

type OCICreateOptions struct {
	AvailabilityDomains []string                        `json:"availabilityDomains"`
	Shapes              []OCIShapeOption                `json:"shapes"`
	Images              []OCICreateImageOption          `json:"images"`
	VCNs                []OCIVCNOption                  `json:"vcns"`
	Subnets             []OCISubnetOption               `json:"subnets"`
	NSGs                []OCINetworkSecurityGroupOption `json:"nsgs"`
}

type OCICreateTaskInput struct {
	Name                string   `json:"name"`
	Account             string   `json:"account"`
	Template            string   `json:"template"`
	AvailabilityDomain  string   `json:"availabilityDomain"`
	DisplayName         string   `json:"displayName"`
	Shape               string   `json:"shape"`
	OCPUs               float64  `json:"ocpus"`
	MemoryInGBs         float64  `json:"memoryInGBs"`
	BootVolumeSizeInGBs int64    `json:"bootVolumeSizeInGBs"`
	ImageID             string   `json:"imageId"`
	ImageName           string   `json:"imageName"`
	VCNID               string   `json:"vcnId"`
	SubnetID            string   `json:"subnetId"`
	AssignPublicIP      bool     `json:"assignPublicIp"`
	AssignIPv6IP        bool     `json:"assignIpv6Ip"`
	NSGIDs              []string `json:"nsgIds"`
	SSHPublicKey        string   `json:"sshPublicKey"`
	CloudInit           string   `json:"cloudInit"`
	MinIntervalSec      int      `json:"minIntervalSec"`
	MaxIntervalSec      int      `json:"maxIntervalSec"`
}

type OCICreateTask struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Account             string   `json:"account"`
	Template            string   `json:"template"`
	AvailabilityDomain  string   `json:"availabilityDomain"`
	DisplayName         string   `json:"displayName"`
	Shape               string   `json:"shape"`
	OCPUs               float64  `json:"ocpus"`
	MemoryInGBs         float64  `json:"memoryInGBs"`
	BootVolumeSizeInGBs int64    `json:"bootVolumeSizeInGBs"`
	ImageID             string   `json:"imageId"`
	ImageName           string   `json:"imageName"`
	VCNID               string   `json:"vcnId"`
	SubnetID            string   `json:"subnetId"`
	AssignPublicIP      bool     `json:"assignPublicIp"`
	AssignIPv6IP        bool     `json:"assignIpv6Ip"`
	NSGIDs              []string `json:"nsgIds"`
	SSHPublicKey        string   `json:"sshPublicKey"`
	CloudInit           string   `json:"cloudInit"`
	MinIntervalSec      int      `json:"minIntervalSec"`
	MaxIntervalSec      int      `json:"maxIntervalSec"`
	Status              string   `json:"status"`
	Attempts            int64    `json:"attempts"`
	LastAttemptAt       string   `json:"lastAttemptAt,omitempty"`
	LastMessage         string   `json:"lastMessage,omitempty"`
	LastError           string   `json:"lastError,omitempty"`
	CreatedInstanceID   string   `json:"createdInstanceId,omitempty"`
	Logs                []string `json:"logs,omitempty"`
	CreatedAt           string   `json:"createdAt"`
	UpdatedAt           string   `json:"updatedAt"`
}

func (o *OCIService) GetCreateOptions(shape string) (*OCICreateOptions, error) {
	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return nil, err
	}

	var rawADs []map[string]interface{}
	if err := o.identityRequestJSON(o.Account.Region, "GET", "/availabilityDomains", url.Values{
		"compartmentId": {compartmentID},
	}, nil, &rawADs, false); err != nil {
		return nil, fmt.Errorf("获取可用域失败: %w", err)
	}
	ads := make([]string, 0, len(rawADs))
	for _, ad := range rawADs {
		if name := stringValue(ad["name"]); name != "" {
			ads = append(ads, name)
		}
	}

	shapes, err := o.ListEditableShapes(OCIInstanceEditInstance{})
	if err != nil {
		return nil, fmt.Errorf("获取实例规格失败: %w", err)
	}

	var rawVCNs []map[string]interface{}
	if err := o.requestJSON("GET", "/vcns", url.Values{
		"compartmentId": {compartmentID},
	}, nil, &rawVCNs, false); err != nil {
		return nil, fmt.Errorf("获取 VCN 失败: %w", err)
	}
	vcns := make([]OCIVCNOption, 0, len(rawVCNs))
	for _, v := range rawVCNs {
		id := stringValue(v["id"])
		if id == "" {
			continue
		}
		vcns = append(vcns, OCIVCNOption{
			ID:             id,
			DisplayName:    valueOrDefault(stringValue(v["displayName"]), id),
			CIDRBlock:      stringValue(v["cidrBlock"]),
			IPv6CIDRBlocks: extractSubnetIPv6CIDRs(v),
		})
	}

	var rawSubnets []map[string]interface{}
	if err := o.requestJSON("GET", "/subnets", url.Values{
		"compartmentId": {compartmentID},
	}, nil, &rawSubnets, false); err != nil {
		return nil, fmt.Errorf("获取子网失败: %w", err)
	}
	subnets := make([]OCISubnetOption, 0, len(rawSubnets))
	for _, s := range rawSubnets {
		id := stringValue(s["id"])
		if id == "" {
			continue
		}
		prohibitPublic, _ := s["prohibitPublicIpOnVnic"].(bool)
		subnets = append(subnets, OCISubnetOption{
			ID:                    id,
			VCNID:                 stringValue(s["vcnId"]),
			DisplayName:           valueOrDefault(stringValue(s["displayName"]), id),
			CIDRBlock:             stringValue(s["cidrBlock"]),
			IPv6CIDRBlocks:        extractSubnetIPv6CIDRs(s),
			ProhibitPublicIPOnNIC: prohibitPublic,
		})
	}

	var rawNSGs []map[string]interface{}
	if err := o.requestJSON("GET", "/networkSecurityGroups", url.Values{
		"compartmentId": {compartmentID},
	}, nil, &rawNSGs, false); err != nil {
		return nil, fmt.Errorf("获取网络安全组失败: %w", err)
	}
	nsgs := make([]OCINetworkSecurityGroupOption, 0, len(rawNSGs))
	for _, g := range rawNSGs {
		state := strings.ToUpper(stringValue(g["lifecycleState"]))
		if state == "TERMINATING" || state == "TERMINATED" {
			continue
		}
		id := stringValue(g["id"])
		if id == "" {
			continue
		}
		nsgs = append(nsgs, OCINetworkSecurityGroupOption{
			ID:    id,
			Name:  valueOrDefault(stringValue(g["displayName"]), id),
			VCNID: stringValue(g["vcnId"]),
		})
	}

	images := make([]OCICreateImageOption, 0)
	if strings.TrimSpace(shape) != "" {
		images, err = o.ListImagesForShape(shape)
		if err != nil {
			return nil, err
		}
	}

	return &OCICreateOptions{
		AvailabilityDomains: ads,
		Shapes:              shapes,
		Images:              images,
		VCNs:                vcns,
		Subnets:             subnets,
		NSGs:                nsgs,
	}, nil
}

func (o *OCIService) ListImagesForShape(shape string) ([]OCICreateImageOption, error) {
	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return nil, err
	}

	query := url.Values{
		"compartmentId":  {compartmentID},
		"lifecycleState": {"AVAILABLE"},
		"limit":          {"100"},
	}
	if strings.TrimSpace(shape) != "" {
		query.Set("shape", strings.TrimSpace(shape))
	}

	var rawImages []map[string]interface{}
	if err := o.requestJSON("GET", "/images", query, nil, &rawImages, false); err != nil {
		return nil, fmt.Errorf("获取系统镜像失败: %w", err)
	}

	images := make([]OCICreateImageOption, 0, len(rawImages))
	for _, img := range rawImages {
		id := stringValue(img["id"])
		if id == "" {
			continue
		}
		images = append(images, OCICreateImageOption{
			ID:              id,
			DisplayName:     valueOrDefault(stringValue(img["displayName"]), id),
			OperatingSystem: stringValue(img["operatingSystem"]),
			OSVersion:       stringValue(img["operatingSystemVersion"]),
			TimeCreated:     stringValue(img["timeCreated"]),
		})
	}

	sort.SliceStable(images, func(i, j int) bool {
		if images[i].OperatingSystem != images[j].OperatingSystem {
			return images[i].OperatingSystem < images[j].OperatingSystem
		}
		if images[i].OSVersion != images[j].OSVersion {
			return images[i].OSVersion > images[j].OSVersion
		}
		return images[i].TimeCreated > images[j].TimeCreated
	})
	return images, nil
}

func (o *OCIService) LaunchInstance(task OCICreateTask) (string, error) {
	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(task.AvailabilityDomain) == "" {
		return "", fmt.Errorf("请选择可用域")
	}
	if strings.TrimSpace(task.Shape) == "" {
		return "", fmt.Errorf("请选择实例规格")
	}
	if strings.TrimSpace(task.ImageID) == "" {
		return "", fmt.Errorf("请选择系统镜像")
	}
	if strings.TrimSpace(task.SubnetID) == "" {
		return "", fmt.Errorf("请选择子网")
	}

	displayName := strings.TrimSpace(task.DisplayName)
	if displayName == "" {
		displayName = valueOrDefault(strings.TrimSpace(task.Name), "oci-vm-"+time.Now().Format("20060102-150405"))
	}

	sourceDetails := map[string]interface{}{
		"sourceType": "image",
		"imageId":    strings.TrimSpace(task.ImageID),
	}
	if task.BootVolumeSizeInGBs > 0 {
		sourceDetails["bootVolumeSizeInGBs"] = task.BootVolumeSizeInGBs
	}

	createVnicDetails := map[string]interface{}{
		"subnetId":       strings.TrimSpace(task.SubnetID),
		"assignPublicIp": task.AssignPublicIP,
	}
	if task.AssignIPv6IP {
		createVnicDetails["assignIpv6Ip"] = true
	}
	if len(task.NSGIDs) > 0 {
		cleanNSGs := make([]string, 0, len(task.NSGIDs))
		for _, id := range task.NSGIDs {
			id = strings.TrimSpace(id)
			if id != "" {
				cleanNSGs = append(cleanNSGs, id)
			}
		}
		if len(cleanNSGs) > 0 {
			createVnicDetails["nsgIds"] = cleanNSGs
		}
	}

	body := map[string]interface{}{
		"compartmentId":      compartmentID,
		"availabilityDomain": strings.TrimSpace(task.AvailabilityDomain),
		"displayName":        displayName,
		"shape":              strings.TrimSpace(task.Shape),
		"sourceDetails":      sourceDetails,
		"createVnicDetails":  createVnicDetails,
	}

	if isFlexShapeName(task.Shape) && (task.OCPUs > 0 || task.MemoryInGBs > 0) {
		shapeCfg := map[string]interface{}{}
		if task.OCPUs > 0 {
			shapeCfg["ocpus"] = task.OCPUs
		}
		if task.MemoryInGBs > 0 {
			shapeCfg["memoryInGBs"] = task.MemoryInGBs
		}
		body["shapeConfig"] = shapeCfg
	}

	metadata := map[string]string{}
	if sshKey := strings.TrimSpace(task.SSHPublicKey); sshKey != "" {
		metadata["ssh_authorized_keys"] = sshKey
	}
	if cloudInit := strings.TrimSpace(task.CloudInit); cloudInit != "" {
		metadata["user_data"] = base64.StdEncoding.EncodeToString([]byte(task.CloudInit))
	}
	if len(metadata) > 0 {
		body["metadata"] = metadata
	}

	var created map[string]interface{}
	if err := o.requestJSON("POST", "/instances", nil, body, &created, false); err != nil {
		return "", err
	}
	instanceID := stringValue(created["id"])
	if instanceID == "" {
		return "", fmt.Errorf("OCI 未返回实例 ID")
	}
	return instanceID, nil
}

func isFlexShapeName(shape string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(shape)), ".flex")
}

type OCIDefaultNetworkResult struct {
	VCN    OCIVCNOption    `json:"vcn"`
	Subnet OCISubnetOption `json:"subnet"`
	Logs   []string        `json:"logs"`
}

func (o *OCIService) CreateDefaultNetwork(enableIPv6 bool) (*OCIDefaultNetworkResult, error) {
	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return nil, err
	}

	suffix := time.Now().Format("01021504")
	vcnBody := map[string]interface{}{
		"compartmentId": compartmentID,
		"cidrBlocks":    []string{"10.0.0.0/16"},
		"displayName":   "default-vcn-" + suffix,
		"dnsLabel":      "vcn" + suffix,
	}
	if enableIPv6 {
		vcnBody["isIpv6Enabled"] = true
	}

	var vcn map[string]interface{}
	if err := o.requestJSON("POST", "/vcns", nil, vcnBody, &vcn, false); err != nil {
		return nil, fmt.Errorf("创建默认 VCN 失败: %w", err)
	}
	vcnID := stringValue(vcn["id"])
	logs := []string{fmt.Sprintf("已创建 VCN: %s (%s)", valueOrDefault(stringValue(vcn["displayName"]), vcnID), "10.0.0.0/16")}

	var igw map[string]interface{}
	if err := o.requestJSON("POST", "/internetGateways", nil, map[string]interface{}{
		"compartmentId": compartmentID,
		"vcnId":         vcnID,
		"isEnabled":     true,
		"displayName":   "default-igw-" + suffix,
	}, &igw, false); err != nil {
		return nil, fmt.Errorf("创建互联网网关 (IGW) 失败: %w", err)
	}
	igwID := stringValue(igw["id"])
	logs = append(logs, "已创建互联网网关 (IGW)")

	defaultRTID := stringValue(vcn["defaultRouteTableId"])
	if defaultRTID != "" && igwID != "" {
		routeRules := []map[string]interface{}{
			{
				"destination":     "0.0.0.0/0",
				"destinationType": "CIDR_BLOCK",
				"networkEntityId": igwID,
			},
		}
		if enableIPv6 {
			routeRules = append(routeRules, map[string]interface{}{
				"destination":     "::/0",
				"destinationType": "CIDR_BLOCK",
				"networkEntityId": igwID,
			})
		}
		if err := o.requestJSON("PUT", "/routeTables/"+url.PathEscape(defaultRTID), nil, map[string]interface{}{
			"routeRules": routeRules,
		}, nil, false); err != nil {
			return nil, fmt.Errorf("配置默认路由表失败: %w", err)
		}
		if enableIPv6 {
			logs = append(logs, "已配置默认路由表 (0.0.0.0/0, ::/0 -> IGW)")
		} else {
			logs = append(logs, "已配置默认路由表 (0.0.0.0/0 -> IGW)")
		}
	}

	if enableIPv6 {
		if defaultSLID := stringValue(vcn["defaultSecurityListId"]); defaultSLID != "" {
			if added, err := o.ensureSecurityListIPv6Egress(defaultSLID); err != nil {
				logs = append(logs, fmt.Sprintf("警告：配置默认安全列表 IPv6 出站失败，请前往「防火墙管理」手动补充 ::/0 出站规则: %v", err))
			} else if added {
				logs = append(logs, "已配置默认安全列表 IPv6 出站规则 (::/0, 所有协议)")
			}
		}
	}

	vcnIPv6CIDRs := extractSubnetIPv6CIDRs(vcn)
	subnetBody := map[string]interface{}{
		"compartmentId":          compartmentID,
		"vcnId":                  vcnID,
		"cidrBlock":              "10.0.0.0/24",
		"displayName":            "public-subnet-" + suffix,
		"dnsLabel":               "sub" + suffix,
		"prohibitPublicIpOnVnic": false,
	}
	if enableIPv6 && len(vcnIPv6CIDRs) > 0 {
		if subCIDR, err := deriveSubnetIPv6CIDR(vcnIPv6CIDRs[0], "00"); err == nil {
			subnetBody["ipv6CidrBlock"] = subCIDR
		}
	}

	var subnet map[string]interface{}
	if err := o.requestJSON("POST", "/subnets", nil, subnetBody, &subnet, false); err != nil {
		return nil, fmt.Errorf("创建默认公有子网失败: %w", err)
	}
	subnetID := stringValue(subnet["id"])
	logs = append(logs, fmt.Sprintf("已创建公有子网: %s (10.0.0.0/24)", valueOrDefault(stringValue(subnet["displayName"]), subnetID)))

	return &OCIDefaultNetworkResult{
		VCN: OCIVCNOption{
			ID:             vcnID,
			DisplayName:    valueOrDefault(stringValue(vcn["displayName"]), vcnID),
			CIDRBlock:      stringValue(vcn["cidrBlock"]),
			IPv6CIDRBlocks: vcnIPv6CIDRs,
		},
		Subnet: OCISubnetOption{
			ID:                    subnetID,
			VCNID:                 vcnID,
			DisplayName:           valueOrDefault(stringValue(subnet["displayName"]), subnetID),
			CIDRBlock:             stringValue(subnet["cidrBlock"]),
			IPv6CIDRBlocks:        extractSubnetIPv6CIDRs(subnet),
			ProhibitPublicIPOnNIC: false,
		},
		Logs: logs,
	}, nil
}

// --- Persistent Task Manager ---

type OCICreateTaskManager struct {
	storePath       string
	serviceResolver func(account string) (*OCIService, error)
	mu              sync.RWMutex
	tasks           map[string]*OCICreateTask
	order           []string
	runners         map[string]chan struct{}
	onSucceeded     func(task OCICreateTask)
}

func NewOCICreateTaskManager(storePath string, resolver func(account string) (*OCIService, error)) *OCICreateTaskManager {
	return &OCICreateTaskManager{
		storePath:       storePath,
		serviceResolver: resolver,
		tasks:           make(map[string]*OCICreateTask),
		runners:         make(map[string]chan struct{}),
	}
}

func (m *OCICreateTaskManager) SetOnSucceeded(fn func(task OCICreateTask)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onSucceeded = fn
}

func (m *OCICreateTaskManager) LoadAndAutoResume() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.storePath != "" {
		data, err := os.ReadFile(m.storePath)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		} else if len(strings.TrimSpace(string(data))) > 0 {
			var loaded []OCICreateTask
			if err := json.Unmarshal(data, &loaded); err != nil {
				return fmt.Errorf("解析 OCI 创建任务文件失败: %w", err)
			}
			m.tasks = make(map[string]*OCICreateTask, len(loaded))
			m.order = make([]string, 0, len(loaded))
			for i := range loaded {
				taskCopy := loaded[i]
				m.tasks[taskCopy.ID] = &taskCopy
				m.order = append(m.order, taskCopy.ID)
			}
		}
	}

	for _, id := range m.order {
		task := m.tasks[id]
		if task != nil && task.Status == OCICreateTaskStatusRunning {
			m.appendTaskLogLocked(task, "服务重启，自动恢复抢购任务")
			m.startRunnerLocked(task)
		}
	}
	return nil
}

func (m *OCICreateTaskManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, stopCh := range m.runners {
		close(stopCh)
		delete(m.runners, id)
	}
}

func (m *OCICreateTaskManager) ListTasks(account string) []OCICreateTask {
	m.mu.RLock()
	defer m.mu.RUnlock()

	account = strings.TrimSpace(account)
	result := make([]OCICreateTask, 0, len(m.order))
	for _, id := range m.order {
		task := m.tasks[id]
		if task == nil {
			continue
		}
		if account != "" && task.Account != account {
			continue
		}
		result = append(result, cloneCreateTask(task))
	}
	return result
}

func (m *OCICreateTaskManager) CreateTask(input OCICreateTaskInput) (OCICreateTask, error) {
	normalized, err := normalizeCreateTaskInput(input)
	if err != nil {
		return OCICreateTask{}, err
	}

	now := time.Now().Format(time.RFC3339)
	id := fmt.Sprintf("oci-task-%d", time.Now().UnixNano())

	m.mu.Lock()
	defer m.mu.Unlock()
	for _, exists := m.tasks[id]; exists; _, exists = m.tasks[id] {
		id = fmt.Sprintf("oci-task-%d", time.Now().UnixNano()+1)
	}

	task := &OCICreateTask{
		ID:                  id,
		Name:                normalized.Name,
		Account:             normalized.Account,
		Template:            normalized.Template,
		AvailabilityDomain:  normalized.AvailabilityDomain,
		DisplayName:         normalized.DisplayName,
		Shape:               normalized.Shape,
		OCPUs:               normalized.OCPUs,
		MemoryInGBs:         normalized.MemoryInGBs,
		BootVolumeSizeInGBs: normalized.BootVolumeSizeInGBs,
		ImageID:             normalized.ImageID,
		ImageName:           normalized.ImageName,
		VCNID:               normalized.VCNID,
		SubnetID:            normalized.SubnetID,
		AssignPublicIP:      normalized.AssignPublicIP,
		AssignIPv6IP:        normalized.AssignIPv6IP,
		NSGIDs:              normalized.NSGIDs,
		SSHPublicKey:        normalized.SSHPublicKey,
		CloudInit:           normalized.CloudInit,
		MinIntervalSec:      normalized.MinIntervalSec,
		MaxIntervalSec:      normalized.MaxIntervalSec,
		Status:              OCICreateTaskStatusIdle,
		Logs:                []string{fmt.Sprintf("[%s] 任务已创建", time.Now().Format("15:04:05"))},
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	m.tasks[id] = task
	m.order = append([]string{id}, m.order...)
	if err := m.saveLocked(); err != nil {
		return OCICreateTask{}, err
	}
	return cloneCreateTask(task), nil
}

func (m *OCICreateTaskManager) UpdateTask(taskID string, input OCICreateTaskInput) (OCICreateTask, error) {
	normalized, err := normalizeCreateTaskInput(input)
	if err != nil {
		return OCICreateTask{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return OCICreateTask{}, fmt.Errorf("未找到创建任务 (%s)", taskID)
	}
	if task.Status == OCICreateTaskStatusRunning {
		return OCICreateTask{}, fmt.Errorf("任务运行中，请先停止后再编辑")
	}

	task.Name = normalized.Name
	task.Account = normalized.Account
	task.Template = normalized.Template
	task.AvailabilityDomain = normalized.AvailabilityDomain
	task.DisplayName = normalized.DisplayName
	task.Shape = normalized.Shape
	task.OCPUs = normalized.OCPUs
	task.MemoryInGBs = normalized.MemoryInGBs
	task.BootVolumeSizeInGBs = normalized.BootVolumeSizeInGBs
	task.ImageID = normalized.ImageID
	task.ImageName = normalized.ImageName
	task.VCNID = normalized.VCNID
	task.SubnetID = normalized.SubnetID
	task.AssignPublicIP = normalized.AssignPublicIP
	task.AssignIPv6IP = normalized.AssignIPv6IP
	task.NSGIDs = normalized.NSGIDs
	task.SSHPublicKey = normalized.SSHPublicKey
	task.CloudInit = normalized.CloudInit
	task.MinIntervalSec = normalized.MinIntervalSec
	task.MaxIntervalSec = normalized.MaxIntervalSec
	task.UpdatedAt = time.Now().Format(time.RFC3339)
	if task.Status == OCICreateTaskStatusSucceeded {
		task.Status = OCICreateTaskStatusIdle
		task.CreatedInstanceID = ""
	}
	m.appendTaskLogLocked(task, "任务参数已更新")

	if err := m.saveLocked(); err != nil {
		return OCICreateTask{}, err
	}
	return cloneCreateTask(task), nil
}

func (m *OCICreateTaskManager) DeleteTask(taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.tasks[taskID]; !ok {
		return fmt.Errorf("未找到创建任务 (%s)", taskID)
	}
	if stopCh, running := m.runners[taskID]; running {
		close(stopCh)
		delete(m.runners, taskID)
	}
	delete(m.tasks, taskID)
	newOrder := make([]string, 0, len(m.order))
	for _, id := range m.order {
		if id != taskID {
			newOrder = append(newOrder, id)
		}
	}
	m.order = newOrder
	return m.saveLocked()
}

func (m *OCICreateTaskManager) StartTask(taskID string) (OCICreateTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return OCICreateTask{}, fmt.Errorf("未找到创建任务 (%s)", taskID)
	}
	if task.Status == OCICreateTaskStatusRunning {
		if _, active := m.runners[taskID]; active {
			return cloneCreateTask(task), nil
		}
	}

	task.Status = OCICreateTaskStatusRunning
	task.LastError = ""
	task.LastMessage = "任务已启动，准备发起创建请求..."
	task.UpdatedAt = time.Now().Format(time.RFC3339)
	m.appendTaskLogLocked(task, fmt.Sprintf("启动任务（间隔 %d-%d 秒）", task.MinIntervalSec, task.MaxIntervalSec))
	if err := m.saveLocked(); err != nil {
		return OCICreateTask{}, err
	}
	m.startRunnerLocked(task)
	return cloneCreateTask(task), nil
}

func (m *OCICreateTaskManager) StopTask(taskID string) (OCICreateTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return OCICreateTask{}, fmt.Errorf("未找到创建任务 (%s)", taskID)
	}
	if stopCh, running := m.runners[taskID]; running {
		close(stopCh)
		delete(m.runners, taskID)
	}
	if task.Status == OCICreateTaskStatusRunning {
		task.Status = OCICreateTaskStatusStopped
		task.LastMessage = "任务已手动停止"
		task.UpdatedAt = time.Now().Format(time.RFC3339)
		m.appendTaskLogLocked(task, "任务已手动停止")
		_ = m.saveLocked()
	}
	return cloneCreateTask(task), nil
}

func (m *OCICreateTaskManager) startRunnerLocked(task *OCICreateTask) {
	if existing, ok := m.runners[task.ID]; ok {
		close(existing)
		delete(m.runners, task.ID)
	}
	stopCh := make(chan struct{})
	m.runners[task.ID] = stopCh
	taskID := task.ID

	go m.runLoop(taskID, stopCh)
}

func (m *OCICreateTaskManager) runLoop(taskID string, stopCh chan struct{}) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for {
		select {
		case <-stopCh:
			return
		default:
		}

		m.mu.RLock()
		taskPtr, exists := m.tasks[taskID]
		if !exists || taskPtr.Status != OCICreateTaskStatusRunning {
			m.mu.RUnlock()
			return
		}
		snapshot := cloneCreateTask(taskPtr)
		m.mu.RUnlock()

		var instanceID string
		var attemptErr error
		if m.serviceResolver == nil {
			attemptErr = fmt.Errorf("OCI 服务解析器未初始化")
		} else {
			svc, err := m.serviceResolver(snapshot.Account)
			if err != nil {
				attemptErr = err
			} else {
				instanceID, attemptErr = svc.LaunchInstance(snapshot)
			}
		}

		shouldContinue := m.recordAttemptResult(taskID, stopCh, instanceID, attemptErr)
		if !shouldContinue {
			return
		}

		waitSec := randomIntervalSeconds(rng, snapshot.MinIntervalSec, snapshot.MaxIntervalSec)
		timer := time.NewTimer(time.Duration(waitSec) * createTaskSleepUnit)
		select {
		case <-stopCh:
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (m *OCICreateTaskManager) recordAttemptResult(taskID string, stopCh chan struct{}, instanceID string, attemptErr error) bool {
	m.mu.Lock()

	if activeCh, ok := m.runners[taskID]; !ok || activeCh != stopCh {
		m.mu.Unlock()
		return false
	}
	task, ok := m.tasks[taskID]
	if !ok || task.Status != OCICreateTaskStatusRunning {
		m.mu.Unlock()
		return false
	}

	now := time.Now()
	task.Attempts++
	task.LastAttemptAt = now.Format(time.RFC3339)
	task.UpdatedAt = task.LastAttemptAt

	if attemptErr == nil && instanceID != "" {
		task.Status = OCICreateTaskStatusSucceeded
		task.CreatedInstanceID = instanceID
		task.LastError = ""
		task.LastMessage = fmt.Sprintf("创建成功！实例 ID: %s", instanceID)
		m.appendTaskLogLocked(task, fmt.Sprintf("第 %d 次尝试成功！实例 ID: %s", task.Attempts, instanceID))
		delete(m.runners, taskID)
		_ = m.saveLocked()
		succeededCopy := cloneCreateTask(task)
		cb := m.onSucceeded
		m.mu.Unlock()
		if cb != nil {
			go cb(succeededCopy)
		}
		return false
	}

	errText := "未知错误"
	if attemptErr != nil {
		errText = attemptErr.Error()
	}
	task.LastError = errText
	task.LastMessage = fmt.Sprintf("第 %d 次尝试未成功，等待重试", task.Attempts)
	m.appendTaskLogLocked(task, fmt.Sprintf("第 %d 次尝试失败: %s", task.Attempts, summarizeCreateError(errText)))
	_ = m.saveLocked()
	m.mu.Unlock()
	return true
}

func (m *OCICreateTaskManager) appendTaskLogLocked(task *OCICreateTask, msg string) {
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	task.Logs = append(task.Logs, entry)
	if len(task.Logs) > maxCreateTaskLogs {
		task.Logs = task.Logs[len(task.Logs)-maxCreateTaskLogs:]
	}
}

func (m *OCICreateTaskManager) saveLocked() error {
	if strings.TrimSpace(m.storePath) == "" {
		return nil
	}
	dir := filepath.Dir(m.storePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	ordered := make([]OCICreateTask, 0, len(m.order))
	for _, id := range m.order {
		if task := m.tasks[id]; task != nil {
			ordered = append(ordered, cloneCreateTask(task))
		}
	}

	payload, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := m.storePath + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, m.storePath)
}

func normalizeCreateTaskInput(input OCICreateTaskInput) (OCICreateTaskInput, error) {
	input.Account = strings.TrimSpace(input.Account)
	input.Name = strings.TrimSpace(input.Name)
	input.Template = strings.TrimSpace(input.Template)
	input.AvailabilityDomain = strings.TrimSpace(input.AvailabilityDomain)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Shape = strings.TrimSpace(input.Shape)
	input.ImageID = strings.TrimSpace(input.ImageID)
	input.ImageName = strings.TrimSpace(input.ImageName)
	input.VCNID = strings.TrimSpace(input.VCNID)
	input.SubnetID = strings.TrimSpace(input.SubnetID)
	input.SSHPublicKey = strings.TrimSpace(input.SSHPublicKey)

	if input.Account == "" {
		return input, fmt.Errorf("缺少 OCI 账号")
	}
	if input.AvailabilityDomain == "" {
		return input, fmt.Errorf("请选择可用域")
	}
	if input.Shape == "" {
		return input, fmt.Errorf("请选择实例规格")
	}
	if input.ImageID == "" {
		return input, fmt.Errorf("请选择系统镜像")
	}
	if input.SubnetID == "" {
		return input, fmt.Errorf("请选择子网")
	}
	if input.Name == "" {
		input.Name = valueOrDefault(input.DisplayName, input.Shape)
	}
	if input.Template == "" {
		input.Template = "custom"
	}
	if input.MinIntervalSec <= 0 {
		input.MinIntervalSec = 30
	}
	if input.MinIntervalSec < 5 {
		input.MinIntervalSec = 5
	}
	if input.MaxIntervalSec < input.MinIntervalSec {
		input.MaxIntervalSec = input.MinIntervalSec
	}

	cleanNSGs := make([]string, 0, len(input.NSGIDs))
	seen := make(map[string]struct{})
	for _, id := range input.NSGIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		cleanNSGs = append(cleanNSGs, id)
	}
	input.NSGIDs = cleanNSGs
	return input, nil
}

func randomIntervalSeconds(rng *rand.Rand, minSec, maxSec int) int {
	if minSec < 5 {
		minSec = 5
	}
	if maxSec <= minSec {
		return minSec
	}
	return minSec + rng.Intn(maxSec-minSec+1)
}

func summarizeCreateError(errText string) string {
	lower := strings.ToLower(errText)
	if strings.Contains(lower, "out of host capacity") || strings.Contains(lower, "out of capacity") {
		return "Out of host capacity（可用区容量不足）"
	}
	if strings.Contains(lower, "429") || strings.Contains(lower, "toomanyrequests") {
		return "429 TooManyRequests（请求过于频繁）"
	}
	if len(errText) > 220 {
		return errText[:220] + "..."
	}
	return errText
}

func cloneCreateTask(task *OCICreateTask) OCICreateTask {
	copyTask := *task
	if task.NSGIDs != nil {
		copyTask.NSGIDs = append([]string(nil), task.NSGIDs...)
	}
	if task.Logs != nil {
		copyTask.Logs = append([]string(nil), task.Logs...)
	}
	return copyTask
}
