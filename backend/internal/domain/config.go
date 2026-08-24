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
	NotifyDownloadEnd     NotificationStatusEnum = "DOWNLOAD_END"
	NotifyOmit            NotificationStatusEnum = "OMIT"
	NotifyError           NotificationStatusEnum = "ERROR"
	NotifyCompleted       NotificationStatusEnum = "COMPLETED"
	NotifyProcrastinating NotificationStatusEnum = "PROCRASTINATING"
)

// NotificationTypeEnum 通知类型枚举（按名称序列化）。
type NotificationTypeEnum string

const (
	NotifyEmbyRefresh NotificationTypeEnum = "EMBY_REFRESH"
	NotifyMail        NotificationTypeEnum = "MAIL"
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
	MailSMTPHost         string                 `json:"mailSMTPHost"`
	MailSMTPPort         int                    `json:"mailSMTPPort"`
	MailFrom             string                 `json:"mailFrom"`
	MailPassword         string                 `json:"mailPassword"`
	MailSSLEnable        bool                   `json:"mailSSLEnable"`
	MailTLSEnable        bool                   `json:"mailTLSEnable"`
	MailAddressee        string                 `json:"mailAddressee"`
	MailImage            bool                   `json:"mailImage"`
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
	EmbyHost             string                 `json:"embyHost"`
	EmbyApiKey           string                 `json:"embyApiKey"`
	EmbyRefreshViewIds   string                 `json:"embyRefreshViewIds"`
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
	MikanHost                      string                `json:"mikanHost"`
	TmdbApi                        string                `json:"tmdbApi"`
	TmdbApiKey                     string                `json:"tmdbApiKey"`
	TmdbImage                       string                `json:"tmdbImage"`
	TmdbAnime                       bool                  `json:"tmdbAnime"`
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
	TitleYear                      bool                  `json:"titleYear"`
	AutoDisabled                   bool                  `json:"autoDisabled"`
	Skip5                          bool                  `json:"skip5"`
	StandbyRss                     bool                  `json:"standbyRss"`
	Coexist                        bool                  `json:"coexist"`
	LogsMax                        int                   `json:"logsMax"`
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
	Tmdb                           bool                  `json:"tmdb"`
	TmdbId                         bool                  `json:"tmdbId"`
	TmdbIdPlexMode                 bool                  `json:"tmdbIdPlexMode"`
	TmdbOriginalName               bool                  `json:"tmdbOriginalName"`
	TmdbLanguage                   string                `json:"tmdbLanguage"`
	IpWhitelist                    bool                  `json:"ipWhitelist"`
	IpWhitelistStr                 string                `json:"ipWhitelistStr"`
	Omit                           bool                  `json:"omit"`
	BgmTokenType                   string                `json:"bgmTokenType"`
	BgmToken                       string                `json:"bgmToken"`
	BgmAppID                       string                `json:"bgmAppID"`
	BgmAppSecret                   string                `json:"bgmAppSecret"`
	BgmRefreshToken                string                `json:"bgmRefreshToken"`
	BgmRedirectUri                 string                `json:"bgmRedirectUri"`
	ApiKey                         string                `json:"apiKey"`
	DownloadNew                    bool                  `json:"downloadNew"`
	RenameTemplate                 string                `json:"renameTemplate"`
	RenameDelYear                  bool                  `json:"renameDelYear"`
	RenameDelTmdbId                bool                  `json:"renameDelTmdbId"`
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
	// AI 设置：用云端大模型解析/过滤 RSS 标题
	AiEnabled   bool   `json:"aiEnabled"`
	AiProvider  string `json:"aiProvider"`
	AiApiKey    string `json:"aiApiKey"`
	AiBaseURL   string `json:"aiBaseURL"`
	AiModel     string `json:"aiModel"`
}

// renameRegStr 是遗留的剧集提取正则，保留作为 customEpisodeStr 的默认值以兼容配置。
const renameRegStr = `(.*|\[.*])(( - |Vol |[Ee][Pp]?)\d+(\.5)?( ?\(\d+\))?|【\d+(\.5)?】|\[\d+(\.5)?( ?\(\d+\))?( ?[vV]\d)?( ?END)?( ?完)?( ?FIN)?]|第\d+(\.5)?[话話集]( - END)?|^\[TOC].* \d+|^六四位元字幕组.*★\d+(\.5)?★)`

// RENAME_REG_STR 暴露剧集提取正则源码。
func RENAME_REG_STR() string { return renameRegStr }

// DefaultConfig 返回与遗留 ConfigUtil 静态块一致的默认配置。
func DefaultConfig() *Config {
	return &Config{
		MikanHost:                   "https://mikanani.me",
		TmdbApi:                     "https://api.themoviedb.org",
		TmdbAnime:                   true,
		DownloadToolType:            "115",
		DownloadRetry:               3,
		DownloadPathTemplate:        "番剧/${title}/Season ${season}",
		OvaDownloadPathTemplate:     "剧场版/${title}",
		RssSleepMinutes:             15,
		Rename:                      true,
		Rss:                         true,
		RssTimeout:                  20,
		TitleYear:                   true,
		Skip5:                       true,
		LogsMax:                     128,
		ProcrastinatingMasterOnly:   true,
		ProxyPort:                   8080,
		Login:                       Login{Username: "admin", Password: md5Hex("admin")},
		MultiLoginForbidden:         true,
		LoginEffectiveHours:         3,
		Exclude:                     []string{"720[Pp]", "\\d-\\d", "合集", "特别篇"},
		Tmdb:                        true,
		TmdbLanguage:                "zh-CN",
		Omit:                        true,
		CustomEpisodeGroupIndex:     2,
		CustomEpisodeStr:            renameRegStr,
		ProcrastinatingDay:          14,
		DownloadTimeout:             60,
		SortType:                    "SCORE",
		LimitLoginAttempts:          true,
		ReverseProxyTrustIpList:     []string{"127.0.0.1"},
		BgmApi:                      "https://api.bgm.tv",
		AiEnabled:                   true,
		AiProvider:                  "deepseek",
		AiApiKey:                    defaultAiApiKey(),
		AiBaseURL:                   "https://api.deepseek.com",
		AiModel:                     "deepseek-v4-flash",
	}
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}