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
	"video-translator/internal/logger"
	"video-translator/internal/text"
	"video-translator/models"
)

const (
	grokAPIEndpoint = "https://api.x.ai/v1/chat/completions"
	grokModel       = "grok-4-1-fast-non-reasoning" // Cheapest: $0.20/$0.50 per million tokens
)

var (
	grokWorkers   = 20 // Parallel API calls
	grokChunkSize = 20 // Subtitles per batch
	grokRetries   = config.DefaultMaxRetries
)

// GrokTranslationService handles translation via xAI's Grok API
// Uses grok-4-1-fast-non-reasoning for cost-effective translation
type GrokTranslationService struct {
	apiKey string
	client *http.Client
}

// NewGrokTranslationService creates a new Grok translation service
func NewGrokTranslationService(apiKey string) *GrokTranslationService {
	return &GrokTranslationService{
		apiKey: apiKey,
		client: &http.Client{Timeout: 2 * time.Minute},
	}
}

// grokJob represents a batch translation job
type grokJob struct {
	batchIndex int
	startIdx   int
	endIdx     int
}

// grokResult represents the result of a batch translation
type grokResult struct {
	batchIndex int
	translated []string
	err        error
}

// TranslateSubtitles translates subtitles using Grok API
func (s *GrokTranslationService) TranslateSubtitles(
	subs models.SubtitleList,
	sourceLang, targetLang string,
	onProgress func(current, total int),
) (models.SubtitleList, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("Grok API key is required. Get one at https://console.x.ai")
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

	// Deduplication: Build unique text list
	uniqueTextMap := make(map[string]int)
	var uniqueTexts []string
	subtitleToUnique := make([]int, total)

	for i, sub := range subs {
		txt := strings.TrimSpace(sub.Text)
		if txt == "" {
			subtitleToUnique[i] = -1
			continue
		}
		if idx, exists := uniqueTextMap[txt]; exists {
			subtitleToUnique[i] = idx
		} else {
			idx := len(uniqueTexts)
			uniqueTextMap[txt] = idx
			uniqueTexts = append(uniqueTexts, txt)
			subtitleToUnique[i] = idx
		}
	}

	uniqueCount := len(uniqueTexts)
	logger.LogInfo("Grok: %d subtitles → %d unique texts (%d%% dedupe), %d workers",
		total, uniqueCount, (total-uniqueCount)*100/total, grokWorkers)

	if uniqueCount == 0 {
		return translatedSubs, nil
	}

	// Create batch jobs
	var jobs []grokJob
	for i := 0; i < uniqueCount; i += grokChunkSize {
		end := i + grokChunkSize
		if end > uniqueCount {
			end = uniqueCount
		}
		jobs = append(jobs, grokJob{
			batchIndex: len(jobs),
			startIdx:   i,
			endIdx:     end,
		})
	}

	// Worker pool
	jobChan := make(chan grokJob, len(jobs))
	resultChan := make(chan grokResult, len(jobs))

	var progressMutex sync.Mutex
	completedCount := 0

	var wg sync.WaitGroup
	numWorkers := grokWorkers
	if len(jobs) < numWorkers {
		numWorkers = len(jobs)
	}

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				textsToTranslate := uniqueTexts[job.startIdx:job.endIdx]
				translated, err := s.translateBatchWithRetry(textsToTranslate, sourceLang, targetLang)
				resultChan <- grokResult{
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
	results := make(map[int]grokResult)
	for result := range resultChan {
		if result.err != nil {
			return nil, fmt.Errorf("Grok translation batch %d failed: %w", result.batchIndex, result.err)
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

	// Build translations array
	uniqueTranslations := make([]string, uniqueCount)
	for batchIdx, job := range jobs {
		result := results[batchIdx]
		for j := 0; j < len(result.translated) && job.startIdx+j < uniqueCount; j++ {
			idx := job.startIdx + j
			uniqueTranslations[idx] = text.Postprocess(result.translated[j])
		}
	}

	// Map back to all subtitles
	for i := 0; i < total; i++ {
		uniqueIdx := subtitleToUnique[i]
		if uniqueIdx >= 0 && uniqueIdx < len(uniqueTranslations) {
			translatedSubs[i].Text = uniqueTranslations[uniqueIdx]
		}
	}

	return translatedSubs, nil
}

func (s *GrokTranslationService) translateBatchWithRetry(texts []string, sourceLang, targetLang string) ([]string, error) {
	var lastErr error
	for attempt := 1; attempt <= grokRetries; attempt++ {
		translated, err := s.translateBatch(texts, sourceLang, targetLang)
		if err == nil {
			return translated, nil
		}
		lastErr = err

		if attempt < grokRetries {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return nil, fmt.Errorf("failed after %d retries: %w", grokRetries, lastErr)
}

func (s *GrokTranslationService) translateBatch(texts []string, sourceLang, targetLang string) ([]string, error) {
	srcName := text.GetLanguageName(sourceLang)
	tgtName := text.GetLanguageName(targetLang)

	inputText := strings.Join(texts, "\n|||SUBTITLE|||\n")

	prompt := fmt.Sprintf(`Translate the following subtitles from %s to %s.
The subtitles are separated by "|||SUBTITLE|||".
Return ONLY the translations, separated by "|||SUBTITLE|||", in the same order.
Keep the translations natural and conversational for video dubbing.
Do not add any explanations or extra text.

%s`, srcName, tgtName, inputText)

	reqBody := map[string]interface{}{
		"model": grokModel,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
		"max_tokens":  8192,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", grokAPIEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Grok API request failed: %w", err)
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
			return nil, fmt.Errorf("Grok API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("Grok API error (status %d): %s", resp.StatusCode, string(respBody))
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
		return nil, fmt.Errorf("no response from Grok")
	}

	translatedText := result.Choices[0].Message.Content
	cleaned := text.SplitByDelimiter(translatedText, "|||SUBTITLE|||")

	for len(cleaned) < len(texts) {
		cleaned = append(cleaned, texts[len(cleaned)])
	}

	return cleaned[:len(texts)], nil
}

// CheckAPIKey validates the API key is set
func (s *GrokTranslationService) CheckAPIKey() error {
	if s.apiKey == "" {
		return fmt.Errorf("Grok API key is not configured")
	}
	return nil
}
