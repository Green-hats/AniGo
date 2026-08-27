package domain

// BgmInfo 是 Bangumi 番剧条目。
type BgmInfo struct {
	ID       string    `json:"id"`
	URL      string    `json:"url"`
	Name     string    `json:"name"`
	NameCn   string    `json:"nameCn"`
	Eps      int       `json:"eps"`            // wiki 解析的集数（已播出）
	TotalEpisodes int  `json:"totalEpisodes"`  // 数据库章节数（总集数）
	Date     DateTime  `json:"date"`
	Images   BgmImages `json:"images"`
	Season   int       `json:"season"`
	Platform string    `json:"platform"`
	Tags     []BgmTag  `json:"tags"`
	Rating   BgmRating `json:"rating"`
}

// BgmImages 是图片 URL 集合。
type BgmImages struct {
	Small  string `json:"small"`
	Grid   string `json:"grid"`
	Large  string `json:"large"`
	Medium string `json:"medium"`
	Common string `json:"common"`
}

// BgmTag 是番剧标签。
type BgmTag struct {
	Name      string `json:"name"`
	Count     int    `json:"count"`
	TotalCont int    `json:"totalCont"`
}

// BgmRating 是评分信息。
type BgmRating struct {
	Rank  int            `json:"rank"`
	Score float64        `json:"score"`
	Total int            `json:"total"`
	Count map[string]int `json:"count"`
}

// BgmEpisode 是一条番剧剧集。
type BgmEpisode struct {
	AirDate         Date   `json:"airdate"`
	Name            string `json:"name"`
	NameCn          string `json:"nameCn"`
	Duration        string `json:"duration"`
	Desc            string `json:"desc"`
	Ep              float64 `json:"ep"`
	Sort            float64 `json:"sort"`
	ID              int    `json:"id"`
	SubjectId       int    `json:"subjectId"`
	Comment         int    `json:"comment"`
	Type            int    `json:"type"`
	Disc            int    `json:"disc"`
	DurationSeconds int    `json:"durationSeconds"`
}

// GroupRegex 是字幕组过滤规则。
type GroupRegex struct {
	RegexList [][]GroupRegexItem `json:"regexList"`
	Tags      []string           `json:"tags"`
}

// GroupRegexItem 是一对 label+regex。
type GroupRegexItem struct {
	Label string `json:"label"`
	Regex string `json:"regex"`
}

// AnimeGarden 是 animes.garden 周列表。
type AnimeGarden struct {
	WeekLabel string               `json:"weekLabel"`
	Subjects  []AnimeGardenSubject `json:"subjects"`
}

// AnimeGardenSubject 是番剧条目。
type AnimeGardenSubject struct {
	ID         StrID    `json:"id"`
	Name       string   `json:"name"`
	Keywords   []string `json:"keywords"`
	ActivedAt  DateTime `json:"activedAt"`
	IsArchived bool     `json:"isArchived"`
	WeekLabel  string   `json:"weekLabel"`
	Exists     bool     `json:"exists"`
	Score      float64  `json:"score"`
	Cover      string   `json:"cover"`
}

// AnimeGardenGroup 是字幕组页面。
type AnimeGardenGroup struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	LastUpdatedAt DateTime          `json:"lastUpdatedAt"`
	Items         []AnimeGardenItem `json:"items"`
	Rss           string            `json:"rss"`
	BgmId         string            `json:"bgmId"`
	GroupRegex    GroupRegex        `json:"groupRegex"`
}

// AnimeGardenItem 是字幕组的一条发布。
type AnimeGardenItem struct {
	ID         StrID                 `json:"id"`
	Provider   string                `json:"provider"`
	ProviderId string                `json:"providerId"`
	Title      string                `json:"title"`
	Href       string                `json:"href"`
	Type       string                `json:"type"`
	Magnet     string                `json:"magnet"`
	Size       int64                 `json:"size"`
	FormatSize string                `json:"formatSize"`
	CreatedAt  DateTime              `json:"createdAt"`
	FetchedAt  DateTime              `json:"fetchedAt"`
	SubjectId  StrID                 `json:"subjectId"`
	Publisher  *AnimeGardenPublisher `json:"publisher"`
	Fansub     *AnimeGardenFansub    `json:"fansub"`
}

// AnimeGardenPublisher 是发布方信息。
type AnimeGardenPublisher struct {
	ID     StrID  `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

// AnimeGardenFansub 是字幕组信息。
type AnimeGardenFansub struct {
	ID     StrID  `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}