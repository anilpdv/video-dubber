package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"unicode"
	"video-translator/models"
)

// Constants for OpenAI translation
const (
	openAITranslateRetries = 3 // Number of retry attempts for API calls
)

// Package-level HTTP client with connection pooling for OpenAI translation
var openaiTranslatorClient = &http.Client{
	Timeout: 2 * time.Minute,
	Transport: &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
	},
}

// TranslatorService uses Argos Translate (free, local, no API key)
type TranslatorService struct {
	pythonPath string
	sourceLang string
	targetLang string
}

func NewTranslatorService() *TranslatorService {
	// Find Python with argostranslate installed
	pythonPath := findPythonWithArgos()

	return &TranslatorService{
		pythonPath: pythonPath,
		sourceLang: "ru",
		targetLang: "en",
	}
}

// findPythonWithArgos searches for a Python installation that has argostranslate
func findPythonWithArgos() string {
	// Common Python paths to check
	pythonPaths := []string{
		"/opt/anaconda3/bin/python3",
		"/opt/homebrew/bin/python3",
		"/usr/local/bin/python3",
		"python3",
		"/usr/bin/python3",
	}

	for _, p := range pythonPaths {
		cmd := exec.Command(p, "-c", "import argostranslate.translate; print('ok')")
		if output, err := cmd.Output(); err == nil && strings.TrimSpace(string(output)) == "ok" {
			return p
		}
	}

	// Fall back to python3 and let CheckInstalled report the error
	return "python3"
}

// CheckInstalled verifies Argos Translate is installed
func (s *TranslatorService) CheckInstalled() error {
	script := `
import argostranslate.translate
print("ok")
`
	cmd := exec.Command(s.pythonPath, "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("argos translate not installed. Run: pip install argostranslate\nError: %s", string(output))
	}
	return nil
}

// CheckLanguagePackage verifies the required language package is installed
func (s *TranslatorService) CheckLanguagePackage(sourceLang, targetLang string) error {
	script := fmt.Sprintf(`
import argostranslate.translate
langs = argostranslate.translate.get_installed_languages()
source = next((l for l in langs if l.code == '%s'), None)
target = next((l for l in langs if l.code == '%s'), None)
if not source or not target:
    print("missing")
else:
    translation = source.get_translation(target)
    if translation:
        print("ok")
    else:
        print("missing")
`, sourceLang, targetLang)

	cmd := exec.Command(s.pythonPath, "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to check language package: %w", err)
	}

	if strings.TrimSpace(string(output)) != "ok" {
		return fmt.Errorf("language package %s→%s not installed. Install with:\npython3 -c \"import argostranslate.package; argostranslate.package.update_package_index(); pkg = next(p for p in argostranslate.package.get_available_packages() if p.from_code == '%s' and p.to_code == '%s'); argostranslate.package.install_from_path(pkg.download())\"",
			sourceLang, targetLang, sourceLang, targetLang)
	}
	return nil
}

// Translate translates a single text string using Argos Translate
func (s *TranslatorService) Translate(text, sourceLang, targetLang string) (string, error) {
	if text == "" {
		return "", nil
	}

	// Preprocess text for better translation quality
	text = preprocessText(text)
	if text == "" {
		return "", nil
	}

	// Escape text for Python
	escapedText := strings.ReplaceAll(text, "\\", "\\\\")
	escapedText = strings.ReplaceAll(escapedText, "'", "\\'")
	escapedText = strings.ReplaceAll(escapedText, "\n", "\\n")

	script := fmt.Sprintf(`
import argostranslate.translate
result = argostranslate.translate.translate('%s', '%s', '%s')
print(result)
`, escapedText, sourceLang, targetLang)

	cmd := exec.Command(s.pythonPath, "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("translation failed: %w\nOutput: %s", err, string(output))
	}

	// Postprocess translated text for better quality
	result := postprocessTranslation(strings.TrimSpace(string(output)))
	return result, nil
}

// TranslateSubtitles translates a list of subtitles while preserving timing
func (s *TranslatorService) TranslateSubtitles(subs models.SubtitleList, sourceLang, targetLang string) (models.SubtitleList, error) {
	return s.TranslateSubtitlesWithProgress(subs, sourceLang, targetLang, nil)
}

// translationChunkSize defines how many subtitles to translate in each batch
// Larger chunks = fewer Python process startups = faster overall
// 50 subtitles per batch reduces Python overhead by 5x compared to 10
const translationChunkSize = 50

