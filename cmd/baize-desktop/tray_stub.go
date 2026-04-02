//go:build !windows

package main

func newDesktopTrayController(*DesktopApp) (desktopTrayController, error) {
	return nil, nil
}
