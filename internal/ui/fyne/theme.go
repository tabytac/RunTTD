package fyne

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	ColorNameSidebar fyne.ThemeColorName = "launcherSidebar"
	ColorNameContent fyne.ThemeColorName = "launcherContent"
	ColorNameHeader  fyne.ThemeColorName = "launcherHeader"
)

// ThemePreset represents a user accent color selection
type ThemePreset struct {
	Name     string
	LightHex string
	DarkHex  string
}

// ThemePresets defines the 8 hand-tuned launcher accent color options
var ThemePresets = []ThemePreset{
	{"Green", "#2D912D", "#3D993D"},
	{"Orange", "#E67300", "#FF8000"},
	{"Red", "#B71C1C", "#D32F2F"},
	{"Blue", "#1565C0", "#1976D2"},
	{"Purple", "#6A1B9A", "#7B1FA2"},
	{"Teal", "#00695C", "#00796B"},
	{"Gold", "#F9A825", "#E6A700"},
	{"Slate", "#37474F", "#455A64"},
}

// LauncherTheme implements Fyne's custom system theming contract
type LauncherTheme struct {
	fyne.Theme
	OverrideVariant *fyne.ThemeVariant
	AccentDark      color.NRGBA
	AccentLight     color.NRGBA
}

// UpdateAccent dynamically changes primary elements colors based on preset selections and light/dark configurations
func (p *LauncherTheme) UpdateAccent(presetIdx int, variant string) {
	preset := ThemePresets[presetIdx]
	light, _ := ParseHexColor(preset.LightHex)
	dark, _ := ParseHexColor(preset.DarkHex)
	p.AccentLight = light
	p.AccentDark = dark

	if variant == "light" {
		v := theme.VariantLight
		p.OverrideVariant = &v
	} else if variant == "dark" {
		v := theme.VariantDark
		p.OverrideVariant = &v
	} else {
		p.OverrideVariant = nil
	}
	fyne.CurrentApp().Settings().SetTheme(p)
}

// Color maps customized colors to the application UI widgets
func (p *LauncherTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if p.OverrideVariant != nil {
		variant = *p.OverrideVariant
	}

	accent := p.AccentLight
	if variant == theme.VariantDark {
		accent = p.AccentDark
	}

	if variant == theme.VariantDark {
		switch name {
		case theme.ColorNameBackground, theme.ColorNameOverlayBackground:
			return color.NRGBA{R: 25, G: 25, B: 25, A: 255} // Neutral Dark BG
		case theme.ColorNameInputBackground:
			return color.NRGBA{R: 32, G: 32, B: 32, A: 255} // Neutral Dark Input
		case ColorNameSidebar:
			return color.NRGBA{R: 19, G: 19, B: 19, A: 255} // Neutral Dark Sidebar
		case ColorNameContent:
			return color.NRGBA{R: 29, G: 29, B: 29, A: 255} // Neutral Dark Content
		case ColorNameHeader:
			return color.NRGBA{R: 35, G: 35, B: 35, A: 255} // Neutral Dark Header

		case theme.ColorNameSelection:
			return withAlpha(accent, 115) // 45% Opacity
		case theme.ColorNameHover:
			return withAlpha(accent, 51) // 20% Opacity
		case theme.ColorNamePrimary:
			return accent
		}
	} else {
		switch name {
		case theme.ColorNameBackground, theme.ColorNameOverlayBackground:
			return color.NRGBA{R: 246, G: 246, B: 246, A: 255}
		case theme.ColorNameInputBackground:
			return color.NRGBA{R: 238, G: 238, B: 238, A: 255}
		case ColorNameSidebar:
			return color.NRGBA{R: 236, G: 236, B: 236, A: 255}
		case ColorNameContent:
			return color.NRGBA{R: 250, G: 250, B: 250, A: 255}
		case ColorNameHeader:
			return color.NRGBA{R: 224, G: 224, B: 224, A: 255}
		case theme.ColorNameSelection:
			return withAlpha(accent, 115) // 45% Opacity
		case theme.ColorNameHover:
			return withAlpha(accent, 51) // 20% Opacity
		case theme.ColorNamePrimary:
			return accent
		}
	}
	return p.Theme.Color(name, variant)
}

