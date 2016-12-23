package member

import (
	"strconv"
	"strings"
)

func needUpgrade(pods []*api.Pod, newVersion string) bool {
	for _, pod := range pods {
		if k8sutil.GetVersion(pod) == newVersion {
			continue
		}
		return true
	}
	return false
}
func findID(name string) int {
	i := strings.LastIndex(name, "-")
	id, err := strconv.Atoi(name[i+1:])
	if err != nil {
		panic(fmt.Sprintf("TODO: fail to extract valid ID from name (%s): %v", name, err))
	}
	return id
}
