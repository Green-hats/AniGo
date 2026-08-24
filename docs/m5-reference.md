# M5 元数据 Provider 实现参考

> 本文档从老项目 ani-rss-go 提取 BGM/TMDB/Mikan/groupregex 的实现要点，
> 作为 anigo M5 元数据 provider 的移植蓝图。同时记录 115 驱动的补充要点。

## 0. 老项目 115 驱动补充要点（M4 待补）

老项目 `pan115.go` 的 `Download()` 与 anigo 的差异：

| 点 | 老项目 | anigo（当前） | 建议 |
|---|---|---|---|
| 提交后等待 | 阻塞轮询 `task_lists` 直到 status==2（成功）或 3/-1（失败） | 提交后立即返回 | M6 通知时补等待逻辑 |
| 完成通知 | 成功后 `notifyDone()` 发 `NotifyDownloadEnd` | 无 | M6 补 |
| 失败重试 | 超时/失败由 service 层 retry | 无 | M4 已留 DownloadRetry 配置 |
| 任务已存在 | errcode==10008 视为已添加继续等待 | 视为成功返回 nil | 一致 |

老项目等待循环关键逻辑（anigo 待补）：
```go
deadline := time.Now().Add(timeout * time.Minute)
for time.Now().Before(deadline) {
    list, _ := p.request("POST", "https://115.com/web/lixian/?ct=lixian&ac=task_lists", url.Values{"page": {"1"}})
    tasks := offlineTasks(list) // 顶层 tasks 或 data.tasks
    for _, t := range tasks {
        name := tm["name"]; infoHash := tm["info_hash"]; status := tm["status"]
        if name == reName || infoHashMatches(hash, infoHash) {
            if status == 2 { 通知完成; return true }
            if status == 3 || status == -1 { return false } // 失败
        }
    }
    // 兜底：任务已移出列表 → 检查云端文件存在
    if p.FileExists(cloudPath) { 通知完成; return true }
    time.Sleep(5 * time.Second)
}
```

---

## 1. BGM Provider（Bangumi）

**API 基础**：`BgmApi`（默认 `https://api.bgm.tv`），需要 `BgmToken`（OAuth Bearer）。

### 1.1 搜索
```
GET /search/subject/{name}?type=2&max_results=25&responseGroup=small
```
- name 需 `url.PathEscape`，`1/2` 先替换成 `½`
- 返回 `list` 数组，每项解析：id、name、name_cn、eps、type(platform)、air_date、images{small/grid/large/medium/common}、rating{score,total}、tags

### 1.2 番剧详情
```
GET /v0/subjects/{subjectId}
```
- 返回：id、name、name_cn、eps、platform、date、images、rating{rank/score/total}、tags
- **缓存 10 分钟**（`BGM_info:{id}`）

### 1.3 剧集列表
```
GET /v0/episodes?subject_id={id}&type={0或1}&limit=100&offset=0
```
- `GetEpisodeTitleMap(ani)` → (map[sort]nameCn, map[sort]name)，缓存 5 分钟
- sort 为 0 时用 ep 字段

### 1.4 剧集数推断 `GetEps(info)`
- `info.Eps < 1` 返回 0
- 否则 `GetEpisodes(info.ID, 0)`，长度 >0 返回 len，否则返回 info.Eps

### 1.5 Season 推断 `GetSeason(info)`
- 依次从 **tags → name_cn → name** 用 `GetSeasonByName` 找季
- `GetSeasonByName` 正则：`第?X季/期`（中文数字）、`Season N`、`Nth Season`、`S(N)$`
- 中文数字支持：一~九、十、十五 等

### 1.6 标题处理 `GetFinalName(info)`
- 优先 name_cn，`BgmJpName` 配置时用 name；空则 "无标题"

