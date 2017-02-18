package keys

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"
)

// This was taken from https://golang.org/src/crypto/tls/generate_cert.go.
// All credit to the Go Authors.
func pemBlockForKey(priv interface{}) (*pem.Block, error) {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}, nil
	case *ecdsa.PrivateKey:
		b, err := x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, err
		}
		return &pem.Block{Type: "EC PRIVATE KEY", Bytes: b}, nil
	default:
		return nil, nil
	}
}

// SerializeCertificate serializes a TLS certificate into the cert and key PEM
// files.
func SerializeCertificate(certificate *tls.Certificate) (*pem.Block, *pem.Block, error) {
	certpem := &pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}
	keypem, err := pemBlockForKey(certificate.PrivateKey)
	return certpem, keypem, err
}

// SelfSignedCertificate generates a self-signed certificate.
// Much of this is from https://golang.org/src/crypto/tls/generate_cert.go.
// All credit to the Go Authors.
func SelfSignedCertificate(dnsNames []string) (*pem.Block, *pem.Block, error) {
	privkey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()

	template := &x509.Certificate{
		IsCA:         true,
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:         "default.autocert.smartgrid.store",
			Country:            []string{"United States of America"},
			Organization:       []string{"University of California, Berkeley"},
			OrganizationalUnit: []string{"Software Defined Buildings"},
			Locality:           []string{"Berkeley"},
			Province:           []string{"California"},
			StreetAddress:      []string{"410 Soda Hall"},
		},
		NotBefore: now.Add(-time.Hour),
		NotAfter:  now.Add(time.Hour * 24 * 365),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,

		DNSNames: dnsNames,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privkey.PublicKey, privkey)
	if err != nil {
		return nil, nil, err
	}

	cert := &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}
	key, err := pemBlockForKey(privkey)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}
