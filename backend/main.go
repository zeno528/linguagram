// linguagram: scan a local project directory and return programming language
// statistics — same algorithm GitHub uses on every repo page (Linguist / go-enry).
package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	enry "github.com/go-enry/go-enry/v2"
	gitignore "github.com/sabhiram/go-gitignore"
)

// LanguageStat is the per-language payload returned to the frontend.
type LanguageStat struct {
	Name       string  `json:"name"`
	Percentage float64 `json:"percentage"`        // 0-100
	Color      string  `json:"color"`             // hex, e.g. "#00ADD8"
	Version    string  `json:"version,omitempty"` // declared runtime / toolchain version
	Bytes      int64   `json:"bytes"`
}

// FileEntry is one file uploaded by the frontend after a drag-and-drop.
type FileEntry struct {
	Path    string `json:"path"` // path relative to the dropped folder root
	Size    int64  `json:"size"`
	Content string `json:"content"` // base64 of first 16 KB
}

// ScanFilesRequest is the drag-and-drop payload. Gitignore carries the root
// .gitignore content (read by the frontend) so the upload path honors the same
// ignore rules as the disk path — keeping both entry points aligned with GitHub.
type ScanFilesRequest struct {
	ProjectName string      `json:"projectName"`
	Gitignore   string      `json:"gitignore"`
	Files       []FileEntry `json:"files"`
}

// ScanResponse is the JSON returned to the frontend.
type ScanResponse struct {
	Languages   []LanguageStat `json:"languages"`
	TotalBytes  int64          `json:"totalBytes"`
	ProjectName string         `json:"projectName"`
	GitHubURL   string         `json:"githubUrl,omitempty"`
}

