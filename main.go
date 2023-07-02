package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mariadb-operator/agent/pkg/filemanager"
	"github.com/mariadb-operator/agent/pkg/logger"
	"github.com/mariadb-operator/init/pkg/config"
	mariadbv1alpha1 "github.com/mariadb-operator/mariadb-operator/api/v1alpha1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	logLevel       string
	logTimeEncoder string
	logDev         bool

	configDir string
	stateDir  string

	mariadbName      string
	mariadbNamespace string
)

func main() {
	flag.StringVar(&logLevel, "log-level", "info", "Log level to use, one of: "+
		"debug, info, warn, error, dpanic, panic, fatal.")
	flag.StringVar(&logTimeEncoder, "log-time-encoder", "epoch", "Log time encoder to use, one of: "+
		"epoch, millis, nano, iso8601, rfc3339 or rfc3339nano")
	flag.BoolVar(&logDev, "log-dev", false, "Enable development logs")

	flag.StringVar(&configDir, "config-dir", "/etc/mysql/mariadb.conf.d", "The directory that contains MariaDB configuration files")
	flag.StringVar(&stateDir, "state-dir", "/var/lib/mysql", "The directory that contains MariaDB state files")

	flag.StringVar(&mariadbName, "mariadb-name", "", "The name of the MariaDB to be initialized")
	flag.StringVar(&mariadbNamespace, "mariadb-namespace", "", "The namespace of the MariaDB to be initialized")

	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), []os.Signal{
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGKILL,
		syscall.SIGHUP,
		syscall.SIGQUIT}...,
	)
	defer cancel()

	logger, err := logger.NewLogger(
		logger.WithLogLevel(logLevel),
		logger.WithTimeEncoder(logTimeEncoder),
		logger.WithDevelopment(logDev),
	)
	if err != nil {
		log.Fatalf("error creating logger: %v", err)
	}
	logger.Info("Staring init")

	restConfig, err := restConfig()
	if err != nil {
		logger.Error(err, "Error getting Kubernetes config")
		os.Exit(1)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		logger.Error(err, "Error creating Kubernetes clientset")
		os.Exit(1)
	}
	mdb, err := mariadb(ctx, mariadbName, mariadbNamespace, clientset)
	if err != nil {
		logger.Error(err, "Error getting MariaDB")
		os.Exit(1)
	}

	fileManager, err := filemanager.NewFileManager(configDir, stateDir)
	if err != nil {
		logger.Error(err, "Error creating file manager")
		os.Exit(1)
	}
	configBytes, err := config.NewConfigFile(mdb).Marshal()
	if err != nil {
		logger.Error(err, "Error getting galera config")
		os.Exit(1)
	}
	logger.Info("Configuring Galera")
	if err := fileManager.WriteConfigFile(config.ConfigFileName, configBytes); err != nil {
		logger.Error(err, "Error writing galera config")
		os.Exit(1)
	}
	logger.Info("Configuring bootstrap")
	if err := fileManager.WriteConfigFile(config.BootstrapFileName, config.BootstrapFile); err != nil {
		logger.Error(err, "Error writing bootstrap config")
		os.Exit(1)
	}
	logger.Info("Init done")
}

func restConfig() (*rest.Config, error) {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

func mariadb(ctx context.Context, name, namespace string, clientset *kubernetes.Clientset) (*mariadbv1alpha1.MariaDB, error) {
	path := fmt.Sprintf("/apis/mariadb.mmontes.io/v1alpha1/namespaces/%s/mariadbs/%s", namespace, name)
	bytes, err := clientset.
		RESTClient().
		Get().
		AbsPath(path).
		DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("error requesting '%s' MariaDB in namespace '%s': %v", name, namespace, err)
	}
	var mdb mariadbv1alpha1.MariaDB
	if err := json.Unmarshal(bytes, &mdb); err != nil {
		return nil, fmt.Errorf("error decoding MariaDB: %v", err)
	}
	return &mdb, nil
}
