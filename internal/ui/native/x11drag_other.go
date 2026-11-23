//go:build !linux

package native

// wmDrag* são no-op fora do Linux; outros SOs usam system.ActionMove.
func wmDragBegin() {}
func wmDragMove()  {}
func wmDragEnd()   {}
