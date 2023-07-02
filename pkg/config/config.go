package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/mariadb-operator/agent/pkg/galera"
	mariadbv1alpha1 "github.com/mariadb-operator/mariadb-operator/api/v1alpha1"
	ctrlresources "github.com/mariadb-operator/mariadb-operator/controllers/resources"
	"github.com/mariadb-operator/mariadb-operator/pkg/statefulset"
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

func (c *ConfigFile) Marshal() ([]byte, error) {
	tpl := createTpl("galera", `[mysqld]
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
wsrep_node_address="{{ .Pod }}.{{ .Service }}"
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
	sst, err := c.mariadb.Spec.Galera.SST.MariaDBFormat()
	if err != nil {
		return nil, fmt.Errorf("error getting SST: %v", err)
	}
	hostname := os.Getenv("HOSTNAME")
	if hostname == "" {
		return nil, errors.New("HOSTNAME environment variable not found")
	}
	rootPassword := os.Getenv("MARIADB_ROOT_PASSWORD")
	if rootPassword == "" {
		return nil, errors.New("MARIADB_ROOT_PASSWORD environment variable not found")
	}

	err = tpl.Execute(buf, struct {
		ClusterAddress string
		Threads        int
		Pod            string
		Service        string
		SST            string
		SSTAuth        bool
		RootPassword   string
	}{
		ClusterAddress: clusterAddr,
		Threads:        c.mariadb.Spec.Galera.ReplicaThreads,
		Pod:            hostname,
		Service: statefulset.ServiceFQDNWithService(
			c.mariadb.ObjectMeta,
			ctrlresources.InternalServiceKey(c.mariadb).Name,
		),
		SST:          sst,
		SSTAuth:      c.mariadb.Spec.Galera.SST == mariadbv1alpha1.SSTMariaBackup || c.mariadb.Spec.Galera.SST == mariadbv1alpha1.SSTMysqldump,
		RootPassword: rootPassword,
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
	pods := make([]string, c.mariadb.Spec.Replicas)
	for i := 0; i < int(c.mariadb.Spec.Replicas); i++ {
		pods[i] = statefulset.PodFQDNWithService(
			c.mariadb.ObjectMeta,
			i,
			ctrlresources.InternalServiceKey(c.mariadb).Name,
		)
	}
	return fmt.Sprintf("gcomm://%s", strings.Join(pods, ",")), nil
}

func createTpl(name, t string) *template.Template {
	return template.Must(template.New(name).Parse(t))
}
