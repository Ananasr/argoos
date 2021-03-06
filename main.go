package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/Smile-SA/argoos/apiutils"
)

// TOKEN global value. If no token set, TOKEN points on nil.
var TOKEN string

// Version changed by deploy script.
var VERSION = "master"

// BadTokenError is raised when header token is not ok
type BadTokenError struct{}

func (b *BadTokenError) Error() string {
	return "Bad Token"
}

// signal handling, if server should stop, cleanup goroutines.
func sig() {
	c := make(chan os.Signal, 0)
	signal.Notify(c, os.Interrupt)

	// Block until a signal is received.
	s := <-c
	apiutils.StopRollout()
	log.Println("Got signal", s)
	os.Exit(0)
}

func checkToken(r *http.Request) error {
	if TOKEN == "" {
		// no token provided in environment or flags,
		// no problem !
		return nil
	}

	token := strings.TrimSpace(r.Header.Get("X-Argoos-Token"))
	if len(token) < 1 {
		return &BadTokenError{}
	}
	if token != TOKEN {
		return &BadTokenError{}
	}
	return nil
}

// Action is sent each time the registry sends an event.
func Action(w http.ResponseWriter, r *http.Request) {
	if err := checkToken(r); err != nil {
		log.Printf("%s %s %s ERROR::%s\n", r.RemoteAddr, r.Method, r.URL, err.Error())
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(err.Error()))
		return
	}

	log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
	c, _ := ioutil.ReadAll(r.Body)
	registry := r.Header.Get("X-Argoos-Registry-Name")
	if apiutils.Verbose {
		log.Println("Registry override:", registry)
	}
	events := apiutils.GetEvents(c, registry)
	for _, e := range events.Events {
		apiutils.ImpactedDeployments(e)
	}
}

// Health return always "ok" with 200 OK. Usefull to check liveness.
func Health(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s %s\n", r.RemoteAddr, r.Method, r.URL)
	w.Write([]byte("ok\n"))
}

func main() {
	host := ":3000"
	var servercert, serverkey string

	askedVersion := false

	flag.BoolVar(&askedVersion, "version", askedVersion, "Print argoos version and exit")

	if v := os.Getenv("KUBE_MASTER_URL"); len(v) > 0 {
		apiutils.KubeMasterURL = v
	}

	if v := os.Getenv("INCLUSTER"); len(v) > 0 {
		switch strings.ToUpper(v) {
		case "FALSE", "NO", "0":
			apiutils.InCluster = false
		}
	}

	if v := os.Getenv("SKIP_SSL_VERIFICATION"); len(v) > 0 {
		switch strings.ToUpper(v) {
		case "FALSE", "NO", "0":
			apiutils.SkipSSLVerification = false
		}
	}

	if v := os.Getenv("VERBOSE"); len(v) > 0 {
		switch strings.ToUpper(v) {
		case "TRUE", "YES", "1":
			apiutils.Verbose = true
		}
	}

	// Certificates
	if v := os.Getenv("CA_FILE"); len(v) > 0 {
		apiutils.CAFile = v
	}

	if v := os.Getenv("CERT_FILE"); len(v) > 0 {
		apiutils.CertFile = v
	}

	if v := os.Getenv("KEY_FILE"); len(v) > 0 {
		apiutils.KeyFile = v
	}

	if v := os.Getenv("SERVER_CERT"); len(v) > 0 {
		servercert = v
	}

	if v := os.Getenv("SERVER_KEY"); len(v) > 0 {
		serverkey = v
	}

	if v := os.Getenv("LISTEN"); len(v) > 0 {
		host = v
	}

	if v := os.Getenv("TOKEN"); len(v) > 0 {
		TOKEN = strings.TrimSpace(v)
	}

	flag.BoolVar(&apiutils.InCluster,
		"incluster",
		apiutils.InCluster,
		"if true, argoos will contact kubernetes internally."+
			"That's the default if you're launching argoos as a Kubernetes application"+
			"Note that in that case, you can avoid to set kube-master url while argoos will find it itself.")

	flag.BoolVar(&apiutils.Verbose,
		"verbose",
		apiutils.Verbose,
		"Be verbose")
	flag.StringVar(&apiutils.KubeMasterURL,
		"master",
		apiutils.KubeMasterURL,
		"Kube master scheme://host:port")
	flag.BoolVar(&apiutils.SkipSSLVerification,
		"skip-ssl-verification",
		apiutils.SkipSSLVerification,
		"Skip SSL verification for kubernetes api")
	flag.StringVar(&host,
		"listen",
		host,
		"Listen interface, could be host:port, or :port")

	// certs
	flag.StringVar(&apiutils.CAFile,
		"ca-file",
		apiutils.CAFile,
		"Certificate Authority certificate file path (only if using https to contact kubernetes api)")
	flag.StringVar(&apiutils.CertFile,
		"cert-file",
		apiutils.CertFile,
		"Client certificate file path (client authentication only)")
	flag.StringVar(&apiutils.KeyFile,
		"key-file",
		apiutils.KeyFile,
		"Client private key file path (client authentication only)")

	// argoos can serve https
	flag.StringVar(&servercert,
		"server-cert",
		servercert,
		"Server certificate to serve SSL")

	flag.StringVar(&serverkey,
		"server-key",
		serverkey,
		"Server key to server SSL")

	flag.StringVar(&TOKEN,
		"token",
		TOKEN,
		"Token that should be sent by docker registry to be authorized. If set, you must add token in X-Argoos-Token header.")

	flag.Parse()

	if askedVersion {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	apiutils.Config()
	go sig()
	apiutils.StartRollout()

	log.Println("Starting")

	http.HandleFunc("/healthz", Health)
	http.HandleFunc("/event", Action)

	if len(serverkey) > 0 && len(servercert) > 0 {
		log.Fatal(http.ListenAndServeTLS(host, servercert, serverkey, nil))
	} else {
		log.Fatal(http.ListenAndServe(host, nil))
	}

}
