package ui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/detect"
	"github.com/Blinks44/hearth/internal/kube"
)

// logStreams records the log page's reads: what each reads, how to
// answer it, and whether it was stopped.
type logStreams struct {
	logs    []kube.Logs
	got     []func([]kube.LogLine)
	done    []func(error)
	stopped []bool
}

func (s *logStreams) stream(l kube.Logs, got func([]kube.LogLine), done func(error)) func() {
	i := len(s.logs)
	s.logs = append(s.logs, l)
	s.got = append(s.got, got)
	s.done = append(s.done, done)
	s.stopped = append(s.stopped, false)
	return func() { s.stopped[i] = true }
}

func (s *logStreams) running() []kube.Logs {
	var ls []kube.Logs
	for i, l := range s.logs {
		if !s.stopped[i] {
			ls = append(ls, l)
		}
	}
	return ls
}

func crashLog() []kube.LogLine {
	at := time.Date(2026, 11, 19, 12, 41, 7, 0, time.Local)
	var lines []kube.LogLine
	for i, l := range []struct {
		text  string
		level kube.Level
	}{
		{"starting checkout-api v2.8.1", kube.LevelInfo},
		{"db: connecting to postgres.payments.svc:5432 (pool=10)", kube.LevelInfo},
		{"db: attempt 1/3 failed: connect ECONNREFUSED 10.96.12.4:5432", kube.LevelWarn},
		{"Error: connect ECONNREFUSED 10.96.12.4:5432", kube.LevelError},
		{"    at Client.connect (/app/node_modules/pg/lib/client.js:132:11)", kube.LevelError},
		{"process exited with code 1", kube.LevelInfo},
	} {
		lines = append(lines, kube.LogLine{At: at.Add(time.Duration(i) * time.Second), Text: l.text, Level: l.level})
	}
	return lines
}

// The log of a pod's container: what is going on tells its previous
// run's last line and opens its log; the current run and the previous
// one are read apart, found in, and no more once the log or the page
// closes.
func TestKubeLogs(t *testing.T) {
	a, tt, w := openShop(t)
	var s logStreams
	a.StreamLogs = s.stream
	if err := tt.Click("checkout-api-7d9f8c5b-x2k4p"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	running := w.running()
	tail := kube.Tail{Namespace: "default", Pod: "checkout-api-7d9f8c5b-x2k4p", Container: "api", Previous: true}
	if len(running) != 1 || running[0].Tail != tail {
		t.Fatalf("watches = %+v, want the last line of api read", running)
	}
	cl := shopCluster()
	cl.LastLine, cl.LastLineOf = "Error: connect ECONNREFUSED 10.96.12.4:5432", tail
	w.got[len(w.got)-1](cl, nil)
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "Last log line: Error: connect ECONNREFUSED 10.96.12.4:5432", "Previous run's log", "Logs")
	if len(s.logs) != 0 {
		t.Fatal("the details read a log")
	}

	if err := tt.Click("Previous run's log"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	want := kube.Logs{Kubectl: "/opt/homebrew/bin/kubectl", Context: "kind-dev", Namespace: "default", Pod: "checkout-api-7d9f8c5b-x2k4p", Container: "api", Previous: true}
	if got := s.running(); len(got) != 1 || got[0] != want {
		t.Fatalf("logs read = %+v, want %+v", got, want)
	}
	if running := w.running(); len(running) != 1 || running[0].Tail != (kube.Tail{}) {
		t.Fatalf("watches = %+v, want no last line read beside the log", running)
	}
	hasTexts(t, tt, "logs", "Reading the log…", "Previous run · code 1", "previous run")
	s.got[0](crashLog())
	s.done[0](nil)
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "6 lines", "WARN", "ERROR", "Error: connect ECONNREFUSED 10.96.12.4:5432", "12:41:07.000",
		"End of the previous run. The current run's log is under Current.")
	if tt.HasText("Whole run") {
		t.Error("a period for the previous run")
	}
	screenshot(t, tt, "kube-logs")

	// Finding marks the matches and counts them.
	a.cluster.logs.queryField.Focus()
	tt.Frame()
	tt.Type("ECONNREFUSED")
	tt.Advance(time.Second)
	hasTexts(t, tt, "1 of 2")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(time.Second)
	hasTexts(t, tt, "2 of 2")
	if err := tt.Click("Time"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if tt.HasText("12:41:07.000") {
		t.Error("times show with Time off")
	}

	// The current run is another read; the previous one stops.
	if err := tt.Click("Current"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	want.Previous = false
	if got := s.running(); len(got) != 1 || got[0] != want || !s.stopped[0] {
		t.Fatalf("logs read = %+v, want %+v alone", got, want)
	}
	hasTexts(t, tt, "Whole run", "Container api does not run now (CrashLoopBackOff): the log is of its last run.")
	s.done[1](&kube.Error{Access: detect.KubeAccess{Detail: `container "api" in pod "checkout-api-7d9f8c5b-x2k4p" is waiting to start`}})
	tt.Frame()
	tt.Advance(time.Second)
	hasTexts(t, tt, "kubectl did not give the log:")

	// The container starting again reads its new run.
	cl = shopCluster()
	cl.Pods[1].Containers[0].Restarts = 15
	w.got[len(w.got)-1](cl, nil)
	tt.Frame()
	tt.Advance(time.Second)
	if got := s.running(); len(got) != 1 || !s.stopped[1] || len(s.logs) != 3 {
		t.Fatalf("logs read = %+v, want the new run read", s.logs)
	}

	// Escape goes back to the pod's details, and the log is read no more.
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	if a.cluster.logs != nil || a.cluster.pod != "p1" || len(s.running()) != 0 {
		t.Fatalf("logs %v, pod %q, reads %+v", a.cluster.logs, a.cluster.pod, s.running())
	}
	hasTexts(t, tt, "Container api keeps failing after it starts")

	// Escape works with the focus on the button that opened the log, gone.
	if err := tt.Click("Logs"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	if a.cluster.logs != nil {
		t.Fatal("Escape did not close the log opened by a button")
	}

	// Leaving the page stops the log too.
	if err := tt.Click("Logs"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if len(s.running()) != 1 {
		t.Fatalf("logs read = %+v", s.running())
	}
	a.router.Push(kubePage)
	tt.Advance(time.Second)
	if len(s.running()) != 0 {
		t.Fatalf("logs read after leaving = %+v", s.running())
	}
}
