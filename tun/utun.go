/*
* @Author: BlahGeek
* @Date:   2015-06-23
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-06-23
 */

package tun

import "fmt"
import "regexp"
import "strconv"
import "syscall"
import "unsafe"
import "net"

const (
	_CTLIOCGINFO      = 3227799043
	_AF_SYS_CONTROL   = 2
	_SYSPROTO_CONTROL = 2
	_PF_SYSTEM        = 32
)

type UTun struct {
	_BaseTun
}

type _ctl_info struct {
	ctl_id   uint32
	ctl_name [96]byte
}

type _sockaddr_ctl struct {
	sc_len      uint8
	sc_family   uint8
	sc_sysaddr  uint16
	sc_id       uint32
	sc_unit     uint32
	sc_reserved [5]uint32
}

func (tun *UTun) Create(name string) error {
	tun.name = name
	var id int

	re := regexp.MustCompile("utun(\\d+)")
	if matches := re.FindStringSubmatch(tun.name); matches == nil {
		return fmt.Errorf("Invalid UTun name: %v", name)
	} else {
		id, _ = strconv.Atoi(matches[1])
		id += 1
	}

	var err error
	tun.fd, err = syscall.Socket(_PF_SYSTEM, syscall.SOCK_DGRAM, _SYSPROTO_CONTROL)
	if tun.fd < 0 || err != nil {
		return fmt.Errorf("Error allocating UTun: %v", err)
	}

	ctl_info := _ctl_info{}
	copy(ctl_info.ctl_name[:], "com.apple.net.utun_control")
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(tun.fd), _CTLIOCGINFO,
		uintptr(unsafe.Pointer(&ctl_info)))
	if errno != 0 {
		return errno
	}

	sc := _sockaddr_ctl{sc_len: uint8(unsafe.Sizeof(_sockaddr_ctl{})),
		sc_family:  _PF_SYSTEM,
		sc_sysaddr: _AF_SYS_CONTROL,
		sc_id:      ctl_info.ctl_id,
		sc_unit:    uint32(id)}
	_, _, errno = syscall.Syscall(syscall.SYS_CONNECT, uintptr(tun.fd),
		uintptr(unsafe.Pointer(&sc)), uintptr(unsafe.Sizeof(sc)))
	if errno != 0 {
		return errno
	}

	// For OS X: setting netmask BEFORE ip address causes kernel panic!
	tun.SetIPv4(ADDRESS, net.IPv4(0, 0, 0, 0))
	tun.SetIPv4(DST_ADDRESS, net.IPv4(0, 0, 0, 0))
	tun.SetIPv4(NETMASK, net.IPv4(255, 255, 255, 255))

	return nil
}
