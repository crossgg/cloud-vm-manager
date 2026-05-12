package main

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
	account  GCPConfig
	client   *http.Client
	key      *rsa.PrivateKey
	token    string
	tokenExp time.Time
}

func NewGCPService(account GCPConfig) *GCPService {
	return &GCPService{
		account: account,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (g *GCPService) ListVMs() ([]map[string]interface{}, error) {
	var result struct {
		Items map[string]struct {
			Instances []map[string]interface{} `json:"instances"`
		} `json:"items"`
	}
	if err := g.requestJSON("GET", fmt.Sprintf("%s/projects/%s/aggregated/instances", gcpComputeBaseURL, url.PathEscape(g.account.ProjectID)), nil, &result); err != nil {
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
		url.PathEscape(g.account.ProjectID),
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

func (g *GCPService) ChangeIP(id string) (*ChangeIPResult, error) {
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

	return &ChangeIPResult{
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
		url.PathEscape(g.account.ProjectID),
		url.PathEscape(zone),
		url.PathEscape(name),
	), nil, &instance)
	return instance, err
}

func (g *GCPService) reserveStaticAddress(region, addressName, ipAddress string) error {
	return g.requestOperation("POST", fmt.Sprintf(
		"%s/projects/%s/regions/%s/addresses",
		gcpComputeBaseURL,
		url.PathEscape(g.account.ProjectID),
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
		url.PathEscape(g.account.ProjectID),
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
		url.PathEscape(g.account.ProjectID),
		url.PathEscape(zone),
		url.PathEscape(name),
		action,
	), nil)
}

func (g *GCPService) accessConfigAction(zone, instanceName, action string, query url.Values, body map[string]interface{}) error {
	endpoint := fmt.Sprintf(
		"%s/projects/%s/zones/%s/instances/%s/%s?%s",
		gcpComputeBaseURL,
		url.PathEscape(g.account.ProjectID),
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
		"accountId":     g.account.Name,
		"group":         g.account.Group,
		"id":            zone + "|" + name,
		"name":          name,
		"location":      zone,
		"zone":          zone,
		"status":        gcpStatusText(stringValue(instance["status"])),
		"vmSize":        machineType,
		"privateIP":     privateIP,
		"publicIP":      publicIP,
		"resourceGroup": g.account.ProjectID,
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

	resp, err := g.client.Do(req)
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
		endpoint = fmt.Sprintf("%s/projects/%s/zones/%s/operations/%s", gcpComputeBaseURL, url.PathEscape(g.account.ProjectID), url.PathEscape(scopeName), url.PathEscape(name))
	case "region":
		endpoint = fmt.Sprintf("%s/projects/%s/regions/%s/operations/%s", gcpComputeBaseURL, url.PathEscape(g.account.ProjectID), url.PathEscape(scopeName), url.PathEscape(name))
	default:
		endpoint = fmt.Sprintf("%s/projects/%s/global/operations/%s", gcpComputeBaseURL, url.PathEscape(g.account.ProjectID), url.PathEscape(name))
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

	resp, err := g.client.PostForm("https://oauth2.googleapis.com/token", url.Values{
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
		return g.key, g.account.ClientEmail, nil
	}

	data, err := os.ReadFile(g.account.KeyFile)
	if err != nil {
		return nil, "", err
	}

	clientEmail := g.account.ClientEmail
	var keyFile struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
		ProjectID   string `json:"project_id"`
	}
	if json.Unmarshal(data, &keyFile) == nil && keyFile.PrivateKey != "" {
		if clientEmail == "" {
			clientEmail = keyFile.ClientEmail
		}
		if g.account.ProjectID == "" {
			g.account.ProjectID = keyFile.ProjectID
		}
		g.key, err = parseRSAPrivateKeyPEM([]byte(keyFile.PrivateKey), g.account.KeyFile)
		return g.key, clientEmail, err
	}

	key, err := loadRSAPrivateKey(g.account.KeyFile)
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
