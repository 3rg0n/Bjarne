package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	onnxVersion = "1.16.3"
)

// getONNXDownloadURL returns the download URL for ONNX runtime
func getONNXDownloadURL() (string, string) {
	base := "https://github.com/microsoft/onnxruntime/releases/download/v" + onnxVersion

	switch runtime.GOOS {
	case "windows":
		return base + "/onnxruntime-win-x64-" + onnxVersion + ".zip", "zip"
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return base + "/onnxruntime-osx-arm64-" + onnxVersion + ".tgz", "tgz"
		}
		return base + "/onnxruntime-osx-x86_64-" + onnxVersion + ".tgz", "tgz"
	default: // linux
		if runtime.GOARCH == "arm64" {
			return base + "/onnxruntime-linux-aarch64-" + onnxVersion + ".tgz", "tgz"
		}
		return base + "/onnxruntime-linux-x64-" + onnxVersion + ".tgz", "tgz"
	}
}

// getONNXLibDir returns the directory where ONNX runtime should be installed
func getONNXLibDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".bjarne", "lib"), nil
}

// EnsureONNXRuntime downloads and installs ONNX runtime if not present
// progressFn is called with status messages during download
func EnsureONNXRuntime(progressFn func(string)) error {
	// Check if already available
	if isONNXAvailable() {
		return nil
	}

	libDir, err := getONNXLibDir()
	if err != nil {
		return fmt.Errorf("cannot determine lib directory: %w", err)
	}

	// Check if already downloaded
	libName := getExpectedLibName()
	libPath := filepath.Join(libDir, libName)
	if _, err := os.Stat(libPath); err == nil {
		return nil // Already exists
	}

	// Download
	url, archiveType := getONNXDownloadURL()
	if progressFn != nil {
		progressFn(fmt.Sprintf("Downloading ONNX Runtime v%s...", onnxVersion))
	}

	// Create temp file for download
	tmpFile, err := os.CreateTemp("", "onnxruntime-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	// Download
	resp, err := http.Get(url) //nolint:gosec // URL is hardcoded
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_ = tmpFile.Close()
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	// Show download progress
	size := resp.ContentLength
	if progressFn != nil && size > 0 {
		progressFn(fmt.Sprintf("Downloading %.1f MB...", float64(size)/(1024*1024)))
	}

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	_ = tmpFile.Close()

	// Create lib directory
	if err := os.MkdirAll(libDir, 0750); err != nil {
		return fmt.Errorf("failed to create lib directory: %w", err)
	}

	// Extract
	if progressFn != nil {
		progressFn("Extracting...")
	}

	if archiveType == "zip" {
		err = extractZip(tmpPath, libDir)
	} else {
		err = extractTarGz(tmpPath, libDir)
	}
	if err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	if progressFn != nil {
		progressFn("ONNX Runtime installed successfully")
	}

	return nil
}

// getExpectedLibName returns the expected library filename for the current platform
func getExpectedLibName() string {
	switch runtime.GOOS {
	case "windows":
		return "onnxruntime.dll"
	case "darwin":
		return "libonnxruntime.dylib"
	default:
		return "libonnxruntime.so"
	}
}

// extractZip extracts relevant files from a zip archive
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	for _, f := range r.File {
		// Only extract library files from lib/ directory
		if !strings.Contains(f.Name, "/lib/") {
			continue
		}

		// Get just the filename
		name := filepath.Base(f.Name)
		if name == "" || name == "." || name == ".." {
			continue
		}

		// Skip non-library files
		if !isLibraryFile(name) {
			continue
		}

		destPath := filepath.Join(destDir, name)

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			_ = rc.Close()
			return err
		}

		// Limit copy to 100MB to prevent decompression bombs
		_, err = io.CopyN(outFile, rc, 100*1024*1024)
		if err != nil && err != io.EOF {
			_ = rc.Close()
			_ = outFile.Close()
			return err
		}
		_ = rc.Close()
		_ = outFile.Close()
	}

	return nil
}

// extractTarGz extracts relevant files from a tar.gz archive
func extractTarGz(tgzPath, destDir string) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Only extract library files from lib/ directory
		if !strings.Contains(header.Name, "/lib/") {
			continue
		}

		// Get just the filename
		name := filepath.Base(header.Name)
		if name == "" || name == "." || name == ".." {
			continue
		}

		// Skip directories and non-library files
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if !isLibraryFile(name) {
			continue
		}

		destPath := filepath.Join(destDir, name)

		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
		if err != nil {
			return err
		}

		// Limit copy to 100MB to prevent decompression bombs
		_, err = io.CopyN(outFile, tr, 100*1024*1024)
		_ = outFile.Close()

		if err != nil && err != io.EOF {
			return err
		}
	}

	return nil
}

// isLibraryFile checks if a filename is a library file we want to extract
func isLibraryFile(name string) bool {
	// Windows
	if strings.HasSuffix(name, ".dll") {
		return true
	}
	// macOS
	if strings.HasSuffix(name, ".dylib") {
		return true
	}
	// Linux - libonnxruntime.so or libonnxruntime.so.1.16.3
	if strings.HasPrefix(name, "libonnxruntime") && strings.Contains(name, ".so") {
		return true
	}
	return false
}
