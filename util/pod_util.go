package util

import (
	"encoding/json"
	"fmt"
	"github.com/golang/glog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "k8s.io/client-go/kubernetes"
	api "k8s.io/client-go/pkg/api/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/apimachinery/pkg/api/resource"
)

//TODO: check which fields should be copied
func CopyPodInfo(oldPod, newPod *api.Pod) {
	//1. typeMeta
	newPod.TypeMeta = oldPod.TypeMeta

	//2. objectMeta
	newPod.ObjectMeta = oldPod.ObjectMeta
	newPod.SelfLink = ""
	newPod.ResourceVersion = ""
	newPod.Generation = 0
	newPod.CreationTimestamp = metav1.Time{}
	newPod.DeletionTimestamp = nil
	newPod.DeletionGracePeriodSeconds = nil

	//3. podSpec
	spec := oldPod.Spec
	spec.Hostname = ""
	spec.Subdomain = ""
	spec.NodeName = ""

	newPod.Spec = spec
	return
}

func CopyPodWithoutLabel(oldPod, newPod *api.Pod) {
	CopyPodInfo(oldPod, newPod)

	// set Labels and OwnerReference to be empty
	newPod.Labels = make(map[string]string)
	newPod.OwnerReferences = []metav1.OwnerReference{}
}

func GenNewPodName(name string) string {
	return name + "-1"
}

func AddLabels(pod *api.Pod, labels map[string]string) {
	pod.Labels = labels
}

//--------------------------

func printPods(pods *api.PodList) {
	fmt.Printf("api version:%s, kind:%s, r.version:%s\n",
		pods.APIVersion,
		pods.Kind,
		pods.ResourceVersion)

	for i := range pods.Items {
		pod := &(pods.Items[i])
		fmt.Printf("%s/%s, phase:%s, node.Name:%s, host:%s\n",
			pod.Namespace,
			pod.Name,
			pod.Status.Phase,
			pod.Spec.NodeName,
			pod.Status.HostIP)
	}
}

func ListPod(client *kclient.Clientset) {
	pods, err := client.CoreV1().Pods(api.NamespaceAll).List(metav1.ListOptions{})
	if err != nil {
		panic(err.Error())
	}
	fmt.Printf("There are %d pods in the cluster\n", len(pods.Items))
	printPods(pods)

	glog.V(2).Info("test finish")
}

func copyPodInfoX(oldPod, newPod *api.Pod) {
	//1. typeMeta -- full copy
	newPod.Kind = oldPod.Kind
	newPod.APIVersion = oldPod.APIVersion

	//2. objectMeta -- partial copy
	newPod.Name = oldPod.Name
	newPod.GenerateName = oldPod.GenerateName
	newPod.Namespace = oldPod.Namespace
	//newPod.SelfLink = oldPod.SelfLink
	newPod.UID = oldPod.UID
	//newPod.ResourceVersion = oldPod.ResourceVersion
	//newPod.Generation = oldPod.Generation
	//newPod.CreationTimestamp = oldPod.CreationTimestamp

	//NOTE: Deletion timestamp and gracePeriod will be set by system when to delete it.
	//newPod.DeletionTimestamp = oldPod.DeletionTimestamp
	//newPod.DeletionGracePeriodSeconds = oldPod.DeletionGracePeriodSeconds

	newPod.Labels = oldPod.Labels
	newPod.Annotations = oldPod.Annotations
	newPod.OwnerReferences = oldPod.OwnerReferences
	newPod.Initializers = oldPod.Initializers
	newPod.Finalizers = oldPod.Finalizers
	newPod.ClusterName = oldPod.ClusterName

	//3. podSpec -- full copy with modifications
	spec := oldPod.Spec
	spec.Hostname = ""
	spec.Subdomain = ""
	spec.NodeName = ""

	newPod.Spec = spec

	//4. status: won't copy status
}

