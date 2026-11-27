package ai

import (
	"context"
	"os"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

func scanLiveAgentProcesses(_ context.Context, _ run.Runner, _ sysenv.Env) []LiveAgentProcess {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer func() { _ = windows.CloseHandle(snap) }()

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return nil
	}
	selfPID := uint32(os.Getpid())
	var out []LiveAgentProcess
	for {
		if entry.ProcessID != selfPID && entry.ProcessID > 4 {
			exe := windows.UTF16ToString(entry.ExeFile[:])
			if agent := matchAgentProcess(exe, nil); agent != "" {
				out = append(out, LiveAgentProcess{
					Agent: agent,
					PID:   strconv.FormatUint(uint64(entry.ProcessID), 10),
				})
			}
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}
	return out
}
