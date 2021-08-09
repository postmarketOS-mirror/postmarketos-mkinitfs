// Copyright 2021 Clayton Craft <clayton@craftyguy.net>
// SPDX-License-Identifier: GPL-3.0-or-later
package misc

import (
	"log"
	"os"
	"path/filepath"
)

type StringSet map[string]bool

// Converts a relative symlink target path (e.g. ../../lib/foo.so), that is
// absolute path
func RelativeSymlinkTargetToDir(symPath string, dir string) (string, error) {
	var path string

	oldWd, err := os.Getwd()
	if err != nil {
		log.Print("Unable to get current working dir")
		return path, err
	}

	if err := os.Chdir(dir); err != nil {
		log.Print("Unable to change to working dir: ", dir)
		return path, err
	}

	path, err = filepath.Abs(symPath)
	if err != nil {
		log.Print("Unable to resolve abs path to: ", symPath)
		return path, err
	}

	if err := os.Chdir(oldWd); err != nil {
		log.Print("Unable to change to old working dir")
		return path, err
	}

	return path, nil
}
