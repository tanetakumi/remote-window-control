package input_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"share-app-host/internal/input"
	"share-app-host/internal/win32"
	"share-app-host/test/testutil"
)

type pcDesktop struct {
	recordingKeys
	hit, front win32.HWND
	denied     bool
	buttonErr  error
	releaseErr error
	mouse      []string
}

func (p *pcDesktop) CaptureRect(win32.HWND) (win32.Rect, error) {
	return win32.Rect{Left: -100, Top: 50, Right: 100, Bottom: 150}, nil
}
func (p *pcDesktop) VirtualDesktop() (win32.Rect, error) {
	return win32.Rect{Left: -200, Right: 200, Bottom: 200}, nil
}
func (p *pcDesktop) WindowAt(int32, int32) win32.HWND    { return p.hit }
func (p *pcDesktop) ForegroundWindow() win32.HWND        { return p.front }
func (p *pcDesktop) IsOwnedBy(h, target win32.HWND) bool { return target == 7 && (h == 7 || h == 8) }
func (p *pcDesktop) Activate(h win32.HWND) error {
	p.calls = append(p.calls, "activate")
	if p.denied {
		return win32.ErrForegroundDenied
	}
	p.front = h
	return nil
}
func (p *pcDesktop) MovePointer(x, y int32) error {
	p.mouse = append(p.mouse, fmt.Sprintf("move:%d,%d", x, y))
	return nil
}
func (p *pcDesktop) MouseButton(b win32.Button, up bool) error {
	p.mouse = append(p.mouse, fmt.Sprintf("button:%d:%v", b, up))
	if up && p.releaseErr != nil {
		return p.releaseErr
	}
	return p.buttonErr
}
func (p *pcDesktop) Wheel(delta int32) error {
	p.mouse = append(p.mouse, fmt.Sprintf("wheel:%d", delta))
	return nil
}

func TestPCRejectsObscuredPressesButAllowsMove(t *testing.T) {
	for _, kind := range []string{"down", "tap", "scroll"} {
		t.Run(kind, func(t *testing.T) {
			desk := &pcDesktop{hit: 9, front: 8}
			s := input.NewSendInputInjectorWithDesktop(fakeTarget{7, true}, maxScale, desk)
			var err error
			switch kind {
			case "down":
				err = s.MouseDown("left", .5, .5)
			case "tap":
				err = s.Tap("left", .5, .5)
			case "scroll":
				err = s.Scroll(1, .5, .5)
			}
			if !errors.Is(err, input.ErrObscured) || len(desk.mouse) != 0 {
				t.Fatalf("err=%v input=%v", err, desk.mouse)
			}
			if err = s.MouseUp("left", .5, .5); err != nil || len(desk.mouse) != 0 {
				t.Fatal("unowned release leaked")
			}
			if err = s.Move(.5, .5); err != nil || len(desk.mouse) != 1 {
				t.Fatal("move should not need family check")
			}
		})
	}
}
func TestPCReleasesWithoutTargetAndRetriesFailedMouseRelease(t *testing.T) {
	desk := &pcDesktop{hit: 8, front: 8}
	target := &mutablePCTarget{7}
	s := input.NewSendInputInjectorWithDesktop(target, maxScale, desk)
	if err := s.MouseDown("left", .5, .5); err != nil {
		t.Fatal(err)
	}
	target.handle = 0
	desk.hit = 9
	desk.front = 9
	desk.buttonErr = errors.New("failure")
	if err := s.MouseUp("left", 0, 0); err == nil {
		t.Fatal("release failure lost")
	}
	desk.buttonErr = nil
	if err := s.MouseUp("left", 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.KeyUp(input.Command{Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(desk.mouse[1:], []string{"button:0:false", "button:0:true", "button:0:true"}) {
		t.Fatal(desk.mouse)
	}
	if !slices.Equal(desk.calls, []string{"send enter up"}) {
		t.Fatal(desk.calls)
	}
}

type mutablePCTarget struct{ handle uint64 }

func (t *mutablePCTarget) CurrentHandle() (uint64, bool) { return t.handle, t.handle != 0 }
func TestPCPopupKeepsFocusForTextAndKeys(t *testing.T) {
	desk := &pcDesktop{hit: 8, front: 8}
	s := input.NewSendInputInjectorWithDesktop(fakeTarget{7, true}, maxScale, desk)
	if err := s.Text("hello", false); err != nil {
		t.Fatal(err)
	}
	if err := s.KeyDown(input.Command{Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	if err := s.KeyUp(input.Command{Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(desk.calls, "activate") {
		t.Fatal("popup was reactivated")
	}
	want := []string{`clipboard "hello"`, "send ctrl down", "send v down", "send v up", "send ctrl up", "send enter down", "send enter up"}
	if !slices.Equal(desk.calls, want) {
		t.Fatalf("calls=%v", desk.calls)
	}
}
func TestPCForegroundDenialBlocksKeysTextAndWheel(t *testing.T) {
	desk := &pcDesktop{hit: 8, front: 9, denied: true}
	s := input.NewSendInputInjectorWithDesktop(fakeTarget{7, true}, maxScale, desk)
	for _, action := range []func() error{func() error { return s.Text("hello", false) }, func() error { return s.KeyDown(input.Command{Key: "Enter"}) }, func() error { return s.Scroll(1, .5, .5) }} {
		if err := action(); !errors.Is(err, win32.ErrForegroundDenied) {
			t.Fatal(err)
		}
	}
	if len(desk.mouse) != 0 || !slices.Equal(desk.calls, []string{"activate", "activate", "activate"}) {
		t.Fatalf("keys=%v mouse=%v", desk.calls, desk.mouse)
	}
}
func TestPCTapAndWheelUseOrderedRealInput(t *testing.T) {
	desk := &pcDesktop{hit: 8, front: 8}
	s := input.NewSendInputInjectorWithDesktop(fakeTarget{7, true}, maxScale, desk)
	if err := s.Tap("right", .5, .5); err != nil {
		t.Fatal(err)
	}
	if err := s.Scroll(2, .5, .5); err != nil {
		t.Fatal(err)
	}
	if len(desk.mouse) != 5 || desk.mouse[1] != "button:1:false" || desk.mouse[2] != "button:1:true" || desk.mouse[4] != "wheel:-240" {
		t.Fatal(desk.mouse)
	}
}

func TestPCFailedTapReleaseIsRetriedOnDisconnect(t *testing.T) {
	desk := &pcDesktop{hit: 8, front: 8, releaseErr: errors.New("release failed")}
	pc := input.NewSendInputInjectorWithDesktop(fakeTarget{7, true}, maxScale, desk)
	d := input.NewDispatcher(&testutil.RecordingInjector{})
	d.SetPCInjector(pc)
	d.SetModeHandler(func(string, bool) {})
	d.Activate()
	if err := modeDispatch(t, d, input.Command{Type: input.TypeMode, Mode: input.ModePC}); err != nil {
		t.Fatal(err)
	}
	d.CompleteMode()
	if err := modeDispatch(t, d, input.Command{Type: input.TypeTap, Button: "left", X: .5, Y: .5}); err == nil {
		t.Fatal("tap release failure lost")
	}
	desk.releaseErr = nil
	d.ReleaseAll()
	if want := []string{"button:0:false", "button:0:true", "button:0:true"}; !slices.Equal(desk.mouse[1:], want) {
		t.Fatalf("disconnect did not retry tap release: %v", desk.mouse)
	}
}
