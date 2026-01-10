package media

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"video-translator/internal/config"
	"video-translator/internal/limiter"
	"video-translator/internal/subtitle"
)

// AudioAssembler handles building timed audio from speech segments.
// This is the common logic shared by all TTS services.
type AudioAssembler struct {
	ffmpeg   *FFmpegService
	tempDir  string
	segments []string
	lastEnd  time.Duration
}

// NewAudioAssembler creates a new audio assembler.
func NewAudioAssembler(ffmpeg *FFmpegService, tempDir string) *AudioAssembler {
	return &AudioAssembler{
		ffmpeg:   ffmpeg,
		tempDir:  tempDir,
		segments: make([]string, 0),
		lastEnd:  0,
	}
}

// Reset clears the assembler state for reuse.
func (a *AudioAssembler) Reset() {
	a.segments = make([]string, 0)
	a.lastEnd = 0
}

// AddSilence adds a silence segment for a gap.
func (a *AudioAssembler) AddSilence(duration time.Duration, index int) error {
	if duration <= config.SilenceGapThreshold {
		return nil
	}

	silencePath := filepath.Join(a.tempDir, fmt.Sprintf("silence_%04d.wav", index))
	if err := a.ffmpeg.GenerateSilence(duration.Seconds(), silencePath); err != nil {
		return fmt.Errorf("failed to generate silence: %w", err)
	}
	a.segments = append(a.segments, silencePath)
	return nil
}

// AddSpeechSegment adds a speech segment, optionally adjusting duration.
func (a *AudioAssembler) AddSpeechSegment(speechPath string, targetDuration time.Duration, index int) error {
	// Adjust duration if target is meaningful
	if targetDuration > 200*time.Millisecond {
		adjustedPath := filepath.Join(a.tempDir, fmt.Sprintf("adjusted_%04d.wav", index))
		if err := a.ffmpeg.AdjustAudioDuration(speechPath, adjustedPath, targetDuration.Seconds()); err == nil {
			speechPath = adjustedPath
		}
		// If adjustment fails, use original speech
	}

	a.segments = append(a.segments, speechPath)
	return nil
}

// AddPadding adds silence padding if audio is shorter than target window.
func (a *AudioAssembler) AddPadding(speechPath string, windowDuration time.Duration, index int) error {
	actualDuration, err := a.ffmpeg.GetAudioDuration(speechPath)
	if err != nil {
		return nil // Ignore errors, padding is optional
	}

	tolerance := config.AudioDurationTolerance.Seconds()
	if actualDuration < windowDuration.Seconds()-tolerance {
		paddingDuration := windowDuration.Seconds() - actualDuration
		paddingPath := filepath.Join(a.tempDir, fmt.Sprintf("padding_%04d.wav", index))
		if err := a.ffmpeg.GenerateSilence(paddingDuration, paddingPath); err == nil {
			a.segments = append(a.segments, paddingPath)
		}
	}
	return nil
}

// ProcessSubtitle handles a single subtitle entry.
// It adds silence for gaps, generates speech or silence, and maintains timing.
func (a *AudioAssembler) ProcessSubtitle(sub subtitle.Subtitle, speechPath string, index int) error {
	// Add silence for gap before subtitle
	if sub.StartTime > a.lastEnd {
		gap := sub.StartTime - a.lastEnd
		if err := a.AddSilence(gap, index); err != nil {
			return err
		}
	}

	// Handle the subtitle content
	if sub.IsEmpty() {
		// Empty subtitle - add silence for the duration
		duration := sub.EndTime - sub.StartTime
		if duration > 0 {
			silencePath := filepath.Join(a.tempDir, fmt.Sprintf("silence_sub_%04d.wav", index))
			if err := a.ffmpeg.GenerateSilence(duration.Seconds(), silencePath); err != nil {
				return fmt.Errorf("failed to generate silence for empty subtitle %d: %w", index+1, err)
			}
			a.segments = append(a.segments, silencePath)
		}
	} else if speechPath != "" {
		// Add the speech segment
		targetDuration := sub.EndTime - sub.StartTime
		if err := a.AddSpeechSegment(speechPath, targetDuration, index); err != nil {
			return err
		}

		// Add padding if needed
		if err := a.AddPadding(speechPath, targetDuration, index); err != nil {
			return err
		}
	}

	a.lastEnd = sub.EndTime
	return nil
}

// Concatenate combines all segments into final output.
func (a *AudioAssembler) Concatenate(outputPath string) error {
	if len(a.segments) == 0 {
		return fmt.Errorf("no segments to concatenate")
	}
	return a.ffmpeg.ConcatAudioFiles(a.segments, outputPath)
}

