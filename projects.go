package main

import (
	"log"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"github.com/Blinks44/hearth/internal/projects"
	"github.com/Blinks44/hearth/internal/store"
	hearthui "github.com/Blinks44/hearth/internal/ui"
	"github.com/Blinks44/hearth/internal/watch"
	"github.com/Blinks44/hearth/locales"
)

// projectList keeps the user's projects: their folders in the settings,
// and what was read in them shown by the app. Its methods run on the UI
// thread; reading and saving go in the background.
type projectList struct {
	win      *mygo.Window
	app      *hearthui.App
	dir      string // where the settings are kept; "" keeps them nowhere
	settings store.Settings
	reads    int            // the latest read, whose result alone is shown
	watch    *watch.Service // follows the project folders; nil follows none

	saving sync.Mutex   // one save at a time
	saves  atomic.Int64 // the latest save, which an older one does not undo
}

// read reads every project again and shows them, and follows their
// folders.
func (l *projectList) read() {
	if l.watch != nil {
		l.watch.Projects(l.dirs())
	}
	l.reads++
	gen := l.reads
	dirs := l.dirs()
	go func() {
		ps := make([]projects.Project, len(dirs))
		for i, d := range dirs {
			ps[i] = projects.Read(d)
		}
		l.win.Update(func() {
			if gen == l.reads {
				l.app.Projects = ps
			}
		})
	}()
}

// update shows p, read again after its files changed, in place of what
// was read of its folder before.
func (l *projectList) update(p projects.Project) {
	ps := slices.Clone(l.app.Projects)
	if i := slices.IndexFunc(ps, func(old projects.Project) bool { return old.Dir == p.Dir }); i >= 0 {
		ps[i] = p
		l.app.Projects = ps
	}
}

// choose asks for a folder and offers to add it with the projects nested
// in it.
func (l *projectList) choose() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: l.win, Directory: true, Title: locales.AddProjectTitle(), ButtonLabel: locales.AddFolder(),
		})
		if err != nil {
			log.Print(err)
		}
		if len(paths) == 0 {
			return
		}
		root, nested := projects.Find(paths[0])
		l.win.Update(func() { l.app.Offer(root, nested) })
	}()
}

// find looks for projects in the usual code folders and offers them;
// once it has, the first start no longer does.
func (l *projectList) find() {
	home := l.app.Home
	go func() {
		folders, found := projects.Suggest(home)
		l.win.Update(func() {
			if !l.settings.Suggested {
				l.settings.Suggested = true
				l.save()
			}
			if len(found) > 0 {
				l.app.OfferFound(folders, found, time.Now())
			}
		})
	}()
}

// add adds the folders not added yet.
func (l *projectList) add(dirs []string) {
	for _, d := range dirs {
		if !slices.Contains(l.dirs(), d) {
			l.settings.Projects = append(l.settings.Projects, store.Project{Path: d})
		}
	}
	l.save()
	l.read()
}

// remove takes the project in dir off the list; the folder stays.
func (l *projectList) remove(dir string) {
	l.settings.Projects = slices.DeleteFunc(l.settings.Projects, func(p store.Project) bool { return p.Path == dir })
	l.app.Projects = slices.DeleteFunc(slices.Clone(l.app.Projects), func(p projects.Project) bool { return p.Dir == dir })
	if l.watch != nil {
		l.watch.Projects(l.dirs())
	}
	l.save()
}

func (l *projectList) save() {
	if l.dir == "" {
		return
	}
	s := l.settings
	s.Projects = slices.Clone(s.Projects)
	gen := l.saves.Add(1)
	go func() {
		l.saving.Lock()
		defer l.saving.Unlock()
		if gen != l.saves.Load() {
			return
		}
		if err := store.SaveSettings(l.dir, s); err != nil {
			log.Print(err)
			l.win.Update(func() { l.app.SettingsError = locales.SettingsError(err.Error()) })
		} else {
			l.win.Update(func() { l.app.SettingsError = "" })
		}
	}()
}

func (l *projectList) dirs() []string {
	dirs := make([]string, len(l.settings.Projects))
	for i, p := range l.settings.Projects {
		dirs[i] = p.Path
	}
	return dirs
}
