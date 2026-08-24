package domain

// IdDTO 是通用 id 请求体。
type IdDTO struct {
	ID string `json:"id"`
}

// RssToAniDTO 是 rssToAni 的输入。
type RssToAniDTO struct {
	URL      string `json:"url"`
	Type     string `json:"type"`
	BgmUrl   string `json:"bgmUrl"`
	Subgroup string `json:"subgroup"`
	Enable   bool   `json:"enable"`
}

// ImportAniDataDTO 是 importAni 的输入。
type ImportAniDataDTO struct {
	Filename string `json:"filename"`
	AniList  []*Ani `json:"aniList"`
	Conflict string `json:"conflict"` // REPLACE | SKIP
}

// PlayItem 是某个订阅云端目录中的一个可播放文件。
type PlayItem struct {
	Episode  int    `json:"episode"`  // 从文件名提取的集号，0 表示未知
	Filename string `json:"filename"`
	PickCode string `json:"pickCode"` // 云端文件 pickcode（代理取流用）
}
