package member

func (pms *PDMemberSet) updateMembers(etcdcli *clientv3.Client) error {
	ctx, _ := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	resp, err := etcdcli.MemberList(ctx)
	if err != nil {
		return err
	}
	ms = etcdutil.MemberSet{}
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
	case needUpgrade(pods, pms.spec):
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

	log.Infof("Expected membership: %s", pms.MS)

	unknownMembers := running.Diff(members)
	if unknownMembers.Size() > 0 {
		logInfo("Removing unexpected pods:", unknownMembers)
		for _, m := range unknownMembers {
			if err := pms.removePodAndService(m.Name); err != nil {
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
