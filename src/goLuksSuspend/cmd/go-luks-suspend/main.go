package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	g "goLuksSuspend"

	"github.com/guns/golibs/sys"
)

var systemdServices = []string{
	// journald may attempt to write to root device
	"syslog.socket",
	"systemd-journald.socket",
	"systemd-journald-dev-log.socket",
	"systemd-journald-audit.socket",
	"systemd-journald.service",
	// udevd often attempts to read from the root device
	"systemd-udevd-control.socket",
	"systemd-udevd-kernel.socket",
	"systemd-udevd.service",
}

const initramfsDir = "/run/initramfs"

func main() {
	g.Debug("invoke main function", "go-luks-suspend/main")
	g.ParseFlags()

	g.Debug("disabling ISIG in TTY", "go-luks-suspend/main")
	restoreTTY, err := sys.AlterTTY(os.Stdin.Fd(), sys.TCSETS, func(tty *syscall.Termios) {
		tty.Lflag &^= syscall.ISIG
	})
	if restoreTTY != nil {
		defer func() {
			g.Debug("restoring TTY to original state", "go-luks-suspend/main")
			g.Assert(restoreTTY())
		}()
	}
	if err != nil {
		g.Warn(err.Error(), "go-luks-suspend/main")
	}

	g.Debug("checking suspend program in initramfs", "go-luks-suspend/main")
	g.Assert(checkInitramfsBinary(filepath.Join(initramfsDir, "suspend")))

	g.Debug("gathering cryptdevices", "go-luks-suspend/main")
	cryptdevs, cdmap, err := g.GetCryptdevices()
	g.Assert(err)
	if g.DebugMode {
		for i := range cryptdevs {
			g.Debug(fmt.Sprintf("Name:%#v IsRootDevice:%#v",
				cryptdevs[i].Name,
				cryptdevs[i].IsRootDevice,
			), "go-luks-suspend/main")
		}
	}

	if len(cryptdevs) == 0 {
		g.IgnoreErrors = true
	}

	g.Debug("running pre-suspend scripts", "go-luks-suspend/main")
	g.Assert(runSystemSuspendScripts("pre"))

	defer func() {
		g.Debug("running post-suspend scripts", "go-luks-suspend/main")
		g.Assert(runSystemSuspendScripts("post"))
	}()

	if len(cryptdevs) == 0 {
		g.Warn("no cryptdevices found, doing normal suspend", "go-luks-suspend/main")
		g.Assert(g.Suspend())
		return
	}

	g.Debug("gathering filesystems with write barriers", "go-luks-suspend/main")
	filesystems, err := getFilesystemsWithWriteBarriers()
	g.Assert(err)
	if g.DebugMode {
		for i := range filesystems {
			g.Debug(fmt.Sprintf("%#v", filesystems[i]), "go-luks-suspend/main")
		}
	}

	g.Debug("preparing initramfs chroot", "go-luks-suspend/main")
	g.Assert(bindInitramfs())

	defer func() {
		g.Debug("unmounting initramfs bind mounts", "go-luks-suspend/main")
		g.Assert(unbindInitramfs())
	}()

	g.Debug("stopping selected system services", "go-luks-suspend/main")
	services, err := stopSystemServices(systemdServices)
	g.Assert(err)

	servicesRestarted := false

	defer func() {
		if !servicesRestarted {
			g.Debug("starting previously stopped system services", "go-luks-suspend/main")
			g.Assert(startSystemServices(services))
		}
	}()

	g.Debug("flushing pending writes", "go-luks-suspend/main")
	syscall.Sync()

	g.Debug("disabling write barriers on filesystems to avoid IO hangs", "go-luks-suspend/main")
	g.Assert(disableWriteBarriers(filesystems))

	defer func() {
		g.Debug("re-enabling write barriers on filesystems", "go-luks-suspend/main")
		enableWriteBarriers(filesystems)
	}()

	g.Debug("calling suspend in initramfs chroot", "go-luks-suspend/main")
	g.Assert(suspendInInitramfsChroot(cryptdevs))

	// We need to start up udevd ASAP so we can detect new block devices
	g.Debug("starting previously stopped system services", "go-luks-suspend/main")
	g.Assert(startSystemServices(services))
	servicesRestarted = true

	defer func() {
		g.Debug("resuming non-root cryptdevices with keyfiles", "go-luks-suspend/main")
		resumeCryptdevicesWithKeyfiles(cryptdevs)
	}()

	// User has unlocked the root device, so let's be less paranoid
	g.IgnoreErrors = true

	// Safe to grab keyfile info after root device is unlocked
	g.Debug("gathering keyfiles from /etc/crypttab", "go-luks-suspend/main")
	g.Assert(g.AddKeyfilesFromCrypttab(cdmap))
	if g.DebugMode {
		for i := range cryptdevs {
			if cryptdevs[i].Keyfile.Defined() {
				g.Debug(fmt.Sprintf("%#v", cryptdevs[i].Keyfile), "go-luks-suspend/main")
			}
		}
	}
}
