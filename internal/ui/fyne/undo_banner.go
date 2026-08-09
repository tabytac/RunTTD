package fyne

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"

	"fyne.io/fyne/v2/widget"

	"runttd/internal/domain"
)

// deleteSelected confirms, then removes the selected profile, reselects a
// neighbour, and offers an undo. Refuses to delete the last remaining profile;
// updateButtonStates already disables Delete there, this is just the backstop.
func (mv *mainView) deleteSelected() {
	um := mv.um
	if mv.selectedIdx < 0 {
		return
	}
	if len(um.Config.Profiles) <= 1 {
		um.showError("cannot delete the last profile")
		return
	}
	profileName := um.Config.Profiles[mv.selectedIdx].Name
	confirmDlg := um.newConfirmDialog("Delete profile", "Delete", "Cancel",
		fmt.Sprintf("Are you sure you want to delete profile %q?", profileName),
		func(confirmed bool) {
			if confirmed {
				mv.performDelete()
			}
		},
	)
	confirmDlg.SetConfirmImportance(widget.DangerImportance)
	confirmDlg.Show()
}

// performDelete removes the selected profile, selects a neighbour, and arms the
// undo offer. Split from the confirm callback so a test can drive the real path.
func (mv *mainView) performDelete() {
	um := mv.um
	deleted, deletedIdx := um.Config.Profiles[mv.selectedIdx], mv.selectedIdx
	um.Config.Profiles = append(um.Config.Profiles[:mv.selectedIdx], um.Config.Profiles[mv.selectedIdx+1:]...)
	um.saveConfigOrWarn()

	nextIdx := mv.selectedIdx
	if nextIdx >= len(um.Config.Profiles) {
		nextIdx = len(um.Config.Profiles) - 1
	}

	mv.selectedIdx = nextIdx
	um.SelectedProfileName = um.Config.Profiles[mv.selectedIdx].Name
	mv.recomputeVisible()
	mv.profileList.Refresh()
	if d := mv.displayPos(mv.selectedIdx); d >= 0 {
		mv.profileList.Select(widget.ListItemID(d))
	} else {
		mv.profileList.UnselectAll()
	}
	mv.refreshDetails()
	mv.updateEmptyState()
	mv.armUndo(deleted, deletedIdx)
	mv.showUndoBanner(deleted)
}

func (mv *mainView) showUndoBanner(deleted domain.Profile) {
	mv.undoLabel.SetText(fmt.Sprintf("Deleted %q", deleted.Name))
	mv.undoBand.Show()
	mv.undoBand.Refresh()

	// Through startAsync so tests can drive a delete without leaving a timer running.
	gen := mv.undoGen
	mv.um.startAsync(func() {
		time.Sleep(10 * time.Second)
		fyne.Do(func() {
			if mv.shouldAutoHideUndo(gen) {
				mv.hideUndoBanner()
			}
		})
	})
}

// armUndo records the pending restore; a later deletion always wins the slot.
func (mv *mainView) armUndo(deleted domain.Profile, idx int) {
	mv.undoProfile = &deleted
	mv.undoIdx = idx
	mv.undoGen++
}

// shouldAutoHideUndo reports whether the timer started for gen may still retire
// the banner: only if no later deletion has bumped undoGen.
func (mv *mainView) shouldAutoHideUndo(gen int) bool {
	return gen == mv.undoGen
}

func (mv *mainView) hideUndoBanner() {
	mv.undoProfile = nil
	mv.undoBand.Hide()
}

// undoDelete puts the pending profile back at the index it was deleted from and
// reselects it. A failed save rolls the restore back, so memory can't diverge
// from disk while the banner still claims the profile is recoverable.
func (mv *mainView) undoDelete() {
	um := mv.um
	if mv.undoProfile == nil {
		return
	}
	restored := *mv.undoProfile
	restored.Name = restoredProfileName(um.Config.Profiles, restored.Name)
	before := um.Config.Profiles
	um.Config.Profiles = restoreProfileAt(before, restored, mv.undoIdx)
	if !um.saveConfigOrWarn() {
		um.Config.Profiles = before
		return
	}
	mv.hideUndoBanner()

	um.SelectedProfileName = restored.Name
	mv.selectedIdx = indexOfProfileByName(um.Config.Profiles, restored.Name)
	mv.recomputeVisible()
	// The filter can have changed since the delete; a restore the user cannot see
	// looks like nothing happened, so clear it rather than restore off-screen.
	if mv.displayPos(mv.selectedIdx) < 0 {
		mv.filterText = ""
		mv.searchEntry.SetText("")
		mv.recomputeVisible()
	}
	mv.profileList.Refresh()
	if d := mv.displayPos(mv.selectedIdx); d >= 0 {
		mv.profileList.Select(widget.ListItemID(d))
	} else {
		mv.profileList.UnselectAll()
	}
	mv.refreshDetails()
	mv.updateEmptyState()
}

// restoreProfileAt reinserts p at idx (clamped to the ends) and returns a new
// slice, leaving profiles untouched so a caller can roll back to it.
func restoreProfileAt(profiles []domain.Profile, p domain.Profile, idx int) []domain.Profile {
	if idx < 0 {
		idx = 0
	}
	if idx > len(profiles) {
		idx = len(profiles)
	}
	out := make([]domain.Profile, 0, len(profiles)+1)
	out = append(out, profiles[:idx]...)
	out = append(out, p)
	out = append(out, profiles[idx:]...)
	return out
}
