package ops

import "time"

// askTimeout is how long the password dialog waits for the user.
const askTimeout = 5 * time.Minute

// AskpassEnv names the variable that makes Hearth's program ask for the
// password, as SUDO_ASKPASS: its value says what for.
const AskpassEnv = "HEARTH_ASKPASS"

// AskpassProgram is Hearth's own program, which sudo runs as SUDO_ASKPASS
// in a step with Askpass; "" leaves such steps without it.
var AskpassProgram string
