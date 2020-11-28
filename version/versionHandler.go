package version

import (
	"crypto/tls"
	"io/ioutil"
	"net"
	"reflect"
	"sync"
	"time"

	yaml "gopkg.in/yaml.v2"

	"hypercloud-api-server/util"
	k8sApiCaller "hypercloud-api-server/util/Caller"

	"net/http"
	"strings"

	"k8s.io/klog"
)

// Get handles ~/version get method
func Get(res http.ResponseWriter, req *http.Request) {
	klog.Infoln("**** GET /version")
	var conf Config

	// 1. READ CONFIG FILE
	// File path should be same with what declared on volume mount in yaml file.
	yamlFile, err := ioutil.ReadFile("/config/module.config")
	if err != nil {
		klog.Errorln(err)
	}
	err = yaml.Unmarshal(yamlFile, &conf)
	if err != nil {
		klog.Errorln(err)
	}
	configSize := len(conf.Modules)
	result := make([]Module, configSize)

	// Main algorithm
	var wg sync.WaitGroup
	wg.Add(configSize)

	for idx, mod := range conf.Modules {
		go func(idx int, mod ModuleInfo) { // GoRoutine
			defer wg.Done()
			klog.Infoln("Module Name = ", mod.Name)
			result[idx].Name = mod.Name

			// 2. GET STATUS
			var labels string
			for i, label := range mod.Selector.MatchLabels.StatusLabel {
				if i == 0 {
					labels = label
				} else {
					labels += ", " + label
				}
			}
			podList, exist := k8sApiCaller.GetPodListByLabel(labels, mod.Namespace)

			ps := NewPodStatus()

			if exist {
				if mod.ReadinessProbe.Exec.Command != nil {
					// by exec command
					for j := range podList.Items {
						stdout, stderr, err := k8sApiCaller.ExecCommand(podList.Items[j], mod.ReadinessProbe.Exec.Command, mod.ReadinessProbe.Exec.Container)
						output := stderr + stdout

						if err != nil {
							klog.Errorln(mod.Name, " exec command error : ", err)
						} else {
							ps.Data[output]++
						}
					}
				} else if mod.ReadinessProbe.HTTPGet.Path != "" {
					// by HTTP
					for j := range podList.Items {
						var url string
						if mod.ReadinessProbe.HTTPGet.Scheme == "" || strings.EqualFold(mod.ReadinessProbe.HTTPGet.Scheme, "http") {
							url = "http://"
						} else if strings.EqualFold(mod.ReadinessProbe.HTTPGet.Scheme, "https") {
							url = "https://"
						}
						url += podList.Items[j].Status.PodIP + ":" + mod.ReadinessProbe.HTTPGet.Port + mod.ReadinessProbe.HTTPGet.Path
						http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // ignore certificate

						client := http.Client{
							Timeout: 15 * time.Second,
						}
						response, err := client.Get(url)
						if err != nil {
							klog.Errorln(mod.Name, " HTTP Error : ", err)
						} else if response.StatusCode >= 200 && response.StatusCode < 400 {
							ps.Data["Running"]++
						}
					}
				} else if mod.ReadinessProbe.TCPSocket.Port != "" {
					// by Port
					port := mod.ReadinessProbe.TCPSocket.Port
					timeout := time.Second
					for j := range podList.Items {
						host := podList.Items[j].Status.PodIP
						conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), timeout)
						defer conn.Close()
						if err != nil {
							klog.Errorln(mod.Name, " TCP Error : ", err)
						} else {
							ps.Data["Running"]++
						}
					}
				} else {
					// by Status.Phase
					for j := range podList.Items {
						if string(podList.Items[j].Status.Phase) == "Running" {
							ps.Data["Running"]++
						}
					}
				}
			}

			if !exist {
				klog.Errorln(mod.Name, " cannot found pods using given label : ", labels)
				result[idx].Status = "Not Installed"
			} else if ps.Data["Running"] == len(podList.Items) {
				// if every pod is 'Running', the module is normal
				result[idx].Status = "Normal"
			} else {
				result[idx].Status = "Abnormal"
			}

			// 3. GET VERSION
			if !(reflect.DeepEqual(mod.Selector.MatchLabels.StatusLabel, mod.Selector.MatchLabels.VersionLabel)) {
				labels = ""
				for i, label := range mod.Selector.MatchLabels.VersionLabel {
					if i == 0 {
						labels = label
					} else {
						labels += ", " + label
					}
				}
				podList, exist = k8sApiCaller.GetPodListByLabel(labels, mod.Namespace)
			}

			if !exist {
				klog.Errorln(mod.Name, " cannot found pods using given label")
				result[idx].Version = "Not Installed"
			} else if mod.VersionProbe.Exec.Command != nil {
				// by exec command
				stdout, stderr, err := k8sApiCaller.ExecCommand(podList.Items[0], mod.VersionProbe.Exec.Command, mod.VersionProbe.Container)
				output := stderr + stdout
				if err != nil {
					klog.Errorln(mod.Name, " exec command error : ", err)
				} else {
					result[idx].Version = ParsingVersion(output)
				}
			} else if podList.Items[0].Labels["version"] != "" {
				// by version label
				result[idx].Version = podList.Items[0].Labels["version"]
			} else {
				// by image tag
				if mod.VersionProbe.Container == "" {
					result[idx].Version = ParsingVersion(podList.Items[0].Spec.Containers[0].Image)
				} else {
					for j := range podList.Items[0].Spec.Containers {
						if podList.Items[0].Spec.Containers[j].Name == mod.VersionProbe.Container {
							result[idx].Version = ParsingVersion(podList.Items[0].Spec.Containers[j].Image)
							break
						}
					}
				}
			}
		}(idx, mod)
	}
	wg.Wait()

	// encode to JSON format and response
	util.SetResponse(res, "", result, http.StatusOK)
	return
}
