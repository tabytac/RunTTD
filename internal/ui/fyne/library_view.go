package fyne

import (
	"context"
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	apppkg "runttd/internal/app"
	"runttd/internal/domain"
	"runttd/internal/platform"
)

// osDisplayLabel maps a raw OS/arch tag to a friendly label, or returns it as-is.
// Linux is handled generically so non-generic builds (dedicated, debian, ubuntu)
// read as "Linux x64 (dedicated)" etc. rather than going untagged.
func osDisplayLabel(tag string) string {
	switch tag {
	case "windows-win64":
		return "Windows 64-bit"
	case "windows-win32":
		return "Windows 32-bit"
	case "windows-arm64":
		return "Windows ARM64"
	case "mingw-win64":
		return "Windows 64-bit"
	case "mingw-win32":
		return "Windows 32-bit"
	case "macos-universal":
		return "macOS"
	case "macosx-universal":
		return "macOS"
	}
	if strings.HasPrefix(tag, "linux-") {
		arch := "x64"
		switch {
		case strings.HasSuffix(tag, "-arm64"):
			arch = "ARM64"
		case strings.HasSuffix(tag, "-i386"):
			arch = "32-bit"
		}
		variant := strings.TrimPrefix(tag, "linux-")
		variant = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(variant, "-amd64"), "-arm64"), "-i386")
		switch {
		case variant == "" || variant == "generic":
			return "Linux " + arch
		case variant == "dedicated":
			return "Linux dedicated server" // headless, not a playable client
		}
		if i := strings.IndexByte(variant, '-'); i > 0 {
			variant = variant[:i] // "debian-bookworm" -> "debian"
		}
		return "Linux " + arch + " (" + variant + ")"
	}
	return tag
}

// clientDisplayName maps a client id to its library group header label.
func clientDisplayName(client string) string {
	switch client {
	case "jgrpp":
		return "JGRPP"
	case "vanilla":
		return "Vanilla OpenTTD (Releases)"
	case "vanilla-nightly":
		return "Vanilla OpenTTD (Nightly)"
	default:
		return "Unrecognized"
	}
}

// shortClientLabel returns a compact client tag for tight spaces like the
// profile list row. An empty client resolves to the configured default.
func shortClientLabel(client, defaultClient string) string {
	if client == "" {
		client = defaultClient
		if client == "" {
			client = "jgrpp"
		}
	}
	switch client {
	case "jgrpp":
		return "JGRPP"
	case "vanilla":
		return "Vanilla"
	case "vanilla-nightly":
		return "Nightly"
	case "custom":
		return "Custom"
	default:
		return client
	}
}

// humanSize formats a byte count as a short human-readable string.
func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// statusColors for the row color-bar and chip. Green = in use, amber = unused.
var (
	libGreen  = color.NRGBA{R: 61, G: 153, B: 61, A: 255}   // #3D993D
	libAmber  = color.NRGBA{R: 230, G: 167, B: 0, A: 255}   // #E6A700
	libGrey   = color.NRGBA{R: 120, G: 125, B: 130, A: 255} // neutral pill for "unrecognized"
	libChipFg = color.NRGBA{R: 255, G: 255, B: 255, A: 255} // white text on colored pills
)

// statusChip returns a small rounded colored pill with bold fg text on fill.
func statusChip(text string, fill, fg color.Color) fyne.CanvasObject {
	label := canvas.NewText(text, fg)
	label.TextStyle = fyne.TextStyle{Bold: true}
	label.TextSize = theme.CaptionTextSize()
	bg := canvas.NewRectangle(fill)
	bg.CornerRadius = 8
	return container.NewStack(bg, container.NewPadded(label))
}

