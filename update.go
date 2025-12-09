package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const (
	githubReleaseURL = "https://api.github.com/repos/3rg0n/bjarne/releases/latest"
	updateCheckURL   = "https://github.com/3rg0n/bjarne/releases/latest"
)

// GitHubRelease represents the GitHub API release response
type GitHubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// CheckForUpdate checks if a newer version is available
// Returns (latestVersion, updateAvailable, error)
func CheckForUpdate(ctx context.Context) (string, bool, error) {
	// Skip check for dev builds
	if Version == "dev" || Version == "" {
		return "", false, nil
	}

	// Create HTTP client with timeout
	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubReleaseURL, nil)
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "bjarne/"+Version)

	resp, err := client.Do(req)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", false, err
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(Version, "v")

	// Compare versions
	if compareVersions(latestVersion, currentVersion) > 0 {
		return release.TagName, true, nil
	}

	return release.TagName, false, nil
}

// compareVersions compares two semantic versions
// Returns: 1 if a > b, -1 if a < b, 0 if equal
func compareVersions(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	// Pad shorter version with zeros
	for len(aParts) < 3 {
		aParts = append(aParts, "0")
	}
	for len(bParts) < 3 {
		bParts = append(bParts, "0")
	}

	for i := 0; i < 3; i++ {
		aNum := parseVersionPart(aParts[i])
		bNum := parseVersionPart(bParts[i])

		if aNum > bNum {
			return 1
		}
		if aNum < bNum {
			return -1
		}
	}

	return 0
}

// parseVersionPart extracts the numeric part of a version component
func parseVersionPart(s string) int {
	// Handle cases like "1-beta" or "2rc1"
	num := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
		} else {
			break
		}
	}
	return num
}

// GetUpdateCommand returns the appropriate update command for the current platform
func GetUpdateCommand() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew upgrade bjarne"
	case "windows":
		// Check if installed via scoop (scoop apps are in ~/scoop/apps/)
		return "scoop update bjarne\n           or: irm https://raw.githubusercontent.com/3rg0n/bjarne/master/install.ps1 | iex"
	default: // linux and others
		return "curl -sSL https://raw.githubusercontent.com/3rg0n/bjarne/master/install.sh | bash"
	}
}

// PrintUpdateNotice prints an update notification if a newer version is available
func PrintUpdateNotice() {
	// Run check with short timeout (non-blocking to startup)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	latestVersion, updateAvailable, err := CheckForUpdate(ctx)
	if err != nil {
		// Silently ignore errors - don't block startup for update check
		return
	}

	if updateAvailable {
		fmt.Printf("\n    \033[93mUpdate available:\033[0m %s -> %s\n", Version, latestVersion)
		fmt.Printf("    Run: \033[96m%s\033[0m\n\n", GetUpdateCommand())
	}
}
