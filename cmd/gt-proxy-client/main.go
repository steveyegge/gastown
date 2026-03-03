// gt-proxy-client is the pass-through binary installed in containers as both `gt` and `bd`.
// When GT_PROXY_URL, GT_PROXY_CERT, GT_PROXY_KEY, and GT_PROXY_CA are all set, it forwards
// os.Args[1:] to the proxy server over mTLS and proxies the response.
// Otherwise it execs the real binary at /usr/local/bin/gt.real (or the path in GT_REAL_BIN).
package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

type execRequest struct {
	Argv []string `json:"argv"`
}

type execResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

func main() {
	// Required environment variables:
	//   GT_PROXY_URL  — proxy base URL (e.g. https://172.17.0.1:9876)
	//   GT_PROXY_CERT — path to PEM client cert (issued by proxy CA)
	//   GT_PROXY_KEY  — path to PEM client private key
	//   GT_PROXY_CA   — path to PEM proxy CA cert (used to verify server cert)
	// Optional:
	//   GT_REAL_BIN   — fallback binary path (default /usr/local/bin/gt.real)
	proxyURL := os.Getenv("GT_PROXY_URL")
	certFile := os.Getenv("GT_PROXY_CERT")
	keyFile := os.Getenv("GT_PROXY_KEY")
	// GT_PROXY_CA is the CA cert for the proxy's server TLS cert.
	// This is the same CA cert as GIT_SSL_CAINFO (which git uses to trust the proxy),
	// but passed separately so the Go HTTP client can also trust the proxy server cert.
	caFile := os.Getenv("GT_PROXY_CA")

	if proxyURL == "" || certFile == "" || keyFile == "" || caFile == "" {
		// One or more proxy env vars unset — not in sandboxed mode, exec the real binary silently.
		execReal()
		return
	}

	// Build mTLS client.
	clientCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gt-proxy-client: load client cert: %v\n", err)
		os.Exit(1)
	}

	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gt-proxy-client: read CA: %v\n", err)
		os.Exit(1)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		fmt.Fprintf(os.Stderr, "gt-proxy-client: invalid CA PEM\n")
		os.Exit(1)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      pool,
	}

	httpClient := &http.Client{
		Timeout:   5 * time.Minute,
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}

	// Determine argv: prepend the binary name so the server knows which tool we are.
	argv := os.Args // os.Args[0] is the binary path; the server needs the tool name as argv[0].
	// Replace argv[0] with the tool name (gt or bd) based on the binary name.
	toolName := toolNameFromArg0(os.Args[0])
	argv = append([]string{toolName}, os.Args[1:]...)

	body, err := json.Marshal(execRequest{Argv: argv})
	if err != nil {
		fmt.Fprintf(os.Stderr, "gt-proxy-client: encode request: %v\n", err)
		os.Exit(1)
	}

	resp, err := httpClient.Post(proxyURL+"/v1/exec", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "gt-proxy-client: proxy request failed: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		fmt.Fprintf(os.Stderr, "gt-proxy-client: server error %d: %s\n", resp.StatusCode, msg)
		os.Exit(1)
	}

	var result execResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Fprintf(os.Stderr, "gt-proxy-client: decode response: %v\n", err)
		os.Exit(1)
	}

	if result.Stdout != "" {
		fmt.Fprint(os.Stdout, result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, result.Stderr)
	}
	os.Exit(result.ExitCode)
}

// toolNameFromArg0 extracts "gt" or "bd" from the argv[0] binary path.
func toolNameFromArg0(arg0 string) string {
	return filepath.Base(arg0)
}

// execReal replaces the current process with the real binary.
func execReal() {
	realBin := os.Getenv("GT_REAL_BIN")
	if realBin == "" {
		realBin = "/usr/local/bin/gt.real"
	}
	if err := syscall.Exec(realBin, os.Args, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "gt-proxy-client: exec %s: %v\n", realBin, err)
		os.Exit(1)
	}
}
