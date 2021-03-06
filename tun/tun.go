/*
* @Author: BlahGeek
* @Date:   2015-06-23
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-18
 */

package tun

import "fmt"
import "net"
import "io"
import "syscall"
import "unsafe"
import "strconv"
import log "github.com/Sirupsen/logrus"
import "strings"
import "runtime"

const (
	ADDRESS = iota
	DST_ADDRESS
	NETMASK
)

type Tun interface {
	// Return interface name
	Name() string
	// Return file descriptor of this interface
	Fileno() int
	GetFlags() (uint16, error)
	SetFlags(uint16) error
	GetMTU() (int, error)
	SetMTU(int) error

	GetIPv4(typ int) (net.IP, error)
	SetIPv4(typ int, ip net.IP) error

	io.Reader
	io.Writer

	// Create interface
	Create(string) error
	// Close and destroy interface
	Destroy() error

	String() string
}

type _BaseTun struct {
	name string
	fd   int
}

func ioctl(cmd, ptr uintptr) error {
	sock, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	defer syscall.Close(sock)
	if err != nil {
		return fmt.Errorf("Ioctl: error opening socket: %v", err)
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(sock), cmd, ptr)
	if errno == 0 {
		return nil
	}
	return errno
}

func (tun *_BaseTun) String() string {
	return fmt.Sprintf("[%s:%d]", tun.name, tun.fd)
}

func (tun *_BaseTun) Name() string {
	return tun.name
}

func (tun *_BaseTun) Fileno() int {
	return tun.fd
}

// Struct for GET/SET interface flags
type _IfReqFlag struct {
	name [16]byte
	flag uint16
}

func (tun *_BaseTun) GetFlags() (uint16, error) {
	ifreq := _IfReqFlag{}
	copy(ifreq.name[:], tun.name)
	err := ioctl(syscall.SIOCGIFFLAGS, uintptr(unsafe.Pointer(&ifreq)))
	return ifreq.flag, err
}

func (tun *_BaseTun) SetFlags(flag uint16) error {
	ifreq := _IfReqFlag{flag: flag}
	copy(ifreq.name[:], tun.name)
	return ioctl(syscall.SIOCSIFFLAGS, uintptr(unsafe.Pointer(&ifreq)))
}

// Struct for GET/SET interface MTU
type _IfReqMTU struct {
	name [16]byte
	mtu  uint32
}

func (tun *_BaseTun) GetMTU() (int, error) {
	ifreq := _IfReqMTU{}
	copy(ifreq.name[:], tun.name)
	err := ioctl(syscall.SIOCGIFMTU, uintptr(unsafe.Pointer(&ifreq)))
	return int(ifreq.mtu), err
}

func (tun *_BaseTun) SetMTU(mtu int) error {
	ifreq := _IfReqMTU{mtu: uint32(mtu)}
	copy(ifreq.name[:], tun.name)
	return ioctl(syscall.SIOCSIFMTU, uintptr(unsafe.Pointer(&ifreq)))
}

// Struct for GET/SET ipv4 address/dst_address/netmask
type _IfReqIPv4 struct {
	name    [16]byte
	family  uint16
	_       [2]byte
	address [4]byte
	_       [8]byte
}

func (tun *_BaseTun) GetIPv4(typ int) (net.IP, error) {
	ifreq := _IfReqIPv4{family: syscall.AF_INET}
	copy(ifreq.name[:], tun.name)
	var cmd uintptr
	switch typ {
	case ADDRESS:
		cmd = syscall.SIOCGIFADDR
	case DST_ADDRESS:
		cmd = syscall.SIOCGIFDSTADDR
	case NETMASK:
		cmd = syscall.SIOCGIFNETMASK
	default:
		return net.IP{}, fmt.Errorf("Invalid type %v", typ)
	}
	err := ioctl(cmd, uintptr(unsafe.Pointer(&ifreq)))
	return net.IP(ifreq.address[:]), err
}

func (tun *_BaseTun) SetIPv4(typ int, ip net.IP) error {
	if typ == NETMASK {
		// Hack for OSX
		// must set IP Address again after setting netmask
		defer func() {
			ip, err := tun.GetIPv4(ADDRESS)
			if err == nil {
				tun.SetIPv4(ADDRESS, ip)
			}
		}()
	}
	ifreq := _IfReqIPv4{family: syscall.AF_INET}
	copy(ifreq.name[:], tun.name)
	if ipv4 := ip.To4(); ipv4 == nil {
		return fmt.Errorf("Invalid IPv4 Address %v", ip)
	} else {
		copy(ifreq.address[:], ipv4)
	}
	var cmd uintptr
	switch typ {
	case ADDRESS:
		cmd = syscall.SIOCSIFADDR
	case DST_ADDRESS:
		cmd = syscall.SIOCSIFDSTADDR
	case NETMASK:
		cmd = syscall.SIOCSIFNETMASK
	default:
		return fmt.Errorf("Invalid type %v", typ)
	}
	return ioctl(cmd, uintptr(unsafe.Pointer(&ifreq)))
}

func (tun *_BaseTun) Close() error {
	defer func() {
		tun.fd = -1
	}()
	return syscall.Close(tun.fd)
}

// New TUN device
func New() (Tun, error) {
	var tun Tun
	var name string
	ifaces, err := net.Interfaces()
	if err != nil {
		return tun, nil
	}

	for i := 0; ; i += 1 {
		name = "tun" + strconv.Itoa(i)
		iface_exist := false
		for _, iface := range ifaces {
			if strings.Contains(iface.Name, name) {
				iface_exist = true
			}
		}
		if !iface_exist {
			break
		}
	}

	log.WithField("name", name).Info("Allocating TUN interface")

	switch runtime.GOOS {
	case "darwin":
		tun = &UTun{}
	case "linux":
		tun = &LinuxTun{}
	default:
		return nil, fmt.Errorf("Tun not supported in %v", runtime.GOOS)
	}
	err = tun.Create(name)
	return tun, err
}
