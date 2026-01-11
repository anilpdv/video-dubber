package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"video-translator/internal/config"
	internalhttp "video-translator/internal/http"
	"video-translator/internal/logger"
	textutil "video-translator/internal/text"
	"video-translator/models"
)

// Use centralized constants from internal/config
var (
	openAITranslateRetries  = config.DefaultMaxRetries
	maxTranslationWorkers   = config.WorkersOpenAI
	argosTranslationWorkers = 4 // Reduced to 4 for ~50% CPU usage (each Python process uses 1 core)
	openAITranslationChunk  = config.ChunkSizeOpenAI
)

// Use shared HTTP client with connection pooling
var openaiTranslatorClient = internalhttp.OpenAIClient

// translationJob represents a batch of texts to translate
type translationJob struct {
	batchIdx int
	texts    []string
}

// translationResult contains the result of translating a batch
type translationResult struct {
	batchIdx     int
	translations []string
	err          error
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
		ctx, cancel := context.WithTimeout(context.Background(), config.ExecTimeoutPython)
		cmd := exec.CommandContext(ctx, p, "-c", "import argostranslate.translate; print('ok')")
		output, err := cmd.Output()
		cancel()
		if err == nil && strings.TrimSpace(string(output)) == "ok" {
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
	ctx, cancel := context.WithTimeout(context.Background(), config.ExecTimeoutPython)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.pythonPath, "-c", script)
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

	ctx, cancel := context.WithTimeout(context.Background(), config.ExecTimeoutPython)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.pythonPath, "-c", script)
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
	text = textutil.Preprocess(text)
	if text == "" {
		return "", nil
	}

	// Escape text for Python using optimized single-pass function
	escapedText := textutil.EscapeForPython(text)

	script := fmt.Sprintf(`
import argostranslate.translate
result = argostranslate.translate.translate('%s', '%s', '%s')
print(result)
`, escapedText, sourceLang, targetLang)

	ctx, cancel := context.WithTimeout(context.Background(), config.ExecTimeoutPython)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.pythonPath, "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("translation failed: %w\nOutput: %s", err, string(output))
	}

	// Postprocess translated text for better quality
	result := textutil.Postprocess(strings.TrimSpace(string(output)))
	return result, nil
}

// TranslateSubtitles translates a list of subtitles while preserving timing
func (s *TranslatorService) TranslateSubtitles(subs models.SubtitleList, sourceLang, targetLang string) (models.SubtitleList, error) {
	return s.TranslateSubtitlesWithProgress(subs, sourceLang, targetLang, nil)
}

// translationChunkSize uses centralized config for batch size.
var translationChunkSize = config.ChunkSizeArgos

// TranslateSubtitlesWithProgress translates subtitles with parallel workers (KrillinAI pattern)
// Uses worker pool for 3-4x faster translation
func (s *TranslatorService) TranslateSubtitlesWithProgress(
	subs models.SubtitleList,
	sourceLang, targetLang string,
	onProgress func(current, total int),
) (models.SubtitleList, error) {
	logger.LogInfo("Argos Translate: %d subtitles (%s → %s) with %d workers", len(subs), sourceLang, targetLang, argosTranslationWorkers)

	if len(subs) == 0 {
		return subs, nil
	}

	total := len(subs)
	translatedSubs := make(models.SubtitleList, total)

	// Split into batches
	var batches [][]string
	var batchIndices []int // Track starting index of each batch
	for i := 0; i < total; i += translationChunkSize {
		end := i + translationChunkSize
		if end > total {
			end = total
		}
		chunkTexts := make([]string, end-i)
		for j := i; j < end; j++ {
			chunkTexts[j-i] = subs[j].Text
		}
		batches = append(batches, chunkTexts)
		batchIndices = append(batchIndices, i)
	}

	// Create job and result channels
	jobs := make(chan translationJob, len(batches))
	results := make(chan translationResult, len(batches))

	// Start worker pool (limited to reduce CPU usage)
	var wg sync.WaitGroup
	for w := 0; w < argosTranslationWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				// Acquire CPU slot to prevent system overload
				AcquireCPUSlot()
				translated, err := s.TranslateBatch(job.texts, sourceLang, targetLang)
				ReleaseCPUSlot()
				results <- translationResult{
					batchIdx:     job.batchIdx,
					translations: translated,
					err:          err,
				}
			}
		}()
	}

	// Send jobs to workers
	for i, batch := range batches {
		jobs <- translationJob{batchIdx: i, texts: batch}
	}
	close(jobs)

	// Close results channel when all workers are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results and track progress
	completedSubs := 0
	translatedBatches := make([][]string, len(batches))
	var firstErr error

	for result := range results {
		if result.err != nil && firstErr == nil {
			firstErr = result.err
			continue
		}
		if result.err == nil {
			translatedBatches[result.batchIdx] = result.translations
			completedSubs += len(result.translations)
			if onProgress != nil {
				onProgress(completedSubs, total)
			}
		}
	}

	if firstErr != nil {
		return nil, fmt.Errorf("translation failed: %w", firstErr)
	}

	// Reassemble results in order
	for batchIdx, translations := range translatedBatches {
		startIdx := batchIndices[batchIdx]
		for j, text := range translations {
			idx := startIdx + j
			translatedSubs[idx] = models.Subtitle{
				Index:     subs[idx].Index,
				StartTime: subs[idx].StartTime,
				EndTime:   subs[idx].EndTime,
				Text:      text,
			}
		}
	}

	return translatedSubs, nil
}

