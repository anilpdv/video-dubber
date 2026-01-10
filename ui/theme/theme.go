package theme

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Custom theme color names
const (
	ColorNameSidebar        fyne.ThemeColorName = "sidebar"
	ColorNameSidebarHover   fyne.ThemeColorName = "sidebarHover"
	ColorNameSidebarActive  fyne.ThemeColorName = "sidebarActive"
	ColorNameSurface        fyne.ThemeColorName = "surface"
	ColorNameSurfaceVariant fyne.ThemeColorName = "surfaceVariant"
	ColorNameBottomPanel    fyne.ThemeColorName = "bottomPanel"
	ColorNameDivider        fyne.ThemeColorName = "divider"
	ColorNameInputBg        fyne.ThemeColorName = "inputBg"

	// Job status colors
	ColorNameJobPending    fyne.ThemeColorName = "jobPending"
	ColorNameJobProcessing fyne.ThemeColorName = "jobProcessing"
	ColorNameJobCompleted  fyne.ThemeColorName = "jobCompleted"
	ColorNameJobFailed     fyne.ThemeColorName = "jobFailed"

	// Text variants
	ColorNameTextSecondary fyne.ThemeColorName = "textSecondary"
	ColorNameTextHint      fyne.ThemeColorName = "textHint"
)

// Custom size names
const (
	SizeNameSidebarWidth   fyne.ThemeSizeName = "sidebarWidth"
	SizeNameBottomBarHeight fyne.ThemeSizeName = "bottomBarHeight"
	SizeNameCardRadius     fyne.ThemeSizeName = "cardRadius"
	SizeNameIconMedium     fyne.ThemeSizeName = "iconMedium"
	SizeNameIconLarge      fyne.ThemeSizeName = "iconLarge"
)

// VideoTranslatorTheme is a custom dark theme for the video translator app
type VideoTranslatorTheme struct{}

var _ fyne.Theme = (*VideoTranslatorTheme)(nil)

// Color returns the color for the specified name
func (t *VideoTranslatorTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	// Always use dark variant for our app
	switch name {
	// Core colors
	case theme.ColorNameBackground:
		return ColorBackground
	case theme.ColorNameForeground:
		return ColorTextPrimary
	case theme.ColorNamePrimary:
		return ColorPrimary
	case theme.ColorNameButton:
		return ColorPrimary

	// Input colors
	case theme.ColorNameInputBackground:
		return ColorInputBg
	case theme.ColorNameInputBorder:
		return ColorDivider
	case theme.ColorNamePlaceHolder:
		return ColorTextHint
	case theme.ColorNameFocus:
		return ColorFocusBorder

	// Selection colors
	case theme.ColorNameSelection:
		return WithAlpha(ColorPrimary, 80).(color.NRGBA)
	case theme.ColorNameHover:
		return ColorHover
	case theme.ColorNamePressed:
		return ColorPressed

	// Disabled colors
	case theme.ColorNameDisabled:
		return ColorTextDisabled
	case theme.ColorNameDisabledButton:
		return ColorDisabledBg

	// Scroll colors
	case theme.ColorNameScrollBar:
		return ColorScrollbar

	// Separator
	case theme.ColorNameSeparator:
		return ColorDivider

	// Error colors
	case theme.ColorNameError:
		return ColorError
	case theme.ColorNameSuccess:
		return ColorSuccess
	case theme.ColorNameWarning:
		return ColorWarning

	// Shadow (for cards, dialogs)
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 100}

	// Menu/overlay
	case theme.ColorNameOverlayBackground:
		return ColorOverlay
	case theme.ColorNameMenuBackground:
		return ColorSurface
	case theme.ColorNameHeaderBackground:
		return ColorSurface

	// Custom colors
	case ColorNameSidebar:
		return ColorSidebar
	case ColorNameSidebarHover:
		return ColorSidebarHover
	case ColorNameSidebarActive:
		return ColorSidebarActive
	case ColorNameSurface:
		return ColorSurface
	case ColorNameSurfaceVariant:
		return ColorSurfaceVariant
	case ColorNameBottomPanel:
		return ColorBottomPanel
	case ColorNameDivider:
		return ColorDivider
	case ColorNameInputBg:
		return ColorInputBg
	case ColorNameJobPending:
		return ColorPending
	case ColorNameJobProcessing:
		return ColorProcessing
	case ColorNameJobCompleted:
		return ColorSuccess
	case ColorNameJobFailed:
		return ColorError
	case ColorNameTextSecondary:
		return ColorTextSecondary
	case ColorNameTextHint:
		return ColorTextHint

	// Hyperlink
	case theme.ColorNameHyperlink:
		return ColorSecondary

	default:
		// Fallback to default dark theme
		return theme.DefaultTheme().Color(name, theme.VariantDark)
	}
}

// Font returns the font for the specified style
func (t *VideoTranslatorTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

// Icon returns the icon for the specified name
func (t *VideoTranslatorTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

// Size returns the size for the specified name
func (t *VideoTranslatorTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 10
	case theme.SizeNameInnerPadding:
		return 6
	case theme.SizeNameText:
		return 14
	case theme.SizeNameHeadingText:
		return 20
	case theme.SizeNameSubHeadingText:
		return 16
	case theme.SizeNameCaptionText:
		return 12
	case theme.SizeNameInlineIcon:
		return 20
	case theme.SizeNameScrollBar:
		return 10
	case theme.SizeNameScrollBarSmall:
		return 4
	case theme.SizeNameSeparatorThickness:
		return 1
	case theme.SizeNameInputBorder:
		return 1
	case theme.SizeNameInputRadius:
		return 6

	// Custom sizes
	case SizeNameSidebarWidth:
		return 200
	case SizeNameBottomBarHeight:
		return 80
	case SizeNameCardRadius:
		return 8
	case SizeNameIconMedium:
		return 24
	case SizeNameIconLarge:
		return 32

	default:
		return theme.DefaultTheme().Size(name)
	}
}
