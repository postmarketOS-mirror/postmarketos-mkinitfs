// Copyright 2021 Clayton Craft <clayton@craftyguy.net>
// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"bufio"
	"debug/elf"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.sr.ht/~sircmpwn/getopt"
	"gitlab.com/postmarketOS/postmarketos-mkinitfs/pkgs/archive"
	"gitlab.com/postmarketOS/postmarketos-mkinitfs/pkgs/deviceinfo"
	"gitlab.com/postmarketOS/postmarketos-mkinitfs/pkgs/misc"
)

func timeFunc(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Printf("%s completed in: %s", name, elapsed)
}

func main() {
	devinfo := deviceinfo.ReadDeviceinfo()

	var outDir string
	getopt.StringVar(&outDir, "d", "/boot", "Directory to output initfs(-extra), default: /boot")

	if err := getopt.Parse(); err != nil {
		log.Fatal(err)
	}

	defer timeFunc(time.Now(), "main")

	kernVer, err := getKernelVersion()
	if err != nil {
		log.Fatal(err)
	}

	if err != nil {
		log.Fatal(err)
	}

	log.Print("Generating for kernel version: ", kernVer)
	log.Print("Output directory: ", outDir)

	if err := generateInitfs("initramfs", outDir, kernVer, devinfo); err != nil {
		log.Fatal(err)
	}

	if err := generateInitfsExtra("initramfs-extra", outDir, devinfo); err != nil {
		log.Fatal(err)
	}

}

func createInitfsRootDirs(initfsRoot string) {
	dirs := []string{
		"/bin", "/sbin", "/usr/bin", "/usr/lib", "/usr/sbin", "/proc", "/sys",
		"/dev", "/tmp", "/lib", "/boot", "/sysroot", "/etc",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(initfsRoot, dir), os.FileMode(0775)); err != nil {
			log.Fatal(err)
		}
	}
}

func exists(file string) bool {
	if _, err := os.Stat(file); err == nil {
		return true
	}
	return false
}

func getHookFiles(filesdir string) misc.StringSet {
	fileInfo, err := ioutil.ReadDir(filesdir)
	if err != nil {
		log.Fatal(err)
	}
	files := make(misc.StringSet)
	for _, file := range fileInfo {
		path := filepath.Join(filesdir, file.Name())
		f, err := os.Open(path)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		s := bufio.NewScanner(f)
		for s.Scan() {
			if !exists(s.Text()) {
				log.Fatalf("Unable to find file %q required by %q", s.Text(), path)
			}
			files[s.Text()] = false
		}
		if err := s.Err(); err != nil {
			log.Fatal(err)
		}
	}
	return files
}

// Recursively list all dependencies for a given ELF binary
func getBinaryDeps(files misc.StringSet, file string) error {
	// if file is a symlink, resolve dependencies for target
	fileStat, err := os.Lstat(file)
	if err != nil {
		log.Print("getBinaryDeps: failed to stat file")
		return err
	}

	// Symlink: write symlink to archive then set 'file' to link target
	if fileStat.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(file)
		if err != nil {
			log.Print("getBinaryDeps: unable to read symlink: ", file)
			return err
		}
		if !filepath.IsAbs(target) {
			target, err = misc.RelativeSymlinkTargetToDir(target, filepath.Dir(file))
			if err != nil {
				return err
			}
		}
		if err := getBinaryDeps(files, target); err != nil {
			return err
		}
		return err
	}

	// get dependencies for binaries
	fd, err := elf.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	libs, _ := fd.ImportedLibraries()
	fd.Close()
	files[file] = false

	if len(libs) == 0 {
		return err
	}

	libdirs := []string{"/usr/lib", "/lib"}
	for _, lib := range libs {
		found := false
		for _, libdir := range libdirs {
			path := filepath.Join(libdir, lib)
			if _, err := os.Stat(path); err == nil {
				err := getBinaryDeps(files, path)
				if err != nil {
					return err
				}
				files[path] = false
				found = true
				break
			}
		}
		if !found {
			log.Fatalf("Unable to locate dependency for %q: %s", file, lib)
		}
	}

	return nil
}

