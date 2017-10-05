package controller

import (
	api "github.com/coreos/etcd-operator/pkg/apis/etcd/v1beta2"
)

const (
	tmp = "/tmp"
)

// TODO: remove this and use backend interface for other options (PV, Azure)
func (b *Backup) handleS3(clusterName string, s3 *api.S3Source) error {
	rc, _, err := GetSnap()
	if err != nil {

	}
	defer rc.Close()
	return WriteSnap(rc, be)

	// pods, err := backup.GetRunningPods(b.kubecli, b.namespace, clusterName)
	// if err != nil {
	// 	return err
	// }

	// // TODO support tls
	// m, rev := backup.GetMemberWithMaxRev(pods, nil)
	// ctx, cancel := context.WithTimeout(context.Background(), constants.DefaultRequestTimeout)
	// rc, version, err := backup.GetSnap(ctx, m, nil)
	// cancel()
	// if err != nil {
	// 	return err
	// }

	// awsDir, err := ioutil.TempDir(tmp, "")
	// if err != nil {
	// 	return err
	// }
	// defer os.RemoveAll(awsDir)

	// so, err := backupstorage.SetupAWSConfig(b.kubecli, b.namespace, s3.AWSSecret, awsDir)
	// if err != nil {
	// 	return err
	// }

	// prefix := backupapi.ToS3Prefix(s3.Prefix, b.namespace, clusterName)
	// s3cli, err := backupS3.NewFromSessionOpt(s3.S3Bucket, prefix, *so)
	// if err != nil {
	// 	return err
	// }

	// s3Dir, err := ioutil.TempDir(tmp, "")
	// if err != nil {
	// 	return err
	// }
	// defer os.RemoveAll(s3Dir)

	// be := backup.NewS3Backend(s3cli, s3Dir)
	// _, err = backup.WriteSnap(version, rev, be, rc)
	// return err
}
