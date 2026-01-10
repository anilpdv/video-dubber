// Package limiter provides global resource limiters for CPU-intensive operations.
package limiter

import "video-translator/internal/config"

// cpuOperationSemaphore limits the total number of concurrent CPU-intensive operations
// across ALL videos being processed. This includes FFmpeg operations (silence generation,
// duration adjustment), local TTS (Piper), and local translation (Argos).
//
// Without this limit, if processing 3 videos simultaneously with 16 FFmpeg workers each,
// we'd have 48+ concurrent CPU-intensive processes causing 100% CPU usage.
//
// With MaxConcurrentCPUOperations=4, we limit to 4 concurrent CPU-heavy operations,
// keeping CPU usage at ~60-70% instead of 100%.
var cpuOperationSemaphore = make(chan struct{}, config.MaxConcurrentCPUOperations)

// AcquireCPUSlot blocks until a CPU operation slot is available.
// Call this before starting a CPU-intensive operation (FFmpeg, local TTS, local translation).
func AcquireCPUSlot() {
	cpuOperationSemaphore <- struct{}{}
}

// ReleaseCPUSlot releases a CPU operation slot.
// Call this when a CPU-intensive operation completes (use defer).
func ReleaseCPUSlot() {
	<-cpuOperationSemaphore
}
