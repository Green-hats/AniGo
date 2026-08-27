package domain

import (
	"crypto/md5"
	"encoding/hex"
)

// Login 是 Config 内嵌的登录设置。密码以 MD5 哈希存储。
type Login struct {
	Username string `json:"username"`
	Password string `json:"password"`
	IP       string `json:"ip,omitempty"`
	Key      string `json:"key,omitempty"`
}

// GitInfo 携带构建元数据（用户不可编辑）。
type GitInfo struct {
	Branch        string `json:"branch"`
	ShortCommitId string `json:"shortCommitId"`
	CommitId      string `json:"commitId"`
}

// NotificationStatusEnum 通知状态枚举（按名称序列化）。
type NotificationStatusEnum string

const (
	NotifyDownloadStart   NotificationStatusEnum = "DOWNLOAD_START"
	NotifyOmit            NotificationStatusEnum = "OMIT"
	NotifyError           NotificationStatusEnum = "ERROR"
	NotifyCompleted       NotificationStatusEnum = "COMPLETED"
	NotifyProcrastinating NotificationStatusEnum = "PROCRASTINATING"
)

// NotificationTypeEnum 通知类型枚举（按名称序列化）。
type NotificationTypeEnum string

const (
	NotifyServerChan  NotificationTypeEnum = "SERVER_CHAN"
	NotifySystem      NotificationTypeEnum = "SYSTEM"
	NotifyTelegram    NotificationTypeEnum = "TELEGRAM"
	NotifyWebHook     NotificationTypeEnum = "WEB_HOOK"
	NotifyShell       NotificationTypeEnum = "SHELL"
	NotifyBark        NotificationTypeEnum = "BARK"
)

// ServerChanTypeEnum ServerChan 类型枚举。
type ServerChanTypeEnum string

const (
	ServerChanType  ServerChanTypeEnum = "SERVER_CHAN"
	ServerChanType3 ServerChanTypeEnum = "SERVER_CHAN_3"
)

// NotificationConfig 是一条通知渠道配置。
type NotificationConfig struct {
	Enable               bool                   `json:"enable"`
	Retry                int                    `json:"retry"`
	Comment              string                 `json:"comment"`
	NotificationTemplate string                 `json:"notificationTemplate"`
	NotificationType     NotificationTypeEnum   `json:"notificationType"`
	ServerChanType       ServerChanTypeEnum     `json:"serverChanType"`
	ServerChanSendKey    string                 `json:"serverChanSendKey"`
	ServerChan3ApiUrl    string                 `json:"serverChan3ApiUrl"`
	TelegramBotToken     string                 `json:"telegramBotToken"`
	TelegramChatId       string                 `json:"telegramChatId"`
	TelegramTopicId      int                    `json:"telegramTopicId"`
	TelegramApiHost      string                 `json:"telegramApiHost"`
	TelegramImage        bool                   `json:"telegramImage"`
	TelegramFormat       string                 `json:"telegramFormat"`
	WebHookMethod        string                 `json:"webHookMethod"`
	WebHookUrl           string                 `json:"webHookUrl"`
	WebHookHeader        string                 `json:"webHookHeader"`
	WebHookBody          string                 `json:"webHookBody"`
	Shell                string                 `json:"shell"`
	BarkServerUrl        string                 `json:"barkServerUrl"`
	BarkDeviceKeys       string                 `json:"barkDeviceKeys"`
	BarkGroup            string                 `json:"barkGroup"`
	BarkUseMarkdown      bool                   `json:"barkUseMarkdown"`
	BarkLevel            string                 `json:"barkLevel"`
	BarkVolume           int                    `json:"barkVolume"`
	StatusList           []NotificationStatusEnum `json:"statusList"`
	Sort                 int64                  `json:"sort"`
}

