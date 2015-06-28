/*
* @Author: BlahGeek
* @Date:   2015-06-23
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-06-28
 */

package tun

import "strings"
import "testing"
import "os/exec"
import "net"
import "runtime"

func _check_ifconfig(t *testing.T, name string, substring string) {
	output, err := exec.Command("ifconfig", name).Output()
	if err != nil {
		t.Fatalf("Failed: ifconfig %v", name)
	}
	out_str := string(output)
	t.Log(out_str)
	if strings.Index(out_str, substring) == -1 {
		t.Errorf("`%v` not found in `ifconfig %v`", substring, name)
	}
}

func TestTunParams(t *testing.T) {
	tun, err := New()
	if err != nil {
		t.Fatalf("Error when creating tun: %v", err)
	}
	tun.SetIPv4(ADDRESS, net.ParseIP("10.42.0.1"))
	tun.SetIPv4(DST_ADDRESS, net.ParseIP("10.42.0.2"))
	tun.SetIPv4(NETMASK, net.ParseIP("255.255.255.0"))

	if runtime.GOOS == "darwin" {
		_check_ifconfig(t, tun.Name(), "inet 10.42.0.1")
		_check_ifconfig(t, tun.Name(), "-> 10.42.0.2")
		_check_ifconfig(t, tun.Name(), "netmask 0xffffff00")
	} else {
		_check_ifconfig(t, tun.Name(), "inet 10.42.0.1")
		_check_ifconfig(t, tun.Name(), "destination 10.42.0.2")
		_check_ifconfig(t, tun.Name(), "netmask 255.255.255.0")
	}

	if ip, e := tun.GetIPv4(ADDRESS); e != nil || !ip.Equal(net.ParseIP("10.42.0.1")) {
		t.Errorf("Address %v not equal", ip)
	}
	if ip, e := tun.GetIPv4(DST_ADDRESS); e != nil || !ip.Equal(net.ParseIP("10.42.0.2")) {
		t.Errorf("Address %v not equal", ip)
	}
	if ip, e := tun.GetIPv4(NETMASK); e != nil || !ip.Equal(net.ParseIP("255.255.255.0")) {
		t.Errorf("Address %v not equal", ip)
	}

	tun.SetMTU(1380)
	_check_ifconfig(t, tun.Name(), "mtu 1380")
	if mtu, e := tun.GetMTU(); e != nil || mtu != 1380 {
		t.Errorf("MTU error: %v", mtu)
	}
}
