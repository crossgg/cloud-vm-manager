package gcp

import (
	cryptoutil "github.com/crossgg/cloud-vm-manager/internal/crypto"
	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/provider"
)

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const gcpComputeBaseURL = "https://compute.googleapis.com/compute/v1"

type GCPService struct {
	Account  config.GCPConfig
	Client   *http.Client
	key      *rsa.PrivateKey
	token    string
	tokenExp time.Time
}

func NewGCPService(account config.GCPConfig) *GCPService {
	return &GCPService{
		Account: account,
		Client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (g *GCPService) ListVMs() ([]map[string]interface{}, error) {
	var result struct {
		Items map[string]struct {
			Instances []map[string]interface{} `json:"instances"`
		} `json:"items"`
	}
	if err := g.requestJSON("GET", fmt.Sprintf("%s/projects/%s/aggregated/instances", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID)), nil, &result); err != nil {
		return nil, err
	}

	vms := make([]map[string]interface{}, 0)
	for _, scopedList := range result.Items {
		for _, instance := range scopedList.Instances {
			vms = append(vms, g.normalizeVM(instance))
		}
	}
	return vms, nil
}

func (g *GCPService) GetVM(id string) (map[string]interface{}, error) {
	zone, name, err := parseGCPID(id)
	if err != nil {
		return nil, err
	}

	var instance map[string]interface{}
	err = g.requestJSON("GET", fmt.Sprintf(
		"%s/projects/%s/zones/%s/instances/%s",
		gcpComputeBaseURL,
		url.PathEscape(g.Account.ProjectID),
		url.PathEscape(zone),
		url.PathEscape(name),
	), nil, &instance)
	if err != nil {
		return nil, err
	}
	return g.normalizeVM(instance), nil
}

func (g *GCPService) StartVM(id string) error {
	return g.instanceAction(id, "start")
}

func (g *GCPService) StopVM(id string) error {
	return g.instanceAction(id, "stop")
}

func (g *GCPService) RestartVM(id string) error {
	return g.instanceAction(id, "reset")
}

func (g *GCPService) ChangeIP(id string) (*provider.ChangeIPResult, error) {
	zone, name, err := parseGCPID(id)
	if err != nil {
		return nil, err
	}

	instance, err := g.rawInstance(zone, name)
	if err != nil {
		return nil, err
	}

	nicName, accessConfigName, oldIP := g.firstNetworkAccessConfig(instance)
	if nicName == "" {
		return nil, fmt.Errorf("GCP instance %s has no network interface", name)
	}
	if accessConfigName == "" {
		accessConfigName = "External NAT"
	}

	region := regionFromZone(zone)
	oldAddressName := ""
	logs := []string{}
	if oldIP != "" {
		oldAddressName = fmt.Sprintf("old-ip-%s-%d", strings.ToLower(name), time.Now().Unix())
		logs = append(logs, fmt.Sprintf("[1/4] reserving current ephemeral IP as static: %s", oldIP))
		if err := g.reserveStaticAddress(region, oldAddressName, oldIP); err != nil {
			return nil, fmt.Errorf("reserve current IP failed: %w", err)
		}

		logs = append(logs, "[2/4] detaching old public IP from network interface")
		if err := g.accessConfigAction(zone, name, "deleteAccessConfig", url.Values{
			"networkInterface": {nicName},
			"accessConfig":     {accessConfigName},
		}, nil); err != nil {
			return nil, fmt.Errorf("detach old public IP failed: %w", err)
		}
	} else {
		logs = append(logs, "[1/4] no current public IP")
		logs = append(logs, "[2/4] skip detach")
	}

	logs = append(logs, "[3/4] attaching a new ephemeral public IP")
	if err := g.accessConfigAction(zone, name, "addAccessConfig", url.Values{
		"networkInterface": {nicName},
	}, map[string]interface{}{
		"name":        accessConfigName,
		"type":        "ONE_TO_ONE_NAT",
		"networkTier": "PREMIUM",
	}); err != nil {
		return nil, fmt.Errorf("attach new public IP failed: %w", err)
	}

	newIP := ""
	if updated, err := g.GetVM(id); err == nil {
		if publicIP, ok := updated["publicIP"].(map[string]interface{}); ok {
			newIP, _ = publicIP["ipAddress"].(string)
		}
	}
	logs = append(logs, fmt.Sprintf("[3/4] new ephemeral public IP: %s", valueOrDefault(newIP, "pending")))

	if oldAddressName != "" {
		logs = append(logs, fmt.Sprintf("[4/4] deleting old static IP resource: %s", oldAddressName))
		if err := g.deleteStaticAddress(region, oldAddressName); err != nil {
			return nil, fmt.Errorf("delete old static IP failed: %w", err)
		}
	} else {
		logs = append(logs, "[4/4] no old static IP resource to delete")
	}

	return &provider.ChangeIPResult{
		Success:      true,
		Message:      "IP changed",
		NewIPAddress: newIP,
		Logs:         logs,
	}, nil
}

func (g *GCPService) rawInstance(zone, name string) (map[string]interface{}, error) {
	var instance map[string]interface{}
	err := g.requestJSON("GET", fmt.Sprintf(
		"%s/projects/%s/zones/%s/instances/%s",
		gcpComputeBaseURL,
		url.PathEscape(g.Account.ProjectID),
		url.PathEscape(zone),
		url.PathEscape(name),
	), nil, &instance)
	return instance, err
}

func (g *GCPService) reserveStaticAddress(region, addressName, ipAddress string) error {
	return g.requestOperation("POST", fmt.Sprintf(
		"%s/projects/%s/regions/%s/addresses",
		gcpComputeBaseURL,
		url.PathEscape(g.Account.ProjectID),
		url.PathEscape(region),
	), map[string]interface{}{
		"name":        addressName,
		"address":     ipAddress,
		"addressType": "EXTERNAL",
	})
}

func (g *GCPService) deleteStaticAddress(region, addressName string) error {
	return g.requestOperation("DELETE", fmt.Sprintf(
		"%s/projects/%s/regions/%s/addresses/%s",
		gcpComputeBaseURL,
		url.PathEscape(g.Account.ProjectID),
		url.PathEscape(region),
		url.PathEscape(addressName),
	), nil)
}

func (g *GCPService) instanceAction(id, action string) error {
	zone, name, err := parseGCPID(id)
	if err != nil {
		return err
	}
	return g.requestOperation("POST", fmt.Sprintf(
		"%s/projects/%s/zones/%s/instances/%s/%s",
		gcpComputeBaseURL,
		url.PathEscape(g.Account.ProjectID),
		url.PathEscape(zone),
		url.PathEscape(name),
		action,
	), nil)
}

func (g *GCPService) accessConfigAction(zone, instanceName, action string, query url.Values, body map[string]interface{}) error {
	endpoint := fmt.Sprintf(
		"%s/projects/%s/zones/%s/instances/%s/%s?%s",
		gcpComputeBaseURL,
		url.PathEscape(g.Account.ProjectID),
		url.PathEscape(zone),
		url.PathEscape(instanceName),
		action,
		query.Encode(),
	)
	return g.requestOperation("POST", endpoint, body)
}

func (g *GCPService) normalizeVM(instance map[string]interface{}) map[string]interface{} {
	name := stringValue(instance["name"])
	zone := resourceTail(stringValue(instance["zone"]))
	machineType := resourceTail(stringValue(instance["machineType"]))
	privateIP := "unassigned"
	publicIP := map[string]interface{}{"ipAddress": "unassigned", "name": "N/A"}

	if nicName, accessName, natIP := g.firstNetworkAccessConfig(instance); nicName != "" {
		if networkInterfaces, ok := instance["networkInterfaces"].([]interface{}); ok && len(networkInterfaces) > 0 {
			if nic, ok := networkInterfaces[0].(map[string]interface{}); ok {
				privateIP = valueOrDefault(stringValue(nic["networkIP"]), "unassigned")
			}
		}
		if natIP != "" {
			publicIP["ipAddress"] = natIP
			publicIP["name"] = valueOrDefault(accessName, "External NAT")
		}
	}

	return map[string]interface{}{
		"provider":      "gcp",
		"accountId":     g.Account.Name,
		"group":         g.Account.Group,
		"id":            zone + "|" + name,
		"name":          name,
		"location":      zone,
		"zone":          zone,
		"status":        gcpStatusText(stringValue(instance["status"])),
		"vmSize":        machineType,
		"privateIP":     privateIP,
		"publicIP":      publicIP,
		"resourceGroup": g.Account.ProjectID,
	}
}

func (g *GCPService) firstNetworkAccessConfig(instance map[string]interface{}) (string, string, string) {
	networkInterfaces, ok := instance["networkInterfaces"].([]interface{})
	if !ok || len(networkInterfaces) == 0 {
		return "", "", ""
	}
	nic, ok := networkInterfaces[0].(map[string]interface{})
	if !ok {
		return "", "", ""
	}
	nicName := valueOrDefault(stringValue(nic["name"]), "nic0")
	accessConfigs, ok := nic["accessConfigs"].([]interface{})
	if !ok || len(accessConfigs) == 0 {
		return nicName, "", ""
	}
	accessConfig, ok := accessConfigs[0].(map[string]interface{})
	if !ok {
		return nicName, "", ""
	}
	return nicName, stringValue(accessConfig["name"]), stringValue(accessConfig["natIP"])
}

func (g *GCPService) requestJSON(method, endpoint string, body interface{}, out interface{}) error {
	token, err := g.getToken()
	if err != nil {
		return err
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GCP API %s %s returned %d: %s", method, endpoint, resp.StatusCode, string(data))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (g *GCPService) requestOperation(method, endpoint string, body interface{}) error {
	var operation map[string]interface{}
	if err := g.requestJSON(method, endpoint, body, &operation); err != nil {
		return err
	}
	return g.waitOperation(operation)
}

func (g *GCPService) waitOperation(operation map[string]interface{}) error {
	if operation == nil || stringValue(operation["name"]) == "" || stringValue(operation["status"]) == "DONE" {
		return g.operationError(operation)
	}

	name := stringValue(operation["name"])
	scope, scopeName := g.operationScope(operation)
	var endpoint string
	switch scope {
	case "zone":
		endpoint = fmt.Sprintf("%s/projects/%s/zones/%s/operations/%s", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(scopeName), url.PathEscape(name))
	case "region":
		endpoint = fmt.Sprintf("%s/projects/%s/regions/%s/operations/%s", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(scopeName), url.PathEscape(name))
	default:
		endpoint = fmt.Sprintf("%s/projects/%s/global/operations/%s", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(name))
	}

	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		var latest map[string]interface{}
		if err := g.requestJSON("GET", endpoint, nil, &latest); err != nil {
			return err
		}
		if stringValue(latest["status"]) == "DONE" {
			return g.operationError(latest)
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("GCP operation %s timed out", name)
}

func (g *GCPService) operationError(operation map[string]interface{}) error {
	if operation == nil {
		return nil
	}
	rawError, ok := operation["error"].(map[string]interface{})
	if !ok || len(rawError) == 0 {
		return nil
	}
	data, _ := json.Marshal(rawError)
	return fmt.Errorf("GCP operation failed: %s", string(data))
}

func (g *GCPService) operationScope(operation map[string]interface{}) (string, string) {
	if zone := resourceTail(stringValue(operation["zone"])); zone != "" {
		return "zone", zone
	}
	if region := resourceTail(stringValue(operation["region"])); region != "" {
		return "region", region
	}
	if selfLink := stringValue(operation["selfLink"]); selfLink != "" {
		parts := strings.Split(selfLink, "/")
		for i := 0; i < len(parts)-1; i++ {
			if parts[i] == "zones" {
				return "zone", parts[i+1]
			}
			if parts[i] == "regions" {
				return "region", parts[i+1]
			}
		}
	}
	return "global", "global"
}

func (g *GCPService) getToken() (string, error) {
	if g.token != "" && time.Now().Before(g.tokenExp.Add(-1*time.Minute)) {
		return g.token, nil
	}

	key, clientEmail, err := g.loadCredential()
	if err != nil {
		return "", err
	}

	now := time.Now()
	assertion, err := signJWT(map[string]string{"alg": "RS256", "typ": "JWT"}, map[string]interface{}{
		"iss":   clientEmail,
		"scope": "https://www.googleapis.com/auth/compute",
		"aud":   "https://oauth2.googleapis.com/token",
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}, key)
	if err != nil {
		return "", err
	}

	resp, err := g.Client.PostForm("https://oauth2.googleapis.com/token", url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("GCP token endpoint returned %d: %s", resp.StatusCode, string(data))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(data, &tokenResp); err != nil {
		return "", err
	}
	if tokenResp.ExpiresIn == 0 {
		tokenResp.ExpiresIn = 3600
	}
	g.token = tokenResp.AccessToken
	g.tokenExp = now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return g.token, nil
}

func (g *GCPService) loadCredential() (*rsa.PrivateKey, string, error) {
	if g.key != nil {
		return g.key, g.Account.ClientEmail, nil
	}

	data, err := os.ReadFile(g.Account.KeyFile)
	if err != nil {
		return nil, "", err
	}

	clientEmail := g.Account.ClientEmail
	var keyFile struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		ProjectID   string `json:"project_id"`
	}
	if json.Unmarshal(data, &keyFile) == nil && keyFile.PrivateKey != "" {
		if clientEmail == "" {
			clientEmail = keyFile.ClientEmail
		}
		if g.Account.ProjectID == "" {
			g.Account.ProjectID = keyFile.ProjectID
		}
		g.key, err = cryptoutil.ParseRSAPrivateKeyPEM([]byte(keyFile.PrivateKey), g.Account.KeyFile)
		return g.key, clientEmail, err
	}

	key, err := cryptoutil.LoadRSAPrivateKey(g.Account.KeyFile)
	if err != nil {
		return nil, "", err
	}
	g.key = key
	return g.key, clientEmail, nil
}

func signJWT(header map[string]string, claims map[string]interface{}, key *rsa.PrivateKey) (string, error) {
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	unsigned := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func parseGCPID(id string) (string, string, error) {
	zone, name, ok := strings.Cut(id, "|")
	if !ok || zone == "" || name == "" {
		return "", "", fmt.Errorf("invalid GCP VM id %q", id)
	}
	return zone, name, nil
}

func gcpStatusText(status string) string {
	switch status {
	case "RUNNING":
		return "VM running"
	case "TERMINATED":
		return "VM stopped"
	default:
		if status == "" {
			return "Unknown"
		}
		return status
	}
}

func resourceTail(resource string) string {
	if resource == "" {
		return ""
	}
	parts := strings.Split(resource, "/")
	return parts[len(parts)-1]
}

func regionFromZone(zone string) string {
	lastDash := strings.LastIndex(zone, "-")
	if lastDash <= 0 {
		return zone
	}
	return zone[:lastDash]
}

func stringValue(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

type GCPFirewallRule struct {
	ID                string   `json:"id,omitempty"`
	CreationTimestamp string   `json:"creationTimestamp,omitempty"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Network           string   `json:"network,omitempty"`
	Priority          int      `json:"priority,omitempty"`
	Direction         string   `json:"direction,omitempty"` // INGRESS or EGRESS
	Action            string   `json:"action,omitempty"`    // ALLOW or DENY
	SourceRanges      []string `json:"sourceRanges,omitempty"`
	DestinationRanges []string `json:"destinationRanges,omitempty"`
	SourceTags        []string `json:"sourceTags,omitempty"`
	TargetTags        []string `json:"targetTags,omitempty"`
	IPProtocol        string   `json:"ipProtocol,omitempty"`
	Ports             []string `json:"ports,omitempty"`
	Disabled          bool     `json:"disabled"`
}

func (g *GCPService) ListFirewalls() ([]GCPFirewallRule, error) {
	var result struct {
		Items []map[string]interface{} `json:"items"`
	}
	endpoint := fmt.Sprintf("%s/projects/%s/global/firewalls", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID))
	if err := g.requestJSON("GET", endpoint, nil, &result); err != nil {
		return nil, err
	}

	rules := make([]GCPFirewallRule, 0, len(result.Items))
	for _, item := range result.Items {
		rules = append(rules, g.normalizeFirewallRule(item))
	}
	return rules, nil
}

func (g *GCPService) normalizeFirewallRule(item map[string]interface{}) GCPFirewallRule {
	rule := GCPFirewallRule{
		ID:                stringValue(item["id"]),
		CreationTimestamp: stringValue(item["creationTimestamp"]),
		Name:              stringValue(item["name"]),
		Description:       stringValue(item["description"]),
		Network:           resourceTail(stringValue(item["network"])),
		Direction:         stringValue(item["direction"]),
	}
	if p, ok := item["priority"].(float64); ok {
		rule.Priority = int(p)
	}
	if d, ok := item["disabled"].(bool); ok {
		rule.Disabled = d
	}
	if rule.Direction == "" {
		rule.Direction = "INGRESS"
	}

	rule.SourceRanges = toStringSlice(item["sourceRanges"])
	rule.DestinationRanges = toStringSlice(item["destinationRanges"])
	rule.SourceTags = toStringSlice(item["sourceTags"])
	rule.TargetTags = toStringSlice(item["targetTags"])

	if allowedList, ok := item["allowed"].([]interface{}); ok && len(allowedList) > 0 {
		rule.Action = "ALLOW"
		rule.IPProtocol, rule.Ports = parseProtocolAndPorts(allowedList)
	} else if deniedList, ok := item["denied"].([]interface{}); ok && len(deniedList) > 0 {
		rule.Action = "DENY"
		rule.IPProtocol, rule.Ports = parseProtocolAndPorts(deniedList)
	} else {
		rule.Action = "ALLOW"
		rule.IPProtocol = "all"
	}

	return rule
}

func parseProtocolAndPorts(list []interface{}) (string, []string) {
	protocols := make([]string, 0)
	ports := make([]string, 0)
	for _, entry := range list {
		if m, ok := entry.(map[string]interface{}); ok {
			proto := stringValue(m["IPProtocol"])
			if proto != "" {
				protocols = append(protocols, proto)
			}
			ports = append(ports, toStringSlice(m["ports"])...)
		}
	}
	protoStr := strings.Join(protocols, ",")
	if protoStr == "" {
		protoStr = "tcp"
	}
	return protoStr, ports
}

func (g *GCPService) buildFirewallPayload(rule GCPFirewallRule) map[string]interface{} {
	priority := rule.Priority
	if priority <= 0 {
		priority = 1000
	}
	direction := strings.ToUpper(strings.TrimSpace(rule.Direction))
	if direction == "" {
		direction = "INGRESS"
	}

	network := rule.Network
	if network == "" || network == "default" {
		network = fmt.Sprintf("projects/%s/global/networks/default", g.Account.ProjectID)
	} else if !strings.HasPrefix(network, "projects/") && !strings.HasPrefix(network, "global/") {
		network = fmt.Sprintf("projects/%s/global/networks/%s", g.Account.ProjectID, network)
	}

	payload := map[string]interface{}{
		"name":        strings.TrimSpace(rule.Name),
		"description": rule.Description,
		"priority":    priority,
		"direction":   direction,
		"network":     network,
		"disabled":    rule.Disabled,
	}

	if len(rule.TargetTags) > 0 {
		payload["targetTags"] = rule.TargetTags
	}

	if direction == "INGRESS" {
		if len(rule.SourceRanges) > 0 {
			payload["sourceRanges"] = rule.SourceRanges
		} else {
			payload["sourceRanges"] = []string{"0.0.0.0/0"}
		}
	} else {
		if len(rule.DestinationRanges) > 0 {
			payload["destinationRanges"] = rule.DestinationRanges
		} else {
			payload["destinationRanges"] = []string{"0.0.0.0/0"}
		}
	}

	proto := strings.ToLower(strings.TrimSpace(rule.IPProtocol))
	if proto == "" {
		proto = "tcp"
	}

	var protoEntry map[string]interface{}
	if proto == "all" {
		protoEntry = map[string]interface{}{
			"IPProtocol": "all",
		}
	} else {
		protoEntry = map[string]interface{}{
			"IPProtocol": proto,
		}
		if len(rule.Ports) > 0 && proto != "icmp" && proto != "esp" && proto != "ah" {
			protoEntry["ports"] = rule.Ports
		}
	}

	if strings.ToUpper(strings.TrimSpace(rule.Action)) == "DENY" {
		payload["denied"] = []map[string]interface{}{protoEntry}
	} else {
		payload["allowed"] = []map[string]interface{}{protoEntry}
	}

	return payload
}

func (g *GCPService) CreateFirewall(rule GCPFirewallRule) error {
	endpoint := fmt.Sprintf("%s/projects/%s/global/firewalls", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID))
	payload := g.buildFirewallPayload(rule)
	return g.requestOperation("POST", endpoint, payload)
}

func (g *GCPService) UpdateFirewall(name string, rule GCPFirewallRule) error {
	endpoint := fmt.Sprintf("%s/projects/%s/global/firewalls/%s", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(name))
	payload := g.buildFirewallPayload(rule)
	return g.requestOperation("PATCH", endpoint, payload)
}

func (g *GCPService) ToggleFirewall(name string, disabled bool) error {
	endpoint := fmt.Sprintf("%s/projects/%s/global/firewalls/%s", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(name))
	payload := map[string]interface{}{
		"disabled": disabled,
	}
	return g.requestOperation("PATCH", endpoint, payload)
}

func (g *GCPService) DeleteFirewall(name string) error {
	endpoint := fmt.Sprintf("%s/projects/%s/global/firewalls/%s", gcpComputeBaseURL, url.PathEscape(g.Account.ProjectID), url.PathEscape(name))
	return g.requestOperation("DELETE", endpoint, nil)
}

func toStringSlice(val interface{}) []string {
	if val == nil {
		return []string{}
	}
	if arr, ok := val.([]interface{}); ok {
		res := make([]string, 0, len(arr))
		for _, item := range arr {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				res = append(res, strings.TrimSpace(s))
			}
		}
		return res
	}
	if arr, ok := val.([]string); ok {
		return arr
	}
	return []string{}
}


func loadPrivateKeyHelper(path, pemStr string) (*rsa.PrivateKey, error) {
	if strings.TrimSpace(path) != "" {
		return cryptoutil.LoadRSAPrivateKey(path)
	}
	return cryptoutil.ParseRSAPrivateKeyPEM([]byte(pemStr), "GCP private key")
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