// TranslateSubtitlesWithProgress translates subtitles with real-time progress updates
// Uses chunked batch translation to provide progress every N subtitles
func (s *TranslatorService) TranslateSubtitlesWithProgress(
	subs models.SubtitleList,
	sourceLang, targetLang string,
	onProgress func(current, total int),
) (models.SubtitleList, error) {
	if len(subs) == 0 {
		return subs, nil
	}

	total := len(subs)
	translatedSubs := make(models.SubtitleList, total)

	// Process in chunks for real-time progress updates
	for i := 0; i < total; i += translationChunkSize {
		end := i + translationChunkSize
		if end > total {
			end = total
		}

		// Extract texts for this chunk
		chunkTexts := make([]string, end-i)
		for j := i; j < end; j++ {
			chunkTexts[j-i] = subs[j].Text
		}

		// Translate this chunk (blocking but small, so UI stays responsive)
		translated, err := s.TranslateBatch(chunkTexts, sourceLang, targetLang)
		if err != nil {
			// Fall back to individual translation for this chunk
			for j := i; j < end; j++ {
				t, e := s.Translate(subs[j].Text, sourceLang, targetLang)
				if e != nil {
					return nil, fmt.Errorf("failed to translate subtitle %d: %w", j+1, e)
				}
				translatedSubs[j] = models.Subtitle{
					Index:     subs[j].Index,
					StartTime: subs[j].StartTime,
					EndTime:   subs[j].EndTime,
					Text:      t,
				}
				if onProgress != nil {
					onProgress(j+1, total)
				}
			}
			continue
		}

		// Store translated results from this chunk
		for j := 0; j < len(translated); j++ {
			idx := i + j
			translatedSubs[idx] = models.Subtitle{
				Index:     subs[idx].Index,
				StartTime: subs[idx].StartTime,
				EndTime:   subs[idx].EndTime,
				Text:      translated[j],
			}
		}

		// Report progress after each chunk completes
		if onProgress != nil {
			onProgress(end, total)
		}
	}

	return translatedSubs, nil
}

// translateIndividualWithProgress falls back to translating one subtitle at a time with progress
func (s *TranslatorService) translateIndividualWithProgress(
	subs models.SubtitleList,
	sourceLang, targetLang string,
	onProgress func(current, total int),
) (models.SubtitleList, error) {
	translatedSubs := make(models.SubtitleList, len(subs))

	for i, sub := range subs {
		translated, err := s.Translate(sub.Text, sourceLang, targetLang)
		if err != nil {
			return nil, fmt.Errorf("failed to translate subtitle %d: %w", i+1, err)
		}

		translatedSubs[i] = models.Subtitle{
			Index:     sub.Index,
			StartTime: sub.StartTime,
			EndTime:   sub.EndTime,
			Text:      translated,
		}
		if onProgress != nil {
			onProgress(i+1, len(subs))
		}
	}

	return translatedSubs, nil
}

// TranslateWithOpenAI uses GPT-4o-mini for fast, high-quality translation
// Cost: ~$0.50 for 5 hours of subtitles
func (s *TranslatorService) TranslateWithOpenAI(subs models.SubtitleList, sourceLang, targetLang, apiKey string, onProgress func(current, total int)) (models.SubtitleList, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}

	if len(subs) == 0 {
		return subs, nil
	}

	total := len(subs)
	translatedSubs := make(models.SubtitleList, total)

	// GPT-4o-mini has a context limit, so we batch in chunks of 50 subtitles
	const chunkSize = 50

	for i := 0; i < total; i += chunkSize {
		end := i + chunkSize
		if end > total {
			end = total
		}

		// Collect texts for this batch
		var textsToTranslate []string
		for j := i; j < end; j++ {
			textsToTranslate = append(textsToTranslate, subs[j].Text)
		}

		// Translate batch with OpenAI (with retry logic)
		translated, err := s.translateBatchWithOpenAIRetry(textsToTranslate, sourceLang, targetLang, apiKey)
		if err != nil {
			return nil, fmt.Errorf("OpenAI translation failed: %w", err)
		}

		// Store results
		for j := 0; j < len(translated) && i+j < total; j++ {
			idx := i + j
			translatedSubs[idx] = models.Subtitle{
				Index:     subs[idx].Index,
				StartTime: subs[idx].StartTime,
				EndTime:   subs[idx].EndTime,
				Text:      postprocessTranslation(translated[j]),
			}
		}

		// Report progress
		if onProgress != nil {
			onProgress(end, total)
		}
	}

	return translatedSubs, nil
}