func withAlpha(c color.Color, alpha uint8) color.Color {
	if c == nil {
		return color.Transparent
	}
	r, g, b, _ := c.RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: alpha}
}

// ThemedBox renders custom theme-aware backgrounds behind GUI panels
type ThemedBox struct {
	widget.BaseWidget
	Content   fyne.CanvasObject
	ColorName fyne.ThemeColorName
}

// NewThemedBox creates a container decorated with theme colors
func NewThemedBox(colorName fyne.ThemeColorName, content fyne.CanvasObject) *ThemedBox {
	b := &ThemedBox{Content: content, ColorName: colorName}
	b.ExtendBaseWidget(b)
	return b
}

// CreateRenderer creates the UI render layout for our themed container box
func (b *ThemedBox) CreateRenderer() fyne.WidgetRenderer {
	rect := canvas.NewRectangle(theme.Color(b.ColorName))
	return &themedBoxRenderer{rect: rect, content: b.Content, b: b}
}

type themedBoxRenderer struct {
	rect    *canvas.Rectangle
	content fyne.CanvasObject
	b       *ThemedBox
}

func (r *themedBoxRenderer) Layout(size fyne.Size) {
	r.rect.Resize(size)
	r.content.Resize(size)
}

func (r *themedBoxRenderer) MinSize() fyne.Size {
	return r.content.MinSize()
}

func (r *themedBoxRenderer) Refresh() {
	r.rect.FillColor = theme.Color(r.b.ColorName)
	r.rect.Refresh()
	r.content.Refresh()
}

func (r *themedBoxRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.rect, r.content}
}

func (r *themedBoxRenderer) Destroy() {}

// SegmentedRadio renders interactive grouped buttons matching a single radio value state
type SegmentedRadio struct {
	Container *fyne.Container
	Selected  string
	Options   []string
	Buttons   []*widget.Button
	OnChanged func(string)
}

// NewSegmentedRadio renders a row of equal-sized toggle buttons
func NewSegmentedRadio(options []string, initial string, onChanged func(string)) *SegmentedRadio {
	s := &SegmentedRadio{
		Options:   options,
		Selected:  initial,
		OnChanged: onChanged,
		Buttons:   make([]*widget.Button, len(options)),
	}

	for i, opt := range options {
		label := opt
		btn := widget.NewButton(label, func() {
			s.SetSelected(label)
			if s.OnChanged != nil {
				s.OnChanged(label)
			}
		})
		if label == initial {
			btn.Importance = widget.HighImportance
		} else {
			btn.Importance = widget.LowImportance
		}
		s.Buttons[i] = btn
	}

	s.Container = container.NewGridWithColumns(len(options))
	for _, b := range s.Buttons {
		s.Container.Add(b)
	}

	return s
}

// SetSelected toggles the highlighted button state to denote active selection
func (s *SegmentedRadio) SetSelected(label string) {
	s.Selected = label
	for _, b := range s.Buttons {
		if b.Text == label {
			b.Importance = widget.HighImportance
		} else {
			b.Importance = widget.LowImportance
		}
		b.Refresh()
	}
}

func (s *SegmentedRadio) Hide() {
	s.Container.Hide()
}

func (s *SegmentedRadio) Show() {
	s.Container.Show()
}

func (s *SegmentedRadio) Refresh() {
	s.Container.Refresh()
}

// ParseHexColor decodes a hexadecimal hex color string (#FFFFFF) to a NRGBA color model
func ParseHexColor(s string) (color.NRGBA, error) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.NRGBA{}, fmt.Errorf("invalid hex length")
	}
	r, err := strconv.ParseUint(s[0:2], 16, 8)
	if err != nil {
		return color.NRGBA{}, err
	}
	g, err := strconv.ParseUint(s[2:4], 16, 8)
	if err != nil {
		return color.NRGBA{}, err
	}
	b, err := strconv.ParseUint(s[4:6], 16, 8)
	if err != nil {
		return color.NRGBA{}, err
	}
	return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}, nil
}
