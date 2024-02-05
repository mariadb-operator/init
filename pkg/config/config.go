package config

import (
	"bytes"
	"errors"
	"fmt"
	"text/template"

	"github.com/mariadb-operator/agent/pkg/galera"
	mariadbv1alpha1 "github.com/mariadb-operator/mariadb-operator/api/v1alpha1"
)

const (
	ConfigFileName    = "0-galera.cnf"
	BootstrapFileName = galera.BootstrapFileName
)

var BootstrapFile = []byte(`[galera]
wsrep_new_cluster="ON"`)

type ConfigFile struct {
	mariadb *mariadbv1alpha1.MariaDB
}

func NewConfigFile(mariadb *mariadbv1alpha1.MariaDB) *ConfigFile {
	return &ConfigFile{
		mariadb: mariadb,
	}
}

func (c *ConfigFile) Marshal(podName, mariadbRootPassword string) ([]byte, error) {
	galera := c.mariadb.Galera()
	if !galera.Enabled {
		return nil, errors.New("MariaDB Galera not enabled, unable to render config file")
	}
	tpl := createTpl("galera", `[mariadb]
bind-address=0.0.0.0
default_storage_engine=InnoDB
binlog_format=row
innodb_autoinc_lock_mode=2

# Cluster configuration
wsrep_on=ON
wsrep_provider=/usr/lib/galera/libgalera_smm.so
wsrep_cluster_address="{{ .ClusterAddress }}"
wsrep_cluster_name=mariadb-operator
wsrep_slave_threads={{ .Threads }}

# Node configuration
wsrep_node_address="{{ .NodeAddress }}"
wsrep_node_name="{{ .Pod }}"
wsrep_sst_method="{{ .SST }}"
{{- if .SSTAuth }}
wsrep_sst_auth="root:{{ .RootPassword }}"
{{- end }}
`)
	buf := new(bytes.Buffer)
	clusterAddr, err := c.clusterAddress()
	if err != nil {
		return nil, fmt.Errorf("error getting cluster address: %v", err)
	}
	nodeAddr, err := nodeAddress(podName)
	if err != nil {
		return nil, fmt.Errorf("error getting node address. %v", err)
	}
	sst, err := galera.SST.MariaDBFormat()
	if err != nil {
		return nil, fmt.Errorf("error getting SST: %v", err)
	}

	err = tpl.Execute(buf, struct {
		ClusterAddress string
		NodeAddress    string
		Threads        int
		Pod            string
		SST            string
		SSTAuth        bool
		RootPassword   string
	}{
		ClusterAddress: clusterAddr,
		NodeAddress:    nodeAddr,
		Threads:        *galera.ReplicaThreads,
		Pod:            podName,
		SST:            sst,
		SSTAuth:        *galera.SST == mariadbv1alpha1.SSTMariaBackup || *galera.SST == mariadbv1alpha1.SSTMysqldump,
		RootPassword:   mariadbRootPassword,
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *ConfigFile) clusterAddress() (string, error) {
	if c.mariadb.Spec.Replicas == 0 {
		return "", errors.New("at least one replica must be specified to get a valid cluster address")
	}
	return "gcomm://172.18.0.140,172.18.0.141,172.18.0.142", nil
	// pods := make([]string, c.mariadb.Spec.Replicas)
	// for i := 0; i < int(c.mariadb.Spec.Replicas); i++ {
	// 	pods[i] = statefulset.PodFQDNWithService(
	// 		c.mariadb.ObjectMeta,
	// 		i,
	// 		c.mariadb.InternalServiceKey().Name,
	// 	)
	// }
	// return fmt.Sprintf("gcomm://%s", strings.Join(pods, ",")), nil
}

func nodeAddress(podName string) (string, error) {
	if podName == "mariadb-galera-0" {
		return "172.18.0.140", nil
	}
	if podName == "mariadb-galera-1" {
		return "172.18.0.141", nil
	}
	if podName == "mariadb-galera-2" {
		return "172.18.0.142", nil
	}
	return "", errors.New("no matching Pod for node address")
}

func createTpl(name, t string) *template.Template {
	return template.Must(template.New(name).Parse(t))
}
