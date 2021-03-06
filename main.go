// Copyright (c) 2014 Kelsey Hightower. All rights reserved.
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"

	"github.com/docker/libcontainer/netlink"
)

var (
	defaultEnvironmentFilePath = "/etc/network-environment"
	environmentFilePath        string
	defaultIfaceName           string
        filterCidrStr              string
)

func init() {
	log.SetFlags(0)
	flag.StringVar(&environmentFilePath, "o", defaultEnvironmentFilePath, "environment file")
	flag.StringVar(&defaultIfaceName, "i", "", "default interface")
        flag.StringVar(&filterCidrStr, "f", "", "a optional CIDR to filter addresses")
}

func main() {
        var filterCidrNet *net.IPNet

	flag.Parse()

        if filterCidrStr != "" {
                var err error
                _, filterCidrNet, err = net.ParseCIDR(filterCidrStr)
                if err != nil {
		        log.Fatal("Cannot parse CIDR", filterCidrStr)
                }
        }

	tempFilePath := environmentFilePath + ".tmp"
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		log.Fatal(err)
	}
	defer tempFile.Close()
	if err := writeEnvironment(tempFile, filterCidrNet); err != nil {
		log.Fatal(err)
	}
	os.Rename(tempFilePath, environmentFilePath)
}

func writeEnvironment(w io.Writer, filterCidrNet *net.IPNet) error {
	var buffer bytes.Buffer
	var err error
	if defaultIfaceName == "" {
		defaultIfaceName, err = getDefaultGatewayIfaceName()
		if err != nil {
			// A default route is not required; log it and keep going.
			log.Println(err)
		}
	}

	buffer.WriteString(fmt.Sprintf("DEFAULT_IFACE=%s\n", defaultIfaceName))

	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return err
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			// Record IPv4 network settings. Stop at the first IPv4 address
			// found for the interface.
			if err == nil && ip.To4() != nil {
                                if filterCidrNet != nil && filterCidrNet.Contains(ip) {
                                        continue
                                }

				buffer.WriteString(fmt.Sprintf("%s_IPV4=%s\n", strings.Replace(strings.ToUpper(iface.Name), ".", "_", -1), ip.String()))
				if defaultIfaceName == iface.Name {
					buffer.WriteString(fmt.Sprintf("DEFAULT_IPV4=%s\n", ip.String()))
				}
				break
			}
		}
		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			// Record IPv4 network settings. Stop at the first IPv4 address
			// found for the interface.
			if err == nil && ip.To4() == nil && ip.IsGlobalUnicast() {
				buffer.WriteString(fmt.Sprintf("%s_IPV6=%s\n", strings.Replace(strings.ToUpper(iface.Name), ".", "_", -1), ip.String()))
				if defaultIfaceName == iface.Name {
					buffer.WriteString(fmt.Sprintf("DEFAULT_IPV6=%s\n", ip.String()))
				}
				break
			}
		}
	}
	if _, err := buffer.WriteTo(w); err != nil {
		return err
	}
	return nil
}

func getDefaultGatewayIfaceName() (string, error) {
	routes, err := netlink.NetworkGetRoutes()
	if err != nil {
		return "", err
	}
	for _, route := range routes {
		if route.Default {
			if route.Iface == nil {
				return "", errors.New("found default route but could not determine interface")
			}
			return route.Iface.Name, nil
		}
	}
	return "", errors.New("unable to find default route")
}
