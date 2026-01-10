package widgets

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Language represents a language option
type Language struct {
	Code string
	Name string
}

// SourceLanguages returns available source languages
func SourceLanguages() []Language {
	return []Language{
		{Code: "ru", Name: "Russian"},
		{Code: "es", Name: "Spanish"},
		{Code: "fr", Name: "French"},
		{Code: "de", Name: "German"},
		{Code: "zh", Name: "Chinese"},
		{Code: "ja", Name: "Japanese"},
		{Code: "ko", Name: "Korean"},
		{Code: "pt", Name: "Portuguese"},
		{Code: "it", Name: "Italian"},
		{Code: "ar", Name: "Arabic"},
	}
}

// TargetLanguages returns available target languages
func TargetLanguages() []Language {
	return []Language{
		{Code: "en", Name: "English"},
		{Code: "es", Name: "Spanish"},
		{Code: "fr", Name: "French"},
		{Code: "de", Name: "German"},
		{Code: "pt", Name: "Portuguese"},
		{Code: "it", Name: "Italian"},
		{Code: "ru", Name: "Russian"},
	}
}

// LanguageSelector is a dropdown for language selection
type LanguageSelector struct {
	widget.BaseWidget

	Label      string
	Languages  []Language
	Selected   string
	OnChanged  func(code string)

	label  *widget.Label
	select_ *widget.Select
}

// NewLanguageSelector creates a new language selector
func NewLanguageSelector(label string, languages []Language, onChanged func(code string)) *LanguageSelector {
	s := &LanguageSelector{
		Label:     label,
		Languages: languages,
		OnChanged: onChanged,
	}
	if len(languages) > 0 {
		s.Selected = languages[0].Code
	}
	s.ExtendBaseWidget(s)
	return s
}

// SetSelected sets the selected language by code
func (s *LanguageSelector) SetSelected(code string) {
	s.Selected = code
	if s.select_ != nil {
		for _, lang := range s.Languages {
			if lang.Code == code {
				s.select_.SetSelected(lang.Name)
				break
			}
		}
	}
}

// GetSelected returns the selected language code
func (s *LanguageSelector) GetSelected() string {
	return s.Selected
}

// Build creates the widget UI
func (s *LanguageSelector) Build() fyne.CanvasObject {
	s.label = widget.NewLabel(s.Label)

	// Create options from languages
	options := make([]string, len(s.Languages))
	for i, lang := range s.Languages {
		options[i] = lang.Name
	}

	s.select_ = widget.NewSelect(options, func(selected string) {
		// Find code for selected name
		for _, lang := range s.Languages {
			if lang.Name == selected {
				s.Selected = lang.Code
				if s.OnChanged != nil {
					s.OnChanged(lang.Code)
				}
				break
			}
		}
	})

	// Set initial selection
	if s.Selected != "" {
		for _, lang := range s.Languages {
			if lang.Code == s.Selected {
				s.select_.SetSelected(lang.Name)
				break
			}
		}
	} else if len(options) > 0 {
		s.select_.SetSelectedIndex(0)
	}

	return container.NewVBox(
		s.label,
		s.select_,
	)
}

// CreateRenderer implements fyne.Widget
func (s *LanguageSelector) CreateRenderer() fyne.WidgetRenderer {
	content := s.Build()
	return widget.NewSimpleRenderer(content)
}

// CompactLanguageSelector is a horizontal language selector without label
type CompactLanguageSelector struct {
	widget.BaseWidget

	Languages []Language
	Selected  string
	OnChanged func(code string)
	select_   *widget.Select
}

// NewCompactLanguageSelector creates a compact language selector
func NewCompactLanguageSelector(languages []Language, onChanged func(code string)) *CompactLanguageSelector {
	s := &CompactLanguageSelector{
		Languages: languages,
		OnChanged: onChanged,
	}
	if len(languages) > 0 {
		s.Selected = languages[0].Code
	}
	s.ExtendBaseWidget(s)
	return s
}

// SetSelected sets the selected language by code
func (s *CompactLanguageSelector) SetSelected(code string) {
	s.Selected = code
	if s.select_ != nil {
		for _, lang := range s.Languages {
			if lang.Code == code {
				s.select_.SetSelected(lang.Name)
				break
			}
		}
	}
}

// GetSelected returns the selected language code
func (s *CompactLanguageSelector) GetSelected() string {
	return s.Selected
}

// Build creates the widget UI
func (s *CompactLanguageSelector) Build() fyne.CanvasObject {
	options := make([]string, len(s.Languages))
	for i, lang := range s.Languages {
		options[i] = lang.Name
	}

	s.select_ = widget.NewSelect(options, func(selected string) {
		for _, lang := range s.Languages {
			if lang.Name == selected {
				s.Selected = lang.Code
				if s.OnChanged != nil {
					s.OnChanged(lang.Code)
				}
				break
			}
		}
	})

	// Set initial selection
	if s.Selected != "" {
		for _, lang := range s.Languages {
			if lang.Code == s.Selected {
				s.select_.SetSelected(lang.Name)
				break
			}
		}
	} else if len(options) > 0 {
		s.select_.SetSelectedIndex(0)
	}

	return s.select_
}

// CreateRenderer implements fyne.Widget
func (s *CompactLanguageSelector) CreateRenderer() fyne.WidgetRenderer {
	content := s.Build()
	return widget.NewSimpleRenderer(content)
}
