package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/SoftwareDefinedBuildings/mr-plotter/keys"
	etcd "github.com/coreos/etcd/clientv3"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Printf("Usage: %s cert.pem key.pem\n", os.Args[0])
		return
	}

	httpscert, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		log.Fatalf("Could not read HTTPS certificate file: %v", err)
	}
	httpskey, err := ioutil.ReadFile(os.Args[2])
	if err != nil {
		log.Fatalf("Could not read HTTPS certificate file: %v", err)
	}

	hardcoded := &keys.HardcodedTLSCertificate{
		Cert: httpscert,
		Key:  httpskey,
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

	success, err := keys.UpsertHardcodedTLSCertificateAtomically(context.Background(), etcdConn, hardcoded)
	if err != nil {
		log.Fatalf("Could not update hardcoded TLS certificate: %v", err)
	}

	if success {
		log.Println("Success")
	} else {
		log.Println("Already exists")
	}
}