// Config 是应用根配置，持久化为 config.v2.json。
type Config struct {
	DownloadToolType               string                `json:"downloadToolType"`
	DownloadRetry                  int                   `json:"downloadRetry"`
	PikpakEmail                    string                `json:"pikpakEmail"`
	PikpakPassword                 string                `json:"pikpakPassword"`
	Pan115Cookie                   string                `json:"pan115Cookie"`
	DownloadPathTemplate           string                `json:"downloadPathTemplate"`
	OvaDownloadPathTemplate        string                `json:"ovaDownloadPathTemplate"`
	DelayedDownload                int                   `json:"delayedDownload"`
	RssSleepMinutes                int                   `json:"rssSleepMinutes"`
	Rename                         bool                  `json:"rename"`
	Rss                            bool                  `json:"rss"`
	RssTimeout                     int                   `json:"rssTimeout"`
	FileExist                      bool                  `json:"fileExist"`
	Offset                         bool                  `json:"offset"`
	AutoDisabled                   bool                  `json:"autoDisabled"`
	Skip5                          bool                  `json:"skip5"`
	StandbyRss                     bool                  `json:"standbyRss"`
	Coexist                        bool                  `json:"coexist"`
	LogsMax                        int                   `json:"logsMax"`
	// LogsLevel 日志级别（DEBUG/INFO/WARN/ERROR），低于该级别的日志不记录。
	LogsLevel string `json:"logsLevel"`
	// LogsFile 日志落盘文件路径（相对配置目录，空表示不落盘）。
	LogsFile string `json:"logsFile"`
	Debug                          bool                  `json:"debug"`
	ProcrastinatingMasterOnly      bool                  `json:"procrastinatingMasterOnly"`
	Proxy                          bool                  `json:"proxy"`
	ProxyHost                      string                `json:"proxyHost"`
	ProxyPort                      int                   `json:"proxyPort"`
	ProxyUsername                  string                `json:"proxyUsername"`
	ProxyPassword                  string                `json:"proxyPassword"`
	Login                          Login                 `json:"login"`
	MultiLoginForbidden            bool                  `json:"multiLoginForbidden"`
	LoginEffectiveHours            int                   `json:"loginEffectiveHours"`
	Exclude                        []string              `json:"exclude"`
	ImportExclude                  bool                  `json:"importExclude"`
	EnabledExclude                 bool                  `json:"enabledExclude"`
	BgmJpName                      bool                  `json:"bgmJpName"`
	IpWhitelist                    bool                  `json:"ipWhitelist"`
	IpWhitelistStr                 string                `json:"ipWhitelistStr"`
	Omit                           bool                  `json:"omit"`
	BgmToken                       string                `json:"bgmToken"`
	ApiKey                         string                `json:"apiKey"`
	DownloadNew                    bool                  `json:"downloadNew"`
	RenameTemplate                 string                `json:"renameTemplate"`
	RenameDelYear                  bool                  `json:"renameDelYear"`
	VerifyLoginIp                  bool                  `json:"verifyLoginIp"`
	NotificationTemplate           string                `json:"notificationTemplate"`
	BgmImage                       string                `json:"bgmImage"`
	CustomCss                      string                `json:"customCss"`
	CustomJs                       string                `json:"customJs"`
	CustomEpisode                  bool                  `json:"customEpisode"`
	CustomEpisodeStr               string                `json:"customEpisodeStr"`
	CustomEpisodeGroupIndex        int                   `json:"customEpisodeGroupIndex"`
	Procrastinating                bool                  `json:"procrastinating"`
	ProcrastinatingDay             int                   `json:"procrastinatingDay"`
	UpdateTotalEpisodeNumber       bool                  `json:"updateTotalEpisodeNumber"`
	ForceUpdateTotalEpisodeNumber  bool                  `json:"forceUpdateTotalEpisodeNumber"`
	DownloadTimeout                int                   `json:"downloadTimeout"`
	NotificationConfigList         []NotificationConfig  `json:"notificationConfigList"`
	CopyMasterToStandby            bool                  `json:"copyMasterToStandby"`
	SortType                       string                `json:"sortType"`
	Replace                        bool                  `json:"replace"`
	MaxFileNameLength              int                   `json:"maxFileNameLength"`
	LimitLoginAttempts             bool                  `json:"limitLoginAttempts"`
	GitInfo                        *GitInfo              `json:"gitInfo"`
	ReverseProxyTrustIpListEnabled bool                  `json:"reverseProxyTrustIpListEnabled"`
	ReverseProxyTrustIpList        []string              `json:"reverseProxyTrustIpList"`
	BgmApi                         string                `json:"bgmApi"`
	AllowCors                      bool                  `json:"allowCors"`
	UUID                           string                `json:"uuid"`
	// BgmRefreshHours BGM 元数据后台刷新周期（小时）。
	BgmRefreshHours                int                   `json:"bgmRefreshHours"`
	// AI 设置：用云端大模型解析/过滤 RSS 标题
	AiEnabled   bool   `json:"aiEnabled"`
	AiProvider  string `json:"aiProvider"`
	AiApiKey    string `json:"aiApiKey"`
	AiBaseURL   string `json:"aiBaseURL"`
	AiModel     string `json:"aiModel"`
	AiPrompt    string `json:"aiPrompt"`
	// AiSubtitleSC 是否仅保留含简体中文字幕的资源（简中或简中双语视为满足）。
	AiSubtitleSC bool `json:"aiSubtitleSC"`
}

