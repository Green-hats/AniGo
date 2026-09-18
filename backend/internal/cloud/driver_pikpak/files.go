package driverpikpak

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/domain"
)

type folderEntry struct {
	id      string
	expires time.Time
}

var errNotFound = errors.New("PikPak 路径不存在")

type remoteFile struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Kind           string      `json:"kind"`
	Size           json.Number `json:"size"`
	Trashed        bool        `json:"trashed"`
	WebContentLink string      `json:"web_content_link"`
	Links          map[string]struct {
		URL string `json:"url"`
	} `json:"links"`
	Medias []struct {
		Link struct {
			URL string `json:"url"`
		} `json:"link"`
		Origin bool `json:"is_origin"`
	} `json:"medias"`
}

func cloudPath(raw string) (string, error) {
	raw = strings.ReplaceAll(raw, "\\", "/")
	for _, part := range strings.Split(raw, "/") {
		if part == ".." {
			return "", errors.New("云端路径不能包含 ..")
		}
	}
	return path.Clean("/" + strings.TrimPrefix(raw, "/")), nil
}
func (p *PikPak) list(ctx context.Context, cfg *domain.Config, parent string) ([]remoteFile, error) {
	query := url.Values{"parent_id": {parent}, "limit": {"500"}, "with_audit": {"false"}, "filters": {`{"trashed":{"eq":false}}`}}
	files := []remoteFile{}
	seen := map[string]bool{}
	for page := 0; page < 1000; page++ {
		var result struct {
			Files []remoteFile `json:"files"`
			Next  string       `json:"next_page_token"`
		}
		if err := p.request(ctx, cfg, "GET", filesPath+"?"+query.Encode(), nil, &result); err != nil {
			return nil, err
		}
		if result.Files == nil {
			return nil, errors.New("PikPak 文件列表响应无效")
		}
		for _, file := range result.Files {
			if !file.Trashed {
				files = append(files, file)
			}
		}
		if result.Next == "" {
			return files, nil
		}
		if seen[result.Next] {
			return nil, errors.New("PikPak 文件分页游标重复")
		}
		seen[result.Next] = true
		query.Set("page_token", result.Next)
	}
	return nil, errors.New("PikPak 文件分页超过上限")
}
func (p *PikPak) folder(ctx context.Context, cfg *domain.Config, raw string, create bool) (string, error) {
	normalized, err := cloudPath(raw)
	if err != nil {
		return "", err
	}
	if normalized == "/" {
		return "", nil
	}
	parent, prefix := "", ""
	if p.folders == nil {
		p.folders = map[string]folderEntry{}
	}
	for _, name := range strings.Split(strings.TrimPrefix(normalized, "/"), "/") {
		prefix += "/" + name
		if entry, ok := p.folders[prefix]; ok && time.Now().Before(entry.expires) {
			parent = entry.id
			continue
		}
		files, err := p.list(ctx, cfg, parent)
		if err != nil {
			p.folders = nil
			return "", err
		}
		id := ""
		for _, f := range files {
			if f.Name == name && f.Kind == "drive#folder" {
				id = f.ID
				break
			}
		}
		if id == "" {
			if !create {
				return "", errNotFound
			}
			var result struct {
				File remoteFile `json:"file"`
			}
			if err := p.request(ctx, cfg, "POST", filesPath, map[string]string{"kind": "drive#folder", "parent_id": parent, "name": name}, &result); err != nil {
				return "", err
			}
			id = result.File.ID
			if id == "" {
				return "", errors.New("PikPak 创建目录未返回 ID")
			}
		}
		if p.folders == nil || len(p.folders) >= 4096 {
			p.folders = map[string]folderEntry{}
		}
		p.folders[prefix] = folderEntry{id: id, expires: time.Now().Add(30 * time.Second)}
		parent = id
	}
	return parent, nil
}
func (p *PikPak) lookup(ctx context.Context, cfg *domain.Config, raw string) (remoteFile, error) {
	normalized, err := cloudPath(raw)
	if err != nil {
		return remoteFile{}, err
	}
	if normalized == "/" {
		return remoteFile{Kind: "drive#folder"}, nil
	}
	parent, err := p.folder(ctx, cfg, path.Dir(normalized), false)
	if err != nil {
		return remoteFile{}, err
	}
	files, err := p.list(ctx, cfg, parent)
	if err != nil {
		p.folders = nil
		return remoteFile{}, err
	}
	for _, file := range files {
		if file.Name == path.Base(normalized) {
			return file, nil
		}
	}
	return remoteFile{}, errNotFound
}
func (p *PikPak) ListDir(ctx context.Context, cfg *domain.Config, raw string) ([]domain.CloudFile, error) {
	if err := p.lock(ctx, cfg); err != nil {
		return nil, err
	}
	defer p.unlock()
	parent, err := p.folder(ctx, cfg, raw, false)
	if err != nil {
		return nil, err
	}
	files, err := p.list(ctx, cfg, parent)
	if err != nil {
		p.folders = nil
		return nil, err
	}
	result := make([]domain.CloudFile, 0, len(files))
	for _, f := range files {
		size, _ := strconv.ParseInt(string(f.Size), 10, 64)
		result = append(result, domain.CloudFile{Name: f.Name, ID: f.ID, PickCode: f.ID, IsDir: f.Kind == "drive#folder", Size: size})
	}
	return result, nil
}
func (p *PikPak) FileExists(ctx context.Context, cfg *domain.Config, raw string) (bool, error) {
	if err := p.lock(ctx, cfg); err != nil {
		return false, err
	}
	defer p.unlock()
	_, err := p.lookup(ctx, cfg, raw)
	if errors.Is(err, errNotFound) {
		return false, nil
	}
	return err == nil, err
}
func (p *PikPak) fileURL(ctx context.Context, cfg *domain.Config, id string) (string, error) {
	if id == "" {
		return "", errors.New("PikPak 文件 ID 为空")
	}
	var file remoteFile
	if err := p.request(ctx, cfg, "GET", filesPath+"/"+url.PathEscape(id), nil, &file); err != nil {
		return "", err
	}
	// The original download link preserves subtitles and quality.
	candidates := []string{file.Links["application/octet-stream"].URL, file.WebContentLink}
	for _, media := range file.Medias {
		if media.Origin {
			candidates = append(candidates, media.Link.URL)
		}
	}
	for _, media := range file.Medias {
		candidates = append(candidates, media.Link.URL)
	}
	for _, candidate := range candidates {
		u, err := url.Parse(candidate)
		if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" {
			return candidate, nil
		}
	}
	return "", errors.New("PikPak 文件尚未完成或没有可用播放链接")
}
func (p *PikPak) FileURLByPickCode(ctx context.Context, cfg *domain.Config, id string) (string, error) {
	if err := p.lock(ctx, cfg); err != nil {
		return "", err
	}
	defer p.unlock()
	return p.fileURL(ctx, cfg, id)
}
func (p *PikPak) FileURL(ctx context.Context, cfg *domain.Config, raw string) (string, error) {
	if err := p.lock(ctx, cfg); err != nil {
		return "", err
	}
	defer p.unlock()
	file, err := p.lookup(ctx, cfg, raw)
	if err != nil {
		return "", err
	}
	if file.Kind == "drive#folder" {
		return "", errors.New("PikPak 目录不能播放")
	}
	return p.fileURL(ctx, cfg, file.ID)
}
func (p *PikPak) DeleteDir(ctx context.Context, cfg *domain.Config, raw string) error {
	if err := p.lock(ctx, cfg); err != nil {
		return err
	}
	defer p.unlock()
	defer func() { p.folders = nil; p.taskCacheUntil = time.Time{} }()
	normalized, err := cloudPath(raw)
	if err != nil {
		return err
	}
	if normalized == "/" {
		return errors.New("不能删除 PikPak 根目录")
	}
	file, err := p.lookup(ctx, cfg, normalized)
	if errors.Is(err, errNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if file.ID == "" {
		return fmt.Errorf("PikPak 文件 ID 为空")
	}
	return p.request(ctx, cfg, "DELETE", filesPath+"/"+url.PathEscape(file.ID), nil, nil)
}
