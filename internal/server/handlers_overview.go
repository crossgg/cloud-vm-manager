package server

import (
	"net/http"

	"github.com/crossgg/cloud-vm-manager/internal/cache"
	"github.com/gin-gonic/gin"
)

func getOverview(c *gin.Context) {
	cStore := cache.Global()
	if cStore == nil {
		c.JSON(http.StatusOK, gin.H{
			"configCount":        len(cloudAccountsSnapshot()),
			"instanceCount":      0,
			"runningCount":       0,
			"stoppedCount":       0,
			"cachedAccountCount": 0,
		})
		return
	}
	c.JSON(http.StatusOK, cStore.GetOverview(cloudAccountsSnapshot()))
}
