# 下载链路：从 RSS 到磁力离线下载

> 说明一条订阅从抓取 RSS 到真正提交 115 离线下载，中间经历了哪些步骤、每一步谁在做、AI 在其中扮演什么角色。

```
RSS 抓取 → 条目解析 → AI 解析+筛选(集号/选版信号/匹配规则/简中)
        → 重命名 → 每集选最优版本 → 查重(已下载?) → 115 离线下载
```

整个过程分两个阶段：**解析阶段**（AI 参与）与**下载阶段**（AI 不参与）。

---

## 阶段一：解析阶段（AI 参与）

入口：`RssService.GetItems` → `getItems`，对主 RSS 与每个备用 RSS 各执行一遍。

### 1. RSS 抓取

- 代码：`rss.GetRSS`（`internal/rss/rss.go`）
- 用配置的 Cookie/超时抓取 RSS XML，校验响应是合法 XML。
- 失败：记录 `rss获取失败`，该源返回空。

### 2. 条目解析（不用 AI）

- 代码：`rss.Parse`（`internal/rss/rss.go`）
- 从 XML 提取每个条目的 **标题 / 磁力链接(torrent) / infoHash / 大小 / 发布时间 / 字幕组**。
- 此时**不提取集号、不做规则过滤**（Episode 保持 0），集号与筛选全部留给 AI。

### 3. AI 解析 + 筛选（AI 的核心作用，一步完成）

- 代码：`ai.Parse`（`internal/provider/ai/deepseek.go`）
- **输入**：一批资源标题字符串 + 订阅上下文（`ani` 的 match/exclude 规则、全局简中开关 `AiSubtitleSC`）。
- **输出**：每个标题的结构化信息——`episode`(集号)、`resolution`(分辨率)、`subgroup`(字幕组)、`title`(纯剧名)、`isSpecial`(是否特别篇)，以及**选版信号**：`subtitleEmbed`(内封/内嵌/外挂)、`videoCodec`(HEVC/AVC)、`source`(BD/WebRip/Web)、`colorDepth`(10bit/8bit)、`subtitleLang`(简繁日等)。
- **同步筛选**：AI 判断每个标题是否满足订阅的 match/exclude 规则、以及（开启简中开关时）是否含简体中文字幕；不满足或无法判断集号的条目 `episode` 返回 0，由调用方丢弃。
- 简中规则：仅保留含**简体中文字幕**的资源（简中或简中双语视为满足）；纯繁中、无中文、仅外挂英文/日文字幕的排除。
- AI 从标题"读懂"集号（`S01E03`、`第03话`、`03`、`Vol.3`、`[03]`、`3.5` 等），这是正则难以稳定覆盖的。
- 失败：整批丢弃（不下载）。

### 4. 重命名

- 代码：`rename.RenameWithEpisode`（`internal/rename/rename.go`）
- 用 AI 给出的集号渲染 `reName`（目标文件名），如 `元祖！BanG Dream Chan S01E01`。
- 同时应用 `Skip5`（是否跳过 x.5 特别篇）等规则。

### 5. 每集选最优版本 + 去重（不用 AI）

- 代码：`rss.DistinctByEpisode` + `RssService.pickBestPerEpisode`（`internal/rss/rss.go` / `internal/service/rss_service.go`）
- 主源 + 备用源聚合后，**每集只保留一个版本**，比较链从高到低：
  1. 分辨率（2160p > 1080p > 720p）
  2. 压制源（BD/BDRip > WebRip > Web）
  3. 视频编码（HEVC/x265 > AVC/x264）
  4. 色深（10bit > 8bit）
  5. 字幕嵌入方式（内封 > 内嵌 > 外挂）
  6. 字幕语言丰富度（简繁日 > 简日 > 简 > 繁）
  7. 是否匹配订阅字幕组
  8. 是否主源（Master）
- 以上信号由 AI 解析阶段从标题提取（`ParsedTitle` 的选版字段）；提取不到的信号视为该档最低，不影响后续比较。
- 保证**每集不重复下载**。

---

## 阶段二：下载阶段（AI 不参与）

入口：`DownloadService.DownloadAni`（`internal/service/download_service.go`），对每个最终条目执行。

### 6. 查重（是否已下载）

- 代码：`DownloadAni` 内
- 依次检查：
  1. 内存 `hash:` 缓存（提交后写入，TTL 24h）
  2. **持久化 `DownloadedHash`**（infoHash 记录，重启后仍有效，避免重复下载）
  3. **持久化 `Downloaded`**（已下载集号）
  4. 用户标记 `NotDownload`（不下指定集）
  5. `DownloadNew`（只下最新集）、延迟下载（发布时间距今不足 `DelayedDownload` 分钟）
  6. `driver.FileExists`（检查 115 云端是否已有同名文件）
- 命中任一"已下载"则跳过该条。

### 7. 提交 115 离线下载

- 代码：`driver.AddOfflineTask`（`internal/cloud/driver_115/pan115.go`）
- 把 **磁力链接 + 目标云端目录** 提交给 115 离线下载（异步转存）。
- 115 返回"任务已存在"(10008) 视为成功（幂等）。
- 成功后：写入内存缓存、追加到 `Downloaded` / `DownloadedHash` 并持久化、发通知。

---

## AI 角色的总结

> AI 只做一件事：**看懂标题**。把一串资源标题解析成"第几集、什么画质、哪个字幕组、是不是简中字幕"，从而让系统能正确重命名、按集分组、选最优版本、判断是否已下载。
>
> 真正把磁力丢给 115 下载、查重、落盘这些**不经过 AI**。

---

## 相关配置项

| 配置 | 位置 | 作用 |
|---|---|---|
| `aiEnabled` | 设置 → AI 解析 | 总开关，关闭则无法解析集号 |
| `aiSubtitleSC` | 设置 → AI 解析 | 仅保留简中字幕资源，默认开启 |
| 订阅 `exclude` / `match` | 订阅 | 交给 AI 在解析阶段判断的匹配/排除规则 |
| 订阅 `subgroup` | 订阅 | 优先的字幕组 |
| 订阅 `downloadNew` | 订阅 | 只下最新一集 |
