package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

const (
	defaultCurrentVersion = "dev"
	githubRepo            = "baize"
	githubLatestTagsAPI   = "https://api.github.com/repos/xuzhougeng/baize/tags?per_page=1"
	githubReleasesURL     = "https://github.com/xuzhougeng/baize/releases"
)

var (
	appVersion string

	currentVersionOnce sync.Once
	currentVersionText string
)

type VersionInfo struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	ReleaseURL     string `json:"releaseUrl,omitempty"`
	Message        string `json:"message"`
}

func (a *DesktopApp) GetVersionInfo() (VersionInfo, error) {
	info := VersionInfo{
		CurrentVersion: currentAppVersion(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	latestVersion, err := latestAppVersion(ctx)
	if err != nil || latestVersion == "" {
		info.Message = fmt.Sprintf("当前版本 %s，暂时无法自动获取最新版本。", info.CurrentVersion)
		return info, nil
	}

	info.LatestVersion = latestVersion
	info.ReleaseURL = githubReleasesURL
	info.HasUpdate = hasVersionUpdate(info.CurrentVersion, latestVersion)
	if info.HasUpdate {
		info.Message = fmt.Sprintf("当前版本 %s，最新版本 %s。", info.CurrentVersion, latestVersion)
		return info, nil
	}

	info.Message = fmt.Sprintf("当前已是最新版本 %s。", latestVersion)
	return info, nil
}

func currentAppVersion() string {
	currentVersionOnce.Do(func() {
		currentVersionText = detectCurrentAppVersion()
	})
	return currentVersionText
}

func detectCurrentAppVersion() string {
	if version := strings.TrimSpace(appVersion); version != "" {
		return version
	}

	if info, ok := debug.ReadBuildInfo(); ok {
		if version := strings.TrimSpace(info.Main.Version); version != "" && version != "(devel)" {
			return version
		}
	}

	if version := gitDescribeVersion(context.Background()); version != "" {
		return version
	}

	return defaultCurrentVersion
}

func latestAppVersion(ctx context.Context) (string, error) {
	latestVersion, err := fetchLatestGitHubTag(ctx)
	if err == nil && latestVersion != "" {
		return latestVersion, nil
	}

	if fallback := latestLocalGitTag(ctx); fallback != "" {
		return fallback, nil
	}

	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("unable to resolve latest version")
}

func fetchLatestGitHubTag(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, githubLatestTagsAPI, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", fmt.Sprintf("%s/%s", githubRepo, currentAppVersion()))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github tags api returned %d", resp.StatusCode)
	}

	var payload []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if len(payload) == 0 || strings.TrimSpace(payload[0].Name) == "" {
		return "", fmt.Errorf("github tags api returned no tags")
	}
	return strings.TrimSpace(payload[0].Name), nil
}

func gitDescribeVersion(ctx context.Context) string {
	output, err := exec.CommandContext(ctx, "git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func latestLocalGitTag(ctx context.Context) string {
	output, err := exec.CommandContext(ctx, "git", "tag", "--sort=-version:refname").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(output), "\n") {
		if version := strings.TrimSpace(line); version != "" {
			return version
		}
	}
	return ""
}

func hasVersionUpdate(currentVersion, latestVersion string) bool {
	currentBase := normalizedVersionBase(currentVersion)
	latestBase := normalizedVersionBase(latestVersion)
	if latestBase == "" {
		return false
	}
	if currentBase == "" {
		return true
	}
	return !strings.EqualFold(currentBase, latestBase)
}

func normalizedVersionBase(version string) string {
	value := strings.TrimSpace(version)
	if value == "" {
		return ""
	}
	if cut := strings.IndexByte(value, '-'); cut >= 0 {
		value = value[:cut]
	}
	value = strings.TrimSpace(value)
	if len(value) > 1 && (value[0] == 'v' || value[0] == 'V') {
		value = strings.TrimSpace(value[1:])
	}
	return value
}
