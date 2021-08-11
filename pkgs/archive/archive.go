// Copyright 2021 Clayton Craft <clayton@craftyguy.net>
// SPDX-License-Identifier: GPL-3.0-or-later
package archive

import (
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/cavaliercoder/go-cpio"
	"github.com/klauspost/pgzip"
	"gitlab.com/postmarketOS/postmarketos-mkinitfs/pkgs/misc"
	"io"
	"io/ioutil"
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
	// Archive verification is done in these steps:
	// 1. write archive to a temp location
	// 2. checksum the temp archive
	// 3. compare size of archive with amount of free space in target dir
	// 4. extract the archive to make sure it's valid / can be extracted
	// 5. copy archive to destination dir
	// 6. checksum target, compare to temp file checksum
	// 7. rename archive at destination to final target name

	targetDir := filepath.Dir(path)

	if err := archive.writeCpio(); err != nil {
		return err
	}

	if err := archive.cpioWriter.Close(); err != nil {
		return err
	}

	// 1. write archive to a temp location
	tmpOutDir, err := ioutil.TempDir("", filepath.Base(path))
	if err != nil {
		log.Print("Unable to create temporary work dir")
		return err
	}

	tmpOutFile := filepath.Join(tmpOutDir, filepath.Base(path))

	if err := archive.writeCompressed(tmpOutFile, mode); err != nil {
		return err
	}
	defer os.Remove(tmpOutFile)

	// 2. checksum the temp archive
	tmpFileChecksum, err := checksum(tmpOutFile)
	if err != nil {
		return err
	}

	// 3. compare size of archive with amount of free space in target dir
	tmpOutFileSize, err := os.Stat(tmpOutFile)
	if err != nil {
		log.Print("Unable to stat tmp output file: ", tmpOutFile)
	}
	actualFreeSpace, err := misc.FreeSpace(targetDir)
	// leave 10% free at target, because we're not monsters
	freeSpace := int64(float64(actualFreeSpace) * 0.9)
	if err != nil {
		log.Print("Unable to verify free space of target directory: ", targetDir)
		return err
	}
	if tmpOutFileSize.Size() >= freeSpace {
		return errors.New(fmt.Sprintf("Not enough free space in target dir (%q) for file. Need: %d bytes, free space: %d bytes",
			targetDir, tmpOutFileSize.Size(), freeSpace))
	}

	// 4. extract the archive to make sure it's valid / can be extracted
	extractDir, err := ioutil.TempDir(tmpOutDir, "extract-test")
	if err != nil {
		return err
	}
	defer os.RemoveAll(extractDir)
	if err := extract(tmpOutFile, extractDir); err != nil {
		log.Print("Extraction of archive failed!")
		return err
	}

	// 5. copy archive to destination dir
	tmpTargetFileFd, err := ioutil.TempFile(targetDir, filepath.Base(path))
	if err != nil {
		log.Print("Unable to create temp file in target dir: ", targetDir)
		return err
	}
	tmpOutFileFd, err := os.Open(tmpOutFile)
	if err != nil {
		log.Print("Unable to open temp file", tmpOutFile)
		return err
	}
	defer tmpOutFileFd.Close()
	if _, err := io.Copy(tmpTargetFileFd, tmpOutFileFd); err != nil {
		return err
	}
	// fsync
	if err := tmpTargetFileFd.Sync(); err != nil {
		log.Print("Unable to call fsync on temp file: ", targetDir)
		return err
	}
	if err := tmpTargetFileFd.Close(); err != nil {
		log.Print("Unable to save temp file to target dir: ", targetDir)
		return err
	}

	// 6. checksum target, compare to temp file checksum
	targetFileChecksum, err := checksum(tmpTargetFileFd.Name())

	if tmpFileChecksum != targetFileChecksum {
		return errors.New(fmt.Sprintf("Unable to save archive to path %q, checksum mismatch (expected: %q, got: %q)",
			path, tmpFileChecksum, targetFileChecksum))
	}

	// 7. rename archive at destination to final target name
	if err := os.Rename(tmpTargetFileFd.Name(), path); err != nil {
		log.Print("Unable to save archive to path: ", path)
		return err
	}
	if err := os.Chmod(path, mode); err != nil {
		return err
	}

	return nil
}

func checksum(path string) (string, error) {
	var sum string

	buf := make([]byte, 64*1024)
	sha256 := sha256.New()
	fd, err := os.Open(path)
	defer fd.Close()

	if err != nil {
		log.Print("Unable to checksum: ", path)
		return sum, err
	}

	// Read file in chunks
	for {
		bytes, err := fd.Read(buf)
		if bytes > 0 {
			_, err := sha256.Write(buf[:bytes])
			if err != nil {
				log.Print("Unable to checksum: ", path)
				return sum, err
			}
		}

		if err == io.EOF {
			break
		}
	}
	sum = hex.EncodeToString(sha256.Sum(nil))
	return sum, nil
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

func extract(path string, dest string) error {
	tDir, err := ioutil.TempDir("", "archive-extract")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tDir)

	srcFd, err := os.Open(path)
	if err != nil {
		return err
	}
	defer srcFd.Close()

	// TODO: support more compression types
	gz, err := pgzip.NewReader(srcFd)

	cpioArchive := cpio.NewReader(gz)
	for {
		hdr, err := cpioArchive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		destPath := filepath.Join(dest, hdr.Name)
		if hdr.Mode.IsDir() {
			os.MkdirAll(destPath, 0755)
		} else {
			destFd, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer destFd.Close()
			if _, err := io.Copy(destFd, cpioArchive); err != nil {
				return err
			}
		}
	}

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