// translateBatchWithOpenAI sends a batch of texts to GPT-4o-mini for translation
func (s *TranslatorService) translateBatchWithOpenAI(texts []string, sourceLang, targetLang, apiKey string) ([]string, error) {
	// Build the prompt
	langNames := map[string]string{
		"ru": "Russian", "en": "English", "de": "German", "fr": "French",
		"es": "Spanish", "it": "Italian", "pt": "Portuguese", "zh": "Chinese",
		"ja": "Japanese", "ko": "Korean", "ar": "Arabic", "hi": "Hindi",
	}

	srcName := langNames[sourceLang]
	if srcName == "" {
		srcName = sourceLang
	}
	tgtName := langNames[targetLang]
	if tgtName == "" {
		tgtName = targetLang
	}

	// Join texts with delimiter
	inputText := strings.Join(texts, "\n|||SUBTITLE|||\n")

	prompt := fmt.Sprintf(`Translate the following subtitles from %s to %s.
The subtitles are separated by "|||SUBTITLE|||".
Return ONLY the translations, separated by "|||SUBTITLE|||", in the same order.
Keep the translations natural and conversational for video dubbing.
Do not add any explanations or extra text.

%s`, srcName, tgtName, inputText)

	// Build request
	reqBody := map[string]interface{}{
		"model": "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
		"max_tokens":  4096,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make request
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Use shared HTTP client with connection pooling
	resp, err := openaiTranslatorClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("OpenAI API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("OpenAI API error: %s", string(respBody))
	}

	// Parse response
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no response from OpenAI")
	}

	// Split translated text back into individual subtitles
	translatedText := result.Choices[0].Message.Content
	translations := strings.Split(translatedText, "|||SUBTITLE|||")

	// Clean up and ensure we have the right number of translations
	var cleaned []string
	for _, t := range translations {
		t = strings.TrimSpace(t)
		if t != "" {
			cleaned = append(cleaned, t)
		}
	}

	// If we don't have enough translations, pad with originals
	for len(cleaned) < len(texts) {
		cleaned = append(cleaned, texts[len(cleaned)])
	}

	return cleaned[:len(texts)], nil
}

