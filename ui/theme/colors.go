package theme

import "image/color"

// Dark theme color palette inspired by Supersonic
var (
	// Background layers (darkest to lightest)
	ColorBackground     = color.NRGBA{R: 18, G: 18, B: 18, A: 255}   // #121212
	ColorSurface        = color.NRGBA{R: 30, G: 30, B: 30, A: 255}   // #1E1E1E
	ColorSurfaceVariant = color.NRGBA{R: 40, G: 40, B: 40, A: 255}   // #282828
	ColorOverlay        = color.NRGBA{R: 50, G: 50, B: 50, A: 255}   // #323232
	ColorSidebar        = color.NRGBA{R: 24, G: 24, B: 24, A: 255}   // #181818
	ColorSidebarHover   = color.NRGBA{R: 45, G: 45, B: 45, A: 255}   // #2D2D2D
	ColorSidebarActive  = color.NRGBA{R: 55, G: 55, B: 55, A: 255}   // #373737

	// Accent colors (Brand plum)
	ColorPrimary        = color.NRGBA{R: 92, G: 58, B: 88, A: 255}    // #5C3A58 (brand plum)
	ColorPrimaryVariant = color.NRGBA{R: 71, G: 45, B: 68, A: 255}    // #472D44 (darker plum)
	ColorSecondary      = color.NRGBA{R: 125, G: 90, B: 121, A: 255}  // #7D5A79 (lighter plum)

	// Text colors
	ColorTextPrimary   = color.NRGBA{R: 255, G: 255, B: 255, A: 255} // #FFFFFF
	ColorTextSecondary = color.NRGBA{R: 158, G: 158, B: 158, A: 255} // #9E9E9E
	ColorTextDisabled  = color.NRGBA{R: 97, G: 97, B: 97, A: 255}    // #616161
	ColorTextHint      = color.NRGBA{R: 117, G: 117, B: 117, A: 255} // #757575

	// Status colors
	ColorSuccess    = color.NRGBA{R: 76, G: 175, B: 80, A: 255}  // #4CAF50 - Completed
	ColorWarning    = color.NRGBA{R: 255, G: 193, B: 7, A: 255}  // #FFC107 - Warning
	ColorError      = color.NRGBA{R: 244, G: 67, B: 54, A: 255}  // #F44336 - Failed
	ColorProcessing = color.NRGBA{R: 33, G: 150, B: 243, A: 255} // #2196F3 - Processing
	ColorPending    = color.NRGBA{R: 117, G: 117, B: 117, A: 255} // #757575 - Pending

	// UI element colors
	ColorDivider      = color.NRGBA{R: 48, G: 48, B: 48, A: 255}   // #303030
	ColorInputBg      = color.NRGBA{R: 35, G: 35, B: 35, A: 255}   // #232323
	ColorHover        = color.NRGBA{R: 255, G: 255, B: 255, A: 20} // White with low opacity
	ColorPressed      = color.NRGBA{R: 255, G: 255, B: 255, A: 30} // White with higher opacity
	ColorFocusBorder  = color.NRGBA{R: 92, G: 58, B: 88, A: 180}   // Primary with transparency
	ColorDisabledBg   = color.NRGBA{R: 38, G: 38, B: 38, A: 255}   // #262626
	ColorScrollbar    = color.NRGBA{R: 80, G: 80, B: 80, A: 255}   // #505050
	ColorBottomPanel  = color.NRGBA{R: 22, G: 22, B: 22, A: 255}   // #161616
)

// Blend blends two colors with a given weight (0.0 = first color, 1.0 = second color)
func Blend(c1, c2 color.Color, weight float64) color.Color {
	r1, g1, b1, a1 := c1.RGBA()
	r2, g2, b2, a2 := c2.RGBA()

	return color.NRGBA{
		R: uint8((float64(r1>>8)*(1-weight) + float64(r2>>8)*weight)),
		G: uint8((float64(g1>>8)*(1-weight) + float64(g2>>8)*weight)),
		B: uint8((float64(b1>>8)*(1-weight) + float64(b2>>8)*weight)),
		A: uint8((float64(a1>>8)*(1-weight) + float64(a2>>8)*weight)),
	}
}

// WithAlpha returns a color with modified alpha value
func WithAlpha(c color.Color, alpha uint8) color.Color {
	r, g, b, _ := c.RGBA()
	return color.NRGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: alpha,
	}
}
