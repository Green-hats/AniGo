package domain

// Item 是一条 RSS 条目。
type Item struct {
	Title         string   `json:"title"`
	ReName        string   `json:"reName"`
	Torrent       string   `json:"torrent"`
	InfoHash      string   `json:"infoHash"`
	Episode       float64  `json:"episode"`
	FormatSize    string   `json:"formatSize"`
	Length        int64    `json:"length"`
	HasDownloaded bool     `json:"hasDownloaded"`
	Master        bool     `json:"master"`
	Subgroup      string   `json:"subgroup"`
	Resolution    string   `json:"resolution"`
	PubDate       DateTime `json:"pubDate"`

	// 选版信号（AI 解析，同集多版本竞争用）
	SubtitleEmbed string `json:"subtitleEmbed"`
	VideoCodec    string `json:"videoCodec"`
	Source        string `json:"source"`
	ColorDepth    string `json:"colorDepth"`
	SubtitleLang  string `json:"subtitleLang"`
}

// Clone 返回 Item 的浅拷贝。
func (i *Item) Clone() *Item {
	if i == nil {
		return nil
	}
	c := *i
	return &c
}

// ListAni 是分组的订阅列表响应。
type ListAni struct {
	ReleaseDateList []string  `json:"releaseDateList"`
	WeekList        []WeekAni `json:"weekList"`
	Total           int       `json:"total"`
}

// WeekAni 按周标签分组 Ani 条目。
type WeekAni struct {
	WeekLabel string `json:"weekLabel"`
	Items     []*Ani `json:"items"`
}

// Log 是一条内存日志条目。
type Log struct {
	Message    string `json:"message"`
	Level      string `json:"level"`
	LoggerName string `json:"loggerName"`
	ThreadName string `json:"threadName"`
}

// About 携带 /api/about 的版本/更新信息。
type About struct {
	Version      string   `json:"version"`
	Latest       string   `json:"latest"`
	Update       bool     `json:"update"`
	DownloadURL  string   `json:"downloadUrl"`
	SHA256       string   `json:"sha256"`
	Size         int64    `json:"size"`
	FormatSize   string   `json:"formatSize"`
	MarkdownBody string   `json:"markdownBody"`
	Date         DateTime `json:"date"`
}

// Result 是全局 JSON 响应包装。
type Result struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
	T       int64       `json:"t"`
}

// NewResult 构建成功结果。
func NewResult(data interface{}) *Result {
	return &Result{Code: 200, Message: "success", Data: data, T: NowMillis()}
}

// NewError 构建错误结果（code 500）。
func NewError(msg string) *Result {
	return &Result{Code: 500, Message: msg, T: NowMillis()}
}

// NewResultCode 构建自定义 code+message 的结果。
func NewResultCode(code int, msg string) *Result {
	return &Result{Code: code, Message: msg, T: NowMillis()}
}

// NewMessage 构建带自定义消息的成功结果。
func NewMessage(msg string) *Result {
	return &Result{Code: 200, Message: msg, T: NowMillis()}
}