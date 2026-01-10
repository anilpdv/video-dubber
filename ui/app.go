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

	"video-translator/models"
	"video-translator/services"
	uicontainer "video-translator/ui/container"
	"video-translator/ui/dialogs"
	"video-translator/ui/layouts"
	appTheme "video-translator/ui/theme"
	"video-translator/ui/widgets"
)

// maxParallelVideos limits concurrent video processing to avoid overloading the computer
const maxParallelVideos = 2

// MainUI is the main application UI
type MainUI struct {
	window   fyne.Window
	jobs     []*models.TranslationJob
	config   *models.Config
	pipeline *services.Pipeline

	// UI Components
	sidebar        *widgets.SidebarNav
	fileListPanel  *uicontainer.FileListPanel
	progressPanel  *uicontainer.ProgressPanel
	bottomControls *widgets.BottomControls
	mainContent    *fyne.Container

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

	// Create bottom controls
	ui.bottomControls = widgets.NewBottomControls()
	ui.bottomControls.OnTranslateSelected = ui.onTranslateSelected
	ui.bottomControls.OnTranslateAll = ui.onTranslateAll
	ui.bottomControls.OnSettings = ui.showSettings
	ui.bottomControls.SetOnPreviewVoice(ui.previewSelectedVoice)
	ui.bottomControls.SetTTSProvider(ui.config.TTSProvider)

	// Main content area (split between file list and progress)
	fileListContent := ui.fileListPanel.Build()
	progressContent := ui.progressPanel.Build()

	mainSplit := container.NewHSplit(fileListContent, progressContent)
	mainSplit.SetOffset(0.4)

	// Content with bottom bar
	contentWithBottom := container.New(
		layouts.NewContentWithBottomBar(140),
		mainSplit,
		ui.bottomControls.Build(),
	)

	// Full layout with sidebar
	ui.mainContent = container.New(
		layouts.NewSidebarLayout(200),
		ui.buildSidebarWithBackground(),
		contentWithBottom,
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
		// Already showing translate view
	case "settings":
		ui.showSettings()
		// Reset to translate after showing settings
		ui.sidebar.SetSelected("translate")
	case "dependencies":
		ui.showDependencyCheck()
		ui.sidebar.SetSelected("translate")
	}
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
		dialog.ShowInformation("No Selection", "Please select a video file to translate.", ui.window)
		return
	}

	job := ui.jobs[selected]
	if job.Status != models.StatusPending {
		dialog.ShowInformation("Already Processing", "This file is already being processed or completed.", ui.window)
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
		dialog.ShowInformation("No Files", "No pending files to translate.", ui.window)
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
			dialog.ShowInformation("Complete", fmt.Sprintf("All %d videos translated!", totalJobs), ui.window)
		})
	}()
}

func (ui *MainUI) translateJob(job *models.TranslationJob) {
	job.SourceLang = ui.bottomControls.GetSourceLang()
	job.TargetLang = ui.bottomControls.GetTargetLang()
	job.Voice = ui.bottomControls.GetVoice()

	if err := ui.pipeline.ValidateJob(job); err != nil {
		dialog.ShowError(err, ui.window)
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
				dialog.ShowError(err, ui.window)
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
			dialog.ShowError(err, ui.window)
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
			dialog.ShowError(err, ui.window)
		}
	})
}

func (ui *MainUI) showSettings() {
	settingsDialog := dialogs.NewSettingsDialog(ui.window, ui.config)
	settingsDialog.OnSave = func(config *models.Config) {
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
		ui.progressPanel.SetOutputDirectory(config.OutputDirectory)
		ui.bottomControls.SetTTSProvider(config.TTSProvider)
	}
	settingsDialog.OnTTSChanged = func(provider string) {
		ui.bottomControls.SetTTSProvider(provider)
	}
	settingsDialog.Show()
}

func (ui *MainUI) showDependencyCheck() {
	results := ui.pipeline.CheckDependencies()
	dialogs.ShowDependencyCheck(ui.window, results)
}

func (ui *MainUI) previewSelectedVoice() {
	voice := ui.bottomControls.GetVoice()
	provider := ui.bottomControls.GetTTSProvider()
	sampleText := "This is a preview of the selected voice."

	services.LogInfo("Preview: provider=%s voice=%s", provider, voice)

	ui.progressPanel.SetStatus(fmt.Sprintf("Generating: %s (%s)", voice, provider))

	go func() {
		homeDir, _ := os.UserHomeDir()
		tempDir := filepath.Join(homeDir, ".cache", "video-translator")
		os.MkdirAll(tempDir, 0755)
		tempPath := filepath.Join(tempDir, "voice_preview.wav")

		var err error

		switch provider {
		case "piper":
			svc := services.NewTTSService(voice)
			err = svc.Synthesize(sampleText, tempPath)
		case "openai":
			svc := services.NewOpenAITTSService(ui.config.OpenAIKey, ui.config.OpenAITTSModel, voice, ui.config.OpenAITTSSpeed)
			err = svc.Synthesize(sampleText, tempPath)
		case "edge-tts":
			svc := services.NewEdgeTTSService(voice)
			err = svc.Synthesize(sampleText, tempPath)
		case "cosyvoice":
			if ui.config.VoiceCloneSamplePath == "" {
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("CosyVoice requires a voice sample. Configure it in Settings"), ui.window)
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
				dialog.ShowError(fmt.Errorf("TTS failed: %w", err), ui.window)
				ui.progressPanel.SetStatus("")
			})
			return
		}

		if _, statErr := os.Stat(tempPath); os.IsNotExist(statErr) {
			fyne.Do(func() {
				dialog.ShowError(fmt.Errorf("audio file was not created"), ui.window)
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
				dialog.ShowError(fmt.Errorf("playback failed: %w", playErr), ui.window)
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
