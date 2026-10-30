package unbound

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// killStale stops an Unbound that a previous offlokd left running on this configuration. macOS has no Pdeathsig:
// when offlokd dies (a crash, a kill), its Unbound lives on and keeps the port.
func killStale(binary, conf string) {
	out, err := exec.Command("/bin/ps", "-axo", "pid=,command=").Output()
	if err != nil {
		return
	}
	for _, pid := range stalePIDs(string(out), binary, conf) {
		if pid == os.Getpid() {
			continue
		}
		_ = syscall.Kill(pid, syscall.SIGTERM)
		for i := 0; i < 50 && syscall.Kill(pid, 0) == nil; i++ {
			time.Sleep(100 * time.Millisecond)
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// stalePIDs picks, from `ps -axo pid=,command=`, the processes running exactly what Start runs.
func stalePIDs(ps, binary, conf string) []int {
	want := binary + " -d -c " + conf
	var pids []int
	for _, line := range strings.Split(ps, "\n") {
		pidField, command, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok || strings.TrimSpace(command) != want {
			continue
		}
		if pid, err := strconv.Atoi(pidField); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}
