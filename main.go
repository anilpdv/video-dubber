package main

import (
	"video-translator/ui"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func main() {
	a := app.New()
	a.Settings().SetTheme(&ui.TranslatorTheme{})

	w := a.NewWindow("Video Translator")
	w.Resize(fyne.NewSize(1000, 700))

	mainUI := ui.NewMainUI(w)
	w.SetContent(mainUI.Build())

	w.ShowAndRun()
}
