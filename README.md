<div align="center">

<img src="docs/logo.png" width="120" alt="AniGo Logo">

# AniGo

**云端追番 · 自动下载 · 智能选版**

订阅动画 RSS，AI 智能解析，自动离线下载到网盘，通知推送，多设备管理。**全程不占本地硬盘，单二进制部署。**

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/React-18-61DAFB?logo=react&logoColor=white" alt="React">
  <img src="https://img.shields.io/badge/TypeScript-5-3178C6?logo=typescript&logoColor=white" alt="TypeScript">
  <img src="https://img.shields.io/badge/license-GPLv3-blue" alt="License">
  <img src="https://img.shields.io/badge/status-stable-brightgreen" alt="Status">
  <img src="https://github.com/Green-hats/AniGo/actions/workflows/ci.yml/badge.svg" alt="CI">
</p>

[快速开始](#快速开始) · [核心特性](#核心特性) · [使用流程](#使用流程) · [配置指南](#配置指南) · [项目结构](#项目结构) · [测试](#测试) · [架构设计](#架构设计)

</div>

---

## 界面预览

| 首页 · 我的订阅 | 番剧源 |
| :---: | :---: |
| <img src="docs/screenshots/home.png" width="400" alt="首页"> | <img src="docs/screenshots/garden.png" width="400" alt="番剧源"> |

| 设置 | 日志 |
| :---: | :---: |
| <img src="docs/screenshots/settings.png" width="400" alt="设置"> | <img src="docs/screenshots/logs.png" width="400" alt="日志"> |

## 核心特性

| 能力 | 说明 |
| --- | --- |
<table>
  <thead>
    <tr>
      <th width="220">能力</th>
      <th>说明</th>
    </tr>
  </thead>
  <tbody>
    <tr><td><b>云端追番</b></td><td>RSS 自动离线下载到 115 网盘，本地零存储</td></tr>
    <tr><td><b>AI 解析</b></td><td>DeepSeek 等大模型批量解析标题，提取集数 / 分辨率 / 字幕组 / 选版信号，同步完成规则与简中字幕筛选</td></tr>
    <tr><td><b>智能选版</b></td><td>同集多版本自动择优（分辨率 > 压制源 > 编码 > 色深 > 字幕嵌入/语言），每集不重复下载</td></tr>
    <tr><td><b>四源聚合</b></td><td><a href="https://animes.garden">animes.garden</a>（動漫花園 + 蜜柑 + 萌番组 + ANi 聚合）作番剧源</td></tr>
    <tr><td><b>在线播放</b></td><td>首页直接调用系统播放器（mpv 等）经本地代理播放 115 云端文件，无需下载</td></tr>
    <tr><td><b>元数据</b></td><td>Bangumi 评分 / 季数 / 总集数、封面下载</td></tr>
    <tr><td><b>通知</b></td><td>Telegram / Bark / ServerChan / WebHook / Shell / 系统日志</td></tr>
    <tr><td><b>单二进制</b></td><td>前端 React 构建产物嵌入后端，一个 <code>anigo</code> 搞定</td></tr>
    <tr><td><b>多设备</b></td><td>服务器部署，任意设备浏览器访问管理</td></tr>
  </tbody>
</table>

## 快速开始

### 环境要求

- **Go 1.26+**（构建）
- **Node.js 20+**（构建前端，`make all` 会先构建前端再嵌入）

> [!NOTE]
> 前端构建产物（`frontend/dist` 与嵌入用的 `backend/internal/httpapi/static`）在 `.gitignore` 中，clone 后需先 `make all` 构建，否则后端启动时没有嵌入前端页面。

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

> [!IMPORTANT]
> 配置目录按**启动时的工作目录**解析：默认 `./config`，`CONFIG` 环境变量可指定。务必始终在仓库根目录启动，避免在 `backend/` 下启动导致生成第二份配置。

### 开发模式（前后端热更新）

```bash
make dev    # 后端 :7789 + Vite 热更新 :37789（/api 自动代理）
```

## 使用流程

1. 一次性配置：115 Cookie、AI Key、通知渠道
2. 番剧源页选番剧 → 选择字幕组 → 一键订阅
3. 后台每 N 分钟自动抓 RSS → AI 解析 → 过滤 → 重命名
4. 查重后自动提交 115 离线下载
5. 缺集/摸鱼/完结自动检测 → 通知推送
6. 任意设备浏览器查看进度、管理订阅
7. 首页点击播放 → 调用系统播放器在线观看云端已下载文件

> 完整下载链路（RSS → AI 解析 → 过滤选版 → 查重 → 115 离线下载）见 [`docs/pipeline.md`](docs/pipeline.md)。

### 在线播放（可选）

首页订阅卡片点击播放图标，通过 `mpv-handler://` 协议拉起系统播放器（mpv 等）观看 115 云端文件：

1. 安装 [mpv-handler](https://github.com/akiirui/mpv-handler) 并注册 `mpv-handler://` 协议
2. 前端通过本地 `/api/file` 代理转发 115 CDN 流（自动带 115 UA 取流），播放器只访问本地端点，不暴露云端地址
3. 播放列表有 30 秒短缓存，避免频繁遍历 115 目录

## 配置指南

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

支持 OpenAI 兼容接口（DeepSeek / OpenAI / 通义 / 智谱 等）。集号提取与规则匹配全部由 AI 完成；某源连续失败会进入退避期，不影响其他订阅。

> [!WARNING]
> 项目内置的测试密钥放在 `backend/internal/domain/secrets.go`（已被 .gitignore 排除，不提交）。该文件被 `config.go` 的默认值引用，**clone 后需自行创建该文件**（否则编译报错），或将 Key/Cookie 直接填到设置页（运行时以配置为准）。

### 115 网盘下载

1. 获取 115 Cookie（`UID=...; CID=...; SEID=...; KID=...`），可用项目内置的扫码脚本（见下）
2. 设置页 → 下载 → 填入 Cookie → 测试 115 登录
3. 通过后开始云端离线下载，文件落盘在 115 网盘

#### 扫码获取 115 Cookie 脚本

`scripts/qrcode_cookie_115.py` 可通过扫码登录 115 并打印 Cookie，避免手动从浏览器复制：

```bash
pip install qrcode          # 需要 qrcode 库在终端输出二维码
python scripts/qrcode_cookie_115.py            # 终端显示二维码，手机 115 扫码
python scripts/qrcode_cookie_115.py -o         # 弹出二维码图片窗口扫码
python scripts/qrcode_cookie_115.py android    # 指定登录设备类型（会踢掉同类型已登录设备）
```

脚本输出形如 `UID=...; CID=...; SEID=...; KID=...`，直接填入设置页即可。

> 出处：[ChenyangGao/qrcode_cookie_115 · Gist](https://gist.github.com/ChenyangGao/d26a592a0aeb13465511c885d5c7ad61)

### 下载路径模板

默认 `番剧/${title}/Season ${season}`，支持占位符：

```
${title} ${season} ${seasonFormat} ${episode} ${episodeFormat}
${letter} ${quarter} ${quarterName} ${year} ${month} ${monthFormat}
${bgmId} ${jpTitle} ${subgroup}
```

### 通知

配置多条通知渠道（Telegram/Bark/ServerChan/WebHook/Shell/系统日志），每条可设定：

- **触发状态**：下载开始 / 完成 / 缺集 / 错误 / 完结 / 摸鱼
- **模板**：`${text} ${title} ${season} ${episode} ${emoji} ${action}` 等
- **重试次数、排序、备注**

## 项目结构

```
anigo/
├── backend/                  # 后端 Go
│   ├── cmd/anigo/            # 入口（DI 组装）
│   └── internal/
│       ├── domain/           # 领域模型 + 端口接口（ports.go）
│       ├── store/            # JSON 文件持久化 + TTL 缓存
│       ├── service/          # 业务服务（订阅/下载/通知/元数据/状态）
│       ├── provider/         # 适配器：bgm/garden/ai/notifier
│       ├── cloud/            # 网盘驱动（driver_115）
│       ├── rss/ rename/      # 纯函数：RSS 解析/剧集提取/重命名
│       ├── httpapi/          # Gin HTTP 层 + 嵌入前端
│       └── task/             # 后台任务循环（RSS 轮询）
├── frontend/                 # 前端 React + TS + Ant Design
│   ├── src/pages/            # 首页/番剧源/设置/日志页面
│   └── src/**/*.test.tsx     # vitest 组件与 API 测试
├── scripts/                  # 辅助脚本
│   ├── e2e.sh                # 端到端集成测试
│   └── qrcode_cookie_115.py  # 扫码获取 115 Cookie（第三方脚本）
├── docs/                     # 架构设计文档（architecture.md / pipeline.md）
├── Makefile                  # 构建/开发/测试统一入口
└── go.work                   # Go workspace（backend 模块）
```

## 测试

```bash
make test                      # 后端：go vet ./... && go test ./...
cd frontend && npm run test    # 前端：vitest（组件/API）
make e2e                       # 端到端集成测试（需真实外部服务：AI/115/BGM/animes.garden）
```

**覆盖范围：**

- **后端单元测试**：store（JSON 持久化/默认值/TTL 缓存）、util（格式化/拼音）、domain（时间/ID 序列化）、rename（集号/重命名模板）、rss、service、provider（AI/BGM/notifier/115/base）
- **前端测试**：API client（mock fetch 验证请求/错误处理）、App 路由渲染、SideMenu 导航
- **E2E**：基础 API / 配置 / AI / 元数据 / RSS 解析 / 订阅管理 / 115 登录 / 通知 / 导出导入

## 架构设计

- **端口-适配器（Hexagonal）**：业务逻辑只依赖 `domain/ports.go` 的接口，网盘/元数据/通知/AI 全部可插拔
- **手动依赖注入**：main 是唯一组装点，无全局单例
- **可测试**：HTTP 请求函数可注入，单测不依赖网络
- **契约兼容**：`config.v2.json` / `ani.v2.json` 格式与上游一致，可迁移

**技术栈：**

| 层 | 选型 |
| --- | --- |
| 后端 | Go 1.26 + Gin |
| 前端 | React + TypeScript + Vite + Ant Design |
| 存储 | 标准库 JSON 文件 |
| 日志 | `log/slog` 结构化日志 |

> 详见 [`docs/architecture.md`](docs/architecture.md) 与 [`docs/pipeline.md`](docs/pipeline.md)。

## License

本项目采用 [GNU General Public License v3.0](LICENSE)（GPL-3.0）。

> 免责声明：本工具为中立性技术辅助工具，请遵守当地法律法规，勿用于盗版传播。

## 致谢

- [ani-rss](https://github.com/wushuo894/ani-rss) — 思路与契约格式来源
- [mpv-handler](https://github.com/akiirui/mpv-handler) — 在线播放协议
- [ChenyangGao/qrcode_cookie_115](https://gist.github.com/ChenyangGao/d26a592a0aeb13465511c885d5c7ad61) — 115 扫码脚本

---

<p align="center">Made with ❤️ · AniGo</p>
