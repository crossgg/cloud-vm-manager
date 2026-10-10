package main

import (
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
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
	// 1. Check custom or runtime binary
	runtimePath := os.Getenv("RUNTIME_BIN")
	if runtimePath == "" {
		runtimePath = "/app/runtime/cloud-vm-manager"
		if runtime.GOOS == "windows" {
			runtimePath = filepath.Join("runtime", "cloud-vm-manager.exe")
		}
	}

	if fi, err := os.Stat(runtimePath); err == nil && !fi.IsDir() {
		return runtimePath, true
	}

	// 2. Check default built-in binary
	defaultPath := os.Getenv("DEFAULT_BIN")
	if defaultPath == "" {
		defaultPath = "/app/cloud-vm-manager"
		if runtime.GOOS == "windows" {
			defaultPath = "cloud-vm-manager.exe"
		}
	}

	if fi, err := os.Stat(defaultPath); err == nil && !fi.IsDir() {
		return defaultPath, false
	}

	// 3. Fallback for local development
	localNames := []string{"cloud-vm-manager", "cloud-vm-manager.exe"}
	for _, name := range localNames {
		if fi, err := os.Stat(name); err == nil && !fi.IsDir() {
			return name, false
		}
	}

	return "", false
}

func rollbackRuntimeBinary(binPath string) {
	corruptPath := binPath + ".corrupted"
	_ = os.Remove(corruptPath)
	if err := os.Rename(binPath, corruptPath); err != nil {
		_ = os.Remove(binPath)
	}
	// Also clean up runtime public directory if needed so fallback to /app/public
	runtimePublic := filepath.Join(filepath.Dir(binPath), "public")
	_ = os.RemoveAll(runtimePublic)
	log.Printf("Rollback complete. Removed runtime binary and public assets.\n")
}
