//go:build !linux || android

package native

func wmNoDecorations()                        {}
func wmSaveGeometry()                         {}
func wmRestoreGeometry()                      {}
func wmShadeToggle(int)                       {}
func wmShadeFull(string, int)                 {}
func wmShadeSet(string, int, int) bool        { return false }
func wmResize(string, int, int)               {}
func wmResizeDrag(string, int, int, int, int) {}
func wmResizeWDrag(string, int, int, int)     {}
func wmOntop(string, bool) bool               { return false }
func wmCenter(string) bool                    { return false }
func wmBelow(string, string, int) bool        { return false }
