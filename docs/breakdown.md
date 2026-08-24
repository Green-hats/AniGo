# anigo — 功能拆分与技术设计

> 本文档基于老项目 ani-rss-go（Java ani-rss 的 Go 精简版）逐模块梳理功能，说明 anigo 重写时**怎么拆分、有哪些功能、设计理念是什么**。实现状态与文档不一致处已标注「未实现」。

## 0. 阅读指南

| 文档 | 内容 |
|---|---|
| `docs/architecture.md` | 分层架构、端口接口、目录结构、里程碑 |
| `docs/breakdown.md`（本文） | 功能清单、模块拆分、设计理念 |
| `docs/pipeline.md` | 下载链路（RSS → AI → 115）的逐步拆解 |

---

## 1. 项目定位回顾

**一句话**：云端追番自动下载工具 —— 订阅动画 RSS → 自动离线下载到网盘（115）→ 外置播放器直连播放，全程不占本地硬盘。

**核心链路**（老项目主流程）：
```
RSS 订阅 → 抓取/解析 → 剧集提取 → 过滤/去重 → 重命名
        → 查重(已下载?) → 调网盘离线下载 → 缺集/摸鱼/完结检测 → 通知
```

---

## 1.5 使用流程与产品形态（已确认）

**部署形态**：**服务器部署 + 多设备**。服务跑在一台服务器上，手机 / 电视 / 多台电脑通过浏览器访问管理，数据（配置/订阅）存在服务端，设备只是入口。

**使用目标**：
- 下载目的地：**纯云端 115**（本地零存储）
- 播放方式：**外置播放器跳转**（PotPlayer/VLC/MPV/Infuse 等，云端直链流式）
- 无本地浏览器/下载工具缓存管理，115 为唯一下载目标

**用户日常使用流程**：
```
① 一次性配置
   配 115 Cookie、AI Key、通知渠道（Telegram/Bark/ServerChan/WebHook/Shell/System）

② 追新番
   在"番剧源"页（animes.garden）选中 → 选字幕组 → 一键订阅

③ 后台自动轮询
   RSS 任务每 N 分钟自动抓取 → AI 解析+筛选 → 每集选最优版本 → 重命名 → 去重

④ 云端自动下载
   查重后把磁力提交给 115 → 离线转存完成 → 通知推送

⑤ 多设备播放
   任意设备浏览器访问 → 点某集 → 跳外置播放器（115 直链/本地代理）

⑥ 监控与维护
   订阅列表看进度/缺集/摸鱼，完结自动停订，通知提醒
```

**对架构/前端的影响**（服务器多设备形态强化）：
- 需要**鉴权**（⚠️ 未实现）：当前 HTTP 层无登录/令牌/IP 白名单，任意访问者可读写配置
- 前端**响应式**：适配手机/电视/电脑
- 服务端为单一数据源，设备只做浏览器入口

---

## 2. 功能模块拆分（怎么拆分）

按**业务能力域**拆，与"端口-适配器"分层结合。老项目是"包=功能"扁平拆分，anigo 是"包=能力域 + 域内分端口/实现"。

### 2.1 拆分后的模块地图

```
┌────────────────────────── HTTP 层（Gin，只做适配）─────────────────────────┐
│  httpapi: 路由 + handler + 鉴权中间件 + 静态资源                              │
└───────────────────────────────▲───────────────────────────────────────────┘
                                │ 调用
┌────────────────────────── 应用层（service，组合编排）───────────────────────┐
│  AniService     订阅 CRUD/列表/分组/导入导出                                  │
│  ConfigService  配置读写/合并/默认值/备份                                     │
│  DownloadService 下载主循环/缺集/摸鱼/完结/重命名调度                          │
│  NotifyService   通知分发（模板渲染 + 遍历 notifier）                         │
│  MetadataService 元数据编排（BGM/TMDB/番剧源 搜索与订阅反解析）                 │
│  RssService      RSS 聚合/AI解析筛选/每集选最优版本/备用源                      │
│  TaskManager    后台任务循环（RSS 轮询），context 控制                         │
└──────────────┬──────────────────────────────┬──────────────────────────────┘
               │ 依赖端口接口                    │ 依赖端口接口
┌──────────────▼───────────────┐  ┌───────────▼───────────────────────────────┐
│  domain（纯模型 + 端口接口）   │  │  适配器（实现端口接口）                      │
│  config.go / ani.go / misc.go │  │  store/    JSON 持久化 + TTL 缓存         │
│  ports.go   ★ 所有接口        │  │  cloud/    网盘（driver_115）             │
│  dto.go     API 传输对象      │  │  provider/ BGM/TMDB/animes.garden/ai     │
│                              │  │  notifier/ Telegram/Bark/ServerChan/...  │
│                              │  │  rss/      纯函数解析                      │
│                              │  │  rename/   纯函数剧集+重命名               │
└──────────────────────────────┘  └───────────────────────────────────────────┘
```