// translateBatchWithOpenAIRetry wraps translateBatchWithOpenAI with retry logic
func (s *TranslatorService) translateBatchWithOpenAIRetry(texts []string, sourceLang, targetLang, apiKey string) ([]string, error) {
	var lastErr error
	for attempt := 1; attempt <= openAITranslateRetries; attempt++ {
		translated, err := s.translateBatchWithOpenAI(texts, sourceLang, targetLang, apiKey)
		if err == nil {
			return translated, nil
		}
		lastErr = err

		// Exponential backoff before retry
		if attempt < openAITranslateRetries {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return nil, fmt.Errorf("failed after %d retries: %w", openAITranslateRetries, lastErr)
}

// TranslateBatch translates multiple texts at once (more efficient)
func (s *TranslatorService) TranslateBatch(texts []string, sourceLang, targetLang string) ([]string, error) {
	if len(texts) == 0 {
		return texts, nil
	}

	// Preprocess all texts before translation
	processedTexts := make([]string, len(texts))
	for i, text := range texts {
		processedTexts[i] = preprocessText(text)
	}

	// Create a Python script that translates all texts
	var builder strings.Builder
	builder.WriteString("import argostranslate.translate\n")
	builder.WriteString("texts = [\n")
	for _, text := range processedTexts {
		escaped := strings.ReplaceAll(text, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "'", "\\'")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		builder.WriteString(fmt.Sprintf("    '%s',\n", escaped))
	}
	builder.WriteString("]\n")
	builder.WriteString(fmt.Sprintf("for t in texts:\n    print(argostranslate.translate.translate(t, '%s', '%s'))\n    print('---SEPARATOR---')\n", sourceLang, targetLang))

	cmd := exec.Command(s.pythonPath, "-c", builder.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("batch translation failed: %w\nOutput: %s", err, string(output))
	}

	// Parse results and postprocess
	results := strings.Split(string(output), "---SEPARATOR---\n")
	translated := make([]string, 0, len(texts))
	for _, r := range results {
		r = strings.TrimSpace(r)
		if r != "" {
			// Postprocess each translated text
			translated = append(translated, postprocessTranslation(r))
		}
	}

	if len(translated) != len(texts) {
		// Fall back to individual translation
		translated = make([]string, len(texts))
		for i, text := range texts {
			t, err := s.Translate(text, sourceLang, targetLang)
			if err != nil {
				return nil, err
			}
			translated[i] = t
		}
	}

	return translated, nil
}

// GetSupportedLanguages returns languages supported by Argos Translate
func GetSupportedSourceLanguages() map[string]string {
	return map[string]string{
		"ru": "Russian",
		"en": "English",
		"de": "German",
		"fr": "French",
		"es": "Spanish",
		"it": "Italian",
		"pt": "Portuguese",
		"nl": "Dutch",
		"pl": "Polish",
		"ja": "Japanese",
		"zh": "Chinese",
		"ko": "Korean",
		"ar": "Arabic",
		"hi": "Hindi",
	}
}

func GetSupportedTargetLanguages() map[string]string {
	return map[string]string{
		"en": "English",
		"de": "German",
		"fr": "French",
		"es": "Spanish",
		"it": "Italian",
		"pt": "Portuguese",
		"nl": "Dutch",
		"pl": "Polish",
		"ja": "Japanese",
		"zh": "Chinese",
		"ru": "Russian",
	}
}

// InstallLanguagePackage installs a language package
func (s *TranslatorService) InstallLanguagePackage(sourceLang, targetLang string) error {
	script := fmt.Sprintf(`
import argostranslate.package
argostranslate.package.update_package_index()
packages = argostranslate.package.get_available_packages()
pkg = next((p for p in packages if p.from_code == '%s' and p.to_code == '%s'), None)
if pkg:
    argostranslate.package.install_from_path(pkg.download())
    print("ok")
else:
    print("not_found")
`, sourceLang, targetLang)

	cmd := exec.Command(s.pythonPath, "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install language package: %w\nOutput: %s", err, string(output))
	}

	if strings.TrimSpace(string(output)) != "ok" {
		return fmt.Errorf("language package %s→%s not available", sourceLang, targetLang)
	}

	return nil
}

// preprocessText cleans up text before translation for better accuracy
func preprocessText(text string) string {
	if text == "" {
		return ""
	}

	// Remove common filler sounds/words that don't translate well
	fillerRegex := regexp.MustCompile(`(?i)\b(uh|um|er|ah|hmm|erm)\b`)
	text = fillerRegex.ReplaceAllString(text, "")

	// Normalize whitespace (multiple spaces, tabs, etc. to single space)
	whitespaceRegex := regexp.MustCompile(`\s+`)
	text = whitespaceRegex.ReplaceAllString(text, " ")

	// Remove leading/trailing whitespace
	text = strings.TrimSpace(text)

	// Simplify repeated punctuation (e.g., "..." or "!!!" to single)
	// Go regex doesn't support backreferences, so handle common cases
	text = regexp.MustCompile(`\.{2,}`).ReplaceAllString(text, ".")
	text = regexp.MustCompile(`!{2,}`).ReplaceAllString(text, "!")
	text = regexp.MustCompile(`\?{2,}`).ReplaceAllString(text, "?")
	text = regexp.MustCompile(`,{2,}`).ReplaceAllString(text, ",")

	return text
}

// postprocessTranslation cleans up translated text for better quality
func postprocessTranslation(text string) string {
	if text == "" {
		return ""
	}

	// Trim whitespace
	text = strings.TrimSpace(text)

	// Capitalize first letter if it's lowercase
	if len(text) > 0 {
		runes := []rune(text)
		if unicode.IsLower(runes[0]) {
			runes[0] = unicode.ToUpper(runes[0])
			text = string(runes)
		}
	}

	// Remove any double spaces that might have been introduced
	whitespaceRegex := regexp.MustCompile(`\s+`)
	text = whitespaceRegex.ReplaceAllString(text, " ")

	// Ensure sentence ends with proper punctuation
	if len(text) > 0 {
		lastChar := text[len(text)-1]
		if lastChar != '.' && lastChar != '!' && lastChar != '?' && lastChar != ',' {
			// Don't add punctuation if it's clearly a fragment or already has some
			if !strings.HasSuffix(text, "...") && !strings.HasSuffix(text, "-") {
				// text = text + "." // Optionally add period
			}
		}
	}

	return text
}
