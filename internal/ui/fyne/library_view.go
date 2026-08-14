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

// clientDisplayName maps a client id to its library group header label; the
// registry is the one table of client names, so the two cannot drift.
func clientDisplayName(client string) string {
	if name := apppkg.ClientDisplayName(client); name != "" {
		return name
	}
	return "Unrecognised"
}

// shortClientLabel returns a compact client tag for tight spaces like the
// profile list row. An empty client resolves to the configured default.
func shortClientLabel(client, defaultClient string) string {
	switch apppkg.EffectiveClient(client, defaultClient) {
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
	libGrey   = color.NRGBA{R: 120, G: 125, B: 130, A: 255} // neutral pill for "unrecognised"
	libChipFg = color.NRGBA{R: 255, G: 255, B: 255, A: 255} // white text on colored pills
	dotRed    = color.NRGBA{R: 229, G: 57, B: 53, A: 255}   // #E53935 status-dot "not installed"
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

// newLibraryButton returns a library-view button that answers F5 as well as
// Escape. Buttons are the only focusable widgets in this view, and a focused one
// swallows both keys before the canvas fallback in main_view.go sees them.
func (um *UIManager) newLibraryButton(label string, tapped func()) *dialogButton {
	b := newDialogButton(label, tapped, um.runViewEscape)
	b.onRefresh = um.runLibraryRescan
	return b
}

// showLibraryView presents the full-screen Installed Clients library, opening in
// a scanning state and populating rows on a background goroutine.
func (um *UIManager) showLibraryView() {
	summary := widget.NewLabel("Scanning installed clients…")
	summary.TextStyle = fyne.TextStyle{Bold: true}

	listBox := container.NewVBox()

	scanGen := 0
	deleting := false

	back := func() {
		scanGen++ // invalidate any in-flight scan render
		um.libraryRescan = nil
		um.viewEscape = nil
		um.Window.SetContent(um.makeMainView())
	}
	backBtn := um.newLibraryButton("Back", back)

	var cleanupBtn *dialogButton
	var rescan func()
	var busyControls func(active bool) // toggles the surrounding controls off while a delete or one-off launch runs
	var busy func(active bool)         // delete's bracket: busyControls plus the Removing… summary line

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
		launch := func(e domain.LibraryEntry) { um.launchInstalledEntry(e, busyControls) }
		for _, g := range apppkg.GroupLibrary(entries, um.Config) {
			head := NewSectionHeader(fmt.Sprintf("%s: %d %s",
				clientDisplayName(g.Client), len(g.Entries), pluralVersions(len(g.Entries), g.Client)))
			listBox.Add(head)
			for _, e := range g.Entries {
				listBox.Add(um.libraryRow(e, launch, busy, rescan))
			}
		}

		// Summary: emphasize reclaimable space.
		if unusedCount > 0 {
			summary.SetText(fmt.Sprintf("%d %s · %s total · %d unused · %s reclaimable",
				len(entries), plural(len(entries), "version"), humanSize(total), unusedCount, humanSize(unusedBytes)))
			cleanupBtn.SetText(fmt.Sprintf("Clean up %d unused · free %s", unusedCount, humanSize(unusedBytes)))
			cleanupBtn.Enable()
			cleanupBtn.OnTapped = func() {
				um.confirmCleanup(orphans, busy, rescan)
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
		summary.SetText("Scanning installed clients…")
		listBox.Objects = nil
		listBox.Refresh()
		cfg := um.snapshotConfig()
		go func() {
			entries := apppkg.BuildLibrary(context.Background(), cfg)
			fyne.Do(func() {
				if gen != scanGen {
					return // a newer scan or Back superseded this one
				}
				render(entries)
			})
		}()
	}

	cleanupBtn = um.newLibraryButton("Clean up unused", func() {})
	cleanupBtn.Importance = widget.DangerImportance
	cleanupBtn.Disable()

	refreshBtn := um.newLibraryButton("Refresh", func() { rescan() })
	refreshBtn.Icon = theme.ViewRefreshIcon()

	// A delete can be slow (a large or network-hosted folder), so it runs off the UI
	// thread; busyControls disables the surrounding controls for that stretch, and
	// also brackets a one-off launch's brief start window. render()'s own
	// Enable/Disable of cleanupBtn on completion is left alone: rescan (called
	// after busy(false)) re-derives whether anything is still unused. Per-row delete
	// buttons are deliberately left enabled, so two different rows can delete
	// concurrently; each targets its own path, and a stale rescan from one is
	// superseded by the other's, so this is accepted rather than gated.
	busyControls = func(active bool) {
		deleting = active
		if active {
			um.viewEscape = nil // Escape leaves this view too, so it goes with the disabled Back button
			backBtn.Disable()
			cleanupBtn.Disable()
			refreshBtn.Disable()
			return
		}
		// A slower concurrent delete can land after Back; don't revive a dead view's hook.
		if um.libraryRescan != nil {
			um.viewEscape = back
		}
		backBtn.Enable()
		refreshBtn.Enable()
	}
	busy = func(active bool) {
		if active {
			summary.SetText("Removing…")
		}
		busyControls(active)
	}

	header := container.NewBorder(nil, nil, summary, container.NewHBox(refreshBtn))
	footer := container.NewBorder(nil, nil, backBtn, cleanupBtn)

	content := container.NewBorder(
		container.NewPadded(header),
		container.NewPadded(footer),
		nil, nil,
		container.NewVScroll(container.NewPadded(listBox)),
	)
	// A rescan mid-delete re-enables the very controls busy() just disabled.
	um.libraryRescan = func() {
		if !deleting {
			rescan()
		}
	}
	um.viewEscape = back
	um.Window.SetContent(content)
	rescan()
}

// pluralVersions returns the singular/plural noun for a group header count
// ("versions", or "folders" for the unrecognised group).
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
func (um *UIManager) libraryRow(e domain.LibraryEntry, launch func(domain.LibraryEntry), busy func(bool), afterChange func()) fyne.CanvasObject {
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
		chipText = "UNRECOGNISED"
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

	launchBtn := um.newLibraryButton("", func() { launch(e) })
	launchBtn.Icon = theme.MediaPlayIcon()
	launchBtn.Importance = widget.LowImportance
	revealBtn := um.newLibraryButton("", func() {
		if err := platform.RevealInFileManager(e.Path); err != nil {
			um.Logger.Append(fmt.Sprintf("Reveal failed for %s: %v", e.Path, err))
			um.showErrorf("could not open the folder: %w", err)
		}
	})
	revealBtn.Icon = theme.FolderOpenIcon()
	revealBtn.Importance = widget.LowImportance
	deleteBtn := um.newLibraryButton("", func() {
		um.confirmDeleteOne(e, busy, afterChange)
	})
	deleteBtn.Icon = theme.DeleteIcon()
	deleteBtn.Importance = widget.DangerImportance

	titleRow := container.NewHBox(titleObjects...)
	left := container.NewVBox(titleRow, meta)
	actions := container.NewHBox(launchBtn, revealBtn, deleteBtn)

	bar := canvas.NewRectangle(barColor)
	bar.SetMinSize(fyne.NewSize(4, 1))

	rowInner := container.NewBorder(nil, nil, bar, actions, container.NewPadded(left))
	return container.NewPadded(rowInner)
}

// confirmDeleteOne asks before deleting a single folder, warning if referenced. The
// delete runs off the UI thread (RemoveAll can be slow on a large or network-hosted
// folder); busy(true/false) brackets it so the window doesn't look frozen meanwhile.
func (um *UIManager) confirmDeleteOne(e domain.LibraryEntry, busy func(bool), afterChange func()) {
	msg := fmt.Sprintf("Delete this folder?\n\n%s\n(%s)", e.Path, humanSize(e.SizeBytes))
	if len(e.ReferencedBy) > 0 {
		msg += "\n\nWarning: used by profile(s): " + strings.Join(e.ReferencedBy, ", ")
	}
	confirmDlg := um.newConfirmDialog("Delete installed version", "Delete", "Cancel", msg, func(ok bool) {
		if !ok {
			return
		}
		busy(true)
		cfg := um.snapshotConfig()
		go func() {
			err := platform.DeleteInstalledVersion(cfg, e.Path)
			fyne.Do(func() {
				busy(false)
				if err != nil {
					um.showErrorf("could not delete installed version: %w", err)
				} else {
					um.Logger.Append("Deleted installed version: " + e.Path)
				}
				um.diskLookups.invalidate() // a removed folder must not still read as installed
				// Always rescan, success or failure: it re-derives the summary and
				// cleanupBtn's enabled state, which busy(false) alone doesn't restore.
				afterChange()
			})
		}()
	})
	confirmDlg.SetConfirmImportance(widget.DangerImportance)
	confirmDlg.Show()
}

// launchInstalledEntry starts a one-off launch of an installed folder, as it
// sits on disk and with no profile settings. It shares the cross-path launch
// guard with profile launches; busy disables the view's controls for the brief
// start window, so Back cannot land while the guard is set and hit adoptLaunch
// with no profile index to adopt. Feedback is a toast or an error dialog.
func (um *UIManager) launchInstalledEntry(e domain.LibraryEntry, busy func(bool)) {
	if um.launchInProgress {
		um.showToast("A launch is already running")
		return
	}
	um.launchInProgress = true
	busy(true)
	title := e.Version
	if title == "" {
		title = filepath.Base(e.Path)
	}
	cfg := um.snapshotConfig()
	um.startAsync(func() {
		result := apppkg.LaunchInstalledFolder(e.Path, e.Client, apppkg.LaunchDeps{
			Config:   cfg,
			Logger:   um.Logger,
			Observer: um,
		})
		fyne.Do(func() {
			um.launchInProgress = false
			busy(false)
			if result == apppkg.LaunchStarted {
				um.showToast("Launched " + title)
			} else {
				um.showErrorf("could not start OpenTTD from %s; see the logs for details", e.Path)
			}
		})
	})
}

// confirmCleanup removes all orphan folders after a single confirm, off the UI thread.
func (um *UIManager) confirmCleanup(orphans []domain.LibraryEntry, busy func(bool), afterChange func()) {
	var list string
	var total int64
	for _, e := range orphans {
		list += fmt.Sprintf("\n• %s (%s)", e.Path, humanSize(e.SizeBytes))
		total += e.SizeBytes
	}
	msg := fmt.Sprintf("Remove %d unused version(s), freeing %s?%s", len(orphans), humanSize(total), list)
	confirmDlg := um.newConfirmDialog("Clean up unused versions", "Clean up", "Cancel", msg, func(ok bool) {
		if !ok {
			return
		}
		busy(true)
		cfg := um.snapshotConfig()
		go func() {
			var failed int
			for _, e := range orphans {
				if err := platform.DeleteInstalledVersion(cfg, e.Path); err != nil {
					failed++
					um.Logger.Append(fmt.Sprintf("Cleanup failed for %s: %v", e.Path, err))
				} else {
					um.Logger.Append("Cleanup removed: " + e.Path)
				}
			}
			fyne.Do(func() {
				busy(false)
				if failed > 0 {
					um.showErrorf("%d folder(s) could not be removed; see logs", failed)
				}
				um.diskLookups.invalidate() // removed folders must not still read as installed
				afterChange()
			})
		}()
	})
	confirmDlg.SetConfirmImportance(widget.DangerImportance)
	confirmDlg.Show()
}
