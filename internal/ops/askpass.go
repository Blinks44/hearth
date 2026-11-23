package ops

// AskpassProgram is Hearth's own program, which sudo runs as SUDO_ASKPASS
// in a step with Askpass; "" leaves such steps without it. Homebrew keeps
// only SUDO_ASKPASS of the environment, so the program knows it runs for
// sudo by that variable naming itself.
var AskpassProgram string
