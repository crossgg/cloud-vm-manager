package oci

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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var ociTransientRetryDelay = 150 * time.Millisecond

type OCIService struct {
	Account    config.OCIConfig
	Client     *http.Client
	key        *rsa.PrivateKey
	mu         sync.Mutex
	homeRegion string
}

func NewOCIService(account config.OCIConfig) *OCIService {
	return &OCIService{
		Account: account,
		Client:  &http.Client{Timeout: 60 * time.Second},
	}
}

func (o *OCIService) ListVMs() ([]map[string]interface{}, error) {
	var instances []map[string]interface{}
	err := o.requestJSON("GET", "/instances", url.Values{
		"compartmentId": {o.Account.CompartmentID},
	}, nil, &instances, false)
	if err != nil {
		return nil, err
	}

	vms := make([]map[string]interface{}, 0, len(instances))
	for _, instance := range instances {
		vms = append(vms, o.normalizeVM(instance))
	}
	return vms, nil
}

func (o *OCIService) GetVM(id string) (map[string]interface{}, error) {
	var instance map[string]interface{}
	if err := o.requestJSON("GET", "/instances/"+url.PathEscape(id), nil, nil, &instance, false); err != nil {
		return nil, err
	}
	return o.normalizeVM(instance), nil
}

func (o *OCIService) StartVM(id string) error {
	return o.instanceAction(id, "START")
}

func (o *OCIService) StopVM(id string) error {
	return o.instanceAction(id, "STOP")
}

func (o *OCIService) RestartVM(id string) error {
	return o.instanceAction(id, "RESET")
}

func (o *OCIService) ChangeIP(id string) (*provider.ChangeIPResult, error) {
	return o.ChangePrivateIPPublicIP(id, "")
}

func (o *OCIService) TerminateInstance(id string, preserveBootVolume bool) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("missing OCI instance id")
	}
	query := url.Values{
		"preserveBootVolume": {fmt.Sprintf("%t", preserveBootVolume)},
	}
	return o.requestJSON("DELETE", "/instances/"+url.PathEscape(id), query, nil, nil, false)
}

func (o *OCIService) deletePublicIP(publicIPID string) error {
	return o.requestJSON("DELETE", "/publicIps/"+url.PathEscape(publicIPID), nil, nil, nil, false)
}

func (o *OCIService) unassignReservedPublicIP(publicIPID string) error {
	return o.requestJSON("PUT", "/publicIps/"+url.PathEscape(publicIPID), nil, map[string]interface{}{
		"privateIpId": nil,
	}, nil, false)
}

func (o *OCIService) waitNoPublicIP(privateIPID string) error {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		publicIP, err := o.publicIPByPrivateIP(privateIPID)
		if err != nil {
			return err
		}
		if publicIP == nil || stringValue(publicIP["id"]) == "" {
			return nil
		}
		time.Sleep(ociIPPollInterval)
	}
	return fmt.Errorf("等待 OCI 私有 IP 解除公网 IP 绑定超时")
}

func (o *OCIService) instanceAction(id, action string) error {
	return o.requestJSON("POST", "/instances/"+url.PathEscape(id), url.Values{"action": {action}}, nil, nil, false)
}

