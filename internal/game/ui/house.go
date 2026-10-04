package ui

import (
	"fmt"
	"io"
	"log"
	"time"

	"libre-enroth/internal/assets/vid"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
	"libre-enroth/internal/media/smacker"
)

// videos opens Anims/mightdod.vid and magicdod.vid once.
//
// mm8: 0x4beb80 (Video_OpenContainers)
func (r *Resources) videos() (vid.Set, error) {
	if r.vids == nil && r.vidErr == nil {
		r.vids, r.vidErr = vid.OpenSet(r.Cache.Data().Dir)
	}
	return r.vids, r.vidErr
}

// houseVideo plays a house clip in the left area, looping at the clip's frame rate (the
// game ticks it whenever SmackWait says the next frame is due).
//
// mm8: 0x4bf45a (Video_OpenHouse: frame 0 at once), 0x4bf248 (Video_HouseFrame at
// (viewport.clipX1 + 8, clipY1 - 6) = (8, 23)), 0x46261d (Game_Loop: SmackWait)
type houseVideo struct {
	v       *smacker.Video
	frame   *gfx.Sprite
	elapsed time.Duration
	stopped bool
}

// House clip position.
const houseVideoX, houseVideoY = 8, 23

func openHouseVideo(r *Resources, name string) *houseVideo {
	set, err := r.videos()
	if err == nil {
		var rd io.ReaderAt
		if rd, err = set.Find(name + ".smk"); err == nil {
			var v *smacker.Video
			if v, err = smacker.Open(rd.(*io.SectionReader)); err == nil {
				v.Loop = true
				h := &houseVideo{v: v}
				h.next()
				return h
			}
		}
	}
	log.Printf("house video %q: %v", name, err)
	return nil
}

func (h *houseVideo) next() {
	img, err := h.v.NextFrame()
	if err != nil {
		// The last frame of a clip that does not loop stays (Video_Stop).
		h.stopped = true
		return
	}
	if h.frame == nil {
		h.frame = &gfx.Sprite{W: img.Rect.Dx(), H: img.Rect.Dy(), Pix: make([]byte, 4*img.Rect.Dx()*img.Rect.Dy())}
	}
	var lut [256][4]byte
	for i, c := range img.Palette {
		r, g, b, _ := c.RGBA()
		lut[i] = [4]byte{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), 0xff}
	}
	for i, p := range img.Pix {
		copy(h.frame.Pix[4*i:4*i+4], lut[p][:])
	}
}

// tick advances by one 60 Hz tick.
func (h *houseVideo) tick(loop bool) {
	h.v.Loop = loop
	if h.stopped {
		return
	}
	h.elapsed += time.Second / 60
	for d := h.v.FrameDuration(); h.elapsed >= d && !h.stopped; h.elapsed -= d {
		h.next()
	}
}

// rewind restarts the clip.
//
// mm8: 0x4bf7a1 (Video_Rewind)
func (h *houseVideo) rewind() {
	h.v.Rewind()
	h.elapsed, h.stopped = 0, false
	h.next()
}

func (h *houseVideo) draw(c *gfx.Canvas) {
	if h != nil && h.frame != nil {
		c.Blit(h.frame, houseVideoX, houseVideoY)
	}
}

// drawHouse is the house screen: the clip, the frame, the house name, then either the
// portraits (with the reply under the clip) or the selected portrait's dialogue.
//
// mm8: 0x4b3dec
func (s *dialogScreen) drawHouse(c *gfx.Canvas) {
	a, d := s.art, s.d
	s.video.draw(c)
	a.drawFrame(c, a.topbar2, 0x15, false)
	if !d.OnExit() && d.Def.Name != "" {
		h := textHeight(a.create, d.Def.Name, 0x82, 0)
		y := max(a.create.Height*2-6-h, 0)/2 + 4
		a.create.DrawCentered(c, text.Rect{W: 622, H: gfx.ScreenH}, 0x1ea, y, inkWhite, d.Def.Name, 3)
	}
	count := len(d.Portraits)
	names := text.Rect{W: 630, H: gfx.ScreenH}
	switch {
	case d.Sel == 0:
		if d.Reply != "" {
			a.drawReply(c, d.Reply, 0x1ca, 0xd, func(int) bool { return false })
		}
		for i, p := range d.Portraits {
			x, y := s.portraitPos(i, count)
			s.drawPortrait(c, p.Icon, x, y)
			if count >= 4 {
				continue
			}
			var name string
			ny := 0x71
			switch {
			case p.Exit:
				name = d.ExitName()
				ny = i*0x5e + 0x71
			case i == 0 && d.Proprietor:
				name = d.Def.Title
			default:
				name = d.NPCName(p.NPC)
				ny = y + 2 + a.icon(p.Icon).H
			}
			a.create.DrawCentered(c, names, 0x1e3, ny, inkName, name, 3)
		}
	default:
		x, y := s.portraitPos(0, 1)
		s.drawPortrait(c, d.Portraits[d.Sel-1].Icon, x, y)
		if d.OnProprietor() {
			t := fmt.Sprintf(s.r.GlobalText(0x1ad), d.Def.Owner, d.Def.Title)
			a.create.DrawCentered(c, names, 0x1e3, 0x71, inkName, t, 3)
			drawTopics(c, a.arrus, s.spots, s.hover) // the services: M6c draws them per type
		} else {
			s.drawResident(c)
		}
	}
	if d.OnExit() {
		// x_x_u and x_ok_u are not in icons.lod: the original draws PENDING twice here.
		c.Blit(a.pending, 0x22c, 0x1c3)
		c.Blit(a.pending, 0x1dc, 0x1c3)
	}
}

// drawResident is a selected house portrait: the "Other Exits" question, or a resident's
// name, greeting, topics and reply.
//
// mm8: 0x4b363b
func (s *dialogScreen) drawResident(c *gfx.Canvas) {
	a, d := s.art, s.d
	if d.OnExit() {
		a.create.DrawCentered(c, text.Rect{X: 0x1ed, W: 0x7e, H: gfx.ScreenH}, 0, 2, 0, d.ExitName(), 3)
		t := d.ExitText()
		h := textHeight(a.create, t, 0x94, 0)
		a.create.DrawCentered(c, text.Rect{X: 0x1e3, W: 0x94, H: gfx.ScreenH}, 0, (0xd4-h)/2+0x65, 0, t, 3)
		drawTopics(c, a.arrus, s.spots, s.hover)
		return
	}
	a.create.DrawCentered(c, text.Rect{W: 630, H: gfx.ScreenH}, 0x1e3, 0x71, inkName, d.Name(), 3)
	if g := d.Greeting(); g != "" {
		a.drawReply(c, g, 0x1cc, 0xd, func(int) bool { return false })
	}
	drawTopics(c, a.arrus, s.spots, s.hover)
	if d.Reply != "" {
		a.drawReply(c, d.Reply, 0x1ca, 0xd, func(th int) bool { return Viewport.Max.Y-1-th < Viewport.Min.Y })
	}
}