// showLibraryView presents the full-screen Installed Clients library, opening in
// a scanning state and populating rows on a background goroutine.
func (um *UIManager) showLibraryView() {
	summary := widget.NewLabel("Scanning installed clients...")
	summary.TextStyle = fyne.TextStyle{Bold: true}

	listBox := container.NewVBox()

	scanGen := 0

	backBtn := widget.NewButton("Back", func() {
		scanGen++ // invalidate any in-flight scan render
		um.Window.SetContent(um.makeMainView())
	})

	var cleanupBtn *widget.Button
	var rescan func()

	render := func(entries []domain.LibraryEntry) {
		listBox.Objects = nil
		var total, unusedBytes int64
		unusedCount := 0
		var orphans []domain.LibraryEntry

		// Empty state.
		if len(entries) == 0 {
			summary.SetText("No clients installed yet")
			empty := widget.NewLabel("Versions you download will appear here.")
			empty.Alignment = fyne.TextAlignCenter
			empty.Wrapping = fyne.TextWrapWord
			listBox.Add(container.NewPadded(empty))
			cleanupBtn.SetText("Clean up unused")
			cleanupBtn.Disable()
			listBox.Refresh()
			return
		}

		// Tally totals/orphans up front (orphan = no profile AND a recognized client).
		for _, e := range entries {
			total += e.SizeBytes
			if len(e.ReferencedBy) == 0 && e.Client != "" {
				unusedCount++
				unusedBytes += e.SizeBytes
				orphans = append(orphans, e)
			}
		}

		// Render grouped by client, version-desc within each group.
		for _, g := range apppkg.GroupLibrary(entries, um.Config) {
			head := NewSectionHeader(fmt.Sprintf("%s — %d %s",
				clientDisplayName(g.Client), len(g.Entries), pluralVersions(len(g.Entries), g.Client)))
			listBox.Add(head)
			for _, e := range g.Entries {
				listBox.Add(um.libraryRow(e, rescan))
			}
		}

		// Summary: emphasize reclaimable space.
		if unusedCount > 0 {
			summary.SetText(fmt.Sprintf("%d %s · %s total · %d unused · %s reclaimable",
				len(entries), plural(len(entries), "version"), humanSize(total), unusedCount, humanSize(unusedBytes)))
			cleanupBtn.SetText(fmt.Sprintf("Clean up %d unused · free %s", unusedCount, humanSize(unusedBytes)))
			cleanupBtn.Enable()
			cleanupBtn.OnTapped = func() {
				um.confirmCleanup(orphans, rescan)
			}
		} else {
			summary.SetText(fmt.Sprintf("%d %s · %s total · none unused",
				len(entries), plural(len(entries), "version"), humanSize(total)))
			cleanupBtn.SetText("Clean up unused")
			cleanupBtn.Disable()
		}
		listBox.Refresh()
	}

	rescan = func() {
		scanGen++
		gen := scanGen
		summary.SetText("Scanning installed clients...")
		listBox.Objects = nil
		listBox.Refresh()
		go func() {
			entries := apppkg.BuildLibrary(context.Background(), um.Config)
			fyne.Do(func() {
				if gen != scanGen {
					return // a newer scan or Back superseded this one
				}
				render(entries)
			})
		}()
	}

	cleanupBtn = widget.NewButton("Clean up unused", func() {})
	cleanupBtn.Importance = widget.HighImportance
	cleanupBtn.Disable()

	refreshBtn := widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() { rescan() })

	header := container.NewBorder(nil, nil, summary, container.NewHBox(refreshBtn))
	footer := container.NewBorder(nil, nil, backBtn, cleanupBtn)

	content := container.NewBorder(
		container.NewPadded(header),
		container.NewPadded(footer),
		nil, nil,
		container.NewVScroll(container.NewPadded(listBox)),
	)
	um.Window.SetContent(content)
	rescan()
}

