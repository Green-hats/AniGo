# anigo — 架构设计文档

> ani-rss-go 的**全新架构重构**版本。本文档是重构的设计蓝图；除第 11 节里程碑外，各节描述的是**目标架构**。实现状态与文档不一致处已在文中标注「未实现」。

## 1. 背景与目标

ani-rss-go 是一个"云端追番"工具：RSS 自动离线下载到网盘（当前主要 115），外置播放器直连播放。老项目是 ani-rss（Java）的精简 Go 重写，但保留了 Java 风格的**全局单例 + 函数钩子**来绕开循环依赖（如 `config.Get()`、`download.Type()`、`rename.BgmSubjectId = ...`）。

本次重写目标：

- **全新架构**：端口-适配器（Hexagonal），彻底根除全局单例和函数钩子，改用依赖注入
- **可扩展下载器**：统一网盘接口，先实现 115，便于以后接阿里云盘等
- **可插拔元数据与通知**：全部 provider 接口化
- **前后端同仓分文件夹**：React + TypeScript
- **保留 JSON 文件存储**：`config.v2.json` / `ani.v2.json` 契约兼容，便于从老项目迁移

## 2. 技术栈

| 层 | 选型 | 说明 |
|---|---|---|
| 语言 | Go 1.26 | 与老项目一致 |
| HTTP | Gin | 路由/中间件/绑定 |
| DI | 手动（main 组装） | 接口注入，不用 fx 等黑盒 |
| 存储 | 标准库 JSON 文件 | `encoding/json` + 文件锁 |
| 前端 | React + TypeScript + Vite | 同仓 `frontend/` 文件夹，构建产物嵌入后端 |
| 日志 | 标准库 `log/slog` | 结构化日志 |
| 并发 | context + goroutine | 后台任务可控优雅退出 |

## 3. 目录结构（端口-适配器）

```
anigo/
├── backend/                      # 后端（Go）
│   ├── cmd/anigo/                # 入口：组装依赖、启动
│   │   └── main.go
│   └── internal/
│       ├── domain/               # 【领域层】纯模型 + 接口（不依赖具体实现）
│       │   ├── config.go         #   Config / Login / NotificationConfig
│       │   ├── ani.go            #   Ani 订阅 / StandbyRss
│       │   ├── misc.go           #   Item / TorrentsInfo / 枚举
│       │   ├── time.go           #   DateTime / Date 自定义 JSON 解析
│       │   └── ports.go          #   ★ 所有端口接口定义（核心）
│       ├── store/                # 【适配器】JSON 文件持久化
│       │   └── json_store.go     #   ConfigStore / AniStore 实现 + TTL 缓存
│       ├── service/              # 【应用层】业务逻辑（依赖端口接口）
│       │   ├── config_service.go #   配置读写/部分合并/默认值
│       │   ├── ani_service.go    #   订阅增删改/列表分组
│       │   ├── download_service.go#  下载主流程/缺集/摸鱼/完结
│       │   ├── rss_service.go    #   RSS 聚合/过滤/剧集提取
│       │   ├── metadata_service.go#  元数据编排（BGM/番剧源）
│       │   └── notify_service.go #   通知分发
│       ├── rss/                  #   纯函数：RSS XML 解析
│       ├── rename/               #   纯函数：剧集识别 + 重命名模板
│       ├── provider/             # 【适配器】外部服务
│       │   ├── bgm/              #   Bangumi（搜索/剧集/评分/封面）
│       │   ├── garden/           #   animes.garden 番剧源（唯一番剧源）
│       │   ├── ai/               #   AI 引擎（DeepSeek，OpenAI 兼容）
│       │   └── notifier/         #   通知器（telegram/bark/serverchan/webhook/shell/system）
│       ├── cloud/                # 【适配器】网盘
│       │   ├── registry.go       #   CloudDriver 注册表
│       │   └── driver_115/       #   115 实现（CloudDriver 接口见 domain/ports.go）
│       ├── httpapi/              # 【适配器】Gin HTTP 层（只做 HTTP 适配）
│       │   ├── server.go         #   Gin 实例 + 中间件 + 路由注册
│       │   ├── static.go         #   嵌入前端静态资源
│       │   └── handler_*.go      #   handler（薄，转发 service）
│       └── task/                 #   后台任务循环（RSS 轮询）
├── frontend/                     # 前端（React+TS+Vite）
├── docs/                         # 文档
└── go.work                       # Go workspace
```

## 4. 分层与依赖规则（Hexagonal）

```
HTTP(Gin) → service → domain(端口接口)
                ↑          ↑
                └── 实现适配器：store / provider / cloud / rss / rename
```

