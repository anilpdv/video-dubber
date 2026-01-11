package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"video-translator/internal/logger"
	"video-translator/models"
	"video-translator/services"
	uicontainer "video-translator/ui/container"
	"video-translator/ui/layouts"
	appTheme "video-translator/ui/theme"
	"video-translator/ui/widgets"
)

// maxParallelVideos limits concurrent video processing to avoid CPU overload.
// Fixed at 2 to keep CPU usage reasonable when combined with parallel
// transcription, TTS, and FFmpeg operations within each video.
const maxParallelVideos = 2

// MainUI is the main application UI
type MainUI struct {
	window   fyne.Window
	jobs     []*models.TranslationJob
	config   *models.Config
	pipeline *services.Pipeline

	// UI Components
	sidebar           *widgets.SidebarNav
	fileListPanel     *uicontainer.FileListPanel
	progressPanel     *uicontainer.ProgressPanel
	settingsPanel     *uicontainer.SettingsPanel
	dependenciesPanel *uicontainer.DependenciesPanel
	bottomControls    *widgets.BottomControls
	mainContent       *fyne.Container

	// View containers for swapping
	translateView    *fyne.Container
	settingsView     fyne.CanvasObject
	dependenciesView fyne.CanvasObject
	contentArea      *fyne.Container

	// Current view
	currentView string
}

// NewMainUI creates the main application UI
func NewMainUI(w fyne.Window) *MainUI {
	config, err := models.LoadConfig()
	if err != nil {
		config = models.DefaultConfig()
	}

	ui := &MainUI{
		window:      w,
		jobs:        make([]*models.TranslationJob, 0),
		config:      config,
		pipeline:    services.NewPipeline(config),
		currentView: "translate",
	}

	// Set up progress callback
	ui.pipeline.SetProgressCallback(func(stage string, percent int, message string) {
		fyne.Do(func() {
			if ui.progressPanel != nil {
				ui.progressPanel.SetProgress(stage, percent)
				ui.progressPanel.SetStatus(message)
			}
		})
	})

	return ui
}

// Build creates the complete UI layout
func (ui *MainUI) Build() fyne.CanvasObject {
	// Create sidebar navigation
	ui.sidebar = widgets.NewSidebarNav([]widgets.NavItem{
		{ID: "translate", Icon: theme.MediaVideoIcon(), Label: "Translate"},
		{ID: "settings", Icon: theme.SettingsIcon(), Label: "Settings"},
		{ID: "dependencies", Icon: theme.InfoIcon(), Label: "Dependencies"},
	}, ui.onNavSelected)

	// Create file list panel
	ui.fileListPanel = uicontainer.NewFileListPanel(ui.window)
	ui.fileListPanel.OnFileAdded = ui.onFileAdded
	ui.fileListPanel.OnFileRemoved = ui.onFileRemoved
	ui.fileListPanel.OnFileSelected = ui.onFileSelected

	// Create progress panel
	ui.progressPanel = uicontainer.NewProgressPanel()
	ui.progressPanel.SetOutputDirectory(ui.config.OutputDirectory)

	// Create settings panel
	ui.settingsPanel = uicontainer.NewSettingsPanel(ui.window, ui.config)
	ui.settingsPanel.OnSave = func(config *models.Config) {
		ui.config = config
		ui.pipeline = services.NewPipeline(config)
		ui.pipeline.SetProgressCallback(func(stage string, percent int, message string) {
			fyne.Do(func() {
				if ui.progressPanel != nil {
					ui.progressPanel.SetProgress(stage, percent)
					ui.progressPanel.SetStatus(message)
				}
			})
		})
		ui.bottomControls.SetTTSProvider(config.TTSProvider)
		ui.progressPanel.SetOutputDirectory(config.OutputDirectory)
	}

	// Create bottom controls
	ui.bottomControls = widgets.NewBottomControls()
	ui.bottomControls.OnTranslateSelected = ui.onTranslateSelected
	ui.bottomControls.OnTranslateAll = ui.onTranslateAll
	ui.bottomControls.SetOnPreviewVoice(ui.previewSelectedVoice)
	ui.bottomControls.SetTTSProvider(ui.config.TTSProvider)

	// Main content area (split between file list and progress)
	fileListContent := ui.fileListPanel.Build()
	progressContent := ui.progressPanel.Build()

	mainSplit := container.NewHSplit(fileListContent, progressContent)
	mainSplit.SetOffset(0.4)

	// Translate view with bottom bar
	ui.translateView = container.New(
		layouts.NewContentWithBottomBar(160),
		mainSplit,
		ui.bottomControls.Build(),
	)

	// Settings view with right margin
	settingsRightSpacer := widgets.NewThemedRectangle(theme.ColorNameBackground)
	settingsRightSpacer.SetMinSize(fyne.NewSize(20, 0))
	ui.settingsView = container.NewBorder(nil, nil, nil, settingsRightSpacer, ui.settingsPanel.Build())

	// Dependencies panel with right margin
	ui.dependenciesPanel = uicontainer.NewDependenciesPanel(ui.pipeline.CheckDependencies)
	depsRightSpacer := widgets.NewThemedRectangle(theme.ColorNameBackground)
	depsRightSpacer.SetMinSize(fyne.NewSize(20, 0))
	ui.dependenciesView = container.NewBorder(nil, nil, nil, depsRightSpacer, ui.dependenciesPanel.Build())

	// Content area that can swap between views
	ui.contentArea = container.NewStack(ui.translateView)

	// Full layout with sidebar
	ui.mainContent = container.New(
		layouts.NewSidebarLayout(200),
		ui.buildSidebarWithBackground(),
		ui.contentArea,
	)

	return ui.mainContent
}

