package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	appTheme "video-translator/ui/theme"
)

// NavItem represents a navigation item
type NavItem struct {
	ID    string
	Icon  fyne.Resource
	Label string
}

// SidebarNav is a vertical navigation sidebar
type SidebarNav struct {
	widget.BaseWidget

	Items       []NavItem
	SelectedID  string
	OnSelected  func(id string)
	itemWidgets []*navItemWidget
}

// NewSidebarNav creates a new sidebar navigation
func NewSidebarNav(items []NavItem, onSelected func(id string)) *SidebarNav {
	s := &SidebarNav{
		Items:      items,
		OnSelected: onSelected,
	}
	if len(items) > 0 {
		s.SelectedID = items[0].ID
	}
	s.ExtendBaseWidget(s)
	return s
}

// SetSelected sets the selected item by ID
func (s *SidebarNav) SetSelected(id string) {
	s.SelectedID = id
	s.Refresh()
}

// CreateRenderer implements fyne.Widget
func (s *SidebarNav) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)

	// App title/logo
	title := canvas.NewText("Video Translator", color.White)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = 16

	// Create nav item widgets
	s.itemWidgets = make([]*navItemWidget, len(s.Items))
	for i, item := range s.Items {
		idx := i
		s.itemWidgets[i] = newNavItemWidget(item, func() {
			s.SelectedID = s.Items[idx].ID
			if s.OnSelected != nil {
				s.OnSelected(s.Items[idx].ID)
			}
			s.Refresh()
		})
	}

	return &sidebarNavRenderer{
		bg:          bg,
		title:       title,
		itemWidgets: s.itemWidgets,
		widget:      s,
	}
}

type sidebarNavRenderer struct {
	bg          *canvas.Rectangle
	title       *canvas.Text
	itemWidgets []*navItemWidget
	widget      *SidebarNav
}

func (r *sidebarNavRenderer) Destroy() {}

func (r *sidebarNavRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)

	padding := float32(16)
	y := padding

	// Title
	titleSize := r.title.MinSize()
	r.title.Move(fyne.NewPos(padding, y))
	y += titleSize.Height + padding*2

	// Nav items
	itemHeight := float32(44)
	for _, item := range r.itemWidgets {
		item.Resize(fyne.NewSize(size.Width, itemHeight))
		item.Move(fyne.NewPos(0, y))
		y += itemHeight + 4
	}
}

func (r *sidebarNavRenderer) MinSize() fyne.Size {
	padding := float32(16)
	height := padding + r.title.MinSize().Height + padding*2
	height += float32(len(r.itemWidgets)) * 48
	return fyne.NewSize(200, height)
}

func (r *sidebarNavRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.bg, r.title}
	for _, item := range r.itemWidgets {
		objs = append(objs, item)
	}
	return objs
}

func (r *sidebarNavRenderer) Refresh() {
	th := fyne.CurrentApp().Settings().Theme()
	variant := fyne.CurrentApp().Settings().ThemeVariant()

	r.bg.FillColor = th.Color(appTheme.ColorNameSidebar, variant)
	r.bg.Refresh()

	r.title.Color = th.Color(theme.ColorNameForeground, variant)
	r.title.Refresh()

	// Update item selection states
	for i, item := range r.itemWidgets {
		item.selected = r.widget.Items[i].ID == r.widget.SelectedID
		item.Refresh()
	}
}

// navItemWidget is a single navigation item
type navItemWidget struct {
	widget.BaseWidget

	item     NavItem
	onTapped func()
	selected bool
	hovered  bool
}

func newNavItemWidget(item NavItem, onTapped func()) *navItemWidget {
	w := &navItemWidget{
		item:     item,
		onTapped: onTapped,
	}
	w.ExtendBaseWidget(w)
	return w
}

func (n *navItemWidget) Tapped(_ *fyne.PointEvent) {
	if n.onTapped != nil {
		n.onTapped()
	}
}

func (n *navItemWidget) TappedSecondary(_ *fyne.PointEvent) {}

func (n *navItemWidget) MouseIn(_ *desktop.MouseEvent) {
	n.hovered = true
	n.Refresh()
}

func (n *navItemWidget) MouseOut() {
	n.hovered = false
	n.Refresh()
}

func (n *navItemWidget) MouseMoved(_ *desktop.MouseEvent) {}

func (n *navItemWidget) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
	bg.CornerRadius = 8

	// Active indicator bar
	indicator := canvas.NewRectangle(color.Transparent)
	indicator.CornerRadius = 2

	icon := canvas.NewImageFromResource(n.item.Icon)
	icon.FillMode = canvas.ImageFillContain

	label := canvas.NewText(n.item.Label, color.White)
	label.TextSize = 14

	return &navItemRenderer{
		bg:        bg,
		indicator: indicator,
		icon:      icon,
		label:     label,
		widget:    n,
	}
}

type navItemRenderer struct {
	bg        *canvas.Rectangle
	indicator *canvas.Rectangle
	icon      *canvas.Image
	label     *canvas.Text
	widget    *navItemWidget
}

func (r *navItemRenderer) Destroy() {}

func (r *navItemRenderer) Layout(size fyne.Size) {
	padding := float32(12)
	iconSize := float32(20)

	// Background fills most of the item
	bgMargin := float32(8)
	r.bg.Resize(fyne.NewSize(size.Width-bgMargin*2, size.Height-4))
	r.bg.Move(fyne.NewPos(bgMargin, 2))

	// Active indicator on the left
	r.indicator.Resize(fyne.NewSize(3, size.Height-16))
	r.indicator.Move(fyne.NewPos(4, 8))

	// Icon
	r.icon.Resize(fyne.NewSize(iconSize, iconSize))
	r.icon.Move(fyne.NewPos(padding+8, (size.Height-iconSize)/2))

	// Label
	labelY := (size.Height - r.label.MinSize().Height) / 2
	r.label.Move(fyne.NewPos(padding+8+iconSize+12, labelY))
}

func (r *navItemRenderer) MinSize() fyne.Size {
	return fyne.NewSize(180, 44)
}

func (r *navItemRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.indicator, r.icon, r.label}
}

func (r *navItemRenderer) Refresh() {
	th := fyne.CurrentApp().Settings().Theme()
	variant := fyne.CurrentApp().Settings().ThemeVariant()

	// Background color based on state
	if r.widget.selected {
		r.bg.FillColor = th.Color(appTheme.ColorNameSidebarActive, variant)
		r.indicator.FillColor = th.Color(theme.ColorNamePrimary, variant)
		r.label.Color = th.Color(theme.ColorNameForeground, variant)
	} else if r.widget.hovered {
		r.bg.FillColor = th.Color(appTheme.ColorNameSidebarHover, variant)
		r.indicator.FillColor = color.Transparent
		r.label.Color = th.Color(theme.ColorNameForeground, variant)
	} else {
		r.bg.FillColor = color.Transparent
		r.indicator.FillColor = color.Transparent
		r.label.Color = th.Color(appTheme.ColorNameTextSecondary, variant)
	}

	r.bg.Refresh()
	r.indicator.Refresh()
	r.icon.Refresh()
	r.label.Refresh()
}
