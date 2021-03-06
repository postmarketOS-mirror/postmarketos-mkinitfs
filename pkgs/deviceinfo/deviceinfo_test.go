// Copyright 2021 Clayton Craft <clayton@craftyguy.net>
// SPDX-License-Identifier: GPL-3.0-or-later

package deviceinfo

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Test conversion of name to DeviceInfo struct field format
func TestNameToField(t *testing.T) {
	tables := []struct {
		in       string
		expected string
	}{
		{"deviceinfo_dtb", "Dtb"},
		{"dtb", "Dtb"},
		{"deviceinfo_modules_initfs", "ModulesInitfs"},
		{"modules_initfs", "ModulesInitfs"},
		{"deviceinfo_modules_initfs___", "ModulesInitfs"},
	}

	for _, table := range tables {
		out := nameToField(table.in)
		if out != table.expected {
			t.Errorf("expected: %q, got: %q", table.expected, out)
		}
	}
}

// Test unmarshalling with lines in deviceinfo
func TestUnmarshal(t *testing.T) {
	tables := []struct {
		// field is just used for reflection within the test, so it must be a
		// valid DeviceInfo field
		field    string
		in       string
		expected string
	}{
		{"ModulesInitfs", "deviceinfo_modules_initfs=\"panfrost foo bar bazz\"\n", "panfrost foo bar bazz"},
		{"ModulesInitfs", "deviceinfo_modules_initfs=\"panfrost foo bar bazz\"", "panfrost foo bar bazz"},
		// line with multiple '='
		{"KernelCmdline",
			"deviceinfo_kernel_cmdline=\"PMOS_NO_OUTPUT_REDIRECT fw_devlink=off nvme_core.default_ps_max_latency_us=5500 pcie_aspm.policy=performance\"\n",
			"PMOS_NO_OUTPUT_REDIRECT fw_devlink=off nvme_core.default_ps_max_latency_us=5500 pcie_aspm.policy=performance"},
		// empty option
		{"ModulesInitfs", "deviceinfo_modules_initfs=\"\"\n", ""},
		{"Dtb", "deviceinfo_dtb=\"freescale/imx8mq-librem5-r2 freescale/imx8mq-librem5-r3 freescale/imx8mq-librem5-r4\"\n",
			"freescale/imx8mq-librem5-r2 freescale/imx8mq-librem5-r3 freescale/imx8mq-librem5-r4"},
		// valid deviceinfo line, just not used in this module
		{"", "deviceinfo_codename=\"pine64-pinebookpro\"", ""},
		// line with comment at the end
		{"MesaDriver", "deviceinfo_mesa_driver=\"panfrost\"  # this is a nice driver", "panfrost"},
		{"", "# this is a comment!\n", ""},
		// empty lines are fine
		{"", "", ""},
		// line with whitepace characters only
		{"", " \t \n\r", ""},
	}
	var d DeviceInfo
	for _, table := range tables {
		testName := fmt.Sprintf("unmarshal::'%s':", strings.ReplaceAll(table.in, "\n", "\\n"))
		if err := unmarshal(strings.NewReader(table.in), &d); err != nil {
			t.Errorf("%s received an unexpected err: ", err)
		}

		// Check against expected value
		field := reflect.ValueOf(&d).Elem().FieldByName(table.field)
		out := ""
		if table.field != "" {
			out = field.String()
		}
		if out != table.expected {
			t.Errorf("%s expected: %q, got: %q", testName, table.expected, out)
		}
	}

}
