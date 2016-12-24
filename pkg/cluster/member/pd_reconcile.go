package member

func (pms *PDMemberSet) updateMembers(pdcli *clientv3.Client) error {
	ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := cli.MemberList(ctx)
	if err != nil {
		return err
	}
	pms = &pdMemberSet{}
	for _, m := range resp.Members {
		if len(m.Name) == 0 {
			pms.MS = nil
			return fmt.Errorf("the name of member (%x) is empty. Not ready yet. Will retry later", m.ID)
		}
		id := findID(m.Name)
		if id+1 > pms.idCounter {
			pms.idCounter = id + 1
		}

		pms.MS[m.Name] = &pdMember{
			Name:       m.Name,
			ID:         m.ID,
			ClientURLs: m.ClientURLs,
			PeerURLs:   m.PeerURLs,
		}
	}
	return nil
}

func (pms *PDMemberSet) pickOneOldMember(pods []*api.Pod, newVersion string) *pdMember {
	for _, pod := range pods {
		if k8sutil.GetVersion(pod) == newVersion {
			continue
		}
		return &pdMember{Name: pod.Name}
	}
	return nil
}

func (pms *PDMemberSet) upgradeOneMember(m *pdMember) error {
	pod, err := pms.KubeCli.Pods(pms.Namespace).Get(m.Name)
	if err != nil {
		return fmt.Errorf("fail to get pod (%s): %v", m.Name, err)
	}
	log.Info("upgrading the pd member %v from %s to %s", m.Name, k8sutil.GetPDVersion(pod), pms.spec.Version)
	pod.Spec.Containers[0].Image = k8sutil.MakePDImage(pms.spec.Version)
	k8sutil.SetPDVersion(pod, pms.spec.Version)
	_, err = pms.KubeCli.Pods(pms.Namespace).Update(pod)
	if err != nil {
		return fmt.Errorf("fail to update the pd member (%s): %v", m.Name, err)
	}
	c.logger.Infof("finished upgrading the pd member %v", m.Name)
	return nil
}

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
	case needUpgrade(pods, pms.spec.Version):
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
		cli, err := clientv3.New(cfg)
		if err != nil {
			return err
		}
		defer cli.Close()
		if err := pms.updateMembers(cli); err != nil {
			log.Errorf("fail to refresh members: %v", err)
			return err
		}
	}

	log.Infof("Expected membership: %s", pms.MS)

	unknownMembers := running.Diff(members)
	if unknownMembers.Size() > 0 {
		logInfo("Removing unexpected pods:", unknownMembers)
		for _, m := range unknownMembers {
			if err := pms.RemovePodAndService(m.Name); err != nil {
				return err
			}
		}
	}
	L := running.Diff(unknownMembers)

	if L.Size() == pms.Size() {
		return pms.resize()
	}
	//TODO
	if L.Size() < pms.Size()/2+1 {
		log.Info("Disaster recovery")
		return pms.disasterRecovery(L)
	}

	log.Infof("Recovering one member")
	toRecover := pms.Diff(L).PickOne()

	if err := pms.removeMember(toRecover); err != nil {
		return err
	}
	return pms.resize()
}

func (pms *PDMemberSet) resize() error {
	if pms.Size() == pms.spec.Size {
		return nil
	}

	if pms.Size() < pms.spec.Size {
		//TODO selfHosted
		if pms.spec.SelfHosted != nil {
			return pms.addOneSelfHostedMember()
		}

		return pms.addOneMember()
	}

	return pms.removeOneMember()
}

func (pms *PDMemberSet) addOneMember() error {
	cfg := clientv3.Config{
		Endpoints:   c.members.ClientURLs(),
		DialTimeout: constants.DefaultDialTimeout,
	}
	etcdcli, err := clientv3.New(cfg)
	if err != nil {
		return err
	}
	defer etcdcli.Close()

	newMemberName := fmt.Sprintf("%s-%04d", pms.Name, pms.idCounter)
	newMember := &pdMember{name: newMemberName}
	ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.MemberAdd(ctx, []string{newMember.PeerAddr()})
	if err != nil {
		c.logger.Errorf("fail to add new member (%s): %v", newMember.Name, err)
		return err
	}
	newMember.ID = resp.Member.ID
	pms.addMember(newMember)

	if err := pms.CreatePodAndService(newMemberName, "existing", false); err != nil {
		log.Errorf("fail to create member (%s): %v", newMemberName, err)
		return err
	}
	pms.idCounter++
	log.Infof("added member (%s)", newMemberName)
	return nil
}

//TODO
func (pms *PDMemberSet) removeOneMember() error {
}
