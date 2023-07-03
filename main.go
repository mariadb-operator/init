package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/mariadb-operator/agent/pkg/filemanager"
	"github.com/mariadb-operator/agent/pkg/logger"
	"github.com/mariadb-operator/init/pkg/config"
	mariadbv1alpha1 "github.com/mariadb-operator/mariadb-operator/api/v1alpha1"
	mariadbpod "github.com/mariadb-operator/mariadb-operator/pkg/pod"
	"github.com/mariadb-operator/mariadb-operator/pkg/statefulset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	logLevel         string
	logTimeEncoder   string
	logDev           bool
	configDir        string
	stateDir         string
	mariadbName      string
	mariadbNamespace string
)

type environment struct {
	podName             string
	mariadbRootPassword string
}

func main() {
	flag.StringVar(&logLevel, "log-level", "info", "Log level to use, one of: debug, info, warn, error, dpanic, panic, fatal.")
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
		log.Fatalf("Error creating logger: %v", err)
	}
	logger.Info("Statring init")

	env, err := env()
	if err != nil {
		logger.Error(err, "Missing environment variables")
		os.Exit(1)
	}

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
	configBytes, err := config.NewConfigFile(mdb).Marshal(env.podName, env.mariadbRootPassword)
	if err != nil {
		logger.Error(err, "Error getting Galera config")
		os.Exit(1)
	}
	logger.Info("Configuring Galera")
	if err := fileManager.WriteConfigFile(config.ConfigFileName, configBytes); err != nil {
		logger.Error(err, "Error writing Galera config")
		os.Exit(1)
	}

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		logger.Error(err, "Error reading state directory")
		os.Exit(1)
	}
	if len(entries) > 0 {
		logger.Info("Already initialized. Init done")
		os.Exit(0)
	}

	idx, err := statefulset.PodIndex(env.podName)
	if err != nil {
		logger.Error(err, "error getting index from Pod", "pod", env.podName)
		os.Exit(1)
	}
	if *idx == 0 {
		logger.Info("Configuring bootstrap")
		if err := fileManager.WriteConfigFile(config.BootstrapFileName, config.BootstrapFile); err != nil {
			logger.Error(err, "Error writing bootstrap config")
			os.Exit(1)
		}
		logger.Info("Init done")
		os.Exit(0)
	}

	previousPodName, err := previousPodName(mdb, *idx)
	if err != nil {
		logger.Error(err, "Error getting previous Pod")
		os.Exit(1)
	}
	logger.Info("Waiting for previous Pod to be ready", "pod", previousPodName)
	if err := waitForPodReady(ctx, mdb, previousPodName, clientset, logger); err != nil {
		logger.Error(err, "Error waiting for previous Pod to be ready", "pod", previousPodName)
		os.Exit(1)
	}
	logger.Info("Init done")
}

func env() (*environment, error) {
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		return nil, errors.New("environment variable 'POD_NAME' is required")
	}
	mariadbRootPassword := os.Getenv("MARIADB_ROOT_PASSWORD")
	if mariadbRootPassword == "" {
		return nil, errors.New("environment variable 'MARIADB_ROOT_PASSWORD' is required")
	}
	return &environment{
		podName:             podName,
		mariadbRootPassword: mariadbRootPassword,
	}, nil
}

func restConfig() (*rest.Config, error) {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}

func mariadb(ctx context.Context, name, namespace string, clientset *kubernetes.Clientset) (*mariadbv1alpha1.MariaDB, error) {
	path := fmt.Sprintf("/apis/mariadb.mmontes.io/v1alpha1/namespaces/%s/mariadbs/%s", namespace, name)
	bytes, err := clientset.RESTClient().Get().AbsPath(path).DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("error requesting '%s' MariaDB in namespace '%s': %v", name, namespace, err)
	}
	var mdb mariadbv1alpha1.MariaDB
	if err := json.Unmarshal(bytes, &mdb); err != nil {
		return nil, fmt.Errorf("error decoding MariaDB: %v", err)
	}
	return &mdb, nil
}

func previousPodName(mariadb *mariadbv1alpha1.MariaDB, podIndex int) (string, error) {
	if podIndex == 0 {
		return "", fmt.Errorf("Pod '%s' is the first Pod", statefulset.PodName(mariadb.ObjectMeta, podIndex))
	}
	previousPodIndex := podIndex - 1
	return statefulset.PodName(mariadb.ObjectMeta, previousPodIndex), nil
}

func waitForPodReady(ctx context.Context, mariadb *mariadbv1alpha1.MariaDB, name string, clientset *kubernetes.Clientset,
	logger logr.Logger) error {
	return wait.PollImmediateUntilWithContext(ctx, 1*time.Second, func(context.Context) (bool, error) {
		pod, err := clientset.CoreV1().Pods(mariadb.Namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			logger.V(1).Info("Error getting Pod", "err", err)
			return false, nil
		}
		if !mariadbpod.PodReady(pod) {
			logger.V(1).Info("Pod not ready", "pod", previousPodName)
			return false, nil
		}
		logger.V(1).Info("Pod ready", "pod", previousPodName)
		return true, nil
	})
}
