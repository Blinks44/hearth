package ui

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
)

func TestKubeSecrets(t *testing.T) {
	for _, width := range []int{1000, 1280} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a := kubeApp()
			var watches clusterWatches
			a.WatchCluster = watches.watch
			type request struct {
				key      string
				done     func([]byte, error)
				canceled bool
			}
			var reads []*request
			a.ReadSecretValue = func(target kube.Target, secret kube.Secret, key string, done func([]byte, error)) func() {
				t.Helper()
				if target.Context != "kind-dev" || target.Kubectl == "" || secret.Namespace != "default" || secret.UID != "s1" {
					t.Fatalf("unexpected target: %+v, %+v", target, secret)
				}
				r := &request{key: key, done: done}
				reads = append(reads, r)
				return func() { r.canceled = true }
			}
			type expiry struct {
				delay time.Duration
				fire  func()
			}
			var timers []expiry
			a.SecretTimer = func(delay time.Duration, done func()) func() {
				timers = append(timers, expiry{delay, done})
				return func() {}
			}

			a.openCluster("kind-dev")
			tt := ui.NewTester(a.View, width, 680)
			tt.Advance(time.Second)
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			click("Secrets")
			if running := watches.running(); len(running) != 1 || !running[0].Secrets {
				t.Fatalf("unexpected watches: %+v", running)
			}
			fixture := kube.Cluster{Taken: time.Now(), Secrets: []kube.Secret{
				{UID: "s1", Name: "checkout-db", Namespace: "default", Type: "Opaque", Created: time.Now(),
					Keys: []kube.SecretKey{{Name: "DB_HOST", Bytes: 15}, {Name: "DB_USER", Bytes: 4}, {Name: "DB_PASSWORD", Bytes: 12}}},
				{UID: "s2", Name: "chart", Namespace: "default", Type: "helm.sh/release.v1", Helm: true},
				{UID: "s3", Name: "shop-tls", Namespace: "default", Type: "kubernetes.io/tls"},
			}}
			watches.got[1](fixture, nil)
			tt.Frame()
			click("checkout-db")
			hasTexts(t, tt, "TYPE · KEYS", "Keys", "DB_HOST", "DB_USER", "DB_PASSWORD")
			if len(reads) != 0 {
				t.Fatal("opening the inspector read a value")
			}
			host, ok := tt.Find("DB_HOST")
			if !ok {
				t.Fatal("missing key label")
			}
			action := func(label string) ui.Rect {
				t.Helper()
				r, ok := tt.Find(label)
				if !ok {
					t.Fatalf("missing %q", label)
				}
				return r
			}
			show, copyButton := action("Show DB_HOST"), action("Copy DB_HOST")
			if host.X >= show.X || show.X+show.W > copyButton.X || show.Y != copyButton.Y || show.H != copyButton.H {
				t.Fatalf("Secret row geometry: label=%+v show=%+v copy=%+v", host, show, copyButton)
			}
			screenshot(t, tt, fmt.Sprintf("secrets-hidden-%d", width))

			click("Show DB_HOST")
			if len(reads) != 1 || reads[0].key != "DB_HOST" {
				t.Fatal("show did not read exactly the intended key")
			}
			first := []byte("postgres.s10.svc")
			reads[0].done(first, nil)
			tt.Frame()
			hasTexts(t, tt, "Hides in 30 s")
			if tt.HasText("postgres.s10.svc") || timers[0].delay != 30*time.Second {
				t.Fatal("revealed value reached the UI text tree or had the wrong lifetime")
			}
			screenshot(t, tt, fmt.Sprintf("secrets-revealed-%d", width))
			click("Show DB_USER")
			if !bytes.Equal(first, make([]byte, len(first))) || a.cluster.secret.revealed != nil {
				t.Fatal("the previous value survived a new reveal")
			}
			second := []byte("demo")
			reads[1].done(second, nil)
			tt.Frame()
			timers[0].fire()
			if a.cluster.secret.revealed == nil || a.cluster.secret.revealed.key != "DB_USER" {
				t.Fatal("an old timer hid a newer value")
			}
			timers[1].fire()
			tt.Frame()
			if a.cluster.secret.revealed != nil || !bytes.Equal(second, make([]byte, len(second))) {
				t.Fatal("expired value remained in the inspector")
			}

			click("Copy DB_PASSWORD")
			copied := []byte("fixture-only")
			reads[2].done(copied, nil)
			tt.Frame()
			if a.cluster.secret.revealed != nil || tt.Clipboard() != "fixture-only" ||
				!bytes.Equal(copied, make([]byte, len(copied))) || timers[2].delay != time.Minute {
				t.Fatal("copy did not keep the value hidden or used the wrong lifetime")
			}
			timers[2].fire()
			if tt.Clipboard() != "" {
				t.Fatal("Hearth's copy remained after expiry")
			}

			click("Copy DB_PASSWORD")
			reads[3].done([]byte{0xff, 0}, nil)
			tt.Frame()
			if tt.Clipboard() != "/wA=" {
				t.Fatal("binary copy did not use Base64")
			}
			tt.SetClipboard("new clipboard text")
			timers[3].fire()
			if tt.Clipboard() != "new clipboard text" {
				t.Fatal("clearing a Secret erased a newer clipboard entry")
			}

			click("Show DB_PASSWORD")
			rotated := fixture
			rotated.Secrets = append([]kube.Secret(nil), fixture.Secrets...)
			rotated.Secrets[0].Keys = rotated.Secrets[0].Keys[:2]
			watches.got[1](rotated, nil)
			tt.Frame()
			if !reads[4].canceled {
				t.Fatal("removing a key did not cancel its pending request")
			}
			watches.got[1](fixture, nil)
			tt.Frame()
			click("Show DB_PASSWORD")
			click("Pods")
			if !reads[5].canceled {
				t.Fatal("leaving Secrets did not cancel the request")
			}
			late := []byte("late fixture")
			reads[5].done(late, nil)
			tt.Frame()
			if a.cluster.secret.revealed != nil || !bytes.Equal(late, make([]byte, len(late))) {
				t.Fatal("a late value survived after leaving Secrets")
			}
		})
	}
}
