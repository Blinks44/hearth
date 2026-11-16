package ops

import (
	"errors"

	"golang.org/x/sys/windows"
)

func instructionLinkDenied(err error) bool {
	return errors.Is(err, windows.ERROR_PRIVILEGE_NOT_HELD) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
