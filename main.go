package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/streadway/amqp"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"

	"github.com/HotCodeGroup/warscript-utils/balancer"
	"github.com/HotCodeGroup/warscript-utils/logging"
	"github.com/HotCodeGroup/warscript-utils/middlewares"
	"github.com/HotCodeGroup/warscript-utils/models"
	"github.com/HotCodeGroup/warscript-utils/postgresql"
	"github.com/HotCodeGroup/warscript-utils/rabbitmq"

	consulapi "github.com/hashicorp/consul/api"
	vaultapi "github.com/hashicorp/vault/api"
)

var (
	logger     *logrus.Logger
	gamesGPRC  models.GamesClient
	authGPRC   models.AuthClient
	notifyGRPC models.NotifyClient

	rabbitChannel *amqp.Channel

	pqConn *sql.DB
)

func deregisterService(consul *consulapi.Client, id string) {
	err := consul.Agent().ServiceDeregister(id)
	if err != nil {
		logger.Errorf("can not derigister %s service: %s", id, err)
	}
	logger.Infof("successfully derigister %s service", id)
}

//nolint: gocyclo
func main() {
	var err error
	logger, err = logging.NewLogger(os.Stdout, os.Getenv("LOGENTRIESRUS_TOKEN"))
	if err != nil {
		log.Printf("can not create logger: %s", err)
		return
	}

	// коннектим консул
	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = os.Getenv("CONSUL_ADDR")
	consul, err := consulapi.NewClient(consulConfig)
	if err != nil {
		logger.Errorf("can not connect consul service: %s", err)
		return
	}

	// коннектим волт
	vaultConfig := vaultapi.DefaultConfig()
	vaultConfig.Address = os.Getenv("VAULT_ADDR")
	vault, err := vaultapi.NewClient(vaultConfig)
	if err != nil {
		logger.Errorf("can not connect vault service: %s", err)
		return
	}
	vault.SetToken(os.Getenv("VAULT_TOKEN"))

	httpPort, _, err := balancer.GetPorts("warscript-bots/bounds", "warscript-bots", consul)
	if err != nil {
		logger.Errorf("can not find empry port: %s", err)
		return
	}

	// получаем конфиг на постгрес и стартуем
	postgreConf, err := vault.Logical().Read("warscript-bots/postgres")
	if err != nil || postgreConf == nil || len(postgreConf.Warnings) != 0 {
		logger.Errorf("can read warscript-bots/postges key: %+v; %+v", err, postgreConf)
		return
	}

	pqConn, err = postgresql.Connect(postgreConf.Data["user"].(string), postgreConf.Data["pass"].(string),
		postgreConf.Data["host"].(string), postgreConf.Data["port"].(string), postgreConf.Data["database"].(string))
	if err != nil {
		logger.Errorf("can not connect to postgresql database: %s", err.Error())
		return
	}
	defer pqConn.Close()

	// получаем конфиг на rabbit и стартуем
	rabbitConf, err := vault.Logical().Read("warscript-bots/rabbitmq")
	if err != nil || rabbitConf == nil || len(rabbitConf.Warnings) != 0 {
		logger.Errorf("can read warscript-bots/rabbitmq key: %+v; %+v", err, rabbitConf)
		return
	}

	rabbitConn, err := rabbitmq.Connect(rabbitConf.Data["user"].(string), rabbitConf.Data["pass"].(string),
		rabbitConf.Data["host"].(string), rabbitConf.Data["port"].(string))
	if err != nil {
		logger.Errorf("can not connect to rabbitmq: %s", err.Error())
		return
	}
	defer rabbitConn.Close()

	rabbitChannel, err = rabbitConn.Channel()
	if err != nil {
		logger.Errorf("can not create rabbitmq channel: %s", err.Error())
		return
	}
	defer rabbitChannel.Close()

	httpServiceID := fmt.Sprintf("warscript-bots-http:%d", httpPort)
	err = consul.Agent().ServiceRegister(&consulapi.AgentServiceRegistration{
		ID:      httpServiceID,
		Name:    "warscript-bots-http",
		Port:    httpPort,
		Address: "127.0.0.1",
	})
	defer deregisterService(consul, httpServiceID)

	authGPRCConn, err := balancer.ConnectClient(consul, "warscript-users-grpc")
	if err != nil {
		logger.Errorf("can not connect to auth grpc: %s", err.Error())
		return
	}
	defer authGPRCConn.Close()
	authGPRC = models.NewAuthClient(authGPRCConn)

	gamesGPRCConn, err := balancer.ConnectClient(consul, "warscript-games-grpc")
	if err != nil {
		logger.Errorf("can not connect to games grpc: %s", err.Error())
		return
	}
	defer gamesGPRCConn.Close()
	gamesGPRC = models.NewGamesClient(gamesGPRCConn)

	notifyGRPCConn, err := balancer.ConnectClient(consul, "warscript-notify-grpc")
	if err != nil {
		logger.Errorf("can not connect to notify grpc: %s", err.Error())
		return
	}
	defer notifyGRPCConn.Close()
	notifyGRPC = models.NewNotifyClient(notifyGRPCConn)

	logger.Info("starting hub...")
	h = &hub{
		sessions:   make(map[int64]map[string]map[string]chan *BotStatusMessage),
		broadcast:  make(chan *BotStatusMessage),
		register:   make(chan *BotVerifyClient),
		unregister: make(chan *BotVerifyClient),
	}
	go h.run()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Kill, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-signals

		// вырубили http
		deregisterService(consul, httpServiceID)
		// вырубили базули
		rabbitChannel.Close()
		rabbitConn.Close()
		pqConn.Close()
		logger.Info("successfully closed warscript-bots postgreSQL connection")

		logger.Infof("[SIGNAL] Stopped by signal!")
		os.Exit(0)
	}()

	r := mux.NewRouter().PathPrefix("/v1").Subrouter()
	r.HandleFunc("/bots", middlewares.WithAuthentication(CreateBot, logger, authGPRC)).Methods("POST")
	r.HandleFunc("/bots", GetBotsList).Methods("GET")
	r.HandleFunc("/bots/verification", OpenVerifyWS).Methods("GET")

	r.HandleFunc("/matches", GetMatchList).Methods("GET")
	r.HandleFunc("/matches/{match_id:[0-9]+}", GetMatch).Methods("GET")

	http.Handle("/metrics", promhttp.Handler())
	http.Handle("/", middlewares.RecoverMiddleware(middlewares.AccessLogMiddleware(r, logger), logger))

	logger.Infof("Bots HTTP service successfully started at port %d", httpPort)
	go startMatchmaking()
	err = http.ListenAndServe(":"+strconv.Itoa(httpPort), nil)
	if err != nil {
		logger.Errorf("cant start main server. err: %s", err.Error())
		return
	}
}
