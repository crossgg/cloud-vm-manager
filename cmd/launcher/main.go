package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func main() {
	log.SetPrefix("[Launcher] ")
	log.Println("Starting cloud-vm-manager launcher...")

	var (
		stopping   atomic.Bool
		currentCmd atomic.Pointer[exec.Cmd]
		cmdMu      sync.Mutex
		crashCount int
	)

	// Catch termination signals to forward to child process
	sigChan := make(chan os.Signal, 2)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		for sig := range sigChan {
			log.Printf("Received signal %v, shutting down child process...\n", sig)
			stopping.Store(true)

			cmdMu.Lock()
			cmd := currentCmd.Load()
			if cmd != nil && cmd.Process != nil {
				// Forward signal to child process
				if runtime.GOOS == "windows" {
					_ = cmd.Process.Kill()
				} else {
					_ = cmd.Process.Signal(sig)
				}
			}
			cmdMu.Unlock()
		}
	}()

	for {
		if stopping.Load() {
			log.Println("Launcher exiting cleanly.")
			os.Exit(0)
		}

		binPath, isRuntime := resolveBinary()
		if binPath == "" {
			log.Fatalln("FATAL: No cloud-vm-manager binary found to execute!")
		}

		log.Printf("Launching %s (isRuntime=%v)...\n", binPath, isRuntime)

		cmd := exec.Command(binPath, os.Args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		cmdMu.Lock()
		currentCmd.Store(cmd)
		cmdMu.Unlock()

		startTime := time.Now()
		startErr := cmd.Start()
		if startErr != nil {
			log.Printf("Failed to start %s: %v\n", binPath, startErr)
			if isRuntime {
				log.Println("Runtime binary failed to start. Rolling back to default binary...")
				rollbackRuntimeBinary(binPath)
			}
			time.Sleep(2 * time.Second)
			continue
		}

		waitErr := cmd.Wait()

		cmdMu.Lock()
		currentCmd.Store(nil)
		cmdMu.Unlock()

		if stopping.Load() {
			log.Println("Child process exited on stop signal. Launcher exiting.")
			os.Exit(0)
		}

		runDuration := time.Since(startTime)
		log.Printf("Child process exited after %v. Exit error: %v\n", runDuration, waitErr)

		// Crash loop detection: if it exited in less than 5 seconds with an error
		if runDuration < 5*time.Second && waitErr != nil {
			crashCount++
			log.Printf("Quick exit detected (%d/3 consecutive crashes)\n", crashCount)
			if crashCount >= 3 && isRuntime {
				log.Println("Detected 3 consecutive crashes with runtime binary. Rolling back to built-in binary...")
				rollbackRuntimeBinary(binPath)
				crashCount = 0
			}
		} else {
			crashCount = 0
		}

		log.Println("Restarting child process in 1 second...")
		time.Sleep(1 * time.Second)
	}
}

func resolveBinary() (string, bool) {
	runtimePath := os.Getenv("RUNTIME_BIN")
	if runtimePath == "" {
		runtimePath = "/app/runtime/cloud-vm-manager"
		if runtime.GOOS == "windows" {
			runtimePath = filepath.Join("runtime", "cloud-vm-manager.exe")
		}
	}

	defaultPath := os.Getenv("DEFAULT_BIN")
	if defaultPath == "" {
		defaultPath = "/app/cloud-vm-manager"
		if runtime.GOOS == "windows" {
			defaultPath = "cloud-vm-manager.exe"
		}
	}

	// Fallback for local development if defaultPath does not exist
	if _, err := os.Stat(defaultPath); err != nil {
		localNames := []string{"cloud-vm-manager", "cloud-vm-manager.exe"}
		for _, name := range localNames {
			if fi, err := os.Stat(name); err == nil && !fi.IsDir() {
				defaultPath = name
				break
			}
		}
	}

	runtimeDir := filepath.Dir(runtimePath)
	baseImageFile := filepath.Join(runtimeDir, ".base_image_id")
	currentImageID := getCurrentImageFingerprint(defaultPath)

	runtimeFi, runtimeErr := os.Stat(runtimePath)
	runtimeExists := (runtimeErr == nil && !runtimeFi.IsDir())

	if runtimeExists {
		storedBaseIDBytes, readErr := os.ReadFile(baseImageFile)
		storedBaseID := strings.TrimSpace(string(storedBaseIDBytes))

		if readErr == nil && storedBaseID != "" {
			if storedBaseID != currentImageID {
				log.Printf("Detected new container image (was: '%s', now: '%s'). Purging stale runtime hot-update to use new image built-in binary...\n", storedBaseID, currentImageID)
				purgeRuntime(runtimePath)
				_ = os.WriteFile(baseImageFile, []byte(currentImageID), 0o644)
				return defaultPath, false
			}
			// Stored base ID matches current image; run the hot-updated runtime binary
			return runtimePath, true
		}

		// .base_image_id did not exist yet (transition from older container version)
		if isBuiltinNewerOrEqual(defaultPath, runtimePath) {
			log.Printf("Built-in binary is newer than or replaces stale runtime binary. Purging old runtime...\n")
			purgeRuntime(runtimePath)
			_ = os.WriteFile(baseImageFile, []byte(currentImageID), 0o644)
			return defaultPath, false
		}

		// Otherwise, runtime is newer; record base image ID for future restarts
		_ = os.WriteFile(baseImageFile, []byte(currentImageID), 0o644)
		return runtimePath, true
	}

	// No runtime binary exists; keep .base_image_id in sync if runtime dir exists
	if currentImageID != "" {
		if _, err := os.Stat(runtimeDir); err == nil {
			_ = os.WriteFile(baseImageFile, []byte(currentImageID), 0o644)
		}
	}

	if fi, err := os.Stat(defaultPath); err == nil && !fi.IsDir() {
		return defaultPath, false
	}

	return "", false
}

