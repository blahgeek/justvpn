/*
* @Author: BlahGeek
* @Date:   2015-07-19
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-07-19
 */

package tun

import "net"
import "runtime"
import "strings"
import "testing"

func TestRouteTableCMD(t *testing.T) {
	cmds := [][]string{
		generateCMD(net.IPNet{
			net.IPv4(166, 111, 8, 28),
			net.IPv4Mask(255, 255, 255, 0),
		}, net.IPv4(166, 111, 8, 1), true),
		generateCMD(net.IPNet{
			net.IPv4(0, 0, 0, 0),
			net.IPv4Mask(0, 0, 0, 0),
		}, net.IPv4(166, 111, 8, 1), false),
		generateCMD(net.IPNet{
			net.IPv4(10, 0, 0, 0),
			net.IPv4Mask(255, 255, 255, 255),
		}, net.IPv4(166, 111, 8, 1), false),
	}
	assert_cmd := func(cmd []string, str string) {
		if strings.Join(cmd, " ") != str {
			t.Errorf("Error route CMD, expect %s\n", str)
		}
	}
	if runtime.GOOS == "darwin" {
		assert_cmd(cmds[0], "route delete -net 166.111.8.28 166.111.8.1 255.255.255.0")
		assert_cmd(cmds[1], "route add -net 0.0.0.0 166.111.8.1 0.0.0.0")
		assert_cmd(cmds[2], "route add -net 10.0.0.0 166.111.8.1 255.255.255.255")
	} else if runtime.GOOS == "linux" {
		assert_cmd(cmds[0], "ip route del 166.111.8.28/24")
		assert_cmd(cmds[1], "ip route add 0.0.0.0/0 via 166.111.8.1")
		assert_cmd(cmds[2], "ip route add 10.0.0.0/32 via 166.111.8.1")
	}
}