### 2.2 各能力域详细功能

#### A. 订阅管理（AniService + domain.Ani）
- CRUD：add / set（部分合并，保留服务器字段）/ delete（可选删文件）
- 列表：按 SCORE / PINYIN / DOWNLOAD_TIME 排序，按"季/周"分组，返回 `ListAni`
- 批量启停、更新总集数（BGM）、刷新单个/全部、预览
- RSS→订阅（rssToAni）：从 animes.garden 的 URL 反解析出 Ani
- 导入/导出订阅

#### B. 配置管理（ConfigService + domain.Config）
- 读（登录密码打码）/写（部分合并，缺失字段补默认）
- 默认值工厂 `DefaultConfig()`（与 Java 静态块对齐）
- 下载工具类型/代理/115 Cookie/AI 配置/通知配置 等字段
- 备份导出（zip：files/ + 两个 json）/恢复导入

#### C. RSS 管道（RssService + rss 纯函数 + rename 纯函数 + provider/ai）
- **番剧源**：**animes.garden**（動漫花園/蜜柑/萌番组/ANi 聚合，开放 API + 自定义 feed.xml），唯一实现（Mikan/ani-bt 未实现）
- **抓取**：拉 feed.xml，校验，超时控制
- **粗筛**：进 AI 前用固定硬规则剔除明显不需要的条目（合集/低清等），降本
- **AI 解析+筛选**（核心）：用云端大模型**理解标题**，批量提取集数/分辨率/字幕组/选版信号，并同步判断 match/exclude 规则与简中字幕（无正则兜底）
- **重命名**：模板 `${title} S${seasonFormat}E${episodeFormat}` 等 20+ 占位符
- **每集选最优版本**：分辨率/压制源/编码/色深/字幕嵌入/字幕语言/字幕组 比较链，每集只保留一个版本
- **聚合**：主 RSS + 备用 RSS（standby）→ 去重 → 按集排序

#### D. 下载（DownloadService + cloud 适配器）
- 统一网盘接口 `CloudDriver`：Login / AddOfflineTask / FileExists / FileURL / FileURLByPickCode / ListDir / DeleteDir
- **115 实现**：浏览器 Cookie 鉴权 → 建目录 → 加离线任务（magnet）
- 查重逻辑：内存 hash 缓存 + 持久化 `DownloadedHash` / `Downloaded` + `NotDownload` + `DownloadNew` + 延迟下载
- 自动完结停订、缺集(omit)/摸鱼(procrastinating)检测、播放列表缓存
- 下载路径模板解析 `${title}/Season ${season}` 等

#### E. 元数据（provider 接口 + 适配器）
- **Bangumi**：搜索、番剧信息、评分、总集数、剧集标题、OAuth（⚠️ OAuth 未接入 HTTP）
- **TMDB**：标题命名、剧组、图片
- **animes.garden**：番剧源（聚合動漫花園/蜜柑/萌番组/ANi），提供番剧列表/字幕组分组/自定义 RSS/资源详情
- **Mikan / ani-bt**：~~备选~~（未实现）

#### F. 通知（NotifyService + notifier 接口）
- 状态：下载开始/结束、缺集、错误、完结、摸鱼
- 通知器：Telegram / Bark / ServerChan / WebHook / Shell / 系统日志
- 模板渲染 `${text}` `${title}` `${episode}` 等，按状态匹配、排序、重试

#### G. 基础设施
- **存储**：`config.v2.json` / `ani.v2.json` JSON 文件（契约兼容）
- **缓存**：内存 TTL 缓存（缺集/摸鱼去重、播放列表短缓存）
- **日志**：内存环形缓冲（供前端查看），slog 结构化输出
- **鉴权**：⚠️ 未实现（老项目有登录/IP 白名单/token，当前没有）
- **代理**：HTTP 代理配置（抓 RSS / 访问外部 API 等）

---

## 3. 设计理念

