package ui

import (
	"errors"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/locales"
)

func TestKubeShell(t *testing.T) {
	a, tt, _ := openShop(t)
	var opened kube.Shell
	var finish func(error)
	a.OpenKubeShell = func(target kube.Shell, done func(error)) { opened, finish = target, done }
	tt.Frame()
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatalf("%v (pod %q, texts %q)", err, a.cluster.pod, tt.Texts())
		}
		tt.Advance(time.Second)
	}
	click("checkout-api-7d9f8c5b-x2k4p")
	click(locales.KubeShellAction())
	hasTexts(t, tt, locales.KubeShellTitle(), locales.KubeShellMissingSh())
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		tt.SetSize(size[0], size[1])
		tt.Advance(time.Second)
		copyButton, copied := tt.Find(locales.KubeShellCopy())
		launchButton, launched := tt.Find(locales.KubeShellLaunch())
		if !copied || !launched || copyButton.Y != launchButton.Y || copyButton.H != launchButton.H ||
			launchButton.X+launchButton.W > float32(size[0]) || launchButton.Y+launchButton.H > float32(size[1]) {
			t.Fatalf("actions outside popover layout at %v: %+v %+v", size, copyButton, launchButton)
		}
		screenshot(t, tt, "kube-shell-"+map[int]string{1280: "normal", 1000: "minimum"}[size[0]])
	}
	click(locales.KubeShellContainer())
	click("envoy")
	click(locales.KubeShellCopy())
	if finish != nil {
		t.Fatal("copy launched the terminal")
	}
	click(locales.KubeShellLaunch())
	if opened.Context != "kind-dev" || opened.Namespace != "default" || opened.Pod != "checkout-api-7d9f8c5b-x2k4p" || opened.Container != "envoy" {
		t.Fatalf("opened %+v", opened)
	}
	if got := tt.Clipboard(); got != run.TerminalCommand(opened.Args()) {
		t.Fatalf("copied %q, opened %q", got, opened.Args())
	}
	finish(errors.New("test launch failure"))
	tt.Advance(time.Second)
	hasTexts(t, tt, locales.KubeShellLaunchFailed("test launch failure"))
	click(locales.KubeShellLaunch())
	finish(nil)
	tt.Advance(time.Second)
	if tt.HasText(locales.KubeShellTitle()) {
		t.Fatal("successful launch kept the popover open")
	}
}