// PublicRepo is the compact, safe subset of GitHub repository metadata shown
// in the profile picker. It deliberately excludes API URLs and owner details.
type PublicRepo struct {
	Name        string `json:"name"`
	FullName    string `json:"fullName"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Language    string `json:"language,omitempty"`
	Stars       int    `json:"stars"`
	UpdatedAt   string `json:"updatedAt"`
}

// Limit how much of each file we read for content-based language detection.
const readLimitBytes = 16 * 1024

// dist holds the production-built frontend (Vite output), embedded at compile
// time so the whole app ships as a single binary. build.ps1 copies
// frontend/dist → backend/dist before `go build`. In dev the frontend runs on
// :5173 and proxies /api here, so this is never touched.
//
//go:embed all:dist
var distFS embed.FS

func main() {
	r := gin.Default()

	// Permissive CORS for local dev (Vite on :5173 calling Gin on :8080).
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:5174", "http://127.0.0.1:5173", "http://127.0.0.1:5174"},
		AllowMethods:     []string{"GET", "POST", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type"},
		AllowCredentials: false,
	}))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	// Drop mode is the only entry now. The deprecated /api/scan (server-side
	// filepath.Walk on the local FS) and the LANG_DISABLE_PATH guard were
	// removed with the input field — there's no UI trigger for arbitrary
	// filesystem reads anymore, and the binary no longer needs that surface.
	r.POST("/api/scan-files", handleScanFiles)
	r.POST("/api/scan-github", handleScanGitHub)
	r.POST("/api/github-profile-repos", handleGitHubProfileRepos)

	// Serve the embedded frontend. Registered routes (/api, /health) match
	// first; everything else falls through to NoRoute → static asset, or
	// index.html for unknown paths (SPA fallback).
	serveDist(r)

	// Bind to localhost only: this is a self-hosted, single-user tool. If you
	// ever need remote access, put a reverse proxy (Caddy/Nginx) in front
	// instead of opening the port.
	// LANG_PORT lets the deployment pick a non-conflicting port if the
	// default 8080 is already in use on the host.
	port := os.Getenv("LANG_PORT")
	if port == "" {
		port = "8080"
	}
	addr := "127.0.0.1:" + port
	log.Printf("linguagram backend listening on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}

// serveDist serves the embedded frontend via NoRoute. Gin matches its
// registered routes (/api/*, /health) before NoRoute, so the API stays
// untouched; every other path resolves to a static asset or, failing that,
// index.html so a client-side route never hard-404s.
func serveDist(r *gin.Engine) {
	r.NoRoute(func(c *gin.Context) {
		// API paths that didn't match a registered route must 404 as JSON —
		// never fall through to the SPA shell, which would return 200 + HTML
		// and confuse the client.
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		reqPath := strings.TrimPrefix(path.Clean("/"+c.Request.URL.Path), "/")
		if reqPath == "" || reqPath == "." {
			reqPath = "index.html"
		}
		if data, err := distFS.ReadFile("dist/" + reqPath); err == nil {
			ct := mime.TypeByExtension(filepath.Ext(reqPath))
			if ct == "" {
				ct = "application/octet-stream"
			}
			c.Data(http.StatusOK, ct, data)
			return
		}
		// SPA fallback: unknown paths serve the app shell.
		if index, err := distFS.ReadFile("dist/index.html"); err == nil {
			c.Data(http.StatusOK, "text/html; charset=utf-8", index)
			return
		}
		c.Status(http.StatusNotFound)
	})
}

// handleScanFiles analyzes a manifest of files dragged into the browser.
// It honors the project's .gitignore (uploaded by the frontend) and runs the
// go-enry detection, so results match GitHub's git-tracked-file scope.
func handleScanFiles(c *gin.Context) {
	var req ScanFilesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}

	var gi *gitignore.GitIgnore
	if strings.TrimSpace(req.Gitignore) != "" {
		gi = gitignore.CompileIgnoreLines(strings.Split(req.Gitignore, "\n")...)
	}

	bytesByLang := make(map[string]int64)
	versionsByLang := make(map[string]string)
	var totalBytes int64
	var mu sync.Mutex
	// 并发 decode + classify：几万文件串行 base64 decode 是后端瓶颈。enry 的
	// GetLanguage/IsVendor 等是纯函数查静态表，并发安全；mutex 保护共享的
	// bytesByLang/totalBytes 累加。
	workers := runtime.NumCPU() * 2
	if workers < 4 {
		workers = 4
	}
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, f := range req.Files {
		if ignored(gi, filepath.ToSlash(f.Path), false) {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(f FileEntry) {
			defer wg.Done()
			defer func() { <-sem }()
			// base64 content is optional; a decode failure simply falls back to
			// extension-only detection rather than failing the whole request.
			content, _ := base64.StdEncoding.DecodeString(f.Content)
			mu.Lock()
			detectDeclaredVersions(f.Path, content, versionsByLang)
			mu.Unlock()
			if lang, counted := classifyFile(f.Path, content); counted {
				mu.Lock()
				bytesByLang[lang] += f.Size
				totalBytes += f.Size
				mu.Unlock()
			}
		}(f)
	}
	wg.Wait()

	name := req.ProjectName
	if name == "" {
		name = "dropped-folder"
	}
	c.JSON(http.StatusOK, ScanResponse{
		Languages:   buildStats(bytesByLang, versionsByLang, totalBytes),
		TotalBytes:  totalBytes,
		ProjectName: name,
	})
}

// --- GitHub public repo analysis ---

// githubURLRe captures owner, repo, and optional branch from a github.com URL.
// owner/repo are constrained to URL-safe chars to prevent path traversal or
// injection into the codeload URL (they're interpolated into an HTTPS GET).
// Non-github.com hosts are rejected outright (SSRF guard).
var githubURLRe = regexp.MustCompile(`^https?://(?:www\.)?github\.com/([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+?)(?:\.git)?(?:/tree/([A-Za-z0-9._-]+))?(?:[/?#].*)?$`)
var githubProfileURLRe = regexp.MustCompile(`^https?://(?:www\.)?github\.com/([A-Za-z0-9-]+)(?:/?(?:[?#].*)?)?$`)

// maxGitHubDownload caps the tarball download so a huge repo can't exhaust
// memory. 200 MB covers most source repos (node_modules etc. are absent from
// git archives, so archives are far smaller than a full clone).
const maxGitHubDownload = 200 * 1024 * 1024

// handleGitHubProfileRepos lists repositories owned by a public GitHub user.
// Forks and archived repositories are hidden because they are usually not the
// author's active projects, and the UI explicitly presents this as a project picker.
func handleGitHubProfileRepos(c *gin.Context) {
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	m := githubProfileURLRe.FindStringSubmatch(strings.TrimSpace(req.URL))
	if m == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不是有效的 GitHub 作者主页链接（示例：https://github.com/owner）"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	type githubRepo struct {
		Name        string  `json:"name"`
		FullName    string  `json:"full_name"`
		Description *string `json:"description"`
		HTMLURL     string  `json:"html_url"`
		Language    *string `json:"language"`
		Stars       int     `json:"stargazers_count"`
		UpdatedAt   string  `json:"updated_at"`
		Fork        bool    `json:"fork"`
		Archived    bool    `json:"archived"`
		Private     bool    `json:"private"`
	}
	repos := make([]PublicRepo, 0)
	for page := 1; ; page++ {
		apiURL := fmt.Sprintf("https://api.github.com/users/%s/repos?type=owner&sort=updated&direction=desc&per_page=100&page=%d", m[1], page)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "构建 GitHub 请求失败"})
			return
		}
		httpReq.Header.Set("Accept", "application/vnd.github+json")
		httpReq.Header.Set("User-Agent", "linguagram")
		resp, err := http.DefaultClient.Do(httpReq)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "获取公开仓库失败：" + err.Error()})
			return
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("GitHub API 返回 %d（用户可能不存在或请求过于频繁）", resp.StatusCode)})
			return
		}
		var data []githubRepo
		err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&data)
		resp.Body.Close()
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "解析 GitHub 仓库列表失败"})
			return
		}
		for _, repo := range data {
			if repo.Private || repo.Fork || repo.Archived || repo.Name == "" || repo.HTMLURL == "" {
				continue
			}
			description, language := "", ""
			if repo.Description != nil {
				description = *repo.Description
			}
			if repo.Language != nil {
				language = *repo.Language
			}
			repos = append(repos, PublicRepo{Name: repo.Name, FullName: repo.FullName, Description: description, URL: repo.HTMLURL, Language: language, Stars: repo.Stars, UpdatedAt: repo.UpdatedAt})
		}
		if len(data) < 100 {
			break
		}
	}
	c.JSON(http.StatusOK, gin.H{"owner": m[1], "repos": repos})
}

