package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/internal/sysenv"
	"github.com/Blinks44/hearth/locales"
)

func TestKubeBulkSelection(t *testing.T) {
	a, tt, _ := openShop(t)
	a.BuildPlan = func(func(sysenv.Env, ops.Terminal) (ops.Plan, error), func(ops.Plan, error)) {}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
	}
	a.cluster.query = "checkout"
	tt.Frame()
	click(locales.KubeBulkSelect("checkout-api-7d9f8c5b-x2k4p"))
	if !a.cluster.bulk.selected["p1"] || a.cluster.pod != "" {
		t.Fatalf("click selected %+v, opened pod %q", a.cluster.bulk.selected, a.cluster.pod)
	}
	hasTexts(t, tt, locales.KubeBulkSelected(1, 1))
	click(locales.KubeBulkSelect("checkout-api"))
	hasTexts(t, tt, locales.KubeBulkSelected(3, 1))
	for _, size := range [][2]int{{1280, 820}, {1000, 680}} {
		tt.SetSize(size[0], size[1])
		tt.Advance(time.Second)
		restart, r := tt.Find(locales.KubeBulkRecreate() + "…")
		remove, d := tt.Find(locales.KubeBulkDelete() + "…")
		clear, c := tt.Find(locales.KubeBulkClear())
		if !r || !d || !c || restart.Y != remove.Y || remove.Y != clear.Y ||
			restart.H != remove.H || remove.H != clear.H || clear.X+clear.W > float32(size[0]) {
			t.Fatalf("misaligned actions at %v: %+v %+v %+v", size, restart, remove, clear)
		}
		screenshot(t, tt, "kube-bulk-"+map[int]string{1280: "normal", 1000: "minimum"}[size[0]])
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if len(a.cluster.bulk.selected) != 0 {
		t.Fatal("Escape kept the selection")
	}
	click(locales.KubeBulkSelectAll())
	if len(a.cluster.bulk.selected) != 3 {
		t.Fatalf("select all included rows outside the filter: %+v", a.cluster.bulk.selected)
	}
	a.cluster.kind = kindDeployments
	tt.Frame()
	if len(a.cluster.bulk.selected) != 0 {
		t.Fatal("selection followed the user to another resource kind")
	}
}
