package flet

import "time"

// Client methods that the generator cannot derive from the Python SDK.
// The rest are in methods_gen.go.

// Enable enables the browser's context menu (web only).
func (c *BrowserContextMenu) Enable() error {
	_, err := c.invoke("enable_menu", nil)
	return err
}

// Disable disables the browser's context menu (web only).
func (c *BrowserContextMenu) Disable() error {
	_, err := c.invoke("disable_menu", nil)
	return err
}

// MoveToOptions are the optional arguments of Tabs.MoveTo.
type MoveToOptions struct {
	// AnimationCurve: the curve of the animation.
	AnimationCurve AnimationCurve
	// AnimationDuration: the duration of the animation. Defaults to
	// Tabs.AnimationDuration.
	AnimationDuration time.Duration
}

// MoveTo selects the tab at index with an animation. A negative index counts
// from the end.
func (c *Tabs) MoveTo(index int, opts ...MoveToOptions) error {
	m := map[string]any{"index": index}
	d := c.AnimationDuration
	if len(opts) > 0 {
		setIf(m, "curve", opts[0].AnimationCurve)
		if opts[0].AnimationDuration != 0 {
			d = opts[0].AnimationDuration
		}
	}
	if d != 0 {
		m["duration"] = d.Milliseconds()
	}
	_, err := c.invoke("move_to", m)
	return err
}
