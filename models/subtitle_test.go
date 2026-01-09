package models

import (
	"testing"
	"time"
)

func TestSubtitle_Fields(t *testing.T) {
	sub := Subtitle{
		Index:     1,
		StartTime: time.Second,
		EndTime:   2 * time.Second,
		Text:      "Hello world",
	}

	if sub.Index != 1 {
		t.Errorf("expected Index 1, got %d", sub.Index)
	}
	if sub.StartTime != time.Second {
		t.Errorf("expected StartTime 1s, got %v", sub.StartTime)
	}
	if sub.EndTime != 2*time.Second {
		t.Errorf("expected EndTime 2s, got %v", sub.EndTime)
	}
	if sub.Text != "Hello world" {
		t.Errorf("expected Text 'Hello world', got %q", sub.Text)
	}
}

func TestSubtitleList_TotalDuration_Empty(t *testing.T) {
	var subs SubtitleList
	if got := subs.TotalDuration(); got != 0 {
		t.Errorf("TotalDuration() of empty list = %v, want 0", got)
	}
}

func TestSubtitleList_TotalDuration(t *testing.T) {
	subs := SubtitleList{
		{Index: 1, StartTime: 0, EndTime: 2 * time.Second, Text: "First"},
		{Index: 2, StartTime: 2 * time.Second, EndTime: 5 * time.Second, Text: "Second"},
		{Index: 3, StartTime: 5 * time.Second, EndTime: 10 * time.Second, Text: "Third"},
	}

	expected := 10 * time.Second
	if got := subs.TotalDuration(); got != expected {
		t.Errorf("TotalDuration() = %v, want %v", got, expected)
	}
}

func TestSubtitleList_GetText_Empty(t *testing.T) {
	var subs SubtitleList
	if got := subs.GetText(); got != "" {
		t.Errorf("GetText() of empty list = %q, want empty string", got)
	}
}

func TestSubtitleList_GetText(t *testing.T) {
	subs := SubtitleList{
		{Index: 1, Text: "Hello"},
		{Index: 2, Text: "world"},
		{Index: 3, Text: "test"},
	}

	expected := "Hello world test "
	if got := subs.GetText(); got != expected {
		t.Errorf("GetText() = %q, want %q", got, expected)
	}
}

func TestSubtitleList_SingleElement(t *testing.T) {
	subs := SubtitleList{
		{Index: 1, StartTime: 0, EndTime: 5 * time.Second, Text: "Only one"},
	}

	if got := subs.TotalDuration(); got != 5*time.Second {
		t.Errorf("TotalDuration() = %v, want 5s", got)
	}
	if got := subs.GetText(); got != "Only one " {
		t.Errorf("GetText() = %q, want 'Only one '", got)
	}
}

func TestSubtitleList_Len(t *testing.T) {
	subs := SubtitleList{
		{Index: 1, Text: "First"},
		{Index: 2, Text: "Second"},
	}

	if len(subs) != 2 {
		t.Errorf("len(subs) = %d, want 2", len(subs))
	}
}

func TestSubtitleList_Iteration(t *testing.T) {
	subs := SubtitleList{
		{Index: 1, Text: "A"},
		{Index: 2, Text: "B"},
		{Index: 3, Text: "C"},
	}

	count := 0
	for _, sub := range subs {
		count++
		if sub.Index != count {
			t.Errorf("expected Index %d, got %d", count, sub.Index)
		}
	}

	if count != 3 {
		t.Errorf("expected 3 iterations, got %d", count)
	}
}
