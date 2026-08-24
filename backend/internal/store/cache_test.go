package store

import (
	"sync"
	"testing"
	"time"
)

func TestCachePutGet(t *testing.T) {
	c := NewTTLCache()
	c.Put("k", "v", time.Minute)
	got, ok := c.Get("k")
	if !ok || got != "v" {
		t.Errorf("Get = %q, %v; want \"v\", true", got, ok)
	}
	if _, ok := c.Get("missing"); ok {
		t.Error("缺失键应返回 false")
	}
}

func TestCacheExpiry(t *testing.T) {
	c := NewTTLCache()
	c.Put("k", "v", 10*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Error("过期后 Get 应返回 false")
	}
	// 过期条目应被惰性清除
	if _, ok := c.m["k"]; ok {
		t.Error("过期条目应从底层 map 移除")
	}
}

func TestCacheContains(t *testing.T) {
	c := NewTTLCache()
	if c.Contains("k") {
		t.Error("空缓存 Contains 应为 false")
	}
	c.Put("k", "v", time.Minute)
	if !c.Contains("k") {
		t.Error("有效键 Contains 应为 true")
	}
}

func TestCacheClear(t *testing.T) {
	c := NewTTLCache()
	c.Put("a", "1", time.Minute)
	c.Put("b", "2", time.Minute)
	c.Clear()
	if n, _ := c.Size(); n != 0 {
		t.Errorf("Clear 后 Size = %d, want 0", n)
	}
	if c.Contains("a") || c.Contains("b") {
		t.Error("Clear 后不应包含任何键")
	}
}

func TestCacheSize(t *testing.T) {
	c := NewTTLCache()
	if n, b := c.Size(); n != 0 || b != 0 {
		t.Errorf("空缓存 Size = %d, %d", n, b)
	}
	c.Put("ab", "cd", time.Minute) // key 2 字节 + value 2 字节 = 4 字节
	if n, b := c.Size(); n != 1 || b != 4 {
		t.Errorf("Size = %d, %d; want 1, 4", n, b)
	}
}

func TestCacheOverwrite(t *testing.T) {
	c := NewTTLCache()
	c.Put("k", "old", time.Minute)
	c.Put("k", "new", time.Minute)
	got, ok := c.Get("k")
	if !ok || got != "new" {
		t.Errorf("覆盖后 Get = %q, %v", got, ok)
	}
	if n, _ := c.Size(); n != 1 {
		t.Errorf("覆盖不应增加条目数: %d", n)
	}
}

func TestCacheConcurrent(t *testing.T) {
	c := NewTTLCache()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := string(rune('a' + i%5))
			c.Put(key, "v", time.Minute)
			c.Get(key)
			c.Size()
			c.Contains(key)
		}(i)
	}
	wg.Wait()
	if n, _ := c.Size(); n != 5 {
		t.Errorf("Size = %d, want 5", n)
	}
}