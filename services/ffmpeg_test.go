package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewFFmpegService(t *testing.T) {
	s := NewFFmpegService()
	if s == nil {
		t.Fatal("NewFFmpegService() returned nil")
	}
	if s.ffmpegPath == "" {
		t.Error("ffmpegPath should not be empty")
	}
}

func TestNewFFmpegServiceWithPath(t *testing.T) {
	path := "/custom/ffmpeg"
	s := NewFFmpegServiceWithPath(path)
	if s == nil {
		t.Fatal("NewFFmpegServiceWithPath() returned nil")
	}
	if s.ffmpegPath != path {
		t.Errorf("ffmpegPath = %q, want %q", s.ffmpegPath, path)
	}
}

func TestFFmpegService_CheckInstalled_NotFound(t *testing.T) {
	s := &FFmpegService{ffmpegPath: "/nonexistent/ffmpeg"}
	err := s.CheckInstalled()
	if err == nil {
		t.Error("CheckInstalled() should return error for nonexistent ffmpeg")
	}
}

func TestFFmpegService_ExtractAudio_InvalidInput(t *testing.T) {
	s := NewFFmpegService()
	tmpDir, err := os.MkdirTemp("", "ffmpeg_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, "output.wav")

	// Try to extract audio from nonexistent file
	err = s.ExtractAudio("/nonexistent/video.mp4", outputPath)
	if err == nil {
		t.Error("ExtractAudio() should return error for nonexistent input")
	}
}

func TestFFmpegService_ExtractAudioMP3_InvalidInput(t *testing.T) {
	s := NewFFmpegService()
	tmpDir, err := os.MkdirTemp("", "ffmpeg_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, "output.mp3")

	// Try to extract audio from nonexistent file
	err = s.ExtractAudioMP3("/nonexistent/video.mp4", outputPath)
	if err == nil {
		t.Error("ExtractAudioMP3() should return error for nonexistent input")
	}
}

func TestFFmpegService_MuxVideoAudio_InvalidInput(t *testing.T) {
	s := NewFFmpegService()
	tmpDir, err := os.MkdirTemp("", "ffmpeg_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, "output.mp4")

	// Try to mux nonexistent files
	err = s.MuxVideoAudio("/nonexistent/video.mp4", "/nonexistent/audio.wav", outputPath)
	if err == nil {
		t.Error("MuxVideoAudio() should return error for nonexistent input")
	}
}

func TestFFmpegService_MuxVideoAudioWithOriginal_InvalidInput(t *testing.T) {
	s := NewFFmpegService()
	tmpDir, err := os.MkdirTemp("", "ffmpeg_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, "output.mp4")

	// Try to mux nonexistent files
	err = s.MuxVideoAudioWithOriginal("/nonexistent/video.mp4", "/nonexistent/audio.wav", outputPath, 0.3)
	if err == nil {
		t.Error("MuxVideoAudioWithOriginal() should return error for nonexistent input")
	}
}

func TestFFmpegService_GetVideoDuration_InvalidInput(t *testing.T) {
	s := NewFFmpegService()

	// Try to get duration from nonexistent file
	duration, err := s.GetVideoDuration("/nonexistent/video.mp4")
	if err == nil {
		t.Error("GetVideoDuration() should return error for nonexistent input")
	}
	if duration != 0 {
		t.Errorf("GetVideoDuration() = %v, want 0 for error case", duration)
	}
}

func TestFFmpegService_ConcatAudioFiles_Empty(t *testing.T) {
	s := NewFFmpegService()
	tmpDir, err := os.MkdirTemp("", "ffmpeg_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, "output.wav")

	// Try to concat empty list
	err = s.ConcatAudioFiles([]string{}, outputPath)
	if err == nil {
		t.Error("ConcatAudioFiles() should return error for empty list")
	}
}

func TestFFmpegService_ConcatAudioFiles_InvalidInput(t *testing.T) {
	s := NewFFmpegService()
	tmpDir, err := os.MkdirTemp("", "ffmpeg_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, "output.wav")

	// Try to concat nonexistent files
	err = s.ConcatAudioFiles([]string{"/nonexistent/audio1.wav", "/nonexistent/audio2.wav"}, outputPath)
	if err == nil {
		t.Error("ConcatAudioFiles() should return error for nonexistent input")
	}
}

func TestFFmpegService_GenerateSilence_InvalidPath(t *testing.T) {
	s := &FFmpegService{ffmpegPath: "/nonexistent/ffmpeg"}

	// Try to generate silence with nonexistent ffmpeg
	err := s.GenerateSilence(1.0, "/tmp/silence.mp3")
	if err == nil {
		t.Error("GenerateSilence() should return error for nonexistent ffmpeg")
	}
}

func TestFFmpegService_OutputDirectoryCreation(t *testing.T) {
	s := NewFFmpegService()
	tmpDir, err := os.MkdirTemp("", "ffmpeg_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Try to extract to a nested directory that doesn't exist
	outputPath := filepath.Join(tmpDir, "nested", "dir", "output.wav")

	// This will fail because input doesn't exist, but directory should be created
	_ = s.ExtractAudio("/nonexistent/video.mp4", outputPath)

	// Check if nested directory was created
	nestedDir := filepath.Dir(outputPath)
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("ExtractAudio() should create output directory")
	}
}

func TestFFmpegService_ExtractAudioMP3_DirectoryCreation(t *testing.T) {
	s := NewFFmpegService()
	tmpDir, err := os.MkdirTemp("", "ffmpeg_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, "nested", "output.mp3")
	_ = s.ExtractAudioMP3("/nonexistent/video.mp4", outputPath)

	nestedDir := filepath.Dir(outputPath)
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("ExtractAudioMP3() should create output directory")
	}
}

func TestFFmpegService_MuxVideoAudio_DirectoryCreation(t *testing.T) {
	s := NewFFmpegService()
	tmpDir, err := os.MkdirTemp("", "ffmpeg_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, "nested", "output.mp4")
	_ = s.MuxVideoAudio("/nonexistent/video.mp4", "/nonexistent/audio.wav", outputPath)

	nestedDir := filepath.Dir(outputPath)
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("MuxVideoAudio() should create output directory")
	}
}

func TestFFmpegService_MuxVideoAudioWithOriginal_DirectoryCreation(t *testing.T) {
	s := NewFFmpegService()
	tmpDir, err := os.MkdirTemp("", "ffmpeg_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, "nested", "output.mp4")
	_ = s.MuxVideoAudioWithOriginal("/nonexistent/video.mp4", "/nonexistent/audio.wav", outputPath, 0.3)

	nestedDir := filepath.Dir(outputPath)
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("MuxVideoAudioWithOriginal() should create output directory")
	}
}

func TestFFmpegService_GenerateSilence_DirectoryCreation(t *testing.T) {
	s := &FFmpegService{ffmpegPath: "/nonexistent/ffmpeg"}
	tmpDir, err := os.MkdirTemp("", "ffmpeg_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, "nested", "silence.mp3")
	_ = s.GenerateSilence(1.0, outputPath)

	nestedDir := filepath.Dir(outputPath)
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("GenerateSilence() should create output directory")
	}
}

func TestFFmpegService_GetVideoDuration_NonexistentFile(t *testing.T) {
	s := NewFFmpegService()
	duration, err := s.GetVideoDuration("/nonexistent/video.mp4")
	if err == nil {
		t.Error("GetVideoDuration() should return error for nonexistent file")
	}
	if duration != 0 {
		t.Errorf("GetVideoDuration() should return 0 for nonexistent file, got %f", duration)
	}
}

func TestFFmpegService_ConcatAudioFiles_SingleFile(t *testing.T) {
	s := NewFFmpegService()
	tmpDir, err := os.MkdirTemp("", "ffmpeg_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, "output.wav")
	// Try to concat single nonexistent file
	err = s.ConcatAudioFiles([]string{"/nonexistent/audio.wav"}, outputPath)
	if err == nil {
		t.Error("ConcatAudioFiles() should return error for nonexistent input")
	}
}