// handleScanGitHub downloads a public GitHub repo's tarball and runs the same
// go-enry classification as drop mode. The tarball is a `git archive` snapshot,
// so it contains only git-tracked files - no .gitignore filtering needed, the
// scope already matches GitHub Linguist's tracked-file view.
func handleScanGitHub(c *gin.Context) {
	var req struct {
		URL string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}

	m := githubURLRe.FindStringSubmatch(strings.TrimSpace(req.URL))
	if m == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不是有效的 GitHub 仓库链接（示例：https://github.com/owner/repo）"})
		return
	}
	owner, repo, branch := m[1], m[2], m[3]

	if branch == "" {
		var err error
		branch, err = defaultBranch(c.Request.Context(), owner, repo)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "无法获取仓库默认分支：" + err.Error()})
			return
		}
	}

	// codeload.github.com serves the tar.gz directly off CDN (no API rate limit).
	tarballURL := "https://codeload.github.com/" + owner + "/" + repo + "/tar.gz/refs/heads/" + branch
	ctx, cancel := context.WithTimeout(c.Request.Context(), 120*time.Second)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, tarballURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "构建请求失败"})
		return
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "下载仓库失败：" + err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("GitHub 返回 %d（仓库可能不存在或为私有）", resp.StatusCode)})
		return
	}

	gz, err := gzip.NewReader(io.LimitReader(resp.Body, maxGitHubDownload))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解压 gzip 失败：" + err.Error()})
		return
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	bytesByLang := make(map[string]int64)
	versionsByLang := make(map[string]string)
	var totalBytes int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取 tar 失败：" + err.Error()})
			return
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		// tarball 顶层目录是 "{repo}-{ref}"，strip 它得到仓库相对路径，
		// classifyFile 的 vendor/dotfile heuristic 依赖相对路径。
		relPath := hdr.Name
		if idx := strings.IndexByte(relPath, '/'); idx >= 0 {
			relPath = relPath[idx+1:]
		} else {
			continue
		}
		// 只读 head 用于内容检测，其余不进内存。
		head, err := io.ReadAll(io.LimitReader(tr, readLimitBytes))
		if err != nil {
			continue
		}
		detectDeclaredVersions(relPath, head, versionsByLang)
		if lang, counted := classifyFile(relPath, head); counted {
			bytesByLang[lang] += hdr.Size
			totalBytes += hdr.Size
		}
	}

	c.JSON(http.StatusOK, ScanResponse{
		Languages:   buildStats(bytesByLang, versionsByLang, totalBytes),
		TotalBytes:  totalBytes,
		ProjectName: owner + "/" + repo,
		GitHubURL:   strings.TrimSpace(req.URL),
	})
}

