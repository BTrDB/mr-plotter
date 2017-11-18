package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/BTrDB/mr-plotter/keys"
	etcd "github.com/coreos/etcd/clientv3"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Printf("Usage: %s encrypt_key_file mac_key_file\n", os.Args[0])
		return
	}

	encrypt, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		log.Fatalf("Could not read encrypt key file: %v", err)
	}
	mac, err := ioutil.ReadFile(os.Args[2])
	if err != nil {
		log.Fatalf("Could not read mac key file: %v", err)
	}

	sk := &keys.SessionKeys{
		EncryptKey: encrypt,
		MACKey:     mac,
	}

	var etcdEndpoint = os.Getenv("ETCD_ENDPOINT")
	if len(etcdEndpoint) == 0 {
		etcdEndpoint = "localhost:2379"
		log.Printf("ETCD_ENDPOINT is not set; using %s", etcdEndpoint)
	}
	var etcdConfig = etcd.Config{Endpoints: []string{etcdEndpoint}}
	log.Println("Connecting to etcd...")
	etcdConn, err := etcd.New(etcdConfig)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	defer etcdConn.Close()

	err = keys.UpsertSessionKeys(context.Background(), etcdConn, sk)
	if err != nil {
		log.Fatalf("Could not update session keys: %v", err)
	}

	log.Println("Success")
}
