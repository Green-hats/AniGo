package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLevelFiltering(t *testing.T) {
	l := New(64)
	l.SetLevel(LevelWarn)
	l.Info("test", "info message")
	l.Log(LevelWarn, "test", "warn message")
	l.Error("test", "error message")

	logs := l.List()
	if len(logs) != 2 {
		t.Fatalf("len = %d, want 2 (WARN/ERROR)", len(logs))
	}
	if logs[0].Level != LevelWarn || logs[1].Level != LevelError {
		t.Errorf("levels = %q, %q; want WARN, ERROR", logs[0].Level, logs[1].Level)
	}
}

func TestUnknownLevelFallsBackToInfo(t *testing.T) {
	l := New(8)
	l.SetLevel("BOGUS")
	l.Info("test", "x")
	if len(l.List()) != 1 {
		t.Errorf("unknown level should fall back to INFO and keep INFO logs")
	}
}

func TestRingCapacity(t *testing.T) {
	l := New(3)
	for i := 0; i < 10; i++ {
		l.Info("test", string(rune('a'+i)))
	}
	logs := l.List()
	if len(logs) != 3 {
		t.Fatalf("len = %d, want 3", len(logs))
	}
	if logs[len(logs)-1].Message != "j" {
		t.Errorf("last message = %q, want j", logs[len(logs)-1].Message)
	}
}

func TestFileSink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs", "anigo.log")
	l := New(16)
	if err := l.SetFile(path); err != nil {
		t.Fatalf("SetFile: %v", err)
	}
	l.Info("test", "hello")
	l.Error("test", "boom")
	l.Close()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "hello") || !strings.Contains(content, "boom") {
		t.Errorf("file content missing entries:\n%s", content)
	}
	if !strings.Contains(content, "[INFO]") || !strings.Contains(content, "[ERROR]") {
		t.Errorf("file content missing level markers:\n%s", content)
	}
}

func TestSetFileEmptyDisables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "anigo.log")
	l := New(8)
	if err := l.SetFile(path); err != nil {
		t.Fatalf("SetFile: %v", err)
	}
	if err := l.SetFile(""); err != nil {
		t.Fatalf("SetFile empty: %v", err)
	}
	l.Info("test", "after-disable")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "after-disable") {
		t.Errorf("log written after file disabled:\n%s", b)
	}
}