**依赖方向**（只允许内向依赖）：
- `httpapi` 依赖 `service`、`domain`
- `service` 依赖 `domain`（端口接口）+ 纯函数包（`rss`/`rename`）+ 适配器接口
- `store`/`provider`/`cloud` 实现 `domain` 里的端口接口
- `domain` 不依赖任何包（纯类型 + 接口）
- `main` 是唯一组装点（composition root）

**循环依赖根治**：端口接口全部定义在 `domain/ports.go`，实现方（adapter）依赖 domain 接口，service 也依赖 domain 接口，两者都指向 domain，不互相引用 → 无环。

## 5. 端口接口设计（domain/ports.go）

### 5.1 存储端口

```go
// ConfigStore 配置持久化
type ConfigStore interface {
    Load() (*Config, error)          // 读 + 应用默认值
    Save(c *Config) error
    LoadAnis() ([]*Ani, error)
    SaveAnis([]*Ani) error
}

// KeyValueCache 轻量内存缓存（缺集/摸鱼去重）
type Cache interface {
    Get(key string) (string, bool)
    Put(key, val string, ttl time.Duration)
    Contains(key string) bool
    Clear()
}
```

### 5.2 网盘端口（统一网盘接口，关键设计）

```go
// CloudDriver 统一网盘驱动接口。目前实现 driver_115，后续扩展其他网盘。
type CloudDriver interface {
    // 元数据
    Name() string
    // 登录/鉴权验证
    Login(test bool, cfg *CloudConfig) error
    // 离线下载一个磁力/ed2k 任务到指定云端目录，阻塞等待完成
    AddOfflineTask(ctx, magnet string, destPath string) error
    // 云端文件操作
    FileExists(ctx, path string) (bool, error)
    FileURL(ctx, path string) (string, error)   // 可播放/下载 URL
    ListDir(ctx, path string) ([]CloudFile, error)
    DeleteDir(ctx, path string) error
}

type CloudFile struct {
    Name string
    Size int64
    IsDir bool
    ID string
    PickCode string
}

// CloudConfig 是网盘驱动专属配置。**115 用浏览器 Cookie 鉴权**：
// 用户在网页登录 115 后复制 Cookie（UID=...; CID=...; SEID=...; KID=...），
// 填到配置的 pan115Cookie 字段。driver 在每次请求带上该 Cookie 完成鉴权。
type CloudConfig struct {
    Pan115Cookie string // 115 浏览器 Cookie，驱动自己决定怎么用（如塞进 HTTP Header）
    // 未来：Aliyun token、PikPak email/password...
}
```

设计要点：
- **一个接口 + 注册表**：`cloud.Register(name, factory)`，`DownloadToolType` 字段决定用哪个 driver
- **鉴权是 driver 的职责**：Cookie 校验在 `Login()` 里做（请求 115 接口验证 cookie 有效），业务层不关心鉴权细节
- 下载路径模板仍由 `download_service` 统一解析，把最终云端路径传给 `CloudDriver.AddOfflineTask`
- `CloudDriver` 不感知 ani/subscription，只做"路径+磁力 → 云端文件"的原子操作，保持可复用

### 5.3 元数据 provider 端口

```go
// MetadataProvider 元数据源（BGM）
type MetadataProvider interface {
    Name() string
    Search(ctx, keyword string) ([]SearchResult, error)
    GetInfo(ctx, subjectID string) (*MediaInfo, error)
    EpisodeTitles(ctx, subjectID string) (map[int]EpisodeTitle, error) // 可选
}

// FansubSource 番剧源（animes.garden / Mikan / ani-bt）
type FansubSource interface {
    Name() string
    // 从订阅 URL 解析出可下载的 RSS
    ResolveRSS(ctx, url string) (string, error)
    Groups(ctx, url string) ([]Group, error)
}
```

**番剧源：animes.garden**（唯一番剧源，用户已确认 RSS 端用它的 API）
- 聚合站：動漫花園 + 蜜柑 + 萌番组 + ANi 四源合一，一个订阅覆盖全部字幕组
- 开放 API（`https://api.animes.garden`）：
  - `GET /feed.xml` — 自定义 RSS 订阅（`?subject=&fansub=&filter=`），标题规整自带 `SxxExx`，guid 带 infohash
  - `GET /resources` — 资源搜索（include/exclude/keywords/fansub/type 过滤）
  - `GET /subjects` — 番剧周列表（首页番剧源浏览）
  - `GET /detail/{provider}/{id}` — 资源详情
- 已实现：`provider/garden` 提供 `ListSubjects` / `Group` / `GetSubjectIdFromURL`
- ~~备选源 Mikan / ani-bt~~（未实现）

