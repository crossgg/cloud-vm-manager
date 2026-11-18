package oci

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type OCIBucketInfo struct {
	Name             string  `json:"name"`
	Namespace        string  `json:"namespace"`
	CompartmentID    string  `json:"compartmentId"`
	PublicAccessType string  `json:"publicAccessType,omitempty"`
	ApproximateCount int64   `json:"approximateCount"`
	ApproximateSize  float64 `json:"approximateSize"`
	TimeCreated      string  `json:"timeCreated,omitempty"`
}

type OCIObjectItem struct {
	Name         string  `json:"name"`
	Size         float64 `json:"size"`
	TimeModified string  `json:"timeModified,omitempty"`
}

type OCIObjectListResult struct {
	Namespace string          `json:"namespace"`
	Bucket    string          `json:"bucket"`
	BucketInfo *OCIBucketInfo `json:"bucketInfo,omitempty"`
	Prefix    string          `json:"prefix"`
	Prefixes  []string        `json:"prefixes"`
	Objects   []OCIObjectItem `json:"objects"`
}

type OCIPARResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AccessType  string `json:"accessType"`
	AccessURI   string `json:"accessUri"`
	FullURL     string `json:"fullUrl"`
	TimeExpires string `json:"timeExpires"`
}

func escapeOCIObjectPath(objectName string) string {
	parts := strings.Split(objectName, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func (o *OCIService) GetObjectStorageNamespace() (string, error) {
	endpoint := o.objectStorageEndpoint("/n/", nil)
	var ns string
	if err := o.doRequest("GET", endpoint, nil, &ns, false); err != nil {
		return "", fmt.Errorf("获取对象存储命名空间失败: %w", err)
	}
	ns = strings.TrimSpace(ns)
	if ns == "" {
		return "", fmt.Errorf("OCI 未返回对象存储命名空间")
	}
	return ns, nil
}

func (o *OCIService) ListBuckets() (string, []OCIBucketInfo, error) {
	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return "", nil, err
	}
	ns, err := o.GetObjectStorageNamespace()
	if err != nil {
		return "", nil, err
	}

	var rawBuckets []map[string]interface{}
	endpoint := o.objectStorageEndpoint("/n/"+url.PathEscape(ns)+"/b", url.Values{
		"compartmentId": {compartmentID},
		"limit":         {"100"},
	})
	if err := o.doRequest("GET", endpoint, nil, &rawBuckets, false); err != nil {
		return ns, nil, fmt.Errorf("获取 Bucket 列表失败: %w", err)
	}

	buckets := make([]OCIBucketInfo, 0, len(rawBuckets))
	for _, b := range rawBuckets {
		name := stringValue(b["name"])
		if name == "" {
			continue
		}
		buckets = append(buckets, OCIBucketInfo{
			Name:          name,
			Namespace:     valueOrDefault(stringValue(b["namespace"]), ns),
			CompartmentID: stringValue(b["compartmentId"]),
			TimeCreated:   stringValue(b["timeCreated"]),
		})
	}
	return ns, buckets, nil
}

// GetBucket fetches metadata and approximate statistics for a single bucket on demand.
func (o *OCIService) GetBucket(bucketName string) (*OCIBucketInfo, error) {
	bucketName = strings.TrimSpace(bucketName)
	if bucketName == "" {
		return nil, fmt.Errorf("缺少 Bucket 名称")
	}
	ns, err := o.GetObjectStorageNamespace()
	if err != nil {
		return nil, err
	}
	endpoint := o.objectStorageEndpoint("/n/"+url.PathEscape(ns)+"/b/"+url.PathEscape(bucketName), url.Values{
		"fields": {"approximateCount", "approximateSize"},
	})
	var raw map[string]interface{}
	if err := o.doRequest("GET", endpoint, nil, &raw, false); err != nil {
		return nil, fmt.Errorf("获取 Bucket 详情失败: %w", err)
	}
	return &OCIBucketInfo{
		Name:             valueOrDefault(stringValue(raw["name"]), bucketName),
		Namespace:        valueOrDefault(stringValue(raw["namespace"]), ns),
		CompartmentID:    stringValue(raw["compartmentId"]),
		PublicAccessType: stringValue(raw["publicAccessType"]),
		ApproximateCount: int64(float64InterfaceValue(raw["approximateCount"])),
		ApproximateSize:  float64InterfaceValue(raw["approximateSize"]),
		TimeCreated:      stringValue(raw["timeCreated"]),
	}, nil
}

func (o *OCIService) CreateBucket(name, publicAccessType string) (*OCIBucketInfo, error) {
	compartmentID, err := o.compartmentIDOrTenancy()
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("Bucket 名称不能为空")
	}
	if strings.TrimSpace(publicAccessType) == "" {
		publicAccessType = "NoPublicAccess"
	}

	ns, err := o.GetObjectStorageNamespace()
	if err != nil {
		return nil, err
	}

	endpoint := o.objectStorageEndpoint("/n/"+url.PathEscape(ns)+"/b/", nil)
	var created map[string]interface{}
	if err := o.doRequest("POST", endpoint, map[string]interface{}{
		"name":             name,
		"compartmentId":    compartmentID,
		"publicAccessType": publicAccessType,
	}, &created, false); err != nil {
		return nil, fmt.Errorf("创建 Bucket 失败: %w", err)
	}

	return &OCIBucketInfo{
		Name:             valueOrDefault(stringValue(created["name"]), name),
		Namespace:        ns,
		CompartmentID:    compartmentID,
		PublicAccessType: valueOrDefault(stringValue(created["publicAccessType"]), publicAccessType),
		TimeCreated:      stringValue(created["timeCreated"]),
	}, nil
}

