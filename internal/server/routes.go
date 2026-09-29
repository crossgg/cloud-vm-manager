package server

import (
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/crossgg/cloud-vm-manager/internal/auth"
	"github.com/gin-gonic/gin"
)

func SetupRouter(authService *auth.AuthService) *gin.Engine {
	r := gin.Default()
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

	// Settings
	r.GET("/api/settings/auth", getAuthSettings)
	r.POST("/api/settings/auth", updateAuthSettings)
	r.POST("/api/settings/update", updateUpdateSettings)

	// Accounts & Proxies
	r.GET("/api/accounts", listAccounts)
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
	r.GET("/api/vm/:provider/:account/:name/security-lists", listOCISecurityLists)
	r.POST("/api/vm/:provider/:account/:name/security-lists/:listID/rules", saveOCISecurityListRules)
	r.GET("/api/vm/:provider/:account/:name/network-security-groups", listOCINetworkSecurityGroups)
	r.POST("/api/vm/:provider/:account/:name/network-security-groups", createOCINetworkSecurityGroup)
	r.POST("/api/vm/:provider/:account/:name/network-security-groups/:groupID/rules", saveOCINetworkSecurityGroupRules)

	// Multi-Cloud Firewall Management APIs
	// GCP Firewall
	r.GET("/api/gcp/:account/firewalls", listGCPFirewalls)
	r.POST("/api/gcp/:account/firewalls", createGCPFirewall)
	r.PUT("/api/gcp/:account/firewalls/:name", updateGCPFirewall)
	r.PATCH("/api/gcp/:account/firewalls/:name/toggle", toggleGCPFirewall)
	r.DELETE("/api/gcp/:account/firewalls/:name", deleteGCPFirewall)

	// OCI Account Security Lists (Account-level firewall)
	r.GET("/api/oci/:account/security-lists", listAccountOCISecurityLists)
	r.PUT("/api/oci/:account/security-lists/:listID/rules", saveAccountOCISecurityListRules)

	// Azure NSG (Account-level firewall)
	r.GET("/api/azure/:account/security-groups", listAzureNSGs)
	r.POST("/api/azure/:account/security-groups/:nsg/rules", saveAzureNSGRule)
	r.DELETE("/api/azure/:account/security-groups/:nsg/rules/:rule", deleteAzureNSGRule)

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
		requested := path.Clean("/" + strings.TrimPrefix(c.Param("filepath"), "/"))
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
		c.File(filepath.Join(ActivePublicDir(), strings.TrimPrefix(requested, "/")))
	})
}

func isLoginAsset(requested string) bool {
	return requested == "/style.css" || requested == "/login.js"
}