func (ui *MainUI) buildSidebarWithBackground() fyne.CanvasObject {
	bg := widgets.NewThemedRectangle(appTheme.ColorNameSidebar)
	return container.NewStack(bg, ui.sidebar)
}

func (ui *MainUI) onNavSelected(id string) {
	ui.currentView = id

	switch id {
	case "translate":
		ui.showView(ui.translateView)
	case "settings":
		ui.showView(ui.settingsView)
	case "dependencies":
		ui.showView(ui.dependenciesView)
	}
}

func (ui *MainUI) showView(view fyne.CanvasObject) {
	ui.contentArea.RemoveAll()
	ui.contentArea.Add(view)
	ui.contentArea.Refresh()
}

func (ui *MainUI) onFileAdded(path string) {
	// Check for duplicates
	for _, job := range ui.jobs {
		if job.InputPath == path {
			return
		}
	}

	job := models.NewTranslationJob(path)
	job.SourceLang = ui.bottomControls.GetSourceLang()
	job.TargetLang = ui.bottomControls.GetTargetLang()
	job.Voice = ui.bottomControls.GetVoice()
	ui.jobs = append(ui.jobs, job)
	ui.fileListPanel.SetJobs(ui.jobs)
}

func (ui *MainUI) onFileRemoved(index int) {
	if index >= 0 && index < len(ui.jobs) {
		ui.jobs = append(ui.jobs[:index], ui.jobs[index+1:]...)
		ui.fileListPanel.SetJobs(ui.jobs)
	}
}

func (ui *MainUI) onFileSelected(index int) {
	if index >= 0 && index < len(ui.jobs) {
		job := ui.jobs[index]
		ui.progressPanel.SetCurrentJob(job)
	}
}

func (ui *MainUI) onTranslateSelected() {
	selected := ui.fileListPanel.GetSelectedIndex()
	if selected < 0 || selected >= len(ui.jobs) {
		dialog.ShowCustom("No Selection", "OK", widget.NewLabel("Please select a video file to translate."), ui.window)
		return
	}

	job := ui.jobs[selected]
	if job.Status != models.StatusPending {
		dialog.ShowCustom("Already Processing", "OK", widget.NewLabel("This file is already being processed or completed."), ui.window)
		return
	}

	ui.translateJob(job)
}

