package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	_ "github.com/go-sql-driver/mysql"
	"github.com/mariadb-operator/agent/pkg/filemanager"
	"github.com/mariadb-operator/agent/pkg/kubeclientset"
	"github.com/mariadb-operator/agent/pkg/logger"
	"github.com/mariadb-operator/init/pkg/config"
	"github.com/mariadb-operator/init/pkg/environment"
	mariadbv1alpha1 "github.com/mariadb-operator/mariadb-operator/api/v1alpha1"
	"github.com/mariadb-operator/mariadb-operator/pkg/statefulset"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
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
	logger.Info("Starting init")

	env, err := environment.GetEnvironment(ctx)
	if err != nil {
		logger.Error(err, "Error getting environment variables")
		os.Exit(1)
	}

	clientset, err := kubeclientset.NewKubeclientSet()
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
	configBytes, err := config.NewConfigFile(mdb).Marshal(env.PodName, env.MariadbRootPassword)
	if err != nil {
		logger.Error(err, "Error getting Galera config")
		os.Exit(1)
	}
	logger.Info("Configuring Galera")
	fmt.Println(string(configBytes))
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
		info, err := os.Stat(path.Join(stateDir, "grastate.dat"))
		if !os.IsNotExist(err) && info.Size() > 0 {
			logger.Info("Already initialized. Init done")
			os.Exit(0)
		}
	}

	idx, err := statefulset.PodIndex(env.PodName)
	if err != nil {
		logger.Error(err, "error getting index from Pod", "pod", env.PodName)
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
	if err := waitForPodReady(ctx, mdb, *idx-1, logger); err != nil {
		logger.Error(err, "Error waiting for previous Pod to be ready", "pod", previousPodName)
		os.Exit(1)
	}
	logger.Info("Init done")
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

func waitForPodReady(ctx context.Context, mariadb *mariadbv1alpha1.MariaDB, podIndex int, logger logr.Logger) error {
	return wait.PollUntilContextCancel(ctx, 1*time.Second, true, func(context.Context) (bool, error) {
		addr := statefulset.PodFQDNWithService(mariadb.ObjectMeta, podIndex, mariadb.InternalServiceKey().Name)
		db, err := sql.Open("mysql", fmt.Sprintf("root:MariaDB11!@tcp(%s:3306)/", addr))
		if err != nil {
			fmt.Println("Error connecting to the database:", err)
			return false, nil
		}
		defer db.Close()

		query := "SELECT variable_value FROM information_schema.global_status WHERE variable_name = 'wsrep_ready'"
		var wsrepReady string
		err = db.QueryRow(query).Scan(&wsrepReady)
		if err != nil {
			fmt.Println("Error executing query:", err)
			return false, nil
		}

		// Check if the variable is ON
		if strings.Contains(wsrepReady, "ON") {
			fmt.Println("Variable is ON")
			return true, nil
		}

		fmt.Println("Variable is not ON")
		return false, nil
	})
}
