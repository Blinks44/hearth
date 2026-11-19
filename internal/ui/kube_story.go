package ui

import (
	"strings"
	"time"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

// podStory is what the pod's details say is going on: only what
// Kubernetes reported, and apart from it what Hearth infers.
type podStory struct {
	title string
	// facts are what Kubernetes reported, in words; guesses what Hearth
	// infers from them.
	facts, guesses []string
	// detail is Kubernetes' own words, shown as they are.
	detail string
	// known is false when Hearth does not know what the reason means.
	known bool
	// waiting tells a pod still waiting for something from one failing.
	waiting bool
	// crashed is the container that keeps failing, whose log tells more;
	// empty for none. ended is true when the run that failed is its
	// current one, which ended, rather than its previous one.
	crashed string
	ended   bool
}

// line is the story in one line, for the pod's row.
func (s podStory) line() string {
	if len(s.facts) == 0 {
		return s.title
	}
	return s.title + ". " + s.facts[0]
}

// storyOf tells what is going on with a pod in trouble, by its status and
// its events, newest first; ok is false for a pod without trouble.
func storyOf(p kube.Pod, events []kube.Event, now time.Time) (s podStory, ok bool) {
	if !podTrouble(p) {
		return podStory{}, false
	}
	if p.Phase == "Failed" && p.Message != "" {
		return podStory{title: locales.KubeWhyPodFailed(p.Reason), detail: p.Message}, true
	}
	for _, ct := range p.Containers {
		if ct.State != "waiting" {
			continue
		}
		switch ct.Reason {
		case "CrashLoopBackOff":
			return crashStory(p, ct, now), true
		case "ImagePullBackOff", "ErrImagePull":
			return pullStory(ct), true
		}
	}
	for _, ct := range p.Containers {
		// Between its tries, a container that keeps failing shows the
		// run that just ended.
		if ct.State == "terminated" && ct.Ended != nil && ct.Reason != "Completed" && ct.Restarts > 0 && p.Phase == "Running" {
			ct.Last = ct.Ended
			s := crashStory(p, ct, now)
			s.ended = true
			return s, true
		}
	}
	if p.Unscheduled != "" {
		s := podStory{title: locales.KubeWhyUnscheduled(), detail: p.Unscheduled, waiting: true}
		for _, cause := range kube.ScheduleCauses(p.Unscheduled) {
			s.facts = append(s.facts, []string{
				kube.NoResources:    locales.KubeWhyNoResources(),
				kube.NoVolume:       locales.KubeWhyNoVolume(),
				kube.NoMatchingNode: locales.KubeWhyNoNode(),
				kube.Tainted:        locales.KubeWhyTainted(),
			}[cause])
		}
		s.known = len(s.facts) > 0
		return s, true
	}
	warning := ""
	for _, e := range events {
		if e.Type == "Warning" {
			warning = e.Reason + ": " + e.Message
			break
		}
	}
	for _, ct := range p.Containers {
		reason := ct.Reason
		if reason == "" || reason == "Completed" {
			continue
		}
		if ct.State == "waiting" && (reason == "ContainerCreating" || reason == "PodInitializing") {
			continue
		}
		s := podStory{title: locales.KubeWhyOther(ct.Name, reason), detail: ct.Message}
		if reason == "OOMKilled" {
			s.facts, s.known = []string{locales.KubeWhyOOM()}, true
		}
		if s.detail == "" {
			s.detail = warning
		}
		return s, true
	}
	if p.Phase == "Pending" {
		return podStory{title: locales.KubeWhyCreating(), detail: warning, known: true, waiting: true}, true
	}
	for _, ct := range p.Containers {
		if ct.State == "running" && !ct.Ready {
			return podStory{title: locales.KubeWhyNotReady(ct.Name), facts: []string{locales.KubeWhyProbe()}, detail: warning, known: true}, true
		}
	}
	return podStory{title: locales.KubeWhyUnknown(p.Phase), detail: warning}, true
}

// crashStory is the story of container ct of p starting and failing
// again and again.
func crashStory(p kube.Pod, ct kube.Container, now time.Time) podStory {
	s := podStory{title: locales.KubeWhyCrash(ct.Name), known: true, crashed: ct.Name}
	if last := ct.Last; last != nil {
		ran := last.Finished.Sub(last.Started)
		s.facts = append(s.facts, locales.KubeWhyLastRun(last.ExitCode, last.Reason, span(ran)))
		switch {
		case last.Reason == "OOMKilled":
			s.facts = append(s.facts, locales.KubeWhyOOM())
		case last.ExitCode == 0:
			s.guesses = append(s.guesses, locales.KubeWhyExit0())
		case last.ExitCode == 126 || last.ExitCode == 127:
			s.guesses = append(s.guesses, locales.KubeWhyExitCommand(last.ExitCode))
		case last.ExitCode == 137:
			s.guesses = append(s.guesses, locales.KubeWhyExit137())
		}
		s.detail = last.Message
	}
	s.facts = append(s.facts, locales.KubeWhyRestarts(ct.Restarts, span(now.Sub(p.Created))))
	if d := kube.BackOff(ct.Message); d > 0 {
		s.facts = append(s.facts, locales.KubeWhyNextTry(span(d)))
	}
	return s
}

// pullStory is the story of the image of ct that does not pull.
func pullStory(ct kube.Container) podStory {
	s := podStory{title: locales.KubeWhyPull(ct.Image), detail: ct.Message, known: true}
	switch kube.PullCause(ct.Message) {
	case kube.PullUnreachable:
		s.facts = []string{locales.KubeWhyPullUnreachable(registry(ct.Image))}
	case kube.PullDenied:
		s.facts = []string{locales.KubeWhyPullDenied(registry(ct.Image))}
	case kube.PullNotFound:
		s.facts = []string{locales.KubeWhyPullNotFound(registry(ct.Image))}
	default:
		s.known = false
	}
	return s
}

// registry is the registry an image comes from: the host its name starts
// with, Docker Hub's without one.
func registry(image string) string {
	host, _, found := strings.Cut(image, "/")
	if found && (strings.ContainsAny(host, ".:") || host == "localhost") {
		return host
	}
	return "docker.io"
}

// span says a duration short, as a resource's age.
func span(d time.Duration) string {
	d = max(d, 0)
	switch {
	case d < time.Minute:
		return locales.KubeAgeSeconds(int(d / time.Second))
	case d < time.Hour:
		return locales.KubeAgeMinutes(int(d / time.Minute))
	case d < 24*time.Hour:
		return locales.KubeAgeHours(int(d / time.Hour))
	}
	return locales.KubeAgeDays(int(d / (24 * time.Hour)))
}