### 1.7 填充订阅 `ToAni(info, ani)`
- BgmUrl = `/subject/{id}`；Title = GetFinalName；JpTitle = info.Name
- Season = GetSeason；TotalEpisodeNumber = GetEps
- Ova = platform 为 "OVA" 或 "剧场版"
- Score、ReleaseDate、Image（BgmImage 配置决定取 small/medium/grid/common/large）
- **SaveCoverFn(imageURL) 下载封面存本地 files/**，ani.Cover 存相对路径

### 1.8 用户/评分
- `GET /v0/me` → 用户信息（id/username/nickname/avatar）
- `GET /v0/users/{username}/collections/{subjectId}` → 当前评分
- `POST /v0/users/-/collections/{subjectId}` body `{"score": n}` → 改评分

### 1.9 OAuth
- `ExchangeCode(code)`：POST `https://bgm.tv/oauth/access_token`，form 带 grant_type/client_id/client_secret/code/redirect_uri → 存 BgmToken + BgmRefreshToken
- `RefreshToken()`：grant_type=refresh_token，`BgmTokenType=="INPUT"` 时跳过
- SetToken：请求头 `Authorization: Bearer {token}`，UA `ani-rss/...`

### 1.10 SubjectId 解析 `GetSubjectId(ani)`
- 优先从 `ani.BgmUrl` 正则 `/subject/(\d+)`
- 否则 `GetSubjectIdByName(title, season)`（Search 后匹配 name/name_cn，缓存 10 分钟）

---

## 2. TMDB Provider

**API 基础**：`TmdbApi`（默认 `https://api.themoviedb.org`），`TmdbApiKey`（默认兜底 key）。

### 2.1 请求
```
GET {TmdbApi}/3/search/tv?query={name}&api_key=...&language=...
GET {TmdbApi}/3/search/movie?query={name}&api_key=...&language=...
GET {TmdbApi}/3/tv/{id}/season/{n}?api_key=...&language=...
GET {TmdbApi}/3/tv/{id}/episode_groups?api_key=...&language=...
GET {TmdbApi}/3/tv/episode_group/{groupId}
GET {TmdbApi}/3/{type}/{id}/images?api_key=...
```

### 2.2 关键数据
- `parseTv(m)` → model.Tmdb：id/name/original_name/poster_path/backdrop_path/overview/vote_average/vote_count/tagline/runtime/genres/networks/videos/cast
- `GetByName(name, ova)`：ova → 先搜 movie 再试 tv；否则先 tv 再 movie
- `GetFinalName(t)`：TmdbOriginalName 用原名；TitleYear 加 ` (year)`；TmdbId 加 ` [tmdbid=N]` 或 ` {tmdb-N}`（PlexMode）

### 2.3 图片
- `ImageURL(path)` = `{TmdbImage}/t/p/original` + path
- `GetTmdbImages` 按 `imageScore` 排序（优先匹配语言 zh-CN）

### 2.4 剧集标题
- `GetEpisodeTitleMap(ani)`：key=`TMDB_getEpisodeTitleMap:{id}:{groupId}:{season}`，缓存 5 分钟（空 10 秒）
- 有 TmdbGroupId 时用 episode_group 按 order==season 取；否则 /tv/{id}/season/{n}

### 2.5 剧组 `GetTmdbGroup(t)`
- `/3/tv/{id}/episode_groups` → results[{id,name}] → 返回 `{ID:0, Name, TmdbGroupId}`

---

## 3. Mikan Provider（HTML 爬取）

**基础**：`MikanHost`（默认 `https://mikanani.me`），需要 `golang.org/x/net/html`。

### 3.1 列表/搜索
```
GET {host}/Home/Search?searchstr={query}
GET {host}/Home/BangumiCoverFlowByDayOfWeek?year={y}&seasonStr={s}
```
- 解析 HTML：`date-select`（季下拉）、`sk-bangumi`（周分组）、`an-ul`（搜索列表）
- `collectAni`：li > span[data-src]（封面）+ a（标题/href）
- 特殊：搜索文本 `id: {数字}` → `GetMikanInfo`

### 3.2 详情页 `GetMikanInfo(bangumiId)`
```
GET {host}/Home/Bangumi/{bangumiId}
```
- `content > img[src]` 封面；`bangumi-title` 标题
- `bangumi-info` 含 "Bangumi番组计划链接：" 的 a[href] → BgmUrl
- `collectGroups`：`leftbar-item` 列表 → subgroup-name[data-anchor]、mikan-rss[href]、items 表、date

### 3.3 条目 `collectItems(tbody)`
- tr > 至少3个a：title(a[0] text)、magnet(a[1] data-clipboard-text)、torrent(a[2] href)
- td[2] size、td[3] date
- `parseMikanDate`：`2006-01-02 15:04` 等格式

### 3.4 工具
- `GetSubgroupId(url)`：提取 `subgroupid=` 参数
- `Host()`：去尾部 `/`

---

## 4. GroupRegex（字幕组过滤规则推导）

纯函数 `ToGroupRegex(titles) → GroupRegex{RegexList, Tags}`：
- 对每个标题匹配预定义正则列表（分辨率/语言/编码/格式）→ 生成 `{regex, label}` 对
- 去重（字符串级和结构级）
- Tags 最多收集 5 个去重标签
- **无外部依赖，纯逻辑，可直接移植为纯函数包**

---

## 5. animes.garden 番剧源（已确认首选）

老项目 animegarden.go 已完整阅读，关键端点：
```
GET https://api.animes.garden/subjects            → 番剧列表
GET https://api.animes.garden/resources?subject=&fansub=&pageSize=200&duplicate=false
GET https://api.animes.garden/feed.xml?subject=&fansub=   → RSS（订阅用）
GET https://api.animes.garden/detail/{provider}/{id}
```
- Group 构造：按 fansub 分组 items → 生成 RSS URL（fansub 名 QueryEscape）
- 分组按 LastUpdatedAt 倒序
- animes.garden 标题自带 SxxExx + guid 带 hash（解析简化）

---

## 6. M5 移植设计建议

- 全部做成本项目的 `provider/` 包（`provider/bgm/`、`provider/tmdb/`、`provider/mikan/`、`provider/garden/`）
- 去掉老项目 `config.Get()` 全局单例 → 改**构造器注入**：`NewBGM(cfg *ConfigService, cache domain.Cache)`
- 缓存用 `domain.Cache` 端口（已有 TTLCache 实现）
- 封面下载 `SaveCover` 放 service 层（需要 ConfigDirFile）
- Season 推断、中文数字、图片排序、groupregex 都是**纯函数**，放各自 provider 内或独立 `util` 子包，方便单测