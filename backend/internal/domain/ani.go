package domain

import (
	"crypto/rand"
	"fmt"
)

func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NewUUID 返回随机 v4 UUID 字符串。
func NewUUID() string { return newUUID() }

// StandbyRss 是一个 Ani 的备用 RSS 源。
type StandbyRss struct {
	Label  string `json:"label"`
	URL    string `json:"url"`
	Offset int    `json:"offset"`
}

// Ani 是一个订阅，持久化为 ani.v2.json 的 JSON 数组。
type Ani struct {
	Sort                         int          `json:"sort"`
	ID                           string       `json:"id"`
	MikanTitle                   string       `json:"mikanTitle"`
	URL                          string       `json:"url"`
	StandbyRssList               []StandbyRss `json:"standbyRssList"`
	Title                        string       `json:"title"`
	JpTitle                      string       `json:"jpTitle"`
	Offset                       int          `json:"offset"`
	ReleaseDate                  Date         `json:"releaseDate"`
	Season                       int          `json:"season"`
	Cover                        string       `json:"cover"`
	Image                        string       `json:"image"`
	Subgroup                     string       `json:"subgroup"`
	Match                        []string     `json:"match"`
	Exclude                      []string     `json:"exclude"`
	GlobalExclude                bool         `json:"globalExclude"`
	Ova                          bool         `json:"ova"`
	Pinyin                       string       `json:"pinyin"`
	PinyinInitials               string       `json:"pinyinInitials"`
	Enable                       bool         `json:"enable"`
	CurrentEpisodeNumber         int          `json:"currentEpisodeNumber"`
	TotalEpisodeNumber           int          `json:"totalEpisodeNumber"`
	BgmAiredEps                  int          `json:"bgmAiredEps"`
	DownloadedEps                int          `json:"downloadedEps"`
	Type                         string       `json:"type"`
	BgmUrl                       string       `json:"bgmUrl"`
	CustomDownloadPath           bool         `json:"customDownloadPath"`
	CustomDownloadPathTemplate   string       `json:"customDownloadPathTemplate"`
	Score                        float64      `json:"score"`
	CustomEpisode                bool         `json:"customEpisode"`
	CustomEpisodeStr             string       `json:"customEpisodeStr"`
	CustomEpisodeGroupIndex      int          `json:"customEpisodeGroupIndex"`
	Omit                         bool         `json:"omit"`
	DownloadNew                  bool         `json:"downloadNew"`
	NotDownload                  []float64    `json:"notDownload"`
	Downloaded                   []float64    `json:"downloaded"`
	DownloadedHash               []string     `json:"downloadedHash"`
	Procrastinating              bool         `json:"procrastinating"`
	CustomRenameTemplateEnable   bool         `json:"customRenameTemplateEnable"`
	CustomRenameTemplate         string       `json:"customRenameTemplate"`
	CustomPriorityKeywordsEnable bool         `json:"customPriorityKeywordsEnable"`
	CustomPriorityKeywords       []string     `json:"customPriorityKeywords"`
	LastDownloadTime             int64        `json:"lastDownloadTime"`
	Message                      bool         `json:"message"`
	CustomTagsEnable             bool         `json:"customTagsEnable"`
	CustomTags                   []string     `json:"customTags"`
}

// Clone 返回 Ani 的浅拷贝。
func (a *Ani) Clone() *Ani {
	if a == nil {
		return nil
	}
	c := *a
	c.StandbyRssList = append([]StandbyRss(nil), a.StandbyRssList...)
	c.Match = append([]string(nil), a.Match...)
	c.Exclude = append([]string(nil), a.Exclude...)
	c.NotDownload = append([]float64(nil), a.NotDownload...)
	c.Downloaded = append([]float64(nil), a.Downloaded...)
	c.DownloadedHash = append([]string(nil), a.DownloadedHash...)
	c.CustomPriorityKeywords = append([]string(nil), a.CustomPriorityKeywords...)
	c.CustomTags = append([]string(nil), a.CustomTags...)
	return &c
}

// DefaultAni 返回 AniUtil.createAni() 使用的默认工厂值。
func DefaultAni() *Ani {
	return &Ani{
		ID:                      newUUID(),
		StandbyRssList:          []StandbyRss{},
		Offset:                  0,
		ReleaseDate:             Date(Now()),
		Enable:                  true,
		Score:                   0,
		LastDownloadTime:        0,
		Match:                   []string{},
		Exclude:                 []string{"720[Pp]", "\\d-\\d", "合集", "特别篇"},
		Omit:                    true,
		DownloadNew:             false,
		NotDownload:             []float64{},
		Downloaded:              []float64{},
		DownloadedHash:          []string{},
		Procrastinating:         true,
		CustomRenameTemplate:    "[${subgroup}] ${title} S${seasonFormat}E${episodeFormat}",
		Message:                 true,
		CustomEpisodeGroupIndex: 2,
	}
}