// GetSegments returns the current list of segment paths.
func (a *AudioAssembler) GetSegments() []string {
	return a.segments
}

// AddSegment manually adds a segment path.
func (a *AudioAssembler) AddSegment(path string) {
	a.segments = append(a.segments, path)
}

// ProcessSubtitles processes all subtitles using a map of speech paths.
// This is the main entry point for building the final audio.
func (a *AudioAssembler) ProcessSubtitles(subs subtitle.List, speechPaths map[int]string) error {
	for i, sub := range subs {
		speechPath := speechPaths[i]
		if err := a.ProcessSubtitle(sub, speechPath, i); err != nil {
			return err
		}
	}
	return nil
}

// AssembleFromSpeechPaths builds final audio from subtitles and pre-synthesized speech.
// This is a convenience method that combines ProcessSubtitles and Concatenate.
func (a *AudioAssembler) AssembleFromSpeechPaths(subs subtitle.List, speechPaths map[int]string, outputPath string) error {
	if err := a.ProcessSubtitles(subs, speechPaths); err != nil {
		return err
	}
	return a.Concatenate(outputPath)
}

// IsSpeechNeeded returns true if a subtitle needs TTS synthesis.
func IsSpeechNeeded(sub subtitle.Subtitle) bool {
	return !sub.IsEmpty() && strings.TrimSpace(sub.Text) != ""
}

// FilterSpeechNeeded returns indices of subtitles that need TTS.
func FilterSpeechNeeded(subs subtitle.List) []int {
	indices := make([]int, 0)
	for i, sub := range subs {
		if IsSpeechNeeded(sub) {
			indices = append(indices, i)
		}
	}
	return indices
}

// GapInfo represents a gap between subtitles that needs silence generation.
type GapInfo struct {
	Index    int
	Duration time.Duration
}

// AdjustmentJob represents a speech file that needs duration adjustment.
type AdjustmentJob struct {
	Index          int
	SpeechPath     string
	TargetDuration time.Duration
}

// AdjustDurationsParallel adjusts all speech durations in parallel.
// Returns a map of index -> adjusted file path.
func (a *AudioAssembler) AdjustDurationsParallel(
	speechPaths map[int]string,
	targetDurations map[int]time.Duration,
	workers int,
) map[int]string {
	if len(speechPaths) == 0 {
		return make(map[int]string)
	}

	// Build job list
	var jobs []AdjustmentJob
	for idx, path := range speechPaths {
		if duration, ok := targetDurations[idx]; ok && duration > 200*time.Millisecond {
			jobs = append(jobs, AdjustmentJob{
				Index:          idx,
				SpeechPath:     path,
				TargetDuration: duration,
			})
		}
	}

	if len(jobs) == 0 {
		return speechPaths // Return original paths if nothing to adjust
	}

	// Use dynamic worker count if not specified
	if workers <= 0 {
		workers = config.DynamicWorkerCount("silence-generation") // FFmpeg is similar workload
	}
	if workers > len(jobs) {
		workers = len(jobs)
	}

	adjustedPaths := make(map[int]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create a channel for jobs
	jobChan := make(chan AdjustmentJob, len(jobs))

	// Start workers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobChan {
				// Acquire global CPU slot to prevent overload
				limiter.AcquireCPUSlot()

				adjustedPath := filepath.Join(a.tempDir, fmt.Sprintf("adjusted_%04d.wav", job.Index))
				if err := a.ffmpeg.AdjustAudioDuration(job.SpeechPath, adjustedPath, job.TargetDuration.Seconds()); err == nil {
					mu.Lock()
					adjustedPaths[job.Index] = adjustedPath
					mu.Unlock()
				} else {
					// If adjustment fails, use original path
					mu.Lock()
					adjustedPaths[job.Index] = job.SpeechPath
					mu.Unlock()
				}

				limiter.ReleaseCPUSlot()
			}
		}()
	}

	// Send jobs
	for _, job := range jobs {
		jobChan <- job
	}
	close(jobChan)

	// Wait for completion
	wg.Wait()

	// Merge with original paths for any that weren't adjusted
	result := make(map[int]string)
	for idx, path := range speechPaths {
		if adjusted, ok := adjustedPaths[idx]; ok {
			result[idx] = adjusted
		} else {
			result[idx] = path
		}
	}

	return result
}

// IdentifyGaps analyzes subtitles and returns all gaps that need silence files.
func (a *AudioAssembler) IdentifyGaps(subs subtitle.List) []GapInfo {
	var gaps []GapInfo
	lastEnd := time.Duration(0)

	for i, sub := range subs {
		if sub.StartTime > lastEnd {
			gap := sub.StartTime - lastEnd
			if gap > config.SilenceGapThreshold {
				gaps = append(gaps, GapInfo{
					Index:    i,
					Duration: gap,
				})
			}
		}
		lastEnd = sub.EndTime
	}

	return gaps
}

