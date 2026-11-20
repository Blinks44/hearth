package kube

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/run"
)

func TestParseLine(t *testing.T) {
	at := time.Date(2026, 11, 19, 12, 41, 7, 112000000, time.UTC)
	tests := []struct {
		line string
		prev Level
		want LogLine
	}{
		{"2026-11-19T12:41:07.112000000Z INFO starting checkout-api v2.8.1", LevelNone, LogLine{At: at, Text: "starting checkout-api v2.8.1", Level: LevelInfo}},
		{"2026-11-19T12:41:07.112Z [warn] db: attempt 1/3 failed", LevelNone, LogLine{At: at, Text: "[warn] db: attempt 1/3 failed", Level: LevelWarn}},
		// "Error:" is the message's own word: it stays.
		{"2026-11-19T12:41:07.112Z Error: connect ECONNREFUSED 10.96.12.4:5432", LevelNone, LogLine{At: at, Text: "Error: connect ECONNREFUSED 10.96.12.4:5432", Level: LevelError}},
		{"2026-11-19T12:41:07.112Z 2026/11/19 12:41:07 WARN slow query", LevelNone, LogLine{At: at, Text: "2026/11/19 12:41:07 WARN slow query", Level: LevelWarn}},
		{`2026-11-19T12:41:07.112Z {"level":"error","msg":"boom"}`, LevelNone, LogLine{At: at, Text: `{"level":"error","msg":"boom"}`, Level: LevelError}},
		{"2026-11-19T12:41:07.112Z time=12:41 level=debug msg=tick", LevelNone, LogLine{At: at, Text: "time=12:41 level=debug msg=tick", Level: LevelDebug}},
		{"2026-11-19T12:41:07.112Z E1119 12:41:07.112 1 main.go:12] failed", LevelNone, LogLine{At: at, Text: "E1119 12:41:07.112 1 main.go:12] failed", Level: LevelError}},
		// A stack trace's lines take the level of the line they continue.
		{"2026-11-19T12:41:07.112Z     at Client.connect (client.js:132:11)", LevelError, LogLine{At: at, Text: "    at Client.connect (client.js:132:11)", Level: LevelError}},
		{"2026-11-19T12:41:07.112Z shutting down: closing http server", LevelError, LogLine{At: at, Text: "shutting down: closing http server", Level: LevelNone}},
		// A level further in is the message's.
		{"2026-11-19T12:41:07.112Z db: attempt failed with error", LevelNone, LogLine{At: at, Text: "db: attempt failed with error", Level: LevelNone}},
		{"no time here", LevelNone, LogLine{Text: "no time here"}},
	}
	for _, tt := range tests {
		if got := ParseLine(tt.line, tt.prev); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseLine(%q) = %+v, want %+v", tt.line, got, tt.want)
		}
	}
}

func TestLogsArgv(t *testing.T) {
	l := Logs{Kubectl: "kubectl", Context: "kind-dev", Namespace: "shop", Pod: "web-1", Container: "api"}
	base := []string{"kubectl", "--context=kind-dev", "logs", "--namespace=shop", "web-1", "--container=api", "--timestamps", "--tail=10000"}
	if got, want := l.argv(), append(slices.Clone(base), "--follow"); !slices.Equal(got, want) {
		t.Errorf("current: %q, want %q", got, want)
	}
	l.Since = 15 * time.Minute
	if got, want := l.argv(), append(slices.Clone(base), "--follow", "--since=15m0s"); !slices.Equal(got, want) {
		t.Errorf("since: %q, want %q", got, want)
	}
	l.Since, l.Previous = 0, true
	if got, want := l.argv(), append(slices.Clone(base), "--previous"); !slices.Equal(got, want) {
		t.Errorf("previous: %q, want %q", got, want)
	}
	l.Previous, l.Terminated = false, true
	if got := l.argv(); !slices.Equal(got, base) {
		t.Errorf("terminated: %q, want %q", got, base)
	}
}

