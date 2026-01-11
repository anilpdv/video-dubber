package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"video-translator/internal/config"
	internalhttp "video-translator/internal/http"
	"video-translator/internal/logger"
	"video-translator/internal/text"
	"video-translator/models"
)

// Use centralized constants and endpoints
var (
	deepSeekEndpoint    = config.DeepSeekAPIEndpoint
	deepSeekModel       = config.DeepSeekModel
	maxTranslateWorkers = config.WorkersDeepSeek
	chunkSize           = config.ChunkSizeDeepSeek
	maxTranslateRetries = config.DefaultMaxRetries
)

// Use shared HTTP client with connection pooling
var deepseekClient = internalhttp.DeepSeekClient

// DeepSeekService handles translation via DeepSeek API (10x cheaper than GPT-4o-mini)
type DeepSeekService struct {
	apiKey string
}

// NewDeepSeekService creates a new DeepSeek translation service
func NewDeepSeekService(apiKey string) *DeepSeekService {
	return &DeepSeekService{
		apiKey: apiKey,
	}
}

// translateJob represents a batch translation job
type translateJob struct {
	batchIndex int
	startIdx   int
	endIdx     int
}

// translateResult represents the result of a batch translation
type translateResult struct {
	batchIndex int
	translated []string
	err        error
}

// TranslateSubtitles translates a list of subtitles using DeepSeek API
// Uses parallel processing with workers for faster translation
// Deduplicates repeated text to reduce API calls
// Cost: ~$0.04 for 5 hours of subtitles (10x cheaper than GPT-4o-mini)
func (s *DeepSeekService) TranslateSubtitles(
	subs models.SubtitleList,
	sourceLang, targetLang string,
	onProgress func(current, total int),
) (models.SubtitleList, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("DeepSeek API key is required")
	}

	if len(subs) == 0 {
		return subs, nil
	}

	total := len(subs)
	translatedSubs := make(models.SubtitleList, total)

	// Copy original subtitles (preserve timing, index)
	for i, sub := range subs {
		translatedSubs[i] = models.Subtitle{
			Index:     sub.Index,
			StartTime: sub.StartTime,
			EndTime:   sub.EndTime,
			Text:      sub.Text,
		}
	}

	// === DEDUPLICATION: Build unique text list ===
	uniqueTextMap := make(map[string]int) // text -> index in uniqueTexts
	var uniqueTexts []string
	subtitleToUnique := make([]int, total) // maps subtitle index -> unique text index

	for i, sub := range subs {
		text := strings.TrimSpace(sub.Text)
		if text == "" {
			subtitleToUnique[i] = -1 // Empty text, skip translation
			continue
		}
		if idx, exists := uniqueTextMap[text]; exists {
			subtitleToUnique[i] = idx // Reuse existing translation
		} else {
			idx := len(uniqueTexts)
			uniqueTextMap[text] = idx
			uniqueTexts = append(uniqueTexts, text)
			subtitleToUnique[i] = idx
		}
	}

	uniqueCount := len(uniqueTexts)
	logger.LogInfo("DeepSeek: %d subtitles → %d unique texts (%d%% dedupe), %d workers",
		total, uniqueCount, (total-uniqueCount)*100/total, maxTranslateWorkers)

	if uniqueCount == 0 {
		return translatedSubs, nil
	}

	// === BATCH TRANSLATION OF UNIQUE TEXTS ===
	// Create batch jobs for unique texts only
	var jobs []translateJob
	for i := 0; i < uniqueCount; i += chunkSize {
		end := i + chunkSize
		if end > uniqueCount {
			end = uniqueCount
		}
		jobs = append(jobs, translateJob{
			batchIndex: len(jobs),
			startIdx:   i,
			endIdx:     end,
		})
	}

	// Worker pool channels
	jobChan := make(chan translateJob, len(jobs))
	resultChan := make(chan translateResult, len(jobs))

	// Progress tracking
	var progressMutex sync.Mutex
	completedCount := 0

	// Start workers
	var wg sync.WaitGroup
	numWorkers := maxTranslateWorkers
	if len(jobs) < numWorkers {
		numWorkers = len(jobs)
	}

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				// Collect unique texts for this batch
				textsToTranslate := uniqueTexts[job.startIdx:job.endIdx]

				// Translate with retry
				translated, err := s.translateBatchWithRetry(textsToTranslate, sourceLang, targetLang)
				resultChan <- translateResult{
					batchIndex: job.batchIndex,
					translated: translated,
					err:        err,
				}
			}
		}()
	}

	// Send jobs to workers
	go func() {
		for _, job := range jobs {
			jobChan <- job
		}
		close(jobChan)
	}()

	// Close results channel when all workers done
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results (order-preserving via batchIndex)
	results := make(map[int]translateResult)
	for result := range resultChan {
		if result.err != nil {
			return nil, fmt.Errorf("DeepSeek translation batch %d failed: %w", result.batchIndex, result.err)
		}
		results[result.batchIndex] = result

		// Report progress (scale to total subtitles for UI)
		if onProgress != nil {
			progressMutex.Lock()
			batchSize := jobs[result.batchIndex].endIdx - jobs[result.batchIndex].startIdx
			completedCount += batchSize
			// Scale progress to total subtitle count
			scaledProgress := (completedCount * total) / uniqueCount
			onProgress(scaledProgress, total)
			progressMutex.Unlock()
		}
	}

	// === BUILD UNIQUE TRANSLATIONS ARRAY ===
	uniqueTranslations := make([]string, uniqueCount)
	for batchIdx, job := range jobs {
		result := results[batchIdx]
		for j := 0; j < len(result.translated) && job.startIdx+j < uniqueCount; j++ {
			idx := job.startIdx + j
			uniqueTranslations[idx] = cleanTranslation(result.translated[j])
		}
	}

	// === MAP BACK TO ALL SUBTITLES ===
	for i := 0; i < total; i++ {
		uniqueIdx := subtitleToUnique[i]
		if uniqueIdx >= 0 && uniqueIdx < len(uniqueTranslations) {
			translatedSubs[i].Text = uniqueTranslations[uniqueIdx]
		}
		// Empty text subtitles keep their original (empty) text
	}

	return translatedSubs, nil
}