// pluralVersions returns the singular/plural noun for a group header count
// ("versions", or "folders" for the unrecognized group).
// plural returns noun with an "s" appended unless n == 1.
func plural(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

func pluralVersions(n int, client string) string {
	noun := "version"
	if client == "" {
		noun = "folder"
	}
	return plural(n, noun)
}

// libraryRow builds one grouped row: status color-bar, version title with an
// inline status chip, muted size/date, and icon-only actions.
func (um *UIManager) libraryRow(e domain.LibraryEntry, afterChange func()) fyne.CanvasObject {
	// Title is the version (client is the group header); fall back to folder base.
	titleText := e.Version
	if titleText == "" {
		titleText = filepath.Base(e.Path)
	}
	titleLabel := widget.NewLabel(titleText)
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}

	var chipText string
	var barColor color.Color = color.Transparent
	var chipFill color.Color = libGrey
	switch {
	case len(e.ReferencedBy) > 0:
		chipText = "IN USE · " + strings.Join(e.ReferencedBy, ", ")
		barColor = libGreen
		chipFill = libGreen
	case e.Client == "":
		chipText = "UNRECOGNIZED"
		chipFill = libGrey
	default:
		chipText = "UNUSED"
		barColor = libAmber
		chipFill = libAmber
	}
	chip := statusChip(chipText, chipFill, libChipFg)

	// Title row: version, muted OS/arch label, then the status pill.
	titleObjects := []fyne.CanvasObject{titleLabel}
	if e.OSTag != "" {
		osLabel := widget.NewLabel(osDisplayLabel(e.OSTag))
		osLabel.TextStyle = fyne.TextStyle{Italic: true}
		titleObjects = append(titleObjects, osLabel)
	}
	titleObjects = append(titleObjects, chip)

	meta := widget.NewLabel(fmt.Sprintf("%s · %s", humanSize(e.SizeBytes), e.ModTime.Format("2006-01-02")))

	revealBtn := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
		if err := platform.RevealInFileManager(e.Path); err != nil {
			um.Logger.Append(fmt.Sprintf("Reveal failed for %s: %v", e.Path, err))
			dialog.ShowError(fmt.Errorf("couldn't open the folder: %w", err), um.Window)
		}
	})
	revealBtn.Importance = widget.LowImportance
	deleteBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		um.confirmDeleteOne(e, afterChange)
	})
	deleteBtn.Importance = widget.LowImportance

	titleRow := container.NewHBox(titleObjects...)
	left := container.NewVBox(titleRow, meta)
	actions := container.NewHBox(revealBtn, deleteBtn)

	bar := canvas.NewRectangle(barColor)
	bar.SetMinSize(fyne.NewSize(4, 1))

	rowInner := container.NewBorder(nil, nil, bar, actions, container.NewPadded(left))
	return container.NewPadded(rowInner)
}

// confirmDeleteOne asks before deleting a single folder, warning if referenced.
func (um *UIManager) confirmDeleteOne(e domain.LibraryEntry, afterChange func()) {
	msg := fmt.Sprintf("Delete this folder?\n\n%s\n(%s)", e.Path, humanSize(e.SizeBytes))
	if len(e.ReferencedBy) > 0 {
		msg += "\n\nWarning: used by profile(s): " + strings.Join(e.ReferencedBy, ", ")
	}
	dialog.NewConfirm("Delete Installed Version", msg, func(ok bool) {
		if !ok {
			return
		}
		if err := platform.DeleteInstalledVersion(um.Config, e.Path); err != nil {
			dialog.ShowError(err, um.Window)
			return
		}
		um.Logger.Append("Deleted installed version: " + e.Path)
		afterChange()
	}, um.Window).Show()
}

// confirmCleanup removes all orphan folders after a single confirm.
func (um *UIManager) confirmCleanup(orphans []domain.LibraryEntry, afterChange func()) {
	var list string
	var total int64
	for _, e := range orphans {
		list += fmt.Sprintf("\n• %s (%s)", e.Path, humanSize(e.SizeBytes))
		total += e.SizeBytes
	}
	msg := fmt.Sprintf("Remove %d unused version(s), freeing %s?%s", len(orphans), humanSize(total), list)
	dialog.NewConfirm("Clean Up Unused Versions", msg, func(ok bool) {
		if !ok {
			return
		}
		var failed int
		for _, e := range orphans {
			if err := platform.DeleteInstalledVersion(um.Config, e.Path); err != nil {
				failed++
				um.Logger.Append(fmt.Sprintf("Cleanup failed for %s: %v", e.Path, err))
			} else {
				um.Logger.Append("Cleanup removed: " + e.Path)
			}
		}
		if failed > 0 {
			dialog.ShowError(fmt.Errorf("%d folder(s) could not be removed; see logs", failed), um.Window)
		}
		afterChange()
	}, um.Window).Show()
}
