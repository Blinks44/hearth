package ui

// Shell opens files and folders with the system's apps. mygo.Shell is
// one; tests give a fake.
type Shell interface {
	ShowItemInFolder(path string)
	OpenPath(path string) error
}