func getFiles(files misc.StringSet, newFiles misc.StringSet, required bool) error {
	for file := range newFiles {
		err := getFile(files, file, required)
		if err != nil {
			return err
		}
	}
	return nil
}

func getFile(files misc.StringSet, file string, required bool) error {
	if !exists(file) {
		if required {
			return errors.New("getFile: File does not exist :" + file)
		}
		return nil
	}

	files[file] = false

	// get dependencies for binaries
	if _, err := elf.Open(file); err != nil {
		// file is not an elf, so don't resolve lib dependencies
		return nil
	}

	err := getBinaryDeps(files, file)
	if err != nil {
		return err
	}

	return nil
}

func getOskConfFontPath(oskConfPath string) (string, error) {
	var path string
	f, err := os.Open(oskConfPath)
	if err != nil {
		return path, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		// "key = val" is 3 fields
		if len(fields) > 2 && fields[0] == "keyboard-font" {
			path = fields[2]
		}
	}
	if !exists(path) {
		return path, errors.New("Unable to find font: " + path)
	}

	return path, nil
}

// Get a list of files and their dependencies related to supporting rootfs full
// disk (d)encryption
func getFdeFiles(files misc.StringSet, devinfo deviceinfo.DeviceInfo) error {
	confFiles := misc.StringSet{
		"/etc/osk.conf":   false,
		"/etc/ts.conf":    false,
		"/etc/pointercal": false,
		"/etc/fb.modes":   false,
		"/etc/directfbrc": false,
	}
	// TODO: this shouldn't be false? though some files (pointercal) don't always exist...
	if err := getFiles(files, confFiles, false); err != nil {
		return err
	}

	// osk-sdl
	oskFiles := misc.StringSet{
		"/usr/bin/osk-sdl":    false,
		"/sbin/cryptsetup":    false,
		"/usr/lib/libGL.so.1": false}
	if err := getFiles(files, oskFiles, true); err != nil {
		return err
	}

	fontFile, err := getOskConfFontPath("/etc/osk.conf")
	if err != nil {
		return err
	}
	files[fontFile] = false

	// Directfb
	dfbFiles := make(misc.StringSet)
	err = filepath.Walk("/usr/lib/directfb-1.7-7", func(path string, f os.FileInfo, err error) error {
		if filepath.Ext(path) == ".so" {
			dfbFiles[path] = false
		}
		return nil
	})
	if err != nil {
		log.Print("getBinaryDeps: failed to stat file")
		return err
	}
	if err := getFiles(files, dfbFiles, true); err != nil {
		return err
	}

	// tslib
	tslibFiles := make(misc.StringSet)
	err = filepath.Walk("/usr/lib/ts", func(path string, f os.FileInfo, err error) error {
		if filepath.Ext(path) == ".so" {
			tslibFiles[path] = false
		}
		return nil
	})
	if err != nil {
		log.Print("getBinaryDeps: failed to stat file")
		return err
	}
	libts, _ := filepath.Glob("/usr/lib/libts*")
	for _, file := range libts {
		tslibFiles[file] = false
	}
	if err = getFiles(files, tslibFiles, true); err != nil {
		return err
	}

	// mesa hw accel
	if devinfo.Deviceinfo_mesa_driver != "" {
		mesaFiles := misc.StringSet{
			"/usr/lib/libEGL.so.1":    false,
			"/usr/lib/libGLESv2.so.2": false,
			"/usr/lib/libgbm.so.1":    false,
			"/usr/lib/libudev.so.1":   false,
			"/usr/lib/xorg/modules/dri/" + devinfo.Deviceinfo_mesa_driver + "_dri.so": false,
		}
		if err := getFiles(files, mesaFiles, true); err != nil {
			return err
		}
	}

	return nil
}

func getHookScripts(files misc.StringSet) {
	scripts, _ := filepath.Glob("/etc/postmarketos-mkinitfs/hooks/*.sh")
	for _, script := range scripts {
		files[script] = false
	}
}

