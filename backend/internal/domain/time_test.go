package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDateTimeMarshal(t *testing.T) {
	tm := time.Date(2026, 8, 25, 3, 30, 45, 0, Loc())
	d := DateTime(tm)
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != `"2026-08-25 03:30:45"` {
		t.Errorf("Marshal = %s", b)
	}
}

func TestDateTimeUnmarshal(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`"2026-08-25 03:30:45"`, "2026-08-25 03:30:45"},
		{`"2026-08-25"`, "2026-08-25 00:00:00"},
		{`"2026-08-25T03:30:45Z"`, "2026-08-25 03:30:45"},
		{`""`, "0001-01-01 00:00:00"},
		{`null`, "0001-01-01 00:00:00"},
	}
	for _, c := range cases {
		var d DateTime
		if err := json.Unmarshal([]byte(c.in), &d); err != nil {
			t.Errorf("Unmarshal(%s): %v", c.in, err)
			continue
		}
		if got := time.Time(d).Format("2006-01-02 15:04:05"); got != c.want {
			t.Errorf("Unmarshal(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDateTimeInvalidInput(t *testing.T) {
	var d DateTime
	// 无法解析的输入应返回 nil（不报错，保持零值）
	if err := json.Unmarshal([]byte(`"not-a-date"`), &d); err != nil {
		t.Errorf("无效输入应不报错, got %v", err)
	}
	if !time.Time(d).IsZero() {
		t.Error("无效输入应保持零值")
	}
}

func TestDateMarshal(t *testing.T) {
	tm := time.Date(2026, 8, 25, 10, 0, 0, 0, Loc())
	b, err := json.Marshal(Date(tm))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != `"2026-08-25"` {
		t.Errorf("Marshal = %s", b)
	}
	// 零值日期序列化为空串
	b2, _ := json.Marshal(Date(time.Time{}))
	if string(b2) != `""` {
		t.Errorf("零值日期 Marshal = %s, want \"\"", b2)
	}
}

func TestDateUnmarshal(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`"2026-08-25"`, "2026-08-25"},
		{`"2026"`, "2026-01-01"},
		{`"2026-08-25 03:30:45"`, "2026-08-25"},
		{`""`, ""},
		{`null`, ""},
	}
	for _, c := range cases {
		var d Date
		if err := json.Unmarshal([]byte(c.in), &d); err != nil {
			t.Errorf("Unmarshal(%s): %v", c.in, err)
			continue
		}
		got := ""
		if !time.Time(d).IsZero() {
			got = time.Time(d).Format("2006-01-02")
		}
		if got != c.want {
			t.Errorf("Unmarshal(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestStrIDUnmarshal(t *testing.T) {
	cases := []struct {
		in   string
		want StrID
	}{
		{`"123"`, "123"},
		{`"abc"`, "abc"},
		{`123`, "123"},
		{`123.5`, "123.5"},
		{`null`, ""},
	}
	for _, c := range cases {
		var s StrID
		if err := json.Unmarshal([]byte(c.in), &s); err != nil {
			t.Errorf("Unmarshal(%s): %v", c.in, err)
			continue
		}
		if s != c.want {
			t.Errorf("Unmarshal(%s) = %q, want %q", c.in, s, c.want)
		}
	}
}

func TestStrIDRoundTrip(t *testing.T) {
	var s StrID = "545008"
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(b) != `"545008"` {
		t.Errorf("Marshal = %s", b)
	}
}

func TestLocAndNow(t *testing.T) {
	_, offset := time.Now().In(Loc()).Zone()
	if offset != 8*3600 {
		t.Errorf("Loc 偏移 = %d, want 28800", offset)
	}
	n := Now()
	if n.Location() != Loc() {
		t.Error("Now 应返回 Loc 时区的时间")
	}
	if NowMillis() <= 0 {
		t.Error("NowMillis 应为正数")
	}
}