// defaultBranch asks the GitHub REST API for the repo's default branch. Public
// repos need no token; the 60/h/IP anonymous limit is fine for a low-frequency
// tool. GitHub requires a User-Agent header or it 403s.
func defaultBranch(ctx context.Context, owner, repo string) (string, error) {
	subCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(subCtx, http.MethodGet,
		"https://api.github.com/repos/"+owner+"/"+repo, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "linguagram")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API 返回 %d", resp.StatusCode)
	}
	var data struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&data); err != nil {
		return "", err
	}
	if data.DefaultBranch == "" {
		return "", fmt.Errorf("未返回默认分支")
	}
	return data.DefaultBranch, nil
}

// ignored reports whether relPath (forward-slash, relative to root) is ignored.
// For directories we also test with a trailing slash so "work/"-style rules
// prune the whole subtree (SkipDir) rather than walking every file inside.
func ignored(gi *gitignore.GitIgnore, relPath string, isDir bool) bool {
	if gi == nil {
		return false
	}
	if gi.MatchesPath(relPath) {
		return true
	}
	return isDir && gi.MatchesPath(relPath+"/")
}

type npmFramework struct {
	Name    string
	Targets []string
}

var npmLanguagePackages = map[string][]string{
	"typescript":   {"TypeScript"},
	"vue":          {"Vue"},
	"astro":        {"Astro"},
	"svelte":       {"Svelte"},
	"coffeescript": {"CoffeeScript"},
	"less":         {"Less"},
	"sass":         {"Sass", "SCSS"},
	"node-sass":    {"Sass", "SCSS"},
	"stylus":       {"Stylus"},
	"pug":          {"Pug"},
	"@mdx-js/mdx":  {"MDX"},
	"elm":          {"Elm"},
	"rescript":     {"ReScript"},
}

var npmFrameworkPackages = map[string]npmFramework{
	"react":            {Name: "React", Targets: []string{"TypeScript", "JavaScript"}},
	"next":             {Name: "Next.js", Targets: []string{"TypeScript", "JavaScript"}},
	"@angular/core":    {Name: "Angular", Targets: []string{"TypeScript", "JavaScript"}},
	"nuxt":             {Name: "Nuxt", Targets: []string{"Vue", "TypeScript", "JavaScript"}},
	"preact":           {Name: "Preact", Targets: []string{"TypeScript", "JavaScript"}},
	"solid-js":         {Name: "SolidJS", Targets: []string{"TypeScript", "JavaScript"}},
	"@builder.io/qwik": {Name: "Qwik", Targets: []string{"TypeScript", "JavaScript"}},
	"@remix-run/react": {Name: "Remix", Targets: []string{"TypeScript", "JavaScript"}},
	"gatsby":           {Name: "Gatsby", Targets: []string{"TypeScript", "JavaScript"}},
	"@sveltejs/kit":    {Name: "SvelteKit", Targets: []string{"Svelte", "TypeScript", "JavaScript"}},
	"express":          {Name: "Express", Targets: []string{"TypeScript", "JavaScript"}},
	"@nestjs/core":     {Name: "NestJS", Targets: []string{"TypeScript", "JavaScript"}},
	"fastify":          {Name: "Fastify", Targets: []string{"TypeScript", "JavaScript"}},
	"koa":              {Name: "Koa", Targets: []string{"TypeScript", "JavaScript"}},
	"hono":             {Name: "Hono", Targets: []string{"TypeScript", "JavaScript"}},
	"vite":             {Name: "Vite", Targets: []string{"TypeScript", "JavaScript", "Vue", "Svelte"}},
	"tailwindcss":      {Name: "Tailwind CSS", Targets: []string{"CSS", "TypeScript", "JavaScript"}},
	"vitepress":        {Name: "VitePress", Targets: []string{"TypeScript", "JavaScript", "Vue"}},
	"@docusaurus/core": {Name: "Docusaurus", Targets: []string{"TypeScript", "JavaScript"}},
	"storybook":        {Name: "Storybook", Targets: []string{"TypeScript", "JavaScript", "Vue", "Svelte"}},
}

var nonNpmFrameworkTargets = map[string][]string{
	"FastAPI":     {"Python"},
	"Django":      {"Python"},
	"Flask":       {"Python"},
	"Spring Boot": {"Java"},
}

var pythonFrameworkPackages = map[string]string{
	"fastapi": "FastAPI",
	"django":  "Django",
	"flask":   "Flask",
}

const frameworkVersionPrefix = "$framework:"