func (ui *MainUI) onTranslateAll() {
	var pendingJobs []*models.TranslationJob
	for _, job := range ui.jobs {
		if job.Status == models.StatusPending {
			pendingJobs = append(pendingJobs, job)
		}
	}

	if len(pendingJobs) == 0 {
		dialog.ShowCustom("No Files", "OK", widget.NewLabel("No pending files to translate."), ui.window)
		return
	}

	totalJobs := len(pendingJobs)

	// Show initial status
	fyne.Do(func() {
		ui.progressPanel.SetStatus(fmt.Sprintf("Starting %d videos (max %d parallel)...", totalJobs, maxParallelVideos))
	})

	// Semaphore to limit concurrent videos (prevents overloading computer)
	sem := make(chan struct{}, maxParallelVideos)
	var wg sync.WaitGroup
	var completedCount int32

	for _, job := range pendingJobs {
		wg.Add(1)
		go func(j *models.TranslationJob) {
			defer wg.Done()

			// Acquire semaphore (blocks if maxParallelVideos already running)
			sem <- struct{}{}
			defer func() { <-sem }() // Release semaphore when done

			// Process this video
			ui.translateJobSync(j)

			// Update completion count
			count := atomic.AddInt32(&completedCount, 1)
			fyne.Do(func() {
				ui.progressPanel.SetStatus(fmt.Sprintf("Completed %d of %d videos...", count, totalJobs))
				ui.fileListPanel.Refresh()
			})
		}(job)
	}

	// Wait for all jobs to complete in background
	go func() {
		wg.Wait()
		fyne.Do(func() {
			ui.progressPanel.SetStatus("")
			dialog.ShowCustom("Complete", "OK", widget.NewLabel(fmt.Sprintf("All %d videos translated!", totalJobs)), ui.window)
		})
	}()
}

func (ui *MainUI) translateJob(job *models.TranslationJob) {
	job.SourceLang = ui.bottomControls.GetSourceLang()
	job.TargetLang = ui.bottomControls.GetTargetLang()
	job.Voice = ui.bottomControls.GetVoice()

	if err := ui.pipeline.ValidateJob(job); err != nil {
		dialog.ShowCustom("Error", "OK", widget.NewLabel(err.Error()), ui.window)
		return
	}

	job.Status = models.StatusProcessing
	ui.fileListPanel.Refresh()
	ui.progressPanel.SetCurrentJob(job)

	go func() {
		err := ui.pipeline.Process(job)

		fyne.Do(func() {
			ui.fileListPanel.Refresh()
			ui.progressPanel.Update()

			if err != nil {
				dialog.ShowCustom("Error", "OK", widget.NewLabel(err.Error()), ui.window)
			} else {
				dialog.ShowCustomConfirm("Complete", "Open Folder", "Close",
					container.NewVBox(
						widgets.NewSectionHeader("Translation Complete"),
						container.NewPadded(
							container.NewWithoutLayout(),
						),
					),
					func(openFolder bool) {
						if openFolder {
							exec.Command("open", filepath.Dir(job.OutputPath)).Start()
						}
					}, ui.window)
			}
		})
	}()
}

func (ui *MainUI) translateJobSync(job *models.TranslationJob) {
	fyne.Do(func() {
		job.SourceLang = ui.bottomControls.GetSourceLang()
		job.TargetLang = ui.bottomControls.GetTargetLang()
		job.Voice = ui.bottomControls.GetVoice()
	})

	if err := ui.pipeline.ValidateJob(job); err != nil {
		fyne.Do(func() {
			dialog.ShowCustom("Error", "OK", widget.NewLabel(err.Error()), ui.window)
		})
		return
	}

	fyne.Do(func() {
		job.Status = models.StatusProcessing
		ui.fileListPanel.Refresh()
		ui.progressPanel.SetCurrentJob(job)
	})

	// Use per-job progress callback for parallel processing
	err := ui.pipeline.ProcessWithCallback(job, func(stage string, percent int, message string) {
		fyne.Do(func() {
			// Update job progress
			job.Progress = percent
			ui.fileListPanel.Refresh()

			// Show detailed progress if this job is selected
			selectedIdx := ui.fileListPanel.GetSelectedIndex()
			if selectedIdx >= 0 && selectedIdx < len(ui.jobs) && ui.jobs[selectedIdx] == job {
				ui.progressPanel.SetProgress(stage, percent)
				ui.progressPanel.SetStatus(message)
			}
		})
	})

	fyne.Do(func() {
		ui.fileListPanel.Refresh()
		ui.progressPanel.Update()

		if err != nil {
			dialog.ShowCustom("Error", "OK", widget.NewLabel(err.Error()), ui.window)
		}
	})
}

