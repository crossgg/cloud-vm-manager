package server

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func terminateOCIInstance(c *gin.Context) {
	instanceID := c.Param("name")
	ociSvc, ok := getOCIService(c)
	if !ok {
		return
	}

	var payload struct {
		PreserveBootVolume bool `json:"preserveBootVolume"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := ociSvc.TerminateInstance(instanceID, payload.PreserveBootVolume); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	msg := "已提交删除实例请求"
	if payload.PreserveBootVolume {
		msg = "已提交删除实例请求（保留引导卷）"
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": msg,
	})
}

func getOCIAccountOverview(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	overview, err := ociSvc.GetOverview()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, overview)
}

// --- IAM Handlers ---

func listOCIIAM(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	data, err := ociSvc.ListIAMData()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func createOCIIAMUser(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	var payload struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Email       string `json:"email"`
		GroupID     string `json:"groupId"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := ociSvc.CreateIAMUser(payload.Name, payload.Description, payload.Email, payload.GroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "user": user})
}

func deleteOCIIAMUser(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	if err := ociSvc.DeleteIAMUser(c.Param("userID")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func resetOCIIAMUserPassword(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	pw, err := ociSvc.ResetIAMUserUIPassword(c.Param("userID"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "password": pw})
}

func listOCIIAMUserAPIKeys(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	keys, err := ociSvc.ListUserAPIKeys(c.Param("userID"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"apiKeys": keys})
}

func uploadOCIIAMUserAPIKey(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	var payload struct {
		Key          string `json:"key"`
		AutoGenerate bool   `json:"autoGenerate"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := ociSvc.UploadUserAPIKey(c.Param("userID"), payload.Key, payload.AutoGenerate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"apiKey":        res.APIKey,
		"privateKeyPem": res.PrivateKeyPEM,
	})
}

func deleteOCIIAMUserAPIKey(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	if err := ociSvc.DeleteUserAPIKey(c.Param("userID"), c.Param("fingerprint")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func createOCIIAMGroup(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	var payload struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	grp, err := ociSvc.CreateIAMGroup(payload.Name, payload.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "group": grp})
}

func deleteOCIIAMGroup(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	if err := ociSvc.DeleteIAMGroup(c.Param("groupID")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func addOCIIAMMembership(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	var payload struct {
		UserID  string `json:"userId"`
		GroupID string `json:"groupId"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ociSvc.AddUserToGroup(payload.UserID, payload.GroupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func deleteOCIIAMMembership(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	if err := ociSvc.RemoveUserFromGroup(c.Param("membershipID")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func listOCIIAMPolicies(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	policies, err := ociSvc.ListIAMPolicies(c.Query("compartmentId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"policies": policies})
}

func createOCIIAMPolicy(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	var payload struct {
		CompartmentID string   `json:"compartmentId"`
		Name          string   `json:"name"`
		Description   string   `json:"description"`
		Statements    []string `json:"statements"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	policy, err := ociSvc.CreateIAMPolicy(payload.CompartmentID, payload.Name, payload.Description, payload.Statements)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "policy": policy})
}

func updateOCIIAMPolicy(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	var payload struct {
		Description string   `json:"description"`
		Statements  []string `json:"statements"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	policy, err := ociSvc.UpdateIAMPolicy(c.Param("policyID"), payload.Description, payload.Statements)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "policy": policy})
}

func deleteOCIIAMPolicy(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	if err := ociSvc.DeleteIAMPolicy(c.Param("policyID")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// --- Object Storage Handlers ---

func listOCIBuckets(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	ns, buckets, err := ociSvc.ListBuckets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"namespace": ns, "buckets": buckets})
}

func createOCIBucket(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	var payload struct {
		Name             string `json:"name"`
		PublicAccessType string `json:"publicAccessType"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	bucket, err := ociSvc.CreateBucket(payload.Name, payload.PublicAccessType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "bucket": bucket})
}

func deleteOCIBucket(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	if err := ociSvc.DeleteBucket(c.Param("bucket")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func listOCIObjects(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	bucketName := c.Param("bucket")
	res, err := ociSvc.ListObjects(bucketName, c.Query("prefix"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if c.Query("includeBucket") == "1" {
		if info, err := ociSvc.GetBucket(bucketName); err == nil {
			res.BucketInfo = info
		}
	}
	c.JSON(http.StatusOK, res)
}

func createOCIFolder(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	var payload struct {
		Folder string `json:"folder"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := ociSvc.CreateObjectFolder(c.Param("bucket"), payload.Folder); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func uploadOCIObject(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请选择上传文件"})
		return
	}
	f, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, 128<<20))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	prefix := strings.TrimLeft(strings.TrimSpace(c.PostForm("prefix")), "/")
	objectName := strings.TrimSpace(c.PostForm("objectName"))
	if objectName == "" {
		objectName = filepath.Base(fileHeader.Filename)
	}
	if prefix != "" {
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		objectName = prefix + strings.TrimLeft(objectName, "/")
	}

	contentType := fileHeader.Header.Get("Content-Type")
	if err := ociSvc.PutObject(c.Param("bucket"), objectName, contentType, data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "objectName": objectName})
}

func downloadOCIObject(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	objectName := c.Query("object")
	if strings.TrimSpace(objectName) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少对象名称"})
		return
	}
	contentType, data, err := ociSvc.GetObject(c.Param("bucket"), objectName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	filename := filepath.Base(objectName)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, strings.ReplaceAll(filename, `"`, "")))
	c.Data(http.StatusOK, contentType, data)
}

func deleteOCIObject(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	objectName := c.Query("object")
	if err := ociSvc.DeleteObject(c.Param("bucket"), objectName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func createOCIPAR(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	var payload struct {
		ObjectName   string `json:"objectName"`
		AccessType   string `json:"accessType"`
		Name         string `json:"name"`
		ExpiresHours int    `json:"expiresHours"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	par, err := ociSvc.CreatePreauthenticatedRequest(c.Param("bucket"), payload.ObjectName, payload.AccessType, payload.Name, payload.ExpiresHours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "par": par})
}

func getOCIInstanceBootVolume(c *gin.Context) {
	if strings.ToLower(c.Param("provider")) != "oci" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only OCI accounts support boot volume management"})
		return
	}
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	res, err := ociSvc.GetInstanceBootVolume(c.Param("name"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func listOCIAccountBootVolumes(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	res, err := ociSvc.ListAccountBootVolumes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func updateOCIBootVolume(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	var payload struct {
		SizeInGBs *int64 `json:"sizeInGBs"`
		VpusPerGB *int64 `json:"vpusPerGB"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := ociSvc.UpdateBootVolume(c.Param("bootVolumeID"), payload.SizeInGBs, payload.VpusPerGB)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "bootVolume": updated})
}

func detachOCIBootVolume(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	if err := ociSvc.DetachBootVolume(c.Param("attachmentID")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func attachOCIBootVolume(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	var payload struct {
		InstanceID string `json:"instanceId"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	att, err := ociSvc.AttachBootVolume(c.Param("bootVolumeID"), payload.InstanceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "attachment": att})
}

func deleteOCIBootVolume(c *gin.Context) {
	ociSvc, ok := getOCIAccountService(c)
	if !ok {
		return
	}
	if err := ociSvc.DeleteBootVolume(c.Param("bootVolumeID")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
