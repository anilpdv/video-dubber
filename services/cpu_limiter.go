package services

import "video-translator/internal/limiter"

// AcquireCPUSlot blocks until a CPU operation slot is available.
// Call this before starting a CPU-intensive operation (FFmpeg, local TTS, local translation).
// This is a re-export from internal/limiter for use by services package.
func AcquireCPUSlot() {
	limiter.AcquireCPUSlot()
}

// ReleaseCPUSlot releases a CPU operation slot.
// Call this when a CPU-intensive operation completes (use defer).
// This is a re-export from internal/limiter for use by services package.
func ReleaseCPUSlot() {
	limiter.ReleaseCPUSlot()
}
