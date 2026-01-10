package services

import "video-translator/internal/config"

// transcriptionSemaphore limits the total number of concurrent transcription processes
// across ALL videos being processed. This prevents CPU overload when batch processing.
//
// Without this limit, if maxParallelVideos=3 and each video has 4 chunks transcribed
// in parallel, we'd have 12 concurrent faster-whisper/whisper-cpp processes.
//
// With MaxConcurrentTranscriptions=7, we limit to 7 concurrent processes total,
// keeping CPU usage at ~60-70% instead of 100%.
var transcriptionSemaphore = make(chan struct{}, config.MaxConcurrentTranscriptions)

// AcquireTranscriptionSlot blocks until a transcription slot is available.
// Call this before starting a transcription process.
func AcquireTranscriptionSlot() {
	transcriptionSemaphore <- struct{}{}
}

// ReleaseTranscriptionSlot releases a transcription slot.
// Call this when a transcription process completes (use defer).
func ReleaseTranscriptionSlot() {
	<-transcriptionSemaphore
}
