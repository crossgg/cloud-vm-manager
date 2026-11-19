package server

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/crossgg/cloud-vm-manager/internal/auth"
	"github.com/gin-gonic/gin"
)

func SetupRouter(authService *auth.AuthService) *gin.Engine {
	if strings.TrimSpace(os.Getenv("GIN_MODE")) == "" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()
	trustedProxies, err := auth.ResolveTrustedProxiesFromEnv()
	if err != nil {
		fmt.Printf("warning: %v, falling back to no trusted proxies\n", err)
		trustedProxies = nil
	}
	if err := r.SetTrustedProxies(trustedProxies); err != nil {
		fmt.Printf("warning: set trusted proxies failed: %v\n", err)
	}
	if authService != nil {
		authService.SetTrustedProxies(trustedProxies)
	}
	r.Use(securityHeadersMiddleware())
	authService.Register(r)
	r.Use(authService.Middleware())

	// Overview
	r.GET("/api/overview", getOverview)

	// Config
	r.GET("/api/config/status", getConfigStatus)
	r.POST("/api/config/reload", reloadConfig)

	// Update
	r.GET("/api/update/status", getUpdateStatus)
	r.POST("/api/update/apply", applyUpdate)
	r.POST("/api/update/reset", resetUpdate)

	// Settings
	r.GET("/api/settings/auth", getAuthSettings)
	r.POST("/api/settings/auth", updateAuthSettings)
	r.POST("/api/settings/update", updateUpdateSettings)

	// Notifications & Interactive Telegram Bot
	r.GET("/api/notify/config", getNotifyConfig)
	r.PUT("/api/notify/config", saveNotifyConfigHandler)
	r.POST("/api/notify/test-channel", testNotifyChannel)
	r.POST("/api/notify/test-bot", testTelegramBotHandler)
	r.GET("/api/notify/logs", getNotifyLogs)

	// Accounts & Proxies
	r.GET("/api/accounts", listAccounts)
	r.GET("/api/accounts/:provider/:account", getAccountDetail)
	r.POST("/api/accounts", saveAccount)
	r.DELETE("/api/accounts/:provider/:account", deleteAccount)
	r.POST("/api/config/upload-key", uploadKeyFile)
	r.GET("/api/proxies", listProxies)
	r.POST("/api/proxies", createProxy)
	r.POST("/api/proxies/import", importProxies)
	r.PUT("/api/proxies/:id", updateProxy)
	r.DELETE("/api/proxies/:id", deleteProxy)
	r.POST("/api/proxies/:id/test", testProxy)
	r.PUT("/api/proxy-bindings", saveProxyBinding)
	r.DELETE("/api/proxy-bindings", deleteProxyBinding)

	// VM operations
	r.GET("/api/vms", listVMs)
	r.GET("/api/vm/:provider/:account/:name", getVM)
	r.POST("/api/vm/:provider/:account/:name/start", startVM)
	r.POST("/api/vm/:provider/:account/:name/stop", stopVM)
	r.POST("/api/vm/:provider/:account/:name/restart", restartVM)
	r.POST("/api/vm/:provider/:account/:name/change-ip", changeIP)
	r.POST("/api/vm/:provider/:account/:name/update-dns", updateDNS)
	r.GET("/api/refresh/:provider/:account/:name", refreshVM)

	// OCI edit & security
	r.GET("/api/vm/:provider/:account/:name/edit-options", getOCIInstanceEditOptions)
	r.POST("/api/vm/:provider/:account/:name/edit", updateOCIInstance)
	r.POST("/api/vm/:provider/:account/:name/terminate", terminateOCIInstance)
	r.GET("/api/vm/:provider/:account/:name/security-lists", listOCISecurityLists)
	r.POST("/api/vm/:provider/:account/:name/security-lists/:listID/rules", saveOCISecurityListRules)
	r.GET("/api/vm/:provider/:account/:name/network-security-groups", listOCINetworkSecurityGroups)
	r.POST("/api/vm/:provider/:account/:name/network-security-groups", createOCINetworkSecurityGroup)
	r.POST("/api/vm/:provider/:account/:name/network-security-groups/attach", attachOCINetworkSecurityGroup)
	r.POST("/api/vm/:provider/:account/:name/network-security-groups/:groupID/detach", detachOCINetworkSecurityGroup)
	r.POST("/api/vm/:provider/:account/:name/network-security-groups/:groupID/rules", saveOCINetworkSecurityGroupRules)

	// OCI IP management
	r.GET("/api/vm/:provider/:account/:name/ips", getOCIInstanceIPs)
	r.POST("/api/vm/:provider/:account/:name/ips/change-ipv4", changeOCIIPv4)
	r.POST("/api/vm/:provider/:account/:name/ips/attach-ipv4", attachOCIIPv4)
	r.POST("/api/vm/:provider/:account/:name/ips/delete-ipv4", deleteOCIIPv4)
	r.POST("/api/vm/:provider/:account/:name/ips/attach-reserved", attachOCIReservedIPv4)
	r.POST("/api/vm/:provider/:account/:name/ips/attach-ipv6", attachOCIIPv6)
	r.POST("/api/vm/:provider/:account/:name/ips/change-ipv6", changeOCIIPv6)
	r.POST("/api/vm/:provider/:account/:name/ips/delete-ipv6", deleteOCIIPv6)
	r.POST("/api/vm/:provider/:account/:name/ips/enable-ipv6", enableOCISubnetIPv6)

	// Multi-Cloud Firewall Management APIs
	// GCP Firewall & Workspace
	r.GET("/api/gcp/:account/firewalls", listGCPFirewalls)
	r.POST("/api/gcp/:account/firewalls", createGCPFirewall)
	r.PUT("/api/gcp/:account/firewalls/:name", updateGCPFirewall)
	r.PATCH("/api/gcp/:account/firewalls/:name/toggle", toggleGCPFirewall)
	r.DELETE("/api/gcp/:account/firewalls/:name", deleteGCPFirewall)
	r.GET("/api/gcp/:account/create-options", getGCPCreateOptions)
	r.POST("/api/gcp/:account/launch-instance", launchGCPInstance)
	r.GET("/api/gcp/:account/resources", scanGCPResources)
	r.POST("/api/gcp/:account/resources/cleanup", cleanupGCPResources)

	// OCI Account Security Lists, NSGs & Route Tables (Account-level firewall & routing)
	r.GET("/api/oci/:account/security-lists", listAccountOCISecurityLists)
	r.PUT("/api/oci/:account/security-lists/:listID/rules", saveAccountOCISecurityListRules)
	r.GET("/api/oci/:account/network-security-groups", listAccountOCINetworkSecurityGroups)
	r.PUT("/api/oci/:account/network-security-groups/:groupID", renameAccountOCINetworkSecurityGroup)
	r.DELETE("/api/oci/:account/network-security-groups/:groupID", deleteAccountOCINetworkSecurityGroup)
	r.PUT("/api/oci/:account/network-security-groups/:groupID/rules", saveAccountOCINetworkSecurityGroupRules)
	r.GET("/api/oci/:account/route-tables", listAccountOCIRouteTables)
	r.PUT("/api/oci/:account/route-tables/:routeTableID/rules", saveAccountOCIRouteTableRules)

	// OCI Create VM Tasks & Default Network
	r.GET("/api/oci/:account/create-options", getOCICreateOptions)
	r.GET("/api/oci/:account/create-images", getOCICreateImages)
	r.POST("/api/oci/:account/default-network", createOCIDefaultNetwork)
	r.POST("/api/oci/:account/launch-instance", launchOCIInstanceOnce)
	r.GET("/api/oci/create-tasks", listOCICreateTasks)
	r.POST("/api/oci/create-tasks", createOCICreateTask)
	r.PUT("/api/oci/create-tasks/:taskID", updateOCICreateTask)
	r.DELETE("/api/oci/create-tasks/:taskID", deleteOCICreateTask)
	r.POST("/api/oci/create-tasks/:taskID/start", startOCICreateTask)
	r.POST("/api/oci/create-tasks/:taskID/stop", stopOCICreateTask)

	// OCI Console: Overview, IAM, Object Storage
	r.GET("/api/oci/:account/overview", getOCIAccountOverview)
	r.GET("/api/oci/:account/iam", listOCIIAM)
	r.POST("/api/oci/:account/iam/users", createOCIIAMUser)
	r.DELETE("/api/oci/:account/iam/users/:userID", deleteOCIIAMUser)
	r.POST("/api/oci/:account/iam/users/:userID/password", resetOCIIAMUserPassword)
	r.GET("/api/oci/:account/iam/users/:userID/api-keys", listOCIIAMUserAPIKeys)
	r.POST("/api/oci/:account/iam/users/:userID/api-keys", uploadOCIIAMUserAPIKey)
	r.DELETE("/api/oci/:account/iam/users/:userID/api-keys/:fingerprint", deleteOCIIAMUserAPIKey)
	r.POST("/api/oci/:account/iam/groups", createOCIIAMGroup)
	r.DELETE("/api/oci/:account/iam/groups/:groupID", deleteOCIIAMGroup)
	r.POST("/api/oci/:account/iam/memberships", addOCIIAMMembership)
	r.DELETE("/api/oci/:account/iam/memberships/:membershipID", deleteOCIIAMMembership)
	r.GET("/api/oci/:account/iam/policies", listOCIIAMPolicies)
	r.POST("/api/oci/:account/iam/policies", createOCIIAMPolicy)
	r.PUT("/api/oci/:account/iam/policies/:policyID", updateOCIIAMPolicy)
	r.DELETE("/api/oci/:account/iam/policies/:policyID", deleteOCIIAMPolicy)

	r.GET("/api/oci/:account/os/buckets", listOCIBuckets)
	r.POST("/api/oci/:account/os/buckets", createOCIBucket)
	r.DELETE("/api/oci/:account/os/buckets/:bucket", deleteOCIBucket)
	r.GET("/api/oci/:account/os/buckets/:bucket/objects", listOCIObjects)
	r.POST("/api/oci/:account/os/buckets/:bucket/folder", createOCIFolder)
	r.POST("/api/oci/:account/os/buckets/:bucket/upload", uploadOCIObject)
	r.GET("/api/oci/:account/os/buckets/:bucket/download", downloadOCIObject)
	r.DELETE("/api/oci/:account/os/buckets/:bucket/objects", deleteOCIObject)
	r.POST("/api/oci/:account/os/buckets/:bucket/par", createOCIPAR)

	// OCI Boot Volumes & Idle Resource Cleanup
	r.GET("/api/vm/:provider/:account/:name/boot-volume", getOCIInstanceBootVolume)
	r.GET("/api/oci/:account/boot-volumes", listOCIAccountBootVolumes)
	r.PUT("/api/oci/:account/boot-volumes/:bootVolumeID", updateOCIBootVolume)
	r.DELETE("/api/oci/:account/boot-volumes/:bootVolumeID", deleteOCIBootVolume)
	r.POST("/api/oci/:account/boot-volumes/:bootVolumeID/attach", attachOCIBootVolume)
	r.POST("/api/oci/:account/boot-volume-attachments/:attachmentID/detach", detachOCIBootVolume)
	r.GET("/api/oci/:account/resources", scanOCIResources)
	r.POST("/api/oci/:account/resources/cleanup", cleanupOCIResources)

	// Azure NSG (Account-level firewall)
	r.GET("/api/azure/:account/security-groups", listAzureNSGs)
	r.POST("/api/azure/:account/security-groups/:nsg/rules", saveAzureNSGRule)
	r.DELETE("/api/azure/:account/security-groups/:nsg/rules/:rule", deleteAzureNSGRule)

	// Azure Create Instance & Default Network
	r.GET("/api/azure/:account/create-options", getAzureCreateOptions)
	r.POST("/api/azure/:account/default-network", createAzureDefaultNetwork)
	r.POST("/api/azure/:account/launch-instance", launchAzureInstance)

	// Azure Cost, Credit Monitor & Orphan Resource Cleanup
	r.GET("/api/azure/:account/cost-overview", getAzureCostOverview)
	r.GET("/api/azure/:account/resources", scanAzureResources)
	r.POST("/api/azure/:account/resources/cleanup", cleanupAzureResources)
	r.GET("/api/azure/:account/monitor/config", getAzureMonitorConfig)
	r.POST("/api/azure/:account/monitor/config", saveAzureMonitorConfig)
	r.POST("/api/azure/:account/monitor/start", startAzureMonitor)
	r.POST("/api/azure/:account/monitor/stop", stopAzureMonitor)
	r.GET("/api/azure/:account/monitor/status", getAzureMonitorStatus)

	// OCI data transfer monitoring APIs
	r.GET("/api/oci/:account/data-transfer", getOCIDataTransfer)
	r.GET("/api/oci/:account/data-transfer/config", getDataTransferConfig)
	r.POST("/api/oci/:account/data-transfer/config", saveDataTransferConfig)
	r.POST("/api/oci/:account/data-transfer/start", startDataTransferMonitor)
	r.POST("/api/oci/:account/data-transfer/stop", stopDataTransferMonitor)
	r.GET("/api/oci/:account/data-transfer/status", getDataTransferMonitorStatus)

	// DNS management APIs
	r.GET("/api/dns/cloudflare", listCloudflareAccounts)
	r.POST("/api/dns/cloudflare", saveCloudflareAccounts)
	r.GET("/api/dns/bindings", listDNSBindings)
	r.POST("/api/dns/bindings", saveDNSBindings)
	r.GET("/api/dns/raw", getDNSConfigRaw)
	r.POST("/api/dns/delete-binding", deleteDNSBinding)
	r.GET("/api/vm/:provider/:account/:name/dns", getVMDNSBindings)
	r.POST("/api/vm/:provider/:account/:name/dns", saveVMDNSBindings)

	// Static & Pages
	registerPages(r, authService)

	return r
}

