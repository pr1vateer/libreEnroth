package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"libre-enroth/internal/assets/vid"
	"libre-enroth/internal/media/smacker"
)

func vidList(p string) error {
	v, err := vid.Open(p)
	if err != nil {
		return err
	}
	defer v.Close()
	for _, e := range v.Entries {
		info := ""
		if sv, err := smacker.Open(v.Reader(e)); err == nil {
			info = fmt.Sprintf("SMK%c %dx%d %d frames %v/frame flags %#x", sv.Version, sv.Width, sv.Height, sv.Frames, sv.FrameDuration(), sv.Flags)
		} else if !strings.HasSuffix(strings.ToLower(e.Name), ".smk") {
			info = "-"
		} else {
			info = "error: " + err.Error()
		}
		fmt.Printf("%-24s %10d %10d  %s\n", e.Name, e.Offset, e.Size, info)
	}
	return nil
}

func vidPNG(o options) error {
	v, err := vid.Open(o.args[1])
	if err != nil {
		return err
	}
	defer v.Close()
	name := o.args[2]
	e, ok := v.Find(name)
	if !ok {
		if e, ok = v.Find(name + ".smk"); !ok {
			return fmt.Errorf("%s: %w", name, vid.ErrNotFound)
		}
	}
	frame := 0
	if len(o.args) == 4 {
		if frame, err = strconv.Atoi(o.args[3]); err != nil || frame < 0 {
			return errors.New("frame must be a number >= 0")
		}
	}
	sv, err := smacker.Open(v.Reader(e))
	if err != nil {
		return err
	}
	sv.Loop = true
	for i := 0; ; i++ {
		img, err := sv.NextFrame()
		if err != nil {
			return err
		}
		if i == frame {
			out := fmt.Sprintf("%s_%03d", strings.TrimSuffix(e.Name, ".smk"), frame)
			if err := writePNG(o.outDir, out, img); err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		}
	}
}
