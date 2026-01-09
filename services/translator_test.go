package services

import (
	"testing"
	"video-translator/models"
)

func TestNewTranslatorService(t *testing.T) {
	s := NewTranslatorService()
	if s == nil {
		t.Fatal("NewTranslatorService() returned nil")
	}
	if s.pythonPath == "" {
		t.Error("pythonPath should not be empty")
	}
	if s.sourceLang != "ru" {
		t.Errorf("sourceLang = %q, want 'ru'", s.sourceLang)
	}
	if s.targetLang != "en" {
		t.Errorf("targetLang = %q, want 'en'", s.targetLang)
	}
}

func TestTranslate_EmptyText(t *testing.T) {
	s := &TranslatorService{pythonPath: "python3"}
	result, err := s.Translate("", "ru", "en")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
}

func TestTranslateSubtitles_Empty(t *testing.T) {
	s := &TranslatorService{pythonPath: "python3"}
	subs := models.SubtitleList{}
	result, err := s.TranslateSubtitles(subs, "ru", "en")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}

func TestTranslateSubtitlesWithProgress_Empty(t *testing.T) {
	s := &TranslatorService{pythonPath: "python3"}
	subs := models.SubtitleList{}
	progressCalled := false

	result, err := s.TranslateSubtitlesWithProgress(subs, "ru", "en", func(current, total int) {
		progressCalled = true
	})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
	if progressCalled {
		t.Error("progress callback should not be called for empty list")
	}
}

func TestTranslateBatch_Empty(t *testing.T) {
	s := &TranslatorService{pythonPath: "python3"}
	result, err := s.TranslateBatch([]string{}, "ru", "en")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}

func TestGetSupportedSourceLanguages(t *testing.T) {
	langs := GetSupportedSourceLanguages()

	if len(langs) == 0 {
		t.Error("GetSupportedSourceLanguages() returned empty map")
	}

	// Check for expected languages
	expectedLangs := []string{"ru", "en", "de", "fr", "es"}
	for _, lang := range expectedLangs {
		if _, ok := langs[lang]; !ok {
			t.Errorf("expected language %q not found", lang)
		}
	}

	// Check Russian specifically
	if langs["ru"] != "Russian" {
		t.Errorf("langs['ru'] = %q, want 'Russian'", langs["ru"])
	}
}

func TestGetSupportedTargetLanguages(t *testing.T) {
	langs := GetSupportedTargetLanguages()

	if len(langs) == 0 {
		t.Error("GetSupportedTargetLanguages() returned empty map")
	}

	// Check for expected languages
	expectedLangs := []string{"en", "de", "fr", "es", "ru"}
	for _, lang := range expectedLangs {
		if _, ok := langs[lang]; !ok {
			t.Errorf("expected language %q not found", lang)
		}
	}

	// Check English specifically
	if langs["en"] != "English" {
		t.Errorf("langs['en'] = %q, want 'English'", langs["en"])
	}
}

func TestFindPythonWithArgos(t *testing.T) {
	// This tests that the function returns a non-empty path
	// It may return "python3" as fallback if argos isn't installed
	path := findPythonWithArgos()
	if path == "" {
		t.Error("findPythonWithArgos() returned empty string")
	}
}

func TestTranslatorService_CheckInstalled_NoPython(t *testing.T) {
	s := &TranslatorService{pythonPath: "/nonexistent/python3"}
	err := s.CheckInstalled()
	if err == nil {
		t.Error("CheckInstalled() should return error for nonexistent python")
	}
}

func TestTranslatorService_CheckLanguagePackage_NoPython(t *testing.T) {
	s := &TranslatorService{pythonPath: "/nonexistent/python3"}
	err := s.CheckLanguagePackage("ru", "en")
	if err == nil {
		t.Error("CheckLanguagePackage() should return error for nonexistent python")
	}
}

func TestInstallLanguagePackage_NoPython(t *testing.T) {
	s := &TranslatorService{pythonPath: "/nonexistent/python3"}
	err := s.InstallLanguagePackage("ru", "en")
	if err == nil {
		t.Error("InstallLanguagePackage() should return error for nonexistent python")
	}
}

func TestTranslate_NoPython(t *testing.T) {
	s := &TranslatorService{pythonPath: "/nonexistent/python3"}
	_, err := s.Translate("Hello", "en", "de")
	if err == nil {
		t.Error("Translate() should return error for nonexistent python")
	}
}

func TestTranslateBatch_NoPython(t *testing.T) {
	s := &TranslatorService{pythonPath: "/nonexistent/python3"}
	texts := []string{"Hello", "World"}
	_, err := s.TranslateBatch(texts, "en", "de")
	if err == nil {
		t.Error("TranslateBatch() should return error for nonexistent python")
	}
}

func TestTranslateSubtitlesWithProgress_NoPython(t *testing.T) {
	s := &TranslatorService{pythonPath: "/nonexistent/python3"}
	subs := models.SubtitleList{
		{Index: 1, Text: "Hello"},
		{Index: 2, Text: "World"},
	}

	_, err := s.TranslateSubtitlesWithProgress(subs, "en", "de", func(current, total int) {
		// Progress callback
	})

	if err == nil {
		t.Error("TranslateSubtitlesWithProgress() should return error for nonexistent python")
	}
}

func TestTranslateSubtitlesWithProgress_EmptyList(t *testing.T) {
	s := &TranslatorService{pythonPath: "python3"}
	subs := models.SubtitleList{}

	result, err := s.TranslateSubtitlesWithProgress(subs, "en", "de", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d items", len(result))
	}
}

func TestTranslatorService_NewWithDefaults(t *testing.T) {
	s := NewTranslatorService()

	if s.sourceLang != "ru" {
		t.Errorf("sourceLang = %q, want 'ru'", s.sourceLang)
	}
	if s.targetLang != "en" {
		t.Errorf("targetLang = %q, want 'en'", s.targetLang)
	}
	// pythonPath should be set (either found or fallback)
	if s.pythonPath == "" {
		t.Error("pythonPath should not be empty")
	}
}

func TestCheckLanguagePackage_NoPython(t *testing.T) {
	s := &TranslatorService{pythonPath: "/nonexistent/python3"}
	err := s.CheckLanguagePackage("ru", "en")
	if err == nil {
		t.Error("CheckLanguagePackage() should return error for nonexistent python")
	}
}
