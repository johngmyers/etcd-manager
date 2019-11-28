/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package external

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sort"

	"github.com/golang/glog"

	"kope.io/etcd-manager/pkg/volumes"
)

// ExternalVolumes defines the aws volume implementation
type ExternalVolumes struct {
	basedir     string
	clusterName string
	localIP     net.IP
}

var _ volumes.Volumes = &ExternalVolumes{}

// NewExternalVolumes returns a new external volume provider
func NewExternalVolumes(clusterName string, basedir string) (*ExternalVolumes, error) {
	localIP, err := findLocalIP()
	if err != nil {
		return nil, err
	}

	a := &ExternalVolumes{
		basedir:     basedir,
		clusterName: clusterName,
		localIP:     localIP,
	}

	return a, nil
}

func findLocalIP() (net.IP, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("error getting network interfaces: %v", err)
	}

	var ips []net.IP

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("error getting addresses for interface: %v", err)
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			default:
				glog.Warningf("ignoring unknown address type %T", v)
			}

			if ip != nil {
				if ip.IsLoopback() {
					glog.V(2).Infof("ignoring loopback address: %v", err)
				} else if ip.IsLinkLocalUnicast() {
					glog.V(2).Infof("ignoring link-local unicast address: %v", err)
				} else {
					ips = append(ips, ip)
				}
			}
		}
	}

	if len(ips) > 1 {

		sort.Slice(ips, func(i, j int) bool {
			ipI := ips[i]
			ipJ := ips[j]

			if len(ipI) != len(ipJ) {
				return len(ipI) < len(ipJ)
			}

			for i := 0; i < len(ipI); i++ {
				if ipI[i] != ipJ[i] {
					return ipI[i] < ipJ[i]
				}
			}

			return false
		})

		glog.Infof("found multiple ips %v; choosing %q", ips, ips[0])
	}

	return ips[0], nil
}

func (a *ExternalVolumes) FindVolumes() ([]*volumes.Volume, error) {
	return a.findVolumes(true)
}

func (a *ExternalVolumes) findVolumes(filterByAZ bool) ([]*volumes.Volume, error) {
	files, err := ioutil.ReadDir(a.basedir)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %v", a.basedir, err)
	}

	var allVolumes []*volumes.Volume
	for _, f := range files {
		if !f.IsDir() {
			continue
		}

		p := filepath.Join(a.basedir, f.Name())

		mntPath := filepath.Join(p, "mnt")
		stat, err := os.Stat(mntPath)
		if err != nil {
			if os.IsNotExist(err) {
				glog.V(2).Infof("did not find dir %s, skipping", mntPath)
				continue
			}
			return nil, fmt.Errorf("error doing stat on %v: %v", mntPath, err)
		}

		if !stat.IsDir() {
			glog.V(2).Infof("expected directory at %s, but was not a directory; skipping")
			continue
		}

		volumeID := f.Name()

		etcdName := volumeID

		vol := &volumes.Volume{
			MountName:  "master-" + mntPath,
			ProviderID: mntPath,
			EtcdName:   etcdName,
			Info: volumes.VolumeInfo{
				Description: mntPath,
			},
			// Report as pre-mounted, so we won't try to attach or format
			Mountpoint: mntPath,
		}

		vol.AttachedTo = a.localIP.String()
		vol.LocalDevice = mntPath

		allVolumes = append(allVolumes, vol)
	}

	return allVolumes, nil
}

// FindMountedVolume implements Volumes::FindMountedVolume
func (a *ExternalVolumes) FindMountedVolume(volume *volumes.Volume) (string, error) {
	// Because we report the volumes as pre-mounted, this should never be called
	return "", fmt.Errorf("FindMountedVolume should not be called for ExternalVolumes")
}

// AttachVolume attaches the specified volume to this instance, returning nil if successful
func (a *ExternalVolumes) AttachVolume(volume *volumes.Volume) error {
	// no-op; the volumes are pre-mounted
	return nil
}

func (a *ExternalVolumes) MyIP() (string, error) {
	return a.localIP.String(), nil
}