func (o *OCIService) normalizeVM(instance map[string]interface{}) map[string]interface{} {
	id := stringValue(instance["id"])
	name := valueOrDefault(stringValue(instance["displayName"]), id)
	shape := stringValue(instance["shape"])
	shapeConfig, _ := instance["shapeConfig"].(map[string]interface{})
	sourceDetails, _ := instance["sourceDetails"].(map[string]interface{})
	region := o.Account.Region
	privateIP := "未分配"
	publicIP := map[string]interface{}{"ipAddress": "未分配", "name": "N/A"}
	ipv6Display := "未分配"
	ipv6Items := make([]map[string]interface{}, 0)

	if primaryIP, vnic, err := o.primaryPrivateIP(id); err == nil {
		privateIP = valueOrDefault(stringValue(primaryIP["ipAddress"]), valueOrDefault(stringValue(vnic["privateIp"]), "未分配"))
		if pub, err := o.publicIPByPrivateIP(stringValue(primaryIP["id"])); err == nil && pub != nil {
			publicIP["ipAddress"] = valueOrDefault(stringValue(pub["ipAddress"]), "未分配")
			publicIP["name"] = valueOrDefault(stringValue(pub["displayName"]), valueOrDefault(stringValue(pub["id"]), "N/A"))
		} else if stringValue(vnic["publicIp"]) != "" {
			publicIP["ipAddress"] = stringValue(vnic["publicIp"])
			publicIP["name"] = "ephemeral"
		}
		if list, err := o.listVNICIPv6s(stringValue(vnic["id"])); err == nil && len(list) > 0 {
			for _, ip6 := range list {
				ipv6Items = append(ipv6Items, map[string]interface{}{
					"id":               ip6.ID,
					"ipAddress":        ip6.IPAddress,
					"cidrPrefixLength": ip6.CIDRPrefixLength,
				})
				if ipv6Display == "未分配" && ip6.CanChange {
					ipv6Display = ip6.IPAddress
				}
			}
			if ipv6Display == "未分配" {
				first := list[0]
				if first.CIDRPrefixLength != 128 {
					ipv6Display = fmt.Sprintf("%s/%d", first.IPAddress, first.CIDRPrefixLength)
				} else {
					ipv6Display = first.IPAddress
				}
			}
		}
	}

	return map[string]interface{}{
		"provider":           "oci",
		"accountId":          o.Account.Name,
		"group":              o.Account.Group,
		"id":                 id,
		"name":               name,
		"location":           region,
		"status":             ociStatusText(stringValue(instance["lifecycleState"])),
		"vmSize":             shape,
		"ocpus":              float64InterfaceValue(shapeConfig["ocpus"]),
		"memoryInGBs":        float64InterfaceValue(shapeConfig["memoryInGBs"]),
		"availabilityDomain": stringValue(instance["availabilityDomain"]),
		"imageId":            stringValue(sourceDetails["imageId"]),
		"privateIP":          privateIP,
		"publicIP":           publicIP,
		"ipv6":               ipv6Display,
		"ipv6s":              ipv6Items,
		"resourceGroup":      o.Account.CompartmentID,
	}
}

func (o *OCIService) primaryPrivateIP(instanceID string) (map[string]interface{}, map[string]interface{}, error) {
	var attachments []map[string]interface{}
	err := o.requestJSON("GET", "/vnicAttachments", url.Values{
		"compartmentId": {o.Account.CompartmentID},
		"instanceId":    {instanceID},
	}, nil, &attachments, false)
	if err != nil {
		return nil, nil, err
	}

	for _, attachment := range attachments {
		if state := stringValue(attachment["lifecycleState"]); state != "" && state != "ATTACHED" {
			continue
		}
		vnicID := stringValue(attachment["vnicId"])
		if vnicID == "" {
			continue
		}

		var vnic map[string]interface{}
		if err := o.requestJSON("GET", "/vnics/"+url.PathEscape(vnicID), nil, nil, &vnic, false); err != nil {
			return nil, nil, err
		}

		var privateIPs []map[string]interface{}
		if err := o.requestJSON("GET", "/privateIps", url.Values{"vnicId": {vnicID}}, nil, &privateIPs, false); err != nil {
			return nil, nil, err
		}
		if len(privateIPs) == 0 {
			continue
		}
		for _, privateIP := range privateIPs {
			if primary, _ := privateIP["isPrimary"].(bool); primary {
				return privateIP, vnic, nil
			}
		}
		return privateIPs[0], vnic, nil
	}

	return nil, nil, fmt.Errorf("no attached VNIC found for OCI instance %s", instanceID)
}

func (o *OCIService) publicIPByPrivateIP(privateIPID string) (map[string]interface{}, error) {
	var publicIP map[string]interface{}
	err := o.requestJSON("POST", "/publicIps/actions/getByPrivateIpId", nil, map[string]interface{}{
		"privateIpId": privateIPID,
	}, &publicIP, true)
	if err != nil {
		return nil, err
	}
	return publicIP, nil
}

func (o *OCIService) requestJSON(method, path string, query url.Values, body interface{}, out interface{}, allowNotFound bool) error {
	endpoint := o.endpoint(path, query)
	return o.doRequest(method, endpoint, body, out, allowNotFound)
}

// doRequest sends a signed request to the given full endpoint URL.
// This is the shared implementation used by both iaas and monitoring APIs.
func (o *OCIService) doRequest(method, endpoint string, body interface{}, out interface{}, allowNotFound bool) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	} else if method == "POST" || method == "PUT" {
		payload = []byte{}
	}

	resp, err := o.sendSignedRequest(method, endpoint, "application/json", payload)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if allowNotFound && resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("OCI API %s %s returned %d: %s", method, endpoint, resp.StatusCode, string(data))
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (o *OCIService) endpoint(path string, query url.Values) string {
	return buildServiceEndpoint("iaas."+o.Account.Region+".oraclecloud.com", "/20160918"+path, query)
}

