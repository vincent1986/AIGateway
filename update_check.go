package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	fallbackAppVersion = "2.0.1"
	latestReleaseAPI   = "https://api.github.com/repos/vincent1986/AIGateway/releases/latest"
)

//go:embed wails.json
var wailsConfigJSON []byte

type VersionUpgradeInfo struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	LatestName     string `json:"latestName"`
	ReleaseURL     string `json:"releaseUrl"`
	PublishedAt    string `json:"publishedAt"`
	CheckedAt      string `json:"checkedAt"`
	HasUpdate      bool   `json:"hasUpdate"`
	Error          string `json:"error,omitempty"`
}

type githubReleaseInfo struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

type wailsConfigVersion struct {
	Info struct {
		ProductVersion string `json:"productVersion"`
	} `json:"info"`
}

// CheckForUpdate returns the latest public GitHub release compared with this build.
// Network failures are reported in the payload so the UI can stay non-blocking.
func (a *App) CheckForUpdate() VersionUpgradeInfo {
	current := currentAppVersion()
	info := VersionUpgradeInfo{
		CurrentVersion: current,
		LatestVersion:  current,
		CheckedAt:      time.Now().Format(time.RFC3339),
		ReleaseURL:     "https://github.com/vincent1986/AIGateway/releases",
	}

	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequest(http.MethodGet, latestReleaseAPI, nil)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "AIGateway/"+current)

	resp, err := client.Do(req)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		info.Error = fmt.Sprintf("GitHub release check returned HTTP %d", resp.StatusCode)
		return info
	}

	var rel githubReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		info.Error = err.Error()
		return info
	}

	latest := strings.TrimSpace(rel.TagName)
	if latest == "" {
		info.Error = "latest release tag is empty"
		return info
	}
	info.LatestVersion = strings.TrimPrefix(latest, "v")
	info.LatestName = rel.Name
	info.ReleaseURL = rel.HTMLURL
	if info.ReleaseURL == "" {
		info.ReleaseURL = "https://github.com/vincent1986/AIGateway/releases/tag/" + latest
	}
	info.PublishedAt = rel.PublishedAt
	info.HasUpdate = compareVersions(info.LatestVersion, info.CurrentVersion) > 0
	return info
}

func currentAppVersion() string {
	var cfg wailsConfigVersion
	if err := json.Unmarshal(wailsConfigJSON, &cfg); err == nil {
		if v := strings.TrimSpace(cfg.Info.ProductVersion); v != "" {
			return v
		}
	}
	return fallbackAppVersion
}

func compareVersions(a, b string) int {
	ap := parseVersionParts(a)
	bp := parseVersionParts(b)
	max := len(ap)
	if len(bp) > max {
		max = len(bp)
	}
	for i := 0; i < max; i++ {
		av, bv := 0, 0
		if i < len(ap) {
			av = ap[i]
		}
		if i < len(bp) {
			bv = bp[i]
		}
		if av > bv {
			return 1
		}
		if av < bv {
			return -1
		}
	}
	return 0
}

func parseVersionParts(v string) []int {
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	if cut := strings.IndexAny(v, "-+"); cut >= 0 {
		v = v[:cut]
	}
	if v == "" {
		return []int{0}
	}
	chunks := strings.Split(v, ".")
	parts := make([]int, 0, len(chunks))
	for _, chunk := range chunks {
		n, err := strconv.Atoi(chunk)
		if err != nil {
			parts = append(parts, 0)
			continue
		}
		parts = append(parts, n)
	}
	return parts
}
