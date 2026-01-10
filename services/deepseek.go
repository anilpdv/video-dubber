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
	"video-translator/models"
)

const (
	deepSeekEndpoint     = "https://api.deepseek.com/v1/chat/completions"
	deepSeekModel        = "deepseek-chat"
	maxTranslateWorkers  = 5  // Increased from 3 for faster parallel translation
	chunkSize            = 75 // 75 subtitles per batch - fewer API calls
	maxTranslateRetries  = 3  // retry failed batches
)

// Package-level HTTP client with connection pooling (reused across requests)
var deepseekClient = &http.Client{
	Timeout: 2 * time.Minute,
	Transport: &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     90 * time.Second,
	},
}

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
// Uses parallel processing with 3 workers for ~3x speedup
// Cost: ~$0.05 for 5 hours of subtitles (10x cheaper than GPT-4o-mini)
func (s *DeepSeekService) TranslateSubtitles(
	subs models.SubtitleList,
	sourceLang, targetLang string,
	onProgress func(current, total int),
) (models.SubtitleList, error) {
	LogInfo("DeepSeek: translating %d subtitles (%s → %s) with %d workers", len(subs), sourceLang, targetLang, maxTranslateWorkers)

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
			Text:      sub.Text, // Will be overwritten with translation
		}
	}

	// Create batch jobs
	var jobs []translateJob
	for i := 0; i < total; i += chunkSize {
		end := i + chunkSize
		if end > total {
			end = total
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
				// Collect texts for this batch
				var textsToTranslate []string
				for j := job.startIdx; j < job.endIdx; j++ {
					textsToTranslate = append(textsToTranslate, subs[j].Text)
				}

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

		// Report progress
		if onProgress != nil {
			progressMutex.Lock()
			completedCount += jobs[result.batchIndex].endIdx - jobs[result.batchIndex].startIdx
			onProgress(completedCount, total)
			progressMutex.Unlock()
		}
	}

	// Assemble results in order
	for batchIdx, job := range jobs {
		result := results[batchIdx]
		for j := 0; j < len(result.translated) && job.startIdx+j < total; j++ {
			idx := job.startIdx + j
			translatedSubs[idx].Text = cleanTranslation(result.translated[j])
		}
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
	// Language name mapping
	langNames := map[string]string{
		"ru": "Russian", "en": "English", "de": "German", "fr": "French",
		"es": "Spanish", "it": "Italian", "pt": "Portuguese", "zh": "Chinese",
		"ja": "Japanese", "ko": "Korean", "ar": "Arabic", "hi": "Hindi",
		"nl": "Dutch", "pl": "Polish", "tr": "Turkish", "vi": "Vietnamese",
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

	// Build request body (OpenAI-compatible format)
	reqBody := map[string]interface{}{
		"model": deepSeekModel,
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

// cleanTranslation post-processes a translation
func cleanTranslation(text string) string {
	text = strings.TrimSpace(text)

	// Capitalize first letter if lowercase
	if len(text) > 0 && text[0] >= 'a' && text[0] <= 'z' {
		text = strings.ToUpper(text[:1]) + text[1:]
	}

	// Remove double spaces
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	return text
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
