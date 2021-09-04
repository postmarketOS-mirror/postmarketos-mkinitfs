// Copyright 2021 Clayton Craft <clayton@craftyguy.net>
// SPDX-License-Identifier: GPL-3.0-or-later

package deviceinfo

import (
	"errors"
	toml "github.com/pelletier/go-toml/v2"
	"os"
)

// Note: fields must be exported (start with capital letter)
// https://github.com/BurntSushi/toml/issues/121
type DeviceInfo struct {
	AppendDtb                     string `toml:"deviceinfo_append_dtb"`
	Arch                          string `toml:"deviceinfo_arch"`
	BootimgAppendSEAndroidEnforce string `toml:"deviceinfo_bootimg_append_seandroidenforce"`
	BootimgBlobpack               string `toml:"deviceinfo_bootimg_blobpack"`
	BootimgDtbSecond              string `toml:"deviceinfo_bootimg_dtb_second"`
	BootimgMtkMkimage             string `toml:"deviceinfo_bootimg_mtk_mkimage"`
	BootimgPxa                    string `toml:"deviceinfo_bootimg_pxa"`
	BootimgQcdt                   string `toml:"deviceinfo_bootimg_qcdt"`
	Dtb                           string `toml:"deviceinfo_dtb"`
	FlashKernelOnUpdate           string `toml:"deviceinfo_flash_kernel_on_update"`
	FlashOffsetBase               string `toml:"deviceinfo_flash_offset_base"`
	FlashOffsetKernel             string `toml:"deviceinfo_flash_offset_kernel"`
	FlashOffsetRamdisk            string `toml:"deviceinfo_flash_offset_ramdisk"`
	FlashOffsetSecond             string `toml:"deviceinfo_flash_offset_second"`
	FlashOffsetTags               string `toml:"deviceinfo_flash_offset_tags"`
	FlashPagesize                 string `toml:"deviceinfo_flash_pagesize"`
	GenerateBootimg               string `toml:"deviceinfo_generate_bootimg"`
	GenerateLegacyUbootInitfs     string `toml:"deviceinfo_generate_legacy_uboot_initfs"`
	InitfsCompression             string `toml:"deviceinfo_initfs_compression"`
	KernelCmdline                 string `toml:"deviceinfo_kernel_cmdline"`
	LegacyUbootLoadAddress        string `toml:"deviceinfo_legacy_uboot_load_address"`
	MesaDriver                    string `toml:"deviceinfo_mesa_driver"`
	MkinitfsPostprocess           string `toml:"deviceinfo_mkinitfs_postprocess"`
	ModulesInitfs                 string `toml:"deviceinfo_modules_initfs"`
}

func ReadDeviceinfo() (DeviceInfo, error) {
	file := "/etc/deviceinfo"
	var deviceinfo DeviceInfo

	_, err := os.Stat(file)
	if err != nil {
		return deviceinfo, errors.New("Unable to find deviceinfo: " + file)
	}

	fd, err := os.Open(file)
	if err != nil {
		return deviceinfo, err
	}
	defer fd.Close()
	// contents,_ := toml.LoadFile(file)
	decoder := toml.NewDecoder(fd)
	if err := decoder.Decode(&deviceinfo); err != nil {
		return deviceinfo, err
	}

	return deviceinfo, nil
}