func (o *OCIService) identityEndpoint(region, path string, query url.Values) string {
	if strings.TrimSpace(region) == "" {
		region = o.Account.Region
	}
	return buildServiceEndpoint("identity."+region+".oraclecloud.com", "/20160918"+path, query)
}

func (o *OCIService) limitsEndpoint(path string, query url.Values) string {
	return buildServiceEndpoint("limits."+o.Account.Region+".oci.oraclecloud.com", "/20190729"+path, query)
}

func (o *OCIService) usageEndpoint(region, path string, query url.Values) string {
	if strings.TrimSpace(region) == "" {
		region = o.Account.Region
	}
	return buildServiceEndpoint("usageapi."+region+".oci.oraclecloud.com", "/20200107"+path, query)
}

func (o *OCIService) objectStorageEndpoint(path string, query url.Values) string {
	return buildServiceEndpoint("objectstorage."+o.Account.Region+".oraclecloud.com", path, query)
}

func buildServiceEndpoint(host, fullPath string, query url.Values) string {
	u := url.URL{
		Scheme: "https",
		Host:   host,
		Path:   fullPath,
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}
	return u.String()
}

func (o *OCIService) identityRequestJSON(region, method, path string, query url.Values, body, out interface{}, allowNotFound bool) error {
	return o.doRequest(method, o.identityEndpoint(region, path, query), body, out, allowNotFound)
}

func (o *OCIService) limitsRequestJSON(method, path string, query url.Values, body, out interface{}, allowNotFound bool) error {
	return o.doRequest(method, o.limitsEndpoint(path, query), body, out, allowNotFound)
}

func (o *OCIService) usageRequestJSON(region, method, path string, query url.Values, body, out interface{}, allowNotFound bool) error {
	return o.doRequest(method, o.usageEndpoint(region, path, query), body, out, allowNotFound)
}

func (o *OCIService) HomeRegion() string {
	o.mu.Lock()
	cached := o.homeRegion
	o.mu.Unlock()
	if cached != "" {
		return cached
	}

	resolved := o.Account.Region
	if strings.TrimSpace(o.Account.Tenancy) != "" {
		var subs []map[string]interface{}
		if err := o.identityRequestJSON(o.Account.Region, "GET", "/tenancies/"+url.PathEscape(o.Account.Tenancy)+"/regionSubscriptions", nil, nil, &subs, false); err == nil {
			for _, sub := range subs {
				if isHome, _ := sub["isHomeRegion"].(bool); isHome {
					if name := strings.TrimSpace(stringValue(sub["regionName"])); name != "" {
						resolved = name
						break
					}
				}
			}
		}
	}

	o.mu.Lock()
	o.homeRegion = resolved
	o.mu.Unlock()
	return resolved
}

func (o *OCIService) doRawRequest(method, endpoint, contentType string, payload []byte, allowNotFound bool) (int, http.Header, []byte, error) {
	if payload == nil && (method == "POST" || method == "PUT") {
		payload = []byte{}
	}

	resp, err := o.sendSignedRequest(method, endpoint, contentType, payload)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, nil, err
	}
	if allowNotFound && resp.StatusCode == http.StatusNotFound {
		return resp.StatusCode, resp.Header, data, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, resp.Header, data, fmt.Errorf("OCI API %s %s returned %d: %s", method, endpoint, resp.StatusCode, string(data))
	}
	return resp.StatusCode, resp.Header, data, nil
}

func (o *OCIService) doRawFileRequest(method, endpoint, contentType string, file *os.File, size int64, contentSHA256Base64 string) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var body io.Reader = file
	if size == 0 {
		body = http.NoBody
	}
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return err
	}
	req.ContentLength = size
	if err := o.signRequestWithDigest(req, contentType, size, contentSHA256Base64); err != nil {
		return err
	}
	resp, err := o.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("OCI API %s %s returned %d: %s", method, endpoint, resp.StatusCode, string(data))
	}
	return nil
}