func (o *OCIService) DeleteBucket(bucketName string) error {
	bucketName = strings.TrimSpace(bucketName)
	if bucketName == "" {
		return fmt.Errorf("缺少 Bucket 名称")
	}
	ns, err := o.GetObjectStorageNamespace()
	if err != nil {
		return err
	}
	endpoint := o.objectStorageEndpoint("/n/"+url.PathEscape(ns)+"/b/"+url.PathEscape(bucketName), nil)
	if err := o.doRequest("DELETE", endpoint, nil, nil, false); err != nil {
		if strings.Contains(err.Error(), "409") || strings.Contains(strings.ToLower(err.Error()), "notempty") {
			return fmt.Errorf("Bucket 非空，请先删除内部对象文件与预认证请求后再删除 Bucket")
		}
		return err
	}
	return nil
}

func (o *OCIService) ListObjects(bucketName, prefix string) (*OCIObjectListResult, error) {
	bucketName = strings.TrimSpace(bucketName)
	if bucketName == "" {
		return nil, fmt.Errorf("缺少 Bucket 名称")
	}
	ns, err := o.GetObjectStorageNamespace()
	if err != nil {
		return nil, err
	}

	query := url.Values{
		"delimiter": {"/"},
		"fields":    {"name,size,timeModified"},
		"limit":     {"1000"},
	}
	if prefix != "" {
		query.Set("prefix", prefix)
	}
	endpoint := o.objectStorageEndpoint("/n/"+url.PathEscape(ns)+"/b/"+url.PathEscape(bucketName)+"/o", query)
	var resp struct {
		Objects  []map[string]interface{} `json:"objects"`
		Prefixes []string                 `json:"prefixes"`
	}
	if err := o.doRequest("GET", endpoint, nil, &resp, false); err != nil {
		return nil, fmt.Errorf("列出对象失败: %w", err)
	}

	objects := make([]OCIObjectItem, 0, len(resp.Objects))
	for _, obj := range resp.Objects {
		name := stringValue(obj["name"])
		if name == "" || name == prefix {
			continue
		}
		objects = append(objects, OCIObjectItem{
			Name:         name,
			Size:         float64InterfaceValue(obj["size"]),
			TimeModified: stringValue(obj["timeModified"]),
		})
	}
	if resp.Prefixes == nil {
		resp.Prefixes = []string{}
	}
	return &OCIObjectListResult{
		Namespace: ns,
		Bucket:    bucketName,
		Prefix:    prefix,
		Prefixes:  resp.Prefixes,
		Objects:   objects,
	}, nil
}

func (o *OCIService) CreateObjectFolder(bucketName, folderPath string) error {
	folderPath = strings.TrimLeft(strings.TrimSpace(folderPath), "/")
	if folderPath == "" {
		return fmt.Errorf("文件夹名称不能为空")
	}
	if !strings.HasSuffix(folderPath, "/") {
		folderPath += "/"
	}
	return o.PutObject(bucketName, folderPath, "application/x-directory", []byte{})
}

