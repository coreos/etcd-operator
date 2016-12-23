package member

import (
	"time"

	"k8s.io/kubernetes/pkg/client/unversioned"
)

type pdMember struct {
	name       string
	id         uint64
	perrURLs   []string
	clientURLs []string
}

func (m *pdMember) clientAddr() string {
	if len(m.clientURLs) != 0 {
		return strings.Join(m.clientURLs, ",")
	}

	return fmt.Sprintf("http://%s:2379", m.name)
}

func (m *pdMember) peerAddr() string {
	if len(m.PeerURLs) != 0 {
		return strings.Join(m.peerURLs, ",")
	}

	return fmt.Sprintf("http://%s:2380", m.name)
}

type PDMemberSet struct {
	Name      string
	IDCounter int
	status    Status
	MS        map[string]*pdMember
	Spec      *spec.ClusterSpec
	kubeCli   *unversioned.Client
}

func SeedPDMemberset(s *spec.ClusterSpec, kubeCli *unversioned.Client) (MemberSet, error) {
	pms := &PDMemberSet{
		Name: "pd",
		MS:   make(map[string]*pdMember),
	}
	fName := ms.makeFirstMemberName()

	m := &pdMember{
		name: fName,
	}
	pms.MS[m.name] = m

	pms.kubeCli = kubeCli
	err := pms.CreatePodAndService(fName, nameSpace, s)
	if err != nil {
		return nil, err
	}
	return pms, nil
}

func (pms *PDMemberSet) makeFirstMemberName() string {
	firstMemberName := fmt.Sprintf("%s-%04d", pms.Name, c.IdCounter)
	pms.IdCounter++
	return fisrtMemberName
}

func (pms *PDMemberSet) Diff(other MemberSet) {
	o, ok := other.(*PDMemberSet)
	if !ok {
		log.Errorf
	}
	s := &PDMemberSet{
		MS: make(map[string]*pdMember),
	}

	for n, m := range pms.MS {
		if _, ok := o.MS[n]; !ok {
			s.MS[n] = m
		}
	}
	return s
}

func (pms *PDMemberSet) PeerURLPairs() []string {
	ps := make([]string, 0)
	for _, m := range pms.MS {
		ps = append(ps, fmt.Sprintf("%s=%s", m.name, m.peerAddr()))
	}
	return ps
}

func (pms *PDMemberSet) ClientURLs() []string {
	endpoints := make([]string, 0, len(pms.MS))
	for _, m := range pms.MS {
		endpoints = append(endpoints, m.clientAddr())
	}
	return endpoints
}

func (pms *PDMemberSet) CreatePodAndService(memberName string, nameSpace string, s *spec.ClusterSpec) error {

	if _, err := k8sutil.CreatePDMemberService(pms.kubeCli, memberName, pms.Name, namespace); err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}
	token := ""
	if state == "new" {
		token = uuid.New()
	}
	pod := k8sutil.MakePDPod(m, pms.PeerURLPairs(), memberName, state, token, s)
	_, err := pms.KubeCli.Pods(namespace).Create(pod)
	return err
}

func (pms *PDMemberSet) RemovePodAndService(name string) error {
	err := pms.KubeCli.Services(pms.Namespace).Delete(name)
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	err = pms.KubeCli.Pods(pms.Namespace).Delete(name, k8sapi.NewDeleteOptions(0))
	if err != nil {
		if !k8sutil.IsKubernetesResourceNotFoundError(err) {
			return err
		}
	}
	return nil
}

func (pms *PDMemberSet) SetSpec(s *spec.ClusterSpec) {
	pms.Spec = s
}

func (pms *PDMemberSet) addMember(m *pdMember) {
	pms.MS[m.name] = m
}

func (pms *PDMemberSet) Run(stopC <-chan struct{}, wg *sync.WaitGroup) {
	for {
		select {
		case <-stopC:
			wg.Done()
			return
		case <-time.After(5 * time.Second):
			if pms.spec.Paused {
				log.Info("control is paused, skipping reconcilation")
				continue
			}
			//TODO only pull PD pod
			running, pending, err := k8sutil.pollPods()
			if err != nil {
				log.Errorf("fail to poll pods: %v", err)
				continue
			}
			if len(pending) > 0 {
				log.Info("skip reconciliation: running (%v), pending (%v)", k8sutil.GetPodNames(running), k8sutil.GetPodNames(pending))
				continue
			}
			//TODO  alert
			if running == 0 {

			}

			if err := pms.reconcile(running); err != nil {
				log.Errorf("fail to reconcile: %v", err)
				if isFatalError(err) {
					log.Errorf("exiting for fatal error: %v", err)
					return
				}
			}
		}
	}
}

func (pms *PDMemberSet) Size() {
	return len(pms.MS)
}

func init() {
	RegisterSeedMemberFunc(PD, SeedPDMemberset)
}