func getInitfsExtraFiles(files misc.StringSet, devinfo deviceinfo.DeviceInfo) error {
	log.Println("== Generating initramfs extra ==")
	binariesExtra := misc.StringSet{
		"/lib/libz.so.1":        false,
		"/sbin/dmsetup":         false,
		"/sbin/e2fsck":          false,
		"/usr/sbin/parted":      false,
		"/usr/sbin/resize2fs":   false,
		"/usr/sbin/resize.f2fs": false,
	}
	log.Println("- Including extra binaries")
	if err := getFiles(files, binariesExtra, true); err != nil {
		return err
	}

	if exists("/usr/bin/osk-sdl") {
		log.Println("- Including FDE support")
		if err := getFdeFiles(files, devinfo); err != nil {
			return err
		}
	} else {
		log.Println("- *NOT* including FDE support")
	}

	return nil
}

func getInitfsFiles(files misc.StringSet, devinfo deviceinfo.DeviceInfo) error {
	log.Println("== Generating initramfs ==")
	requiredFiles := misc.StringSet{
		"/bin/busybox":        false,
		"/bin/sh":             false,
		"/bin/busybox-extras": false,
		"/usr/sbin/telnetd":   false,
		"/sbin/kpartx":        false,
		"/etc/deviceinfo":     false,
	}

	// Hook files & scripts
	if exists("/etc/postmarketos-mkinitfs/files") {
		log.Println("- Including hook files")
		hookFiles := getHookFiles("/etc/postmarketos-mkinitfs/files")
		if err := getFiles(files, hookFiles, true); err != nil {
			return err
		}
	}
	log.Println("- Including hook scripts")
	getHookScripts(files)

	log.Println("- Including required binaries")
	if err := getFiles(files, requiredFiles, true); err != nil {
		return err
	}

	return nil
}

func getInitfsModules(files misc.StringSet, devinfo deviceinfo.DeviceInfo, kernelVer string) error {
	log.Println("- Including kernel modules")

	modDir := filepath.Join("/lib/modules", kernelVer)
	if !exists(modDir) {
		return errors.New("Kernel module directory not found: " + modDir)
	}

	// modules.* required by modprobe
	modprobeFiles, _ := filepath.Glob(filepath.Join(modDir, "modules.*"))
	for _, file := range modprobeFiles {
		files[file] = false
	}

	// module name (without extension), or directory (trailing slash is important! globs OK)
	requiredModules := []string{
		"loop",
		"dm-crypt",
		"kernel/fs/overlayfs/",
		"kernel/crypto/",
		"kernel/arch/*/crypto/",
	}

	for _, item := range requiredModules {
		dir, file := filepath.Split(item)
		if file == "" {
			// item is a directory
			dir = filepath.Join(modDir, dir)
			dirs, _ := filepath.Glob(dir)
			for _, d := range dirs {
				if err := getModulesInDir(files, d); err != nil {
					log.Print("Unable to get modules in dir: ", d)
					return err
				}
			}
			continue
		} else if dir == "" {
			// item is a module name
			if err := getModule(files, file, modDir); err != nil {
				log.Print("Unable to get module: ", file)
				return err
			}
			continue
		} else {
			log.Printf("Unknown module entry: %q", item)
		}
	}

	// deviceinfo modules
	for _, module := range strings.Fields(devinfo.Deviceinfo_modules_initfs) {
		if err := getModule(files, module, modDir); err != nil {
			log.Print("Unable to get modules from deviceinfo")
			return err
		}
	}

	// /etc/postmarketos-mkinitfs/modules/*.modules
	initfsModFiles, _ := filepath.Glob("/etc/postmarketos-mkinitfs/modules/*.modules")
	for _, modFile := range initfsModFiles {
		f, err := os.Open(modFile)
		if err != nil {
			log.Print("getInitfsModules: unable to open mkinitfs modules file: ", modFile)
			return err
		}
		defer f.Close()
		s := bufio.NewScanner(f)
		for s.Scan() {
			if err := getModule(files, s.Text(), modDir); err != nil {
				log.Print("getInitfsModules: unable to get module file: ", s.Text())
				return err
			}
		}
	}

	return nil
}

func getKernelReleaseFile() (string, error) {
	files, _ := filepath.Glob("/usr/share/kernel/*/kernel.release")
	// only one kernel flavor supported
	if len(files) != 1 {
		return "", errors.New(fmt.Sprintf("Only one kernel release/flavor is supported, found: %q", files))
	}

	return files[0], nil
}

