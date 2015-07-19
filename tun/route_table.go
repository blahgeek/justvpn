/*
* @Author: BlahGeek
* @Date:   2015-07-18
* @Last Modified by:   BlahGeek
* @Last Modified time: 2015-07-18
 */

package tun

import "runtime"
import "fmt"
import "log"
import "os/exec"
import "net"

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
				log.Printf("Applying router: %s\n", rule)
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

	log.Printf("Applying routers done, %d/%d failed\n",
		total_err_count, len(rules))
	if total_err_count > 0 {
		return fmt.Errorf("Applying routers error")
	}
	return nil
}
