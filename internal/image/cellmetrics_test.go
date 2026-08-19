package image

import "testing"

func TestCellPixels_EnvOverride(t *testing.T) {
	saved := getenv
	defer func() { getenv = saved }()
	getenv = func(k string) string {
		switch k {
		case "COLORTERM_CELL_WIDTH":
			return "10"
		case "COLORTERM_CELL_HEIGHT":
			return "20"
		}
		return ""
	}

	w, h := CellPixels(0)
	if w != 10 || h != 20 {
		t.Errorf("got (%d,%d), want (10,20)", w, h)
	}
}

func TestCellPixels_FallbackWhenNoEnvAndNoFD(t *testing.T) {
	saved := getenv
	defer func() { getenv = saved }()
	getenv = func(k string) string { return "" }

	// fd = -1 forces ioctl to fail.
	w, h := CellPixels(-1)
	if w != 8 || h != 16 {
		t.Errorf("got (%d,%d), want (8,16) fallback", w, h)
	}
}

func resetCellPixels(t *testing.T) {
	t.Helper()
	cellPxW.Store(0)
	cellPxH.Store(0)
}

func TestCellPixels_DefaultsTo8x16(t *testing.T) {
	resetCellPixels(t)
	w, h := cellPixels()
	if w != 8 || h != 16 {
		t.Errorf("cellPixels() = %dx%d, want 8x16 default", w, h)
	}
}

func TestSetCellPixels_UsesMeasuredMetrics(t *testing.T) {
	resetCellPixels(t)
	SetCellPixels(14, 33)
	t.Cleanup(func() { resetCellPixels(t) })

	if w, h := cellPixels(); w != 14 || h != 33 {
		t.Errorf("cellPixels() = %dx%d, want 14x33", w, h)
	}
}

func TestSetCellPixels_IgnoresNonPositive(t *testing.T) {
	resetCellPixels(t)
	SetCellPixels(0, 33)
	SetCellPixels(14, -1)
	if w, h := cellPixels(); w != 8 || h != 16 {
		t.Errorf("cellPixels() = %dx%d, want the 8x16 default to survive bad input", w, h)
	}
}
