package rss

import (
	"encoding/xml"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/greenhats/anigo/internal/domain"
	"github.com/greenhats/anigo/internal/rename"
	"github.com/greenhats/anigo/internal/util"
)

var (
	regMagnet    = regexp.MustCompile(`^magnet:\?xt=urn:btih:(\w+)`)
	regEd2k      = regexp.MustCompile(`^ed2k://\|file\|([^|]+)\|(\d+)\|([A-Fa-f0-9]{32})\|/$`)
	regGuidHex   = regexp.MustCompile(`^([a-z]|[0-9])+$`)
	regSubgroup  = regexp.MustCompile(`^\{\{(.+)}}:(.+)$`)
)

// xmlItem 是原始 RSS 条目。
type xmlItem struct {
	Title     string     `xml:"title"`
	Link      string     `xml:"link"`
	GUID      string     `xml:"guid"`
	PubDate   string     `xml:"pubDate"`
	Enclosure *enclosure `xml:"enclosure"`
	NyaaSize  string     `xml:"nyaa:size"`
	NyaaHash  string     `xml:"nyaa:infoHash"`
	Torrent   *xmlTorrent `xml:"torrent"`
}

type enclosure struct {
	URL    string `xml:"url,attr"`
	Length string `xml:"length,attr"`
}

type xmlTorrent struct {
	InfoHash string `xml:"infohash"`
	PubDate  string `xml:"pubDate"`
}

type rssChannel struct {
	Items []xmlItem `xml:"item"`
}

type rssDocument struct {
	Channel rssChannel `xml:"channel"`
}

// GetRSS 抓取并校验 RSS XML 内容。
func GetRSS(cfg *domain.Config, rawURL string) (string, error) {
	timeout := cfg.RssTimeout
	if timeout <= 0 {
		timeout = 20
	}
	rawURL = normalizeURL(rawURL)
	req, err := util.NewRequest(cfg, "GET", rawURL)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", util.UserAgent())
	resp, err := util.ClientFor(cfg, timeout).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errors.New("http status " + resp.Status)
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if ct != "" && !strings.Contains(ct, "xml") {
		return "", errors.New("content type not xml: " + ct)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	body := strings.TrimSpace(string(b))
	if !strings.HasPrefix(body, "<") {
		return "", errors.New("xml error")
	}
	return body, nil
}

// normalizeURL 对查询参数做百分号编码，
// 让用户输入 RSS URL 中的未编码字符（如中文字幕组名）能正确发送。
func normalizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if q := u.Query(); len(q) > 0 {
		u.RawQuery = q.Encode()
		return u.String()
	}
	return rawURL
}

