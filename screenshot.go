package main

import (
	"github.com/pkg/errors"
)

func screenshotWithScreencap(filename string) (err error) {
	_, err = runShellOutput("screencap", "-p", filename)
	err = errors.Wrap(err, "screencap")
	return
}

func isMinicapSupported() bool {
	return false
}
