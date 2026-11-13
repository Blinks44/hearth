package projects

import "io/fs"

// device is not known on Windows: a folder's info does not say its
// volume, and other volumes are rarely mounted under the home folder.
func device(fs.FileInfo) (uint64, bool) { return 0, false }
