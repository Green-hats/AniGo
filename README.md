# 🌸 anigo — 云端追番自动下载

> 基于 [ani-rss](https://github.com/wushuo894/ani-rss) 思路的**全新架构重写**：订阅动画 RSS → AI 智能解析 → 自动离线下载到网盘（115）→ 通知推送 → 多设备前端管理。**全程不占本地硬盘，单二进制部署。**

## 核心特性

| 能力 | 说明 |
|---|---|
| **云端追番** | RSS 自动离线下载到 115 网盘，本地零存储 |
| **AI 解析** | DeepSeek 等大模型批量解析标题，提取集数/分辨率/字幕组，正则兜底 |
| **四源聚合** | animes.garden（動漫花園+蜜柑+萌番组+ANi 聚合）作番剧源 |
| **元数据** | Bangumi 评分/季数/总集数、TMDB 标题命名、封面下载 |
| **通知** | Telegram / Bark / ServerChan / WebHook / Shell / 系统日志 |
| **单二进制** | 前端 React 构建产物嵌入后端，一个 `anigo` 搞定 |
| **多设备** | 服务器部署，任意设备浏览器访问管理 |

## 快速开始

### 环境要求
- Go 1.26+（构建）
- Node.js 20+（构建前端，可选——可跳过用预置产物）

### 构建

```bash
make all    # 前端 + 后端一体构建，产物 backend/bin/anigo
```

若只改后端、前端已构建：

```bash
make build  # 仅构建后端（使用已嵌入的前端）
```

### 运行

```bash
./backend/bin/anigo               # 默认端口 7789，配置目录 ./config
PORT=9000 ./backend/bin/anigo     # 自定义端口
CONFIG=/path ./backend/bin/anigo  # 自定义配置目录
```

首次启动自动生成 `config.v2.json` / `ani.v2.json`。浏览器打开 `http://服务器:7789` 即可管理。

### 开发模式（前后端热更新）

```bash
make dev    # 后端 :7789 + Vite 热更新 :37789（/api 自动代理）
```

## 配置

### AI 解析（可选，默认开启）

设置页 → AI 解析，或直接改 `config.v2.json`：

```json
{
  "aiEnabled": true,
  "aiProvider": "deepseek",
  "aiApiKey": "sk-...",
  "aiBaseURL": "https://api.deepseek.com",
  "aiModel": "deepseek-v4-flash"
}
```

支持 OpenAI 兼容接口（DeepSeek / OpenAI / 通义 / 智谱 等）。AI 失败时自动回退正则解析。

> 项目内置的测试密钥放在 `backend/internal/domain/secrets.go`（已被 .gitignore 排除，不提交）。clone 后需自行创建该文件或改用环境变量。

### 115 网盘下载

1. 浏览器登录 [115 网盘](https://115.com)，复制 Cookie（`UID=...; CID=...; SEID=...; KID=...`）
2. 设置页 → 下载 → 填入 Cookie → 测试 115 登录
3. 通过后开始云端离线下载，文件落盘在 115 网盘

### 下载路径模板

默认 `番剧/${title}/Season ${season}`，支持占位符：

```
${title} ${season} ${seasonFormat} ${episode} ${episodeFormat}
${letter} ${quarter} ${quarterName} ${year} ${month} ${monthFormat}
${tmdbid} ${bgmId} ${jpTitle} ${themoviedbName} ${subgroup}
```

### 通知

配置多条通知渠道（Telegram/Bark/ServerChan/WebHook/Shell/系统日志），每条可设定：
- 触发状态：下载开始 / 完成 / 缺集 / 错误 / 完结 / 摸鱼
- 模板：`${text} ${title} ${season} ${episode} ${emoji} ${action}` 等
- 重试次数、排序、备注

## 使用流程

```
① 一次性配置：115 Cookie、AI Key、通知渠道
② 番剧源页选番剧 → 选择字幕组 → 一键订阅
③ 后台每 N 分钟自动抓 RSS → AI 解析 → 过滤 → 重命名
④ 查重后自动提交 115 离线下载
⑤ 缺集/摸鱼/完结自动检测 → 通知推送
⑥ 任意设备浏览器查看进度、管理订阅
```

## 项目结构

```
anigo/
├── backend/                  # 后端 Go
│   ├── cmd/anigo/            # 入口（DI 组装）
│   └── internal/
│       ├── domain/           # 领域模型 + 端口接口（ports.go）
│       ├── store/            # JSON 文件持久化 + TTL 缓存
│       ├── service/          # 业务服务（订阅/下载/通知/元数据）
│       ├── provider/         # 适配器：bgm/tmdb/garden/ai/notifier
│       ├── cloud/            # 网盘驱动（driver_115）
│       ├── rss/ rename/      # 纯函数：RSS 解析/剧集提取/重命名
│       ├── httpapi/          # Gin HTTP 层 + 嵌入前端
│       └── task/             # 后台任务循环（RSS 轮询）
├── frontend/                 # 前端 React + TS + Ant Design
├── scripts/e2e.sh            # 端到端集成测试
└── docs/                     # 架构设计文档
```

## 测试

```bash
make test   # 单元测试 + vet
make e2e    # 端到端集成测试（需真实外部服务）
```

E2E 覆盖：基础 API / 前端 / 配置 / AI / 元数据 / RSS 解析 / 订阅管理 / 115 登录 / 通知 / 导出导入 / 删除。

## 里程碑

```
M1 骨架 → M2 配置 → M3 订阅+RSS → M3.5 AI引擎 → M4 下载核心 → M5 元数据 → M6 通知 → M7 前端
```

## 架构设计

- **端口-适配器（Hexagonal）**：业务逻辑只依赖 `domain/ports.go` 的接口，网盘/元数据/通知/AI 全部可插拔
- **手动依赖注入**：main 是唯一组装点，无全局单例
- **可测试**：HTTP 请求函数可注入，单测不依赖网络
- **契约兼容**：`config.v2.json` / `ani.v2.json` 格式与上游一致，可迁移

详见 [`docs/architecture.md`](docs/architecture.md) 与 [`docs/breakdown.md`](docs/breakdown.md)。

## 免责声明

本工具为中立性技术辅助工具，请遵守当地法律法规，勿用于盗版传播。