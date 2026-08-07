//go:build windows

package platform

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// CreateShortcut writes a .lnk into destDir and returns the file it wrote.
//
// It drives WScript.Shell through COM, the same interface Explorer's own "Create
// shortcut" uses, so the result is an ordinary shell link with no RunTTD-specific
// format to maintain.
func CreateShortcut(destDir string, spec ShortcutSpec) (path string, err error) {
	stem, err := validateShortcutSpec(destDir, spec)
	if err != nil {
		return "", err
	}
	lnkPath := filepath.Join(destDir, stem+".lnk")

	// COM apartments are per-thread, so every call below has to land on the same
	// one; a bare goroutine could be rescheduled between them.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// A panic here would otherwise cross the COM boundary and take the process
	// with it, and on the -H=windowsgui build that death is completely silent.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("creating the shortcut failed: %v", r)
		}
	}()

	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		// S_FALSE means this thread was already initialised, which is harmless.
		if oleErr, ok := err.(*ole.OleError); !ok || oleErr.Code() != 1 {
			return "", fmt.Errorf("could not initialise COM: %w", err)
		}
	}
	defer ole.CoUninitialize()

	shellObj, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return "", fmt.Errorf("Windows Script Host is unavailable: %w", err)
	}
	defer shellObj.Release()

	shell, err := shellObj.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return "", fmt.Errorf("could not talk to Windows Script Host: %w", err)
	}
	defer shell.Release()

	created, err := oleutil.CallMethod(shell, "CreateShortcut", lnkPath)
	if err != nil {
		return "", fmt.Errorf("could not create %s: %w", lnkPath, err)
	}
	// ToIDispatch borrows the variant's pointer rather than adding a reference, so
	// releasing the interface is the whole cleanup; clearing the variant too would
	// free it twice and take the process down with an access violation.
	link := created.ToIDispatch()
	defer link.Release()

	for _, prop := range []struct{ name, value string }{
		{"TargetPath", spec.ExePath},
		{"Arguments", windowsArgString(spec.Args)},
		{"WorkingDirectory", spec.WorkDir},
		{"Description", spec.Description},
		{"IconLocation", spec.ExePath + ",0"},
	} {
		if _, err := oleutil.PutProperty(link, prop.name, prop.value); err != nil {
			return "", fmt.Errorf("could not set the shortcut's %s: %w", prop.name, err)
		}
	}
	if _, err := oleutil.CallMethod(link, "Save"); err != nil {
		return "", fmt.Errorf("could not save %s: %w", lnkPath, err)
	}
	return lnkPath, nil
}

// windowsArgString joins args so CommandLineToArgvW splits them back into the
// same strings. An argument needing no quotes is passed through; otherwise a
// backslash run is doubled only where it precedes a quote (including the closing
// one), which is the rule that parser actually applies.
func windowsArgString(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "" && !strings.ContainsAny(arg, " \t\"") {
			quoted = append(quoted, arg)
			continue
		}
		var b strings.Builder
		b.WriteByte('"')
		for i := 0; i < len(arg); i++ {
			slashes := 0
			for i < len(arg) && arg[i] == '\\' {
				slashes++
				i++
			}
			switch {
			case i == len(arg):
				b.WriteString(strings.Repeat(`\`, slashes*2))
			case arg[i] == '"':
				b.WriteString(strings.Repeat(`\`, slashes*2+1))
				b.WriteByte('"')
			default:
				b.WriteString(strings.Repeat(`\`, slashes))
				b.WriteByte(arg[i])
			}
		}
		b.WriteByte('"')
		quoted = append(quoted, b.String())
	}
	return strings.Join(quoted, " ")
}
