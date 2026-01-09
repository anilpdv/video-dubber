package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type TranslatorTheme struct{}

var _ fyne.Theme = (*TranslatorTheme)(nil)

func (t *TranslatorTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 59, G: 130, B: 246, A: 255} // Blue
	case theme.ColorNameButton:
		return color.NRGBA{R: 59, G: 130, B: 246, A: 255}
	}
	// Delegate all other colors (including Background and Foreground) to default theme
	// This properly handles dark/light mode variants
	return theme.DefaultTheme().Color(name, variant)
}

func (t *TranslatorTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *TranslatorTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *TranslatorTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 8
	case theme.SizeNameInnerPadding:
		return 4
	case theme.SizeNameText:
		return 14
	}
	return theme.DefaultTheme().Size(name)
}
