package goLuksSuspend

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

var DebugMode bool = true
var PoweroffOnError bool = false
var IgnoreErrors bool = false

const (
	S2RAM = 1 + iota
	S2DISK
	S2BOTH
	NOSUSPEND
)

var SuspendMode = S2RAM

func ParseFlags() {
	Debug("parseFlags", "lib/ParseFlags")

	DebugFlag := flag.Bool("debug", false, "print debug messages and spawn a shell on errors")
	NoDebugFlag := flag.Bool("nodebug", false, "disable debug messages and feature")
	PoweroffOnErrorFlag := flag.Bool("poweroff", false, "power off on errors and failure to unlock root device")

	VersionFlag := flag.Bool("version", false, "print version and exit")
	S2RFlag := flag.Bool("s2ram", false, "Suspend to RAM")
	S2DFlag := flag.Bool("s2disk", false, "Suspend to disk")
	// S2HFlag := flag.Bool("s2hybrid", false, "Suspend to RAM and disk")
	NoSuspendFlag := flag.Bool("nosuspend", false, "Do not suspend (for debug purpose)")

	flag.Parse()

	var DebugFlagCount int = 0
	var SuspendModeCount int = 0

	if *(DebugFlag) {
		DebugMode = true
		DebugFlagCount += 1
	}

	if *(NoDebugFlag) {
		DebugMode = false
		DebugFlagCount += 1
	}

	if DebugFlagCount > 1 {
		fmt.Println("You cannot set -debug and -nodebug at the same time")
		os.Exit(1)
	}

	if PoweroffOnErrorFlag != nil {
		PoweroffOnError = *(PoweroffOnErrorFlag)
	}

	if *(VersionFlag) {
		Debug("flag version present !", "lib/ParseFlags")
		fmt.Println(Version)
		os.Exit(0)
	}

	if *(S2RFlag) {
		Debug("Suspend to RAM", "lib/ParseFlags")
		SuspendMode = S2RAM
		SuspendModeCount += 1
	}

	if *(S2DFlag) {
		Debug("Suspend to disk", "lib/ParseFlags")
		SuspendMode = S2DISK
		SuspendModeCount += 1
	}

	// if *(S2HFlag) {
	// 	Debug("Suspend to RAM and disk", "lib/ParseFlags")
	// 	SuspendMode = S2BOTH
	// 	SuspendModeCount += 1
	// }

	if *(NoSuspendFlag) {
		Debug("Do not suspend", "lib/ParseFlags")
		SuspendMode = NOSUSPEND
		SuspendModeCount += 1
	}

	if SuspendModeCount > 2 {
		fmt.Println("You can use only once suspend mode at the same time")
		os.Exit(1)
	}

}

func Debug(msg string, ctx string) {
	if ctx == "" {
		ctx = "none"
	}
	if DebugMode {
		log.Printf("[go-luks-suspend][%s][debug] %s", ctx, msg)
	}
}

func Warn(msg string, ctx string) {
	if ctx == "" {
		ctx = "none"
	}
	log.Printf("[go-luks-suspend][%s][warning] %s", ctx, msg)
}

func Assert(err error) {
	if err == nil {
		return
	}

	Warn(err.Error(), "lib/assert")

	if IgnoreErrors {
		return
	}

	if DebugMode {
		DebugShell()
	} else if PoweroffOnError {
		Poweroff()
	} else {
		os.Exit(1)
	}
}

func DebugShell() {
	log.Println("===========================")
	log.Println("        DEBUG SHELL        ")
	log.Println("===========================")

	cmd := exec.Command("/bin/sh")
	cmd.Env = []string{"PS1=[\\w \\u\\$] "}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	_ = cmd.Run()

	fmt.Println("EXIT DEBUG SHELL")
}

func Run(cmd *exec.Cmd) error {
	if DebugMode {
		if len(cmd.Args) > 0 {
			Warn("exec: "+strings.Join(cmd.Args, " "), "lib/run")
		} else {
			Warn("exec: "+cmd.Path, "lib/run")
		}
	}
	return cmd.Run()
}

func Cryptsetup(args ...string) error {
	return Run(exec.Command("/usr/bin/cryptsetup", args...))
}

func Systemctl(args ...string) error {
	return Run(exec.Command("/usr/bin/systemctl", args...))
}

const freezeTimeoutPath = "/sys/power/pm_freeze_timeout"

func SetFreezeTimeout(timeout []byte) (oldtimeout []byte, err error) {
	oldtimeout, err = ioutil.ReadFile(freezeTimeoutPath)
	if err != nil {
		return nil, err
	}
	return oldtimeout, ioutil.WriteFile(freezeTimeoutPath, timeout, 0644)
}

func Suspend() error {
	Debug("Suspend invoked", "lib/Suspend")

	var err error
	var retries int = 5

	if SuspendMode == NOSUSPEND {
		Debug("No suspend mode", "lib/Suspend")
		return nil
	}

	for {

		switch SuspendMode {
		case S2RAM:
			Debug("Suspend to RAM", "lib/Suspend")
			err = ioutil.WriteFile("/sys/power/state", []byte{'m', 'e', 'm'}, 0600)
		case S2DISK:
			Debug("Suspend to DISK", "lib/Suspend")
			err = ioutil.WriteFile("/sys/power/state", []byte{'d', 'i', 's', 'k'}, 0600)
		// case S2BOTH:
		// 	Debug("Suspend to RAM and disk", "lib/Suspend")
		// 	err = ioutil.WriteFile("/sys/power/state", []byte{'m', 'e', 'm'}, 0600)
		default:
			Warn("Invalid SuspendMode !", "lib/Suspend")
			return nil
		}

		if err != nil {

			if retries > 0 {
				Warn("Suspend failed !", "lib/Suspend")
				// Warn(fmt.Errorf("%s", err.Error()), "lib/Suspend")
				time.Sleep(100 * time.Millisecond)
				retries -= 1
			} else { // no more retry
				return fmt.Errorf("%s\n\nSuspend failed. Unlock the root volume and investigate `dmesg`.", err.Error())
			}

		} else { // no error
			return nil
		}

	}

}

func Poweroff() {
	for {
		_ = ioutil.WriteFile("/proc/sysrq-trigger", []byte{'o'}, 0600) // errcheck: POWERING OFF!
	}
}
