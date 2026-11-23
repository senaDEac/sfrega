//go:build !linux

package native

func wmNoDecorations()  {}
func wmSaveGeometry()   {}
func wmRestoreGeometry() {}
func wmShadeToggle(int)      {}
func wmShadeFull(string, int) {}
