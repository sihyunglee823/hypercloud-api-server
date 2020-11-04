package version

import (
	"regexp"
	"strconv"
	"strings"

	"k8s.io/klog"
)

// ModuleInfo is a strcut for storing configMap file.
// It must support all possible cases described in configMap.
type ModuleInfo struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
	Selector  struct {
		MatchLabels struct {
			StatusLabel  []string `yaml:"statusLabel"`
			VersionLabel []string `yaml:"versionLabel"`
		} `yaml:"matchLabels"`
	} `yaml:"selector"`
	ReadinessProbe struct {
		Exec struct {
			Command   []string `yaml:"command"`
			Container string   `yaml:"container"`
		} `yaml:"exec"`
		HTTPGet struct {
			Path        string `yaml:"path"`
			Port        string `yaml:"port"`
			Scheme      string `yaml:"scheme"`
			ServiceName string `yaml:"serviceName"`
		} `yaml:"httpGet"`
		TCPSocket struct {
			Port string `yaml:"port"`
		} `yaml:"tcpSocket"`
	} `yaml:"readinessProbe"`
	VersionProbe struct {
		Exec struct {
			Command   []string `yaml:"command"`
			Container string   `yaml:"container"`
		} `yaml:"exec"`
	} `yaml:"versionProbe"`
}

// Config struct is array of ModuleInfo.
type Config struct {
	Modules []ModuleInfo `yaml:"modules"`
}

// Module struct is for storing result and returning to client.
type Module struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Version string `json:"version"`
}

// PodStatus struct is for storing status of each pod.
type PodStatus struct {
	Data map[string]int
}

// NewPodStatus initializes PodStatus struct.
func NewPodStatus() *PodStatus {
	p := PodStatus{}
	p.Data = map[string]int{}
	return &p
}

// AppendStatusResult connects status of each pod to one string.
func AppendStatusResult(p PodStatus) string {
	temp := ""
	for s, num := range p.Data {
		temp += s + "(" + strconv.Itoa(num) + "),"
	}
	return strings.TrimRight(temp, ",")
}

// ParsingVersion parses version using regular expression
func ParsingVersion(str string) string {
	isLatest, err := regexp.MatchString("latest", str)
	if err != nil {
		klog.Errorln(err)
	} else if isLatest {
		return "latest"
	}

	r, err := regexp.Compile("[a-z]*[A-Z]*[0-9]*\\.[0-9]*\\.[0-9]*")
	if err != nil {
		klog.Errorln(err)
	}

	return r.FindString(str)
}