### 3.1 端口-适配器（Hexagonal）
- **核心**：业务逻辑（service）只依赖 `domain` 里定义的**接口**，不依赖任何具体实现
- **效果**：存储/网盘/元数据/通知都是"可插拔"的。换网盘 = 加一个 driver 实现并注册，改一行配置，不改业务代码
- **根治循环依赖**：所有接口集中在 `domain/ports.go`，实现方与调用方都指向 domain，天然无环

### 3.2 依赖注入（手动）
- 不用全局单例 `config.Get()` / `download.Type()`，改用**构造器注入**
- `main` 是唯一组装点（composition root），显式看到所有依赖如何连起来
- 好处：可测试（mock 接口）、可替换、无隐藏状态

### 3.3 领域模型纯净
- `domain` 只含纯数据结构和接口，**无 IO、无全局状态、无外部依赖**
- 所有副作用（网络/磁盘）都在适配器层

### 3.4 纯函数与有状态分离
- RSS 解析、剧集提取、重命名、路径模板、过滤 → **纯函数**（输入输出，无状态，易单测）
- 下载循环、通知分发、任务调度 → **有状态服务**（持有依赖，编排纯函数）

### 3.5 并发模型：context + goroutine
- 后台任务用 `context` 控制生命周期，优雅退出（避免 goroutine 泄漏）
- 下载循环串行（每订阅一个下载锁），任务循环各自独立

### 3.6 契约兼容优先
- JSON tag 与老项目完全一致 → 用户可从 ani-rss-go **无缝迁移**配置和订阅数据
- API 端点路径/响应结构尽量保持一致 → 前端迁移成本低

### 3.7 前后端分文件夹
- 后端 `backend/`，前端 `frontend/`，同仓库共享契约文档
- 前端构建产物 embed 进后端二进制，单文件部署

---

## 4. 老项目痛点 → 重写对策对照表

| 老项目痛点 | anigo 对策 |
|---|---|
| 全局单例 `config.Get()` 散落各处 | 构造器注入，显式依赖 |
| 函数钩子 `rename.BgmSubjectId = ...` 绕循环依赖 | 端口接口集中在 domain，无环 |
| `download.Type()` 按全局配置懒加载 | CloudRegistry 注册表，main 显式组装 |
| service 包内再包 `var fn = func()` 钩子 | service 通过构造器接收接口 |
| 手写 Router | 用 Gin |
| 日志 fmt.Printf 散落 | slog 统一 + 内存缓冲 |
| 单测少 | 纯函数 + 接口 mock，重点覆盖 rss/rename |
| 正则解析/过滤常漏、误伤字幕组格式 | 引入 AI 解析+筛选端口（云端 LLM），一步完成 |

---

## 5. 里程碑分解（实现顺序）

| 阶段 | 交付 | 状态 |
|---|---|---|
| **M1 骨架** | domain 模型 + ports + store + Gin 起服务 + ping/config 读 + main 组装 | ✅ |
| **M2 配置** | ConfigService 完整（合并/默认值/导出导入）| ✅ |
| **M3 订阅+RSS** | AniService + RssService（抓取/过滤/剧集/重命名）+ listAni/addAni/previewAni | ✅ |
| **M3.5 AI 引擎** | AI 解析+筛选端口 + 云端 LLM（DeepSeek）| ✅ |
| **M4 下载** | CloudDriver + driver_115 + DownloadService + TaskManager | ✅ |
| **M5 元数据** | provider 接口 + bgm/tmdb/animes.garden | ✅（Mikan 未实现）|
| **M6 通知** | Notifier 接口 + 6 个实现 + NotifyService | ✅ |
| **M7 前端** | React 骨架 + 首页/番剧源/设置/日志页面 | ✅ |
| **M8 打磨** | 日志/代理/备份 + 测试 | ✅ 鉴权 ⚠️ 未实现 |

---

## 6. 待办/开放问题

- [x] RSS 端确认：**animes.garden** API（已确认并实现为唯一番剧源）
- [x] AI：确认 provider（**DeepSeek**，OpenAI 兼容）、prompt 设计（可编辑）
- [x] 前端页面范围（首页/番剧源/设置/日志 已实现）
- [ ] **鉴权**：HTTP 层无登录/令牌/IP 白名单（最优先，任意访问者可改配置读密钥）
- [ ] 下载优雅退出：`DownloadAni` 不感知 ctx，SIGTERM 时 `TaskManager.Stop()` 可能阻塞
- [ ] 老项目真实 `config.v2.json` / `ani.v2.json` 样本用于迁移测试
- [ ] Mikan/ani-bt 是否作为备选番剧源补回（当前仅 animes.garden）