func registerPages(r *gin.Engine, a *auth.AuthService) {
	r.GET("/", func(c *gin.Context) {
		auth.NoStore(c)
		if a.Enabled() {
			if _, ok := a.ValidSession(c); !ok {
				c.File(PublicAssetPath("login.html"))
				return
			}
		}
		c.File(PublicAssetPath("index.html"))
	})

	r.GET("/login", func(c *gin.Context) {
		auth.NoStore(c)
		if !a.Enabled() {
			c.Redirect(http.StatusFound, "/")
			return
		}
		if _, ok := a.ValidSession(c); ok {
			c.Redirect(http.StatusFound, "/")
			return
		}
		c.File(PublicAssetPath("login.html"))
	})

	r.GET("/public/*filepath", func(c *gin.Context) {
		auth.NoStore(c)
		raw := c.Param("filepath")
		if strings.Contains(raw, "\\") || strings.Contains(raw, "\x00") {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		requested := path.Clean("/" + strings.TrimPrefix(raw, "/"))
		if requested == "/" {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		if a.Enabled() {
			if _, ok := a.ValidSession(c); !ok && !isLoginAsset(requested) {
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
		}
		baseDir := ActivePublicDir()
		targetPath := filepath.Join(baseDir, filepath.FromSlash(strings.TrimPrefix(requested, "/")))
		absBase, errBase := filepath.Abs(baseDir)
		absTarget, errTarget := filepath.Abs(targetPath)
		if errBase != nil || errTarget != nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		rel, errRel := filepath.Rel(absBase, absTarget)
		if errRel != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.File(absTarget)
	})
}

func securityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Next()
	}
}

func isLoginAsset(requested string) bool {
	return requested == "/style.css" || requested == "/login.js"
}