BGM 提供搜索/剧集/评分元数据；评分与 OAuth 相关能力未接入，已从代码中移除。

### 5.4 通知端口

```go
type Notifier interface {
    Name() string
    Enabled() bool
    Send(ctx, msg *Notification) error
}

type Notification struct {
    Title    string
    Body     string
    Status   NotificationStatus // download_start / download_end / omit / error / ...
    Ani      *Ani
    Template string
}
```

通知配置 `NotificationConfig` 里的 `notificationType` 枚举决定选哪个 notifier，`service` 遍历所有 notifier 按配置分发。

### 5.5 AI 解析/过滤端口（全新能力）

老项目用**正则**从 RSS 标题抠集数/分辨率/字幕组 + 用排除/匹配规则过滤，痛点：
- 字幕组格式多样，正则常漏/误伤（如 `\d-\d` 误伤 `H265-10bit`）
- 规则维护成本高

anigo 引入 **AI（云端大模型 API）** 理解标题。做成可插拔端口：

```go
// TitleParser 用 AI 批量解析 RSS 标题（一次调用处理多条，控成本降延迟）
type TitleParser interface {
    Parse(ctx, titles []string) ([]ParsedTitle, error)
}

type ParsedTitle struct {
    RawTitle   string
    Episode    float64 // 集数（含 .5 特别篇）
    Resolution string  // 1080P / 2160P...
    Subgroup   string  // 字幕组
    Title      string  // 去掉格式后的剧名
    IsSpecial  bool    // 是否 x.5 特别篇
}

// ItemFilter 用 AI 判断条目是否匹配订阅（match/exclude 用自然语言描述）
type ItemFilter interface {
    Filter(ctx, ani *Ani, items []*Item) ([]bool, error) // 每个条目 keep/drop
}
```

**引擎**（仅 AI，无正则兜底）：
```
        ┌────────────────────────────────┐
        │  RssService（业务层）            │
        └───────────────┬────────────────┘
                        │ 依赖接口
        ┌───────────────▼────────────────┐
        │         TitleParser 接口        │
        └───────────────┬────────────────┘
                        ▼
        ┌───────────────────────────────┐
        │  AI 引擎（DeepSeek，OpenAI兼容） │
        └───────────────────────────────┘
```

- **AI 引擎**：调用云端大模型 API（OpenAI 兼容协议，支持 DeepSeek/通义/智谱等），解析与规则筛选一步完成（`ai.Parse`）
- ~~正则引擎兜底~~（已移除，提交 `fe2a1ed`；`aiEnabled=false` 或未配 Key 时不下载任何条目）
- **配置开关**：`aiEnabled`（默认 true）、`aiProvider`、`aiApiKey`、`aiBaseURL`、`aiModel`、`aiSubtitleSC`

设计要点：
- **批量调用**：AI 一次处理多条标题，减少请求数、省成本、降延迟
- **结果结构化**：AI 返回 JSON（集数/分辨率/字幕组/选版信号），程序解析后复用老的重命名模板
- **失败退避**：某源连续 AI 解析失败达到阈值后进入退避期，期间跳过该源；不阻断其他订阅
- **简中筛选**：`aiSubtitleSC` 开启时仅保留含简中字幕的资源
- 前端配置页已实现"AI 设置"区与**测试按钮**（验证 Key/连通性）

## 6. 服务层（service）职责

| 服务 | 职责 |
|---|---|
| `ConfigService` | 读/写/部分合并配置，应用默认值，写回 store |
| `AniService` | 订阅 CRUD、列表分组（按季/周/拼音排序）、批量启停、导入导出 |
| `RssService` | 聚合主+备用 RSS → 粗筛 → AI 解析+筛选（集号/规则/简中）→ 每集选最优版本 → 重命名 |
| `DownloadService` | 下载主循环：登录→遍历订阅→解析RSS→查重→调网盘→缺集/摸鱼/完结检测→通知 |
| `MetadataService` | 元数据编排：BGM/番剧源 的搜索、详情、订阅反解析、封面下载 |
| `NotifyService` | 遍历 notifier 分发，应用模板 |

每个 service 用**构造函数注入**依赖（store / cloud / provider / 其他 service），例如：

```go
type DownloadService struct {
    cfg       *ConfigService
    rss       *RssService
    cloud     *CloudRegistry
    notify    *NotifyService
    cache     domain.Cache
    rename    *rename.Engine
    repo       domain.AniRepo   // store 接口
}
func NewDownloadService(cfg, rss, cloud, notify, cache, rename, repo) *DownloadService
```

## 7. 后台任务（context + goroutine）

