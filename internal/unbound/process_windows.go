package unbound

import (
	"fmt"
	"os"
	"os/exec"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows has no SIGHUP: offlokd restarts Unbound instead.
func reload(*os.Process) error { return ErrNoReload }

// asLocalService is cleared once Windows would not run Unbound as LocalService on this machine: from then on, until
// offlokd restarts, it runs as SYSTEM, as offlokd does. A resolver that does not start at all would break the
// internet; one that runs with more rights than it needs does not.
var asLocalService atomic.Bool

// account is the one the last Unbound ran as, so that the log tells of a change only.
var account atomic.Value

func init() { asLocalService.Store(true) }

// launch starts Unbound as LocalService, an account with no rights over the machine's files beyond what every
// service has (Linux drops it to unbound, macOS to _offlok): a flaw in Unbound, which reads what any server on the
// internet sends, then gives no one SYSTEM. Failing that, as SYSTEM.
func launch(binary, conf string, log func(string)) (*launched, error) {
	if asLocalService.Load() {
		l, err := launchAsLocalService(binary, conf, log)
		if err == nil {
			tell(log, "LocalService")
			return l, nil
		}
		asLocalService.Store(false)
		if log != nil {
			log(fmt.Sprintf("cannot run as LocalService (%v)", err))
		}
	}
	tell(log, "SYSTEM")
	return launchAsSystem(binary, conf)
}

func tell(log func(string), name string) {
	if log != nil && account.Swap(name) != name {
		log("runs as " + name)
	}
}

// launchAsSystem runs Unbound as offlokd does, SYSTEM, in a job object.
func launchAsSystem(binary, conf string) (*launched, error) {
	cmd := exec.Command(binary, "-d", "-c", conf)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stdout = cmd.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start unbound: %w", err)
	}
	job, err := newJob()
	if err == nil {
		err = assign(job, uint32(cmd.Process.Pid))
	}
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("tie unbound to offlokd: %w", err)
	}
	return &launched{proc: cmd.Process, stderr: stderr, wait: cmd.Wait, release: func() { windows.CloseHandle(job) }}, nil
}

var procLogonUserW = windows.NewLazySystemDLL("advapi32.dll").NewProc("LogonUserW")

const (
	logon32LogonService    = 5
	logon32ProviderDefault = 0
	// Unbound gone at once with one of these, LocalService is out of the question here: a DLL that cannot start
	// without the window station it may not have, or a file it may not read.
	statusDLLInitFailed = 0xC0000142
	statusAccessDenied  = 0xC0000022
)

// launchAsLocalService starts Unbound with LocalService's token, suspended until it is in offlokd's job. Its
// desktop is "", so that Windows connects it to LocalService's own window station, as it does for a service that
// runs as LocalService: NLnet Labs' unbound.exe loads USER32, which cannot start in SYSTEM's. Its standard output
// and error go to offlokd through a pipe, the only handle it inherits.
func launchAsLocalService(binary, conf string, log func(string)) (*launched, error) {
	user, _ := windows.UTF16PtrFromString("LocalService")
	domain, _ := windows.UTF16PtrFromString("NT AUTHORITY")
	var token windows.Token
	if r, _, err := procLogonUserW.Call(uintptr(unsafe.Pointer(user)), uintptr(unsafe.Pointer(domain)), 0,
		logon32LogonService, logon32ProviderDefault, uintptr(unsafe.Pointer(&token))); r == 0 {
		return nil, fmt.Errorf("log on as LocalService: %w", err)
	}
	defer token.Close()

	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer w.Close() // Unbound has its own copy
	out := windows.Handle(w.Fd())
	if err := windows.SetHandleInformation(out, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
		r.Close()
		return nil, err
	}
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		r.Close()
		return nil, err
	}
	defer attrs.Delete()
	inherit := []windows.Handle{out}
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&inherit[0]),
		uintptr(len(inherit))*unsafe.Sizeof(inherit[0])); err != nil {
		r.Close()
		return nil, err
	}
	app, _ := windows.UTF16PtrFromString(binary)
	line, _ := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{binary, "-d", "-c", conf}))
	desktop, _ := windows.UTF16PtrFromString("")
	si := windows.StartupInfoEx{
		StartupInfo:             windows.StartupInfo{Flags: windows.STARTF_USESTDHANDLES, StdOutput: out, StdErr: out, Desktop: desktop},
		ProcThreadAttributeList: attrs.List(),
	}
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi windows.ProcessInformation
	flags := uint32(windows.CREATE_SUSPENDED | windows.CREATE_NO_WINDOW | windows.EXTENDED_STARTUPINFO_PRESENT)
	if err := windows.CreateProcessAsUser(token, app, line, nil, nil, true, flags, nil, nil, &si.StartupInfo, &pi); err != nil {
		r.Close()
		return nil, fmt.Errorf("start unbound as LocalService: %w", err)
	}
	defer windows.CloseHandle(pi.Thread)
	defer windows.CloseHandle(pi.Process)
	fail := func(err error) (*launched, error) {
		_ = windows.TerminateProcess(pi.Process, 1)
		r.Close()
		return nil, err
	}
	job, err := newJob()
	if err != nil {
		return fail(fmt.Errorf("tie unbound to offlokd: %w", err))
	}
	if err := windows.AssignProcessToJobObject(job, pi.Process); err != nil {
		windows.CloseHandle(job)
		return fail(fmt.Errorf("tie unbound to offlokd: %w", err))
	}
	proc, err := os.FindProcess(int(pi.ProcessId))
	if err != nil {
		windows.CloseHandle(job)
		return fail(err)
	}
	if _, err := windows.ResumeThread(pi.Thread); err != nil {
		windows.CloseHandle(job)
		return fail(err)
	}
	started := time.Now()
	wait := func() error {
		st, err := proc.Wait()
		if err != nil {
			return err
		}
		if st.Success() {
			return nil
		}
		code := uint32(st.ExitCode())
		if (code == statusDLLInitFailed || code == statusAccessDenied) && time.Since(started) < 10*time.Second {
			asLocalService.Store(false)
			if log != nil {
				log(fmt.Sprintf("exited at once as LocalService (%#x): it runs as SYSTEM from now on", code))
			}
		}
		return fmt.Errorf("unbound exited with code %#x", code)
	}
	return &launched{proc: proc, stderr: r, wait: wait, release: func() { windows.CloseHandle(job) }}, nil
}

// newJob is a job object that Windows closes, killing Unbound, when offlokd's handle to it goes (offlokd dying
// included): a resolver left behind would keep answering after offlokd gave the system DNS back.
func newJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func assign(job windows.Handle, pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.AssignProcessToJobObject(job, h)
}
