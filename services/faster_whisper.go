package services

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"video-translator/internal/logger"
	"video-translator/internal/subtitle"
	"video-translator/internal/text"
	"video-translator/models"
)

// FasterWhisperService handles transcription using faster-whisper (4-10x faster than whisper.cpp)
type FasterWhisperService struct {
	pythonPath string
	model      string // tiny, base, small, medium, large-v2, large-v3
	device     string // auto, cuda, cpu
}

// FasterWhisper model options
var FasterWhisperModels = []string{
	"tiny",
	"base",
	"small",
	"medium",
	"large-v2",
	"large-v3",
}

// NewFasterWhisperService creates a new FasterWhisper transcription service
func NewFasterWhisperService(pythonPath, model, device string) *FasterWhisperService {
	if pythonPath == "" {
		pythonPath = "python3"
	}
	if model == "" {
		model = "base"
	}
	if device == "" {
		device = "auto"
	}
	return &FasterWhisperService{
		pythonPath: pythonPath,
		model:      model,
		device:     device,
	}
}

// CheckInstalled verifies faster-whisper is available
func (s *FasterWhisperService) CheckInstalled() error {
	// Check if faster-whisper Python package is installed
	script := `
import sys
try:
    import faster_whisper
    print("OK")
except ImportError:
    print("NOT_INSTALLED")
    sys.exit(1)
`
	cmd := exec.Command(s.pythonPath, "-c", script)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("faster-whisper not installed. Run: pip install faster-whisper")
	}
	if strings.TrimSpace(string(output)) != "OK" {
		return fmt.Errorf("faster-whisper not installed. Run: pip install faster-whisper")
	}
	return nil
}

// Transcribe converts audio to text with timestamps
func (s *FasterWhisperService) Transcribe(audioPath, language string) (models.SubtitleList, error) {
	return s.TranscribeWithProgress(audioPath, language, 0, nil)
}

// TranscribeWithProgress transcribes audio while reporting progress via callback
func (s *FasterWhisperService) TranscribeWithProgress(
	audioPath, language string,
	audioDuration float64,
	onProgress func(currentSec float64, percent int),
) (models.SubtitleList, error) {
	logger.LogInfo("FasterWhisper: model=%s device=%s lang=%s file=%s", s.model, s.device, language, filepath.Base(audioPath))

	if err := s.CheckInstalled(); err != nil {
		return nil, err
	}

	// Create output directory for SRT
	outputDir := filepath.Dir(audioPath)
	baseName := strings.TrimSuffix(filepath.Base(audioPath), filepath.Ext(audioPath))
	srtPath := filepath.Join(outputDir, baseName+"_faster.srt")

	// Python script to run faster-whisper with progress output
	script := fmt.Sprintf(`
import sys
from faster_whisper import WhisperModel

# Configure device
device = "%s"
if device == "auto":
    import torch
    device = "cuda" if torch.cuda.is_available() else "cpu"

compute_type = "float16" if device == "cuda" else "int8"

# Load model
print("LOADING_MODEL", file=sys.stderr, flush=True)
model = WhisperModel("%s", device=device, compute_type=compute_type)

# Transcribe
print("TRANSCRIBING", file=sys.stderr, flush=True)
segments, info = model.transcribe("%s", language="%s", beam_size=5)

# Write SRT format
def format_timestamp(seconds):
    hours = int(seconds // 3600)
    minutes = int((seconds %% 3600) // 60)
    secs = int(seconds %% 60)
    millis = int((seconds %% 1) * 1000)
    return f"{hours:02d}:{minutes:02d}:{secs:02d},{millis:03d}"

with open("%s", "w", encoding="utf-8") as f:
    for i, segment in enumerate(segments, 1):
        start = format_timestamp(segment.start)
        end = format_timestamp(segment.end)
        text = segment.text.strip()

        f.write(f"{i}\n")
        f.write(f"{start} --> {end}\n")
        f.write(f"{text}\n\n")

        # Print progress to stderr
        print(f"PROGRESS:{segment.end:.2f}", file=sys.stderr, flush=True)

print("DONE", file=sys.stderr, flush=True)
`, s.device, s.model, audioPath, language, srtPath)

	cmd := exec.Command(s.pythonPath, "-c", script)

	// Get stdout and stderr pipes
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start faster-whisper: %w", err)
	}

	// Drain stdout to prevent blocking
	go io.Copy(io.Discard, stdout)

	// Read stderr and capture all output for debugging
	var stderrLines []string
	scanner := bufio.NewScanner(stderr)

	for scanner.Scan() {
		line := scanner.Text()
		stderrLines = append(stderrLines, line)

		if matches := text.WhisperProgressRegex.FindStringSubmatch(line); len(matches) > 1 {
			currentSec, _ := strconv.ParseFloat(matches[1], 64)
			if audioDuration > 0 && onProgress != nil {
				percent := int((currentSec / audioDuration) * 25) + 15
				if percent > 40 {
					percent = 40
				}
				onProgress(currentSec, percent)
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		logger.LogError("faster-whisper failed. Output:\n%s", strings.Join(stderrLines, "\n"))
		return nil, fmt.Errorf("faster-whisper transcription failed: %w", err)
	}

	// Verify SRT file was created
	if _, err := os.Stat(srtPath); os.IsNotExist(err) {
		logger.LogError("faster-whisper did not create SRT file. Output:\n%s", strings.Join(stderrLines, "\n"))
		return nil, fmt.Errorf("faster-whisper did not create SRT output file at %s", srtPath)
	}

	// Parse the SRT file using internal/subtitle package
	internalSubs, err := subtitle.ParseSRTFile(srtPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SRT output: %w", err)
	}

	// Clean up the temporary SRT file
	os.Remove(srtPath)

	return models.FromInternalSubtitles(internalSubs), nil
}

// TranscribeToText transcribes audio to plain text (no timestamps)
func (s *FasterWhisperService) TranscribeToText(audioPath, language string) (string, error) {
	subs, err := s.Transcribe(audioPath, language)
	if err != nil {
		return "", err
	}

	var texts []string
	for _, sub := range subs {
		texts = append(texts, sub.Text)
	}

	return strings.Join(texts, " "), nil
}
