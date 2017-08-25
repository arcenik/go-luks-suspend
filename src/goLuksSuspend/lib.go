package goLuksSuspend

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

var DebugMode = false

func Log(msg string) {
	if DebugMode {
		log.Println(msg)
	}
}

// Poweroff attempts to shutdown the system via /proc/sysrq-trigger
func Poweroff() {
	if DebugMode {
		log.Println("==========================================================")
		log.Println("  DEBUG SHELL: spawning /bin/sh instead of powering off!  ")
		log.Println("   `exit 42` if go-luks-suspend should resume execution   ")
		log.Println("==========================================================")
		err := Run([]string{"PS1=[\\w \\u\\$] "}, []string{"/bin/sh"}, true) // errcheck: debugmode only
		if err != nil {
			if exiterr, ok := err.(*exec.ExitError); ok {
				if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
					if status.ExitStatus() == 42 {
						return
					}
				}
			}
		}
		os.Exit(1)
	}

	for {
		_ = ioutil.WriteFile("/proc/sysrq-trigger", []byte{'o'}, 0600)
	}
}

// SuspendToRAM attempts to suspend the system via /sys/power/state
func SuspendToRAM() error {
	return ioutil.WriteFile("/sys/power/state", []byte{'m', 'e', 'm'}, 0600)
}

// Dump writes the names of a slice of CryptDevices as a NUL delimited
// sequence of bytes, starting with the name of the CryptDevice mounted on /.
// An error is returned if no CryptDevice is found that is mounted on /.
func Dump(path string, cryptdevices []CryptDevice) error {
	buf := make([][]byte, len(cryptdevices))
	j := 1
	for i := range cryptdevices {
		if cryptdevices[i].IsRootDevice {
			if len(buf[0]) > 0 {
				return errors.New("multiple root cryptdevices")
			}
			buf[0] = []byte(cryptdevices[i].Name)
		} else if j >= len(buf) {
			return errors.New("no root cryptdevice")
		} else {
			buf[j] = []byte(cryptdevices[i].Name)
			j++
		}
	}
	return ioutil.WriteFile(path, bytes.Join(buf, []byte{0}), 0600)
}

// Load loads the names written to a path by Dump
func Load(path string) ([]string, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return strings.Split(string(buf), "\x00"), nil
}

func Run(env []string, cmdline []string, stdio bool) error {
	cmd := exec.Command(cmdline[0], cmdline[1:]...)
	cmd.Env = env
	if stdio {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}