// detectDeclaredVersions extracts versions explicitly declared by project
// manifests. Nested manifests are included so monorepos and split frontend /
// backend projects can annotate the language rows they contribute to.
func detectDeclaredVersions(relPath string, content []byte, versions map[string]string) {
	if len(content) == 0 {
		return
	}
	text := string(content)
	switch filepath.Base(relPath) {
	case "go.mod":
		if match := regexp.MustCompile(`(?m)^go\s+([^\s]+)`).FindStringSubmatch(text); len(match) == 2 {
			addVersion(versions, "Go", match[1])
		}
	case "package.json":
		var pkg struct {
			Engines         map[string]string `json:"engines"`
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		if json.Unmarshal(content, &pkg) != nil {
			return
		}
		dependencies := make(map[string]string, len(pkg.Dependencies)+len(pkg.DevDependencies))
		for name, version := range pkg.Dependencies {
			dependencies[name] = version
		}
		for name, version := range pkg.DevDependencies {
			if _, exists := dependencies[name]; !exists {
				dependencies[name] = version
			}
		}
		for packageName, version := range dependencies {
			if strings.TrimSpace(version) == "" {
				continue
			}
			for _, language := range npmLanguagePackages[packageName] {
				addVersion(versions, language, version)
			}
			if framework, ok := npmFrameworkPackages[packageName]; ok {
				addVersion(versions, frameworkVersionPrefix+framework.Name, version)
			}
		}
		if version := pkg.Engines["node"]; version != "" {
			addVersion(versions, "JavaScript", "Node "+version)
		}
	case ".nvmrc":
		if version := strings.TrimSpace(text); version != "" {
			addVersion(versions, "JavaScript", "Node "+version)
		}
	case "pyproject.toml":
		if match := regexp.MustCompile(`(?m)^\s*requires-python\s*=\s*["']([^"']+)["']`).FindStringSubmatch(text); len(match) == 2 {
			addVersion(versions, "Python", match[1])
		}
		detectNamedFrameworks(text, pythonFrameworkPackages, versions)
	case "requirements.txt":
		detectNamedFrameworks(text, pythonFrameworkPackages, versions)
	case "pom.xml":
		for _, pattern := range []string{
			`<maven\.compiler\.release>\s*([^<\s]+)\s*</maven\.compiler\.release>`,
			`<maven\.compiler\.target>\s*([^<\s]+)\s*</maven\.compiler\.target>`,
			`<maven\.compiler\.source>\s*([^<\s]+)\s*</maven\.compiler\.source>`,
		} {
			if match := regexp.MustCompile(pattern).FindStringSubmatch(text); len(match) == 2 {
				addVersion(versions, "Java", match[1])
				break
			}
		}
		if strings.Contains(text, "org.springframework.boot") {
			version := "已声明"
			if match := regexp.MustCompile(`(?s)spring-boot-starter-parent.*?<version>\s*([^<\s]+)\s*</version>`).FindStringSubmatch(text); len(match) == 2 {
				version = match[1]
			}
			addVersion(versions, frameworkVersionPrefix+"Spring Boot", version)
		}
	case "build.gradle", "build.gradle.kts":
		for _, pattern := range []string{
			`JavaLanguageVersion\.of\((\d+)\)`,
			`JavaVersion\.VERSION_(\d+)`,
			`sourceCompatibility\s*=\s*["']?([0-9.]+)`,
		} {
			if match := regexp.MustCompile(pattern).FindStringSubmatch(text); len(match) == 2 {
				addVersion(versions, "Java", match[1])
				break
			}
		}
		if strings.Contains(text, "org.springframework.boot") || strings.Contains(text, "spring-boot-starter") {
			addVersion(versions, frameworkVersionPrefix+"Spring Boot", "已声明")
		}
	}
}

// detectNamedFrameworks accepts conventional dependency strings from
// requirements.txt and pyproject.toml, including forms such as
// fastapi[standard]>=0.115 and django==5.1. The version is optional because
// some manifests intentionally leave it to a lockfile.
func detectNamedFrameworks(text string, packages map[string]string, versions map[string]string) {
	for packageName, frameworkName := range packages {
		pattern := `(?im)(?:^|["'\s,])` + regexp.QuoteMeta(packageName) + `(?:\[[^\]]+\])?\s*(?:(?:==|~=|>=|<=|>|<|\^|~)\s*([0-9][0-9A-Za-z.+_-]*))?`
		match := regexp.MustCompile(pattern).FindStringSubmatch(text)
		if len(match) == 0 {
			continue
		}
		version := "已声明"
		if len(match) == 2 && match[1] != "" {
			version = match[1]
		}
		addVersion(versions, frameworkVersionPrefix+frameworkName, version)
	}
}

func addVersion(versions map[string]string, key, version string) {
	version = strings.TrimSpace(version)
	if version == "" {
		return
	}
	values := strings.Split(versions[key], " / ")
	seen := make(map[string]bool, len(values)+1)
	for _, value := range values {
		if value != "" {
			seen[value] = true
		}
	}
	seen[version] = true
	values = values[:0]
	for value := range seen {
		values = append(values, value)
	}
	sort.Strings(values)
	versions[key] = strings.Join(values, " / ")
}

// classifyFile applies enry's skip rules and language detection to a single
// file, returning the detected language and whether it counts toward the bar.
// relPath is relative to the project root with "/" separators, so the vendor
// / dotfile / documentation / configuration heuristics behave as expected.
// content is the file head (≤ readLimitBytes); nil ⇒ extension-only.
func classifyFile(relPath string, content []byte) (string, bool) {
	// Parent-segment dotfile check — enry.IsDotFile only inspects the
	// basename (".wrangler/tmp/dev-x/index.js" has basename "index.js" which
	// returns false). Pruning the walk via directory SkipDir would be cleaner
	// but the upload path has no walker; do the equivalent here.
	for seg := range strings.SplitSeq(filepath.ToSlash(relPath), "/") {
		if seg != "" && strings.HasPrefix(seg, ".") {
			return "", false
		}
	}
	if enry.IsVendor(relPath) || enry.IsDotFile(relPath) ||
		enry.IsDocumentation(relPath) || enry.IsConfiguration(relPath) {
		return "", false
	}
	language := enry.GetLanguage(filepath.Base(relPath), content)
	if language == enry.OtherLanguage || language == "" {
		return "", false
	}
	// GitHub-style filter: only programming + markup count toward the bar.
	lt := enry.GetLanguageType(language)
	if lt != enry.Programming && lt != enry.Markup {
		return "", false
	}
	// Linguist groups dialects and companion syntaxes under their parent
	// language in repository statistics (for example TSX -> TypeScript).
	if group := enry.GetLanguageGroup(language); group != "" {
		language = group
	}
	return language, true
}

// buildStats turns a {language → bytes} tally into the sorted, percentage-laden
// payload the frontend renders.
func buildStats(bytesByLang map[string]int64, versionsByLang map[string]string, totalBytes int64) []LanguageStat {
	resolvedVersions := make(map[string]string, len(versionsByLang))
	for language, version := range versionsByLang {
		if !strings.HasPrefix(language, frameworkVersionPrefix) {
			resolvedVersions[language] = version
		}
	}
	frameworkTargets := make(map[string][]string, len(npmFrameworkPackages)+len(nonNpmFrameworkTargets))
	for _, framework := range npmFrameworkPackages {
		frameworkTargets[framework.Name] = framework.Targets
	}
	for name, targets := range nonNpmFrameworkTargets {
		frameworkTargets[name] = targets
	}
	frameworkNames := make([]string, 0, len(frameworkTargets))
	for name := range frameworkTargets {
		frameworkNames = append(frameworkNames, name)
	}
	sort.Strings(frameworkNames)
	for _, frameworkName := range frameworkNames {
		version := versionsByLang[frameworkVersionPrefix+frameworkName]
		if version == "" {
			continue
		}
		for _, target := range frameworkTargets[frameworkName] {
			if bytesByLang[target] == 0 {
				continue
			}
			labelled := frameworkName + " " + version
			if resolvedVersions[target] == "" {
				resolvedVersions[target] = labelled
			} else {
				resolvedVersions[target] += " · " + labelled
			}
			break
		}
	}

	stats := make([]LanguageStat, 0, len(bytesByLang))
	for name, b := range bytesByLang {
		pct := 0.0
		if totalBytes > 0 {
			pct = float64(b) / float64(totalBytes) * 100
		}
		stats = append(stats, LanguageStat{
			Name:       name,
			Percentage: round2(pct),
			Color:      enry.GetColor(name),
			Version:    resolvedVersions[name],
			Bytes:      b,
		})
	}
	// Highest percentage first — matches GitHub UI order.
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Percentage > stats[j].Percentage
	})
	return stats
}

func round2(f float64) float64 {
	return float64(int64(f*100+0.5)) / 100
}
