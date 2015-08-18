/*
* @Author: BlahGeek
* @Date:   2015-06-23
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-06-24
 */

package tun

import "os"
import "syscall"
import "unsafe"

type LinuxTun struct {
	_BaseTun
}

const (
	_IFF_TUN   = 0x0001
	_IFF_NO_PI = 0x1000
	_IFF_UP    = 1
	_TUNSETIFF = 0x400454ca
)

type _IfReq struct {
	name [16]byte
	flag uint16
}

func (tun *LinuxTun) Create(name string) error {
	tun.name = name
	var err error
	tun.fd, err = syscall.Open("/dev/net/tun", os.O_RDWR, 0x1ff)
	if err != nil {
		return err
	}
	ifreq := _IfReq{flag: _IFF_TUN | _IFF_NO_PI}
	copy(ifreq.name[:], name)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(tun.fd),
		_TUNSETIFF, uintptr(unsafe.Pointer(&ifreq)))
	if errno != 0 {
		return errno
	}
	return tun.Up()
}

func (tun *LinuxTun) Up() error {
	flags, err := tun.GetFlags()
	if err != nil {
		return err
	}
	return tun.SetFlags(flags | _IFF_UP)
}

func (tun *LinuxTun) Down() error {
	flags, err := tun.GetFlags()
	if err != nil {
		return err
	}
	return tun.SetFlags(flags &^ _IFF_UP)
}

func (tun *LinuxTun) Destroy() error {
    if err := tun.Down(); err != nil {
        return err
    }
    return tun.Close()
}

func (tun *LinuxTun) Read(buf []byte) (int, error) {
	return syscall.Read(tun.fd, buf)
}

func (tun *LinuxTun) Write(buf []byte) (int, error) {
	return syscall.Write(tun.fd, buf)
}