func getKernelVersion() (string, error) {
	var version string

	releaseFile, err := getKernelReleaseFile()
	if err != nil {
		return version, err
	}

	contents, err := os.ReadFile(releaseFile)
	if err != nil {
		return version, err
	}

	return strings.TrimSpace(string(contents)), nil
}


func generateInitfs(name string, path string, kernVer string, devinfo deviceinfo.DeviceInfo) error {
	initfsArchive, err := archive.New()
	if err != nil {
		return err
	}

	requiredDirs := []string{
		"/bin", "/sbin", "/usr/bin", "/usr/sbin", "/proc", "/sys",
		"/dev", "/tmp", "/lib", "/boot", "/sysroot", "/etc",
	}
	for _, dir := range requiredDirs {
		initfsArchive.Dirs[dir] = false
	}

	if err := getInitfsFiles(initfsArchive.Files, devinfo); err != nil {
		return err
	}

	if err := getInitfsModules(initfsArchive.Files, devinfo, kernVer); err != nil {
		return err
	}

	if err := initfsArchive.AddFile("/usr/share/postmarketos-mkinitfs/init.sh", "/init"); err != nil {
		return err
	}

	// splash images
	log.Println("- Including splash images")
	splashFiles, _ := filepath.Glob("/usr/share/postmarketos-splashes/*.ppm.gz")
	for _, file := range splashFiles {
		// splash images are expected at /<file>
		if err := initfsArchive.AddFile(file, filepath.Join("/", filepath.Base(file))); err != nil {
			return err
		}
	}

	// initfs_functions
	if err := initfsArchive.AddFile("/usr/share/postmarketos-mkinitfs/init_functions.sh", "/init_functions.sh"); err != nil {
		return err
	}

	log.Println("- Writing and verifying initramfs archive")
	if err := initfsArchive.Write(filepath.Join(path, name), os.FileMode(0644)); err != nil {
		return err
	}

	return nil
}

func generateInitfsExtra(name string, path string, devinfo deviceinfo.DeviceInfo) error {
	initfsExtraArchive, err := archive.New()
	if err != nil {
		return err
	}

	if err := getInitfsExtraFiles(initfsExtraArchive.Files, devinfo); err != nil {
		return err
	}

	log.Println("- Writing and verifying initramfs-extra archive")
	if err := initfsExtraArchive.Write(filepath.Join(path, name), os.FileMode(0644)); err != nil {
		return err
	}

	return nil
}

func stripExts(file string) string {
	for {
		if filepath.Ext(file) == "" {
			break
		}
		file = strings.Trim(file, filepath.Ext(file))
	}
	return file
}

func getModulesInDir(files misc.StringSet, modPath string) error {
	err := filepath.Walk(modPath, func(path string, f os.FileInfo, err error) error {
		// TODO: need to support more extensions?
		if filepath.Ext(path) != ".ko" && filepath.Ext(path) != ".xz" {
			return nil
		}
		files[path] = false
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}

// Given a module name, e.g. 'dwc_wdt', resolve the full path to the module
// file and all of its dependencies
func getModule(files misc.StringSet, modName string, modDir string) error {
	modDep := filepath.Join(modDir, "modules.dep")
	if !exists(modDep) {
		log.Fatal("Kernel module.dep not found: ", modDir)
	}

	fd, err := os.Open(modDep)
	if err != nil {
		log.Print("Unable to open modules.dep: ", modDep)
		return err
	}
	defer fd.Close()
	s := bufio.NewScanner(fd)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		fields[0] = strings.TrimSuffix(fields[0], ":")
		if modName != filepath.Base(stripExts(fields[0])) {
			continue
		}
		for _, modPath := range fields {
			p := filepath.Join(modDir, modPath)
			if !exists(p) {
				log.Print(fmt.Sprintf("Tried to include a module that doesn't exist in the modules directory (%s): %s", modDir, p))
				return err
			}
			files[p] = false
		}
	}
	if err := s.Err(); err != nil {
		log.Print("Unable to get module + dependencies: ", modName)
		return err
	}

	return err
}
