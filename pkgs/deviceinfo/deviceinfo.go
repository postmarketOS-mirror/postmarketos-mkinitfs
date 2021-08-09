// Copyright 2021 Clayton Craft <clayton@craftyguy.net>
// SPDX-License-Identifier: GPL-3.0-or-later
package deviceinfo

import (
	"github.com/BurntSushi/toml"
	"log"
	"os"
)

// Note: fields must be exported (start with capital letter)
// https://github.com/BurntSushi/toml/issues/121
type DeviceInfo struct {
	Deviceinfo_append_dtb                      string
	Deviceinfo_arch                            string
	Deviceinfo_bootimg_append_seandroidenforce string
	Deviceinfo_bootimg_blobpack                string
	Deviceinfo_bootimg_dtb_second              string
	Deviceinfo_bootimg_mtk_mkimage             string
	Deviceinfo_bootimg_pxa                     string
	Deviceinfo_bootimg_qcdt                    string
	Deviceinfo_dtb                             string
	Deviceinfo_flash_offset_base               string
	Deviceinfo_flash_offset_kernel             string
	Deviceinfo_flash_offset_ramdisk            string
	Deviceinfo_flash_offset_second             string
	Deviceinfo_flash_offset_tags               string
	Deviceinfo_flash_pagesize                  string
	Deviceinfo_generate_bootimg                string
	Deviceinfo_generate_legacy_uboot_initfs    string
	Deviceinfo_mesa_driver                     string
	Deviceinfo_mkinitfs_postprocess            string
	Deviceinfo_initfs_compression              string
	Deviceinfo_kernel_cmdline                  string
	Deviceinfo_legacy_uboot_load_address       string
	Deviceinfo_modules_initfs                  string
	Deviceinfo_flash_kernel_on_update          string
}

func ReadDeviceinfo() DeviceInfo {
	file := "/etc/deviceinfo"
	_, err := os.Stat(file)
	if err != nil {
		log.Fatal("Unable to find deviceinfo: ", file)
	}

	var deviceinfo DeviceInfo
	if _, err := toml.DecodeFile(file, &deviceinfo); err != nil {
		log.Fatal(err)
	}
	return deviceinfo
}
