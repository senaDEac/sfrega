//go:build !linux

package native

func wmNoDecorations()  {}
func wmSaveGeometry()   {}
func wmRestoreGeometry() {}
func wmShadeToggle(int)      {}
func wmShadeFull(string, int) {}
func wmShadeSet(string, int, int) bool { return false }
func wmResize(string, int, int)        {}
func wmOntop(string, bool)             {}
func wmCenter(string) bool              { return false }
