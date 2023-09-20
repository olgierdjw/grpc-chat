package client

import (
	"github.com/rivo/tview"
	"sort"
)

type Iterator[T any] interface {
	addItem(T, int)
	getCurrent() T
	next() T
}

type FocusElement struct {
	priority int
	element  *tview.Box
}

type ActiveBoxManager struct {
	guiElements  []FocusElement
	currentIndex int
}

func (fn *ActiveBoxManager) addItem(guiElement *tview.Box, priority int) {
	fn.guiElements = append(fn.guiElements, FocusElement{priority, guiElement})
	sort.Slice(fn.guiElements, func(i, j int) bool {
		return fn.guiElements[i].priority < fn.guiElements[j].priority
	})
}

func (fn *ActiveBoxManager) next() *tview.Box {
	nextIndex := (fn.currentIndex + 1) % len(fn.guiElements)
	fn.currentIndex = nextIndex
	return fn.guiElements[nextIndex].element
}

func (fn *ActiveBoxManager) getCurrent() *tview.Box {
	return fn.guiElements[fn.currentIndex].element
}