// PrepareGapsParallel generates all silence files in parallel.
// This is called before processing subtitles to speed up assembly.
// Returns a map of index -> silence file path.
func (a *AudioAssembler) PrepareGapsParallel(gaps []GapInfo, workers int) map[int]string {
	if len(gaps) == 0 {
		return make(map[int]string)
	}

	// Use dynamic worker count if not specified
	if workers <= 0 {
		workers = config.DynamicWorkerCount("silence-generation")
	}
	if workers > len(gaps) {
		workers = len(gaps)
	}

	silencePaths := make(map[int]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Create a channel for jobs
	jobs := make(chan GapInfo, len(gaps))

	// Start workers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for gap := range jobs {
				// Acquire global CPU slot to prevent overload
				limiter.AcquireCPUSlot()

				silencePath := filepath.Join(a.tempDir, fmt.Sprintf("silence_gap_%04d.wav", gap.Index))
				if err := a.ffmpeg.GenerateSilence(gap.Duration.Seconds(), silencePath); err == nil {
					mu.Lock()
					silencePaths[gap.Index] = silencePath
					mu.Unlock()
				}

				limiter.ReleaseCPUSlot()
			}
		}()
	}

	// Send jobs
	for _, gap := range gaps {
		jobs <- gap
	}
	close(jobs)

	// Wait for completion
	wg.Wait()

	return silencePaths
}

// ProcessSubtitlesWithPreparedGaps processes subtitles using pre-generated silence files.
// This is faster than ProcessSubtitles for videos with many gaps.
func (a *AudioAssembler) ProcessSubtitlesWithPreparedGaps(
	subs subtitle.List,
	speechPaths map[int]string,
	silencePaths map[int]string,
) error {
	for i, sub := range subs {
		// Add pre-generated silence for gap before subtitle
		if silencePath, ok := silencePaths[i]; ok {
			a.segments = append(a.segments, silencePath)
		}

		// Handle the subtitle content
		if sub.IsEmpty() {
			// Empty subtitle - add silence for the duration
			duration := sub.EndTime - sub.StartTime
			if duration > 0 {
				silencePath := filepath.Join(a.tempDir, fmt.Sprintf("silence_sub_%04d.wav", i))
				if err := a.ffmpeg.GenerateSilence(duration.Seconds(), silencePath); err != nil {
					return fmt.Errorf("failed to generate silence for empty subtitle %d: %w", i+1, err)
				}
				a.segments = append(a.segments, silencePath)
			}
		} else if speechPath, ok := speechPaths[i]; ok && speechPath != "" {
			// Add the speech segment (already adjusted)
			a.segments = append(a.segments, speechPath)

			// Add padding if needed
			targetDuration := sub.EndTime - sub.StartTime
			if err := a.AddPadding(speechPath, targetDuration, i); err != nil {
				return err
			}
		}
	}
	return nil
}

// AssembleFromSpeechPathsParallel builds final audio with parallel gap and duration processing.
// This is an optimized version of AssembleFromSpeechPaths that parallelizes:
// 1. Gap silence generation
// 2. Audio duration adjustments (biggest win - 200 sequential FFmpeg calls → parallel)
func (a *AudioAssembler) AssembleFromSpeechPathsParallel(
	subs subtitle.List,
	speechPaths map[int]string,
	outputPath string,
) error {
	// Build target durations map for parallel adjustment
	targetDurations := make(map[int]time.Duration)
	for i, sub := range subs {
		if _, hasSpeech := speechPaths[i]; hasSpeech {
			targetDurations[i] = sub.EndTime - sub.StartTime
		}
	}

	// Run gap generation and duration adjustment in parallel
	var silencePaths map[int]string
	var adjustedPaths map[int]string
	var wg sync.WaitGroup

	// Parallel: identify and generate gap silences
	wg.Add(1)
	go func() {
		defer wg.Done()
		gaps := a.IdentifyGaps(subs)
		silencePaths = a.PrepareGapsParallel(gaps, 0)
	}()

	// Parallel: adjust all speech durations
	wg.Add(1)
	go func() {
		defer wg.Done()
		adjustedPaths = a.AdjustDurationsParallel(speechPaths, targetDurations, 0)
	}()

	wg.Wait()

	// Process subtitles with pre-generated gaps and pre-adjusted speech
	if err := a.ProcessSubtitlesWithPreparedGaps(subs, adjustedPaths, silencePaths); err != nil {
		return err
	}

	return a.Concatenate(outputPath)
}
