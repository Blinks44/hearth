//go:build e2e

package main

// The end-to-end build lets the test in e2e/ drive the app through the
// socket MYGO_AUTOMATION names.
import _ "github.com/egoist/mygo/plugins/automation"
