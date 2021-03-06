// Copyright 2021 Clayton Craft <clayton@craftyguy.net>
// SPDX-License-Identifier: GPL-3.0-or-later

package archive

import (
	"bytes"
	"compress/flate"
	"github.com/cavaliercoder/go-cpio"
	"github.com/klauspost/pgzip"
	"gitlab.com/postmarketOS/postmarketos-mkinitfs/pkgs/misc"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Archive struct {
	Dirs       misc.StringSet
	Files      misc.StringSet
	cpioWriter *cpio.Writer
	buf        *bytes.Buffer
}

func New() (*Archive, error) {
	buf := new(bytes.Buffer)
	archive := &Archive{
		cpioWriter: cpio.NewWriter(buf),
		Files:      make(misc.StringSet),
		Dirs:       make(misc.StringSet),
		buf:        buf,
	}

	return archive, nil
}

func (archive *Archive) Write(path string, mode os.FileMode) error {
	if err := archive.writeCpio(); err != nil {
		return err
	}

	if err := archive.cpioWriter.Close(); err != nil {
		return err
	}

	// Write archive to path
	if err := archive.writeCompressed(path, mode); err != nil {
		log.Print("Unable to write archive to location: ", path)
		return err
	}

	if err := os.Chmod(path, mode); err != nil {
		return err
	}

	return nil
}

func (archive *Archive) AddFile(file string, dest string) error {
	if err := archive.addDir(filepath.Dir(dest)); err != nil {
		return err
	}

	if archive.Files[file] {
		// Already written to cpio
		return nil
	}

	fileStat, err := os.Lstat(file)
	if err != nil {
		log.Print("AddFile: failed to stat file: ", file)
		return err
	}

	// Symlink: write symlink to archive then set 'file' to link target
	if fileStat.Mode()&os.ModeSymlink != 0 {
		// log.Printf("File %q is a symlink", file)
		target, err := os.Readlink(file)
		if err != nil {
			log.Print("AddFile: failed to get symlink target: ", file)
			return err
		}

		destFilename := strings.TrimPrefix(dest, "/")
		hdr := &cpio.Header{
			Name:     destFilename,
			Linkname: target,
			Mode:     0644 | cpio.ModeSymlink,
			Size:     int64(len(target)),
			// Checksum: 1,
		}
		if err := archive.cpioWriter.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err = archive.cpioWriter.Write([]byte(target)); err != nil {
			return err
		}

		archive.Files[file] = true
		if filepath.Dir(target) == "." {
			target = filepath.Join(filepath.Dir(file), target)
		}
		// make sure target is an absolute path
		if !filepath.IsAbs(target) {
			target, err = misc.RelativeSymlinkTargetToDir(target, filepath.Dir(file))
			if err != nil {
				return err
			}
		}
		// TODO: add verbose mode, print stuff like this:
		// log.Printf("symlink: %q, target: %q", file, target)
		// write symlink target
		err = archive.AddFile(target, target)
		return err
	}

	// log.Printf("writing file: %q", file)

	fd, err := os.Open(file)
	if err != nil {
		return err
	}
	defer fd.Close()

	destFilename := strings.TrimPrefix(dest, "/")
	hdr := &cpio.Header{
		Name: destFilename,
		Mode: cpio.FileMode(fileStat.Mode().Perm()),
		Size: fileStat.Size(),
		// Checksum: 1,
	}
	if err := archive.cpioWriter.WriteHeader(hdr); err != nil {
		return err
	}

	if _, err = io.Copy(archive.cpioWriter, fd); err != nil {
		return err
	}

	archive.Files[file] = true

	return nil
}

func (archive *Archive) writeCompressed(path string, mode os.FileMode) error {
	// TODO: support other compression formats, based on deviceinfo
	fd, err := os.Create(path)
	if err != nil {
		return err
	}

	gz, err := pgzip.NewWriterLevel(fd, flate.BestSpeed)
	if err != nil {
		return err
	}

	if _, err = io.Copy(gz, archive.buf); err != nil {
		return err
	}

	if err := gz.Close(); err != nil {
		return err
	}

	// call fsync just to be sure
	if err := fd.Sync(); err != nil {
		return err
	}

	if err := os.Chmod(path, mode); err != nil {
		return err
	}

	return nil
}

func (archive *Archive) writeCpio() error {
	// Write any dirs added explicitly
	for dir := range archive.Dirs {
		archive.addDir(dir)
	}

	// Write files and any missing parent dirs
	for file, imported := range archive.Files {
		if imported {
			continue
		}
		if err := archive.AddFile(file, file); err != nil {
			return err
		}
	}

	return nil
}

func (archive *Archive) addDir(dir string) error {
	if archive.Dirs[dir] {
		// Already imported
		return nil
	}
	if dir == "/" {
		dir = "."
	}

	subdirs := strings.Split(strings.TrimPrefix(dir, "/"), "/")
	for i, subdir := range subdirs {
		path := filepath.Join(strings.Join(subdirs[:i], "/"), subdir)
		if archive.Dirs[path] {
			// Subdir already imported
			continue
		}
		err := archive.cpioWriter.WriteHeader(&cpio.Header{
			Name: path,
			Mode: cpio.ModeDir | 0755,
		})
		if err != nil {
			return err
		}
		archive.Dirs[path] = true
		// log.Print("wrote dir: ", path)
	}

	return nil
}