func (ui *MainUI) previewSelectedVoice() {
	voice := ui.bottomControls.GetVoice()
	provider := ui.bottomControls.GetTTSProvider()
	sampleText := "This is a preview of the selected voice."

	logger.LogInfo("Preview: provider=%s voice=%s", provider, voice)

	ui.progressPanel.SetStatus(fmt.Sprintf("Generating: %s (%s)", voice, provider))

	go func() {
		homeDir, _ := os.UserHomeDir()
		tempDir := filepath.Join(homeDir, ".cache", "video-translator")
		os.MkdirAll(tempDir, 0755)
		tempPath := filepath.Join(tempDir, "voice_preview.wav")

		var err error

		switch provider {
		case "piper":
			// Check if voice model exists, download if missing
			if !services.VoiceModelExists(voice) {
				fyne.Do(func() {
					ui.progressPanel.SetStatus(fmt.Sprintf("Downloading voice: %s...", voice))
				})
				if downloadErr := services.DownloadVoiceModel(voice); downloadErr != nil {
					fyne.Do(func() {
						dialog.ShowCustom("Error", "OK", widget.NewLabel(fmt.Sprintf("failed to download voice: %v", downloadErr)), ui.window)
						ui.progressPanel.SetStatus("")
					})
					return
				}
			}
			svc := services.NewTTSService(voice)
			err = svc.Synthesize(sampleText, tempPath)
		case "openai":
			svc := services.NewOpenAITTSService(ui.config.OpenAIKey, ui.config.OpenAITTSModel, voice, ui.config.OpenAITTSSpeed)
			err = svc.Synthesize(sampleText, tempPath)
		case "edge-tts":
			svc := services.NewEdgeTTSService(voice)
			err = svc.Synthesize(sampleText, tempPath)
		case "fish-audio":
			svc := services.NewFishAudioTTSService(
				ui.config.FishAudioAPIKey,
				ui.config.FishAudioModel,
				voice, // Use voice from dropdown (contains reference_id)
				ui.config.FishAudioSpeed,
			)
			err = svc.Synthesize(sampleText, tempPath)
		case "cosyvoice":
			if ui.config.VoiceCloneSamplePath == "" {
				fyne.Do(func() {
					dialog.ShowCustom("Error", "OK", widget.NewLabel("CosyVoice requires a voice sample. Configure it in Settings"), ui.window)
					ui.progressPanel.SetStatus("")
				})
				return
			}
			svc := services.NewCosyVoiceService(
				ui.config.CosyVoicePath,
				ui.config.CosyVoiceMode,
				ui.config.CosyVoiceAPIURL,
				ui.config.VoiceCloneSamplePath,
				ui.config.PythonPath,
			)
			err = svc.Synthesize(sampleText, tempPath)
		default:
			err = fmt.Errorf("unknown TTS provider: %s", provider)
		}

		if err != nil {
			fyne.Do(func() {
				dialog.ShowCustom("Error", "OK", widget.NewLabel(fmt.Sprintf("TTS failed: %v", err)), ui.window)
				ui.progressPanel.SetStatus("")
			})
			return
		}

		if _, statErr := os.Stat(tempPath); os.IsNotExist(statErr) {
			fyne.Do(func() {
				dialog.ShowCustom("Error", "OK", widget.NewLabel("audio file was not created"), ui.window)
				ui.progressPanel.SetStatus("")
			})
			return
		}

		fyne.Do(func() {
			ui.progressPanel.SetStatus(fmt.Sprintf("Playing: %s (%s)", voice, provider))
		})

		playErr := playAudio(tempPath)

		fyne.Do(func() {
			if playErr != nil {
				dialog.ShowCustom("Error", "OK", widget.NewLabel(fmt.Sprintf("playback failed: %v", playErr)), ui.window)
			}
			ui.progressPanel.SetStatus("")
		})
	}()
}

func playAudio(path string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("afplay", path)
	case "linux":
		cmd = exec.Command("paplay", path)
	case "windows":
		cmd = exec.Command("powershell", "-c", fmt.Sprintf("(New-Object Media.SoundPlayer '%s').PlaySync()", path))
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return cmd.Run()
}

// GetWindow returns the main window
func (ui *MainUI) GetWindow() fyne.Window {
	return ui.window
}