// TranslateWithOpenAI uses GPT-4o-mini for fast, high-quality translation with parallel workers
// Cost: ~$0.50 for 5 hours of subtitles
func (s *TranslatorService) TranslateWithOpenAI(subs models.SubtitleList, sourceLang, targetLang, apiKey string, onProgress func(current, total int)) (models.SubtitleList, error) {
	logger.LogInfo("OpenAI Translation: model=gpt-4o-mini %d subtitles (%s → %s) with %d workers", len(subs), sourceLang, targetLang, maxTranslationWorkers)

	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}

	if len(subs) == 0 {
		return subs, nil
	}

	total := len(subs)
	translatedSubs := make(models.SubtitleList, total)

	// Split into batches
	var batches [][]string
	var batchIndices []int
	for i := 0; i < total; i += openAITranslationChunk {
		end := i + openAITranslationChunk
		if end > total {
			end = total
		}
		var textsToTranslate []string
		for j := i; j < end; j++ {
			textsToTranslate = append(textsToTranslate, subs[j].Text)
		}
		batches = append(batches, textsToTranslate)
		batchIndices = append(batchIndices, i)
	}

	// Create job and result channels
	jobs := make(chan translationJob, len(batches))
	results := make(chan translationResult, len(batches))

	// Start worker pool (KrillinAI pattern)
	var wg sync.WaitGroup
	for w := 0; w < maxTranslationWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				translated, err := s.translateBatchWithOpenAIRetry(job.texts, sourceLang, targetLang, apiKey)
				results <- translationResult{
					batchIdx:     job.batchIdx,
					translations: translated,
					err:          err,
				}
			}
		}()
	}

	// Send jobs to workers
	for i, batch := range batches {
		jobs <- translationJob{batchIdx: i, texts: batch}
	}
	close(jobs)

	// Close results channel when all workers are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results and track progress
	completedSubs := 0
	translatedBatches := make([][]string, len(batches))
	var firstErr error

	for result := range results {
		if result.err != nil && firstErr == nil {
			firstErr = result.err
			continue
		}
		if result.err == nil {
			translatedBatches[result.batchIdx] = result.translations
			completedSubs += len(result.translations)
			if onProgress != nil {
				onProgress(completedSubs, total)
			}
		}
	}

	if firstErr != nil {
		return nil, fmt.Errorf("OpenAI translation failed: %w", firstErr)
	}

	// Reassemble results in order
	for batchIdx, translations := range translatedBatches {
		startIdx := batchIndices[batchIdx]
		for j, text := range translations {
			idx := startIdx + j
			if idx < total {
				translatedSubs[idx] = models.Subtitle{
					Index:     subs[idx].Index,
					StartTime: subs[idx].StartTime,
					EndTime:   subs[idx].EndTime,
					Text:      textutil.Postprocess(text),
				}
			}
		}
	}

	return translatedSubs, nil
}

