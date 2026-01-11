package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// VoiceOption represents a voice choice
type VoiceOption struct {
	ID       string
	Name     string
	Provider string
}

// PiperVoices returns available Piper voices
func PiperVoices() []VoiceOption {
	return []VoiceOption{
		{ID: "en_US-amy-medium", Name: "Amy (US)", Provider: "piper"},
		{ID: "en_US-ryan-medium", Name: "Ryan (US)", Provider: "piper"},
		{ID: "en_GB-alba-medium", Name: "Alba (UK)", Provider: "piper"},
		{ID: "de_DE-thorsten-medium", Name: "Thorsten (DE)", Provider: "piper"},
		{ID: "fr_FR-upmc-medium", Name: "UPMC (FR)", Provider: "piper"},
	}
}

// OpenAIVoices returns available OpenAI TTS voices
func OpenAIVoices() []VoiceOption {
	return []VoiceOption{
		{ID: "alloy", Name: "Alloy", Provider: "openai"},
		{ID: "echo", Name: "Echo", Provider: "openai"},
		{ID: "fable", Name: "Fable", Provider: "openai"},
		{ID: "onyx", Name: "Onyx", Provider: "openai"},
		{ID: "nova", Name: "Nova", Provider: "openai"},
		{ID: "shimmer", Name: "Shimmer", Provider: "openai"},
	}
}

// EdgeTTSVoices returns available Edge TTS voices
func EdgeTTSVoices() []VoiceOption {
	return []VoiceOption{
		{ID: "en-US-AriaNeural", Name: "Aria (US)", Provider: "edge-tts"},
		{ID: "en-US-GuyNeural", Name: "Guy (US)", Provider: "edge-tts"},
		{ID: "en-GB-SoniaNeural", Name: "Sonia (UK)", Provider: "edge-tts"},
		{ID: "en-GB-RyanNeural", Name: "Ryan (UK)", Provider: "edge-tts"},
		{ID: "de-DE-KatjaNeural", Name: "Katja (DE)", Provider: "edge-tts"},
		{ID: "fr-FR-DeniseNeural", Name: "Denise (FR)", Provider: "edge-tts"},
		{ID: "es-ES-ElviraNeural", Name: "Elvira (ES)", Provider: "edge-tts"},
		{ID: "ru-RU-DariyaNeural", Name: "Dariya (RU)", Provider: "edge-tts"},
	}
}

// CosyVoices returns CosyVoice options
func CosyVoices() []VoiceOption {
	return []VoiceOption{
		{ID: "clone", Name: "Voice Clone", Provider: "cosyvoice"},
	}
}

// FishAudioVoices returns available Fish Audio voices
// Voice IDs are reference_ids from fish.audio
// Browse more voices at https://fish.audio/discover
func FishAudioVoices() []VoiceOption {
	return []VoiceOption{
		{ID: "933563129e564b19a115bedd57b7406a", Name: "Sarah", Provider: "fish-audio"},
		{ID: "bf322df2096a46f18c579d0baa36f41d", Name: "Adrian", Provider: "fish-audio"},
		{ID: "b347db033a6549378b48d00acb0d06cd", Name: "Selene", Provider: "fish-audio"},
		{ID: "536d3a5e000945adb7038665781a4aca", Name: "Ethan", Provider: "fish-audio"},
		{ID: "802e3bc2b27e49c2995d23ef70e6ac89", Name: "Energetic Male", Provider: "fish-audio"},
		{ID: "8ef4a238714b45718ce04243307c57a7", Name: "E-girl", Provider: "fish-audio"},
	}
}

// GetVoicesForProvider returns voices for a given provider
func GetVoicesForProvider(provider string) []VoiceOption {
	switch provider {
	case "piper":
		return PiperVoices()
	case "openai":
		return OpenAIVoices()
	case "edge-tts":
		return EdgeTTSVoices()
	case "cosyvoice":
		return CosyVoices()
	case "fish-audio":
		return FishAudioVoices()
	default:
		return EdgeTTSVoices()
	}
}

// VoiceSelector allows voice selection with preview
type VoiceSelector struct {
	widget.BaseWidget

	Voices      []VoiceOption
	Selected    string
	Provider    string
	OnChanged   func(voiceID string)
	OnPreview   func()

	select_     *widget.Select
	previewBtn  *widget.Button
}

// NewVoiceSelector creates a new voice selector
func NewVoiceSelector(provider string, onChanged func(voiceID string), onPreview func()) *VoiceSelector {
	voices := GetVoicesForProvider(provider)
	s := &VoiceSelector{
		Voices:    voices,
		Provider:  provider,
		OnChanged: onChanged,
		OnPreview: onPreview,
	}
	if len(voices) > 0 {
		s.Selected = voices[0].ID
	}
	s.ExtendBaseWidget(s)
	return s
}

// SetProvider updates the provider and available voices
func (s *VoiceSelector) SetProvider(provider string) {
	s.Provider = provider
	s.Voices = GetVoicesForProvider(provider)

	if len(s.Voices) > 0 {
		s.Selected = s.Voices[0].ID
	}

	if s.select_ != nil {
		options := make([]string, len(s.Voices))
		for i, v := range s.Voices {
			options[i] = v.Name
		}
		s.select_.Options = options
		if len(options) > 0 {
			s.select_.SetSelectedIndex(0)
		}
		s.select_.Refresh()
	}
}

// SetSelected sets the selected voice by ID
func (s *VoiceSelector) SetSelected(voiceID string) {
	s.Selected = voiceID
	if s.select_ != nil {
		for _, v := range s.Voices {
			if v.ID == voiceID {
				s.select_.SetSelected(v.Name)
				break
			}
		}
	}
}

// GetSelected returns the selected voice ID
func (s *VoiceSelector) GetSelected() string {
	return s.Selected
}

// Build creates the widget UI
func (s *VoiceSelector) Build() fyne.CanvasObject {
	options := make([]string, len(s.Voices))
	for i, v := range s.Voices {
		options[i] = v.Name
	}

	s.select_ = widget.NewSelect(options, func(selected string) {
		for _, v := range s.Voices {
			if v.Name == selected {
				s.Selected = v.ID
				if s.OnChanged != nil {
					s.OnChanged(v.ID)
				}
				break
			}
		}
	})

	// Set initial selection
	if s.Selected != "" {
		for _, v := range s.Voices {
			if v.ID == s.Selected {
				s.select_.SetSelected(v.Name)
				break
			}
		}
	} else if len(options) > 0 {
		s.select_.SetSelectedIndex(0)
	}

	// Preview button
	s.previewBtn = widget.NewButtonWithIcon("", theme.MediaPlayIcon(), func() {
		if s.OnPreview != nil {
			s.OnPreview()
		}
	})

	return container.NewBorder(
		nil, nil, nil,
		s.previewBtn,
		s.select_,
	)
}

// CreateRenderer implements fyne.Widget
func (s *VoiceSelector) CreateRenderer() fyne.WidgetRenderer {
	content := s.Build()
	return widget.NewSimpleRenderer(content)
}
