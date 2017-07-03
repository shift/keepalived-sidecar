/*
Copyright 2017 The Kubernetes Authors.
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

package main

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"text/template"
	"time"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/pkg/util/exec"
	"k8s.io/kubernetes/pkg/util/wait"

	"github.com/golang/glog"
)

// keepalivedController is a controller which list&watch keepalived
// configuration information(VIP, neighbors IPs, etc) from a Service
// and update keepalived config file
type keepalivedController struct {
	clientset  *kubernetes.Clientset
	keepalived *keepalived

	namespace   string
	serviceName string
	podName     string

	template *template.Template
	config   map[string]interface{}
}

func newKeepalivedController(clientset *kubernetes.Clientset, namespace, serviceName, podName string) (*keepalivedController, error) {
	c := &keepalivedController{
		clientset:  clientset,
		keepalived: &keepalived{},

		namespace:   namespace,
		serviceName: serviceName,
		podName:     podName,

		config: make(map[string]interface{}),
	}

	tmpl, err := template.ParseFiles(keepalivedTmpl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse keepalived config template due to: %v", err)
	}
	c.template = tmpl

	return c, nil
}

// Run begins watching and syncing.
func (c *keepalivedController) Run(period time.Duration, stopCh <-chan struct{}) {
	go c.keepalived.Start()

	ch := make(chan struct{})
	wait.Until(func() {
		if c.keepalived.cmd != nil && c.keepalived.cmd.Process != nil {
			close(ch)
		}
	}, time.Second, ch)

	go wait.Until(func() {
		if err := c.Sync(); err != nil {
			glog.Error(err)
		}
	}, period, stopCh)

	<-stopCh
}

// Stop stops controller and remove configured VIP from interface
func (c *keepalivedController) Stop() error {
	vip := c.config["vip"]
	iface := c.config["iface"]

	defer c.keepalived.Stop()

	return c.removeVIP(iface.(string), vip.(string))
}

// Sync fetches keepalived configuration parameter and update config files
func (c *keepalivedController) Sync() error {
	conf, err := c.fetchConfig()
	if err != nil {
		return err
	}

	if reflect.DeepEqual(conf, c.config) {
		return nil
	}
	c.config = conf

	w, err := os.Create(keepalivedCfg)
	if err != nil {
		return fmt.Errorf("failed to create keepalbed config file")
	}
	defer w.Close()

	if err := c.template.Execute(w, conf); err != nil {
		return err
	}

	return c.keepalived.Reload()
}

// The k8s informer framework support List&Watch, instead of Get&Watch, so currently
// periodically fetch config parameter inetsad of watch
func (c *keepalivedController) fetchConfig() (conf map[string]interface{}, err error) {
	service, err := c.clientset.Core().Services(c.namespace).Get(c.serviceName, meta_v1.GetOptions{})
	if err != nil {
		return conf, fmt.Errorf("can not get service due to %v", err)
	}
	var vip, vrid, iface, track_iface string
	if service.Annotations != nil {
		vip = service.Annotations[KeepAlivedVIPAnnotationKey]
		vrid = service.Annotations[KeepAlivedVRIDAnnotationKey]
                iface = service.Annotations[KeepAlivedInterfaceKey]
                track_iface = service.Annotations[KeepAlivedTrackInterfaceKey]
	}
	if vip == "" {
		return conf, fmt.Errorf("no vip has been assigned to service")
	}
	if vrid == "" {
		return conf, fmt.Errorf("no vrid has been assigned to service")
	}
	if iface == "" {
		return conf, fmt.Errorf("no iface has been assigned to service")
	}
	if track_iface == "" {
		return conf, fmt.Errorf("no trackiface has been assigned to service")
	}
	endpoint, err := c.clientset.Core().Endpoints(c.namespace).Get(c.serviceName, meta_v1.GetOptions{})
	if err != nil {
		return conf, fmt.Errorf("can not get endpoint due to %v", err)
	}

	peers := []string{}
	for _, subset := range endpoint.Subsets {
		for _, addr := range subset.Addresses {
			peers = append(peers, addr.IP)
		}
		for _, addr := range subset.NotReadyAddresses {
			peers = append(peers, addr.IP)
		}
	}
	sort.Strings(peers)

	pod, err := c.clientset.Core().Pods(c.namespace).Get(c.podName, meta_v1.GetOptions{})
	if err != nil {
		return conf, fmt.Errorf("can not get pod due to %v", err)
	}
	selfIP := pod.Status.PodIP

	neighbors := getNeighbors(selfIP, peers)

	conf = make(map[string]interface{})
	conf["iface"] = iface
        conf["trackiface"] = track_iface
	conf["selfIP"] = selfIP
	conf["vip"] = vip
	conf["vrid"] = vrid
	conf["neighbors"] = neighbors
	conf["priority"] = getPriority(selfIP, peers)

	return conf, nil
}

func (c *keepalivedController) removeVIP(iface, vip string) error {
	if iface == "" || vip == "" {
		return nil
	}

	glog.Infof("removing configured VIP %v", vip)
	out, err := exec.New().Command("ip", "addr", "del", vip+"/32", "dev", iface).CombinedOutput()
	if err != nil {
		return fmt.Errorf("error removing VIP %s on interface %s", err, out)
	}
	return nil
}