// translateBatchWithOpenAI sends a batch of texts to GPT-4o-mini for translation
func (s *TranslatorService) translateBatchWithOpenAI(texts []string, sourceLang, targetLang, apiKey string) ([]string, error) {
	// Use centralized language mappings from internal/text package
	srcName := textutil.GetLanguageName(sourceLang)
	tgtName := textutil.GetLanguageName(targetLang)

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

// translateBatchWithOpenAIEmotions sends a batch of texts to GPT-4o-mini for translation with emotion detection
func (s *TranslatorService) translateBatchWithOpenAIEmotions(texts []string, sourceLang, targetLang, apiKey string) ([]emotionTranslation, error) {
	srcName := textutil.GetLanguageName(sourceLang)
	tgtName := textutil.GetLanguageName(targetLang)

	inputText := strings.Join(texts, "\n|||SUBTITLE|||\n")

	// Emotion-aware prompt for Fish Audio TTS
	prompt := fmt.Sprintf(`You are translating video subtitles from %s to %s for text-to-speech dubbing.

TASK: For each subtitle, provide BOTH an emotion tag AND the translation.

WHY EMOTIONS MATTER:
- This translation will be spoken by an AI voice (Fish Audio TTS)
- Fish Audio supports emotion tags like (happy), (sad), (excited) to make speech sound natural
- Without emotions, the dubbed audio sounds flat and robotic
- Matching emotions to content makes the video feel professionally dubbed

AVAILABLE EMOTIONS (pick the most fitting one):
- (happy) - cheerful, positive content
- (sad) - melancholic, disappointing news
- (excited) - energetic, enthusiastic announcements
- (calm) - neutral explanations, instructions
- (angry) - frustrated, complaints
- (surprised) - unexpected information
- (nervous) - uncertain, worried
- (confident) - assertive statements
- (curious) - questions, wondering
- (empathetic) - understanding, supportive

FORMAT: Return each line as: emotion|translated_text
Example: happy|This is amazing news!

RULES:
1. Use ONLY emotions from the list above (lowercase, no parentheses in output)
2. Use "calm" for neutral/informational content (most common)
3. Match emotion to the MEANING of the text, not just keywords
4. Keep translations natural and conversational
5. Separate entries with "|||SUBTITLE|||"

SUBTITLES TO TRANSLATE:
%s`, srcName, tgtName, inputText)

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

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := openaiTranslatorClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

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

	// Parse emotion|text format
	translatedText := result.Choices[0].Message.Content
	parts := textutil.SplitByDelimiter(translatedText, "|||SUBTITLE|||")

	translations := make([]emotionTranslation, 0, len(parts))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		emotion, translatedPart := parseEmotionText(part)
		if translatedPart == "" && i < len(texts) {
			translatedPart = texts[i]
		}

		translations = append(translations, emotionTranslation{
			emotion: emotion,
			text:    textutil.Postprocess(translatedPart),
		})
	}

	for len(translations) < len(texts) {
		translations = append(translations, emotionTranslation{
			emotion: "calm",
			text:    texts[len(translations)],
		})
	}

	return translations[:len(texts)], nil
}

