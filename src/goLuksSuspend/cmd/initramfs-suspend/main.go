package main

import (
	"os"

	g "goLuksSuspend"
)

func main() {
	g.Debug("invoke main function", "initramfs-suspend/main")
	g.ParseFlags()

	g.Debug("loading cryptdevices", "initramfs-suspend/main")
	r := os.NewFile(uintptr(3), "r")
	cryptdevs, err := loadCryptdevices(r)
	g.Assert(err)
	g.Assert(r.Close())

	if len(cryptdevs) == 0 {
		// This branch should be impossible.
		g.Warn("no cryptdevices found, doing normal suspend", "initramfs-suspend/main")
		g.Assert(g.Suspend())
		return
	}

	if cryptdevs[0].Keyfile.Defined() {
		g.Debug("starting udevd from initramfs", "initramfs-suspend/main")
		g.Assert(startUdevDaemon())

		defer func() {
			g.Debug("stopping udevd within initramfs", "initramfs-suspend/main")
			g.Assert(stopUdevDaemon())
		}()
	}

	g.Debug("suspending cryptdevices", "initramfs-suspend/main")
	g.Assert(suspendCryptdevices(cryptdevs))

	// Crypt keys have been purged, so be less paranoid
	g.IgnoreErrors = true

	// Shorten task freeze timeout
	oldtimeout, err := g.SetFreezeTimeout([]byte{'1', '0', '0', '0'})
	if err == nil {
		defer func() {
			_, e := g.SetFreezeTimeout(oldtimeout)
			g.Assert(e)
		}()
	} else {
		g.Assert(err)
	}

	g.Assert(g.Suspend())

	g.Debug("resuming root cryptdevice", "initramfs-suspend/main")
	for {
		var err error
		for i := 0; i < 3; i++ {
			err = resumeRootCryptdevice(&cryptdevs[0])
			if err == nil {
				return
			}
		}
		// The -poweroff flag indicates the user's desire to take the
		// system offline on failure to unlock.
		if g.PoweroffOnError {
			g.IgnoreErrors = false
			g.Assert(err)
		}
	}
}