// translateBatchWithRetry translates a batch with retry logic
func (s *DeepSeekService) translateBatchWithRetry(texts []string, sourceLang, targetLang string) ([]string, error) {
	var lastErr error
	for attempt := 1; attempt <= maxTranslateRetries; attempt++ {
		translated, err := s.translateBatch(texts, sourceLang, targetLang)
		if err == nil {
			return translated, nil
		}
		lastErr = err

		// Exponential backoff before retry
		if attempt < maxTranslateRetries {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return nil, fmt.Errorf("failed after %d retries: %w", maxTranslateRetries, lastErr)
}

// translateBatch sends a batch of texts to DeepSeek for translation
func (s *DeepSeekService) translateBatch(texts []string, sourceLang, targetLang string) ([]string, error) {
	return s.translateBatchStandard(texts, sourceLang, targetLang)
}

// translateBatchWithEmotions sends a batch of texts to DeepSeek for translation with emotion detection
func (s *DeepSeekService) translateBatchWithEmotions(texts []string, sourceLang, targetLang string) ([]emotionTranslation, error) {
	// Use centralized language mappings from internal/text package
	srcName := text.GetLanguageName(sourceLang)
	tgtName := text.GetLanguageName(targetLang)

	// Join texts with delimiter
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

	// Build request body
	reqBody := map[string]interface{}{
		"model": deepSeekModel,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
		"max_tokens":  config.DeepSeekMaxTokens,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", deepSeekEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := deepseekClient.Do(req)
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
			return nil, fmt.Errorf("DeepSeek API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("DeepSeek API error (status %d): %s", resp.StatusCode, string(respBody))
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
		return nil, fmt.Errorf("no response from DeepSeek")
	}

	// Parse emotion|text format
	translatedText := result.Choices[0].Message.Content
	parts := text.SplitByDelimiter(translatedText, "|||SUBTITLE|||")

	translations := make([]emotionTranslation, 0, len(parts))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Parse "emotion|text" format
		emotion, translatedPart := parseEmotionText(part)

		// Fallback to original if parsing fails
		if translatedPart == "" && i < len(texts) {
			translatedPart = texts[i]
		}

		translations = append(translations, emotionTranslation{
			emotion: emotion,
			text:    cleanTranslation(translatedPart),
		})
	}

	// Pad with originals if needed
	for len(translations) < len(texts) {
		translations = append(translations, emotionTranslation{
			emotion: "calm",
			text:    texts[len(translations)],
		})
	}

	return translations[:len(texts)], nil
}

// emotionTranslation holds a translation with its detected emotion
type emotionTranslation struct {
	emotion string
	text    string
}

// parseEmotionText parses "emotion|text" format, returns emotion and text
func parseEmotionText(s string) (string, string) {
	// Try to split by pipe
	if idx := strings.Index(s, "|"); idx > 0 && idx < 20 {
		emotion := strings.TrimSpace(s[:idx])
		text := strings.TrimSpace(s[idx+1:])

		// Validate emotion is one we support
		validEmotions := map[string]bool{
			"happy": true, "sad": true, "excited": true, "calm": true,
			"angry": true, "surprised": true, "nervous": true, "confident": true,
			"curious": true, "empathetic": true, "worried": true, "frustrated": true,
		}

		// Clean up emotion (remove parentheses if present)
		emotion = strings.TrimPrefix(emotion, "(")
		emotion = strings.TrimSuffix(emotion, ")")
		emotion = strings.ToLower(emotion)

		if validEmotions[emotion] && text != "" {
			return emotion, text
		}
	}

	// No valid emotion found, return calm as default
	return "calm", s
}

// translateBatchEmotionsWithRetry translates a batch with emotions and retry logic
func (s *DeepSeekService) translateBatchEmotionsWithRetry(texts []string, sourceLang, targetLang string) ([]emotionTranslation, error) {
	var lastErr error
	for attempt := 1; attempt <= maxTranslateRetries; attempt++ {
		translated, err := s.translateBatchWithEmotions(texts, sourceLang, targetLang)
		if err == nil {
			return translated, nil
		}
		lastErr = err

		// Exponential backoff before retry
		if attempt < maxTranslateRetries {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return nil, fmt.Errorf("failed after %d retries: %w", maxTranslateRetries, lastErr)
}

// TranslateSubtitlesWithEmotions translates subtitles and detects emotions for Fish Audio TTS
// This enables expressive speech synthesis by adding emotion tags like (happy), (sad), etc.
func (s *DeepSeekService) TranslateSubtitlesWithEmotions(
	subs models.SubtitleList,
	sourceLang, targetLang string,
	onProgress func(current, total int),
) (models.SubtitleList, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("DeepSeek API key is required")
	}

	if len(subs) == 0 {
		return subs, nil
	}

	total := len(subs)
	translatedSubs := make(models.SubtitleList, total)

	// Copy original subtitles (preserve timing, index)
	for i, sub := range subs {
		translatedSubs[i] = models.Subtitle{
			Index:     sub.Index,
			StartTime: sub.StartTime,
			EndTime:   sub.EndTime,
			Text:      sub.Text,
			Emotion:   "calm", // Default emotion
		}
	}

	// === DEDUPLICATION: Build unique text list ===
	uniqueTextMap := make(map[string]int)
	var uniqueTexts []string
	subtitleToUnique := make([]int, total)

	for i, sub := range subs {
		text := strings.TrimSpace(sub.Text)
		if text == "" {
			subtitleToUnique[i] = -1
			continue
		}
		if idx, exists := uniqueTextMap[text]; exists {
			subtitleToUnique[i] = idx
		} else {
			idx := len(uniqueTexts)
			uniqueTextMap[text] = idx
			uniqueTexts = append(uniqueTexts, text)
			subtitleToUnique[i] = idx
		}
	}

	uniqueCount := len(uniqueTexts)
	logger.LogInfo("DeepSeek (with emotions): %d subtitles → %d unique texts, %d workers",
		total, uniqueCount, maxTranslateWorkers)

	if uniqueCount == 0 {
		return translatedSubs, nil
	}

	// === BATCH TRANSLATION WITH EMOTIONS ===
	var jobs []translateJob
	for i := 0; i < uniqueCount; i += chunkSize {
		end := i + chunkSize
		if end > uniqueCount {
			end = uniqueCount
		}
		jobs = append(jobs, translateJob{
			batchIndex: len(jobs),
			startIdx:   i,
			endIdx:     end,
		})
	}

	// Emotion result channel
	type emotionResult struct {
		batchIndex int
		translated []emotionTranslation
		err        error
	}

	jobChan := make(chan translateJob, len(jobs))
	resultChan := make(chan emotionResult, len(jobs))

	var progressMutex sync.Mutex
	completedCount := 0

	var wg sync.WaitGroup
	numWorkers := maxTranslateWorkers
	if len(jobs) < numWorkers {
		numWorkers = len(jobs)
	}

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				textsToTranslate := uniqueTexts[job.startIdx:job.endIdx]
				translated, err := s.translateBatchEmotionsWithRetry(textsToTranslate, sourceLang, targetLang)
				resultChan <- emotionResult{
					batchIndex: job.batchIndex,
					translated: translated,
					err:        err,
				}
			}
		}()
	}

	go func() {
		for _, job := range jobs {
			jobChan <- job
		}
		close(jobChan)
	}()

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	results := make(map[int]emotionResult)
	for result := range resultChan {
		if result.err != nil {
			return nil, fmt.Errorf("DeepSeek emotion translation batch %d failed: %w", result.batchIndex, result.err)
		}
		results[result.batchIndex] = result

		if onProgress != nil {
			progressMutex.Lock()
			batchSize := jobs[result.batchIndex].endIdx - jobs[result.batchIndex].startIdx
			completedCount += batchSize
			scaledProgress := (completedCount * total) / uniqueCount
			onProgress(scaledProgress, total)
			progressMutex.Unlock()
		}
	}

	// === BUILD UNIQUE TRANSLATIONS ARRAY ===
	uniqueTranslations := make([]emotionTranslation, uniqueCount)
	for batchIdx, job := range jobs {
		result := results[batchIdx]
		for j := 0; j < len(result.translated) && job.startIdx+j < uniqueCount; j++ {
			idx := job.startIdx + j
			uniqueTranslations[idx] = result.translated[j]
		}
	}

	// === MAP BACK TO ALL SUBTITLES ===
	for i := 0; i < total; i++ {
		uniqueIdx := subtitleToUnique[i]
		if uniqueIdx >= 0 && uniqueIdx < len(uniqueTranslations) {
			translatedSubs[i].Text = uniqueTranslations[uniqueIdx].text
			translatedSubs[i].Emotion = uniqueTranslations[uniqueIdx].emotion
		}
	}

	return translatedSubs, nil
}