```go
type TaskManager struct {
    ctx    context.Context
    cancel context.CancelFunc
    wg     sync.WaitGroup
}
func (t *TaskManager) Start() {
    t.ctx, t.cancel = context.WithCancel(context.Background())
    t.wg.Add(1)
    go t.runRSSLoop()   // 每 RssSleepMinutes 分钟（runBgmLoop 未实现）
}
func (t *TaskManager) Stop() { t.cancel(); t.wg.Wait() }
```

- 每个循环内部用 `select { case <-ctx.Done(): return; case <-time.After(d): }`
- 循环内每轮调用 service 时也透传 ctx，可被优雅中断
- `DownloadService.DownloadAni` 内部串行下载，用 `ctx` 控制取消
- ⚠️ 已知问题：`DownloadAni` 尚不感知 `ctx`，若下载进行中收到 SIGTERM，`Stop()` 会阻塞（未实现优雅中断）

## 8. HTTP 层（Gin）

- `httpapi.Server` 持有所有 service 的引用（main 注入）
- 路由用 Gin group，路径/契约**尽量保持与老项目一致**（前端迁移成本低）
- 中间件：当前仅 `gin.Logger()` / `gin.Recovery()`
- ⚠️ **鉴权未实现**：`auth.CheckAuth`（登录/令牌/IP 白名单）尚未落地，任意访问者都可读写配置（含 AI Key、115 Cookie）
- handler 只做：绑定请求体 → 调 service → 包装 `model.Result{code,message,data,t}` 响应
- 静态资源：`frontend/dist` 构建产物拷入 `internal/httpapi/static/` 用 `embed` 嵌入，Gin `NoRoute` 兜底 SPA

## 9. 前端（React + TS，同仓 `frontend/`）

```
frontend/
├── package.json
├── vite.config.ts
├── index.html
└── src/
    ├── main.tsx
    ├── App.tsx
    ├── api/            # REST 客户端封装（fetch）
    ├── pages/          # 首页/番剧源/设置/日志
    ├── components/     # 通用组件（SideMenu 等）
    ├── hooks/
    ├── types/          # TS 类型（对齐后端契约）
    └── store/          # 状态管理（zustand）
```

- 数据获取：**React Query**（TanStack Query），全局状态 **zustand**
- UI 组件库：**Ant Design**（中后台配置项多，组件丰富）
- 构建产物拷入 `internal/httpapi/static/`，后端 `embed` 嵌入

## 10. 数据契约兼容

保留 `config.v2.json` / `ani.v2.json` 字段名，`domain.Config` / `domain.Ani` 的 JSON tag 与老项目一致，实现无缝迁移。

## 11. 里程碑（实现顺序）

1. **M1 骨架**：domain 模型 + ports + JSON store + main 组装 + Gin 起服务 + `/api/ping`、`/api/config` 读 ✅
2. **M2 配置**：ConfigService 完整（读写/合并/默认值/导出导入）✅
3. **M3 订阅**：AniService + RssService（RSS 抓取/过滤/剧集/重命名）+ listAni/addAni/previewAni ✅
4. **M3.5 AI 引擎**：AI 解析+筛选端口（DeepSeek，OpenAI 兼容）✅
5. **M4 下载**：CloudDriver 接口 + driver_115 + DownloadService + 后台任务 ✅
6. **M5 元数据**：bgm/animes.garden provider ✅（Mikan 未实现）
7. **M6 通知**：Notifier 接口 + Telegram/Bark/ServerChan/WebHook/Shell/System ✅
8. **M7 前端**：React 骨架 + 首页/番剧源/设置/日志页面 ✅
9. **M8 打磨**：日志/前端体验 ✅；鉴权 ⚠️ 未实现；e2e 集成测试 ✅

> 实际实现还包含：状态接口（`/api/status`）、日志页、115 扫码脚本、播放代理（`/api/file`）、订阅自动触发下载。

---

**已确认决策**（见评审）：
1. 分层：端口-适配器（Hexagonal）
2. HTTP：Gin
3. 下载器：统一 `CloudDriver` 接口，先实现 115
4. 元数据：全部 provider 接口化
5. 通知：接口化多 notifier
6. 并发：context + goroutine
7. 前端：React + TS + Ant Design + React Query + zustand，同仓 `frontend/`
8. 存储：JSON 文件（契约兼容）
9. DI：手动（main 组装）
10. 番剧源：animes.garden 为主（Mikan/ani-bt 未实现）

**开放问题**：
1. 鉴权方案（当前无鉴权，见第 8 节 ⚠️）
2. 是否需要老项目真实 json 样本做迁移测试
