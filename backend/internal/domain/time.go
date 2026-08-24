package domain

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

var loc = func() *time.Location {
	l, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("GMT+8", 8*3600)
	}
	return l
}()

// Loc 返回统一使用的 GMT+8 时区，用于所有日期格式化。
func Loc() *time.Location { return loc }

// Now 返回配置时区下的当前时间。
func Now() time.Time { return time.Now().In(loc) }

// NowMillis 返回当前 Unix 毫秒时间戳。
func NowMillis() int64 { return Now().UnixMilli() }

// DateTime 序列化为 yyyy-MM-dd HH:mm:ss（与 GsonStatic 日期格式一致）。
type DateTime time.Time

func (d DateTime) Time() time.Time { return time.Time(d) }

func (d DateTime) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(d).Format("2006-01-02 15:04:05") + `"`), nil
}

func (d *DateTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*d = DateTime(time.Time{})
		return nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02", time.RFC3339} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			*d = DateTime(t)
			return nil
		}
	}
	return nil
}

// Date 序列化为 yyyy-MM-dd（与 DateAdapter 一致）。
type Date time.Time

func (d Date) Time() time.Time { return time.Time(d) }

func (d Date) MarshalJSON() ([]byte, error) {
	t := time.Time(d)
	if t.IsZero() {
		return []byte(`""`), nil
	}
	return []byte(`"` + t.Format("2006-01-02") + `"`), nil
}

func (d *Date) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*d = Date(time.Time{})
		return nil
	}
	for _, layout := range []string{"2006-01-02", "2006", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			*d = Date(t)
			return nil
		}
	}
	return nil
}

// StrID 是字符串 ID，解码时也接受 JSON 数字
// （animes.garden API 返回数字 id；Gson 会将其强转为字符串）。
type StrID string

func (s *StrID) UnmarshalJSON(b []byte) error {
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch t := v.(type) {
	case string:
		*s = StrID(t)
	case float64:
		*s = StrID(strconv.FormatFloat(t, 'f', -1, 64))
	case json.Number:
		*s = StrID(t.String())
	default:
		*s = ""
	}
	return nil
}