func getCurrentImageFingerprint(defaultPath string) string {
	candidates := []string{
		"/app/.image_id",
		filepath.Join(filepath.Dir(defaultPath), ".image_id"),
		".image_id",
	}
	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil {
			trimmed := strings.TrimSpace(string(data))
			if trimmed != "" {
				return trimmed
			}
		}
	}

	if fi, err := os.Stat(defaultPath); err == nil {
		ver := getBinaryVersion(defaultPath)
		return fmt.Sprintf("%s-%d-%d", ver, fi.ModTime().Unix(), fi.Size())
	}

	return ""
}

func getBinaryVersion(binPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "-v")
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	return ""
}

func isBuiltinNewerOrEqual(defaultPath, runtimePath string) bool {
	builtinVer := getBinaryVersion(defaultPath)
	runtimeVer := getBinaryVersion(runtimePath)

	log.Printf("Version check - Builtin: '%s', Runtime: '%s'\n", builtinVer, runtimeVer)

	if builtinVer != "" && builtinVer != "dev" {
		if runtimeVer == "" || runtimeVer == "dev" {
			return true
		}
		cmp := compareSemVer(builtinVer, runtimeVer)
		if cmp >= 0 {
			return true // builtin >= runtime
		}
		return false // runtime > builtin
	}

	defaultFi, dErr := os.Stat(defaultPath)
	runtimeFi, rErr := os.Stat(runtimePath)
	if dErr == nil && rErr == nil {
		return !defaultFi.ModTime().Before(runtimeFi.ModTime())
	}

	return false
}

func compareSemVer(v1, v2 string) int {
	v1Clean := strings.TrimPrefix(strings.TrimSpace(v1), "v")
	v2Clean := strings.TrimPrefix(strings.TrimSpace(v2), "v")

	parts1 := strings.Split(v1Clean, ".")
	parts2 := strings.Split(v2Clean, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			_, _ = fmt.Sscanf(parts1[i], "%d", &n1)
		}
		if i < len(parts2) {
			_, _ = fmt.Sscanf(parts2[i], "%d", &n2)
		}
		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}
	return 0
}

func purgeRuntime(runtimePath string) {
	_ = os.Remove(runtimePath)
	_ = os.Remove(runtimePath + ".corrupted")
	runtimePublic := filepath.Join(filepath.Dir(runtimePath), "public")
	_ = os.RemoveAll(runtimePublic)
	log.Printf("Purged runtime binary and public assets.\n")
}

func rollbackRuntimeBinary(binPath string) {
	corruptPath := binPath + ".corrupted"
	_ = os.Remove(corruptPath)
	if err := os.Rename(binPath, corruptPath); err != nil {
		_ = os.Remove(binPath)
	}
	runtimePublic := filepath.Join(filepath.Dir(binPath), "public")
	_ = os.RemoveAll(runtimePublic)
	log.Printf("Rollback complete. Removed runtime binary and public assets.\n")
}
