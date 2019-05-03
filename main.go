package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/streadway/amqp"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"

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
	logger    *logrus.Logger
	gamesGPRC models.GamesClient
	authGPRC  models.AuthClient

	rabbitChannel *amqp.Channel
)

func connectClient(consulCli *consulapi.Client, service string) (*grpc.ClientConn, error) {
	nameResolver, servers, err := balancer.NewNameResolver(consulCli, service)
	if err != nil {
		return nil, errors.Wrap(err, "can not create name resolver")

	}

	grpcConn, err := grpc.Dial(
		servers[0],
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithBalancer(grpc.RoundRobin(nameResolver)),
	)
	if err != nil {
		return nil, errors.Wrap(err, "can not connect to auth grpc")
	}

	nameResolver.LoadServers(servers)
	go balancer.OnlineServiceDiscovery(consulCli, nameResolver, service, servers, 15*time.Second)

	return grpcConn, nil
}

func main() {
	var err error
	logger, err = logging.NewLogger(os.Stdout, os.Getenv("LOGENTRIESRUS_TOKEN"))
	if err != nil {
		log.Printf("can not create logger: %s", err)
		return
	}

	consulConfig := consulapi.DefaultConfig()
	consulConfig.Address = os.Getenv("CONSUL_ADDR")
	consul, err := consulapi.NewClient(consulConfig)
	if err != nil {
		logger.Errorf("can not connect consul service: %s", err)
		return
	}

	httpPort, _, err := balancer.GetPorts("warscript-bots/bounds", "warscript-bots", consul)
	if err != nil {
		logger.Errorf("can not find empry port: %s", err)
		return
	}

	vaultConfig := vaultapi.DefaultConfig()
	vaultConfig.Address = os.Getenv("VAULT_ADDR")
	vault, err := vaultapi.NewClient(vaultConfig)
	if err != nil {
		logger.Errorf("can not connect vault service: %s", err)
		return
	}

	vault.SetToken(os.Getenv("VAULT_TOKEN"))
	postgreConf, err := vault.Logical().Read("warscript-bots/postgres")
	if err != nil || postgreConf == nil || len(postgreConf.Warnings) != 0 {
		logger.Errorf("can read warscript-bots/postges key: %+v; %+v", err, postgreConf)
		return
	}
	rabbitConf, err := vault.Logical().Read("warscript-bots/rabbitmq")
	if err != nil || rabbitConf == nil || len(rabbitConf.Warnings) != 0 {
		logger.Errorf("can read warscript-bots/rabbitmq key: %+v; %+v", err, rabbitConf)
		return
	}

	httpServiceID := fmt.Sprintf("warscript-bots-http:%d", httpPort)
	err = consul.Agent().ServiceRegister(&consulapi.AgentServiceRegistration{
		ID:      httpServiceID,
		Name:    "warscript-bots-http",
		Port:    httpPort,
		Address: "127.0.0.1",
	})
	defer func() {
		err = consul.Agent().ServiceDeregister(httpServiceID)
		if err != nil {
			logger.Errorf("can not derigister http service: %s", err)
		}
		logger.Info("successfully derigister http service")
	}()

	pgxConn, err = postgresql.Connect(postgreConf.Data["user"].(string), postgreConf.Data["pass"].(string),
		postgreConf.Data["host"].(string), postgreConf.Data["port"].(string), postgreConf.Data["database"].(string))
	if err != nil {
		logger.Errorf("can not connect to postgresql database: %s", err.Error())
		return
	}
	defer pgxConn.Close()

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

	authGPRCConn, err := connectClient(consul, "warscript-users-grpc")
	if err != nil {
		logger.Errorf("can not connect to auth grpc: %s", err.Error())
		return
	}
	defer authGPRCConn.Close()
	authGPRC = models.NewAuthClient(authGPRCConn)

	// gamesGPRCConn, err := connectClient(consul, "warscript-games")
	// if err != nil {
	// 	logger.Errorf("can not connect to games grpc: %s", err.Error())
	// 	return
	// }
	// defer gamesGPRCConn.Close()
	// gamesGPRC = models.NewGamesClient(gamesGPRCConn)
	gamesGPRC = &LocalGameClient{}

	h = &hub{
		sessions:   make(map[int64]map[string]map[string]chan *BotVerifyStatusMessage),
		broadcast:  make(chan *BotVerifyStatusMessage),
		register:   make(chan *BotVerifyClient),
		unregister: make(chan *BotVerifyClient),
	}
	go h.run()

	r := mux.NewRouter().PathPrefix("/v1").Subrouter()
	r.HandleFunc("/bots", middlewares.WithAuthentication(CreateBot, logger, authGPRC)).Methods("POST")
	r.HandleFunc("/bots", GetBotsList).Methods("GET")
	r.HandleFunc("/bots/verification", middlewares.WithAuthentication(OpenVerifyWS, logger, authGPRC)).Methods("GET")

	logger.Infof("Bots HTTP service successfully started at port %d", httpPort)
	err = http.ListenAndServe(":"+strconv.Itoa(httpPort),
		middlewares.RecoverMiddleware(middlewares.AccessLogMiddleware(r, logger), logger))
	if err != nil {
		logger.Errorf("cant start main server. err: %s", err.Error())
		return
	}
}
