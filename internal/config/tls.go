package config

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"mafia/log/lib"
)

type TLSConfig struct {
	CertFile      string
	KeyFile       string
	CAFile        string
	ServerAddress string
	Server        bool
}

func SetupTLSConfig(cfg TLSConfig) (config *tls.Config, err error) {
	config = &tls.Config{}

	if cfg.CertFile != "" && cfg.KeyFile != "" {
		config.Certificates = make([]tls.Certificate, 1)
		config.Certificates[0], err = tls.LoadX509KeyPair(
			cfg.CertFile,
			cfg.KeyFile,
		)
		if err != nil {
			return nil, lib.Wrap(err, "Unable to load key pair")
		}
	}

	if cfg.CAFile != "" {
		b, err := ioutil.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, lib.Wrap(err, "Unable to read CA file")
		}

		ca := x509.NewCertPool()
		ok := ca.AppendCertsFromPEM(b)
		if !ok {
			return nil, lib.Wrap(err, "Unable to parse root certificate")
		}

		if cfg.Server {
			config.ClientCAs = ca
			config.ClientAuth = tls.RequireAndVerifyClientCert
		} else {
			config.RootCAs = ca
		}

		config.ServerName = cfg.ServerAddress
	}

	return
}
