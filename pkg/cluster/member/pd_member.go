package member

import (
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
}

func SeedPDMemberset(s *spec.ClusterSpec) (MemberSet, error) {
	pms := &PDMemberSet{
		Name: "pd",
		MS:   make(map[string]*pdMember),
	}
	fName := ms.makeFirstMemberName()

	m := &pdMember{
		name: fName,
	}
	pms.MS[m.name] = m

	err := pms.CreatePodAndService(kubeCli*unversioned.Client, fName, nameSpace, s)
	if err != nil {
		return nil, err
	}
	return pms, nil
}

func (pms *PDMemberSet) makeFirstMemberName() string {
	firstMemberName := fmt.Sprintf("%s-%04d", pms.Name, c.IdCounter)
	c.IdCounter++
	return fisrtMemberName
}

func (pms *PDMemberSet) Diff(other MemberSet) {
	o, ok := other.(*PDMemberSet)
	if !ok {
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

func (pms *PDMemberSet) CreatePodAndService(kubeCli *unversioned.Client, memberName string, nameSpace string, s *spec.ClusterSpec) error {

	if _, err := k8sutil.CreatePDMemberService(kubeCli, memberName, pms.Name, namespace); err != nil {
		if !k8sutil.IsKubernetesResourceAlreadyExistError(err) {
			return err
		}
	}
	token := ""
	if state == "new" {
		token = uuid.New()
	}
	pod := k8sutil.MakePDPod(m, pms.PeerURLPairs(), memberName, state, token, s)
	_, err := c.KubeCli.Pods(namespace).Create(pod)
	return err
}

func (pms *PDMemberSet) SetSpec(s *spec.ClusterSpec) {
	pms.Spec = s
}

func (pms *PDMemberSet) addMember(m *pdMember) {
	pms.MS[m.name] = m
}

func (pms *PDMemberSet) updateMembers() {}

func (pms *PDMemberSet) reconcile(pods []*api.Pod) {
	log.Info("PDMemberSet Start reconciling")
	defer log.Info("PDMemberSet Finish reconciling")

	switch {
	case len(pods) != pms.spec.Size:
		running := &PDMemberSet{}
		for _, pod := range pods {
			m := &pdMember{Name: pod.Name}
			//TODO selfhost support
			running.addMember(m)
		}
		return pms.reconcileSize(running)
	case needUpgrade(pods, c.spec):
		pms.status.upgradeVersionTo(pms.spec.Version)

		m := pickOneOldMember(pods, pms.spec.Version)
		return pms.upgradeOneMember(m)
	default:
		pms.status.setVersion(pms.spec.Version)
		return nil
	}
}

func (pms *PDMemberSet) reconcileSize(running *PDMemberSet) error {
	c.logger.Infof("running members: %s", running)
	if len(pms.MS) == 0 {
		cfg := clientv3.Config{
			Endpoints:   running.ClientURLs(),
			DialTimeout: constants.DefaultDialTimeout,
		}
		etcdcli, err := clientv3.New(cfg)
		if err != nil {
			return err
		}
		defer etcdcli.Close()
		if err := pms.updateMembers(etcdcli); err != nil {
			log.Errorf("fail to refresh members: %v", err)
			return err
		}
	}

	c.logger.Infof("Expected membership: %s", c.members)

	unknownMembers := running.Diff(c.members)
	if unknownMembers.Size() > 0 {
		c.logger.Infof("Removing unexpected pods:", unknownMembers)
		for _, m := range unknownMembers {
			if err := c.removePodAndService(m.Name); err != nil {
				return err
			}
		}
	}
	L := running.Diff(unknownMembers)

	if L.Size() == pms.Size() {
		return c.resize()
	}

	if L.Size() < pms.Size()/2+1 {
		c.logger.Infof("Disaster recovery")
		return c.disasterRecovery(L)
	}

	log.Infof("Recovering one member")
	toRecover := pms.Diff(L).PickOne()

	if err := pms.removeMember(toRecover); err != nil {
		return err
	}
	return pms.resize()
}

func (pms *PDMemberSet) Run(stopC <-chan struct{}, wg *sync.WaitGroup) {
	for {
		select {
		case <-stopC:
			return
		case <-time.After(5 * time.Second):
			if pms.spec.Paused {
				log.Info("control is paused, skipping reconcilation")
				continue
			}
			//TODO
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
				c.logger.Errorf("fail to reconcile: %v", err)
				if isFatalError(err) {
					c.logger.Errorf("exiting for fatal error: %v", err)
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
