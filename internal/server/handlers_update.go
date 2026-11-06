package server

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/crossgg/cloud-vm-manager/internal/updater"
	"github.com/gin-gonic/gin"
)

func getUpdateStatus(c *gin.Context) {
	info := updater.UpdateInfo{
		CurrentVersion: updater.Version,
		RuntimePath:    updater.RuntimeBinPath,
		DownloadProxy:  defaultDownloadProxy(),
	}

	if exe, err := os.Executable(); err == nil {
		info.IsRuntime = strings.Contains(filepath.ToSlash(exe), "/runtime/")
	} else if fi, err := os.Stat(updater.RuntimeBinPath); err == nil && !fi.IsDir() {
		info.IsRuntime = true
	}

	if c.Query("check") == "true" {
		release, asset, err := updater.LatestReleaseAsset(c.Query("download_proxy"))
		if err != nil {
			info.CheckError = err.Error()
		} else {
			info.LatestVersion = release.TagName
			info.UpdateAvailable = updater.Version == "dev" || release.TagName != updater.Version
			info.AssetName = asset.Name
			info.ReleaseURL = release.HTMLURL
		}
	}

	c.JSON(http.StatusOK, info)
}

func applyUpdate(c *gin.Context) {
	var payload struct {
		DownloadProxy string `json:"downloadProxy"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	release, asset, err := updater.LatestReleaseAsset(payload.DownloadProxy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := updater.InstallReleaseAsset(release, asset, payload.DownloadProxy, RuntimePublicDir); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "update installed; restarting process",
		"latestVersion": release.TagName,
		"assetName":     asset.Name,
	})

	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
}

func defaultDownloadProxy() string {
	runtimeState.mu.RLock()
	cfg := runtimeState.cfg
	runtimeState.mu.RUnlock()
	if cfg != nil && cfg.Update.DownloadProxy != "" {
		return cfg.Update.DownloadProxy
	}
	return strings.TrimSpace(os.Getenv("UPDATE_DOWNLOAD_PROXY"))
}

func resetUpdate(c *gin.Context) {
	runtimeBin := updater.RuntimeBinPath
	_ = os.Remove(runtimeBin)
	_ = os.Remove(runtimeBin + ".corrupted")
	_ = os.RemoveAll(RuntimePublicDir)
	_ = os.Remove(filepath.Join(filepath.Dir(runtimeBin), ".base_image_id"))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "已清除 runtime 运行缓存，正在平滑重启并切换回镜像内置版本...",
	})

	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
}

