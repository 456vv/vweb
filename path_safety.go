package vweb

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

var (
	errPathEscapesRoot = errors.New("vweb: path escapes root")

	errInvalidRootPath = errors.New("vweb: invalid RootPath")
	errInvalidFilePath = errors.New("vweb: invalid file path")
	errPathTraversal   = errors.New("vweb: path traversal detected")
	errSymlinkEscape   = errors.New("vweb: path traversal detected")

	// 文件级错误，供动态/静态处理器映射状态码。
	errFileNotFound     = errors.New("vweb: file not found")
	errPermissionDenied = errors.New("vweb: permission denied")
)

// evalPathWithinRoot 返回 targetAbs 解析符号链接后的真实路径，并保证其位于 rootAbs 下。
func evalPathWithinRoot(rootAbs, targetAbs string) (string, error) {
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", errInvalidRootPath
	}

	existing := filepath.Clean(targetAbs)
	var suffix []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		suffix = append(suffix, filepath.Base(existing))
		existing = parent
	}

	realExisting, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}

	realTarget := realExisting
	for i := len(suffix) - 1; i >= 0; i-- {
		realTarget = filepath.Join(realTarget, suffix[i])
	}

	rel, err := filepath.Rel(rootReal, realTarget)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errPathEscapesRoot
	}
	return realTarget, nil
}

// resolvePagePath 规范化页面路径，并保证最终文件位于 rootPath 之下（含符号链接校验）。
//
// 支持 /a/../b：path.Clean("/"+pagePath) 消解 ".."，前导 "/" 保证不能越出根。
// 安全性由三层兜底：isLocalPath(rel)（跨平台分隔符）、evalPathWithinRoot（symlink）、
// 调用方的 os.Stat（404）。
func resolvePagePath(rootPath, pagePath, urlPath, defaultPage string) (cleanPage, fileAbs, rel string, err error) {
	if pagePath == "" {
		pagePath = urlPath
	}
	pagePath = path.Clean("/" + pagePath)
	pagePath = strings.TrimPrefix(pagePath, "/")
	if pagePath == "" || pagePath == "." {
		pagePath = defaultPage
	}

	filePath := filepath.Join(rootPath, filepath.FromSlash(pagePath))

	rootAbs, err := filepath.Abs(rootPath)
	if err != nil {
		return "", "", "", errInvalidRootPath
	}
	fileAbs, err = filepath.Abs(filepath.Clean(filePath))
	if err != nil {
		return "", "", "", errInvalidFilePath
	}
	rel, err = filepath.Rel(rootAbs, fileAbs)
	if err != nil || !isLocalPath(rel) {
		return "", "", "", errPathTraversal
	}
	fileAbs, err = evalPathWithinRoot(rootAbs, fileAbs)
	if err != nil {
		if errors.Is(err, errInvalidRootPath) {
			return "", "", "", errInvalidRootPath
		}
		return "", "", "", errSymlinkEscape
	}
	return pagePath, fileAbs, rel, nil
}

// resolveVirtualPath 解析自定义存储（ReadFile != nil）的页面路径。
// 不访问本地文件系统：RootPath 视为逻辑前缀，fileAbs 直接交给 ReadFile。
// 支持 /a/../b：path.Clean 消解 ".."，isLocalPath 兜底跨平台分隔符。
func resolveVirtualPath(rootPath, pagePath, urlPath, defaultPage string) (cleanPage, fileAbs, rel string, err error) {
	if pagePath == "" {
		pagePath = urlPath
	}
	pagePath = path.Clean("/" + pagePath)
	pagePath = strings.TrimPrefix(pagePath, "/")
	if pagePath == "" || pagePath == "." {
		pagePath = defaultPage
	}
	if !isLocalPath(pagePath) {
		return "", "", "", errPathTraversal
	}
	fileAbs = path.Join(rootPath, pagePath)
	return pagePath, fileAbs, pagePath, nil
}

// isLocalPath 检查相对路径是否安全。
func isLocalPath(rel string) bool {
	if rel == "" || rel == "." {
		return true
	}
	if filepath.IsAbs(rel) {
		return false
	}
	if filepath.VolumeName(rel) != "" {
		return false
	}
	if strings.HasPrefix(rel, `\\`) || strings.HasPrefix(rel, "//") {
		return false
	}
	if !filepath.IsLocal(rel) {
		return false
	}
	return !slices.Contains(strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == filepath.Separator }), "..")
}
