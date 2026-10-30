package unbound

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows has no SIGHUP: offlokd restarts Unbound instead.
func reload(*os.Process) error { return ErrNoReload }

// afterStart puts Unbound in a job that Windows closes, killing Unbound, when offlokd's handle to it goes (offlokd
// dying included): a resolver left behind would keep answering after offlokd gave the system DNS back. release
// closes the job once Unbound has exited.
func afterStart(p *os.Process) (release func(), err error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(p.Pid))
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	return func() { windows.CloseHandle(job) }, nil
}
