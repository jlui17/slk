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

	w, h, measured := CellPixels(0)
	if w != 10 || h != 20 || !measured {
		t.Errorf("got (%d,%d,%v), want (10,20,true)", w, h, measured)
	}
}

func TestCellPixels_FallbackWhenNoEnvAndNoFD(t *testing.T) {
	saved := getenv
	defer func() { getenv = saved }()
	getenv = func(k string) string { return "" }

	// fd = -1 forces ioctl to fail.
	w, h, measured := CellPixels(-1)
	if w != 8 || h != 16 || measured {
		t.Errorf("got (%d,%d,%v), want (8,16,false) fallback", w, h, measured)
	}
}