func ParseParentInfo(pod *api.Pod) (string, string, error) {
	//1. check ownerReferences:
	if pod.OwnerReferences != nil && len(pod.OwnerReferences) > 0 {
		for _, owner := range pod.OwnerReferences {
			if *owner.Controller {
				return owner.Kind, owner.Name, nil
			}
		}
	}

	glog.V(3).Infof("cannot find pod-%v/%v parent by OwnerReferences.", pod.Namespace, pod.Name)

	//2. check annotations:
	if pod.Annotations != nil && len(pod.Annotations) > 0 {
		key := "kubernetes.io/created-by"
		if value, ok := pod.Annotations[key]; ok {
			var ref api.SerializedReference

			if err := json.Unmarshal([]byte(value), &ref); err != nil {
				err = fmt.Errorf("failed to decode parent annoation:%v\n[%v]", err.Error(), value)
				glog.Error(err.Error())
				return "", "", err
			}

			return ref.Reference.Kind, ref.Reference.Name, nil
		}
	}

	glog.V(3).Infof("cannot find pod-%v/%v parent by Annotations.", pod.Namespace, pod.Name)

	return "", "", nil
}

func GetKubeClient(masterUrl, kubeConfig string) *kclient.Clientset {
	if masterUrl == "" && kubeConfig == "" {
		fmt.Println("must specify masterUrl or kubeConfig.")
		return nil
	}

	var err error
	var config *restclient.Config

	if kubeConfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfig)
	} else {
		config, err = clientcmd.BuildConfigFromFlags(masterUrl, "")
	}

	if err != nil {
		panic(err.Error())
	}

	config.QPS = 20
	config.Burst = 30
	// creates the clientset
	clientset, err := kclient.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset
}

func CheckPodMoveHealth(client *kclient.Clientset, nameSpace, podName, nodeName string) error {
	podClient := client.CoreV1().Pods(nameSpace)

	id := fmt.Sprintf("%v/%v", nameSpace, podName)

	getOption := metav1.GetOptions{}
	pod, err := podClient.Get(podName, getOption)
	if err != nil {
		err = fmt.Errorf("failed ot get Pod-%v: %v", id, err.Error())
		glog.Error(err.Error())
		return err
	}

	if pod.Status.Phase != api.PodRunning {
		err = fmt.Errorf("pod-%v is not running: %v", id, pod.Status.Phase)
		glog.Error(err.Error())
		return err
	}

	if pod.Spec.NodeName != nodeName {
		err = fmt.Errorf("pod-%v is running on another Node (%v Vs. %v)",
			id, pod.Spec.NodeName, nodeName)
		glog.Error(err.Error())
		return err
	}

	return nil
}

func ParseInputLimit(cpuLimit, memLimit int) (api.ResourceList, error) {
	if cpuLimit <= 0 && memLimit <= 0 {
		err := fmt.Errorf("cpuLimit=[%d], memLimit=[%d]", cpuLimit, memLimit)
		glog.Error(err)
		return nil, err
	}

	result := make(api.ResourceList)
	if cpuLimit > 0 {
		result[api.ResourceCPU] = resource.MustParse(fmt.Sprintf("%dm", cpuLimit))
	}
	if memLimit > 0 {
		result[api.ResourceMemory] = resource.MustParse(fmt.Sprintf("%dMi", memLimit))
	}

	if cpu, exist := result[api.ResourceCPU]; exist {
		glog.V(2).Infof("cpu: %+v, %v", cpu, cpu.MilliValue())
	}
	if mem, exist := result[api.ResourceMemory]; exist {
		glog.V(2).Infof("memory: %+v, %v", mem, mem.Value())
	}
	return result, nil
}

// update the Pod.Containers[index]'s Resources.Limits and Resources.Requests.
func updateLimit(pod *api.Pod, patchCapacity api.ResourceList, index int) (bool, error) {
	glog.V(2).Infof("begin to update Capacity.")
	changed := false

	if index >= len(pod.Spec.Containers) {
		err := fmt.Errorf("Cannot find container[%d] in pod[%s]", index, pod.Name)
		glog.Error(err)
		return false, err
	}
	container := &(pod.Spec.Containers[index])

	//1. get the original capacities
	result := make(api.ResourceList)
	for k, v := range container.Resources.Limits {
		result[k] = v
	}

	//2. apply the patch
	for k, v := range patchCapacity {
		oldv, exist := result[k]
		if !exist || oldv.Cmp(v) != 0 {
			result[k] = v
			changed = true
			glog.V(2).Infof("Update pod(%v) %v capaicty", k.String(), v)
		}
	}

	if !changed {
		glog.V(2).Infof("Nothing changed of Capacity")
		return false, nil
	}
	container.Resources.Limits = result

	return changed, nil
}