func TestStream(t *testing.T) {
	l := Logs{Kubectl: "kubectl", Context: "kind-dev", Namespace: "shop", Pod: "web-1", Container: "api", Previous: true}
	t.Run("lines", func(t *testing.T) {
		lines := func(_ context.Context, c run.Cmd, line func(string)) error {
			if !slices.Equal(c.Args, l.argv()) {
				t.Errorf("argv %q", c.Args)
			}
			for i := range 3 {
				line(fmt.Sprintf("2026-11-19T12:41:0%dZ ERROR line %d", i, i))
			}
			line("2026-11-19T12:41:03Z     at x")
			return nil
		}
		var got []LogLine
		if err := Stream(t.Context(), lines, nil, l, func(b []LogLine) { got = append(got, b...) }); err != nil {
			t.Fatal(err)
		}
		if len(got) != 4 || got[0].Text != "line 0" || got[3].Level != LevelError {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("kubectl refuses", func(t *testing.T) {
		lines := func(_ context.Context, c run.Cmd, _ func(string)) error {
			_, _ = fmt.Fprint(c.Output, `Error from server (BadRequest): previous terminated container "api" in pod "web-1" not found`+"\n")
			return errors.New("exit status 1")
		}
		err := Stream(t.Context(), lines, nil, l, func([]LogLine) {})
		kerr, ok := errors.AsType[*Error](err)
		if !ok || kerr.Access.State != detect.KubeUnknown || kerr.Access.Detail != `Error from server (BadRequest): previous terminated container "api" in pod "web-1" not found` {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("kubelet cannot follow", func(t *testing.T) {
		const message = "failed to create fsnotify watcher: too many open files"
		lines := func(_ context.Context, _ run.Cmd, line func(string)) error {
			line("2026-11-20T10:00:01Z " + message)
			line(message)
			return nil
		}
		var got []LogLine
		err := Stream(t.Context(), lines, nil, l, func(b []LogLine) { got = append(got, b...) })
		if err == nil || !strings.Contains(err.Error(), message) || len(got) != 1 || got[0].Text != message {
			t.Errorf("error = %v, lines = %+v", err, got)
		}
	})
}

func TestReadLastLine(t *testing.T) {
	var f run.Fake
	f.Stdout(`{"items": []}`, getArgv("--namespace=shop")...)
	f.Stdout("Error: connect ECONNREFUSED 10.96.12.4:5432\n", "kubectl", "--context=kind-dev", "logs", "--namespace=shop", "web-1",
		"--container=api", "--previous", "--tail=1", "--request-timeout=15s")
	tg := target
	tg.Tail = Tail{Namespace: "shop", Pod: "web-1", Container: "api", Previous: true}
	c, err := Read(context.Background(), &f, nil, tg)
	if err != nil {
		t.Fatal(err)
	}
	if c.LastLine != "Error: connect ECONNREFUSED 10.96.12.4:5432" || c.LastLineOf != tg.Tail {
		t.Errorf("last line %q of %+v", c.LastLine, c.LastLineOf)
	}
	// The run that ended is the current one: no --previous. No line: the
	// read does not fail.
	tg.Tail.Previous = false
	if c, err = Read(context.Background(), &f, nil, tg); err != nil || c.LastLine != "" {
		t.Errorf("no such run: %q, %v", c.LastLine, err)
	}
	f.Stdout("boom\n", "kubectl", "--context=kind-dev", "logs", "--namespace=shop", "web-1", "--container=api", "--tail=1", "--request-timeout=15s")
	if c, err = Read(context.Background(), &f, nil, tg); err != nil || c.LastLine != "boom" {
		t.Errorf("current run: %q, %v", c.LastLine, err)
	}
}

func TestReadOwnerLogPods(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ kind, parentKind, parentUID string }{
		{"Deployment", "ReplicaSet", "current-set"},
		{"StatefulSet", "StatefulSet", "current-owner"},
		{"Job", "Job", "current-owner"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			var r run.Fake
			target := Target{Kubectl: "kubectl", Context: "test", Namespaces: []string{"elsewhere"},
				LogOwner: LogOwner{Kind: tc.kind, Namespace: "shop", Name: "work", UID: "current-owner"}}
			get := func(resource string) []string {
				return []string{"kubectl", "--context=test", "get", resource, "--namespace=shop", "--output=json", "--request-timeout=15s"}
			}
			if tc.kind == "Deployment" {
				r.Stdout(`{"items":[
					{"metadata":{"uid":"current-set","ownerReferences":[{"kind":"Deployment","name":"work","uid":"current-owner","controller":true}]}},
					{"metadata":{"uid":"old-set","ownerReferences":[{"kind":"Deployment","name":"work","uid":"old-owner","controller":true}]}}
				]}`, get("replicasets.apps")...)
			}
			item := func(name, uid, kind string, controller bool) string {
				return fmt.Sprintf(`{"metadata":{"uid":%q,"namespace":"shop","name":%q,"ownerReferences":[
					{"kind":%q,"name":"work","uid":%q,"controller":%t}
				]},"spec":{"containers":[{"name":"api"}]}}`, name, name, kind, uid, controller)
			}
			r.Stdout(fmt.Sprintf(`{"items":[%s,%s,%s,%s]}`,
				item("work-b", tc.parentUID, tc.parentKind, true),
				item("old", "old-set", tc.parentKind, true),
				item("work-a", tc.parentUID, tc.parentKind, true),
				item("non-controller", tc.parentUID, tc.parentKind, false)), get("pods")...)
			got, err := Read(t.Context(), &r, nil, target)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, pod := range got.Pods {
				names = append(names, pod.Name)
			}
			if !slices.Equal(names, []string{"work-a", "work-b"}) {
				t.Fatalf("pods = %v", names)
			}
		})
	}
}