// renameRegStr 是遗留的剧集提取正则，保留作为 customEpisodeStr 的默认值以兼容配置。
const renameRegStr = `(.*|\[.*])(( - |Vol |[Ee][Pp]?)\d+(\.5)?( ?\(\d+\))?|【\d+(\.5)?】|\[\d+(\.5)?( ?\(\d+\))?( ?[vV]\d)?( ?END)?( ?完)?( ?FIN)?]|第\d+(\.5)?[话話集]( - END)?|^\[TOC].* \d+|^六四位元字幕组.*★\d+(\.5)?★)`

// RENAME_REG_STR 暴露剧集提取正则源码。
func RENAME_REG_STR() string { return renameRegStr }

// defaultAiPrompt 是 AI 标题解析的内置固定规则（不允许用户修改）。
// 由 provider 拼接到固定格式提示词的中间（输入输出格式不可变）。
const defaultAiPrompt = `规则：
1. 从标题中提取集数（episode）。可能是 "S01E03"、"第03话"、"03"、"Vol.3"、"EP3" 等格式，也可能是 "[03]" 或 "03" 的特别篇（如 3.5、06.5）。
2. 提取分辨率（resolution）：1080P、720P、2160P 等；没有则返回 "none"。
3. 提取字幕组（subgroup）：通常是标题开头的方括号内容，如 [ANi]、[喵萌奶茶屋]；没有则返回 ""。
4. 提取剧名（title）：去掉字幕组、集数、分辨率、编码等信息后的纯剧名。
5. 如果某个标题无法判断集数，episode 返回 0，isSpecial 返回 false。
6. 提取字幕嵌入方式（subtitleEmbed）：内封（内封字幕/内封简繁）/内嵌（硬字幕/内嵌字幕）/外挂（外挂字幕）；没有明确标识返回 ""。
7. 提取视频编码（videoCodec）：HEVC 或 x265、AVC 或 x264 等；没有返回 ""。
8. 提取压制源（source）：BD、BDRip、WebRip、Web、TV、RAW 等；没有返回 ""。
9. 提取色深（colorDepth）：10bit、8bit；没有返回 ""。
10. 提取字幕语言（subtitleLang）：从标题里的字幕语言标记提取，如 简繁日、简日、简、繁、日、英；简中可写"简"，繁中可写"繁"；没有返回 ""。`

// DEFAULT_AI_PROMPT 暴露默认 AI 要求。
func DEFAULT_AI_PROMPT() string { return defaultAiPrompt }

// defaultNotificationTemplate 默认全局通知模板（对齐上游 ani-rss ConfigUtil）。
// 去掉上游依赖下载服务/集标题的 ${downloadPath} 与 ${episodeTitle}（当前未接入）。
const defaultNotificationTemplate = `${emoji}${emoji}${emoji}
事件类型: ${action}
标题: ${title}
评分: ${score}
BGM: ${bgmUrl}
季: ${season}
集: ${episode}
字幕组: ${subgroup}
进度: ${currentEpisodeNumber}/${totalEpisodeNumber}
首播:  ${year}年${month}月${date}日
事件: ${text}
${emoji}${emoji}${emoji}`

// DefaultConfig 返回与遗留 ConfigUtil 静态块一致的默认配置。
func DefaultConfig() *Config {
	return &Config{
		DownloadToolType:            "115",
		Pan115Cookie:                "",
		DownloadRetry:               3,
		DownloadPathTemplate:        "番剧/${title}/Season ${season}",
		OvaDownloadPathTemplate:     "剧场版/${title}",
		RssSleepMinutes:             15,
		Rename:                      true,
		Rss:                         true,
		RssTimeout:                  20,
		Skip5:                       true,
		LogsMax:                     128,
		LogsLevel:                   "INFO",
		LogsFile:                    "",
		ProcrastinatingMasterOnly:   true,
		ProxyPort:                   8080,
		Login:                       Login{Username: "admin", Password: md5Hex("admin")},
		MultiLoginForbidden:         true,
		LoginEffectiveHours:         3,
		Exclude:                     []string{"720[Pp]", "\\d-\\d", "合集", "特别篇"},
		Omit:                        true,
		CustomEpisodeGroupIndex:     2,
		CustomEpisodeStr:            renameRegStr,
		ProcrastinatingDay:          14,
		UpdateTotalEpisodeNumber:    true,
		DownloadTimeout:             60,
		SortType:                    "SCORE",
		LimitLoginAttempts:          true,
		ReverseProxyTrustIpList:     []string{"127.0.0.1"},
		BgmApi:                      "https://api.bgm.tv",
		BgmRefreshHours:             6,
		AiEnabled:                   true,
		AiProvider:                  "deepseek",
		AiApiKey:                    "",
		AiBaseURL:                   "https://api.deepseek.com",
		AiModel:                     "deepseek-v4-flash",
		AiPrompt:                    defaultAiPrompt,
		AiSubtitleSC:                true,
		NotificationTemplate:        defaultNotificationTemplate,
	}
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}