// translateBatchStandard sends a batch of texts to DeepSeek for translation (no emotions)
func (s *DeepSeekService) translateBatchStandard(texts []string, sourceLang, targetLang string) ([]string, error) {
	// Use centralized language mappings from internal/text package
	srcName := text.GetLanguageName(sourceLang)
	tgtName := text.GetLanguageName(targetLang)

	// Join texts with delimiter
	inputText := strings.Join(texts, "\n|||SUBTITLE|||\n")

	prompt := fmt.Sprintf(`Translate the following subtitles from %s to %s.
The subtitles are separated by "|||SUBTITLE|||".
Return ONLY the translations, separated by "|||SUBTITLE|||", in the same order.
Keep the translations natural and conversational for video dubbing.
Do not add any explanations or extra text.

%s`, srcName, tgtName, inputText)

	// Build request body (OpenAI-compatible format)
	reqBody := map[string]interface{}{
		"model": deepSeekModel,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
		"max_tokens":  8192, // Increased for larger batches
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Make request using shared client (connection pooling)
	req, err := http.NewRequest("POST", deepSeekEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := deepseekClient.Do(req)
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
			return nil, fmt.Errorf("DeepSeek API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("DeepSeek API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse response (OpenAI-compatible format)
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
		return nil, fmt.Errorf("no response from DeepSeek")
	}

	// Split translated text back into individual subtitles
	translatedText := result.Choices[0].Message.Content
	cleaned := text.SplitByDelimiter(translatedText, "|||SUBTITLE|||")

	// If we don't have enough translations, pad with originals
	for len(cleaned) < len(texts) {
		cleaned = append(cleaned, texts[len(cleaned)])
	}

	return cleaned[:len(texts)], nil
}

// cleanTranslation post-processes a translation using internal/text package.
func cleanTranslation(s string) string {
	return text.Postprocess(s)
}

// Translate translates a single text using DeepSeek
func (s *DeepSeekService) Translate(text, sourceLang, targetLang string) (string, error) {
	if text == "" {
		return "", nil
	}

	results, err := s.translateBatchWithRetry([]string{text}, sourceLang, targetLang)
	if err != nil {
		return "", err
	}

	if len(results) > 0 {
		return results[0], nil
	}
	return text, nil
}

// CheckAPIKey validates that the API key is set
func (s *DeepSeekService) CheckAPIKey() error {
	if s.apiKey == "" {
		return fmt.Errorf("DeepSeek API key is not configured")
	}
	return nil
}