func (o *OCIService) PutObject(bucketName, objectName, contentType string, data []byte) error {
	bucketName = strings.TrimSpace(bucketName)
	objectName = strings.TrimLeft(objectName, "/")
	if bucketName == "" || objectName == "" {
		return fmt.Errorf("缺少 Bucket 或对象名称")
	}
	ns, err := o.GetObjectStorageNamespace()
	if err != nil {
		return err
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	endpoint := o.objectStorageEndpoint("/n/"+url.PathEscape(ns)+"/b/"+url.PathEscape(bucketName)+"/o/"+escapeOCIObjectPath(objectName), nil)
	_, _, _, err = o.doRawRequest(http.MethodPut, endpoint, contentType, data, false)
	return err
}

func (o *OCIService) PutObjectFile(bucketName, objectName, contentType string, file *os.File, size int64, contentSHA256Base64 string) error {
	bucketName = strings.TrimSpace(bucketName)
	objectName = strings.TrimLeft(objectName, "/")
	if bucketName == "" || objectName == "" {
		return fmt.Errorf("缺少 Bucket 或对象名称")
	}
	ns, err := o.GetObjectStorageNamespace()
	if err != nil {
		return err
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	endpoint := o.objectStorageEndpoint("/n/"+url.PathEscape(ns)+"/b/"+url.PathEscape(bucketName)+"/o/"+escapeOCIObjectPath(objectName), nil)
	return o.doRawFileRequest(http.MethodPut, endpoint, contentType, file, size, contentSHA256Base64)
}

func (o *OCIService) GetObject(bucketName, objectName string) (string, []byte, error) {
	bucketName = strings.TrimSpace(bucketName)
	objectName = strings.TrimLeft(objectName, "/")
	if bucketName == "" || objectName == "" {
		return "", nil, fmt.Errorf("缺少 Bucket 或对象名称")
	}
	ns, err := o.GetObjectStorageNamespace()
	if err != nil {
		return "", nil, err
	}
	endpoint := o.objectStorageEndpoint("/n/"+url.PathEscape(ns)+"/b/"+url.PathEscape(bucketName)+"/o/"+escapeOCIObjectPath(objectName), nil)
	_, headers, data, err := o.doRawRequest(http.MethodGet, endpoint, "", nil, false)
	if err != nil {
		return "", nil, err
	}
	ct := headers.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	return ct, data, nil
}

func (o *OCIService) DeleteObject(bucketName, objectName string) error {
	bucketName = strings.TrimSpace(bucketName)
	objectName = strings.TrimLeft(objectName, "/")
	if bucketName == "" || objectName == "" {
		return fmt.Errorf("缺少 Bucket 或对象名称")
	}
	ns, err := o.GetObjectStorageNamespace()
	if err != nil {
		return err
	}
	endpoint := o.objectStorageEndpoint("/n/"+url.PathEscape(ns)+"/b/"+url.PathEscape(bucketName)+"/o/"+escapeOCIObjectPath(objectName), nil)
	return o.doRequest(http.MethodDelete, endpoint, nil, nil, false)
}

func (o *OCIService) CreatePreauthenticatedRequest(bucketName, objectName, accessType, name string, expiresHours int) (*OCIPARResult, error) {
	bucketName = strings.TrimSpace(bucketName)
	if bucketName == "" {
		return nil, fmt.Errorf("缺少 Bucket 名称")
	}
	ns, err := o.GetObjectStorageNamespace()
	if err != nil {
		return nil, err
	}
	if expiresHours <= 0 {
		expiresHours = 24
	}
	if strings.TrimSpace(accessType) == "" {
		if strings.TrimSpace(objectName) != "" {
			accessType = "ObjectRead"
		} else {
			accessType = "AnyObjectRead"
		}
	}
	if strings.TrimSpace(name) == "" {
		name = "par-" + time.Now().Format("20060102150405")
	}

	expires := time.Now().UTC().Add(time.Duration(expiresHours) * time.Hour).Format(time.RFC3339)
	body := map[string]interface{}{
		"name":        name,
		"accessType":  accessType,
		"timeExpires": expires,
	}
	if strings.TrimSpace(objectName) != "" {
		body["objectName"] = strings.TrimLeft(objectName, "/")
	} else if accessType == "AnyObjectRead" || accessType == "AnyObjectReadWrite" {
		body["bucketListingAction"] = "ListObjects"
	}

	endpoint := o.objectStorageEndpoint("/n/"+url.PathEscape(ns)+"/b/"+url.PathEscape(bucketName)+"/p/", nil)
	var created map[string]interface{}
	if err := o.doRequest(http.MethodPost, endpoint, body, &created, false); err != nil {
		return nil, fmt.Errorf("创建预认证请求失败: %w", err)
	}
	accessURI := stringValue(created["accessUri"])
	fullURL := "https://objectstorage." + o.Account.Region + ".oraclecloud.com" + accessURI

	return &OCIPARResult{
		ID:          stringValue(created["id"]),
		Name:        valueOrDefault(stringValue(created["name"]), name),
		AccessType:  valueOrDefault(stringValue(created["accessType"]), accessType),
		AccessURI:   accessURI,
		FullURL:     fullURL,
		TimeExpires: valueOrDefault(stringValue(created["timeExpires"]), expires),
	}, nil
}
