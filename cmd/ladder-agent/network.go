package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type networkPolicy struct {
	Fingerprint string   `json:"fingerprint"`
	Networks    []string `json:"networks"`
}

func currentNetworkPolicy() (networkPolicy, error) {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return networkPolicy{}, fmt.Errorf("cannot inspect host interfaces")
	}
	var networks []string
	for _, address := range addresses {
		prefix, err := netip.ParsePrefix(address.String())
		if err != nil || !prefix.IsValid() || prefix.Bits() == 0 {
			return networkPolicy{}, fmt.Errorf("invalid connected network")
		}
		networks = append(networks, prefix.Masked().String())
	}
	if len(networks) == 0 {
		return networkPolicy{}, fmt.Errorf("no host networks found")
	}
	sort.Strings(networks)
	unique := networks[:0]
	for _, network := range networks {
		if len(unique) == 0 || unique[len(unique)-1] != network {
			unique = append(unique, network)
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(unique, "\n")))
	return networkPolicy{Fingerprint: hex.EncodeToString(sum[:]), Networks: unique}, nil
}

func verifyNetworkPolicy() error {
	path, err := os.Executable()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(path), "host-network.json"))
	if err != nil {
		return err
	}
	var saved networkPolicy
	if err := json.Unmarshal(data, &saved); err != nil {
		return err
	}
	current, err := currentNetworkPolicy()
	if err != nil {
		return err
	}
	if current.Fingerprint != saved.Fingerprint {
		return fmt.Errorf("host network changed")
	}
	return nil
}
