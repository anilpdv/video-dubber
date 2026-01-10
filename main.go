package main

import (
	"video-translator/ui"
	"video-translator/ui/theme"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func main() {
	a := app.New()
	a.Settings().SetTheme(&theme.VideoTranslatorTheme{})

	w := a.NewWindow("Video Translator")
	w.Resize(fyne.NewSize(1100, 750))

	mainUI := ui.NewMainUI(w)
	w.SetContent(mainUI.Build())

	w.ShowAndRun()
}
