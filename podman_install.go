package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const podmanVersion = "5.7.0"

// PodmanInstallInfo contains information about how to install Podman
type PodmanInstallInfo struct {
	Available   bool   // True if we can auto-install
	DownloadURL string // URL to download installer/binary
	InstallCmd  string // Command to run for installation
	Manual      string // Manual instructions if auto-install not available
}

// GetPodmanInstallInfo returns platform-specific Podman installation information
func GetPodmanInstallInfo() PodmanInstallInfo {
	base := fmt.Sprintf("https://github.com/containers/podman/releases/download/v%s", podmanVersion)

	switch runtime.GOOS {
	case "darwin":
		arch := "arm64"
		if runtime.GOARCH == "amd64" {
			arch = "amd64"
		}
		return PodmanInstallInfo{
			Available:   true,
			DownloadURL: fmt.Sprintf("%s/podman-installer-macos-%s.pkg", base, arch),
			InstallCmd:  "installer -pkg podman-installer.pkg -target /",
			Manual:      "brew install podman\n   or\n   Download from: https://podman.io/docs/installation#macos",
		}

	case "windows":
		return PodmanInstallInfo{
			Available:   true,
			DownloadURL: fmt.Sprintf("%s/podman-%s-setup.exe", base, podmanVersion),
			Manual:      "Download Podman Desktop from: https://podman-desktop.io/\n   or\n   winget install RedHat.Podman",
		}

	case "linux":
		// Linux requires system packages - no static binary available
		distro := detectLinuxDistro()
		var manual string
		switch distro {
		case "debian", "ubuntu":
			manual = "sudo apt-get update && sudo apt-get install -y podman"
		case "fedora", "rhel", "centos":
			manual = "sudo dnf install -y podman"
		case "arch":
			manual = "sudo pacman -S podman"
		case "opensuse":
			manual = "sudo zypper install podman"
		default:
			manual = "Install podman using your distribution's package manager.\n   See: https://podman.io/docs/installation"
		}
		return PodmanInstallInfo{
			Available: false,
			Manual:    manual,
		}

	default:
		return PodmanInstallInfo{
			Available: false,
			Manual:    "See: https://podman.io/docs/installation",
		}
	}
}

// detectLinuxDistro attempts to detect the Linux distribution
func detectLinuxDistro() string {
	// Check /etc/os-release
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}

	content := string(data)
	content = strings.ToLower(content)

	if strings.Contains(content, "ubuntu") {
		return "ubuntu"
	}
	if strings.Contains(content, "debian") {
		return "debian"
	}
	if strings.Contains(content, "fedora") {
		return "fedora"
	}
	if strings.Contains(content, "rhel") || strings.Contains(content, "red hat") {
		return "rhel"
	}
	if strings.Contains(content, "centos") {
		return "centos"
	}
	if strings.Contains(content, "arch") {
		return "arch"
	}
	if strings.Contains(content, "opensuse") {
		return "opensuse"
	}
	return ""
}

// EnsurePodman checks for Podman and offers to help install it if missing
// Returns the path to podman binary if found/installed, or error with instructions
func EnsurePodman(ctx context.Context, progressFn func(string)) (string, error) {
	// First check if podman is already available
	if path, err := exec.LookPath("podman"); err == nil {
		return path, nil
	}

	// Check ~/.bjarne/bin/ for podman
	home, err := os.UserHomeDir()
	if err == nil {
		localPath := filepath.Join(home, ".bjarne", "bin", "podman")
		if runtime.GOOS == "windows" {
			localPath += ".exe"
		}
		if _, err := os.Stat(localPath); err == nil {
			return localPath, nil
		}
	}

	// Podman not found - return instructions
	info := GetPodmanInstallInfo()

	if info.Available && runtime.GOOS == "windows" {
		// For Windows, we can try to download the installer
		if progressFn != nil {
			progressFn("Podman not found. Downloading installer...")
		}
		err := downloadPodmanInstaller(ctx, info.DownloadURL, progressFn)
		if err == nil {
			return "", fmt.Errorf("podman installer downloaded. Please run the installer and restart bjarne")
		}
	}

	return "", &PodmanNotFoundError{Instructions: info.Manual}
}

// PodmanNotFoundError indicates podman is not installed with instructions
type PodmanNotFoundError struct {
	Instructions string
}

func (e *PodmanNotFoundError) Error() string {
	return fmt.Sprintf("podman not found.\n\nTo install:\n   %s", e.Instructions)
}

// downloadPodmanInstaller downloads the Podman installer to a temporary location
func downloadPodmanInstaller(ctx context.Context, url string, progressFn func(string)) error {
	if progressFn != nil {
		progressFn(fmt.Sprintf("Downloading from %s...", url))
	}

	// Create temp file for download
	tmpDir, err := os.MkdirTemp("", "bjarne-podman-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}

	filename := filepath.Base(url)
	destPath := filepath.Join(tmpDir, filename)

	// Download
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// Show progress
	if progressFn != nil && resp.ContentLength > 0 {
		progressFn(fmt.Sprintf("Downloading %.1f MB...", float64(resp.ContentLength)/(1024*1024)))
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = out.Close() }()

	// Limit to 200MB to prevent excessive downloads
	_, err = io.CopyN(out, resp.Body, 200*1024*1024)
	if err != nil && err != io.EOF {
		return fmt.Errorf("download failed: %w", err)
	}

	if progressFn != nil {
		progressFn(fmt.Sprintf("Downloaded to: %s", destPath))
		progressFn("Please run the installer to complete setup.")
	}

	return nil
}
