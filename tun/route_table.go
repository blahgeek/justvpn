/*
* @Author: BlahGeek
* @Date:   2015-07-18
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-08-14
 */

package tun

import "runtime"
import "fmt"
import log "github.com/Sirupsen/logrus"
import "os/exec"
import "net"
import "regexp"
import "strings"

const APPLY_ROUTER_CONCURRENT = 25

func generateCMD(dst net.IPNet, gw net.IP, is_delete bool) []string {
	var netmask_str string = net.IP(dst.Mask).String()
	var ipaddr_str string = dst.IP.String()
	var gateway_str string = gw.String()
	var network_str string = dst.String()

	switch runtime.GOOS {
	case "darwin":
		if is_delete {
			return []string{"route", "delete", "-net",
				ipaddr_str, gateway_str, netmask_str}
		} else {
			return []string{"route", "add", "-net",
				ipaddr_str, gateway_str, netmask_str}
		}
	case "linux":
		if is_delete {
			return []string{"ip", "route", "del", network_str}
		} else {
			return []string{"ip", "route", "add", network_str, "via", gateway_str}
		}
	default:
		return nil
	}

	return nil
}

func GetWireDefaultGateway() (net.IP, error) {
	var regex *regexp.Regexp
	var command *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("netstat", "-rn", "-f", "inet")
		regex = regexp.MustCompile(`default\s+(\d+\.\d+\.\d+\.\d+)\s+.*`)
	case "linux":
		command = exec.Command("ip", "-4", "route", "show")
		regex = regexp.MustCompile(`default\s+via\s+(\d+\.\d+\.\d+\.\d+)\s+.*`)
	default:
		return nil, fmt.Errorf("Getting gateway is not supported in %v", runtime.GOOS)
	}

	output, err := command.Output()
	if err != nil {
		return nil, err
	}
	if result := regex.FindSubmatch(output); result == nil {
		return nil, fmt.Errorf("Unable to get default gateway")
	} else {
		return net.ParseIP(string(result[1])), nil
	}
}

func ApplyInterfaceRouter(tun Tun) error {
	// For OSX: run `route add -host ... -interface tunX`
	if runtime.GOOS != "darwin" {
		return nil
	}
	vpn_dst_addr, _ := tun.GetIPv4(DST_ADDRESS)
	cmd := exec.Command("route", "add", "-host", vpn_dst_addr.String(),
		"-interface", tun.Name())
	log.WithField("cmd", fmt.Sprintf("%s %s", cmd.Path, strings.Join(cmd.Args, " "))).
		Debug("Applying interface router")
	return cmd.Run()
}

func ApplyRouter(wire_rules, vpn_rules []net.IPNet,
	wire_gw, vpn_gw net.IP, is_delete bool) error {

	rules := make([][]string, 0, len(wire_rules)+len(vpn_rules))
	for _, wire_rule := range wire_rules {
		rules = append(rules, generateCMD(wire_rule, wire_gw, is_delete))
	}
	for _, vpn_rule := range vpn_rules {
		rules = append(rules, generateCMD(vpn_rule, vpn_gw, is_delete))
	}

	total_err_count := 0

	for i := 0; i < len(rules); i += APPLY_ROUTER_CONCURRENT {
		slice_max := i + APPLY_ROUTER_CONCURRENT
		if slice_max > len(rules) {
			slice_max = len(rules)
		}
		waiter := make(chan error)
		running_count := 0
		for _, rule := range rules[i:slice_max] {
			if rule == nil {
				continue
			}
			running_count += 1
			go func() {
				log.WithField("cmd", strings.Join(rule, " ")).
					Debug("Applying router")
				waiter <- exec.Command(rule[0], rule[1:]...).Run()
			}()
		}

		for j := 0; j < running_count; j += 1 {
			err := <-waiter
			if err != nil {
				total_err_count += 1
			}
		}
	}

	log.WithFields(log.Fields{
		"all":   len(rules),
		"error": total_err_count,
	}).Info("Applying routers done")

	if total_err_count > 0 {
		return fmt.Errorf("Applying routers error")
	}
	return nil
}
