// Copyright 2021 Clayton Craft <clayton@craftyguy.net>
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"testing"
)

func TestStripExts(t *testing.T) {
	tables := []struct {
		in       string
		expected string
	}{
		{"/foo/bar/bazz.tar", "/foo/bar/bazz"},
		{"file.tar.gz.xz.zip", "file"},
		{"another_file", "another_file"},
		{"a.b.c.d.e.f.g.h.i", "a"},
		{"virtio_blk.ko", "virtio_blk"},
	}
	for _, table := range tables {
		out := stripExts(table.in)
		if out != table.expected {
			t.Errorf("Expected: %q, got: %q", table.expected, out)
		}
	}
}