// Parse 解析 RSS XML 为条目，应用过滤与重命名。
// cfg 用于全局排除规则与 Skip5 等。
func Parse(cfg *domain.Config, ani *domain.Ani, rssURL, subgroupName, body string) []*domain.Item {
	var doc rssDocument
	if err := xml.Unmarshal([]byte(body), &doc); err != nil {
		return nil
	}
	globalExcludeList := cfg.Exclude

	var items []*domain.Item
	for i := len(doc.Channel.Items) - 1; i >= 0; i-- {
		raw := doc.Channel.Items[i]
		itemTitle := raw.Title
		torrent := ""
		length := "1"
		infoHash := ""
		formatSize := "0MiB"
		var pubDate time.Time

		if raw.Enclosure != nil {
			torrent = raw.Enclosure.URL
			if raw.Enclosure.Length != "" {
				length = raw.Enclosure.Length
			}
			if m := regMagnet.FindStringSubmatch(torrent); len(m) > 1 {
				infoHash = m[1]
			}
			if m := regEd2k.FindStringSubmatch(torrent); len(m) > 3 {
				infoHash = m[3]
			}
		}
		if regGuidHex.MatchString(raw.GUID) {
			infoHash = raw.GUID
		}
		if raw.NyaaHash != "" {
			infoHash = raw.NyaaHash
		}
		if raw.NyaaSize != "" {
			formatSize = raw.NyaaSize
		}
		if t, err := parseDate(raw.PubDate); err == nil {
			pubDate = t
		}
		if raw.Torrent != nil {
			if raw.Torrent.InfoHash != "" {
				infoHash = raw.Torrent.InfoHash
			}
			if pubDate.IsZero() {
				if t, err := parseDate(raw.Torrent.PubDate); err == nil {
					pubDate = t
				}
			}
		}
		if strings.HasSuffix(raw.Link, ".torrent") {
			torrent = raw.Link
		}

		if torrent == "" {
			continue
		}
		if infoHash == "" {
			infoHash = baseName(torrent)
		}
		infoHash = strings.ToLower(infoHash)
		if dec, err := url.PathUnescape(infoHash); err == nil {
			infoHash = dec
		}

		if formatSize == "0MiB" {
			if n := parseInt64Safe(length); n > 0 {
				formatSize = util.FormatSize(n)
			}
		}

		it := &domain.Item{
			Subgroup:   subgroupName,
			Episode:    1.0,
			Title:      itemTitle,
			ReName:     itemTitle,
			Torrent:    torrent,
			InfoHash:   infoHash,
			FormatSize: formatSize,
		}
		if !pubDate.IsZero() {
			it.PubDate = domain.DateTime(pubDate)
		}

		mapFn := func(s string) string {
			m := regSubgroup.FindStringSubmatch(s)
			if len(m) < 3 {
				return s
			}
			if m[1] == subgroupName {
				return m[2]
			}
			return ""
		}

		if len(ani.Exclude) > 0 {
			drop := false
			for _, ex := range ani.Exclude {
				mapped := mapFn(ex)
				if mapped == "" {
					continue
				}
				if regexpMatch(mapped, it.Title) {
					drop = true
					break
				}
			}
			if drop {
				continue
			}
		}
		if len(ani.Match) > 0 {
			drop := false
			for _, mt := range ani.Match {
				mapped := mapFn(mt)
				if mapped == "" {
					continue
				}
				if !regexpMatch(mapped, it.Title) {
					drop = true
					break
				}
			}
			if drop {
				continue
			}
		}
		if ani.GlobalExclude {
			drop := false
			for _, ex := range globalExcludeList {
				mapped := mapFn(ex)
				if mapped == "" {
					continue
				}
				if regexpMatch(mapped, it.Title) {
					drop = true
					break
				}
			}
			if drop {
				continue
			}
		}
		items = append(items, it)
	}

	var filtered []*domain.Item
	for _, it := range items {
		if rename.Rename(ani, it, cfg) {
			filtered = append(filtered, it)
		}
	}
	filtered = distinctByEpisodeKeepLast(filtered)
	return filtered
}

// DistinctItems 按剧集或 reName 去重。
func DistinctItems(items []*domain.Item, coexist bool) []*domain.Item {
	seen := map[string]bool{}
	var out []*domain.Item
	for _, it := range items {
		var key string
		if coexist {
			key = it.ReName
		} else {
			key = strconv.FormatFloat(it.Episode, 'f', -1, 64)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, it)
	}
	return out
}

func distinctByEpisodeKeepLast(items []*domain.Item) []*domain.Item {
	m := map[string]*domain.Item{}
	var order []string
	for _, it := range items {
		k := strconv.FormatFloat(it.Episode, 'f', -1, 64)
		if _, ok := m[k]; !ok {
			order = append(order, k)
		}
		m[k] = it
	}
	var out []*domain.Item
	for _, k := range order {
		out = append(out, m[k])
	}
	return out
}

func regexpMatch(pattern, s string) bool {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return strings.Contains(s, pattern)
	}
	return re.MatchString(s)
}

func parseInt64Safe(s string) int64 {
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	return n
}

func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("empty")
	}
	for _, layout := range []string{
		time.RFC1123Z, time.RFC1123, time.RFC822, time.RFC822Z,
		"2006-01-02 15:04:05", "2006-01-02", time.RFC3339,
	} {
		if t, err := time.ParseInLocation(layout, s, domain.Loc()); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("parse failed")
}

func baseName(torrent string) string {
	if idx := strings.LastIndexAny(torrent, "/\\"); idx >= 0 {
		return torrent[idx+1:]
	}
	return torrent
}