// translateBatchWithOpenAIEmotionsRetry wraps translateBatchWithOpenAIEmotions with retry logic
func (s *TranslatorService) translateBatchWithOpenAIEmotionsRetry(texts []string, sourceLang, targetLang, apiKey string) ([]emotionTranslation, error) {
	var lastErr error
	for attempt := 1; attempt <= openAITranslateRetries; attempt++ {
		translated, err := s.translateBatchWithOpenAIEmotions(texts, sourceLang, targetLang, apiKey)
		if err == nil {
			return translated, nil
		}
		lastErr = err

		if attempt < openAITranslateRetries {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return nil, fmt.Errorf("failed after %d retries: %w", openAITranslateRetries, lastErr)
}

// TranslateWithOpenAIEmotions translates subtitles with emotion detection for Fish Audio TTS
func (s *TranslatorService) TranslateWithOpenAIEmotions(
	subs models.SubtitleList,
	sourceLang, targetLang, apiKey string,
	onProgress func(current, total int),
) (models.SubtitleList, error) {
	logger.LogInfo("OpenAI Translation (with emotions): model=gpt-4o-mini %d subtitles (%s → %s)", len(subs), sourceLang, targetLang)

	if apiKey == "" {
		return nil, fmt.Errorf("OpenAI API key is required")
	}

	if len(subs) == 0 {
		return subs, nil
	}

	total := len(subs)
	translatedSubs := make(models.SubtitleList, total)

	// Split into batches
	var batches [][]string
	var batchIndices []int
	for i := 0; i < total; i += openAITranslationChunk {
		end := i + openAITranslationChunk
		if end > total {
			end = total
		}
		var textsToTranslate []string
		for j := i; j < end; j++ {
			textsToTranslate = append(textsToTranslate, subs[j].Text)
		}
		batches = append(batches, textsToTranslate)
		batchIndices = append(batchIndices, i)
	}

	// Emotion result type
	type emotionResultBatch struct {
		batchIdx     int
		translations []emotionTranslation
		err          error
	}

	jobs := make(chan translationJob, len(batches))
	results := make(chan emotionResultBatch, len(batches))

	var wg sync.WaitGroup
	for w := 0; w < maxTranslationWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				translated, err := s.translateBatchWithOpenAIEmotionsRetry(job.texts, sourceLang, targetLang, apiKey)
				results <- emotionResultBatch{
					batchIdx:     job.batchIdx,
					translations: translated,
					err:          err,
				}
			}
		}()
	}

	for i, batch := range batches {
		jobs <- translationJob{batchIdx: i, texts: batch}
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(results)
	}()

	completedSubs := 0
	translatedBatches := make([][]emotionTranslation, len(batches))
	var firstErr error

	for result := range results {
		if result.err != nil && firstErr == nil {
			firstErr = result.err
			continue
		}
		if result.err == nil {
			translatedBatches[result.batchIdx] = result.translations
			completedSubs += len(result.translations)
			if onProgress != nil {
				onProgress(completedSubs, total)
			}
		}
	}

	if firstErr != nil {
		return nil, fmt.Errorf("OpenAI emotion translation failed: %w", firstErr)
	}

	// Reassemble results in order
	for batchIdx, translations := range translatedBatches {
		startIdx := batchIndices[batchIdx]
		for j, trans := range translations {
			idx := startIdx + j
			if idx < total {
				translatedSubs[idx] = models.Subtitle{
					Index:     subs[idx].Index,
					StartTime: subs[idx].StartTime,
					EndTime:   subs[idx].EndTime,
					Text:      trans.text,
					Emotion:   trans.emotion,
				}
			}
		}
	}

	return translatedSubs, nil
}

// TranslateBatch translates multiple texts at once (more efficient)
func (s *TranslatorService) TranslateBatch(texts []string, sourceLang, targetLang string) ([]string, error) {
	if len(texts) == 0 {
		return texts, nil
	}

	// Preprocess all texts before translation
	processedTexts := make([]string, len(texts))
	for i, text := range texts {
		processedTexts[i] = textutil.Preprocess(text)
	}

	// Create a Python script that translates all texts
	var builder strings.Builder
	builder.WriteString("import argostranslate.translate\n")
	builder.WriteString("texts = [\n")
	for _, text := range processedTexts {
		escaped := textutil.EscapeForPython(text)
		builder.WriteString(fmt.Sprintf("    '%s',\n", escaped))
	}
	builder.WriteString("]\n")
	builder.WriteString(fmt.Sprintf("for t in texts:\n    print(argostranslate.translate.translate(t, '%s', '%s'))\n    print('---SEPARATOR---')\n", sourceLang, targetLang))

	ctx, cancel := context.WithTimeout(context.Background(), config.ExecTimeoutPython)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.pythonPath, "-c", builder.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("batch translation failed: %w\nOutput: %s", err, string(output))
	}

	// Parse results and postprocess
	results := textutil.SplitByDelimiter(string(output), "---SEPARATOR---\n")
	translated := make([]string, 0, len(results))
	for _, r := range results {
		translated = append(translated, textutil.Postprocess(r))
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

// GetSupportedSourceLanguages returns languages supported by Argos Translate.
// Uses centralized language definitions from internal/text package.
func GetSupportedSourceLanguages() map[string]string {
	return textutil.SupportedSourceLanguages
}

// GetSupportedTargetLanguages returns languages for translation output.
// Uses centralized language definitions from internal/text package.
func GetSupportedTargetLanguages() map[string]string {
	return textutil.SupportedTargetLanguages
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

	ctx, cancel := context.WithTimeout(context.Background(), config.ExecTimeoutPython)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.pythonPath, "-c", script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to install language package: %w\nOutput: %s", err, string(output))
	}

	if strings.TrimSpace(string(output)) != "ok" {
		return fmt.Errorf("language package %s→%s not available", sourceLang, targetLang)
	}

	return nil
}

