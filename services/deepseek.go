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
