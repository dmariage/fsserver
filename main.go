package main

import (
	"fmt"
	"log"
	"os"
	"time"
	"flag"
	"encoding/json"
	"crypto/sha256"
)

const ENV_FS_SERVER_PORT = "FS_SERVER_PORT"
const ENV_FS_BASE_DIR    = "FS_BASE_DIR"
const ENV_FS_ADMIN_USERNAME    = "FS_ADMIN_USERNAME"
const ENV_FS_ADMIN_USERPASSWD    = "FS_ADMIN_USERPASSWD"

const realm = "Restricted area"

const defaultServerPort = "5000"
const defaultUserName = "admin"
const defaultUserPassword = "$CrazyUnforgettablePassword?"
const defaultUploadPath = "./tmp"
const defaultReadWriteMode = "rw"

var fsBaseDir = "./fs"
var readWriteMode = defaultReadWriteMode
var config Config
var port = defaultServerPort
var adminUserName = defaultUserName
var adminUserPassword = defaultUserPassword

//type FsPath struct {
//	Path string `json:"path"`
//	Mode string `json:"mode"`
//}
type UserRights struct {
	//LoginH []byte
	Password string `json:"password"`
	//PasswordH []byte
	Paths map[string]string `json:"paths"`
}
type Config struct {
	Users map[string]UserRights `json:"users"`
}

func main() {
	flag.StringVar(&port, "port", getEnv(ENV_FS_SERVER_PORT, defaultServerPort), "Specify the port to listen to")
	flag.StringVar(&fsBaseDir, "dir", getEnv(ENV_FS_BASE_DIR, defaultUploadPath), "Base directory of for exposed FileSystem")
	flag.StringVar(&adminUserName, "user", getEnv(ENV_FS_ADMIN_USERNAME,defaultUserName), "the username for BasicAuthentication admin user")
	flag.StringVar(&adminUserPassword, "passwd", getEnv(ENV_FS_ADMIN_USERPASSWD, defaultUserPassword), "the password for BasicAuthentication admin user")
	flag.StringVar(&readWriteMode,"mode", defaultReadWriteMode, "the global file access mode : 'r' for readonly, 'rw' for readwrite")
	flag.Parse()

	config, _ = LoadConfiguration("./conf/config.json")

	nbUsers := len(config.Users)
	log.Printf("%d users configured", nbUsers)
	for userName, userRights := range config.Users {
		//userRights.LoginH = hasher(userName)
		//userRights.PasswordH = hasher(userRights.Password)

		log.Printf("user: %s with password %s", userName, userRights.Password)
		for path, mode := range userRights.Paths {
			log.Printf("\t- %s (%s)", path, mode)
		}
	}

	userhash := hasher(adminUserName)
	passhash := hasher(adminUserPassword)

	os.MkdirAll(fsBaseDir, os.ModePerm);

	// create a logger, router and server
	logger := log.New(os.Stdout, "", log.LstdFlags)
	router := newRouter(userhash, passhash, realm)
	server := newServer(
		port,
		(middlewares{
			tracing(func() string {
				return fmt.Sprintf("%d", time.Now().UnixNano())
			}),
			logging(logger)}).apply(router),
		logger,
	)

	// run our server
	if err := server.run(); err != nil {
		log.Fatal(err)
	}
}

func LoadConfiguration(file string) (Config, error) {
	var config Config
	configFile, err := os.Open(file)
	defer configFile.Close()
	if err != nil {
		fmt.Println(err.Error())
		return config, err
	}
	jsonParser := json.NewDecoder(configFile)
	jsonParser.Decode(&config)
	return config, nil
}

func hasher(s string) []byte {
	val := sha256.Sum256([]byte(s))
	return val[:]
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		log.Printf("found value '%s' in '%s'\n", value, key)
		return value
	}
	log.Printf("return fallback value '%s'\n", fallback)
	return fallback
}