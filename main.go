package main

import (
	"fmt"

	"github.com/crossgg/cloud-vm-manager/internal/auth"
	"github.com/crossgg/cloud-vm-manager/internal/cache"
	"github.com/crossgg/cloud-vm-manager/internal/config"
	"github.com/crossgg/cloud-vm-manager/internal/server"
)

func main() {
	cfg, configPath, err := config.LoadConfigWithPath()
	if err != nil {
		fmt.Printf("load config failed: %v\n", err)
		return
	}

	authService, err := auth.NewAuthService(cfg.Auth)
	if err != nil {
		fmt.Printf("auth config failed: %v\n", err)
		return
	}

	if err := server.InitRuntime(cfg, configPath, authService); err != nil {
		fmt.Printf("runtime init failed: %v\n", err)
		return
	}

	cache.InitVMCache(configPath)
	server.InitDTMonitors()

	r := server.SetupRouter(authService)
	_ = r.Run(":3000")
}