func (o *OCIService) sendSignedRequest(method, endpoint, contentType string, payload []byte) (*http.Response, error) {
	const maxAttempts = 3
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			closeClientIdleConnections(o.Client)
			if ociTransientRetryDelay > 0 {
				time.Sleep(ociTransientRetryDelay)
			}
		}

		var reader io.Reader
		if payload != nil {
			reader = bytes.NewReader(payload)
		}
		req, err := http.NewRequest(method, endpoint, reader)
		if err != nil {
			return nil, err
		}
		if attempt > 0 {
			// Force a fresh TCP/TLS connection when recovering from stale keep-alive EOF.
			req.Close = true
		}
		if err := o.signWithContentType(req, payload, contentType); err != nil {
			return nil, err
		}

		resp, err := o.Client.Do(req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !shouldRetryOCITransientError(method, err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func shouldRetryOCITransientError(method string, err error) bool {
	if err == nil {
		return false
	}
	if method != http.MethodGet && method != http.MethodHead {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "eof") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "client connection lost") ||
		strings.Contains(msg, "server sent goaway")
}

func closeClientIdleConnections(client *http.Client) {
	if client == nil {
		return
	}
	if client.Transport == nil {
		if c, ok := http.DefaultTransport.(interface{ CloseIdleConnections() }); ok {
			c.CloseIdleConnections()
		}
		return
	}
	if c, ok := client.Transport.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

func (o *OCIService) sign(req *http.Request, payload []byte) error {
	return o.signWithContentType(req, payload, "application/json")
}

func (o *OCIService) signWithContentType(req *http.Request, payload []byte, contentType string) error {
	sum := sha256.Sum256(payload)
	return o.signRequestWithDigest(req, contentType, int64(len(payload)), base64.StdEncoding.EncodeToString(sum[:]))
}

func (o *OCIService) signRequestWithDigest(req *http.Request, contentType string, contentLength int64, contentSHA256Base64 string) error {
	if o.key == nil {
		key, err := cryptoutil.LoadRSAPrivateKey(o.Account.KeyFile)
		if err != nil {
			return err
		}
		o.key = key
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}

	req.Header.Set("Date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("Host", req.URL.Host)

	headers := []string{"(request-target)", "host", "date"}
	if req.Method == "POST" || req.Method == "PUT" {
		req.Header.Set("x-content-sha256", contentSHA256Base64)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Content-Length", fmt.Sprintf("%d", contentLength))
		headers = append(headers, "x-content-sha256", "content-type", "content-length")
	}

	signingString := o.signingString(req, headers)
	sum := sha256.Sum256([]byte(signingString))
	sig, err := rsa.SignPKCS1v15(rand.Reader, o.key, crypto.SHA256, sum[:])
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf(
		`Signature version="1",keyId="%s/%s/%s",algorithm="rsa-sha256",headers="%s",signature="%s"`,
		o.Account.Tenancy,
		o.Account.User,
		o.Account.Fingerprint,
		strings.Join(headers, " "),
		base64.StdEncoding.EncodeToString(sig),
	))
	return nil
}

func (o *OCIService) signingString(req *http.Request, headers []string) string {
	lines := make([]string, 0, len(headers))
	for _, header := range headers {
		switch header {
		case "(request-target)":
			target := strings.ToLower(req.Method) + " " + req.URL.EscapedPath()
			if req.URL.RawQuery != "" {
				target += "?" + normalizeQuery(req.URL.RawQuery)
			}
			lines = append(lines, "(request-target): "+target)
		case "host":
			lines = append(lines, "host: "+req.URL.Host)
		default:
			lines = append(lines, header+": "+req.Header.Get(header))
		}
	}
	return strings.Join(lines, "\n")
}

func normalizeQuery(rawQuery string) string {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return rawQuery
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0)
	for _, key := range keys {
		vals := values[key]
		sort.Strings(vals)
		for _, value := range vals {
			parts = append(parts, url.QueryEscape(key)+"="+url.QueryEscape(value))
		}
	}
	return strings.Join(parts, "&")
}

func ociStatusText(status string) string {
	switch status {
	case "RUNNING":
		return "VM running"
	case "STOPPED", "TERMINATED", "TERMINATING":
		return "VM stopped"
	default:
		if status == "" {
			return "Unknown"
		}
		return status
	}
}

func loadPrivateKeyHelper(path, pemStr string) (*rsa.PrivateKey, error) {
	if strings.TrimSpace(path) != "" {
		return cryptoutil.LoadRSAPrivateKey(path)
	}
	return cryptoutil.ParseRSAPrivateKeyPEM([]byte(pemStr), "OCI private key")
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
